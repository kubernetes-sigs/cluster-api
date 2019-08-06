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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	infrastructurev1alpha2 "sigs.k8s.io/cluster-api-provider-docker/api/v1alpha2"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DockerClusterReconciler reconciles a DockerCluster object
type DockerClusterReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=dockerclusters/status,verbs=get;update;patch

// Reconcile handles DockerCluster events
func (r *DockerClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("dockercluster", req.NamespacedName)
	log.Info("Reconciling cluster")

	dockerCluster := &infrastructurev1alpha2.DockerCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get cluster")
		return ctrl.Result{}, err
	}

	if dockerCluster.Status.Ready {
		return ctrl.Result{}, nil
	}

	if err := r.reconcileNetwork(dockerCluster.Name, r.Log); err != nil {
		log.Error(err, "Failed to reconcile network for cluster")
		return ctrl.Result{}, err
	}

	// Store Config's state, pre-modifications, to allow patching
	patchCluster := client.MergeFrom(dockerCluster.DeepCopy())

	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	gvk := dockerCluster.GroupVersionKind()
	if err := r.Patch(ctx, dockerCluster, patchCluster); err != nil {
		log.Error(err, "failed to update dockerCluster")
		return ctrl.Result{}, err
	}

	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	dockerCluster.SetGroupVersionKind(gvk)
	if err := r.Status().Patch(ctx, dockerCluster, patchCluster); err != nil {
		log.Error(err, "failed to update docker cluster status")
		return ctrl.Result{}, err
	}

	dockerCluster.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *DockerClusterReconciler) reconcileNetwork(clusterName string, log logr.Logger) error {
	// reconcile elb
	log.Info("Reconciling ELB for cluster", "cluster-name", clusterName)
	elb, err := actions.GetExternalLoadBalancerNode(clusterName)
	if err != nil {
		log.Error(err, "Failed to get ELB for cluster")
		return err
	}

	if elb == nil {
		log.Info("External loadbalancer does not exist for cluster, creating ELB", "cluster-name", clusterName)
		elb, err = actions.SetUpLoadBalancer(clusterName)
		if err != nil {
			log.Error(err, "Failed to setup ELB for cluster")
			return err
		}
	}
	log.Info("External loadbalancer node for cluster", "cluster-name", clusterName, "elb", elb)
	return nil
}

// SetupWithManager will add watches for this controller
func (r *DockerClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha2.DockerCluster{}).
		Complete(r)
}
