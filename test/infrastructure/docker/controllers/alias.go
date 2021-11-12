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

	dockercontrollers "sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/controllers"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// Following types provides access to reconcilers implemented in internal/controllers, thus
// allowing users to provide a single binary "batteries included" with Cluster API and providers of choice.

// DockerMachineReconciler reconciles a DockerMachine object.
type DockerMachineReconciler struct {
	Client client.Client
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *DockerMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&dockercontrollers.DockerMachineReconciler{
		Client: r.Client,
	}).SetupWithManager(ctx, mgr, options)
}

// DockerClusterReconciler reconciles a DockerMachine object.
type DockerClusterReconciler struct {
	Client client.Client
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *DockerClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&dockercontrollers.DockerClusterReconciler{
		Client: r.Client,
	}).SetupWithManager(ctx, mgr, options)
}
