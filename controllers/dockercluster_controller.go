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
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	infrastructurev1alpha2 "sigs.k8s.io/cluster-api-provider-docker/api/v1alpha2"
	"sigs.k8s.io/cluster-api-provider-docker/docker"
	"sigs.k8s.io/cluster-api-provider-docker/third_party/forked/loadbalancer"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
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
func (r *DockerClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, rerr error) {
	ctx := context.Background()
	log := r.Log.WithValues("dockercluster", req.NamespacedName)
	log.Info("Reconciling cluster")

	dockerCluster := &infrastructurev1alpha2.DockerCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, dockerCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get DockerCluster")
		return ctrl.Result{}, err
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, dockerCluster.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to get owning cluster")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Error(err, "Waiting for an OwnerReference to appear")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(dockerCluster, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Always attempt to Patch the DockerCluster object and status after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, dockerCluster); err != nil {
			log.Error(err, "failed to patch DockerCluster")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	if len(dockerCluster.Status.APIEndpoints) == 0 {
		lb, err := docker.NewLoadBalancer(cluster.Name, r.Log.WithName("new-load-balancer"))
		if err != nil {
			log.Error(err, "failed to create load balancer initializer")
			return ctrl.Result{}, err
		}
		if err := lb.Create(); err != nil {
			log.Error(err, "Failed to create load balancer infrastructure")
			return ctrl.Result{}, err
		}
		ipv4, _, err := lb.Node.IP()
		if err != nil {
			log.Error(err, "Failed to get load balancer IP")
			return ctrl.Result{}, err
		}

		dockerCluster.Status.APIEndpoints = []infrastructurev1alpha2.APIEndpoint{
			{
				Host: ipv4,
				Port: loadbalancer.ControlPlanePort, // this is the controller port used internally by node/containers to communicate with the load balancer/container; from the host the port mapping should be used instead
			},
		}
	}

	log.Info("Reconcile cluster successful", "APIEndPoint", dockerCluster.Status.APIEndpoints[0])
	dockerCluster.Status.Ready = true
	return ctrl.Result{}, nil
}

// SetupWithManager will add watches for this controller
func (r *DockerClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha2.DockerCluster{}).
		Complete(r)
}
