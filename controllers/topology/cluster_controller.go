/*
Copyright 2021 The Kubernetes Authors.

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

package topology

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusterclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// ClusterReconciler reconciles a managed topology for a Cluster object.
type ClusterReconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader        client.Reader
	WatchFilterValue string

	// UnstructuredCachingClient provides a client that forces caching of unstructured objects,
	// thus allowing to optimize reads for templates or provider specific objects in a managed topology.
	UnstructuredCachingClient client.Client

	externalTracker external.ObjectTracker
}

func (r *ClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		Named("topology/cluster").
		Watches(
			&source.Kind{Type: &clusterv1.ClusterClass{}},
			handler.EnqueueRequestsFromMapFunc(r.clusterClassToCluster),
		).
		Watches(
			&source.Kind{Type: &clusterv1.MachineDeployment{}},
			handler.EnqueueRequestsFromMapFunc(r.machineDeploymentToCluster),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.externalTracker = external.ObjectTracker{
		Controller: c,
	}
	return nil
}

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the Cluster instance.
	cluster := &clusterv1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// Return early, if the Cluster does not use a managed topology.
	// NOTE: This should be removed as soon as we start to support Clusters moving from managed <-> unmanaged.
	if cluster.Spec.Topology == nil {
		return ctrl.Result{}, nil
	}

	// Return early if the Cluster is paused.
	// TODO: What should we do if the cluster class is paused?
	if annotations.IsPaused(cluster, cluster) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// TODO: Add patching as soon as we define how to report managed topology state into conditions

	// In case the object is deleted, the managed topology stops to reconcile;
	// (the other controllers will take care of deletion).
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		// TODO: When external patching is supported, we should handle the deletion
		// of those external CRDs we created.
		return ctrl.Result{}, nil
	}

	// Create a scope initialized with only the cluster; during reconcile
	// additional information will be added about the Cluster blueprint, current state and desired state.
	scope := scope.New(cluster)

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, scope)
}

// reconcile handles cluster reconciliation.
func (r *ClusterReconciler) reconcile(ctx context.Context, s *scope.Scope) (ctrl.Result, error) {
	var err error

	// Gets the blueprint with the ClusterClass and the referenced templates
	// and store it in the request scope.
	s.Blueprint, err = r.getBlueprint(ctx, s.Current.Cluster)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error reading the ClusterClass")
	}

	// Gets the current state of the Cluster and store it in the request scope.
	s.Current, err = r.getCurrentState(ctx, s)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error reading current state of the Cluster topology")
	}

	// Watch Infrastructure and ControlPlane CRs when they exist.
	if s.Current.InfrastructureCluster != nil {
		if err := r.externalTracker.Watch(ctrl.LoggerFrom(ctx), s.Current.InfrastructureCluster,
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Cluster{}}); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "error watching Infrastructure CR")
		}
	}
	if s.Current.ControlPlane.Object != nil {
		if err := r.externalTracker.Watch(ctrl.LoggerFrom(ctx), s.Current.ControlPlane.Object,
			&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Cluster{}}); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "error watching ControlPlane CR")
		}
	}

	// Computes the desired state of the Cluster and store it in the request scope.
	s.Desired, err = r.computeDesiredState(ctx, s)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error computing the desired state of the Cluster topology")
	}

	// Reconciles current and desired state of the Cluster
	if err := r.reconcileState(ctx, s); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "error reconciling the Cluster topology")
	}

	return ctrl.Result{}, nil
}

// clusterClassToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when its own ClusterClass gets updated.
func (r *ClusterReconciler) clusterClassToCluster(o client.Object) []ctrl.Request {
	clusterClass, ok := o.(*clusterv1.ClusterClass)
	if !ok {
		panic(fmt.Sprintf("Expected a ClusterClass but got a %T", o))
	}

	clusterList := &clusterv1.ClusterList{}
	if err := r.Client.List(
		context.TODO(),
		clusterList,
		client.MatchingFields{index.ClusterClassNameField: clusterClass.Name},
		client.InNamespace(clusterClass.Namespace),
	); err != nil {
		return nil
	}

	// There can be more than one cluster using the same cluster class.
	// create a request for each of the clusters.
	requests := []ctrl.Request{}
	for i := range clusterList.Items {
		requests = append(requests, ctrl.Request{NamespacedName: util.ObjectKey(&clusterList.Items[i])})
	}
	return requests
}

// machineDeploymentToCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for Cluster to update when one of its own MachineDeployments gets updated.
func (r *ClusterReconciler) machineDeploymentToCluster(o client.Object) []ctrl.Request {
	md, ok := o.(*clusterv1.MachineDeployment)
	if !ok {
		panic(fmt.Sprintf("Expected a MachineDeployment but got a %T", o))
	}
	if md.Spec.ClusterName == "" {
		return nil
	}

	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: md.Namespace,
			Name:      md.Spec.ClusterName,
		},
	}}
}
