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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/api/v1alpha1"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/finalizers"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// InMemoryClusterReconciler reconciles a InMemoryCluster object.
type InMemoryClusterReconciler struct {
	client.Client
	InMemoryManager inmemoryruntime.Manager
	APIServerMux    *inmemoryserver.WorkloadClustersMux

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

	// Add finalizer first if not set to avoid the race condition between init and delete.
	if finalizerAdded, err := finalizers.EnsureFinalizer(ctx, r.Client, inMemoryCluster, infrav1.ClusterFinalizer); err != nil || finalizerAdded {
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

	if isPaused, conditionChanged, err := paused.EnsurePausedCondition(ctx, r.Client, cluster, inMemoryCluster); err != nil || isPaused || conditionChanged {
		return ctrl.Result{}, err
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(inMemoryCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If the controller is restarting and there is already an existing set of InMemoryClusters,
	// tries to rebuild the internal state of the APIServerMux accordingly.
	if err := r.reconcileHotRestart(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the InMemoryCluster object and status after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, inMemoryCluster); err != nil {
			rerr = kerrors.NewAggregate([]error{rerr, err})
		}
	}()

	// Handle deleted clusters
	if !inMemoryCluster.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.reconcileDelete(ctx, cluster, inMemoryCluster)
	}

	// Handle non-deleted clusters
	return ctrl.Result{}, r.reconcileNormal(ctx, cluster, inMemoryCluster)
}

// reconcileHotRestart tries to setup the APIServerMux according to an existing sets of InMemoryCluster.
// NOTE: This is done at best effort in order to make iterative development workflow easier.
func (r *InMemoryClusterReconciler) reconcileHotRestart(ctx context.Context) error {
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

	inMemoryClusterList := &infrav1.InMemoryClusterList{}
	if err := r.Client.List(ctx, inMemoryClusterList); err != nil {
		return err
	}
	if err := r.APIServerMux.HotRestart(inMemoryClusterList); err != nil {
		return err
	}

	r.hotRestartDone = true
	return nil
}

func (r *InMemoryClusterReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, inMemoryCluster *infrav1.InMemoryCluster) error {
	// Compute the name for resource group and listener.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()

	// Store the resource group used by this inMemoryCluster.
	inMemoryCluster.Annotations[infrav1.ListenerAnnotationName] = listenerName

	// Create a resource group for all the in memory resources belonging the workload cluster;
	// if the resource group already exists, the operation is a no-op.
	// NOTE: We are storing in this resource group both the in memory resources (e.g. VM) as
	// well as Kubernetes resources that are expected to exist on the workload cluster (e.g Nodes).
	r.InMemoryManager.AddResourceGroup(resourceGroup)

	inmemoryClient := r.InMemoryManager.GetResourceGroup(resourceGroup).GetClient()

	// Create default Namespaces.
	for _, nsName := range []string{metav1.NamespaceDefault, metav1.NamespacePublic, metav1.NamespaceSystem} {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					"kubernetes.io/metadata.name": nsName,
				},
			},
		}

		if err := inmemoryClient.Get(ctx, client.ObjectKeyFromObject(ns), ns); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "failed to get %s Namespace", nsName)
			}

			if err := inmemoryClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
				return errors.Wrapf(err, "failed to create %s Namespace", nsName)
			}
		}
	}

	// Initialize a listener for the workload cluster; if the listener has been already initialized
	// the operation is a no-op.
	listener, err := r.APIServerMux.InitWorkloadClusterListener(listenerName)
	if err != nil {
		return errors.Wrap(err, "failed to init the listener for the workload cluster")
	}
	if err := r.APIServerMux.RegisterResourceGroup(listenerName, resourceGroup); err != nil {
		return errors.Wrap(err, "failed to register the resource group for the workload cluster")
	}

	// Surface the control plane endpoint
	if inMemoryCluster.Spec.ControlPlaneEndpoint.Host == "" {
		inMemoryCluster.Spec.ControlPlaneEndpoint.Host = listener.Host()
		inMemoryCluster.Spec.ControlPlaneEndpoint.Port = listener.Port()
	}

	// Mark the InMemoryCluster ready
	inMemoryCluster.Status.Ready = true

	return nil
}

func (r *InMemoryClusterReconciler) reconcileDelete(_ context.Context, cluster *clusterv1.Cluster, inMemoryCluster *infrav1.InMemoryCluster) error {
	// Compute the name for resource group and listener.
	// NOTE: we are using the same name for convenience, but it is not required.
	resourceGroup := klog.KObj(cluster).String()
	listenerName := klog.KObj(cluster).String()

	// Delete the resource group hosting all the in memory resources belonging the workload cluster;
	r.InMemoryManager.DeleteResourceGroup(resourceGroup)

	// Delete the listener for the workload cluster;
	if err := r.APIServerMux.DeleteWorkloadClusterListener(listenerName); err != nil {
		return err
	}

	controllerutil.RemoveFinalizer(inMemoryCluster, infrav1.ClusterFinalizer)
	return nil
}

// SetupWithManager will add watches for this controller.
func (r *InMemoryClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.InMemoryManager == nil || r.APIServerMux == nil {
		return errors.New("Client, InMemoryManager and APIServerMux must not be nil")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "inmemorycluster")
	err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.InMemoryCluster{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(util.ClusterToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind("InMemoryCluster"), mgr.GetClient(), &infrav1.InMemoryCluster{})),
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
