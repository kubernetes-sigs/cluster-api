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
	"time"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	utilexp "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	infraexpv1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/exp/internal/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/labels/format"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DockerMachinePoolReconciler reconciles a DockerMachinePool object.
type DockerMachinePoolReconciler struct {
	Client           client.Client
	Scheme           *runtime.Scheme
	ContainerRuntime container.Runtime
	Tracker          *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	controller      controller.Controller
	recorder        record.EventRecorder
	externalTracker external.ObjectTracker
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinepools/status;dockermachinepools/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch;create;update;patch;delete
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

	dockerMachinePool.Status.InfrastructureMachineKind = "DockerMachine"
	// Patch now so that the status and selectors are available.
	if err := patchHelper.Patch(ctx, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the DockerMachinePool object and status after each reconciliation.
	defer func() {
		if err := patchDockerMachinePool(ctx, patchHelper, dockerMachinePool); err != nil {
			log.Error(err, "failed to patch DockerMachinePool")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Handle deleted machines
	if !dockerMachinePool.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, cluster, machinePool, dockerMachinePool)
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	if !controllerutil.ContainsFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer) {
		controllerutil.AddFinalizer(dockerMachinePool, infraexpv1.MachinePoolFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle non-deleted machines
	res, err = r.reconcileNormal(ctx, cluster, machinePool, dockerMachinePool)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	if errors.Is(err, remote.ErrClusterLocked) {
		log.V(5).Info("Requeuing because another worker has the lock on the ClusterCacheTracker")
		return ctrl.Result{Requeue: true}, nil
	}
	return res, err
}

// SetupWithManager will add watches for this controller.
func (r *DockerMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	clusterToDockerMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infraexpv1.DockerMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infraexpv1.DockerMachinePool{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))).
		Watches(
			&expv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(utilexp.MachinePoolToInfrastructureMapFunc(
				infraexpv1.GroupVersion.WithKind("DockerMachinePool"), ctrl.LoggerFrom(ctx))),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDockerMachinePools),
			builder.WithPredicates(
				predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
			),
		).Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("dockermachinepool-controller")
	r.externalTracker = external.ObjectTracker{
		Controller: c,
		Cache:      mgr.GetCache(),
	}

	return nil
}

