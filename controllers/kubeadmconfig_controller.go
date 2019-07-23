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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmv1alpha1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	machineKind = v1alpha2.SchemeGroupVersion.WithKind("Machine").String()
)

// KubeadmConfigReconciler reconciles a KubeadmConfig object
type KubeadmConfigReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.sigs.k8s.io,resources=clusters;machines,verbs=get;list;watch

// Reconcile TODO
func (r *KubeadmConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("kubeadmconfig", req.NamespacedName)

	config := kubeadmv1alpha1.KubeadmConfig{}
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		log.Error(err, "failed to get config", "stacktrace", fmt.Sprintf("%+v", err))
		return ctrl.Result{}, err
	}

	// Find the owner reference
	var machineRef *v1.OwnerReference
	for _, ref := range config.OwnerReferences {
		if ref.Kind == machineKind {
			machineRef = &ref
			break
		}
	}
	if machineRef == nil {
		log.Info("did not find matching machine reference, requeuing")
		return ctrl.Result{Requeue: true}, nil
	}

	// Get the machine
	machine := &v1alpha2.Machine{}
	machineKey := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      machineRef.Name,
	}

	if err := r.Get(ctx, machineKey, machine); err != nil {
		log.Error(err, "failed to get machine")
		return ctrl.Result{}, errors.WithStack(err)
	}

	if machine.Labels[v1alpha2.MachineClusterLabelName] == "" {
		return ctrl.Result{}, errors.New("machine has no associated cluster")
	}

	// Get the cluster
	cluster := &v1alpha2.Cluster{}
	clusterKey := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      machine.Labels[v1alpha2.MachineClusterLabelName],
	}
	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		log.Error(err, "failed to get cluster")
		return ctrl.Result{}, errors.WithStack(err)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager TODO
func (r *KubeadmConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeadmv1alpha1.KubeadmConfig{}).
		Complete(r)
}
