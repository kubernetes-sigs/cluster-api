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
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/alpha"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
)

// Client is exposes the clusterctl high-level client library.
type Client interface {
	// GetProvidersConfig returns the list of providers configured for this instance of clusterctl.
	GetProvidersConfig() ([]Provider, error)

	// GetProviderComponents returns the provider components for a given provider with options including targetNamespace.
	GetProviderComponents(provider string, providerType clusterctlv1.ProviderType, options ComponentsOptions) (Components, error)

	// Init initializes a management cluster by adding the requested list of providers.
	Init(options InitOptions) ([]Components, error)

	// InitImages returns the list of images required for executing the init command.
	InitImages(options InitOptions) ([]string, error)

	// GetClusterTemplate returns a workload cluster template.
	GetClusterTemplate(options GetClusterTemplateOptions) (Template, error)

	// GetKubeconfig returns the kubeconfig of the workload cluster.
	GetKubeconfig(options GetKubeconfigOptions) (string, error)

	// Delete deletes providers from a management cluster.
	Delete(options DeleteOptions) error

	// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster.
	Move(options MoveOptions) error

	// Backup saves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster.
	Backup(options BackupOptions) error

	// Restore restores all the Cluster API objects existing in a configured directory based on a glob to a target management cluster.
	Restore(options RestoreOptions) error

	// PlanUpgrade returns a set of suggested Upgrade plans for the cluster, and more specifically:
	// - Upgrade to the latest version in the the v1alpha3 series: ....
	// - Upgrade to the latest version in the the v1alpha4 series: ....
	PlanUpgrade(options PlanUpgradeOptions) ([]UpgradePlan, error)

	// PlanCertManagerUpgrade returns a CertManagerUpgradePlan.
	PlanCertManagerUpgrade(options PlanUpgradeOptions) (CertManagerUpgradePlan, error)

	// ApplyUpgrade executes an upgrade plan.
	ApplyUpgrade(options ApplyUpgradeOptions) error

	// ProcessYAML provides a direct way to process a yaml and inspect its
	// variables.
	ProcessYAML(options ProcessYAMLOptions) (YamlPrinter, error)

	// DescribeCluster returns the object tree representing the status of a Cluster API cluster.
	DescribeCluster(options DescribeClusterOptions) (*tree.ObjectTree, error)

	// AlphaClient is an Interface for alpha features in clusterctl
	AlphaClient
}

// AlphaClient exposes the alpha features in clusterctl high-level client library.
type AlphaClient interface {
	// RolloutRestart provides rollout restart of cluster-api resources
	RolloutRestart(options RolloutOptions) error
	// RolloutPause provides rollout pause of cluster-api resources
	RolloutPause(options RolloutOptions) error
	// RolloutResume provides rollout resume of paused cluster-api resources
	RolloutResume(options RolloutOptions) error
	// RolloutUndo provides rollout rollback of cluster-api resources
	RolloutUndo(options RolloutOptions) error
	// TopologyPlan dry runs the topology reconciler
	TopologyPlan(options TopologyPlanOptions) (*TopologyPlanOutput, error)
}

// YamlPrinter exposes methods that prints the processed template and
// variables.
type YamlPrinter interface {
	// Variables required by the template.
	Variables() []string

	// Yaml returns yaml defining all the cluster template objects as a byte array.
	Yaml() ([]byte, error)
}

// clusterctlClient implements Client.
type clusterctlClient struct {
	configClient            config.Client
	repositoryClientFactory RepositoryClientFactory
	clusterClientFactory    ClusterClientFactory
	alphaClient             alpha.Client
}

// RepositoryClientFactoryInput represents the inputs required by the factory.
type RepositoryClientFactoryInput struct {
	Provider  Provider
	Processor Processor
}

// RepositoryClientFactory is a factory of repository.Client from a given input.
type RepositoryClientFactory func(RepositoryClientFactoryInput) (repository.Client, error)

// ClusterClientFactoryInput reporesents the inputs required by the factory.
type ClusterClientFactoryInput struct {
	Kubeconfig Kubeconfig
	Processor  Processor
}

// ClusterClientFactory is a factory of cluster.Client from a given input.
type ClusterClientFactory func(ClusterClientFactoryInput) (cluster.Client, error)

// Ensure clusterctlClient implements Client.
var _ Client = &clusterctlClient{}

// Option is a configuration option supplied to New.
type Option func(*clusterctlClient)

// InjectConfig allows to override the default configuration client used by clusterctl.
func InjectConfig(config config.Client) Option {
	return func(c *clusterctlClient) {
		c.configClient = config
	}
}

// InjectRepositoryFactory allows to override the default factory used for creating
// RepositoryClient objects.
func InjectRepositoryFactory(factory RepositoryClientFactory) Option {
	return func(c *clusterctlClient) {
		c.repositoryClientFactory = factory
	}
}

// InjectClusterClientFactory allows to override the default factory used for creating
// ClusterClient objects.
func InjectClusterClientFactory(factory ClusterClientFactory) Option {
	return func(c *clusterctlClient) {
		c.clusterClientFactory = factory
	}
}

// New returns a configClient.
func New(path string, options ...Option) (Client, error) {
	return newClusterctlClient(path, options...)
}

func newClusterctlClient(path string, options ...Option) (*clusterctlClient, error) {
	client := &clusterctlClient{}
	for _, o := range options {
		o(client)
	}

	// if there is an injected config, use it, otherwise use the default one
	// provided by the config low level library.
	if client.configClient == nil {
		c, err := config.New(path)
		if err != nil {
			return nil, err
		}
		client.configClient = c
	}

	// if there is an injected RepositoryFactory, use it, otherwise use a default one.
	if client.repositoryClientFactory == nil {
		client.repositoryClientFactory = defaultRepositoryFactory(client.configClient)
	}

	// if there is an injected ClusterFactory, use it, otherwise use a default one.
	if client.clusterClientFactory == nil {
		client.clusterClientFactory = defaultClusterFactory(client.configClient)
	}

	// if there is an injected alphaClient, use it, otherwise use a default one.
	if client.alphaClient == nil {
		c := alpha.New()
		client.alphaClient = c
	}

	return client, nil
}

// defaultRepositoryFactory is a RepositoryClientFactory func the uses the default client provided by the repository low level library.
func defaultRepositoryFactory(configClient config.Client) RepositoryClientFactory {
	return func(input RepositoryClientFactoryInput) (repository.Client, error) {
		return repository.New(
			input.Provider,
			configClient,
			repository.InjectYamlProcessor(input.Processor),
		)
	}
}

// defaultClusterFactory is a ClusterClientFactory func the uses the default client provided by the cluster low level library.
func defaultClusterFactory(configClient config.Client) ClusterClientFactory {
	return func(input ClusterClientFactoryInput) (cluster.Client, error) {
		return cluster.New(
			// Kubeconfig is a type alias to cluster.Kubeconfig
			cluster.Kubeconfig(input.Kubeconfig),
			configClient,
			cluster.InjectYamlProcessor(input.Processor),
		), nil
	}
}
