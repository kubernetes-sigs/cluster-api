/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controllers implements controller functionality.
package controllers

import (
	"context"
	"fmt"
	"sort"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	// dockerMachinePoolLabel is the label used to identify the DockerMachinePool that is responsible for a Docker container.
	dockerMachinePoolLabel = "docker.cluster.x-k8s.io/machine-pool"

	dockerMachinePoolControllerName = "dockermachinepool-controller"
)

// DockerMachinePoolReconciler reconciles a DockerMachinePool object.
type DockerMachinePoolReconciler struct {
	Client           client.Client
	ContainerRuntime container.Runtime

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	ssaCache        ssa.Cache
	recorder        record.EventRecorder
	externalTracker external.ObjectTracker
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools/status;dockermachinepools/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

func (r *DockerMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DockerMachinePool instance.
	dockerMachinePool := &infraexpv1.DockerMachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachinePool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the MachinePool.
	machinePool, err := utilexp.GetOwnerMachinePool(ctx, r.Client, dockerMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("Waiting for MachinePool Controller to set OwnerRef on DockerMachinePool")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("MachinePool", machinePool.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("DockerMachinePool owner MachinePool is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}

	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine pool with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerMachinePool, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the DockerMachinePool object and status after each reconciliation.
	defer func() {
		if err := patchDockerMachinePool(ctx, patchHelper, dockerMachinePool); err != nil {
			log.Error(err, "Failed to patch DockerMachinePool")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Handle deleted machines
	if !dockerMachinePool.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, cluster, machinePool, dockerMachinePool)
	}

	// Add finalizer and the InfrastructureMachineKind if they aren't already present, and requeue if either were added.
	// We want to add the finalizer here to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	needsPatch := controllerutil.AddFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
	needsPatch = setInfrastructureMachineKind(dockerMachinePool) || needsPatch
	if needsPatch {
		return ctrl.Result{}, nil
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, cluster, machinePool, dockerMachinePool)
}

// SetupWithManager will add watches for this controller.
func (r *DockerMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.ContainerRuntime == nil {
		return errors.New("Client and ContainerRuntime must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "dockermachinepool")
	clusterToDockerMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infraexpv1.DockerMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infraexpv1.DockerMachinePool{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&expv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(utilexp.MachinePoolToInfrastructureMapFunc(ctx,
				infraexpv1.GroupVersion.WithKind("DockerMachinePool"))),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&infrav1.DockerMachine{},
			handler.EnqueueRequestsFromMapFunc(dockerMachineToDockerMachinePool),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDockerMachinePools),
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				//nolint:staticcheck // This usage will be removed when adding v1beta2 status and implementing the Paused condition.
				predicates.ClusterUnpausedAndInfrastructureReady(mgr.GetScheme(), predicateLog),
			)),
		).Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor(dockerMachinePoolControllerName)
	r.externalTracker = external.ObjectTracker{
		Controller:      c,
		Cache:           mgr.GetCache(),
		Scheme:          mgr.GetScheme(),
		PredicateLogger: &predicateLog,
	}
	r.ssaCache = ssa.NewCache("dockermachinepool")

	return nil
}

func (r *DockerMachinePoolReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	log := ctrl.LoggerFrom(ctx)

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return err
	}

	if len(dockerMachineList.Items) > 0 {
		log.Info("DockerMachinePool still has dependent DockerMachines, deleting them first and requeuing", "count", len(dockerMachineList.Items))

		var errs []error

		for _, dockerMachine := range dockerMachineList.Items {
			if !dockerMachine.GetDeletionTimestamp().IsZero() {
				// Don't handle deleted child
				continue
			}

			if err := r.deleteMachinePoolMachine(ctx, dockerMachine); err != nil {
				err = errors.Wrapf(err, "error deleting DockerMachinePool %s/%s: failed to delete %s %s", dockerMachinePool.Namespace, dockerMachinePool.Name, dockerMachine.Namespace, dockerMachine.Name)
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			return kerrors.NewAggregate(errs)
		}
		return nil
	}

	// Once there are no DockerMachines left, ensure there are no Docker containers left behind.
	// This can occur if deletion began after containers were created but before the DockerMachines were created, or if creation of a DockerMachine failed.
	log.Info("DockerMachines have been deleted, deleting any remaining Docker containers")

	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}
	// List Docker containers, i.e. external machines in the cluster.
	externalMachines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster with label \"%s:%s\"", dockerMachinePoolLabel, dockerMachinePool.Name)
	}

	// Providers should similarly ensure that all infrastructure instances are deleted even if the InfraMachine has not been created yet.
	for _, externalMachine := range externalMachines {
		log.Info("Deleting Docker container", "container", externalMachine.Name())
		if err := externalMachine.Delete(ctx); err != nil {
			return errors.Wrapf(err, "failed to delete machine %s", externalMachine.Name())
		}
	}

	// Once all DockerMachines and Docker containers are deleted, remove the finalizer.
	controllerutil.RemoveFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)

	return nil
}

