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
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmv1alpha2 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/cloudinit"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	machineKind = v1alpha2.SchemeGroupVersion.WithKind("Machine")
)

const (
	// InfrastructureReadyAnnotationKey identifies when the infrastructure is ready for use such as joining new nodes.
	// TODO move this into cluster-api to be imported by providers
	InfrastructureReadyAnnotationKey = "cluster.x-k8s.io/infrastructure-ready"
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

	// Store Config's state, pre-modifications, to allow patching
	patchConfig := client.MergeFrom(config.DeepCopy())

	// if the machine has cluster and or init defined on it then generate the init regular join
	if config.Spec.InitConfiguration != nil && config.Spec.ClusterConfiguration != nil {
		// get both of these to strings to pass to the cloud init control plane generator
		initdata, err := json.Marshal(config.Spec.InitConfiguration)
		if err != nil {
			log.Error(err, "failed to marshal init configuration")
			return ctrl.Result{}, err
		}
		clusterdata, err := json.Marshal(config.Spec.ClusterConfiguration)
		if err != nil {
			log.Error(err, "failed to marshal cluster configuration")
			return ctrl.Result{}, err
		}

		cloudInitData, err := cloudinit.NewInitControlPlane(&cloudinit.ControlPlaneInput{
			InitConfiguration:    initdata,
			ClusterConfiguration: clusterdata,
		})
		if err != nil {
			log.Error(err, "failed to generate cloud init for bootstrap control plane")
			return ctrl.Result{}, err
		}

		config.Status.BootstrapData = cloudInitData
		config.Status.Ready = true

		if err := r.Update(ctx, &config); err != nil {
			log.Error(err, "failed to update config")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check for control-plane ready. If it's not ready then we will requeue the machine until it is.
	// The infrastructure provider *must* set this value to use this bootstrap provider.
	if cluster.Annotations == nil {
		log.Info("No annotation exists on the cluster yet. Requeing until infrastructure is ready.")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if cluster.Annotations[InfrastructureReadyAnnotationKey] != "true" {
		log.Info("Infrastructure is not ready, requeing until ready.")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Every other case it's a join scenario
	joinBytes, err := json.Marshal(config.Spec.JoinConfiguration)
	if err != nil {
		log.Error(err, "failed to marshal join configuration")
		return ctrl.Result{}, err
	}

	// it's a control plane join
	if util.IsControlPlaneMachine(machine) {
		// TODO return a sensible error if join config is not specified (implies empty configuration)
		joinData, err := cloudinit.NewJoinControlPlane(&cloudinit.ControlPlaneJoinInput{
			// TODO do a len check or something here
			ControlPlaneAddress: fmt.Sprintf("https://%s:%d", cluster.Status.APIEndpoints[0].Host, cluster.Status.APIEndpoints[0].Port),
			JoinConfiguration:   joinBytes,
		})
		if err != nil {
			log.Error(err, "failed to create a control plane join configuration")
			return ctrl.Result{}, err
		}

		config.Status.BootstrapData = joinData
		config.Status.Ready = true
		if err := r.Update(ctx, &config); err != nil {
			log.Error(err, "failed to update config")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	joinData, err := cloudinit.NewNode(&cloudinit.NodeInput{
		JoinConfiguration: joinBytes,
	})
	if err != nil {
		log.Error(err, "failed to create a worker join configuration")
		return ctrl.Result{}, err
	}
	config.Status.BootstrapData = joinData
	config.Status.Ready = true

	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	gvk := config.GroupVersionKind()
	if err := r.Patch(ctx, &config, patchConfig); err != nil {
		log.Error(err, "failed to update config")
		return ctrl.Result{}, err
	}

	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	config.SetGroupVersionKind(gvk)
	if err := r.Status().Patch(ctx, &config, patchConfig); err != nil {
		log.Error(err, "failed to update config status")
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
