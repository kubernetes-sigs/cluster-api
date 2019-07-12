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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubeadmv1alpha1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha1"
	clusterapiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// KubeadmBootstrapConfigReconciler reconciles a KubeadmBootstrapConfig object
type KubeadmBootstrapConfigReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=kubeadm.bootstrap.cluster.sigs.k8s.io,resources=kubeadmbootstrapconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubeadm.bootstrap.cluster.sigs.k8s.io,resources=kubeadmbootstrapconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.sigs.k8s.io,resources=clusters;machines,verbs=get;list;watch

// Reconcile TODO
func (r *KubeadmBootstrapConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("kubeadmbootstrapconfig", req.NamespacedName)

	config := &kubeadmv1alpha1.KubeadmBootstrapConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		log.Error(err, "stacktrace", fmt.Sprintf("%+v", err))
	}

	// find the first machine reference
	var machineRef v1.OwnerReference
	for _, ref := range config.OwnerReferences {
		if ref.Kind == "Machine" {
			machineRef = ref
			break
		}
	}

	machine := &clusterapiv1alpha2.Machine{}
	machineKey := types.NamespacedName{
		Name:      machineRef.Name,
		Namespace: config.Namespace, // must be in the same namespace
	}
	if err := r.Get(ctx, machineKey, machine); err != nil {
		log.Error(err, "stacktrace", fmt.Sprintf("%+v", err))
	}

	cluster := &clusterapiv1alpha2.Cluster{}
	clusterKey := types.NamespacedName{
		Name:      machine.ClusterName,
		Namespace: config.Namespace, // must still be in the same namespace
	}
	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		log.Error(err, "stacktrace", fmt.Sprintf("%+v", err))
	}
	log.Info("debug", "cluster", cluster, "machine", machine, "config", config)
	return ctrl.Result{}, nil
}

// SetupWithManager TODO
func (r *KubeadmBootstrapConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeadmv1alpha1.KubeadmBootstrapConfig{}).
		Complete(r)
}
func (r *KubeadmBootstrapConfigReconciler) Map(object handler.MapObject) []reconcile.Request {
	log := r.Log.WithValues("ns", object.Meta.GetNamespace(), "name", object.Meta.GetName())

	log.Info("mapping machine to config")

	result := []reconcile.Request{}
	m := &clusterapiv1alpha2.Machine{}
	key := client.ObjectKey{Namespace: object.Meta.GetNamespace(), Name: object.Meta.GetName()}
	if err := r.Client.Get(context.Background(), key, m); err != nil {
		return result
	}

	// There is no config reference for this machine, so don't enqueue anything
	if m.Spec.Bootstrap.ConfigRef == nil {
		return result
	}

	// Enqueue the associated config ref (not sure if this is actually what this should do?)
	if m.Spec.Bootstrap.ConfigRef.Name != "" && m.Spec.Bootstrap.ConfigRef.Namespace != "" {
		result = append(result, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: m.Spec.Bootstrap.ConfigRef.Namespace,
				Name:      m.Spec.Bootstrap.ConfigRef.Name,
			},
		})
	}
	return result
}
