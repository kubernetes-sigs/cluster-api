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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ClusterCacheReconciler is responsible for stopping remote cluster caches when
// the cluster for the remote cache is being deleted.
type ClusterCacheReconciler struct {
	Log     logr.Logger
	Client  client.Client
	Tracker *ClusterCacheTracker
}

func (r *ClusterCacheReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

// Reconcile reconciles Clusters and removes ClusterCaches for any Cluster that cannot be retrieved from the
// management cluster.
func (r *ClusterCacheReconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	ctx := context.Background()

	log := r.Log.WithValues("namespace", req.Namespace, "name", req.Name)
	log.V(4).Info("Reconciling")

	var cluster clusterv1.Cluster

	err := r.Client.Get(ctx, req.NamespacedName, &cluster)
	if err == nil {
		log.V(4).Info("Cluster still exists")
		return reconcile.Result{}, nil
	} else if !kerrors.IsNotFound(err) {
		log.Error(err, "Error retrieving cluster")
		return reconcile.Result{}, err
	}

	log.V(2).Info("Cluster no longer exists")

	r.Tracker.deleteAccessor(req.NamespacedName)

	return reconcile.Result{}, nil

}
