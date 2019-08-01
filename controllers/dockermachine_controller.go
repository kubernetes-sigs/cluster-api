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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	infrastructurev1alpha2 "sigs.k8s.io/cluster-api-provider-docker/api/v1alpha2"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// DockerMachineReconciler reconciles a DockerMachine object
type DockerMachineReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockermachines/status,verbs=get;update;patch

// Reconcile handles DockerMachine events
func (r *DockerMachineReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("dockermachine", req.NamespacedName)

	dockerMachine := &infrastructurev1alpha2.DockerMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerMachine); err != nil {
		log.Error(err, "failed to get dockerMachine")
	}

	// should this get created?
	// should this get deleted?
	// just log it and see what happens
	log.Info("an informational log", "dm", dockerMachine)

	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller
func (r *DockerMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha2.DockerMachine{}).
		Watches(
			&source.Kind{Type: &infrastructurev1alpha2.DockerMachine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: MachineToDockerMachine(capiv1alpha2.SchemeGroupVersion.WithKind("Machine")),
			},
		).
		Complete(r)
}

// MachineToDockerMachine watches v1alpha2 machines and enqueues associated DockerMachines
func MachineToDockerMachine(gvk schema.GroupVersionKind) handler.ToRequestsFunc {
	return func(mo handler.MapObject) []reconcile.Request {
		m, ok := mo.Object.(*capiv1alpha2.Machine)
		if !ok {
			return nil
		}

		infraRef := m.Spec.InfrastructureRef

		if gvk != infraRef.GroupVersionKind() {
			return nil
		}
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: m.Namespace,
					Name:      m.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}
