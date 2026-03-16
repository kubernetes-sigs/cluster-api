/*
Copyright 2025 The Kubernetes Authors.

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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// DockerMachineTemplateReconciler reconciles a DockerMachineTemplate object.
type DockerMachineTemplateReconciler struct {
	client.Client
	ContainerRuntime container.Runtime

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachinetemplates/status,verbs=get;watch;list;update;patch

// Reconcile reconciles the DockerMachineTemplate to set the capcity information.
func (r *DockerMachineTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the DockerMachineTemplate instance
	machineTemplate := &infrav1.DockerMachineTemplate{}
	if err := r.Get(ctx, req.NamespacedName, machineTemplate); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(machineTemplate, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	capacity, err := fetchSystemResourceCapacity(ctx, r.ContainerRuntime)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.V(3).Info("Calculated capacity for DockerMachineTemplate", "capacity", capacity)
	machineTemplate.Status.Capacity = capacity
	if err := patchHelper.Patch(ctx, machineTemplate); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller.
func (r *DockerMachineTemplateReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.ContainerRuntime == nil {
		return errors.New("Client and ContainerRuntime must not be nil")
	}
	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "dockermachinetemplate")
	err := capicontrollerutil.NewControllerManagedBy(mgr, predicateLog).
		For(&infrav1.DockerMachineTemplate{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue)).
		Complete(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	return nil
}
