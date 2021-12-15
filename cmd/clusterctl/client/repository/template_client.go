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
	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

// TemplateClient has methods to work with cluster templates hosted on a provider repository.
// Templates are yaml files to be used for creating a guest cluster.
type TemplateClient interface {
	Get(flavor, targetNamespace string, listVariablesOnly bool) (Template, error)
}

// templateClient implements TemplateClient.
type templateClient struct {
	provider              config.Provider
	version               string
	repository            Repository
	configVariablesClient config.VariablesClient
	processor             yaml.Processor
}

// TemplateClientInput is an input strict for newTemplateClient.
type TemplateClientInput struct {
	version               string
	provider              config.Provider
	repository            Repository
	configVariablesClient config.VariablesClient
	processor             yaml.Processor
}

// Ensure templateClient implements the TemplateClient interface.
var _ TemplateClient = &templateClient{}

// newTemplateClient returns a templateClient. It uses the SimpleYamlProcessor
// by default.
func newTemplateClient(input TemplateClientInput) *templateClient {
	return &templateClient{
		provider:              input.provider,
		version:               input.version,
		repository:            input.repository,
		configVariablesClient: input.configVariablesClient,
		processor:             input.processor,
	}
}

// Get return the template for the flavor specified.
// In case the template does not exists, an error is returned.
// Get assumes the following naming convention for templates: cluster-template[-<flavor_name>].yaml.
func (c *templateClient) Get(flavor, targetNamespace string, skipTemplateProcess bool) (Template, error) {
	log := logf.Log

	if targetNamespace == "" {
		return nil, errors.New("invalid arguments: please provide a targetNamespace")
	}

	version := c.version
	name := c.processor.GetTemplateName(version, flavor)

	// read the component YAML, reading the local override file if it exists, otherwise read from the provider repository
	rawArtifact, err := getLocalOverride(&newOverrideInput{
		configVariablesClient: c.configVariablesClient,
		provider:              c.provider,
		version:               version,
		filePath:              name,
	})
	if err != nil {
		return nil, err
	}

	if rawArtifact == nil {
		log.V(5).Info("Fetching", "File", name, "Provider", c.provider.Name(), "Type", c.provider.Type(), "Version", version)
		rawArtifact, err = c.repository.GetFile(version, name)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %q from provider's repository %q", name, c.provider.ManifestLabel())
		}
	} else {
		log.V(1).Info("Using", "Override", name, "Provider", c.provider.ManifestLabel(), "Version", version)
	}

	return NewTemplate(TemplateInput{
		rawArtifact,
		c.configVariablesClient,
		c.processor,
		targetNamespace,
		skipTemplateProcess,
	})
}
