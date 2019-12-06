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
	"k8s.io/klog"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

const (
	ClusterAPIName     = "cluster-api"
	ProvidersConfigKey = "providers"
)

// ProvidersClient has methods to work with provider configurations.
type ProvidersClient interface {
	// List returns all the provider configurations, including provider configurations hard-coded in clusterctl
	// and user-defined provider configurations read from the clusterctl configuration file.
	// In case of conflict, user-defined provider override the hard-coded configurations.
	List() ([]Provider, error)

	// Get returns the configuration for the provider with a given name.
	// In case the name does not correspond to any existing provider, an error is returned.
	Get(name string) (*Provider, error)
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

	// clusterctl includes an hard-coded list of "default" providers in order to provide to the users the simplest
	// out-of-box experience. Other providers can be added by using the clusterctl configuration file.

	// if you are a developer of a community maintained provider, you can send a PR to extend the following list.

	defaults := []Provider{
		// cluster API core provider
		&provider{
			name:         ClusterAPIName,
			url:          "https://github.com/kubernetes-sigs/cluster-api/releases/latest/cluster-api-components.yaml",
			providerType: clusterctlv1.CoreProviderType,
		},

		// Infrastructure providersClient
		&provider{
			name:         "aws",
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         "docker",
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-docker/releases/latest/provider_components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},
		&provider{
			name:         "vsphere",
			url:          "https://github.com/kubernetes-sigs/cluster-api-provider-vsphere/releases/latest/infrastructure-components.yaml",
			providerType: clusterctlv1.InfrastructureProviderType,
		},

		// Bootstrap providersClient
		// TODO: CABPK in v1alpha3 will be included into CAPI, so this entry can be removed as soon as v1alpha3 is ready for test
		&provider{
			name:         "kubeadm",
			url:          "https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/releases/latest/bootstrap-components.yaml",
			providerType: clusterctlv1.BootstrapProviderType,
		},
	}

	return defaults
}

// configProvider mirrors config.Provider interface and allows serialization of the corresponding info
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

	var userDefinedProviders []configProvider //nolint
	if err := p.reader.UnmarshalKey(ProvidersConfigKey, &userDefinedProviders); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal providers from the clusterctl configuration file")
	}

	for _, u := range userDefinedProviders {
		provider := NewProvider(u.Name, u.URL, u.Type)
		if err := validateProvider(provider); err != nil {
			return nil, errors.Wrapf(err, "error validating configuration from %q. Please fix the providers value in clusterctl configuration file", provider.Name())
		}

		override := false
		for i := range providers {
			if providers[i].Name() == provider.Name() {
				providers[i] = provider
				override = true
				klog.V(3).Infof("The clusterctl configuration file overrides default configuration for provider %s", provider.Name())
			}
		}

		if !override {
			providers = append(providers, provider)
		}
	}

	// ensure provider configurations are consistently sorted
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name() < providers[j].Name()
	})

	return providers, nil
}

func (p *providersClient) Get(name string) (*Provider, error) {
	l, err := p.List()
	if err != nil {
		return nil, err
	}

	for _, r := range l {
		if name == r.Name() {
			return &r, nil
		}
	}

	return nil, errors.Errorf("failed to get configuration for %q provider. Please check the provider name and/or add configuration for new providers using the .clusterctl config file", name)
}

func validateProvider(r Provider) error {
	if r.Name() == "" {
		return errors.New("name value cannot be empty")
	}
	errMsgs := validation.IsDNS1123Subdomain(r.Name())
	if len(errMsgs) != 0 {
		return errors.Errorf("invalid provider name: %s", strings.Join(errMsgs, "; "))
	}
	if r.URL() == "" {
		return errors.New("provider URL value cannot be empty")
	}

	_, err := url.Parse(r.URL())
	if err != nil {
		return errors.Wrap(err, "error parsing provider URL")
	}

	switch r.Type() {
	case clusterctlv1.CoreProviderType,
		clusterctlv1.BootstrapProviderType,
		clusterctlv1.InfrastructureProviderType:
		break
	default:
		return errors.Errorf("invalid provider type. Allowed values are [%s, %s, %s]",
			clusterctlv1.CoreProviderType,
			clusterctlv1.BootstrapProviderType,
			clusterctlv1.InfrastructureProviderType)
	}
	return nil
}
