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

package client

import (
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
)

// getComponentsByName is a utility method that returns components
// for a given provider with options including targetNamespace.
func (c *clusterctlClient) getComponentsByName(provider string, providerType clusterctlv1.ProviderType, options repository.ComponentsOptions) (repository.Components, error) {
	// Parse the abbreviated syntax for name[:version]
	name, version, err := parseProviderName(provider)
	if err != nil {
		return nil, err
	}
	options.Version = version

	// Gets the provider configuration (that includes the location of the provider repository)
	providerConfig, err := c.configClient.Providers().Get(name, providerType)
	if err != nil {
		return nil, err
	}

	// Get a client for the provider repository and read the provider components;
	// during the process, provider components will be processed performing variable substitution, customization of target
	// namespace etc.
	// Currently we are not supporting custom yaml processors for the provider
	// components. So we revert to using the default SimpleYamlProcessor.
	repositoryClientFactory, err := c.repositoryClientFactory(RepositoryClientFactoryInput{Provider: providerConfig})
	if err != nil {
		return nil, err
	}

	components, err := repositoryClientFactory.Components().Get(options)
	if err != nil {
		return nil, err
	}
	return components, nil
}

// parseProviderName defines a utility function that parses the abbreviated syntax for name[:version].
func parseProviderName(provider string) (name string, version string, err error) {
	t := strings.Split(strings.ToLower(provider), ":")
	if len(t) > 2 {
		return "", "", errors.Errorf("invalid provider name %q. Provider name should be in the form name[:version]", provider)
	}

	if t[0] == "" {
		return "", "", errors.Errorf("invalid provider name %q. Provider name should be in the form name[:version] and name cannot be empty", provider)
	}

	name = t[0]
	if err := validateDNS1123Label(name); err != nil {
		return "", "", errors.Wrapf(err, "invalid provider name %q. Provider name should be in the form name[:version] and the name should be valid", provider)
	}

	version = ""
	if len(t) > 1 {
		if t[1] == "" {
			return "", "", errors.Errorf("invalid provider name %q. Provider name should be in the form name[:version] and version cannot be empty", provider)
		}
		version = t[1]
	}

	return name, version, nil
}

func validateDNS1123Label(label string) error {
	errs := validation.IsDNS1123Label(label)
	if len(errs) != 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func validateDNS1123Domanin(subdomain string) error {
	errs := validation.IsDNS1123Subdomain(subdomain)
	if len(errs) != 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
