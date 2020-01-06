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
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
)

// getComponentsByName is a utility method that returns components for a given provider, targetNamespace, and watchingNamespace.
func (c *clusterctlClient) getComponentsByName(provider string, targetNamespace string, watchingNamespace string) (repository.Components, error) {

	// parse the abbreviated syntax for name[:version]
	name, version, err := parseProviderName(provider)
	if err != nil {
		return nil, err
	}

	// gets the provider configuration (that includes the location of the provider repository)
	providerConfig, err := c.configClient.Providers().Get(name)
	if err != nil {
		return nil, err
	}

	// get a client for the provider repository and read the provider components;
	// during the process, provider components will be processed performing variable substitution, customization of target
	// and watching namespace etc.

	repository, err := c.repositoryClientFactory(*providerConfig)
	if err != nil {
		return nil, err
	}

	components, err := repository.Components().Get(version, targetNamespace, watchingNamespace)
	if err != nil {
		return nil, err
	}
	return components, nil
}

// parseProviderName defines a utility function that parses the abbreviated syntax for name[:version]
func parseProviderName(provider string) (name string, version string, err error) {
	t := strings.Split(strings.ToLower(provider), ":")
	if len(t) > 2 {
		return "", "", errors.Errorf("invalid provider name %q. Provider name should be in the form name[:version]", provider)
	}

	if t[0] == "" {
		return "", "", errors.Errorf("invalid provider name %q. Provider name should be in the form name[:version] and name cannot be empty", provider)
	}

	name = t[0]
	errs := validation.IsDNS1123Label(name)
	if len(errs) != 0 {
		return "", "", errors.Errorf("invalid name value: %s", strings.Join(errs, "; "))
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
