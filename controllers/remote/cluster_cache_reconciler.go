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

package remote

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// ClusterCacheReconciler is responsible for stopping remote cluster caches when
// the cluster for the remote cache is being deleted.
type ClusterCacheReconciler struct {
	// Deprecated: this field is unused and will be dropped in an upcoming release.
	Log     logr.Logger
	Client  client.Client
	Tracker *ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterCacheReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		Named("remote/clustercache").
		For(&clusterv1.Cluster{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Complete(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

// Reconcile reconciles Clusters and removes ClusterCaches for any Cluster that cannot be retrieved from the
// management cluster.
func (r *ClusterCacheReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Reconciling")

	var cluster clusterv1.Cluster

	err := r.Client.Get(ctx, req.NamespacedName, &cluster)
	if err == nil {
		log.V(4).Info("Cluster still exists")
		return reconcile.Result{}, nil
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Error retrieving cluster")
		return reconcile.Result{}, err
	}

	log.V(2).Info("Cluster no longer exists")

	r.Tracker.deleteAccessor(req.NamespacedName)

	return reconcile.Result{}, nil
}
