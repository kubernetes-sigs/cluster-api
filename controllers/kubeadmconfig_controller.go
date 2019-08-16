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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	cabpkv1alpha2 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/certs"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/cloudinit"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	capierrors "sigs.k8s.io/cluster-api/pkg/errors"
	"sigs.k8s.io/cluster-api/pkg/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ControlPlaneReadyAnnotationKey identifies when the infrastructure is ready for use such as joining new nodes.
	// TODO move this into cluster-api to be imported by providers
	ControlPlaneReadyAnnotationKey = "cluster.x-k8s.io/control-plane-ready"
)

// KubeadmConfigReconciler reconciles a KubeadmConfig object
type KubeadmConfigReconciler struct {
	client.Client
	SecretsClientFactory SecretsClientFactory
	Log                  logr.Logger
}

// SecretsClientFactory define behaviour for creating a secrets client
type SecretsClientFactory interface {
	// NewSecretsClient returns a new client supporting SecretInterface
	NewSecretsClient(client.Client, *capiv1alpha2.Cluster) (typedcorev1.SecretInterface, error)
}

// ClusterCertificatesSecretName returns the name of the certificates secret, given a cluster name
func ClusterCertificatesSecretName(clusterName string) string {
	return fmt.Sprintf("%s-certs", clusterName)
}

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;events,verbs=get;list;watch;create;update;patch

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

	holdLock := false
	initLocker := newControlPlaneInitLocker(ctx, log, r.Client)
	defer func() {
		if !holdLock {
			initLocker.Release(cluster)
		}
	}()

	// Check for control plane ready. If it's not ready then we will requeue the machine until it is.
	// The cluster-api machine controller set this value.
	if cluster.Annotations[ControlPlaneReadyAnnotationKey] != "true" {
		// if it's NOT a control plane machine, requeue
		if !util.IsControlPlaneMachine(machine) {
			log.Info(fmt.Sprintf("Machine is not a control plane. If it should be a control plane, add `%s: true` as a label to the Machine", capiv1alpha2.MachineControlPlaneLabelName))
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// if the machine has not ClusterConfiguration and InitConfiguration, requeue
		if config.Spec.InitConfiguration == nil && config.Spec.ClusterConfiguration == nil {
			log.Info("Control plane is not ready, requeing joining control planes until ready.")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// acquire the init lock so that only the first machine configured
		// as control plane get processed here
		// if not the first, requeue
		if !initLocker.Acquire(cluster) {
			log.Info("A control plane is already being initialized, requeing until control plane is ready")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		log.Info("Creating BootstrapData for the init control plane")

		// get both of ClusterConfiguration and InitConfiguration strings to pass to the cloud init control plane generator
		// kubeadm allows one of these values to be empty; CABPK replace missing values with an empty config, so the cloud init generation
		// should not handle special cases.

		if config.Spec.InitConfiguration == nil {
			config.Spec.InitConfiguration = &kubeadmv1beta1.InitConfiguration{
				TypeMeta: v1.TypeMeta{
					APIVersion: "kubeadm.k8s.io/v1beta1",
					Kind:       "InitConfiguration",
				},
			}
		}
		initdata, err := kubeadmv1beta1.ConfigurationToYAML(config.Spec.InitConfiguration)
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

		// If there no ControlPlaneEndpoint defined in ClusterConfiguration but there are APIEndpoints defined at cluster level (e.g. the load balancer endpoint),
		// then use cluster APIEndpoints as a control plane endpoint for the K8s cluster
		if config.Spec.ClusterConfiguration.ControlPlaneEndpoint == "" && len(cluster.Status.APIEndpoints) > 0 {
			// NB. CABPK only uses the first APIServerEndpoint defined in cluster status if there are multiple defined.
			config.Spec.ClusterConfiguration.ControlPlaneEndpoint = fmt.Sprintf("%s:%d", cluster.Status.APIEndpoints[0].Host, cluster.Status.APIEndpoints[0].Port)
			log.Info("Altering ClusterConfiguration", "ControlPlaneEndpoint", config.Spec.ClusterConfiguration.ControlPlaneEndpoint)
		}

		clusterdata, err := kubeadmv1beta1.ConfigurationToYAML(config.Spec.ClusterConfiguration)
		if err != nil {
			log.Error(err, "failed to marshal cluster configuration")
			return ctrl.Result{}, err
		}

		certificates, err := r.getClusterCertificates(ctx, cluster.GetName(), config.GetNamespace())
		if err != nil {
			if apierrors.IsNotFound(err) {
				certificates, err = r.createClusterCertificates(ctx, cluster.GetName(), config)
				if err != nil {
					log.Error(err, "unable to create cluster certificates")
					return ctrl.Result{}, err
				}
			} else {
				log.Error(err, "unable to lookup cluster certificates")
				return ctrl.Result{}, err
			}
		}

		err = r.createKubeconfigSecret(ctx, config.Spec.ClusterConfiguration.ClusterName, config.Spec.ClusterConfiguration.ControlPlaneEndpoint, req.Namespace, certificates)
		if err != nil {
			log.Error(err, "unable to create and write kubeconfig as a Secret")
			return ctrl.Result{}, err
		}

		cloudInitData, err := cloudinit.NewInitControlPlane(&cloudinit.ControlPlaneInput{
			BaseUserData: cloudinit.BaseUserData{
				AdditionalFiles: config.Spec.AdditionalUserDataFiles,
			},
			InitConfiguration:    string(initdata),
			ClusterConfiguration: string(clusterdata),
			Certificates:         *certificates,
		})
		if err != nil {
			log.Error(err, "failed to generate cloud init for bootstrap control plane")
			return ctrl.Result{}, err
		}

		config.Status.BootstrapData = cloudInitData
		config.Status.Ready = true

		holdLock = true

		return ctrl.Result{}, nil
	}

	// Every other case it's a join scenario
	// Nb. in this case ClusterConfiguration and JoinConfiguration should not be defined by users, but in case of misconfigurations, CABPK simply ignore them

	// Release any locks that might have been set during init process
	initLocker.Release(cluster)

	if config.Spec.JoinConfiguration == nil {
		return ctrl.Result{}, errors.New("Control plane already exists for the cluster, only KubeadmConfig objects with JoinConfiguration are allowed")
	}

	// ensure that joinConfiguration.Discovery is properly set for joining node on the current cluster
	if err := r.reconcileDiscovery(cluster, config); err != nil {
		if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
			log.Info(err.Error())
			return ctrl.Result{RequeueAfter: requeueErr.GetRequeueAfter()}, nil
		}
		return ctrl.Result{}, err
	}

	joinBytes, err := kubeadmv1beta1.ConfigurationToYAML(config.Spec.JoinConfiguration)
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

		certificates, err := r.getClusterCertificates(ctx, cluster.GetName(), config.GetNamespace())
		if err != nil {
			log.Error(err, "unable to locate cluster certificates")
			return ctrl.Result{}, err
		}

		joinData, err := cloudinit.NewJoinControlPlane(&cloudinit.ControlPlaneJoinInput{
			JoinConfiguration: string(joinBytes),
			Certificates:      *certificates,
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

	// otherwise it is a node
	if config.Spec.JoinConfiguration.ControlPlane != nil {
		return ctrl.Result{}, errors.New("Machine is a Worker, but JoinConfiguration.ControlPlane is set in the KubeadmConfig object")
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

// SetupWithManager TODO
func (r *KubeadmConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cabpkv1alpha2.KubeadmConfig{}).
		Complete(r)
}

// reconcileDiscovery ensure that config.JoinConfiguration.Discovery is properly set for the joining node.
// The implementation func respect user provided discovery configurations, but in case some of them are missing, a valid BootstrapToken object
// is automatically injected into config.JoinConfiguration.Discovery.
// This allows to simplify configuration UX, by providing the option to delegate to CABPK the configuration of kubeadm join discovery.
func (r *KubeadmConfigReconciler) reconcileDiscovery(cluster *capiv1alpha2.Cluster, config *cabpkv1alpha2.KubeadmConfig) error {
	log := r.Log.WithValues("kubeadmconfig", fmt.Sprintf("%s/%s", config.Namespace, config.Name))

	// if config already contains a file discovery configuration, respect it without further validations
	if config.Spec.JoinConfiguration.Discovery.File != nil {
		return nil
	}

	// otherwise it is necessary to ensure token discovery is properly configured
	if config.Spec.JoinConfiguration.Discovery.BootstrapToken == nil {
		config.Spec.JoinConfiguration.Discovery.BootstrapToken = &kubeadmv1beta1.BootstrapTokenDiscovery{}
	}

	// if BootstrapToken already contains an APIServerEndpoint, respect it; otherwise inject the APIServerEndpoint endpoint defined in cluster status
	//TODO(fp) might be we want to validate user provided APIServerEndpoint and warn/error if it doesn't match the api endpoint defined at cluster level
	apiServerEndpoint := config.Spec.JoinConfiguration.Discovery.BootstrapToken.APIServerEndpoint
	if apiServerEndpoint == "" {
		if len(cluster.Status.APIEndpoints) == 0 {
			return errors.Wrap(&capierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}, "Waiting for Cluster Controller to set cluster.Status.APIEndpoints")
		}

		// NB. CABPK only uses the first APIServerEndpoint defined in cluster status if there are multiple defined.
		apiServerEndpoint = fmt.Sprintf("%s:%d", cluster.Status.APIEndpoints[0].Host, cluster.Status.APIEndpoints[0].Port)
		config.Spec.JoinConfiguration.Discovery.BootstrapToken.APIServerEndpoint = apiServerEndpoint
		log.Info("Altering JoinConfiguration.Discovery.BootstrapToken", "APIServerEndpoint", apiServerEndpoint)
	}

	// if BootstrapToken already contains a token, respect it; otherwise create a new bootstrap token for the node to join
	if config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token == "" {
		// gets the remote secret interface client for the current cluster
		secretsClient, err := r.SecretsClientFactory.NewSecretsClient(r.Client, cluster)
		if err != nil {
			return err
		}

		token, err := createToken(secretsClient)
		if err != nil {
			return errors.Wrapf(err, "failed to create new bootstrap token")
		}

		config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token = token
		log.Info("Altering JoinConfiguration.Discovery.BootstrapToken", "Token", token)
	}

	// if BootstrapToken already contains a CACertHashes or UnsafeSkipCAVerification, respect it; otherwise set for UnsafeSkipCAVerification
	// TODO: set CACertHashes instead of UnsafeSkipCAVerification
	if len(config.Spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes) == 0 && config.Spec.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification == false {
		config.Spec.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification = true
		log.Info("Altering JoinConfiguration.Discovery.BootstrapToken", "UnsafeSkipCAVerification", true)
	}

	return nil
}

func (r *KubeadmConfigReconciler) getClusterCertificates(ctx context.Context, clusterName, namespace string) (*certs.Certificates, error) {
	secret := &corev1.Secret{}

	err := r.Get(ctx, types.NamespacedName{Name: ClusterCertificatesSecretName(clusterName), Namespace: namespace}, secret)
	if err != nil {
		return nil, err
	}

	return certs.NewCertificatesFromMap(secret.Data), nil
}

func (r *KubeadmConfigReconciler) createClusterCertificates(ctx context.Context, clusterName string, config *cabpkv1alpha2.KubeadmConfig) (*certs.Certificates, error) {
	certificates, err := certs.NewCertificates()
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      ClusterCertificatesSecretName(clusterName),
			Namespace: config.GetNamespace(),
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion: cabpkv1alpha2.GroupVersion.String(),
					Kind:       "KubeadmConfig",
					Name:       config.GetName(),
					UID:        config.GetUID(),
				},
			},
		},
		Data: certificates.ToMap(),
	}

	if err := r.Create(ctx, secret); err != nil {
		return nil, err
	}

	return certificates, nil
}

