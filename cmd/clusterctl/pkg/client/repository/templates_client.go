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

// TemplatesClient has methods to work with templatesClient hosted on a provider repository.
// Templates are yaml files to be used for creating a guest cluster.
type TemplatesClient interface {
	Get(flavor, bootstrap string, options TemplateOptions) (Template, error)
}

// templatesClient implements TemplatesClient.
type templatesClient struct {
	provider              config.Provider
	version               string
	repository            Repository
	configVariablesClient config.VariablesClient
}

// ensure templatesClient implements TemplatesClient.
var _ TemplatesClient = &templatesClient{}

// newTemplatesClient returns a templatesClient.
func newTemplatesClient(provider config.Provider, version string, repository Repository, configVariablesClient config.VariablesClient) *templatesClient {
	return &templatesClient{
		provider:              provider,
		version:               version,
		repository:            repository,
		configVariablesClient: configVariablesClient,
	}
}

// Get return the template for the flavor/bootstrap provider specified.
// In case the template does not exists, an error is returned.
func (f *templatesClient) Get(flavor, bootstrap string, options TemplateOptions) (Template, error) {
	//TODO: implement in a follow up PR
	return nil, nil
}
