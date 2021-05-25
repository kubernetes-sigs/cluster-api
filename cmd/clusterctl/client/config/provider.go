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
	"encoding/json"
	"path/filepath"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

// Provider defines a provider configuration.
type Provider interface {
	// Name returns the name of the provider.
	Name() string

	// Type returns the type of the provider.
	Type() clusterctlv1.ProviderType

	// URL returns the name of the provider repository.
	URL() string

	// SameAs returns true if two providers have the same name and type.
	// Please note that this uniquely identifies a provider configuration, but not the provider instances in the cluster
	// because it is possible to create many instances of the same provider.
	SameAs(other Provider) bool

	// ManifestLabel returns the cluster.x-k8s.io/provider label value for a provider.
	// Please note that this label uniquely identifies the provider, e.g. bootstrap-kubeadm, but not the instances of
	// the provider, e.g. namespace-1/bootstrap-kubeadm and namespace-2/bootstrap-kubeadm
	ManifestLabel() string

	// Less func can be used to ensure a consist order of provider lists.
	Less(other Provider) bool
}

// provider implements Provider.
type provider struct {
	name         string
	url          string
	providerType clusterctlv1.ProviderType
}

// ensure provider implements provider.
var _ Provider = &provider{}

func (p *provider) Name() string {
	return p.name
}

func (p *provider) URL() string {
	return p.url
}

func (p *provider) Type() clusterctlv1.ProviderType {
	return p.providerType
}

func (p *provider) SameAs(other Provider) bool {
	return p.name == other.Name() && p.providerType == other.Type()
}

func (p *provider) ManifestLabel() string {
	return clusterctlv1.ManifestLabel(p.name, p.Type())
}

func (p *provider) Less(other Provider) bool {
	return p.providerType.Order() < other.Type().Order() ||
		(p.providerType.Order() == other.Type().Order() && p.name < other.Name())
}

// NewProvider creates a new Provider with the given input.
func NewProvider(name string, url string, ttype clusterctlv1.ProviderType) Provider {
	return &provider{
		name:         name,
		url:          url,
		providerType: ttype,
	}
}

func (p provider) MarshalJSON() ([]byte, error) {
	dir, file := filepath.Split(p.url)
	j, err := json.Marshal(struct {
		Name         string
		ProviderType clusterctlv1.ProviderType
		URL          string
		File         string
	}{
		Name:         p.name,
		ProviderType: p.providerType,
		URL:          dir,
		File:         file,
	})
	if err != nil {
		return nil, err
	}
	return j, nil
}
