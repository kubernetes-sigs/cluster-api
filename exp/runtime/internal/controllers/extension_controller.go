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

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	expv1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1beta1"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	"sigs.k8s.io/cluster-api/internal/runtime/registry"
)

// +kubebuilder:rbac:groups=runtime.cluster.x-k8s.io,resources=extension,verbs=get;list;watch;create;update;patch;delete

// ExtensionReconciler reconciles a Extension object.
type ExtensionReconciler struct {
	Client        client.Client
	RuntimeClient runtimeclient.Client
	Registry      registry.Registry
}

func (r *ExtensionReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&expv1.Extension{}).
		WithOptions(options).
		Complete(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}
	return nil
}

func (r *ExtensionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Reconciling")

	ext := &expv1.Extension{}
	err := r.Client.Get(ctx, req.NamespacedName, ext)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	// FIXME add defer patch helper..

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(ext, expv1.ExtensionFinalizer) {
		controllerutil.AddFinalizer(ext, expv1.ExtensionFinalizer)
		return ctrl.Result{}, nil
	}

	// Handle deletion reconciliation loop.
	if !ext.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ext)
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, ext)
}

func (r *ExtensionReconciler) reconcileDelete(ctx context.Context, ext *expv1.Extension) (ctrl.Result, error) {
	r.Registry.RemoveRuntimeExtension(ext)

	return ctrl.Result{}, nil
}

func (r *ExtensionReconciler) reconcile(ctx context.Context, ext *expv1.Extension) (ctrl.Result, error) {
	// FIXME:
	// * should call Discover on RuntimeExtension and then update Status
	// Note: Has to work with ext.Spec.ClientConfig without underlying registry
	// Q: Is it enough to do the Discovery only once initially or on each reconcile

	if ext.Status.Discovered {
		r.Registry.RegisterRuntimeExtension(ext)
		return ctrl.Result{}, nil
	}

	runtimeExtensions, err := r.RuntimeClient.Extension(ext).Discover()
	if err != nil {
		return ctrl.Result{}, err
	}

	ext.Status.Discovered = true
	ext.Status.RuntimeExtensions = runtimeExtensions

	r.Registry.RegisterRuntimeExtension(ext)

	return ctrl.Result{}, nil
}
