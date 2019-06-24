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
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	bootstrapv1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/cloudinit"
	internalcluster "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/internal/cluster"
	kubeadmv1beta1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/kubeadm/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// InitLocker is a lock that is used around kubeadm init
type InitLocker interface {
	Lock(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) bool
	Unlock(ctx context.Context, cluster *clusterv1.Cluster) bool
}

// SecretsClientFactory define behaviour for creating a secrets client
type SecretsClientFactory interface {
	// NewSecretsClient returns a new client supporting SecretInterface
	NewSecretsClient(client.Client, *clusterv1.Cluster) (typedcorev1.SecretInterface, error)
}

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigs;kubeadmconfigs/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete

// KubeadmConfigReconciler reconciles a KubeadmConfig object
type KubeadmConfigReconciler struct {
	client.Client
	SecretsClientFactory SecretsClientFactory
	KubeadmInitLock      InitLocker
	Log                  logr.Logger
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *KubeadmConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1.KubeadmConfig{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(r.MachineToBootstrapMapFunc),
			},
		).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: handler.ToRequestsFunc(r.ClusterToKubeadmConfigs),
			},
		).
		Complete(r)
}

// Reconcile handles KubeadmConfig events
func (r *KubeadmConfigReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, rerr error) {
	ctx := context.Background()
	log := r.Log.WithValues("kubeadmconfig", req.NamespacedName)

	// Lookup the kubeadm config
	config := &bootstrapv1.KubeadmConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "failed to get config")
		return ctrl.Result{}, err
	}

	// Look up the Machine that owns this KubeConfig if there is one
	machine, err := util.GetOwnerMachine(ctx, r.Client, config.ObjectMeta)
	if err != nil {
		log.Error(err, "could not get owner machine")
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on the KubeadmConfig")
		return ctrl.Result{}, nil
	}
	log = log.WithValues("machine-name", machine.Name)

	// Lookup the cluster the machine is associated with
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		if errors.Cause(err) == util.ErrNoCluster {
			log.Info("Machine does not belong to a cluster yet, waiting until its part of a cluster")
			return ctrl.Result{}, nil
		}

		if apierrors.IsNotFound(err) {
			log.Info("Cluster does not exist yet , waiting until it is created")
			return ctrl.Result{}, nil
		}
		log.Error(err, "could not get cluster by machine metadata")
		return ctrl.Result{}, err
	}

	switch {
	// Wait patiently for the infrastructure to be ready
	case !cluster.Status.InfrastructureReady:
		log.Info("Infrastructure is not ready, waiting until ready.")
		return ctrl.Result{}, nil
	// bail super early if it's already ready
	case config.Status.Ready && machine.Status.InfrastructureReady:
		log.Info("ignoring config for an already ready machine")
		return ctrl.Result{}, nil
	// Reconcile status for machines that have already copied bootstrap data
	case machine.Spec.Bootstrap.Data != nil && !config.Status.Ready:
		config.Status.Ready = true
		// Initialize the patch helper
		patchHelper, err := patch.NewHelper(config, r)
		if err != nil {
			return ctrl.Result{}, err
		}
		err = patchHelper.Patch(ctx, config)
		return ctrl.Result{}, err
	// If we've already embedded a time-limited join token into a config, but are still waiting for the token to be used, refresh it
	case config.Status.Ready && (config.Spec.JoinConfiguration != nil && config.Spec.JoinConfiguration.Discovery.BootstrapToken != nil):
		token := config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token

		// gets the remote secret interface client for the current cluster
		secretsClient, err := r.SecretsClientFactory.NewSecretsClient(r.Client, cluster)
		if err != nil {
			return ctrl.Result{}, err
		}

		log.Info("refreshing token until the infrastructure has a chance to consume it")
		err = refreshToken(secretsClient, token)
		if err != nil {
			// It would be nice to re-create the bootstrap token if the error was "not found", but we have no way to update the Machine's bootstrap data
			return ctrl.Result{}, errors.Wrapf(err, "failed to refresh bootstrap token")
		}
		// NB: this may not be sufficient to keep the token live if we don't see it before it expires, but when we generate a config we will set the status to "ready" which should generate an update event
		return ctrl.Result{
			RequeueAfter: DefaultTokenTTL / 2,
		}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(config, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Attempt to Patch the KubeadmConfig object and status after each reconciliation if no error occurs.
	defer func() {
		if rerr == nil {
			if rerr = patchHelper.Patch(ctx, config); rerr != nil {
				log.Error(rerr, "failed to patch config")
			}
		}
	}()

	if !cluster.Status.ControlPlaneInitialized {
		// if it's NOT a control plane machine, requeue
		if !util.IsControlPlaneMachine(machine) {
			log.Info(fmt.Sprintf("Machine is not a control plane. If it should be a control plane, add `%s: true` as a label to the Machine", clusterv1.MachineControlPlaneLabelName))
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
		if !r.KubeadmInitLock.Lock(ctx, cluster, machine) {
			log.Info("A control plane is already being initialized, requeing until control plane is ready")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		defer func() {
			if rerr != nil {
				r.KubeadmInitLock.Unlock(ctx, cluster)
			}
		}()

		log.Info("Creating BootstrapData for the init control plane")

		// Nb. in this case JoinConfiguration should not be defined by users, but in case of misconfigurations, CABPK simply ignore it

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

		// injects into config.ClusterConfiguration values from top level object
		r.reconcileTopLevelObjectSettings(cluster, machine, config)

		clusterdata, err := kubeadmv1beta1.ConfigurationToYAML(config.Spec.ClusterConfiguration)
		if err != nil {
			log.Error(err, "failed to marshal cluster configuration")
			return ctrl.Result{}, err
		}

		certificates := internalcluster.NewCertificatesForInitialControlPlane(config.Spec.ClusterConfiguration)
		if err := certificates.LookupOrGenerate(ctx, r.Client, cluster, config); err != nil {
			log.Error(err, "unable to lookup or create cluster certificates")
			return ctrl.Result{}, err
		}

		cloudInitData, err := cloudinit.NewInitControlPlane(&cloudinit.ControlPlaneInput{
			BaseUserData: cloudinit.BaseUserData{
				AdditionalFiles:     config.Spec.Files,
				NTP:                 config.Spec.NTP,
				PreKubeadmCommands:  config.Spec.PreKubeadmCommands,
				PostKubeadmCommands: config.Spec.PostKubeadmCommands,
				Users:               config.Spec.Users,
			},
			InitConfiguration:    initdata,
			ClusterConfiguration: clusterdata,
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
	// Nb. in this case ClusterConfiguration and InitConfiguration should not be defined by users, but in case of misconfigurations, CABPK simply ignore them

	// Unlock any locks that might have been set during init process
	r.KubeadmInitLock.Unlock(ctx, cluster)

	// if the JoinConfiguration is missing, create a default one
	if config.Spec.JoinConfiguration == nil {
		log.Info("Creating default JoinConfiguration")
		config.Spec.JoinConfiguration = &kubeadmv1beta1.JoinConfiguration{}
	}

	// it's a control plane join
	if util.IsControlPlaneMachine(machine) {
		if config.Spec.JoinConfiguration.ControlPlane == nil {
			config.Spec.JoinConfiguration.ControlPlane = &kubeadmv1beta1.JoinControlPlane{}
		}

		certificates := internalcluster.NewCertificatesForJoiningControlPlane()
		if err := certificates.Lookup(ctx, r.Client, cluster); err != nil {
			log.Error(err, "unable to lookup cluster certificates")
			return ctrl.Result{}, err
		}
		if err := certificates.EnsureAllExist(); err != nil {
			return ctrl.Result{}, err
		}

		// ensure that joinConfiguration.Discovery is properly set for joining node on the current cluster
		if err := r.reconcileDiscovery(cluster, config, certificates); err != nil {
			if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
				log.Info(err.Error())
				return ctrl.Result{RequeueAfter: requeueErr.GetRequeueAfter()}, nil
			}
			return ctrl.Result{}, err
		}

		joinData, err := kubeadmv1beta1.ConfigurationToYAML(config.Spec.JoinConfiguration)
		if err != nil {
			log.Error(err, "failed to marshal join configuration")
			return ctrl.Result{}, err
		}

		log.Info("Creating BootstrapData for the join control plane")
		cloudJoinData, err := cloudinit.NewJoinControlPlane(&cloudinit.ControlPlaneJoinInput{
			JoinConfiguration: joinData,
			Certificates:      certificates,
			BaseUserData: cloudinit.BaseUserData{
				AdditionalFiles:     config.Spec.Files,
				NTP:                 config.Spec.NTP,
				PreKubeadmCommands:  config.Spec.PreKubeadmCommands,
				PostKubeadmCommands: config.Spec.PostKubeadmCommands,
				Users:               config.Spec.Users,
			},
		})
		if err != nil {
			log.Error(err, "failed to create a control plane join configuration")
			return ctrl.Result{}, err
		}

		config.Status.BootstrapData = cloudJoinData
		config.Status.Ready = true
		return ctrl.Result{}, nil
	}

	// It's a worker join
	certificates := internalcluster.NewCertificatesForWorker(config.Spec.JoinConfiguration.CACertPath)
	if err := certificates.Lookup(ctx, r.Client, cluster); err != nil {
		log.Error(err, "unable to lookup cluster certificates")
		return ctrl.Result{}, err
	}
	if err := certificates.EnsureAllExist(); err != nil {
		log.Error(err, "Missing certificates")
		return ctrl.Result{}, err
	}

	// ensure that joinConfiguration.Discovery is properly set for joining node on the current cluster
	if err := r.reconcileDiscovery(cluster, config, certificates); err != nil {
		if requeueErr, ok := errors.Cause(err).(capierrors.HasRequeueAfterError); ok {
			log.Info(err.Error())
			return ctrl.Result{RequeueAfter: requeueErr.GetRequeueAfter()}, nil
		}
		return ctrl.Result{}, err
	}

	joinData, err := kubeadmv1beta1.ConfigurationToYAML(config.Spec.JoinConfiguration)
	if err != nil {
		log.Error(err, "failed to marshal join configuration")
		return ctrl.Result{}, err
	}

	if config.Spec.JoinConfiguration.ControlPlane != nil {
		return ctrl.Result{}, errors.New("Machine is a Worker, but JoinConfiguration.ControlPlane is set in the KubeadmConfig object")
	}

	log.Info("Creating BootstrapData for the worker node")

	cloudJoinData, err := cloudinit.NewNode(&cloudinit.NodeInput{
		BaseUserData: cloudinit.BaseUserData{
			AdditionalFiles:     config.Spec.Files,
			NTP:                 config.Spec.NTP,
			PreKubeadmCommands:  config.Spec.PreKubeadmCommands,
			PostKubeadmCommands: config.Spec.PostKubeadmCommands,
			Users:               config.Spec.Users,
		},
		JoinConfiguration: joinData,
	})
	if err != nil {
		log.Error(err, "failed to create a worker join configuration")
		return ctrl.Result{}, err
	}
	config.Status.BootstrapData = cloudJoinData
	config.Status.Ready = true
	return ctrl.Result{}, nil
}

// ClusterToKubeadmConfigs is a handler.ToRequestsFunc to be used to enqeue
// requests for reconciliation of KubeadmConfigs.
func (r *KubeadmConfigReconciler) ClusterToKubeadmConfigs(o handler.MapObject) []ctrl.Request {
	result := []ctrl.Request{}

	c, ok := o.Object.(*clusterv1.Cluster)
	if !ok {
		r.Log.Error(errors.Errorf("expected a Cluster but got a %T", o.Object), "failed to get Machine for Cluster")
		return nil
	}

	selectors := []client.ListOption{
		client.InNamespace(c.Namespace),
		client.MatchingLabels{
			clusterv1.MachineClusterLabelName: c.Name,
		},
	}

	machineList := &clusterv1.MachineList{}
	if err := r.List(context.Background(), machineList, selectors...); err != nil {
		r.Log.Error(err, "failed to list Machines", "Cluster", c.Name, "Namespace", c.Namespace)
		return nil
	}

	for _, m := range machineList.Items {
		if m.Spec.Bootstrap.ConfigRef != nil &&
			m.Spec.Bootstrap.ConfigRef.GroupVersionKind() == bootstrapv1.GroupVersion.WithKind("KubeadmConfig") {
			name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}
			result = append(result, ctrl.Request{NamespacedName: name})
		}
	}

	return result
}

// MachineToBootstrapMapFunc is a handler.ToRequestsFunc to be used to enqeue
// request for reconciliation of KubeadmConfig.
func (r *KubeadmConfigReconciler) MachineToBootstrapMapFunc(o handler.MapObject) []ctrl.Request {
	result := []ctrl.Request{}

	m, ok := o.Object.(*clusterv1.Machine)
	if !ok {
		return nil
	}
	if m.Spec.Bootstrap.ConfigRef != nil && m.Spec.Bootstrap.ConfigRef.GroupVersionKind() == bootstrapv1.GroupVersion.WithKind("KubeadmConfig") {
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}
	return result
}

// reconcileDiscovery ensures that config.JoinConfiguration.Discovery is properly set for the joining node.
// The implementation func respect user provided discovery configurations, but in case some of them are missing, a valid BootstrapToken object
// is automatically injected into config.JoinConfiguration.Discovery.
// This allows to simplify configuration UX, by providing the option to delegate to CABPK the configuration of kubeadm join discovery.
func (r *KubeadmConfigReconciler) reconcileDiscovery(cluster *clusterv1.Cluster, config *bootstrapv1.KubeadmConfig, certificates internalcluster.Certificates) error {
	log := r.Log.WithValues("kubeadmconfig", fmt.Sprintf("%s/%s", config.Namespace, config.Name))

	// if config already contains a file discovery configuration, respect it without further validations
	if config.Spec.JoinConfiguration.Discovery.File != nil {
		return nil
	}

	// otherwise it is necessary to ensure token discovery is properly configured
	if config.Spec.JoinConfiguration.Discovery.BootstrapToken == nil {
		config.Spec.JoinConfiguration.Discovery.BootstrapToken = &kubeadmv1beta1.BootstrapTokenDiscovery{}
	}

	// calculate the ca cert hashes if they are not already set
	if len(config.Spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes) == 0 {
		hashes, err := certificates.GetByPurpose(secret.ClusterCA).Hashes()
		if err != nil {
			log.Error(err, "Unable to generate Cluster CA certificate hashes")
			return err
		}
		config.Spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes = hashes
	}

	// if BootstrapToken already contains an APIServerEndpoint, respect it; otherwise inject the APIServerEndpoint endpoint defined in cluster status
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

	// If the BootstrapToken does not contain any CACertHashes then force skip CA Verification
	if len(config.Spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes) == 0 {
		log.Info("No CAs were provided. Falling back to insecure discover method by skipping CA Cert validation")
		config.Spec.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification = true
	}

	return nil
}

// reconcileTopLevelObjectSettings injects into config.ClusterConfiguration values from top level objects like cluster and machine.
// The implementation func respect user provided config values, but in case some of them are missing, values from top level objects are used.
func (r *KubeadmConfigReconciler) reconcileTopLevelObjectSettings(cluster *clusterv1.Cluster, machine *clusterv1.Machine, config *bootstrapv1.KubeadmConfig) {
	log := r.Log.WithValues("kubeadmconfig", fmt.Sprintf("%s/%s", config.Namespace, config.Name))

	// If there are no ControlPlaneEndpoint defined in ClusterConfiguration but there are APIEndpoints defined at cluster level (e.g. the load balancer endpoint),
	// then use cluster APIEndpoints as a control plane endpoint for the K8s cluster
	if config.Spec.ClusterConfiguration.ControlPlaneEndpoint == "" && len(cluster.Status.APIEndpoints) > 0 {
		// NB. CABPK only uses the first APIServerEndpoint defined in cluster status if there are multiple defined.
		config.Spec.ClusterConfiguration.ControlPlaneEndpoint = fmt.Sprintf("%s:%d", cluster.Status.APIEndpoints[0].Host, cluster.Status.APIEndpoints[0].Port)
		log.Info("Altering ClusterConfiguration", "ControlPlaneEndpoint", config.Spec.ClusterConfiguration.ControlPlaneEndpoint)
	}

	// If there are no ClusterName defined in ClusterConfiguration, use Cluster.Name
	if config.Spec.ClusterConfiguration.ClusterName == "" {
		config.Spec.ClusterConfiguration.ClusterName = cluster.Name
		log.Info("Altering ClusterConfiguration", "ClusterName", config.Spec.ClusterConfiguration.ClusterName)
	}

	// If there are no Network settings defined in ClusterConfiguration, use ClusterNetwork settings, if defined
	if cluster.Spec.ClusterNetwork != nil {
		if config.Spec.ClusterConfiguration.Networking.DNSDomain == "" && cluster.Spec.ClusterNetwork.ServiceDomain != "" {
			config.Spec.ClusterConfiguration.Networking.DNSDomain = cluster.Spec.ClusterNetwork.ServiceDomain
			log.Info("Altering ClusterConfiguration", "DNSDomain", config.Spec.ClusterConfiguration.Networking.DNSDomain)
		}
		if config.Spec.ClusterConfiguration.Networking.ServiceSubnet == "" &&
			cluster.Spec.ClusterNetwork.Services != nil &&
			len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
			config.Spec.ClusterConfiguration.Networking.ServiceSubnet = strings.Join(cluster.Spec.ClusterNetwork.Services.CIDRBlocks, "")
			log.Info("Altering ClusterConfiguration", "ServiceSubnet", config.Spec.ClusterConfiguration.Networking.ServiceSubnet)
		}
		if config.Spec.ClusterConfiguration.Networking.PodSubnet == "" &&
			cluster.Spec.ClusterNetwork.Pods != nil &&
			len(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
			config.Spec.ClusterConfiguration.Networking.PodSubnet = strings.Join(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks, "")
			log.Info("Altering ClusterConfiguration", "PodSubnet", config.Spec.ClusterConfiguration.Networking.PodSubnet)
		}
	}

	// If there are no KubernetesVersion settings defined in ClusterConfiguration, use Version from machine, if defined
	if config.Spec.ClusterConfiguration.KubernetesVersion == "" && machine.Spec.Version != nil {
		config.Spec.ClusterConfiguration.KubernetesVersion = *machine.Spec.Version
		log.Info("Altering ClusterConfiguration", "KubernetesVersion", config.Spec.ClusterConfiguration.KubernetesVersion)
	}
}
