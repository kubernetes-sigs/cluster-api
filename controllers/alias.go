/*
Copyright 2022 The Kubernetes Authors.

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
	"golang.org/x/net/context"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/cluster-api/controllers/remote"
	clustercontroller "sigs.k8s.io/cluster-api/internal/controllers/cluster"
	machinecontroller "sigs.k8s.io/cluster-api/internal/controllers/machine"
	machinedeploymentcontroller "sigs.k8s.io/cluster-api/internal/controllers/machinedeployment"
	machinehealthcheckcontroller "sigs.k8s.io/cluster-api/internal/controllers/machinehealthcheck"
	machinesetcontroller "sigs.k8s.io/cluster-api/internal/controllers/machineset"
)

// Following types provides access to reconcilers implemented in internal/controllers, thus
// allowing users to provide a single binary "batteries included" with Cluster API and providers of choice.

// ClusterReconciler reconciles a Cluster object.
type ClusterReconciler struct {
	Client    client.Client
	APIReader client.Reader

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&clustercontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// MachineReconciler reconciles a Machine object.
type MachineReconciler struct {
	Client    client.Client
	APIReader client.Reader
	Tracker   *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *MachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinecontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		Tracker:          r.Tracker,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// MachineSetReconciler reconciles a MachineSet object.
type MachineSetReconciler struct {
	Client    client.Client
	APIReader client.Reader
	Tracker   *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *MachineSetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinesetcontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		Tracker:          r.Tracker,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// MachineDeploymentReconciler reconciles a MachineDeployment object.
type MachineDeploymentReconciler struct {
	Client    client.Client
	APIReader client.Reader

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *MachineDeploymentReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinedeploymentcontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// MachineHealthCheckReconciler reconciles a MachineHealthCheck object.
type MachineHealthCheckReconciler struct {
	Client  client.Client
	Tracker *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *MachineHealthCheckReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinehealthcheckcontroller.Reconciler{
		Client:           r.Client,
		Tracker:          r.Tracker,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}
