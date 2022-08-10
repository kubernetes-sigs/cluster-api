/*
Copyright 2022 The Kubernetes Authors.

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

package client

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/version"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

func (c *clusterctlClient) GenerateProvider(provider string, providerType clusterctlv1.ProviderType, options ComponentsOptions) (Components, error) {
	providerName, providerVersion, err := parseProviderName(provider)
	if err != nil {
		return nil, err
	}

	configRepository, err := c.configClient.Providers().Get(providerName, providerType)
	if err != nil {
		return nil, err
	}

	providerRepositoryClient, err := c.repositoryClientFactory(RepositoryClientFactoryInput{Provider: configRepository})
	if err != nil {
		return nil, err
	}

	if providerVersion == "" {
		providerVersion = providerRepositoryClient.DefaultVersion()
	}

	latestMetadata, err := providerRepositoryClient.Metadata(providerVersion).Get()
	if err != nil {
		return nil, err
	}

	currentVersion, err := version.ParseSemantic(providerVersion)
	if err != nil {
		return nil, err
	}

	releaseSeries := latestMetadata.GetReleaseSeriesForVersion(currentVersion)
	if releaseSeries == nil {
		return nil, errors.Errorf("invalid provider metadata: version %s for the provider %s does not match any release series", providerVersion, providerName)
	}

	if releaseSeries.Contract != clusterv1.GroupVersion.Version {
		return nil, errors.Errorf("current version of clusterctl is only compatible with %s providers, detected %s for provider %s", clusterv1.GroupVersion.Version, releaseSeries.Contract, providerName)
	}

	return c.GetProviderComponents(provider, providerType, options)
}
