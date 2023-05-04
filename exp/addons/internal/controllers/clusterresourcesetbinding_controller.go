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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// +kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete

// ClusterResourceSetBindingReconciler reconciles a ClusterResourceSetBinding object.
type ClusterResourceSetBindingReconciler struct {
	Client client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterResourceSetBindingReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&addonsv1.ClusterResourceSetBinding{}).
		Watches(
			&clusterv1.Cluster{},
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
	if err := r.updateClusterReference(ctx, binding); err != nil {
		return ctrl.Result{}, err
	}
	cluster, err := util.GetClusterByName(ctx, r.Client, req.Namespace, binding.Spec.ClusterName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the owner cluster is already deleted, delete its ClusterResourceSetBinding
			log.Info("deleting ClusterResourceSetBinding because the owner Cluster no longer exists")
			return ctrl.Result{}, r.Client.Delete(ctx, binding)
		}
		return ctrl.Result{}, err
	}
	// If the owner cluster is in deletion process, delete its ClusterResourceSetBinding
	if !cluster.DeletionTimestamp.IsZero() {
		if feature.Gates.Enabled(feature.RuntimeSDK) && feature.Gates.Enabled(feature.ClusterTopology) {
			if cluster.Spec.Topology != nil && !hooks.IsOkToDelete(cluster) {
				// If the Cluster is not yet ready to be deleted then do not delete the ClusterResourceSetBinding.
				return ctrl.Result{}, nil
			}
		}
		log.Info("deleting ClusterResourceSetBinding because the owner Cluster is currently being deleted")
		return ctrl.Result{}, r.Client.Delete(ctx, binding)
	}

	return ctrl.Result{}, nil
}

// clusterToClusterResourceSetBinding is mapper function that maps clusters to ClusterResourceSetBinding.
func (r *ClusterResourceSetBindingReconciler) clusterToClusterResourceSetBinding(_ context.Context, o client.Object) []ctrl.Request {
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: o.GetNamespace(),
				Name:      o.GetName(),
			},
		},
	}
}

// updateClusterReference updates how the ClusterResourceSetBinding references the Cluster.
// Before 1.4 cluster name was stored as an ownerReference. This function migrates the cluster name to the spec.clusterName and removes the Cluster OwnerReference.
// Ref: https://github.com/kubernetes-sigs/cluster-api/issues/7669.
func (r *ClusterResourceSetBindingReconciler) updateClusterReference(ctx context.Context, binding *addonsv1.ClusterResourceSetBinding) error {
	patchHelper, err := patch.NewHelper(binding, r.Client)
	if err != nil {
		return errors.Wrap(err, "failed to configure the patch helper")
	}

	// If the `.spec.clusterName` is not set, take the value from the ownerReference.
	if binding.Spec.ClusterName == "" {
		// Update the clusterName field of the existing ClusterResourceSetBindings with ownerReferences.
		// More details please refer to: https://github.com/kubernetes-sigs/cluster-api/issues/7669.
		clusterName, err := getClusterNameFromOwnerRef(binding.ObjectMeta)
		if err != nil {
			return err
		}
		binding.Spec.ClusterName = clusterName
	}

	// Remove the Cluster OwnerReference if it exists. This is a no-op if the OwnerReference does not exist.
	// TODO: (killianmuldoon) This can be removed in CAPI v1beta2.
	binding.OwnerReferences = util.RemoveOwnerRef(binding.OwnerReferences, metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       binding.Spec.ClusterName,
	})

	return patchHelper.Patch(ctx, binding)
}
