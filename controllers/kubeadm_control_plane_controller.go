/*
Copyright 2019 The Kubernetes Authors.

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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=kubeadmcontrolplanes;kubeadmcontrolplanes/status,verbs=get;list;watch;create;update;patch;delete

// KubeadmControlPlaneReconciler reconciles a KubeadmControlPlane object
type KubeadmControlPlaneReconciler struct {
	Client client.Client
	Log    logr.Logger

	controller controller.Controller
	recorder   record.EventRecorder
}

func (r *KubeadmControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.KubeadmControlPlane{}).
		Owns(&clusterv1.Machine{}).
		WithOptions(options).
		Build(r)

	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("kubeadm-control-plane-controller")

	return nil
}

func (r *KubeadmControlPlaneReconciler) Reconcile(req ctrl.Request) (res ctrl.Result, _ error) {
	logger := r.Log.WithValues("kubeadmControlPlane", req.Name, "namespace", req.Namespace)
	ctx := context.Background()

	// Fetch the KubeadmControlPlane instance.
	kubeadmControlPlane := &clusterv1.KubeadmControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, kubeadmControlPlane); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to retrieve requested KubeadmControlPlane resource from the API Server")
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(kubeadmControlPlane, r.Client)
	if err != nil {
		logger.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	defer func() {
		// Always attempt to Patch the KubeadmControlPlane object and status after each reconciliation.
		if patchErr := patchHelper.Patch(ctx, kubeadmControlPlane); patchErr != nil {
			logger.Error(err, "Failed to retrieve requested KubeadmControlPlane resource from the API Server")
			res.Requeue = true
		}
	}()

	// Handle deletion reconciliation loop.
	if !kubeadmControlPlane.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, kubeadmControlPlane, logger), nil
	}

	// Handle normal reconciliation loop.
	return r.reconcile(ctx, kubeadmControlPlane, logger), nil
}

// reconcile handles KubeadmControlPlane reconciliation.
func (r *KubeadmControlPlaneReconciler) reconcile(_ context.Context, kubeadmControlPlane *clusterv1.KubeadmControlPlane, logger logr.Logger) ctrl.Result {
	// If object doesn't have a finalizer, add one.
	controllerutil.AddFinalizer(kubeadmControlPlane, clusterv1.KubeadmControlPlaneFinalizer)

	logger.Error(errors.New("Not Implemented"), "Not Implemented")
	return ctrl.Result{Requeue: true}

}

// reconcileDelete handles KubeadmControlPlane deletion.
func (r *KubeadmControlPlaneReconciler) reconcileDelete(_ context.Context, kubeadmControlPlane *clusterv1.KubeadmControlPlane, logger logr.Logger) ctrl.Result {
	err := errors.New("Not Implemented")

	if err != nil {
		logger.Error(err, "Not Implemented")
		return ctrl.Result{Requeue: true}
	}

	controllerutil.RemoveFinalizer(kubeadmControlPlane, clusterv1.KubeadmControlPlaneFinalizer)
	return ctrl.Result{}
}
