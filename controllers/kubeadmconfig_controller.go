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
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	"sigs.k8s.io/cluster-api/pkg/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cabpkv1alpha2 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/cloudinit"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
)

var (
	machineKind = capiv1alpha2.SchemeGroupVersion.WithKind("Machine")
)

const (
	// ControlPlaneReadyAnnotationKey identifies when the infrastructure is ready for use such as joining new nodes.
	// TODO move this into cluster-api to be imported by providers
	ControlPlaneReadyAnnotationKey = "cluster.x-k8s.io/control-plane-ready"
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
func (r *KubeadmConfigReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, rerr error) {

	ctx := context.Background()
	log := r.Log.WithValues("kubeadmconfig", req.NamespacedName)

	config := &cabpkv1alpha2.KubeadmConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
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

	machine, err := util.GetOwnerMachine(ctx, r.Client, config.ObjectMeta)
	if err != nil {
		log.Error(err, "could not get owner machine")
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on the KubeadmConfig")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Ignore machines that already have bootstrap data
	if machine.Spec.Bootstrap.Data != nil {
		return ctrl.Result{}, nil
	}

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Error(err, "could not get cluster by machine metadata")
		return ctrl.Result{}, err
	}

	// Check for infrastructure ready. If it's not ready then we will requeue the machine until it is.
	// The cluster-api machine controller set this value.
	if cluster.Status.InfrastructureReady != true {
		log.Info("Infrastructure is not ready, requeing until ready.")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Store Config's state, pre-modifications, to allow patching
	patchConfig := client.MergeFrom(config.DeepCopy())
	defer func() {
		err := r.patchConfig(ctx, config, patchConfig)
		if err != nil {
			log.Error(err, "failed to patch config")
			rerr = err
		}
	}()

	// Check for control plane ready. If it's not ready then we will requeue the machine until it is.
	// The cluster-api machine controller set this value.
	if cluster.Annotations[ControlPlaneReadyAnnotationKey] != "true" {
		// if it's NOT a control plane machine, requeue
		if !util.IsControlPlaneMachine(machine) {
			log.Info("Control plane is not ready, requeing worker nodes until ready.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// if the machine has not ClusterConfiguration and InitConfiguration, requeue
		if config.Spec.InitConfiguration == nil && config.Spec.ClusterConfiguration == nil {
			log.Info("Control plane is not ready, requeing joining control planes until ready.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		//TODO(fp) use init lock so only the first machine configured as control plane get processed, everything else gets requeued

		// otherwise it is a init control plane
		// Nb. in this case JoinConfiguration should not be defined by users, but in case of misconfigurations, CAPBK simply ignore it

		log.Info("Creating BootstrapData for the init control plane")

		// get both of ClusterConfiguration and InitConfiguration strings to pass to the cloud init control plane generator
		// kubeadm allows one this values to be empty; cabpk replace missing values with an empty config, so the cloud init generation
		// should not handle special cases.

		if config.Spec.InitConfiguration == nil {
			config.Spec.InitConfiguration = &kubeadmv1beta1.InitConfiguration{
				TypeMeta: v1.TypeMeta{
					APIVersion: "kubeadm.k8s.io/v1beta1",
					Kind:       "InitConfiguration",
				},
			}
		}
		initdata, err := json.Marshal(config.Spec.InitConfiguration)
		if err != nil {
			log.Error(err, "failed to marshal init configuration")
			return ctrl.Result{}, err
		}

		if config.Spec.ClusterConfiguration == nil {
			config.Spec.ClusterConfiguration = &kubeadmv1beta1.ClusterConfiguration{
				TypeMeta: v1.TypeMeta{
					APIVersion: "kubeadm.k8s.io/v1beta1",
					Kind:       "ClusterConfiguration",
				},
			}
		}
		// If there is a control plane endpoint defined at cluster in cluster status, use it as a control plane endpoint for the K8s cluster
		// NB. we are only using the first one defined if there are multiple defined.
		if len(cluster.Status.APIEndpoints) > 0 {
			config.Spec.ClusterConfiguration.ControlPlaneEndpoint = fmt.Sprintf("https://%s:%d", cluster.Status.APIEndpoints[0].Host, cluster.Status.APIEndpoints[0].Port)
		}

		clusterdata, err := json.Marshal(config.Spec.ClusterConfiguration)
		if err != nil {
			log.Error(err, "failed to marshal cluster configuration")
			return ctrl.Result{}, err
		}

		//TODO(fp) Implement the expected flow for certificates
		certificates, _ := r.getCertificates()

		cloudInitData, err := cloudinit.NewInitControlPlane(&cloudinit.ControlPlaneInput{
			BaseUserData: cloudinit.BaseUserData{
				AdditionalFiles: config.Spec.AdditionalUserDataFiles,
			},
			InitConfiguration:    string(initdata),
			ClusterConfiguration: string(clusterdata),
			Certificates:         certificates,
		})
		if err != nil {
			log.Error(err, "failed to generate cloud init for bootstrap control plane")
			return ctrl.Result{}, err
		}

		config.Status.BootstrapData = cloudInitData
		config.Status.Ready = true
		return ctrl.Result{}, nil
	}

	// Every other case it's a join scenario
	// Nb. in this case ClusterConfiguration and JoinConfiguration should not be defined by users, but in case of misconfigurations, CAPBK simply ignore them

	if config.Spec.JoinConfiguration == nil {
		return ctrl.Result{}, errors.New("Control plane already exists for the cluster, only KubeadmConfig objects with JoinConfiguration are allowed")
	}

	if len(cluster.Status.APIEndpoints) == 0 {
		return ctrl.Result{}, errors.New("Control plane already exists for the cluster but cluster.Status.APIEndpoints are not set")
	}

	// Injects the controlplane endpoint address
	// NB. this assumes nodes using token discovery
	if config.Spec.JoinConfiguration.Discovery.BootstrapToken == nil {
		config.Spec.JoinConfiguration.Discovery.BootstrapToken = &kubeadmv1beta1.BootstrapTokenDiscovery{}
	}

	config.Spec.JoinConfiguration.Discovery.BootstrapToken.APIServerEndpoint = fmt.Sprintf("https://%s:%d", cluster.Status.APIEndpoints[0].Host, cluster.Status.APIEndpoints[0].Port)

	joinBytes, err := json.Marshal(config.Spec.JoinConfiguration)
	if err != nil {
		log.Error(err, "failed to marshal join configuration")
		return ctrl.Result{}, err
	}

	//TODO(fp) remove init lock

	// it's a control plane join
	if util.IsControlPlaneMachine(machine) {
		if config.Spec.JoinConfiguration.ControlPlane == nil {
			return ctrl.Result{}, errors.New("Machine is a ControlPlane, but JoinConfiguration.ControlPlane is not set in the KubeadmConfig object")
		}

		//TODO(fp) Implement the expected flow for certificates
		certificates, _ := r.getCertificates()

		joinData, err := cloudinit.NewJoinControlPlane(&cloudinit.ControlPlaneJoinInput{
			JoinConfiguration: string(joinBytes),
			Certificates:      certificates,
			BaseUserData: cloudinit.BaseUserData{
				AdditionalFiles: config.Spec.AdditionalUserDataFiles,
			},
		})
		if err != nil {
			log.Error(err, "failed to create a control plane join configuration")
			return ctrl.Result{}, err
		}

		config.Status.BootstrapData = joinData
		config.Status.Ready = true
		return ctrl.Result{}, nil
	}

	joinData, err := cloudinit.NewNode(&cloudinit.NodeInput{
		BaseUserData: cloudinit.BaseUserData{
			AdditionalFiles: config.Spec.AdditionalUserDataFiles,
		},
		JoinConfiguration: string(joinBytes),
	})
	if err != nil {
		log.Error(err, "failed to create a worker join configuration")
		return ctrl.Result{}, err
	}
	config.Status.BootstrapData = joinData
	config.Status.Ready = true
	return ctrl.Result{}, nil
}

func (r *KubeadmConfigReconciler) getCertificates() (cloudinit.Certificates, error) {
	//TODO(fp) check what is the expected flow for certificates
	certificates, _ := NewCertificates()

	return cloudinit.Certificates{
		CACert:           string(certificates.ClusterCA.Cert),
		CAKey:            string(certificates.ClusterCA.Key),
		EtcdCACert:       string(certificates.EtcdCA.Cert),
		EtcdCAKey:        string(certificates.EtcdCA.Key),
		FrontProxyCACert: string(certificates.FrontProxyCA.Cert),
		FrontProxyCAKey:  string(certificates.FrontProxyCA.Key),
		SaCert:           string(certificates.ServiceAccount.Cert),
		SaKey:            string(certificates.ServiceAccount.Key),
	}, nil
}

// SetupWithManager TODO
func (r *KubeadmConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cabpkv1alpha2.KubeadmConfig{}).
		Complete(r)
}

func (r *KubeadmConfigReconciler) patchConfig(ctx context.Context, config *cabpkv1alpha2.KubeadmConfig, patchConfig client.Patch) error {
	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	gvk := config.GroupVersionKind()
	if err := r.Patch(ctx, config, patchConfig); err != nil {
		return err
	}
	// TODO(ncdc): remove this once we've updated to a version of controller-runtime with
	// https://github.com/kubernetes-sigs/controller-runtime/issues/526.
	config.SetGroupVersionKind(gvk)
	if err := r.Status().Patch(ctx, config, patchConfig); err != nil {
		return err
	}
	return nil
}