func (r *KubeadmConfigReconciler) createKubeconfigSecret(ctx context.Context, clusterName, endpoint, namespace string, certificates *certs.Certificates) error {
	cert, err := certs.DecodeCertPEM(certificates.ClusterCA.Cert)
	if err != nil {
		return errors.Wrap(err, "failed to decode CA Cert")
	} else if cert == nil {
		return errors.New("certificate not found in config")
	}

	key, err := certs.DecodePrivateKeyPEM(certificates.ClusterCA.Key)
	if err != nil {
		return errors.Wrap(err, "failed to decode private key")
	} else if key == nil {
		return errors.New("CA private key not found")
	}

	server := fmt.Sprintf("https://%s", endpoint)
	cfg, err := certs.NewKubeconfig(clusterName, server, cert, key)
	if err != nil {
		return errors.Wrap(err, "failed to generate a kubeconfig")
	}

	yaml, err := clientcmd.Write(*cfg)
	if err != nil {
		return errors.Wrap(err, "failed to serialize config to yaml")
	}

	secret := &corev1.Secret{}
	secretName := fmt.Sprintf("%s-kubeconfig", clusterName)

	secret.ObjectMeta.Name = secretName
	secret.ObjectMeta.Namespace = namespace
	secret.StringData = map[string]string{"value": string(yaml)}

	return r.Create(ctx, secret)
}

func (r *KubeadmConfigReconciler) patchConfig(ctx context.Context, config *cabpkv1alpha2.KubeadmConfig, patchConfig client.Patch) error {
	if err := r.Patch(ctx, config, patchConfig); err != nil {
		return err
	}
	if err := r.Status().Patch(ctx, config, patchConfig); err != nil {
		return err
	}
	return nil
}
