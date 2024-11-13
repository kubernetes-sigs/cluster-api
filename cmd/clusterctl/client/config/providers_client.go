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

package config

import (
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/drone/envsubst/v2"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

// core providers.
const (
	// ClusterAPIProviderName is the name for the core provider.
	ClusterAPIProviderName = "cluster-api"
)

// Infra providers.
const (
	AWSProviderName            = "aws"
	AzureProviderName          = "azure"
	BYOHProviderName           = "byoh"
	CloudStackProviderName     = "cloudstack"
	DockerProviderName         = "docker"
	DOProviderName             = "digitalocean"
	GCPProviderName            = "gcp"
	HetznerProviderName        = "hetzner"
	HivelocityProviderName     = "hivelocity-hivelocity"
	OutscaleProviderName       = "outscale"
	IBMCloudProviderName       = "ibmcloud"
	InMemoryProviderName       = "in-memory"
	LinodeProviderName         = "linode-linode"
	Metal3ProviderName         = "metal3"
	NestedProviderName         = "nested"
	NutanixProviderName        = "nutanix"
	OCIProviderName            = "oci"
	OpenStackProviderName      = "openstack"
	PacketProviderName         = "packet"
	TinkerbellProviderName     = "tinkerbell-tinkerbell"
	SideroProviderName         = "sidero"
	VCloudDirectorProviderName = "vcd"
	VSphereProviderName        = "vsphere"
	MAASProviderName           = "maas"
	KubevirtProviderName       = "kubevirt"
	KubeKeyProviderName        = "kubekey"
	VclusterProviderName       = "vcluster"
	VirtinkProviderName        = "virtink"
	CoxEdgeProviderName        = "coxedge"
	ProxmoxProviderName        = "proxmox"
	K0smotronProviderName      = "k0sproject-k0smotron"
	IonosCloudProviderName     = "ionoscloud-ionoscloud"
	VultrProviderName          = "vultr-vultr"
)

// Bootstrap providers.
const (
	KubeadmBootstrapProviderName           = "kubeadm"
	TalosBootstrapProviderName             = "talos"
	MicroK8sBootstrapProviderName          = "microk8s"
	OracleCloudNativeBootstrapProviderName = "ocne"
	KubeKeyK3sBootstrapProviderName        = "kubekey-k3s"
	RKE2BootstrapProviderName              = "rke2"
	K0smotronBootstrapProviderName         = "k0sproject-k0smotron"
)

// ControlPlane providers.
const (
	KubeadmControlPlaneProviderName           = "kubeadm"
	TalosControlPlaneProviderName             = "talos"
	MicroK8sControlPlaneProviderName          = "microk8s"
	NestedControlPlaneProviderName            = "nested"
	OracleCloudNativeControlPlaneProviderName = "ocne"
	KubeKeyK3sControlPlaneProviderName        = "kubekey-k3s"
	KamajiControlPlaneProviderName            = "kamaji"
	RKE2ControlPlaneProviderName              = "rke2"
	K0smotronControlPlaneProviderName         = "k0sproject-k0smotron"
)

// IPAM providers.
const (
	InClusterIPAMProviderName = "in-cluster"
	NutanixIPAMProviderName   = "nutanix"
)

// Add-on providers.
const (
	HelmAddonProviderName = "helm"
)

// Runtime extensions providers.
const (
	NutanixRuntimeExtensionsProviderName = "nutanix"
)

// Other.
const (
	// ProvidersConfigKey is a constant for finding provider configurations with the ProvidersClient.
	ProvidersConfigKey = "providers"
)

// ProvidersClient has methods to work with provider configurations.
type ProvidersClient interface {
	// List returns all the provider configurations, including provider configurations hard-coded in clusterctl
	// and user-defined provider configurations read from the clusterctl configuration file.
	// In case of conflict, user-defined provider override the hard-coded configurations.
	List() ([]Provider, error)

	// Get returns the configuration for the provider with a given name/type.
	// In case the name/type does not correspond to any existing provider, an error is returned.
	Get(name string, providerType clusterctlv1.ProviderType) (Provider, error)
}

// providersClient implements ProvidersClient.
type providersClient struct {
	reader Reader
}

// ensure providersClient implements ProvidersClient.
var _ ProvidersClient = &providersClient{}

func newProvidersClient(reader Reader) *providersClient {
	return &providersClient{
		reader: reader,
	}
}

func (p *providersClient) defaults() []Provider {
	// clusterctl includes a predefined list of Cluster API providers sponsored by SIG-cluster-lifecycle to provide users the simplest
	// out-of-box experience. This is an opt-in feature; other providers can be added by using the clusterctl configuration file.

	// if you are a developer of a SIG-cluster-lifecycle project, you can send a PR to extend the following list.

	defaults := []Provider{
		// cluster API core provider
		&provider{
			name:         ClusterAPIProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/core-components.yaml",
			providerType: clusterctlv1.CoreProviderType,
		},

		// Infrastructure providers
		&provider{
			name:         LinodeProviderName,
			url:          "https://github.com/linode/cluster-api-provider-linode/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         AWSProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         AzureProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-azure/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			// NB. The Docker provider is not designed for production use and is intended for development environments only.
			name:         DockerProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/infrastructure-components-development.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         CloudStackProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-cloudstack/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         DOProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-digitalocean/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         GCPProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-gcp/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         PacketProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-packet/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         TinkerbellProviderName,
			url:          "https://github.com/tinkerbell/cluster-api-provider-tinkerbell/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         Metal3ProviderName,
			url:          "https://github.com/metal3-io/cluster-api-provider-metal3/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         NestedProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-nested/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         OCIProviderName,
			url:          "https://github.com/oracle/cluster-api-provider-oci/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         OpenStackProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-openstack/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         SideroProviderName,
			url:          "https://github.com/siderolabs/sidero/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         VCloudDirectorProviderName,
			url:          "https://github.com/vmware/cluster-api-provider-cloud-director/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         VSphereProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         MAASProviderName,
			url:          "https://github.com/spectrocloud/cluster-api-provider-maas/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         CoxEdgeProviderName,
			url:          "https://github.com/coxedge/cluster-api-provider-coxedge/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         BYOHProviderName,
			url:          "https://github.com/vmware-tanzu/cluster-api-provider-bringyourownhost/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         HetznerProviderName,
			url:          "https://github.com/syself/cluster-api-provider-hetzner/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         HivelocityProviderName,
			url:          "https://github.com/hivelocity/cluster-api-provider-hivelocity/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         OutscaleProviderName,
			url:          "https://github.com/outscale/cluster-api-provider-outscale/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         IBMCloudProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         InMemoryProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/infrastructure-components-in-memory-development.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         NutanixProviderName,
			url:          "https://github.com/nutanix-cloud-native/cluster-api-provider-nutanix/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         KubeKeyProviderName,
			url:          "https://github.com/kubesphere/kubekey/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         KubevirtProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-kubevirt/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         VclusterProviderName,
			url:          "https://github.com/loft-sh/cluster-api-provider-vcluster/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         VirtinkProviderName,
			url:          "https://github.com/smartxworks/cluster-api-provider-virtink/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         ProxmoxProviderName,
			url:          "https://github.com/ionos-cloud/cluster-api-provider-proxmox/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         K0smotronProviderName,
			url:          "https://github.com/k0sproject/k0smotron/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         IonosCloudProviderName,
			url:          "https://github.com/ionos-cloud/cluster-api-provider-ionoscloud/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         VultrProviderName,
			url:          "https://github.com/vultr/cluster-api-provider-vultr/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},

		// Bootstrap providers
		&provider{
			name:         KubeadmBootstrapProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         KubeKeyK3sBootstrapProviderName,
			url:          "https://github.com/kubesphere/kubekey/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         TalosBootstrapProviderName,
			url:          "https://github.com/siderolabs/cluster-api-bootstrap-provider-talos/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         MicroK8sBootstrapProviderName,
			url:          "https://github.com/canonical/cluster-api-bootstrap-provider-microk8s/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         OracleCloudNativeBootstrapProviderName,
			url:          "https://github.com/verrazzano/cluster-api-provider-ocne/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         RKE2BootstrapProviderName,
			url:          "https://github.com/rancher/cluster-api-provider-rke2/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         K0smotronBootstrapProviderName,
			url:          "https://github.com/k0sproject/k0smotron/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},

		// ControlPlane providers
		&provider{
			name:         KubeadmControlPlaneProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         KubeKeyK3sControlPlaneProviderName,
			url:          "https://github.com/kubesphere/kubekey/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         TalosControlPlaneProviderName,
			url:          "https://github.com/siderolabs/cluster-api-control-plane-provider-talos/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         MicroK8sControlPlaneProviderName,
			url:          "https://github.com/canonical/cluster-api-control-plane-provider-microk8s/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         NestedControlPlaneProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-nested/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         OracleCloudNativeControlPlaneProviderName,
			url:          "https://github.com/verrazzano/cluster-api-provider-ocne/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         KamajiControlPlaneProviderName,
			url:          "https://github.com/clastix/cluster-api-control-plane-provider-kamaji/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         RKE2ControlPlaneProviderName,
			url:          "https://github.com/rancher/cluster-api-provider-rke2/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         K0smotronControlPlaneProviderName,
			url:          "https://github.com/k0sproject/k0smotron/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},

		// IPAM providers
		&provider{
			name:         InClusterIPAMProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-ipam-provider-in-cluster/releases/latest/ipam-components.yaml",
			providerType: clusterctlv1.IPAMProviderType,
		},
		&provider{
			name:         NutanixIPAMProviderName,
			url:          "https://github.com/nutanix-cloud-native/cluster-api-ipam-provider-nutanix/releases/latest/ipam-components.yaml",
			providerType: clusterctlv1.IPAMProviderType,
		},

		// Add-on providers
		&provider{
			name:         HelmAddonProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-addon-provider-helm/releases/latest/addon-components.yaml",
			providerType: clusterctlv1.AddonProviderType,
		},

		// Runtime extensions providers
		&provider{
			name:         NutanixRuntimeExtensionsProviderName,
			url:          "https://github.com/nutanix-cloud-native/cluster-api-runtime-extensions-nutanix/releases/latest/runtime-extensions-components.yaml",
			providerType: clusterctlv1.RuntimeExtensionProviderType,
		},
	}

	return defaults
}

// configProvider mirrors config.Provider interface and allows serialization of the corresponding info.
type configProvider struct {
	Name string                    `json:"name,omitempty"`
	URL  string                    `json:"url,omitempty"`
	Type clusterctlv1.ProviderType `json:"type,omitempty"`
}

func (p *providersClient) List() ([]Provider, error) {
	// Creates a maps with all the defaults provider configurations
	providers := p.defaults()

	// Gets user defined provider configurations, validate them, and merges with
	// hard-coded configurations handling conflicts (user defined take precedence on hard-coded)

	userDefinedProviders := []configProvider{}
	if err := p.reader.UnmarshalKey(ProvidersConfigKey, &userDefinedProviders); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal providers from the clusterctl configuration file")
	}

	for _, u := range userDefinedProviders {
		var err error
		u.URL, err = envsubst.Eval(u.URL, os.Getenv)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to evaluate url: %q", u.URL)
		}

		provider := NewProvider(u.Name, u.URL, u.Type)
		if err := validateProvider(provider); err != nil {
			return nil, errors.Wrapf(err, "error validating configuration for the %s with name %s. Please fix the providers value in clusterctl configuration file", provider.Type(), provider.Name())
		}

		override := false
		for i := range providers {
			if providers[i].SameAs(provider) {
				providers[i] = provider
				override = true
			}
		}

		if !override {
			providers = append(providers, provider)
		}
	}

	// ensure provider configurations are consistently sorted
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Less(providers[j])
	})

	return providers, nil
}

