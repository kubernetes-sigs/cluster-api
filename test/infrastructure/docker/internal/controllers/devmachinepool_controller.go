/*
Copyright 2026 The Kubernetes Authors.

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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	capicontrollerutil "sigs.k8s.io/cluster-api/internal/util/controller"
	"sigs.k8s.io/cluster-api/internal/util/ssa"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends"
	dockerbackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/docker"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	devMachinePoolControllerName = "devmachinepool-controller"
)

// DevMachinePoolReconciler reconciles a DevMachinePool object.
type DevMachinePoolReconciler struct {
	Client           client.Client
	ContainerRuntime container.Runtime

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	recorder        record.EventRecorder
	externalTracker external.ObjectTracker
}

// SetupWithManager will add watches for this controller.
func (r *DevMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.ContainerRuntime == nil {
		return errors.New("Client and ContainerRuntime must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "devmachinepool")
	clusterToDevMachinePools, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.DevMachinePoolList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := capicontrollerutil.NewControllerManagedBy(mgr, predicateLog).
		For(&infrav1.DevMachinePool{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(util.MachinePoolToInfrastructureMapFunc(ctx,
				infrav1.GroupVersion.WithKind("DevMachinePool"))),
		).
		Watches(
			&infrav1.DevMachine{},
			handler.EnqueueRequestsFromMapFunc(devMachineToDevMachinePool),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToDevMachinePools),
			predicates.ClusterPausedTransitionsOrInfrastructureProvisioned(mgr.GetScheme(), predicateLog),
		).Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.recorder = mgr.GetEventRecorderFor(devMachinePoolControllerName)
	r.externalTracker = external.ObjectTracker{
		Controller:      c,
		Cache:           mgr.GetCache(),
		Scheme:          mgr.GetScheme(),
		PredicateLogger: &predicateLog,
	}

	return nil
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=devmachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=devmachinepools/status;devmachinepools/finalizers,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

func (r *DevMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	log.Info("Its not called")

	// Fetch the DevMachinePool instance.
	devMachinePool := &infrav1.DevMachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, devMachinePool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the MachinePool.
	machinePool, err := util.GetOwnerMachinePool(ctx, r.Client, devMachinePool.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}

	if machinePool == nil {
		// Note: If ownerRef was not set, there is nothing to delete. Remove finalizer so deletion can succeed.
		if !devMachinePool.DeletionTimestamp.IsZero() {
			if controllerutil.ContainsFinalizer(devMachinePool, infrav1.MachinePoolFinalizer) {
				devMachinePoolWithoutFinalizer := devMachinePool.DeepCopy()
				controllerutil.RemoveFinalizer(devMachinePoolWithoutFinalizer, infrav1.MachinePoolFinalizer)
				if err := r.Client.Patch(ctx, devMachinePoolWithoutFinalizer, client.MergeFrom(devMachinePool)); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "failed to patch DevMachinePool %s", klog.KObj(devMachinePool))
				}
			}
			return ctrl.Result{}, nil
		}

		log.Info("Waiting for MachinePool Controller to set OwnerRef on DevMachinePool")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("MachinePool", machinePool.Name)
	ctx = ctrl.LoggerInto(ctx, log)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("DevMachinePool owner MachinePool is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}

	if cluster == nil {
		log.Info(fmt.Sprintf("Please associate this machine pool with a cluster using the label %s: <name of cluster>", clusterv1.ClusterNameLabel))
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(devMachinePool, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	backendReconciler := r.backendReconcilerFactory()

	// Always attempt to Patch the DevMachinePool object and status after each reconciliation.
	defer func() {
		if err := backendReconciler.PatchDevMachinePool(ctx, patchHelper, devMachinePool); err != nil {
			rerr = kerrors.NewAggregate([]error{rerr, err})
		}
	}()

	// Handle deleted machines
	if !devMachinePool.DeletionTimestamp.IsZero() {
		return backendReconciler.ReconcileDelete(ctx, cluster, machinePool, devMachinePool)
	}

	// Add finalizer and the InfrastructureMachineKind if they aren't already present, and requeue if either were added.
	// We want to add the finalizer here to avoid the race condition between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp is not set.
	needsPatch := controllerutil.AddFinalizer(devMachinePool, infrav1.MachinePoolFinalizer)
	needsPatch = setInfrastructureMachineKindForMachinePool(devMachinePool) || needsPatch
	if needsPatch {
		return ctrl.Result{}, nil
	}

	// Handle non-deleted clusters
	return backendReconciler.ReconcileNormal(ctx, cluster, machinePool, devMachinePool)
}

func (r *DevMachinePoolReconciler) backendReconcilerFactory() backends.DevMachinePoolBackendReconciler {
	return &dockerbackend.MachinePoolBackEndReconciler{
		Client:           r.Client,
		ContainerRuntime: r.ContainerRuntime,
		SsaCache:         ssa.NewCache("devmachinepool"),
	}
}

// devMachineToDevMachinePool creates a mapping handler to transform DevMachine to DevMachinePool.
func devMachineToDevMachinePool(_ context.Context, o client.Object) []ctrl.Request {
	devMachine, ok := o.(*infrav1.DevMachine)
	if !ok {
		panic(fmt.Sprintf("Expected a DevMachine but got a %T", o))
	}

	for _, ownerRef := range devMachine.GetOwnerReferences() {
		gv, err := schema.ParseGroupVersion(ownerRef.APIVersion)
		if err != nil {
			return nil
		}
		if ownerRef.Kind == "DevMachinePool" && gv.Group == infrav1.GroupVersion.Group {
			return []ctrl.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      ownerRef.Name,
						Namespace: devMachine.Namespace,
					},
				},
			}
		}
	}

	return nil
}

// setInfrastructureMachineKindForMachinePool sets the infrastructure machine kind in the status if it is not set already to support
// MachinePool Machines and returns a boolean indicating if the status was updated.
func setInfrastructureMachineKindForMachinePool(devMachinePool *infrav1.DevMachinePool) bool {
	if devMachinePool != nil && devMachinePool.Status.InfrastructureMachineKind != "DevMachine" {
		devMachinePool.Status.InfrastructureMachineKind = "DevMachine"
		return true
	}

	return false
}
