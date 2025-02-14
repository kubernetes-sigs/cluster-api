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

// Package controllers provides access to reconcilers implemented in internal/controllers.
package controllers

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	dockercontrollers "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers"
	inmemoryruntime "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/runtime"
	inmemoryserver "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/server"
)

// Following types provides access to reconcilers implemented in internal/controllers, thus
// allowing users to provide a single binary "batteries included" with Cluster API and providers of choice.

// DockerMachineReconciler reconciles a DockerMachine object.
type DockerMachineReconciler struct {
	Client           client.Client
	ContainerRuntime container.Runtime
	ClusterCache     clustercache.ClusterCache

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *DockerMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&dockercontrollers.DockerMachineReconciler{
		Client:           r.Client,
		ContainerRuntime: r.ContainerRuntime,
		ClusterCache:     r.ClusterCache,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// DockerClusterReconciler reconciles a DevCluster object.
type DockerClusterReconciler struct {
	Client           client.Client
	ContainerRuntime container.Runtime

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *DockerClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&dockercontrollers.DockerClusterReconciler{
		Client:           r.Client,
		ContainerRuntime: r.ContainerRuntime,
		WatchFilterValue: r.WatchFilterValue,
	}).SetupWithManager(ctx, mgr, options)
}

// DevMachineReconciler reconciles a DevMachine object.
type DevMachineReconciler struct {
	Client           client.Client
	ContainerRuntime container.Runtime
	ClusterCache     clustercache.ClusterCache
	InMemoryManager  inmemoryruntime.Manager
	APIServerMux     *inmemoryserver.WorkloadClustersMux

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *DevMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&dockercontrollers.DevMachineReconciler{
		Client:           r.Client,
		WatchFilterValue: r.WatchFilterValue,
		ContainerRuntime: r.ContainerRuntime,
		ClusterCache:     r.ClusterCache,
		InMemoryManager:  r.InMemoryManager,
		APIServerMux:     r.APIServerMux,
	}).SetupWithManager(ctx, mgr, options)
}

// DevClusterReconciler reconciles a DockerMachine object.
type DevClusterReconciler struct {
	Client           client.Client
	ContainerRuntime container.Runtime
	InMemoryManager  inmemoryruntime.Manager
	APIServerMux     *inmemoryserver.WorkloadClustersMux

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *DevClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&dockercontrollers.DevClusterReconciler{
		Client:           r.Client,
		WatchFilterValue: r.WatchFilterValue,
		ContainerRuntime: r.ContainerRuntime,
		InMemoryManager:  r.InMemoryManager,
		APIServerMux:     r.APIServerMux,
	}).SetupWithManager(ctx, mgr, options)
}
