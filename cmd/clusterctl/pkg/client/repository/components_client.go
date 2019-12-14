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

package repository

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// ComponentsClient has methods to work with yaml file for generating provider components.
// Assets are yaml files to be used for deploying a provider into a management cluster.
type ComponentsClient interface {
	Get(version, targetNamespace, watchingNamespace string) (Components, error)
}

// componentsClient implements ComponentsClient.
type componentsClient struct {
	provider              config.Provider
	repository            Repository
	configVariablesClient config.VariablesClient
}

// ensure componentsClient implements ComponentsClient.
var _ ComponentsClient = &componentsClient{}

// newComponentsClient returns a componentsClient.
func newComponentsClient(provider config.Provider, repository Repository, configVariablesClient config.VariablesClient) *componentsClient {
	return &componentsClient{
		provider:              provider,
		repository:            repository,
		configVariablesClient: configVariablesClient,
	}
}

func (f *componentsClient) Get(version, targetNamespace, watchingNamespace string) (Components, error) {
	// if the request does not target a specific version, read from the default repository version that is derived from the repository URL, e.g. latest.
	if version == "" {
		version = f.repository.DefaultVersion()
	}

	// retrieve the path where the path is stored
	path := f.repository.ComponentsPath()

	// read from the components path.
	// If file is a path, the entire content of the path is returned.
	file, err := f.repository.GetFile(version, path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read %q from the repository for provider %q", path, f.provider.Name())
	}

	return newComponents(f.provider, version, file, f.configVariablesClient, targetNamespace, watchingNamespace)
}