func (p *providersClient) Get(name string, providerType clusterctlv1.ProviderType) (Provider, error) {
	l, err := p.List()
	if err != nil {
		return nil, err
	}

	provider := NewProvider(name, "", providerType) // NB. Having the url empty is fine because the url is not considered by SameAs.
	for _, r := range l {
		if r.SameAs(provider) {
			return r, nil
		}
	}

	return nil, errors.Errorf("failed to get configuration for the %s with name %s. Please check the provider name and/or add configuration for new providers using the .clusterctl config file", providerType, name)
}

func validateProvider(r Provider) error {
	if r.Name() == "" {
		return errors.New("name value cannot be empty")
	}

	if (r.Name() == ClusterAPIProviderName) != (r.Type() == clusterctlv1.CoreProviderType) {
		return errors.Errorf("name %s must be used with the %s type (name: %s, type: %s)", ClusterAPIProviderName, clusterctlv1.CoreProviderType, r.Name(), r.Type())
	}

	if errMsgs := validation.IsDNS1123Subdomain(r.Name()); len(errMsgs) != 0 {
		return errors.Errorf("invalid provider name: %s", strings.Join(errMsgs, "; "))
	}
	if r.URL() == "" {
		return errors.New("provider URL value cannot be empty")
	}

	if _, err := url.Parse(r.URL()); err != nil {
		return errors.Wrap(err, "error parsing provider URL")
	}

	switch r.Type() {
	case clusterctlv1.CoreProviderType,
		clusterctlv1.BootstrapProviderType,
		clusterctlv1.InfrastructureProviderType,
		clusterctlv1.ControlPlaneProviderType,
		clusterctlv1.IPAMProviderType,
		clusterctlv1.RuntimeExtensionProviderType,
		clusterctlv1.AddonProviderType:
		break
	default:
		return errors.Errorf("invalid provider type. Allowed values are [%s, %s, %s, %s, %s, %s, %s]",
			clusterctlv1.CoreProviderType,
			clusterctlv1.BootstrapProviderType,
			clusterctlv1.InfrastructureProviderType,
			clusterctlv1.ControlPlaneProviderType,
			clusterctlv1.IPAMProviderType,
			clusterctlv1.RuntimeExtensionProviderType,
			clusterctlv1.AddonProviderType)
	}
	return nil
}
