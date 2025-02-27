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

// Package controllers implements controller functionality.
package controllers

import (
	"context"
	"sync"

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
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends"
	dockerbackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/docker"
	inmemorybackend "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers/backends/inmemory"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/finalizers"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DevClusterReconciler reconciles a DevCluster object.
type DevClusterReconciler struct {
	client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	ContainerRuntime container.Runtime
	InMemoryManager  inmemoryruntime.Manager
	APIServerMux     *inmemoryserver.WorkloadClustersMux

	hotRestartDone bool
	hotRestartLock sync.RWMutex
}

// SetupWithManager will add watches for this controller.
func (r *DevClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.InMemoryManager == nil || r.APIServerMux == nil || r.ContainerRuntime == nil {
		return errors.New("Client, InMemoryManager and APIServerMux, ContainerRuntime must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "devcluster")
	err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.DevCluster{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind("DevCluster"), mgr.GetClient(), &infrav1.DevCluster{})),
			builder.WithPredicates(predicates.All(mgr.GetScheme(), predicateLog,
				predicates.ResourceIsChanged(mgr.GetScheme(), predicateLog),
				predicates.ClusterPausedTransitions(mgr.GetScheme(), predicateLog),
			)),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=devclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=devclusters/status;devclusters/finalizers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch

func (r *DevClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	ctx = container.RuntimeInto(ctx, r.ContainerRuntime)

	// Fetch the DevCluster instance
	devCluster := &infrav1.DevCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, devCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, devCluster, infrav1.ClusterFinalizer); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, devCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef on DevCluster")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, devCluster); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	backendReconciler := r.backendReconcilerFactory(ctx, devCluster)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(devCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If the selected backend has to perform specific tasks when restarting, do it!
	if restarter, ok := backendReconciler.(backends.DevClusterBackendHotRestarter); ok {
		if err := r.reconcileHotRestart(ctx, restarter); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Always attempt to Patch the DevCluster object and status after each reconciliation.
	defer func() {
		if err := backendReconciler.PatchDevCluster(ctx, patchHelper, devCluster); err != nil {
			rerr = kerrors.NewAggregate([]error{rerr, err})
		}
	}()

	// Handle deleted clusters
	if !devCluster.DeletionTimestamp.IsZero() {
		return backendReconciler.ReconcileDelete(ctx, cluster, devCluster)
	}

	// Handle non-deleted clusters
	return backendReconciler.ReconcileNormal(ctx, cluster, devCluster)
}

func (r *DevClusterReconciler) backendReconcilerFactory(_ context.Context, devCluster *infrav1.DevCluster) backends.DevClusterBackendReconciler {
	if devCluster.Spec.Backend.InMemory != nil {
		return &inmemorybackend.ClusterBackendReconciler{
			Client:          r.Client,
			InMemoryManager: r.InMemoryManager,
			APIServerMux:    r.APIServerMux,
		}
	}
	return &dockerbackend.ClusterBackEndReconciler{
		Client:           r.Client,
		ContainerRuntime: r.ContainerRuntime,
	}
}

func (r *DevClusterReconciler) reconcileHotRestart(ctx context.Context, restarter backends.DevClusterBackendHotRestarter) error {
	r.hotRestartLock.RLock()
	if r.hotRestartDone {
		// Return if the hot restart was already done.
		r.hotRestartLock.RUnlock()
		return nil
	}
	r.hotRestartLock.RUnlock()

	r.hotRestartLock.Lock()
	defer r.hotRestartLock.Unlock()

	// Check again if another go routine did the hot restart before we got the write lock.
	if r.hotRestartDone {
		return nil
	}

	if err := restarter.HotRestart(ctx); err != nil {
		return err
	}

	r.hotRestartDone = true

	return nil
}
