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
	"context"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/cluster-api/controllers/remote"
	clustercontroller "sigs.k8s.io/cluster-api/internal/controllers/cluster"
	clusterclasscontroller "sigs.k8s.io/cluster-api/internal/controllers/clusterclass"
	machinecontroller "sigs.k8s.io/cluster-api/internal/controllers/machine"
	machinedeploymentcontroller "sigs.k8s.io/cluster-api/internal/controllers/machinedeployment"
	machinehealthcheckcontroller "sigs.k8s.io/cluster-api/internal/controllers/machinehealthcheck"
	machinepoolcontroller "sigs.k8s.io/cluster-api/internal/controllers/machinepool"
	machinesetcontroller "sigs.k8s.io/cluster-api/internal/controllers/machineset"
	clustertopologycontroller "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster"
	machinedeploymenttopologycontroller "sigs.k8s.io/cluster-api/internal/controllers/topology/machinedeployment"
	machinesettopologycontroller "sigs.k8s.io/cluster-api/internal/controllers/topology/machineset"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
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

	// NodeDrainClientTimeout timeout of the client used for draining nodes.
	NodeDrainClientTimeout time.Duration
}

func (r *MachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinecontroller.Reconciler{
		Client:                 r.Client,
		APIReader:              r.APIReader,
		Tracker:                r.Tracker,
		WatchFilterValue:       r.WatchFilterValue,
		NodeDrainClientTimeout: r.NodeDrainClientTimeout,
	}).SetupWithManager(ctx, mgr, options)
}

// MachineSetReconciler reconciles a MachineSet object.
type MachineSetReconciler struct {
	Client    client.Client
	APIReader client.Reader
	Tracker   *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// Deprecated: DeprecatedInfraMachineNaming. Name the InfraStructureMachines after the InfraMachineTemplate.
	DeprecatedInfraMachineNaming bool
}

func (r *MachineSetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinesetcontroller.Reconciler{
		Client:                       r.Client,
		APIReader:                    r.APIReader,
		Tracker:                      r.Tracker,
		WatchFilterValue:             r.WatchFilterValue,
		DeprecatedInfraMachineNaming: r.DeprecatedInfraMachineNaming,
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

// MachinePoolReconciler reconciles a MachinePool object.
type MachinePoolReconciler struct {
	Client    client.Client
	APIReader client.Reader
	Tracker   *remote.ClusterCacheTracker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *MachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinepoolcontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		Tracker:          r.Tracker,
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

// ClusterTopologyReconciler reconciles a managed topology for a Cluster object.
type ClusterTopologyReconciler struct {
	Client  client.Client
	Tracker *remote.ClusterCacheTracker
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader client.Reader

	RuntimeClient runtimeclient.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterTopologyReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&clustertopologycontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		Tracker:          r.Tracker,
		RuntimeClient:    r.RuntimeClient,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// MachineDeploymentTopologyReconciler deletes referenced templates during deletion of topology-owned MachineDeployments.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineDeployment deletion completes.
// Note: To achieve this the cluster topology controller sets a finalizer to hook into the MachineDeployment deletions.
type MachineDeploymentTopologyReconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader        client.Reader
	WatchFilterValue string
}

func (r *MachineDeploymentTopologyReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinedeploymenttopologycontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// MachineSetTopologyReconciler deletes referenced templates during deletion of topology-owned MachineSets.
// The templates are only deleted, if they are not used in other MachineDeployments or MachineSets which are not in deleting state,
// i.e. the templates would otherwise be orphaned after the MachineSet deletion completes.
// Note: To achieve this the reconciler sets a finalizer to hook into the MachineSet deletions.
type MachineSetTopologyReconciler struct {
	Client client.Client
	// APIReader is used to list MachineSets directly via the API server to avoid
	// race conditions caused by an outdated cache.
	APIReader        client.Reader
	WatchFilterValue string
}

func (r *MachineSetTopologyReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&machinesettopologycontroller.Reconciler{
		Client:           r.Client,
		APIReader:        r.APIReader,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// ClusterClassReconciler reconciles the ClusterClass object.
type ClusterClassReconciler struct {
	// internalReconciler is used to store the reconciler after SetupWithManager
	// so that the Reconcile function can work.
	internalReconciler *clusterclasscontroller.Reconciler

	Client client.Client

	// RuntimeClient is a client for calling runtime extensions.
	RuntimeClient runtimeclient.Client

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

func (r *ClusterClassReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	r.internalReconciler = &clusterclasscontroller.Reconciler{
		Client:           r.Client,
		RuntimeClient:    r.RuntimeClient,
		WatchFilterValue: r.WatchFilterValue,
	}
	return r.internalReconciler.SetupWithManager(ctx, mgr, options)
}

// Reconcile can be used to reconcile a ClusterClass.
// Before it can be used, all fields of the ClusterClassReconciler have to be set
// and SetupWithManager has to be called.
// This method can be used when testing the behavior of the desired state computation of
// the Cluster topology controller (because that requires a reconciled ClusterClass).
func (r *ClusterClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.internalReconciler.Reconcile(ctx, req)
}
