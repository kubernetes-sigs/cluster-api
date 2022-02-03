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
	"sort"
	"strings"

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
	AWSProviderName       = "aws"
	AzureProviderName     = "azure"
	BYOHProviderName      = "byoh"
	DockerProviderName    = "docker"
	DOProviderName        = "digitalocean"
	GCPProviderName       = "gcp"
	HetznerProviderName   = "hetzner"
	IBMCloudProviderName  = "ibmcloud"
	Metal3ProviderName    = "metal3"
	NestedProviderName    = "nested"
	OpenStackProviderName = "openstack"
	PacketProviderName    = "packet"
	SideroProviderName    = "sidero"
	VSphereProviderName   = "vsphere"
	MAASProviderName      = "maas"
)

// Bootstrap providers.
const (
	KubeadmBootstrapProviderName = "kubeadm"
	TalosBootstrapProviderName   = "talos"
	AWSEKSBootstrapProviderName  = "aws-eks"
)

// ControlPlane providers.
const (
	KubeadmControlPlaneProviderName = "kubeadm"
	TalosControlPlaneProviderName   = "talos"
	AWSEKSControlPlaneProviderName  = "aws-eks"
	NestedControlPlaneProviderName  = "nested"
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
			name:         OpenStackProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-openstack/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         SideroProviderName,
			url:          "https://github.com/talos-systems/sidero/releases/latest/infrastructure-components.yaml",
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
			name:         IBMCloudProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},

		// Bootstrap providers
		&provider{
			name:         KubeadmBootstrapProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         TalosBootstrapProviderName,
			url:          "https://github.com/talos-systems/cluster-api-bootstrap-provider-talos/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		&provider{
			name:         AWSEKSBootstrapProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/latest/eks-bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
		// ControlPlane providers
		&provider{
			name:         KubeadmControlPlaneProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         TalosControlPlaneProviderName,
			url:          "https://github.com/talos-systems/cluster-api-control-plane-provider-talos/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         AWSEKSControlPlaneProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/latest/eks-controlplane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
		},
		&provider{
			name:         NestedControlPlaneProviderName,
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-nested/releases/latest/control-plane-components.yaml",
			providerType: clusterctlv1.ControlPlaneProviderType,
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

	errMsgs := validation.IsDNS1123Subdomain(r.Name())
	if len(errMsgs) != 0 {
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
		clusterctlv1.ControlPlaneProviderType:
		break
	default:
		return errors.Errorf("invalid provider type. Allowed values are [%s, %s, %s, %s]",
			clusterctlv1.CoreProviderType,
			clusterctlv1.BootstrapProviderType,
			clusterctlv1.InfrastructureProviderType,
			clusterctlv1.ControlPlaneProviderType)
	}
	return nil
}
