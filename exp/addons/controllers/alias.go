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

package controllers

import (
	"context"

	"sigs.k8s.io/cluster-api/controllers/remote"
	clusterresourcesets "sigs.k8s.io/cluster-api/exp/addons/internal/controllers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// ClusterResourceSetReconciler reconciles a ClusterResourceSet object.
type ClusterResourceSetReconciler struct {
	Client  client.Client
	Tracker *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterResourceSetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&clusterresourcesets.ClusterResourceSetReconciler{
		Client:           r.Client,
		Tracker:          r.Tracker,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// ClusterResourceSetBindingReconciler reconciles a ClusterResourceSetBinding object.
type ClusterResourceSetBindingReconciler struct {
	Client client.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterResourceSetBindingReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&clusterresourcesets.ClusterResourceSetBindingReconciler{
		Client:           r.Client,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}
