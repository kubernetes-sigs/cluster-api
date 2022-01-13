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
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/cluster-api/controllers/remote"
	kubeadmcontrolplanecontrollers "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/controllers"
)

// KubeadmControlPlaneReconciler reconciles a KubeadmControlPlane object.
type KubeadmControlPlaneReconciler struct {
	Client    client.Client
	APIReader client.Reader
	Tracker   *remote.ClusterCacheTracker

	EtcdDialTimeout time.Duration

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *KubeadmControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&kubeadmcontrolplanecontrollers.KubeadmControlPlaneReconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		Tracker:          r.Tracker,
		EtcdDialTimeout:  r.EtcdDialTimeout,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}