func (r *DockerMachinePoolReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool) error {
	_ = ctrl.LoggerFrom(ctx)

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return err
	}

	// Since nodepools don't persist the instances list, we need to construct it from the list of DockerMachines.
	nodePoolInstances, err := r.initNodePoolMachineStatusList(ctx, dockerMachineList.Items, dockerMachinePool)
	if err != nil {
		return err
	}

	pool, err := docker.NewNodePool(ctx, r.Client, cluster, machinePool, dockerMachinePool, nodePoolInstances)
	if err != nil {
		return errors.Wrap(err, "failed to build new node pool")
	}

	if err := pool.Delete(ctx); err != nil {
		return errors.Wrap(err, "failed to delete all machines in the node pool")
	}

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
		machinePool.Spec.Replicas = pointer.Int32(1)
	}

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("DockerMachineList at beginning of reconcileNormal")
	for _, dockerMachine := range dockerMachineList.Items {
		log.Info("DockerMachine", "dockerMachine", dockerMachine.Name)
	}

	// Since nodepools don't persist the instances list, we need to construct it from the list of DockerMachines.
	log.Info("Initializing node pool machine statuses to call NewNodePool()")
	nodePoolMachineStatuses, err := r.initNodePoolMachineStatusList(ctx, dockerMachineList.Items, dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}
	log.Info("Node pool machine statuses are", "nodePoolMachineStatuses", nodePoolMachineStatuses)

	pool, err := docker.NewNodePool(ctx, r.Client, cluster, machinePool, dockerMachinePool, nodePoolMachineStatuses)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to build new node pool")
	}
	log.Info("Initialized node pool")

	log.Info("Reconciling node pool machines")
	res, err := pool.ReconcileMachines(ctx)
	if err != nil {
		return res, err
	}

	log.Info("Getting result node pool machine statuses")
	nodePoolMachineStatusesResult := pool.GetNodePoolMachineStatuses()
	log.Info("Result node pool machine statuses are", "nodePoolInstancesResult", nodePoolMachineStatusesResult)

	// We want to get this from the node pool result instead of the DockerMachine list as it's more up to date.
	// This is because if the node pool deleted an instance but the Machine or DockerMachine failed to delete, we
	// don't want to include the providerID.
	dockerMachinePool.Spec.ProviderIDList = []string{}
	for _, status := range nodePoolMachineStatusesResult {
		if status.ProviderID != nil {
			dockerMachinePool.Spec.ProviderIDList = append(dockerMachinePool.Spec.ProviderIDList, *status.ProviderID)
		}
	}

	// Delete all DockerMachines that are not in the list of instances returned by the node pool.
	if err := r.DeleteOrphanedDockerMachines(ctx, cluster, machinePool, dockerMachinePool, nodePoolMachineStatusesResult); err != nil {
		conditions.MarkFalse(dockerMachinePool, clusterv1.ReadyCondition, "FailedToDeleteOrphanedMachines", clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, errors.Wrap(err, "failed to delete orphaned machines")
	}

	// Create a DockerMachine for each instance returned by the node pool if it doesn't exist.
	if err := r.CreateDockerMachinesIfNotExists(ctx, cluster, machinePool, dockerMachinePool, nodePoolMachineStatusesResult); err != nil {
		conditions.MarkFalse(dockerMachinePool, clusterv1.ReadyCondition, "FailedToCreateNewMachines", clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, errors.Wrap(err, "failed to create missing machines")
	}

	dockerMachinePool.Status.Replicas = int32(len(dockerMachineList.Items))

	if dockerMachinePool.Spec.ProviderID == "" {
		// This is a fake provider ID which does not tie back to any docker infrastructure. In cloud providers,
		// this ID would tie back to the resource which manages the machine pool implementation. For example,
		// Azure uses a VirtualMachineScaleSet to manage a set of like machines.
		dockerMachinePool.Spec.ProviderID = getDockerMachinePoolProviderID(cluster.Name, dockerMachinePool.Name)
	}

	if len(dockerMachinePool.Spec.ProviderIDList) == int(*machinePool.Spec.Replicas) {
		dockerMachinePool.Status.Ready = true
		conditions.MarkTrue(dockerMachinePool, expv1.ReplicasReadyCondition)

		return ctrl.Result{}, nil
	}

	dockerMachinePool.Status.Ready = false
	conditions.MarkFalse(dockerMachinePool, expv1.ReplicasReadyCondition, expv1.WaitingForReplicasReadyReason, clusterv1.ConditionSeverityInfo, "")

	// if some machine is still provisioning, force reconcile in few seconds to check again infrastructure.
	if res.IsZero() {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return res, nil
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

// DeleteOrphanedDockerMachines deletes any DockerMachines owned by the DockerMachinePool that reference an invalid providerID, i.e. not in the latest copy of the node pool instances.
func (r *DockerMachinePoolReconciler) DeleteOrphanedDockerMachines(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool, nodePoolMachineStatuses []docker.NodePoolMachineStatus) error {
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Deleting orphaned machines", "dockerMachinePool", dockerMachinePool.Name, "namespace", dockerMachinePool.Namespace, "nodePoolMachineStatuses", nodePoolMachineStatuses)
	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return err
	}

	log.V(2).Info("DockerMachineList kind is", "kind", dockerMachineList.GetObjectKind())

	nodePoolMachineNames := map[string]struct{}{}
	for _, nodePoolMachineStatus := range nodePoolMachineStatuses {
		nodePoolMachineNames[nodePoolMachineStatus.Name] = struct{}{}
	}

	for i := range dockerMachineList.Items {
		dockerMachine := &dockerMachineList.Items[i]
		if _, ok := nodePoolMachineNames[dockerMachine.Name]; !ok {
			machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
			// Not found doesn't return an error, so we need to check for nil.
			if err != nil {
				return errors.Wrapf(err, "error getting owner Machine for DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
			}
			if machine == nil {
				// If MachinePool is deleted, the DockerMachine and owner Machine doesn't already exist, then it will never come online.
				if mpDeleted := isMachinePoolDeleted(ctx, r.Client, machinePool); mpDeleted {
					log.Info("DockerMachine is has no parent Machine and MachinePool is deleted", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
					return errors.Errorf("DockerMachine %s/%s has no parent Machine and MachinePool is already deleted", dockerMachine.Namespace, dockerMachine.Name)
					// log.Info("DockerMachine is orphaned and MachinePool is deleted, deleting DockerMachine", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
					// if err := r.Client.Delete(ctx, dockerMachine); err != nil {
					// 	return errors.Wrapf(err, "failed to delete orphaned DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
					// }
				} else { // If the MachinePool still exists, then the Machine will be created, so we need to wait for that to happen.
					log.Info("DockerMachine has no parent Machine, will reattempt deletion once parent Machine is present", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
					return errors.Errorf("DockerMachine %s/%s has no parent Machine, will reattempt deletion once parent Machine is present", dockerMachine.Namespace, dockerMachine.Name)
				}
			}

			log.Info("Deleting Machine for DockerMachine", "machine", machine.Name, "dockerMachine", dockerMachine.Name)
			if err := r.Client.Delete(ctx, machine); err != nil {
				return errors.Wrapf(err, "failed to delete Machine %s/%s", machine.Namespace, machine.Name)
			}
		} else {
			log.V(2).Info("Keeping DockerMachine, nothing to do", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
		}
	}

	return nil
}

// CreateDockerMachinesIfNotExists creates a DockerMachine for each instance returned by the node pool if it doesn't exist.
func (r *DockerMachinePoolReconciler) CreateDockerMachinesIfNotExists(ctx context.Context, cluster *clusterv1.Cluster, machinePool *expv1.MachinePool, dockerMachinePool *infraexpv1.DockerMachinePool, nodePoolMachineStatuses []docker.NodePoolMachineStatus) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Creating missing machines", "dockerMachinePool", dockerMachinePool.Name, "namespace", dockerMachinePool.Namespace, "nodePoolMachineStatuses", nodePoolMachineStatuses)

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return err
	}

	dockerMachineMap := make(map[string]infrav1.DockerMachine)
	for _, dockerMachine := range dockerMachineList.Items {
		dockerMachineMap[dockerMachine.Name] = dockerMachine
	}

	for _, nodePoolMachineStatus := range nodePoolMachineStatuses {
		if _, exists := dockerMachineMap[nodePoolMachineStatus.Name]; exists {
			continue
		}

		labels := map[string]string{
			clusterv1.ClusterNameLabel:     cluster.Name,
			clusterv1.MachinePoolNameLabel: format.MustFormatValue(machinePool.Name),
		}
		dockerMachine := &infrav1.DockerMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   dockerMachinePool.Namespace,
				Name:        nodePoolMachineStatus.Name,
				Labels:      labels,
				Annotations: make(map[string]string),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: dockerMachinePool.APIVersion,
						Kind:       dockerMachinePool.Kind,
						Name:       dockerMachinePool.Name,
						UID:        dockerMachinePool.UID,
					},
					// Note: Since the MachinePool controller has not created its parent Machine yet, we want to set the DockerMachinePool as the owner so it's not orphaned.
				},
			},
			Spec: infrav1.DockerMachineSpec{
				CustomImage:   dockerMachinePool.Spec.Template.CustomImage, // TODO: drop?
				PreLoadImages: dockerMachinePool.Spec.Template.PreLoadImages,
			},
		}

		log.V(2).Info("Instance name for dockerMachine is", "instanceName", nodePoolMachineStatus.Name, "dockerMachine", dockerMachine.GetName())

		if err := r.Client.Create(ctx, dockerMachine); err != nil {
			return errors.Wrap(err, "failed to create dockerMachine")
		}
	}

	return nil
}

