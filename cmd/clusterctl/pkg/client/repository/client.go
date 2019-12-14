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
	"net/url"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

// Client is used to interact with provider repositories.
// Provider repository are expected to contain two types of YAML files:
// - YAML files defining the provider components (CRD, Controller, RBAC etc.)
// - YAML files defining the cluster templates (Cluster, Machines)
type Client interface {
	config.Provider

	// Components provide access to YAML file for creating provider components.
	Components() ComponentsClient

	// Templates provide access to YAML file for generating workload cluster templates.
	// Please note that templates are expected to exist for the infrastructure providers only.
	Templates(version string) TemplatesClient
}

// repositoryClient implements Client.
type repositoryClient struct {
	config.Provider
	configVariablesClient config.VariablesClient
	repository            Repository
}

// ensure repositoryClient implements Client.
var _ Client = &repositoryClient{}

func (c *repositoryClient) Components() ComponentsClient {
	return newComponentsClient(c.Provider, c.repository, c.configVariablesClient)
}

func (c *repositoryClient) Templates(version string) TemplatesClient {
	return newTemplatesClient(c.Provider, version, c.repository, c.configVariablesClient)
}

// NewOptions carries the options supported by New
type NewOptions struct {
	injectRepository Repository
}

// Option is a configuration option supplied to New
type Option func(*NewOptions)

// InjectRepository allows to override the repository implementation to use;
// by default, the repository implementation to use is created according to the
// repository URL.
func InjectRepository(repository Repository) Option {
	return func(c *NewOptions) {
		c.injectRepository = repository
	}
}

// New returns a Client.
func New(provider config.Provider, configVariablesClient config.VariablesClient, options Options) (Client, error) {
	return newRepositoryClient(provider, configVariablesClient, options)
}

func newRepositoryClient(provider config.Provider, configVariablesClient config.VariablesClient, options Options) (*repositoryClient, error) {
	repository := options.InjectRepository
	if repository == nil {
		r, err := repositoryFactory(provider, configVariablesClient)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get repository client for %q", provider.Name())
		}
		repository = r
	}

	return &repositoryClient{
		Provider:              provider,
		repository:            repository,
		configVariablesClient: configVariablesClient,
	}, nil
}

// Options allow to set Client options
type Options struct {
	InjectRepository Repository
}

// Repository defines the behavior of a repository implementation.
// clusterctl is designed to support different repository types; each repository implementation should be aware of
// the provider version they are hosting, and possibly to host more than one version.
type Repository interface {
	// DefaultVersion returns the default provider version returned by a repository.
	// In case the repository URL points to latest, this method returns the current latest version; in other cases
	// it returns the version of the provider hosted in the repository.
	DefaultVersion() string

	// RootPath returns the path inside the repository where the YAML file for creating provider components and
	// the YAML file for generating workload cluster templates are stored.
	// This value is derived from the repository URL; all the paths returned by this interface should be relative to this path.
	RootPath() string

	// ComponentsPath return the path (a folder name or file name) of the YAML file for creating provider components.
	// This value is derived from the repository URL.
	ComponentsPath() string

	// GetFile return a file for a given provider version.
	GetFile(version string, path string) ([]byte, error)
}

var _ Repository = &test.FakeRepository{}

//repositoryFactory returns the repository implementation corresponding to the provider URL.
func repositoryFactory(providerConfig config.Provider, configVariablesClient config.VariablesClient) (Repository, error) { //nolint
	// parse the repository url
	rURL, err := url.Parse(providerConfig.URL())
	if err != nil {
		return nil, errors.Errorf("failed to parse repository url %q", providerConfig.URL())
	}

	// if the url is a github repository
	//TODO: implement in a follow up PR

	// if the url is a local repository
	//TODO: implement in a follow up PR

	return nil, errors.Errorf("invalid provider url. there are no provider implementation for %q schema", rURL.Scheme)
}
