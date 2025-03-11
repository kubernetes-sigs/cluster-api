/*
Copyright 2021 The Kubernetes Authors.

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

package v1beta1

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// InitConfiguration contains a list of elements that is specific "kubeadm init"-only runtime
// information.
type InitConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// bootstrapTokens is respected at `kubeadm init` time and describes a set of Bootstrap Tokens to create.
	// This information IS NOT uploaded to the kubeadm cluster configmap, partly because of its sensitive nature
	// +optional
	// +kubebuilder:validation:MaxItems=100
	BootstrapTokens []BootstrapToken `json:"bootstrapTokens,omitempty"`

	// nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.
	// When used in the context of control plane nodes, NodeRegistration should remain consistent
	// across both InitConfiguration and JoinConfiguration
	// +optional
	NodeRegistration NodeRegistrationOptions `json:"nodeRegistration,omitempty"`

	// localAPIEndpoint represents the endpoint of the API server instance that's deployed on this control plane node
	// In HA setups, this differs from ClusterConfiguration.ControlPlaneEndpoint in the sense that ControlPlaneEndpoint
	// is the global endpoint for the cluster, which then loadbalances the requests to each individual API server. This
	// configuration object lets you customize what IP/DNS name and port the local API server advertises it's accessible
	// on. By default, kubeadm tries to auto-detect the IP of the default interface and use that, but in case that process
	// fails you may set the desired value here.
	// +optional
	LocalAPIEndpoint APIEndpoint `json:"localAPIEndpoint,omitempty"`

	// skipPhases is a list of phases to skip during command execution.
	// The list of phases can be obtained with the "kubeadm init --help" command.
	// This option takes effect only on Kubernetes >=1.22.0.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MaxLength=256
	SkipPhases []string `json:"skipPhases,omitempty"`

	// patches contains options related to applying patches to components deployed by kubeadm during
	// "kubeadm init". The minimum kubernetes version needed to support Patches is v1.22
	// +optional
	Patches *Patches `json:"patches,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterConfiguration contains cluster-wide configuration for a kubeadm cluster.
type ClusterConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// etcd holds configuration for etcd.
	// NB: This value defaults to a Local (stacked) etcd
	// +optional
	Etcd Etcd `json:"etcd,omitempty"`

	// networking holds configuration for the networking topology of the cluster.
	// NB: This value defaults to the Cluster object spec.clusterNetwork.
	// +optional
	Networking Networking `json:"networking,omitempty"`

	// kubernetesVersion is the target version of the control plane.
	// NB: This value defaults to the Machine object spec.version
	// +optional
	// +kubebuilder:validation:MaxLength=256
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

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
	// +kubebuilder:validation:MaxLength=512
	ControlPlaneEndpoint string `json:"controlPlaneEndpoint,omitempty"`

	// apiServer contains extra settings for the API server control plane component
	// +optional
	APIServer APIServer `json:"apiServer,omitempty"`

	// controllerManager contains extra settings for the controller manager control plane component
	// +optional
	ControllerManager ControlPlaneComponent `json:"controllerManager,omitempty"`

	// scheduler contains extra settings for the scheduler control plane component
	// +optional
	Scheduler ControlPlaneComponent `json:"scheduler,omitempty"`

	// dns defines the options for the DNS add-on installed in the cluster.
	// +optional
	DNS DNS `json:"dns,omitempty"`

	// certificatesDir specifies where to store or look for all required certificates.
	// NB: if not provided, this will default to `/etc/kubernetes/pki`
	// +optional
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
	// +kubebuilder:validation:MaxLength=512
	ImageRepository string `json:"imageRepository,omitempty"`

	// featureGates enabled by the user.
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`

	// clusterName is the cluster name
	// +optional
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`
}

// ControlPlaneComponent holds settings common to control plane component of the cluster.
type ControlPlaneComponent struct {
	// extraArgs is an extra set of flags to pass to the control plane component.
	// TODO: This is temporary and ideally we would like to switch all components to use ComponentConfig + ConfigMaps.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// extraVolumes is an extra set of host volumes, mounted to the control plane component.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	ExtraVolumes []HostPathMount `json:"extraVolumes,omitempty"`

	// extraEnvs is an extra set of environment variables to pass to the control plane component.
	// Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.
	// This option takes effect only on Kubernetes >=1.31.0.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	ExtraEnvs []EnvVar `json:"extraEnvs,omitempty"`
}

// APIServer holds settings necessary for API server deployments in the cluster.
type APIServer struct {
	ControlPlaneComponent `json:",inline"`

	// certSANs sets extra Subject Alternative Names for the API Server signing cert.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MaxLength=253
	CertSANs []string `json:"certSANs,omitempty"`

	// timeoutForControlPlane controls the timeout that we use for API server to appear
	// +optional
	TimeoutForControlPlane *metav1.Duration `json:"timeoutForControlPlane,omitempty"`
}

// DNS defines the DNS addon that should be used in the cluster.
type DNS struct {
	// ImageMeta allows to customize the image used for the DNS component
	ImageMeta `json:",inline"`
}

// ImageMeta allows to customize the image used for components that are not
// originated from the Kubernetes/Kubernetes release process.
type ImageMeta struct {
	// imageRepository sets the container registry to pull images from.
	// if not set, the ImageRepository defined in ClusterConfiguration will be used instead.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	ImageRepository string `json:"imageRepository,omitempty"`

	// imageTag allows to specify a tag for the image.
	// In case this value is set, kubeadm does not change automatically the version of the above components during upgrades.
	// +optional
	// +kubebuilder:validation:MaxLength=256
	ImageTag string `json:"imageTag,omitempty"`

	// TODO: evaluate if we need also a ImageName based on user feedbacks
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterStatus contains the cluster status. The ClusterStatus will be stored in the kubeadm-config
// ConfigMap in the cluster, and then updated by kubeadm when additional control plane instance joins or leaves the cluster.
//
// Deprecated: ClusterStatus has been removed from kubeadm v1beta3 API; This type is preserved only to support
// conversion to older versions of the kubeadm API.
type ClusterStatus struct {
	metav1.TypeMeta `json:",inline"`

	// apiEndpoints currently available in the cluster, one for each control plane/api server instance.
	// The key of the map is the IP of the host's default interface
	// +optional
	APIEndpoints map[string]APIEndpoint `json:"apiEndpoints"`
}

// APIEndpoint struct contains elements of API server instance deployed on a node.
type APIEndpoint struct {
	// advertiseAddress sets the IP address for the API server to advertise.
	// +optional
	// +kubebuilder:validation:MaxLength=39
	AdvertiseAddress string `json:"advertiseAddress,omitempty"`

	// bindPort sets the secure port for the API Server to bind to.
	// Defaults to 6443.
	// +optional
	BindPort int32 `json:"bindPort,omitempty"`
}

// NodeRegistrationOptions holds fields that relate to registering a new control-plane or node to the cluster, either via "kubeadm init" or "kubeadm join".
// Note: The NodeRegistrationOptions struct has to be kept in sync with the structs in MarshalJSON.
type NodeRegistrationOptions struct {

	// name is the `.Metadata.Name` field of the Node API object that will be created in this `kubeadm init` or `kubeadm join` operation.
	// This field is also used in the CommonName field of the kubelet's client certificate to the API server.
	// Defaults to the hostname of the node if not provided.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name,omitempty"`

	// criSocket is used to retrieve container runtime info. This information will be annotated to the Node API object, for later re-use
	// +optional
	// +kubebuilder:validation:MaxLength=512
	CRISocket string `json:"criSocket,omitempty"`

	// taints specifies the taints the Node API object should be registered with. If this field is unset, i.e. nil, in the `kubeadm init` process
	// it will be defaulted to []v1.Taint{'node-role.kubernetes.io/master=""'}. If you don't want to taint your control-plane node, set this field to an
	// empty slice, i.e. `taints: []` in the YAML file. This field is solely used for Node registration.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Taints []corev1.Taint `json:"taints,omitempty"`

	// kubeletExtraArgs passes through extra arguments to the kubelet. The arguments here are passed to the kubelet command line via the environment file
	// kubeadm writes at runtime for the kubelet to source. This overrides the generic base-level configuration in the kubelet-config-1.X ConfigMap
	// Flags have higher priority when parsing. These values are local and specific to the node kubeadm is executing on.
	// +optional
	KubeletExtraArgs map[string]string `json:"kubeletExtraArgs,omitempty"`

	// ignorePreflightErrors provides a slice of pre-flight errors to be ignored when the current node is registered.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MaxLength=512
	IgnorePreflightErrors []string `json:"ignorePreflightErrors,omitempty"`

	// imagePullPolicy specifies the policy for image pulling
	// during kubeadm "init" and "join" operations. The value of
	// this field must be one of "Always", "IfNotPresent" or
	// "Never". Defaults to "IfNotPresent". This can be used only
	// with Kubernetes version equal to 1.22 and later.
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	// +optional
	ImagePullPolicy string `json:"imagePullPolicy,omitempty"`

	// imagePullSerial specifies if image pulling performed by kubeadm must be done serially or in parallel.
	// This option takes effect only on Kubernetes >=1.31.0.
	// Default: true (defaulted in kubeadm)
	// +optional
	ImagePullSerial *bool `json:"imagePullSerial,omitempty"`
}

// MarshalJSON marshals NodeRegistrationOptions in a way that an empty slice in Taints is preserved.
// Taints are then rendered as:
// * nil => omitted from the marshalled JSON
// * [] => rendered as empty array (`[]`)
// * [regular-array] => rendered as usual
// We have to do this as the regular Golang JSON marshalling would just omit
// the empty slice (xref: https://github.com/golang/go/issues/22480).
// Note: We can't re-use the original struct as that would lead to an infinite recursion.
// Note: The structs in this func have to be kept in sync with the NodeRegistrationOptions struct.
func (n *NodeRegistrationOptions) MarshalJSON() ([]byte, error) {
	// Marshal an empty Taints slice array without omitempty so it's preserved.
	if n.Taints != nil && len(n.Taints) == 0 {
		return json.Marshal(struct {
			Name                  string            `json:"name,omitempty"`
			CRISocket             string            `json:"criSocket,omitempty"`
			Taints                []corev1.Taint    `json:"taints"`
			KubeletExtraArgs      map[string]string `json:"kubeletExtraArgs,omitempty"`
			IgnorePreflightErrors []string          `json:"ignorePreflightErrors,omitempty"`
			ImagePullPolicy       string            `json:"imagePullPolicy,omitempty"`
			ImagePullSerial       *bool             `json:"imagePullSerial,omitempty"`
		}{
			Name:                  n.Name,
			CRISocket:             n.CRISocket,
			Taints:                n.Taints,
			KubeletExtraArgs:      n.KubeletExtraArgs,
			IgnorePreflightErrors: n.IgnorePreflightErrors,
			ImagePullPolicy:       n.ImagePullPolicy,
			ImagePullSerial:       n.ImagePullSerial,
		})
	}

	// If Taints is nil or not empty we can use omitempty.
	return json.Marshal(struct {
		Name                  string            `json:"name,omitempty"`
		CRISocket             string            `json:"criSocket,omitempty"`
		Taints                []corev1.Taint    `json:"taints,omitempty"`
		KubeletExtraArgs      map[string]string `json:"kubeletExtraArgs,omitempty"`
		IgnorePreflightErrors []string          `json:"ignorePreflightErrors,omitempty"`
		ImagePullPolicy       string            `json:"imagePullPolicy,omitempty"`
		ImagePullSerial       *bool             `json:"imagePullSerial,omitempty"`
	}{
		Name:                  n.Name,
		CRISocket:             n.CRISocket,
		Taints:                n.Taints,
		KubeletExtraArgs:      n.KubeletExtraArgs,
		IgnorePreflightErrors: n.IgnorePreflightErrors,
		ImagePullPolicy:       n.ImagePullPolicy,
		ImagePullSerial:       n.ImagePullSerial,
	})
}

// Networking contains elements describing cluster's networking configuration.
type Networking struct {
	// serviceSubnet is the subnet used by k8s services.
	// Defaults to a comma-delimited string of the Cluster object's spec.clusterNetwork.pods.cidrBlocks, or
	// to "10.96.0.0/12" if that's unset.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	ServiceSubnet string `json:"serviceSubnet,omitempty"`
	// podSubnet is the subnet used by pods.
	// If unset, the API server will not allocate CIDR ranges for every node.
	// Defaults to a comma-delimited string of the Cluster object's spec.clusterNetwork.services.cidrBlocks if that is set
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	PodSubnet string `json:"podSubnet,omitempty"`
	// dnsDomain is the dns domain used by k8s services. Defaults to "cluster.local".
	// +optional
	// +kubebuilder:validation:MaxLength=253
	DNSDomain string `json:"dnsDomain,omitempty"`
}

// BootstrapToken describes one bootstrap token, stored as a Secret in the cluster.
type BootstrapToken struct {
	// token is used for establishing bidirectional trust between nodes and control-planes.
	// Used for joining nodes in the cluster.
	// +required
	Token *BootstrapTokenString `json:"token"`
	// description sets a human-friendly message why this token exists and what it's used
	// for, so other administrators can know its purpose.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Description string `json:"description,omitempty"`
	// ttl defines the time to live for this token. Defaults to 24h.
	// Expires and TTL are mutually exclusive.
	// +optional
	TTL *metav1.Duration `json:"ttl,omitempty"`
	// expires specifies the timestamp when this token expires. Defaults to being set
	// dynamically at runtime based on the TTL. Expires and TTL are mutually exclusive.
	// +optional
	Expires *metav1.Time `json:"expires,omitempty"`
	// usages describes the ways in which this token can be used. Can by default be used
	// for establishing bidirectional trust, but that can be changed here.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MaxLength=256
	Usages []string `json:"usages,omitempty"`
	// groups specifies the extra groups that this token will authenticate as when/if
	// used for authentication
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MaxLength=256
	Groups []string `json:"groups,omitempty"`
}

// Etcd contains elements describing Etcd configuration.
type Etcd struct {

	// local provides configuration knobs for configuring the local etcd instance
	// Local and External are mutually exclusive
	// +optional
	Local *LocalEtcd `json:"local,omitempty"`

	// external describes how to connect to an external etcd cluster
	// Local and External are mutually exclusive
	// +optional
	External *ExternalEtcd `json:"external,omitempty"`
}

// LocalEtcd describes that kubeadm should run an etcd cluster locally.
type LocalEtcd struct {
	// ImageMeta allows to customize the container used for etcd
	ImageMeta `json:",inline"`

	// dataDir is the directory etcd will place its data.
	// Defaults to "/var/lib/etcd".
	// +optional
	// +kubebuilder:validation:MaxLength=512
	DataDir string `json:"dataDir,omitempty"`

	// extraArgs are extra arguments provided to the etcd binary
	// when run inside a static pod.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// extraEnvs is an extra set of environment variables to pass to the control plane component.
	// Environment variables passed using ExtraEnvs will override any existing environment variables, or *_proxy environment variables that kubeadm adds by default.
	// This option takes effect only on Kubernetes >=1.31.0.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	ExtraEnvs []EnvVar `json:"extraEnvs,omitempty"`

	// serverCertSANs sets extra Subject Alternative Names for the etcd server signing cert.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MaxLength=253
	ServerCertSANs []string `json:"serverCertSANs,omitempty"`

	// peerCertSANs sets extra Subject Alternative Names for the etcd peer signing cert.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MaxLength=253
	PeerCertSANs []string `json:"peerCertSANs,omitempty"`
}

// ExternalEtcd describes an external etcd cluster.
// Kubeadm has no knowledge of where certificate files live and they must be supplied.
type ExternalEtcd struct {
	// endpoints of etcd members. Required for ExternalEtcd.
	// +kubebuilder:validation:MaxItems=50
	// +required
	// +kubebuilder:validation:items:MaxLength=256
	Endpoints []string `json:"endpoints"`

	// caFile is an SSL Certificate Authority file used to secure etcd communication.
	// Required if using a TLS connection.
	// +required
	// +kubebuilder:validation:MaxLength=512
	CAFile string `json:"caFile"`

	// certFile is an SSL certification file used to secure etcd communication.
	// Required if using a TLS connection.
	// +required
	// +kubebuilder:validation:MaxLength=512
	CertFile string `json:"certFile"`

	// keyFile is an SSL key file used to secure etcd communication.
	// Required if using a TLS connection.
	// +required
	// +kubebuilder:validation:MaxLength=512
	KeyFile string `json:"keyFile"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// JoinConfiguration contains elements describing a particular node.
type JoinConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// nodeRegistration holds fields that relate to registering the new control-plane node to the cluster.
	// When used in the context of control plane nodes, NodeRegistration should remain consistent
	// across both InitConfiguration and JoinConfiguration
	// +optional
	NodeRegistration NodeRegistrationOptions `json:"nodeRegistration,omitempty"`

	// caCertPath is the path to the SSL certificate authority used to
	// secure comunications between node and control-plane.
	// Defaults to "/etc/kubernetes/pki/ca.crt".
	// +optional
	// TODO: revisit when there is defaulting from k/k
	// +kubebuilder:validation:MaxLength=512
	CACertPath string `json:"caCertPath,omitempty"`

	// discovery specifies the options for the kubelet to use during the TLS Bootstrap process
	// +optional
	// TODO: revisit when there is defaulting from k/k
	Discovery Discovery `json:"discovery,omitempty"`

	// controlPlane defines the additional control plane instance to be deployed on the joining node.
	// If nil, no additional control plane instance will be deployed.
	// +optional
	ControlPlane *JoinControlPlane `json:"controlPlane,omitempty"`

	// skipPhases is a list of phases to skip during command execution.
	// The list of phases can be obtained with the "kubeadm init --help" command.
	// This option takes effect only on Kubernetes >=1.22.0.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:items:MaxLength=256
	SkipPhases []string `json:"skipPhases,omitempty"`

	// patches contains options related to applying patches to components deployed by kubeadm during
	// "kubeadm join". The minimum kubernetes version needed to support Patches is v1.22
	// +optional
	Patches *Patches `json:"patches,omitempty"`
}

// JoinControlPlane contains elements describing an additional control plane instance to be deployed on the joining node.
type JoinControlPlane struct {
	// localAPIEndpoint represents the endpoint of the API server instance to be deployed on this node.
	// +optional
	LocalAPIEndpoint APIEndpoint `json:"localAPIEndpoint,omitempty"`
}

// Discovery specifies the options for the kubelet to use during the TLS Bootstrap process.
type Discovery struct {
	// bootstrapToken is used to set the options for bootstrap token based discovery
	// BootstrapToken and File are mutually exclusive
	// +optional
	BootstrapToken *BootstrapTokenDiscovery `json:"bootstrapToken,omitempty"`

	// file is used to specify a file or URL to a kubeconfig file from which to load cluster information
	// BootstrapToken and File are mutually exclusive
	// +optional
	File *FileDiscovery `json:"file,omitempty"`

	// tlsBootstrapToken is a token used for TLS bootstrapping.
	// If .BootstrapToken is set, this field is defaulted to .BootstrapToken.Token, but can be overridden.
	// If .File is set, this field **must be set** in case the KubeConfigFile does not contain any other authentication information
	// +optional
	// +kubebuilder:validation:MaxLength=512
	TLSBootstrapToken string `json:"tlsBootstrapToken,omitempty"`

	// timeout modifies the discovery timeout
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// BootstrapTokenDiscovery is used to set the options for bootstrap token based discovery.
type BootstrapTokenDiscovery struct {
	// token is a token used to validate cluster information
	// fetched from the control-plane.
	// +required
	// +kubebuilder:validation:MaxLength=512
	Token string `json:"token"`

	// apiServerEndpoint is an IP or domain name to the API server from which info will be fetched.
	// +optional
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
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MaxLength=512
	CACertHashes []string `json:"caCertHashes,omitempty"`

	// unsafeSkipCAVerification allows token-based discovery
	// without CA verification via CACertHashes. This can weaken
	// the security of kubeadm since other nodes can impersonate the control-plane.
	// +optional
	UnsafeSkipCAVerification bool `json:"unsafeSkipCAVerification,omitempty"`
}

// FileDiscovery is used to specify a file or URL to a kubeconfig file from which to load cluster information.
type FileDiscovery struct {
	// kubeConfigPath is used to specify the actual file path or URL to the kubeconfig file from which to load cluster information
	// +required
	// +kubebuilder:validation:MaxLength=512
	KubeConfigPath string `json:"kubeConfigPath"`

	// kubeConfig is used (optionally) to generate a KubeConfig based on the KubeadmConfig's information.
	// The file is generated at the path specified in KubeConfigPath.
	//
	// Host address (server field) information is automatically populated based on the Cluster's ControlPlaneEndpoint.
	// Certificate Authority (certificate-authority-data field) is gathered from the cluster's CA secret.
	//
	// +optional
	KubeConfig *FileDiscoveryKubeConfig `json:"kubeConfig,omitempty"`
}

// FileDiscoveryKubeConfig contains elements describing how to generate the kubeconfig for bootstrapping.
type FileDiscoveryKubeConfig struct {
	// cluster contains information about how to communicate with the kubernetes cluster.
	//
	// By default the following fields are automatically populated:
	// - Server with the Cluster's ControlPlaneEndpoint.
	// - CertificateAuthorityData with the Cluster's CA certificate.
	// +optional
	Cluster *KubeConfigCluster `json:"cluster,omitempty"`

	// user contains information that describes identity information.
	// This is used to tell the kubernetes cluster who you are.
	// +required
	User KubeConfigUser `json:"user"`
}

// KubeConfigCluster contains information about how to communicate with a kubernetes cluster.
//
// Adapted from clientcmdv1.Cluster.
type KubeConfigCluster struct {
	// server is the address of the kubernetes cluster (https://hostname:port).
	//
	// Defaults to https:// + Cluster.Spec.ControlPlaneEndpoint.
	//
	// +optional
	// +kubebuilder:validation:MaxLength=512
	Server string `json:"server,omitempty"`

	// tlsServerName is used to check server certificate. If TLSServerName is empty, the hostname used to contact the server is used.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	TLSServerName string `json:"tlsServerName,omitempty"`

	// insecureSkipTLSVerify skips the validity check for the server's certificate. This will make your HTTPS connections insecure.
	// +optional
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`

	// certificateAuthorityData contains PEM-encoded certificate authority certificates.
	//
	// Defaults to the Cluster's CA certificate if empty.
	//
	// +optional
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
	// +kubebuilder:validation:MaxLength=512
	ProxyURL string `json:"proxyURL,omitempty"`
}

// KubeConfigUser contains information that describes identity information.
// This is used to tell the kubernetes cluster who you are.
//
// Either authProvider or exec must be filled.
//
// Adapted from clientcmdv1.AuthInfo.
type KubeConfigUser struct {
	// authProvider specifies a custom authentication plugin for the kubernetes cluster.
	// +optional
	AuthProvider *KubeConfigAuthProvider `json:"authProvider,omitempty"`

	// exec specifies a custom exec-based authentication plugin for the kubernetes cluster.
	// +optional
	Exec *KubeConfigAuthExec `json:"exec,omitempty"`
}

// KubeConfigAuthProvider holds the configuration for a specified auth provider.
type KubeConfigAuthProvider struct {
	// name is the name of the authentication plugin.
	// +required
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`

	// config holds the parameters for the authentication plugin.
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// KubeConfigAuthExec specifies a command to provide client credentials. The command is exec'd
// and outputs structured stdout holding credentials.
//
// See the client.authentication.k8s.io API group for specifications of the exact input
// and output format.
type KubeConfigAuthExec struct {
	// command to execute.
	// +required
	// +kubebuilder:validation:MaxLength=1024
	Command string `json:"command"`

	// args is the arguments to pass to the command when executing it.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MaxLength=512
	Args []string `json:"args,omitempty"`

	// env defines additional environment variables to expose to the process. These
	// are unioned with the host's environment, as well as variables client-go uses
	// to pass argument to the plugin.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	Env []KubeConfigAuthExecEnv `json:"env,omitempty"`

	// apiVersion is preferred input version of the ExecInfo. The returned ExecCredentials MUST use
	// the same encoding version as the input.
	// Defaults to client.authentication.k8s.io/v1 if not set.
	// +optional
	// +kubebuilder:validation:MaxLength=512
	APIVersion string `json:"apiVersion,omitempty"`

	// provideClusterInfo determines whether or not to provide cluster information,
	// which could potentially contain very large CA data, to this exec plugin as a
	// part of the KUBERNETES_EXEC_INFO environment variable. By default, it is set
	// to false. Package k8s.io/client-go/tools/auth/exec provides helper methods for
	// reading this environment variable.
	// +optional
	ProvideClusterInfo bool `json:"provideClusterInfo,omitempty"`
}

// KubeConfigAuthExecEnv is used for setting environment variables when executing an exec-based
// credential plugin.
type KubeConfigAuthExecEnv struct {
	// name of the environment variable
	// +required
	// +kubebuilder:validation:MaxLength=512
	Name string `json:"name"`
	// value of the environment variable
	// +required
	// +kubebuilder:validation:MaxLength=512
	Value string `json:"value"`
}

// HostPathMount contains elements describing volumes that are mounted from the
// host.
type HostPathMount struct {
	// name of the volume inside the pod template.
	// +required
	// +kubebuilder:validation:MaxLength=512
	Name string `json:"name"`
	// hostPath is the path in the host that will be mounted inside
	// the pod.
	// +required
	// +kubebuilder:validation:MaxLength=512
	HostPath string `json:"hostPath"`
	// mountPath is the path inside the pod where hostPath will be mounted.
	// +required
	// +kubebuilder:validation:MaxLength=512
	MountPath string `json:"mountPath"`
	// readOnly controls write access to the volume
	// +optional
	ReadOnly bool `json:"readOnly,omitempty"`
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
	// +kubebuilder:validation:MaxLength=512
	Directory string `json:"directory,omitempty"`
}

// EnvVar represents an environment variable present in a Container.
type EnvVar struct {
	corev1.EnvVar `json:",inline"`
}