func (r *DockerMachinePoolReconciler) initNodePoolMachineStatusList(ctx context.Context, dockerMachines []infrav1.DockerMachine, dockerMachinePool *infraexpv1.DockerMachinePool) ([]docker.NodePoolMachineStatus, error) {
	log := ctrl.LoggerFrom(ctx)

	nodePoolInstances := make([]docker.NodePoolMachineStatus, len(dockerMachines))
	for i := range dockerMachines {
		// Needed to avoid implicit memory aliasing of the loop variable.
		dockerMachine := dockerMachines[i]

		// Try to get the owner machine to see if it has a delete annotation. If it doesn't exist, we'll requeue until it does.
		machine, err := util.GetOwnerMachine(ctx, r.Client, dockerMachine.ObjectMeta)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get owner Machine for DockerMachine %s/%s", dockerMachine.Namespace, dockerMachine.Name)
		}

		hasDeleteAnnotation := false
		if machine != nil {
			if machine.Annotations != nil {
				_, hasDeleteAnnotation = machine.Annotations[clusterv1.DeleteMachineAnnotation]
			}
		} else {
			log.Info("DockerMachine has no parent Machine, return error and requeue", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
			return nil, errors.Errorf("DockerMachine %s/%s has no parent Machine, return error and requeue", dockerMachine.Namespace, dockerMachine.Name)
			// sampleMachine := &clusterv1.Machine{}
			// sampleMachine.APIVersion = clusterv1.GroupVersion.String()
			// sampleMachine.Kind = "Machine"
			// log.Info("DockerMachine has no parent Machine, will set up a watch and initNodePoolMachineStatusList", "dockerMachine", dockerMachine.Name, "namespace", dockerMachine.Namespace)
			// // If machine == nil, then no Machine was found in the ownerRefs at all. Don't block nodepool reconciliation, but set up a Watch() instead.
			// if err := r.externalTracker.Watch(log, sampleMachine, handler.EnqueueRequestsFromMapFunc(r.machineToDockerMachinePoolMapper(ctx, &dockerMachine, dockerMachinePool))); err != nil {
			// 	return nil, errors.Wrapf(err, "failed to set watch for Machines %s/%s", dockerMachine.Namespace, dockerMachine.Name)
			// }
		}

		nodePoolInstances[i] = docker.NodePoolMachineStatus{
			Name:             dockerMachine.Name,
			PrioritizeDelete: hasDeleteAnnotation,
			ProviderID:       dockerMachine.Spec.ProviderID,
		}
	}

	return nodePoolInstances, nil
}