func (r *DockerMachinePoolReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Make sure bootstrap data is available and populated.
	if machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		return ctrl.Result{}, nil
	}

	if machinePool.Spec.Replicas == nil {
		machinePool.Spec.Replicas = ptr.To[int32](1)
	}

	// First, reconcile the Docker containers, but do not delete any as we need to delete the Machine to ensure node cordon/drain.
	// Similarly, providers implementing MachinePool Machines will need to reconcile their analogous infrastructure instances (aside
	// from deletion) before reconciling InfraMachinePoolMachines.
	if err := r.reconcileDockerContainers(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	// Second, once the Docker containers are created, reconcile the DockerMachines. This function creates a DockerMachine for each newly created Docker
	// container, and handles container deletion. Instead of deleting an infrastructure instance directly, we want to delete the owner Machine. This will
	// trigger a cordon and drain of the node, as well as trigger the deletion of the DockerMachine, which in turn causes the Docker container to be deleted.
	// Similarly, providers will need to create InfraMachines for each instance, and instead of deleting instances directly, delete the owner Machine.
	if err := r.reconcileDockerMachines(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the list of DockerMachines to ensure the provider IDs are up to date.
	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Derive providerIDList from the provider ID on each DockerMachine if it exists. The providerID is set by the DockerMachine controller.
	dockerMachinePool.Spec.ProviderIDList = []string{}
	for _, dockerMachine := range dockerMachineList.Items {
		if dockerMachine.Spec.ProviderID != nil {
			dockerMachinePool.Spec.ProviderIDList = append(dockerMachinePool.Spec.ProviderIDList, *dockerMachine.Spec.ProviderID)
		}
	}
	// Ensure the providerIDList is deterministic (getDockerMachines doesn't guarantee a specific order)
	sort.Strings(dockerMachinePool.Spec.ProviderIDList)

	dockerMachinePool.Status.Replicas = int32(len(dockerMachineList.Items))

	if dockerMachinePool.Spec.ProviderID == "" {
		// This is a fake provider ID which does not tie back to any docker infrastructure. In cloud providers,
		// this ID would tie back to the resource which manages the machine pool implementation. For example,
		// Azure uses a VirtualMachineScaleSet to manage a set of like machines.
		dockerMachinePool.Spec.ProviderID = getDockerMachinePoolProviderID(cluster.Name, dockerMachinePool.Name)
	}

	if len(dockerMachinePool.Spec.ProviderIDList) == int(*machinePool.Spec.Replicas) && len(dockerMachineList.Items) == int(*machinePool.Spec.Replicas) {
		dockerMachinePool.Status.Ready = true
		conditions.MarkTrue(dockerMachinePool, expv1.ReplicasReadyCondition)

		return ctrl.Result{}, nil
	}

	return r.updateStatus(ctx, cluster, machinePool, dockerMachinePool, dockerMachineList.Items)
}

func getDockerMachines(ctx context.Context, c client.Client, cluster clusterv1.Cluster, machinePool expv1.MachinePool, dockerMachinePool infraexpv1.DockerMachinePool) (*infrav1.DockerMachineList, error) {
	dockerMachineList := &infrav1.DockerMachineList{}
	labels := map[string]string{
		clusterv1.ClusterNameLabel:     cluster.Name,
		clusterv1.MachinePoolNameLabel: machinePool.Name,
	}
	if err := c.List(ctx, dockerMachineList, client.InNamespace(dockerMachinePool.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return dockerMachineList, nil
}

func getDockerMachinePoolProviderID(clusterName, dockerMachinePoolName string) string {
	return fmt.Sprintf("docker:////%s-dmp-%s", clusterName, dockerMachinePoolName)
}

// setInfrastructureMachineKind sets the infrastructure machine kind in the status if it is not set already to support
// MachinePool Machines and returns a boolean indicating if the status was updated.
func setInfrastructureMachineKind(dockerMachinePool *infraexpv1.DockerMachinePool) bool {
	if dockerMachinePool != nil && dockerMachinePool.Status.InfrastructureMachineKind != "DockerMachine" {
		dockerMachinePool.Status.InfrastructureMachineKind = "DockerMachine"
		return true
	}

	return false
}

// dockerMachineToDockerMachinePool creates a mapping handler to transform DockerMachine to DockerMachinePools.
func dockerMachineToDockerMachinePool(_ context.Context, o client.Object) []ctrl.Request {
	dockerMachine, ok := o.(*infrav1.DockerMachine)
	if !ok {
		panic(fmt.Sprintf("Expected a DockerMachine but got a %T", o))
	}

	for _, ownerRef := range dockerMachine.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(ownerRef.APIVersion)
		if err != nil {
			return nil
		}
		if ownerRef.Kind == "DockerMachinePool" && gv.Group == infraexpv1.GroupVersion.Group {
			return []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      ownerRef.Name,
						Namespace: dockerMachine.Namespace,
					},
				},
			}
		}
	}

	return nil
}

