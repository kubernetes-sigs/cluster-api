/*
Copyright 2021 The Kubernetes Authors.

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

// ClusterClassClient has methods to work with cluster class templates hosted on a provider repository.
// Templates are yaml files to be used for creating a guest cluster.
type ClusterClassClient interface {
	Get(name, targetNamespace string, skipTemplateProcess bool) (Template, error)
}

type clusterClassClient struct {
	version               string
	provider              config.Provider
	repository            Repository
	configVariablesClient config.VariablesClient
	processor             yaml.Processor
}

// ClusterClassClientInput is an input struct for newClusterClassClient.
type ClusterClassClientInput struct {
	version               string
	provider              config.Provider
	repository            Repository
	configVariablesClient config.VariablesClient
	processor             yaml.Processor
}

func newClusterClassClient(input ClusterClassClientInput) *clusterClassClient {
	return &clusterClassClient{
		version:               input.version,
		provider:              input.provider,
		repository:            input.repository,
		configVariablesClient: input.configVariablesClient,
		processor:             input.processor,
	}
}

func (cc *clusterClassClient) Get(name, targetNamespace string, skipTemplateProcess bool) (Template, error) {
	log := logf.Log

	if targetNamespace == "" {
		return nil, errors.New("invalid arguments: please provide a targetNamespace")
	}

	version := cc.version
	filename := cc.processor.GetClusterClassTemplateName(version, name)

	// read the component YAML, reading the local override file if it exists, otherwise read from the provider repository
	rawArtifact, err := getLocalOverride(&newOverrideInput{
		configVariablesClient: cc.configVariablesClient,
		provider:              cc.provider,
		version:               version,
		filePath:              filename,
	})
	if err != nil {
		return nil, err
	}

	if rawArtifact == nil {
		log.V(5).Info("Fetching", "File", filename, "Provider", cc.provider.Name(), "Type", cc.provider.Type(), "Version", version)
		rawArtifact, err = cc.repository.GetFile(version, filename)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read %q from provider's repository %q", filename, cc.provider.ManifestLabel())
		}
	} else {
		log.V(1).Info("Using", "Override", filename, "Provider", cc.provider.ManifestLabel(), "Version", version)
	}

	return NewTemplate(TemplateInput{
		rawArtifact,
		cc.configVariablesClient,
		cc.processor,
		targetNamespace,
		skipTemplateProcess,
	})
}
