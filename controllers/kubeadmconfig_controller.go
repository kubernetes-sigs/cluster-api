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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmv1alpha2 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	machineKind = v1alpha2.SchemeGroupVersion.WithKind("Machine")
)

// KubeadmConfigReconciler reconciles a KubeadmConfig object
type KubeadmConfigReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch

// Reconcile TODO
func (r *KubeadmConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("kubeadmconfig", req.NamespacedName)

	config := kubeadmv1alpha2.KubeadmConfig{}
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get config")
		return ctrl.Result{}, err
	}

	// bail super early if it's already ready
	if config.Status.Ready {
		log.Info("ignoring an already ready config")
		return ctrl.Result{}, nil
	}

	// Find the owner reference
	var machineRef *v1.OwnerReference
	for _, ref := range config.OwnerReferences {
		if ref.Kind == machineKind.Kind && ref.APIVersion == machineKind.Version {
			machineRef = &ref
			break
		}
	}
	if machineRef == nil {
		log.Info("did not find matching machine reference")
		return ctrl.Result{}, nil
	}

	// Get the machine
	machine := &v1alpha2.Machine{}
	machineKey := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      machineRef.Name,
	}

	if err := r.Get(ctx, machineKey, machine); err != nil {
		log.Error(err, "failed to get machine")
		return ctrl.Result{}, err
	}

	// Ignore machines that already have bootstrap data
	if machine.Spec.Bootstrap.Data != nil {
		return ctrl.Result{}, nil
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
		return ctrl.Result{}, err
	}

	// maybe do something with cluster some day
	// maybe do something interesting here some day
	config.Status.BootstrapData = []byte("hello world")
	config.Status.Ready = true

	if err := r.Update(ctx, &config); err != nil {
		log.Error(err, "failed to update config")
		return ctrl.Result{}, err
	}
	log.Info("Updated config with bootstrap data")
	return ctrl.Result{}, nil
}

// SetupWithManager TODO
func (r *KubeadmConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeadmv1alpha2.KubeadmConfig{}).
		Complete(r)
}
