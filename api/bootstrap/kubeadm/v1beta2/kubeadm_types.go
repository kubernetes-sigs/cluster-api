/*
Copyright 2025 The Kubernetes Authors.

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

package v1beta2

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// KubeadmConfig's Ready condition and corresponding reasons.
const (
	// KubeadmConfigReadyCondition is true if the KubeadmConfig is not deleted,
	// and both DataSecretCreated, CertificatesAvailable conditions are true.
	KubeadmConfigReadyCondition = clusterv1.ReadyCondition

	// KubeadmConfigReadyReason surfaces when the KubeadmConfig is ready.
	KubeadmConfigReadyReason = clusterv1.ReadyReason

	// KubeadmConfigNotReadyReason surfaces when the KubeadmConfig is not ready.
	KubeadmConfigNotReadyReason = clusterv1.NotReadyReason

	// KubeadmConfigReadyUnknownReason surfaces when KubeadmConfig readiness is unknown.
	KubeadmConfigReadyUnknownReason = clusterv1.ReadyUnknownReason
)

// KubeadmConfig's CertificatesAvailable condition and corresponding reasons.
const (
	// KubeadmConfigCertificatesAvailableCondition documents that cluster certificates required
	// for generating the bootstrap data secret are available.
	KubeadmConfigCertificatesAvailableCondition = "CertificatesAvailable"

	// KubeadmConfigCertificatesAvailableReason surfaces when certificates required for machine bootstrap are is available.
	KubeadmConfigCertificatesAvailableReason = clusterv1.AvailableReason

	// KubeadmConfigCertificatesAvailableInternalErrorReason surfaces unexpected failures when reading or
	// generating certificates required for machine bootstrap.
	KubeadmConfigCertificatesAvailableInternalErrorReason = clusterv1.InternalErrorReason
)

// KubeadmConfig's DataSecretAvailable condition and corresponding reasons.
const (
	// KubeadmConfigDataSecretAvailableCondition is true if the bootstrap secret is available.
	KubeadmConfigDataSecretAvailableCondition = "DataSecretAvailable"

	// KubeadmConfigDataSecretAvailableReason surfaces when the bootstrap secret is available.
	KubeadmConfigDataSecretAvailableReason = clusterv1.AvailableReason

	// KubeadmConfigDataSecretNotAvailableReason surfaces when the bootstrap secret is not available.
	KubeadmConfigDataSecretNotAvailableReason = clusterv1.NotAvailableReason
)

// InitConfiguration contains a list of elements that is specific "kubeadm init"-only runtime
// information.
// +kubebuilder:validation:MinProperties=1
type InitConfiguration struct {
	// bootstrapTokens is respected at `kubeadm init` time and describes a set of Bootstrap Tokens to create.
	// This information IS NOT uploaded to the kubeadm cluster configmap, partly because of its sensitive nature
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	BootstrapTokens []BootstrapToken `json:"bootstrapTokens,omitempty"`

	// nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.
	// When used in the context of control plane nodes, NodeRegistration should remain consistent
	// across both InitConfiguration and JoinConfiguration
	// +optional
	NodeRegistration NodeRegistrationOptions `json:"nodeRegistration,omitempty,omitzero"`

	// localAPIEndpoint represents the endpoint of the API server instance that's deployed on this control plane node
	// In HA setups, this differs from ClusterConfiguration.ControlPlaneEndpoint in the sense that ControlPlaneEndpoint
	// is the global endpoint for the cluster, which then loadbalances the requests to each individual API server. This
	// configuration object lets you customize what IP/DNS name and port the local API server advertises it's accessible
	// on. By default, kubeadm tries to auto-detect the IP of the default interface and use that, but in case that process
	// fails you may set the desired value here.
	// +optional
	LocalAPIEndpoint APIEndpoint `json:"localAPIEndpoint,omitempty,omitzero"`

	// skipPhases is a list of phases to skip during command execution.
	// The list of phases can be obtained with the "kubeadm init --help" command.
	// This option takes effect only on Kubernetes >=1.22.0.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	SkipPhases []string `json:"skipPhases,omitempty"`

	// patches contains options related to applying patches to components deployed by kubeadm during
	// "kubeadm init". The minimum kubernetes version needed to support Patches is v1.22
	// +optional
	Patches Patches `json:"patches,omitempty,omitzero"`

	// timeouts holds various timeouts that apply to kubeadm commands.
	// +optional
	Timeouts Timeouts `json:"timeouts,omitempty,omitzero"`
}

// IsDefined returns true if the InitConfiguration is defined.
func (r *InitConfiguration) IsDefined() bool {
	return !reflect.DeepEqual(r, &InitConfiguration{})
}

// ClusterConfiguration contains cluster-wide configuration for a kubeadm cluster.
// +kubebuilder:validation:MinProperties=1
type ClusterConfiguration struct {
	// etcd holds configuration for etcd.
	// NB: This value defaults to a Local (stacked) etcd
	// +optional
	Etcd Etcd `json:"etcd,omitempty,omitzero"`

	// controlPlaneEndpoint sets a stable IP address or DNS name for the control plane; it
	// can be a valid IP address or a RFC-1123 DNS subdomain, both with optional TCP port.
	// In case the ControlPlaneEndpoint is not specified, the AdvertiseAddress + BindPort
	// are used; in case the ControlPlaneEndpoint is specified but without a TCP port,
	// the BindPort is used.
	// Possible usages are:
	// e.g. In a cluster with more than one control plane instances, this field should be
	// assigned the address of the external load balancer in front of the
	// control plane instances.
	// e.g.  in environments with enforced node recycling, the ControlPlaneEndpoint
	// could be used for assigning a stable DNS to the control plane.
	// NB: This value defaults to the first value in the Cluster object status.apiEndpoints array.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ControlPlaneEndpoint string `json:"controlPlaneEndpoint,omitempty"`

	// apiServer contains extra settings for the API server control plane component
	// +optional
	APIServer APIServer `json:"apiServer,omitempty,omitzero"`

	// controllerManager contains extra settings for the controller manager control plane component
	// +optional
	ControllerManager ControllerManager `json:"controllerManager,omitempty,omitzero"`

	// scheduler contains extra settings for the scheduler control plane component
	// +optional
	Scheduler Scheduler `json:"scheduler,omitempty,omitzero"`

	// dns defines the options for the DNS add-on installed in the cluster.
	// +optional
	DNS DNS `json:"dns,omitempty,omitzero"`

	// certificatesDir specifies where to store or look for all required certificates.
	// NB: if not provided, this will default to `/etc/kubernetes/pki`
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	CertificatesDir string `json:"certificatesDir,omitempty"`

	// imageRepository sets the container registry to pull images from.
	// * If not set, the default registry of kubeadm will be used, i.e.
	//   * registry.k8s.io (new registry): >= v1.22.17, >= v1.23.15, >= v1.24.9, >= v1.25.0
	//   * k8s.gcr.io (old registry): all older versions
	//   Please note that when imageRepository is not set we don't allow upgrades to
	//   versions >= v1.22.0 which use the old registry (k8s.gcr.io). Please use
	//   a newer patch version with the new registry instead (i.e. >= v1.22.17,
	//   >= v1.23.15, >= v1.24.9, >= v1.25.0).
	// * If the version is a CI build (kubernetes version starts with `ci/` or `ci-cross/`)
	//  `gcr.io/k8s-staging-ci-images` will be used as a default for control plane components
	//   and for kube-proxy, while `registry.k8s.io` will be used for all the other images.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ImageRepository string `json:"imageRepository,omitempty"`

	// featureGates enabled by the user.
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`

	// certificateValidityPeriodDays specifies the validity period for non-CA certificates generated by kubeadm.
	// If not specified, kubeadm will use a default of 365 days (1 year).
	// This field is only supported with Kubernetes v1.31 or above.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1095
	CertificateValidityPeriodDays int32 `json:"certificateValidityPeriodDays,omitempty"`

	// caCertificateValidityPeriodDays specifies the validity period for CA certificates generated by Cluster API.
	// If not specified, Cluster API will use a default of 3650 days (10 years).
	// This field cannot be modified.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=36500
	CACertificateValidityPeriodDays int32 `json:"caCertificateValidityPeriodDays,omitempty"`
}

// IsDefined returns true if the ClusterConfiguration is defined.
func (r *ClusterConfiguration) IsDefined() bool {
	return !reflect.DeepEqual(r, &ClusterConfiguration{})
}

// APIServer holds settings necessary for API server deployments in the cluster.
// +kubebuilder:validation:MinProperties=1
type APIServer struct {
	// extraArgs is a list of args to pass to the control plane component.
	// The arg name must match the command line flag name except without leading dash(es).
	// Extra arguments will override existing default arguments set by kubeadm.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=value
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.name == y.name))",message="extraArgs name must be unique"
	ExtraArgs []Arg `json:"extraArgs,omitempty"`

	// extraVolumes is an extra set of host volumes, mounted to the control plane component.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	ExtraVolumes []HostPathMount `json:"extraVolumes,omitempty"`

	// extraEnvs is an extra set of environment variables to pass to the control plane component.
	// Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.
	// This option takes effect only on Kubernetes >=1.31.0.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	ExtraEnvs *[]EnvVar `json:"extraEnvs,omitempty"`

	// certSANs sets extra Subject Alternative Names for the API Server signing cert.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=253
	CertSANs []string `json:"certSANs,omitempty"`
}

// ControllerManager holds settings necessary for controller-manager deployments in the cluster.
// +kubebuilder:validation:MinProperties=1
type ControllerManager struct {
	// extraArgs is a list of args to pass to the control plane component.
	// The arg name must match the command line flag name except without leading dash(es).
	// Extra arguments will override existing default arguments set by kubeadm.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=value
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.name == y.name))",message="extraArgs name must be unique"
	ExtraArgs []Arg `json:"extraArgs,omitempty"`

	// extraVolumes is an extra set of host volumes, mounted to the control plane component.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	ExtraVolumes []HostPathMount `json:"extraVolumes,omitempty"`

	// extraEnvs is an extra set of environment variables to pass to the control plane component.
	// Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.
	// This option takes effect only on Kubernetes >=1.31.0.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	ExtraEnvs *[]EnvVar `json:"extraEnvs,omitempty"`
}

// Scheduler holds settings necessary for scheduler deployments in the cluster.
// +kubebuilder:validation:MinProperties=1
type Scheduler struct {
	// extraArgs is a list of args to pass to the control plane component.
	// The arg name must match the command line flag name except without leading dash(es).
	// Extra arguments will override existing default arguments set by kubeadm.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=value
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.name == y.name))",message="extraArgs name must be unique"
	ExtraArgs []Arg `json:"extraArgs,omitempty"`

	// extraVolumes is an extra set of host volumes, mounted to the control plane component.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	ExtraVolumes []HostPathMount `json:"extraVolumes,omitempty"`

	// extraEnvs is an extra set of environment variables to pass to the control plane component.
	// Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.
	// This option takes effect only on Kubernetes >=1.31.0.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	ExtraEnvs *[]EnvVar `json:"extraEnvs,omitempty"`
}

// DNS defines the DNS addon that should be used in the cluster.
// +kubebuilder:validation:MinProperties=1
type DNS struct {
	// imageRepository sets the container registry to pull images from.
	// if not set, the ImageRepository defined in ClusterConfiguration will be used instead.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ImageRepository string `json:"imageRepository,omitempty"`

	// imageTag allows to specify a tag for the image.
	// In case this value is set, kubeadm does not change automatically the version of the above components during upgrades.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	ImageTag string `json:"imageTag,omitempty"`

	// TODO: evaluate if we need also a ImageName based on user feedbacks
}

// APIEndpoint struct contains elements of API server instance deployed on a node.
// +kubebuilder:validation:MinProperties=1
type APIEndpoint struct {
	// advertiseAddress sets the IP address for the API server to advertise.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=39
	AdvertiseAddress string `json:"advertiseAddress,omitempty"`

	// bindPort sets the secure port for the API Server to bind to.
	// Defaults to 6443.
	// +optional
	// +kubebuilder:validation:Minimum=1
	BindPort int32 `json:"bindPort,omitempty"`
}

// NodeRegistrationOptions holds fields that relate to registering a new control-plane or node to the cluster, either via "kubeadm init" or "kubeadm join".
// Note: The NodeRegistrationOptions struct has to be kept in sync with the structs in MarshalJSON.
// +kubebuilder:validation:MinProperties=1
type NodeRegistrationOptions struct {
	// name is the `.Metadata.Name` field of the Node API object that will be created in this `kubeadm init` or `kubeadm join` operation.
	// This field is also used in the CommonName field of the kubelet's client certificate to the API server.
	// Defaults to the hostname of the node if not provided.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// criSocket is used to retrieve container runtime info. This information will be annotated to the Node API object, for later re-use
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	CRISocket string `json:"criSocket,omitempty"`

	// taints specifies the taints the Node API object should be registered with. If this field is unset, i.e. nil, in the `kubeadm init` process
	// it will be defaulted to []v1.Taint{'node-role.kubernetes.io/master=""'}. If you don't want to taint your control-plane node, set this field to an
	// empty slice, i.e. `taints: []` in the YAML file. This field is solely used for Node registration.
	// +optional
	// +kubebuilder:validation:MinItems=0
	// +kubebuilder:validation:MaxItems=100
	Taints *[]corev1.Taint `json:"taints,omitempty"`

	// kubeletExtraArgs is a list of args to pass to kubelet.
	// The arg name must match the command line flag name except without leading dash(es).
	// Extra arguments will override existing default arguments set by kubeadm.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=value
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.name == y.name))",message="kubeletExtraArgs name must be unique"
	KubeletExtraArgs []Arg `json:"kubeletExtraArgs,omitempty"`

	// ignorePreflightErrors provides a slice of pre-flight errors to be ignored when the current node is registered, e.g. 'IsPrivilegedUser,Swap'.
	// Value 'all' ignores errors from all checks.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	IgnorePreflightErrors []string `json:"ignorePreflightErrors,omitempty"`

	// imagePullPolicy specifies the policy for image pulling
	// during kubeadm "init" and "join" operations. The value of
	// this field must be one of "Always", "IfNotPresent" or
	// "Never". Defaults to "IfNotPresent" if not set.
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// imagePullSerial specifies if image pulling performed by kubeadm must be done serially or in parallel.
	// This option takes effect only on Kubernetes >=1.31.0.
	// Default: true (defaulted in kubeadm)
	// +optional
	ImagePullSerial *bool `json:"imagePullSerial,omitempty"`
}

// BootstrapToken describes one bootstrap token, stored as a Secret in the cluster.
type BootstrapToken struct {
	// token is used for establishing bidirectional trust between nodes and control-planes.
	// Used for joining nodes in the cluster.
	// +required
	Token BootstrapTokenString `json:"token,omitempty,omitzero"`

	// description sets a human-friendly message why this token exists and what it's used
	// for, so other administrators can know its purpose.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Description string `json:"description,omitempty"`

	// ttlSeconds defines the time to live for this token. Defaults to 24h.
	// Expires and ttlSeconds are mutually exclusive.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TTLSeconds *int32 `json:"ttlSeconds,omitempty"`

	// expires specifies the timestamp when this token expires. Defaults to being set
	// dynamically at runtime based on the ttlSeconds. Expires and ttlSeconds are mutually exclusive.
	// +optional
	Expires metav1.Time `json:"expires,omitempty,omitzero"`

	// usages describes the ways in which this token can be used. Can by default be used
	// for establishing bidirectional trust, but that can be changed here.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	Usages []string `json:"usages,omitempty"`

	// groups specifies the extra groups that this token will authenticate as when/if
	// used for authentication
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	Groups []string `json:"groups,omitempty"`
}

// Etcd contains elements describing Etcd configuration.
// +kubebuilder:validation:MinProperties=1
type Etcd struct {
	// local provides configuration knobs for configuring the local etcd instance
	// Local and External are mutually exclusive
	// +optional
	Local LocalEtcd `json:"local,omitempty,omitzero"`

	// external describes how to connect to an external etcd cluster
	// Local and External are mutually exclusive
	// +optional
	External ExternalEtcd `json:"external,omitempty,omitzero"`
}

// LocalEtcd describes that kubeadm should run an etcd cluster locally.
// +kubebuilder:validation:MinProperties=1
type LocalEtcd struct {
	// imageRepository sets the container registry to pull images from.
	// if not set, the ImageRepository defined in ClusterConfiguration will be used instead.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ImageRepository string `json:"imageRepository,omitempty"`

	// imageTag allows to specify a tag for the image.
	// In case this value is set, kubeadm does not change automatically the version of the above components during upgrades.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	ImageTag string `json:"imageTag,omitempty"`

	// TODO: evaluate if we need also a ImageName based on user feedbacks

	// dataDir is the directory etcd will place its data.
	// Defaults to "/var/lib/etcd".
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	DataDir string `json:"dataDir,omitempty"`

	// extraArgs is a list of args to pass to etcd.
	// The arg name must match the command line flag name except without leading dash(es).
	// Extra arguments will override existing default arguments set by kubeadm.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=value
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.name == y.name))",message="extraArgs name must be unique"
	ExtraArgs []Arg `json:"extraArgs,omitempty"`

	// extraEnvs is an extra set of environment variables to pass to etcd.
	// Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.
	// This option takes effect only on Kubernetes >=1.31.0.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	ExtraEnvs *[]EnvVar `json:"extraEnvs,omitempty"`

	// serverCertSANs sets extra Subject Alternative Names for the etcd server signing cert.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=253
	ServerCertSANs []string `json:"serverCertSANs,omitempty"`

	// peerCertSANs sets extra Subject Alternative Names for the etcd peer signing cert.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=253
	PeerCertSANs []string `json:"peerCertSANs,omitempty"`
}

// IsDefined returns true if the LocalEtcd is defined.
func (r *LocalEtcd) IsDefined() bool {
	return !reflect.DeepEqual(r, &LocalEtcd{})
}

// ExternalEtcd describes an external etcd cluster.
// Kubeadm has no knowledge of where certificate files live and they must be supplied.
type ExternalEtcd struct {
	// endpoints of etcd members. Required for ExternalEtcd.
	// +required
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	Endpoints []string `json:"endpoints,omitempty"`

	// caFile is an SSL Certificate Authority file used to secure etcd communication.
	// Required if using a TLS connection.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	CAFile string `json:"caFile,omitempty"`

	// certFile is an SSL certification file used to secure etcd communication.
	// Required if using a TLS connection.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	CertFile string `json:"certFile,omitempty"`

	// keyFile is an SSL key file used to secure etcd communication.
	// Required if using a TLS connection.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	KeyFile string `json:"keyFile,omitempty"`
}

// IsDefined returns true if the ExternalEtcd is defined.
func (r *ExternalEtcd) IsDefined() bool {
	return !reflect.DeepEqual(r, &ExternalEtcd{})
}

// JoinConfiguration contains elements describing a particular node.
// +kubebuilder:validation:MinProperties=1
type JoinConfiguration struct {
	// nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.
	// When used in the context of control plane nodes, NodeRegistration should remain consistent
	// across both InitConfiguration and JoinConfiguration
	// +optional
	NodeRegistration NodeRegistrationOptions `json:"nodeRegistration,omitempty,omitzero"`

	// caCertPath is the path to the SSL certificate authority used to
	// secure communications between node and control-plane.
	// Defaults to "/etc/kubernetes/pki/ca.crt".
	// +optional
	// TODO: revisit when there is defaulting from k/k
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	CACertPath string `json:"caCertPath,omitempty"`

	// discovery specifies the options for the kubelet to use during the TLS Bootstrap process
	// +optional
	// TODO: revisit when there is defaulting from k/k
	Discovery Discovery `json:"discovery,omitempty,omitzero"`

	// controlPlane defines the additional control plane instance to be deployed on the joining node.
	// If nil, no additional control plane instance will be deployed.
	// +optional
	ControlPlane *JoinControlPlane `json:"controlPlane,omitempty"`

	// skipPhases is a list of phases to skip during command execution.
	// The list of phases can be obtained with the "kubeadm init --help" command.
	// This option takes effect only on Kubernetes >=1.22.0.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	SkipPhases []string `json:"skipPhases,omitempty"`

	// patches contains options related to applying patches to components deployed by kubeadm during
	// "kubeadm join". The minimum kubernetes version needed to support Patches is v1.22
	// +optional
	Patches Patches `json:"patches,omitempty,omitzero"`

	// timeouts holds various timeouts that apply to kubeadm commands.
	// +optional
	Timeouts Timeouts `json:"timeouts,omitempty,omitzero"`
}

// IsDefined returns true if the JoinConfiguration is defined.
func (r *JoinConfiguration) IsDefined() bool {
	return !reflect.DeepEqual(r, &JoinConfiguration{})
}

// JoinControlPlane contains elements describing an additional control plane instance to be deployed on the joining node.
type JoinControlPlane struct {
	// localAPIEndpoint represents the endpoint of the API server instance to be deployed on this node.
	// +optional
	LocalAPIEndpoint APIEndpoint `json:"localAPIEndpoint,omitempty,omitzero"`
}

// Discovery specifies the options for the kubelet to use during the TLS Bootstrap process.
// +kubebuilder:validation:MinProperties=1
type Discovery struct {
	// bootstrapToken is used to set the options for bootstrap token based discovery
	// BootstrapToken and File are mutually exclusive
	// +optional
	BootstrapToken BootstrapTokenDiscovery `json:"bootstrapToken,omitempty,omitzero"`

	// file is used to specify a file or URL to a kubeconfig file from which to load cluster information
	// BootstrapToken and File are mutually exclusive
	// +optional
	File FileDiscovery `json:"file,omitempty,omitzero"`

	// tlsBootstrapToken is a token used for TLS bootstrapping.
	// If .BootstrapToken is set, this field is defaulted to .BootstrapToken.Token, but can be overridden.
	// If .File is set, this field **must be set** in case the KubeConfigFile does not contain any other authentication information
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	TLSBootstrapToken string `json:"tlsBootstrapToken,omitempty"`
}

// BootstrapTokenDiscovery is used to set the options for bootstrap token based discovery.
// +kubebuilder:validation:MinProperties=1
type BootstrapTokenDiscovery struct {
	// token is a token used to validate cluster information
	// fetched from the control-plane.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Token string `json:"token,omitempty"`

	// apiServerEndpoint is an IP or domain name to the API server from which info will be fetched.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	APIServerEndpoint string `json:"apiServerEndpoint,omitempty"`

	// caCertHashes specifies a set of public key pins to verify
	// when token-based discovery is used. The root CA found during discovery
	// must match one of these values. Specifying an empty set disables root CA
	// pinning, which can be unsafe. Each hash is specified as "<type>:<value>",
	// where the only currently supported type is "sha256". This is a hex-encoded
	// SHA-256 hash of the Subject Public Key Info (SPKI) object in DER-encoded
	// ASN.1. These hashes can be calculated using, for example, OpenSSL:
	// openssl x509 -pubkey -in ca.crt openssl rsa -pubin -outform der 2>&/dev/null | openssl dgst -sha256 -hex
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	CACertHashes []string `json:"caCertHashes,omitempty"`

	// unsafeSkipCAVerification allows token-based discovery
	// without CA verification via CACertHashes. This can weaken
	// the security of kubeadm since other nodes can impersonate the control-plane.
	// +optional
	UnsafeSkipCAVerification *bool `json:"unsafeSkipCAVerification,omitempty"`
}

// IsDefined returns true if the BootstrapTokenDiscovery is defined.
func (r *BootstrapTokenDiscovery) IsDefined() bool {
	return !reflect.DeepEqual(r, &BootstrapTokenDiscovery{})
}

// FileDiscovery is used to specify a file or URL to a kubeconfig file from which to load cluster information.
type FileDiscovery struct {
	// kubeConfigPath is used to specify the actual file path or URL to the kubeconfig file from which to load cluster information
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	KubeConfigPath string `json:"kubeConfigPath,omitempty"`

	// kubeConfig is used (optionally) to generate a KubeConfig based on the KubeadmConfig's information.
	// The file is generated at the path specified in KubeConfigPath.
	//
	// Host address (server field) information is automatically populated based on the Cluster's ControlPlaneEndpoint.
	// Certificate Authority (certificate-authority-data field) is gathered from the cluster's CA secret.
	//
	// +optional
	KubeConfig FileDiscoveryKubeConfig `json:"kubeConfig,omitempty,omitzero"`
}

// IsDefined returns true if the FileDiscovery is defined.
func (r *FileDiscovery) IsDefined() bool {
	return !reflect.DeepEqual(r, &FileDiscovery{})
}

// FileDiscoveryKubeConfig contains elements describing how to generate the kubeconfig for bootstrapping.
type FileDiscoveryKubeConfig struct {
	// cluster contains information about how to communicate with the kubernetes cluster.
	//
	// By default the following fields are automatically populated:
	// - Server with the Cluster's ControlPlaneEndpoint.
	// - CertificateAuthorityData with the Cluster's CA certificate.
	// +optional
	Cluster KubeConfigCluster `json:"cluster,omitempty,omitzero"`

	// user contains information that describes identity information.
	// This is used to tell the kubernetes cluster who you are.
	// +required
	User KubeConfigUser `json:"user,omitempty,omitzero"`
}

// IsDefined returns true if the FileDiscoveryKubeConfig is defined.
func (r *FileDiscoveryKubeConfig) IsDefined() bool {
	return !reflect.DeepEqual(r, &FileDiscoveryKubeConfig{})
}

// KubeConfigCluster contains information about how to communicate with a kubernetes cluster.
//
// Adapted from clientcmdv1.Cluster.
// +kubebuilder:validation:MinProperties=1
type KubeConfigCluster struct {
	// server is the address of the kubernetes cluster (https://hostname:port).
	//
	// Defaults to https:// + Cluster.Spec.ControlPlaneEndpoint.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Server string `json:"server,omitempty"`

	// tlsServerName is used to check server certificate. If TLSServerName is empty, the hostname used to contact the server is used.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	TLSServerName string `json:"tlsServerName,omitempty"`

	// insecureSkipTLSVerify skips the validity check for the server's certificate. This will make your HTTPS connections insecure.
	// +optional
	InsecureSkipTLSVerify *bool `json:"insecureSkipTLSVerify,omitempty"`

	// certificateAuthorityData contains PEM-encoded certificate authority certificates.
	//
	// Defaults to the Cluster's CA certificate if empty.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=51200
	CertificateAuthorityData []byte `json:"certificateAuthorityData,omitempty"`

	// proxyURL is the URL to the proxy to be used for all requests made by this
	// client. URLs with "http", "https", and "socks5" schemes are supported.  If
	// this configuration is not provided or the empty string, the client
	// attempts to construct a proxy configuration from http_proxy and
	// https_proxy environment variables. If these environment variables are not
	// set, the client does not attempt to proxy requests.
	//
	// socks5 proxying does not currently support spdy streaming endpoints (exec,
	// attach, port forward).
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ProxyURL string `json:"proxyURL,omitempty"`
}

// IsDefined returns true if the KubeConfigCluster is defined.
func (r *KubeConfigCluster) IsDefined() bool {
	return !reflect.DeepEqual(r, &KubeConfigCluster{})
}

// KubeConfigUser contains information that describes identity information.
// This is used to tell the kubernetes cluster who you are.
//
// Either authProvider or exec must be filled.
//
// Adapted from clientcmdv1.AuthInfo.
// +kubebuilder:validation:MinProperties=1
type KubeConfigUser struct {
	// authProvider specifies a custom authentication plugin for the kubernetes cluster.
	// +optional
	AuthProvider KubeConfigAuthProvider `json:"authProvider,omitempty,omitzero"`

	// exec specifies a custom exec-based authentication plugin for the kubernetes cluster.
	// +optional
	Exec KubeConfigAuthExec `json:"exec,omitempty,omitzero"`
}

// KubeConfigAuthProvider holds the configuration for a specified auth provider.
type KubeConfigAuthProvider struct {
	// name is the name of the authentication plugin.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name,omitempty"`

	// config holds the parameters for the authentication plugin.
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// IsDefined returns true if the KubeConfigAuthProvider is defined.
func (r *KubeConfigAuthProvider) IsDefined() bool {
	return !reflect.DeepEqual(r, &KubeConfigAuthProvider{})
}

// KubeConfigAuthExec specifies a command to provide client credentials. The command is exec'd
// and outputs structured stdout holding credentials.
//
// See the client.authentication.k8s.io API group for specifications of the exact input
// and output format.
type KubeConfigAuthExec struct {
	// command to execute.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Command string `json:"command,omitempty"`

	// args is the arguments to pass to the command when executing it.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	Args []string `json:"args,omitempty"`

	// env defines additional environment variables to expose to the process. These
	// are unioned with the host's environment, as well as variables client-go uses
	// to pass argument to the plugin.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Env []KubeConfigAuthExecEnv `json:"env,omitempty"`

	// apiVersion is preferred input version of the ExecInfo. The returned ExecCredentials MUST use
	// the same encoding version as the input.
	// Defaults to client.authentication.k8s.io/v1 if not set.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	APIVersion string `json:"apiVersion,omitempty"`

	// provideClusterInfo determines whether or not to provide cluster information,
	// which could potentially contain very large CA data, to this exec plugin as a
	// part of the KUBERNETES_EXEC_INFO environment variable. By default, it is set
	// to false. Package k8s.io/client-go/tools/auth/exec provides helper methods for
	// reading this environment variable.
	// +optional
	ProvideClusterInfo *bool `json:"provideClusterInfo,omitempty"`
}

// IsDefined returns true if the KubeConfigAuthExec is defined.
func (r *KubeConfigAuthExec) IsDefined() bool {
	return !reflect.DeepEqual(r, &KubeConfigAuthExec{})
}

// KubeConfigAuthExecEnv is used for setting environment variables when executing an exec-based
// credential plugin.
type KubeConfigAuthExecEnv struct {
	// name of the environment variable
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Name string `json:"name,omitempty"`

	// value of the environment variable
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Value string `json:"value,omitempty"`
}

// HostPathMount contains elements describing volumes that are mounted from the
// host.
type HostPathMount struct {
	// name of the volume inside the pod template.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Name string `json:"name,omitempty"`

	// hostPath is the path in the host that will be mounted inside
	// the pod.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	HostPath string `json:"hostPath,omitempty"`

	// mountPath is the path inside the pod where hostPath will be mounted.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	MountPath string `json:"mountPath,omitempty"`

	// readOnly controls write access to the volume
	// +optional
	ReadOnly *bool `json:"readOnly,omitempty"`

	// pathType is the type of the HostPath.
	// +optional
	PathType corev1.HostPathType `json:"pathType,omitempty"`
}

// BootstrapTokenString is a token of the format abcdef.abcdef0123456789 that is used
// for both validation of the practically of the API server from a joining node's point
// of view and as an authentication method for the node in the bootstrap phase of
// "kubeadm join". This token is and should be short-lived.
//
// +kubebuilder:validation:Type=string
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=23
type BootstrapTokenString struct {
	ID     string `json:"-"`
	Secret string `json:"-"`
}

// MarshalJSON implements the json.Marshaler interface.
func (bts BootstrapTokenString) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", bts.String())), nil
}

// UnmarshalJSON implements the json.Unmarshaller interface.
func (bts *BootstrapTokenString) UnmarshalJSON(b []byte) error {
	// If the token is represented as "", just return quickly without an error
	if len(b) == 0 {
		return nil
	}

	// Remove unnecessary " characters coming from the JSON parser
	token := strings.ReplaceAll(string(b), `"`, ``)
	// Convert the string Token to a BootstrapTokenString object
	newbts, err := NewBootstrapTokenString(token)
	if err != nil {
		return err
	}
	bts.ID = newbts.ID
	bts.Secret = newbts.Secret
	return nil
}

// String returns the string representation of the BootstrapTokenString.
func (bts BootstrapTokenString) String() string {
	if bts.ID != "" && bts.Secret != "" {
		return bootstraputil.TokenFromIDAndSecret(bts.ID, bts.Secret)
	}
	return ""
}

// NewBootstrapTokenString converts the given Bootstrap Token as a string
// to the BootstrapTokenString object used for serialization/deserialization
// and internal usage. It also automatically validates that the given token
// is of the right format.
func NewBootstrapTokenString(token string) (*BootstrapTokenString, error) {
	substrs := bootstraputil.BootstrapTokenRegexp.FindStringSubmatch(token)
	// TODO: Add a constant for the 3 value here, and explain better why it's needed (other than because how the regexp parsin works)
	if len(substrs) != 3 {
		return nil, errors.Errorf("the bootstrap token %q was not of the form %q", token, bootstrapapi.BootstrapTokenPattern)
	}

	return &BootstrapTokenString{ID: substrs[1], Secret: substrs[2]}, nil
}

// Patches contains options related to applying patches to components deployed by kubeadm.
// +kubebuilder:validation:MinProperties=1
type Patches struct {
	// directory is a path to a directory that contains files named "target[suffix][+patchtype].extension".
	// For example, "kube-apiserver0+merge.yaml" or just "etcd.json". "target" can be one of
	// "kube-apiserver", "kube-controller-manager", "kube-scheduler", "etcd". "patchtype" can be one
	// of "strategic" "merge" or "json" and they match the patch formats supported by kubectl.
	// The default "patchtype" is "strategic". "extension" must be either "json" or "yaml".
	// "suffix" is an optional string that can be used to determine which patches are applied
	// first alpha-numerically.
	// These files can be written into the target directory via KubeadmConfig.Files which
	// specifies additional files to be created on the machine, either with content inline or
	// by referencing a secret.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	Directory string `json:"directory,omitempty"`
}

// Arg represents an argument with a name and a value.
type Arg struct {
	// name is the Name of the extraArg.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name,omitempty"`

	// value is the Value of the extraArg.
	// +required
	// +kubebuilder:validation:MinLength=0
	// +kubebuilder:validation:MaxLength=1024
	Value *string `json:"value,omitempty"`
}

// EnvVar represents an environment variable present in a Container.
type EnvVar struct {
	corev1.EnvVar `json:",inline"`
}

// Timeouts holds various timeouts that apply to kubeadm commands.
// +kubebuilder:validation:MinProperties=1
type Timeouts struct {
	// controlPlaneComponentHealthCheckSeconds is the amount of time to wait for a control plane
	// component, such as the API server, to be healthy during "kubeadm init" and "kubeadm join".
	// If not set, it defaults to 4m (240s).
	// +kubebuilder:validation:Minimum=0
	// +optional
	ControlPlaneComponentHealthCheckSeconds *int32 `json:"controlPlaneComponentHealthCheckSeconds,omitempty"`

	// kubeletHealthCheckSeconds is the amount of time to wait for the kubelet to be healthy
	// during "kubeadm init" and "kubeadm join".
	// If not set, it defaults to 4m (240s).
	// +kubebuilder:validation:Minimum=0
	// +optional
	KubeletHealthCheckSeconds *int32 `json:"kubeletHealthCheckSeconds,omitempty"`

	// kubernetesAPICallSeconds is the amount of time to wait for the kubeadm client to complete a request to
	// the API server. This applies to all types of methods (GET, POST, etc).
	// If not set, it defaults to 1m (60s).
	// +kubebuilder:validation:Minimum=0
	// +optional
	KubernetesAPICallSeconds *int32 `json:"kubernetesAPICallSeconds,omitempty"`

	// etcdAPICallSeconds is the amount of time to wait for the kubeadm etcd client to complete a request to
	// the etcd cluster.
	// If not set, it defaults to 2m (120s).
	// +kubebuilder:validation:Minimum=0
	// +optional
	EtcdAPICallSeconds *int32 `json:"etcdAPICallSeconds,omitempty"`

	// tlsBootstrapSeconds is the amount of time to wait for the kubelet to complete TLS bootstrap
	// for a joining node.
	// If not set, it defaults to 5m (300s).
	// +kubebuilder:validation:Minimum=0
	// +optional
	TLSBootstrapSeconds *int32 `json:"tlsBootstrapSeconds,omitempty"`

	// discoverySeconds is the amount of time to wait for kubeadm to validate the API server identity
	// for a joining node.
	// If not set, it defaults to 5m (300s).
	// +kubebuilder:validation:Minimum=0
	// +optional
	DiscoverySeconds *int32 `json:"discoverySeconds,omitempty"`
}
