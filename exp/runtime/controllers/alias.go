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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	extensions "sigs.k8s.io/cluster-api/exp/runtime/internal/controllers"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	"sigs.k8s.io/cluster-api/internal/runtime/registry"
)

// ExtensionReconciler reconciles an Extension object.
type ExtensionReconciler struct {
	Client        client.Client
	RuntimeClient runtimeclient.Client
	Registry      registry.Registry
}

func (r *ExtensionReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	return (&extensions.ExtensionReconciler{
		Client:        r.Client,
		RuntimeClient: r.RuntimeClient,
		Registry:      r.Registry,
	}).SetupWithManager(ctx, mgr, options)
}
