/*
Copyright 2023 The Kubernetes Authors.

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
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/api/v1alpha1"
	"sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/cloud"
	"sigs.k8s.io/cluster-api/test/infrastructure/inmemory/internal/server"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// InMemoryClusterReconciler reconciles a InMemoryCluster object.
type InMemoryClusterReconciler struct {
	client.Client
	CloudManager cloud.Manager
	APIServerMux *server.WorkloadClustersMux

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	hotRestartDone bool
	hotRestartLock sync.RWMutex
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=inmemoryclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=inmemoryclusters/status;inmemoryclusters/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch

// Reconcile reads that state of the cluster for a InMemoryCluster object and makes changes based on the state read
// and what is in the InMemoryCluster.Spec.
func (r *InMemoryClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the InMemoryCluster instance
	inMemoryCluster := &infrav1.InMemoryCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, inMemoryCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, inMemoryCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef on InMemoryCluster")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(inMemoryCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If the controller is restarting and there is already an existing set of InMemoryClusters,
	// tries to rebuild the internal state of the APIServerMux accordingly.
	if ret, err := r.reconcileHotRestart(ctx); !ret.IsZero() || err != nil {
		return ret, err
	}

	// Always attempt to Patch the InMemoryCluster object and status after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, inMemoryCluster); err != nil {
			log.Error(err, "failed to patch InMemoryCluster")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(inMemoryCluster, infrav1.ClusterFinalizer) {
		controllerutil.AddFinalizer(inMemoryCluster, infrav1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle deleted clusters
	if !inMemoryCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, inMemoryCluster)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, cluster, inMemoryCluster)
}

// reconcileHotRestart tries to setup the APIServerMux according to an existing sets of InMemoryCluster.
// NOTE: This is done at best effort in order to make iterative development workflow easier.
func (r *InMemoryClusterReconciler) reconcileHotRestart(ctx context.Context) (ctrl.Result, error) {
	r.hotRestartLock.RLock()
	if r.hotRestartDone {
		// Return if the hot restart was already done.
		r.hotRestartLock.RUnlock()
		return ctrl.Result{}, nil
	}
	r.hotRestartLock.RUnlock()

	r.hotRestartLock.Lock()
	defer r.hotRestartLock.Unlock()

	// Check again if another go routine did the hot restart before we got the write lock.
	if r.hotRestartDone {
		return ctrl.Result{}, nil
	}

	inMemoryClusterList := &infrav1.InMemoryClusterList{}
	if err := r.Client.List(ctx, inMemoryClusterList); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.APIServerMux.HotRestart(inMemoryClusterList); err != nil {
		return ctrl.Result{}, err
	}

	r.hotRestartDone = true
	return ctrl.Result{}, nil
}

func (r *InMemoryClusterReconciler) reconcileNormal(_ context.Context, cluster *clusterv1.Cluster, inMemoryCluster *infrav1.InMemoryCluster) (ctrl.Result, error) {
	// Compute the resource group unique name.
	resourceGroup := klog.KObj(cluster).String()

	// Store the resource group used by this inMemoryCluster.
	inMemoryCluster.Annotations[infrav1.ResourceGroupAnnotationName] = resourceGroup

	// Create a resource group for all the cloud resources belonging the workload cluster;
	// if the resource group already exists, the operation is a no-op.
	// NOTE: We are storing in this resource group both the cloud resources (e.g. VM) as
	// well as Kubernetes resources that are expected to exist on the workload cluster (e.g Nodes).
	r.CloudManager.AddResourceGroup(resourceGroup)

	// Initialize a listener for the workload cluster; if the listener has been already initialized
	// the operation is a no-op.
	// NOTE: We are using reconcilerGroup also as a name for the listener for sake of simplicity.
	// IMPORTANT: The fact that both the listener and the resourceGroup for a workload cluster have
	// the same name is used by the current implementation of the resourceGroup resolvers in the APIServerMux.
	listener, err := r.APIServerMux.InitWorkloadClusterListener(resourceGroup)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to init the listener for the workload cluster")
	}

	// Surface the control plane endpoint
	if inMemoryCluster.Spec.ControlPlaneEndpoint.Host == "" {
		inMemoryCluster.Spec.ControlPlaneEndpoint.Host = listener.Host()
		inMemoryCluster.Spec.ControlPlaneEndpoint.Port = listener.Port()
	}

	// Mark the InMemoryCluster ready
	inMemoryCluster.Status.Ready = true

	return ctrl.Result{}, nil
}

//nolint:unparam // once we implemented this func we will also return errors
func (r *InMemoryClusterReconciler) reconcileDelete(_ context.Context, inMemoryCluster *infrav1.InMemoryCluster) (ctrl.Result, error) {
	// TODO: implement
	controllerutil.RemoveFinalizer(inMemoryCluster, infrav1.ClusterFinalizer)

	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller.
func (r *InMemoryClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.InMemoryCluster{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind("InMemoryCluster"), mgr.GetClient(), &infrav1.InMemoryCluster{})),
			builder.WithPredicates(
				predicates.ClusterUnpaused(ctrl.LoggerFrom(ctx)),
			),
		).Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}
