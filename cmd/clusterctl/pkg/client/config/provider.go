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
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

// Provider defines a provider configuration.
type Provider interface {
	Name() string
	URL() string
	Type() clusterctlv1.ProviderType
}

// provider implements provider
type provider struct {
	name         string
	url          string
	providerType clusterctlv1.ProviderType
}

// ensure provider implements provider
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

func NewProvider(name string, url string, ttype clusterctlv1.ProviderType) Provider {
	return &provider{
		name:         name,
		url:          url,
		providerType: ttype,
	}
}
