/*
Copyright 2024 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends"
	dockerbackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/docker"
	inmemorybackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/inmemory"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/finalizers"
	clog "sigs.k8s.io/cluster-api/util/log"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DevMachineReconciler reconciles a DevMachine object.
type DevMachineReconciler struct {
	client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	ContainerRuntime container.Runtime
	ClusterCache     clustercache.ClusterCache
	InMemoryManager  inmemoryruntime.Manager
	APIServerMux     *inmemoryserver.WorkloadClustersMux
}

// SetupWithManager will add watches for this controller.
func (r *DevMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.InMemoryManager == nil || r.APIServerMux == nil || r.ContainerRuntime == nil || r.ClusterCache == nil {
		return errors.New("Client, InMemoryManager and APIServerMux, ContainerRuntime and ClusterCache must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "devmachine")
	clusterToDevMachines, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.DevMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.DevMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("DevMachine"))),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&infrav1.DevCluster{},
			handler.EnqueueRequestsFromMapFunc(r.DevClusterToDevMachines),
			builder.WithPredicates(predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog)),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDevMachines),
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				predicates.ClusterPausedTransitionsOrInfrastructureReady(mgr.GetScheme(), predicateLog),
			)),
		).
		WatchesRawSource(r.ClusterCache.GetClusterSource("devmachine", clusterToDevMachines)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=devmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=devmachines/status;devmachines/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machinesets;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile handles DevMachine events.
func (r *DevMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DevMachine instance.
	devMachine := &infrav1.DevMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, devMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, devMachine, infrav1.MachineFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of DevMachine as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, devMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, devMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on DevMachine")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", klog.KObj(machine))
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("DevMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, devMachine); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	if cluster.Spec.InfrastructureRef == nil {
		log.Info("Cluster infrastructureRef is not available yet")
		return ctrl.Result{}, nil
	}

	// Fetch the DevCluster.
	devCluster := &infrav1.DevCluster{}
	devClusterName := client.ObjectKey{
		Namespace: devMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, devClusterName, devCluster); err != nil {
		log.Info("DevCluster is not available yet")
		return ctrl.Result{}, nil
	}

	backendReconciler := r.backendReconcilerFactory(ctx, devMachine)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(devMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Always attempt to Patch the DevMachine object and status after each reconciliation.
	defer func() {
		if err := backendReconciler.PatchDevMachine(ctx, patchHelper, devMachine, util.IsControlPlaneMachine(machine)); err != nil {
			rerr = kerrors.NewAggregate([]error{rerr, err})
		}
	}()

	// Handle deleted machines
	if !devMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return backendReconciler.ReconcileDelete(ctx, cluster, devCluster, machine, devMachine)
	}

	// Handle non-deleted machines
	return backendReconciler.ReconcileNormal(ctx, cluster, devCluster, machine, devMachine)
}

func (r *DevMachineReconciler) backendReconcilerFactory(_ context.Context, devMachine *infrav1.DevMachine) backends.DevMachineBackendReconciler {
	if devMachine.Spec.Backend.InMemory != nil {
		return &inmemorybackend.MachineBackendReconciler{
			Client:          r.Client,
			InMemoryManager: r.InMemoryManager,
			APIServerMux:    r.APIServerMux,
		}
	}
	return &dockerbackend.MachineBackendReconciler{
		Client:           r.Client,
		ContainerRuntime: r.ContainerRuntime,
		ClusterCache:     r.ClusterCache,
	}
}

// DevClusterToDevMachines is a handler.ToRequestsFunc to be used to enqueue
// requests for reconciliation of DevMachines.
func (r *DevMachineReconciler) DevClusterToDevMachines(ctx context.Context, o client.Object) []ctrl.Request {
	result := []ctrl.Request{}
	c, ok := o.(*infrav1.DevCluster)
	if !ok {
		panic(fmt.Sprintf("Expected a DevCluster but got a %T", o))
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		return result
	}

	labels := map[string]string{clusterv1.ClusterNameLabel: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(c.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}
