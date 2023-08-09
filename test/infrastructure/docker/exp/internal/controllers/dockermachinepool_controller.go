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
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	dockerMachinePoolLabel = "docker.cluster.x-k8s.io/machine-pool"
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

	labelFilters := map[string]string{dockerMachinePoolLabel: dockerMachinePool.Name}
	machines, err := docker.ListMachinesByCluster(ctx, cluster, labelFilters)
	if err != nil {
		return errors.Wrapf(err, "failed to list all machines in the cluster")
	}

	for _, machine := range machines {
		key := client.ObjectKey{
			Namespace: dockerMachinePool.Namespace,
			Name:      machine.Name(),
		}
		dockerMachine := &infrav1.DockerMachine{}
		if err := r.Client.Get(ctx, key, dockerMachine); err != nil {
			if err := machine.Delete(ctx); err != nil {
				return errors.Wrapf(err, "failed to delete machine %s", machine.Name())
			}
		}

		if err := r.deleteMachinePoolMachine(ctx, *dockerMachine, *machinePool); err != nil {
			return errors.Wrapf(err, "failed to delete machine %s", dockerMachine.Name)
		}
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

	if err := r.CreateNewInstances(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.DeleteExtraDockerMachines(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.createMissingDockerMachines(ctx, cluster, machinePool, dockerMachinePool); err != nil {
		return ctrl.Result{}, err
	}

	dockerMachineList, err := getDockerMachines(ctx, r.Client, *cluster, *machinePool, *dockerMachinePool)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Derive info from DockerMachines
	dockerMachinePool.Spec.ProviderIDList = []string{}
	for _, dockerMachine := range dockerMachineList.Items {
		if dockerMachine.Spec.ProviderID != nil {
			dockerMachinePool.Spec.ProviderIDList = append(dockerMachinePool.Spec.ProviderIDList, *dockerMachine.Spec.ProviderID)
		}
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

	// TODO: replace with a watch on DockerMachines matching the label.
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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