// updateStatus updates the Status field for the MachinePool object.
// It checks for the current state of the replicas and updates the Status of the MachinePool.
func (r *DockerMachinePoolReconciler) updateStatus(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool, dockerMachines []infrav1.DockerMachine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// List the Docker containers. This corresponds to an InfraMachinePool instance for providers.
	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}
	externalMachines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to list all external machines in the cluster")
	}

	externalMachineMap := make(map[string]*docker.Machine)
	for _, externalMachine := range externalMachines {
		externalMachineMap[externalMachine.Name()] = externalMachine
	}
	// We can use reuse getDeletionCandidates to get the list of ready DockerMachines and avoid another API call, even though we aren't deleting them here.
	_, readyMachines, err := r.getDeletionCandidates(ctx, dockerMachines, externalMachineMap, machinePool, dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	readyReplicaCount := len(readyMachines)
	desiredReplicas := int(*machinePool.Spec.Replicas)

	switch {
	// We are scaling up
	case readyReplicaCount < desiredReplicas:
		conditions.MarkFalse(dockerMachinePool, clusterv1.ResizedCondition, clusterv1.ScalingUpReason, clusterv1.ConditionSeverityWarning, "Scaling up MachinePool to %d replicas (actual %d)", desiredReplicas, readyReplicaCount)
	// We are scaling down
	case readyReplicaCount > desiredReplicas:
		conditions.MarkFalse(dockerMachinePool, clusterv1.ResizedCondition, clusterv1.ScalingDownReason, clusterv1.ConditionSeverityWarning, "Scaling down MachinePool to %d replicas (actual %d)", desiredReplicas, readyReplicaCount)
	default:
		// Make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created). This convention is also used by KCP.
		if len(dockerMachines) == readyReplicaCount {
			if conditions.IsFalse(dockerMachinePool, clusterv1.ResizedCondition) {
				log.Info("All the replicas are ready", "replicas", readyReplicaCount)
			}
			conditions.MarkTrue(dockerMachinePool, clusterv1.ResizedCondition)
		}
		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(dockerMachinePool, clusterv1.MachinesCreatedCondition)
	}

	getters := make([]conditions.Getter, 0, len(dockerMachines))
	for i := range dockerMachines {
		getters = append(getters, &dockerMachines[i])
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(dockerMachinePool, expv1.ReplicasReadyCondition, getters, conditions.AddSourceRef())

	return ctrl.Result{}, nil
}

func patchDockerMachinePool(ctx context.Context, patchHelper *patch.Helper, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	conditions.SetSummary(dockerMachinePool,
		conditions.WithConditions(
			expv1.ReplicasReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		dockerMachinePool,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			expv1.ReplicasReadyCondition,
		}},
	)
}
