/*
Copyright 2020 The Kubernetes Authors.

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

package repository

import (
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// TemplateOptions defines a set of well-know variables that all the cluster templates are expected to manage;
// this set of variables defines a simple, day1 experience that will be made accessible via flags in the clusterctl CLI.
// Please note that each provider/each template is allowed to add more variables, but additional variables are exposed
// only via environment variables or the clusterctl configuration file.
type TemplateOptions struct {
	ClusterName       string
	Namespace         string
	KubernetesVersion string
	ControlplaneCount int
	WorkerCount       int
}

// TemplateClient has methods to work with cluster templates hosted on a provider repository.
// Templates are yaml files to be used for creating a guest cluster.
type TemplateClient interface {
	Get(flavor, targetNamespace string) (Template, error)
}

// templateClient implements TemplateClient.
type templateClient struct {
	provider              config.Provider
	version               string
	repository            Repository
	configVariablesClient config.VariablesClient
}

// Ensure templateClient implements the TemplateClient interface.
var _ TemplateClient = &templateClient{}

// newTemplateClient returns a templateClient.
func newTemplateClient(provider config.Provider, version string, repository Repository, configVariablesClient config.VariablesClient) *templateClient {
	return &templateClient{
		provider:              provider,
		version:               version,
		repository:            repository,
		configVariablesClient: configVariablesClient,
	}
}

// Get return the template for the flavor specified.
// In case the template does not exists, an error is returned.
// Get assumes the following naming convention for templates: cluster-template[-<flavor_name>].yaml
func (c *templateClient) Get(flavor, targetNamespace string) (Template, error) {
	if targetNamespace == "" {
		return nil, errors.New("invalid arguments: please provide a targetNamespace")
	}

	// we are always reading templateClient for a well know version, that usually is
	// the version of the provider installed in the management cluster.
	version := c.version

	// building template name according with the naming convention
	name := "cluster-template"
	if flavor != "" {
		name = fmt.Sprintf("%s-%s", name, flavor)
	}
	name = fmt.Sprintf("%s.yaml", name)

	// read the component YAML, reading the local override file if it exists, otherwise read from the provider repository
	rawYaml, err := getLocalOverride(c.provider, version, name)
	if err != nil {
		return nil, err
	}

	if rawYaml == nil {
		rawYaml, err = c.repository.GetFile(version, name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %q from provider's repository %q", name, c.provider.Name())
		}
	}

	return newTemplate(newTemplateOptions{
		provider:              c.provider,
		version:               version,
		flavor:                flavor,
		rawYaml:               rawYaml,
		configVariablesClient: c.configVariablesClient,
		targetNamespace:       targetNamespace,
	})
}