func isMachinePoolDeleted(ctx context.Context, c client.Client, machinePool *expv1.MachinePool) bool {
	mp := &expv1.MachinePool{}
	key := client.ObjectKey{Name: machinePool.Name, Namespace: machinePool.Namespace}
	if err := c.Get(ctx, key, mp); apierrors.IsNotFound(err) || !mp.DeletionTimestamp.IsZero() {
		return true
	}

	return false
}

// machineToDockerMachinePoolMapper is a mapper function that maps an InfraMachine to the MachinePool that owns it.
// This is used to trigger an update of the MachinePool when a InfraMachine is changed.
func (r *DockerMachinePoolReconciler) machineToDockerMachinePoolMapper(_ context.Context, dockerMachine *infrav1.DockerMachine, dockerMachinePool *infraexpv1.DockerMachinePool) func(context.Context, client.Object) []ctrl.Request {
	return func(ctx context.Context, o client.Object) []ctrl.Request {
		machine, ok := o.(*clusterv1.Machine)
		if !ok {
			return nil
		}

		if machine.Spec.InfrastructureRef.Name == dockerMachine.Name {
			return []ctrl.Request{
				{
					NamespacedName: client.ObjectKey{
						Namespace: dockerMachinePool.Namespace,
						Name:      dockerMachinePool.Name,
					},
				},
			}
		}

		return nil
	}
}

func getDockerMachinePoolProviderID(clusterName, dockerMachinePoolName string) string {
	return fmt.Sprintf("docker:////%s-dmp-%s", clusterName, dockerMachinePoolName)
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
