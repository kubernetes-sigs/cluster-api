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

package controllers

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// +kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete

// ClusterResourceSetBindingReconciler reconciles a ClusterResourceSetBinding object.
type ClusterResourceSetBindingReconciler struct {
	Client           client.Client
	WatchFilterValue string
}

func (r *ClusterResourceSetBindingReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1.ClusterResourceSetBinding{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			handler.EnqueueRequestsFromMapFunc(r.clusterToClusterResourceSetBinding),
		).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	return nil
}

func (r *ClusterResourceSetBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the ClusterResourceSetBinding instance.
	binding := &addonsv1.ClusterResourceSetBinding{}
	if err := r.Client.Get(ctx, req.NamespacedName, binding); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, binding.ObjectMeta)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// If the owner cluster is already deleted or in deletion process, delete its ClusterResourceSetBinding
	if apierrors.IsNotFound(err) || !cluster.DeletionTimestamp.IsZero() {
		log.Info("deleting ClusterResourceSetBinding because the owner Cluster no longer exists")
		err := r.Client.Delete(ctx, binding)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// clusterToClusterResourceSetBinding is mapper function that maps clusters to ClusterResourceSetBinding.
func (r *ClusterResourceSetBindingReconciler) clusterToClusterResourceSetBinding(o client.Object) []ctrl.Request {
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: o.GetNamespace(),
				Name:      o.GetName(),
			},
		},
	}
}
