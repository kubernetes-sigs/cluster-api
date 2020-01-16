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
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/repository"
)

// InitOptions carries the options supported by Init
type InitOptions struct {
	Kubeconfig              string
	CoreProvider            string
	BootstrapProviders      []string
	InfrastructureProviders []string
	TargetNameSpace         string
	WatchingNamespace       string
}

// Client is exposes the clusterctl high-level client library
type Client interface {
	// GetProvidersConfig returns the list of providers configured for this instance of clusterctl.
	GetProvidersConfig() ([]Provider, error)

	// GetProviderComponents returns the provider components for a given provider, targetNamespace, watchingNamespace.
	GetProviderComponents(provider, targetNameSpace, watchingNamespace string) (Components, error)

	// Init initializes a management cluster by adding the requested list of providers.
	Init(options InitOptions) ([]Components, bool, error)
}

// clusterctlClient implements Client.
type clusterctlClient struct {
	configClient            config.Client
	repositoryClientFactory RepositoryClientFactory
	clusterClientFactory    ClusterClientFactory
}

type RepositoryClientFactory func(config.Provider) (repository.Client, error)
type ClusterClientFactory func(string) (cluster.Client, error)

// Ensure clusterctlClient implements Client.
var _ Client = &clusterctlClient{}

// NewOptions carries the options supported by New
type NewOptions struct {
	injectConfig            config.Client
	injectRepositoryFactory RepositoryClientFactory
	injectClusterFactory    ClusterClientFactory
}

// Option is a configuration option supplied to New
type Option func(*NewOptions)

// InjectConfig implements a New Option that allows to override the default configuration client used by clusterctl.
func InjectConfig(config config.Client) Option {
	return func(c *NewOptions) {
		c.injectConfig = config
	}
}

// InjectRepositoryFactory implements a New Option that allows to override the default factory used for creating
// RepositoryClient objects.
func InjectRepositoryFactory(factory RepositoryClientFactory) Option {
	return func(c *NewOptions) {
		c.injectRepositoryFactory = factory
	}
}

// InjectClusterClientFactory implements a New Option that allows to override the default factory used for creating
// ClusterClient objects.
func InjectClusterClientFactory(factory ClusterClientFactory) Option {
	return func(c *NewOptions) {
		c.injectClusterFactory = factory
	}
}

// New returns a configClient.
func New(path string, options ...Option) (Client, error) {
	return newClusterctlClient(path, options...)
}

func newClusterctlClient(path string, options ...Option) (*clusterctlClient, error) {
	cfg := &NewOptions{}
	for _, o := range options {
		o(cfg)
	}

	// if there is an injected config, use it, otherwise use the default one
	// provided by the config low level library
	configClient := cfg.injectConfig
	if configClient == nil {
		c, err := config.New(path)
		if err != nil {
			return nil, err
		}
		configClient = c
	}

	// if there is an injected RepositoryFactory, use it, otherwise use a default one
	repositoryClientFactory := cfg.injectRepositoryFactory
	if repositoryClientFactory == nil {
		repositoryClientFactory = defaultRepositoryFactory(configClient)
	}

	// if there is an injected ClusterFactory, use it, otherwise use a default one
	clusterClientFactory := cfg.injectClusterFactory
	if clusterClientFactory == nil {
		clusterClientFactory = defaultClusterFactory()
	}

	return &clusterctlClient{
		configClient:            configClient,
		repositoryClientFactory: repositoryClientFactory,
		clusterClientFactory:    clusterClientFactory,
	}, nil
}

// defaultClusterFactory is a ClusterClientFactory func the uses the default client provided by the cluster low level library
func defaultClusterFactory() func(kubeconfig string) (cluster.Client, error) {
	return func(kubeconfig string) (cluster.Client, error) {
		return cluster.New(kubeconfig, cluster.Options{}), nil
	}
}

// defaultRepositoryFactory is a RepositoryClientFactory func the uses the default client provided by the repository low level library
func defaultRepositoryFactory(configClient config.Client) func(providerConfig config.Provider) (repository.Client, error) {
	return func(providerConfig config.Provider) (repository.Client, error) {
		return repository.New(providerConfig, configClient.Variables(), repository.Options{})
	}
}
