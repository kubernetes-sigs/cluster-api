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

// InitOptions carries the options supported by Init.
type InitOptions struct {
	// Kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig
	// discovery will be used.
	Kubeconfig string

	// CoreProvider version (e.g. cluster-api:v0.3.0) to add to the management cluster. By default (empty), the
	// cluster-api core provider's latest release is used.
	CoreProvider string

	// BootstrapProviders and versions (e.g. kubeadm-bootstrap:v0.3.0) to add to the management cluster.
	// By default (empty), the kubeadm bootstrap provider's latest release is used.
	BootstrapProviders []string

	// InfrastructureProviders and versions (e.g. aws:v0.5.0) to add to the management cluster.
	InfrastructureProviders []string

	// ControlPlaneProviders and versions (e.g. kubeadm-control-plane:v0.3.0) to add to the management cluster.
	// By default (empty), the kubeadm control plane provider latest release is used.
	ControlPlaneProviders []string

	// TargetNamespace defines the namespace where the providers should be deployed. If not specified, each provider
	// will be installed in a provider's default namespace.
	TargetNamespace string

	// WatchingNamespace defines the namespace the providers should watch to reconcile Cluster API objects.
	// If unspecified, the providers watches for Cluster API objects across all namespaces.
	WatchingNamespace string

	// LogUsageInstructions instructs the init command to print the usage instructions in case of first run.
	LogUsageInstructions bool
}

// GetClusterTemplateOptions carries the options supported by GetClusterTemplate.
type GetClusterTemplateOptions struct {
	// Kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig
	// discovery will be used.
	Kubeconfig string

	// InfrastructureProvider that should be used for creating the workload cluster.
	InfrastructureProvider string

	// Flavor defines the template variant to be used for creating the workload cluster.
	Flavor string

	// TargetNamespace where the objects describing the workload cluster should be deployed. If not specified,
	// the current namespace will be used.
	TargetNamespace string

	// ClusterName to be used for the workload cluster.
	ClusterName string

	// KubernetesVersion to use for the workload cluster. By default (empty), the value from os env variables
	// or the .cluster-api/clusterctl.yaml config file will be used.
	KubernetesVersion string

	// ControlPlaneMachineCount defines the number of control plane machines to be added to the workload cluster.
	ControlPlaneMachineCount int

	// WorkerMachineCount defines number of worker machines to be added to the workload cluster.
	WorkerMachineCount int
}

// DeleteOptions carries the options supported by Delete.
type DeleteOptions struct {
	// Kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig
	// discovery will be used.
	Kubeconfig string

	// ForceDeleteNamespace forces the deletion of the namespace where the providers are hosted
	// (and of all the contained objects).
	ForceDeleteNamespace bool

	// ForceDeleteCRD forces the deletion of the provider's CRDs (and of all the related objects)".
	ForceDeleteCRD bool

	// Namespace where the provider to be deleted lives. If not specified, the namespace name will be inferred
	// from the current configuration.
	Namespace string

	// Providers to be delete. By default (empty), all the provider will be deleted.
	Providers []string
}

// MoveOptions carries the options supported by move.
type MoveOptions struct {
	// FromKubeconfig defines the kubeconfig file to use for accessing the source management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	FromKubeconfig string

	// ToKubeconfig defines the path to the kubeconfig file to use for accessing the target management cluster.
	ToKubeconfig string

	// Namespace where the objects describing the workload cluster exists. If not specified, the current
	// namespace will be used.
	Namespace string
}

// PlanUpgradeOptions carries the options supported by upgrade plan.
type PlanUpgradeOptions struct {
	// Kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig discovery will be used.
	Kubeconfig string
}

// Client is exposes the clusterctl high-level client library.
type Client interface {
	// GetProvidersConfig returns the list of providers configured for this instance of clusterctl.
	GetProvidersConfig() ([]Provider, error)

	// GetProviderComponents returns the provider components for a given provider, targetNamespace, watchingNamespace.
	GetProviderComponents(provider, targetNameSpace, watchingNamespace string) (Components, error)

	// Init initializes a management cluster by adding the requested list of providers.
	Init(options InitOptions) ([]Components, error)

	// GetClusterTemplate returns a workload cluster template.
	GetClusterTemplate(options GetClusterTemplateOptions) (Template, error)

	// Delete deletes providers from a management cluster.
	Delete(options DeleteOptions) error

	// Move moves all the Cluster API objects existing in a namespace (or from all the namespaces if empty) to a target management cluster.
	Move(options MoveOptions) error

	// PlanUpgrade returns a set of suggested Upgrade plans for the cluster, and more specifically:
	// - Each management group gets separated upgrade plans.
	// - For each management group, an upgrade plan is generated for each API Version of Cluster API (contract) available, e.g.
	//   - Upgrade to the latest version in the the v1alpha2 series: ....
	//   - Upgrade to the latest version in the the v1alpha3 series: ....
	PlanUpgrade(options PlanUpgradeOptions) ([]UpgradePlan, error)
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

// Option is a configuration option supplied to New
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

	return client, nil
}

// defaultClusterFactory is a ClusterClientFactory func the uses the default client provided by the cluster low level library.
func defaultClusterFactory(configClient config.Client) func(kubeconfig string) (cluster.Client, error) {
	return func(kubeconfig string) (cluster.Client, error) {
		return cluster.New(kubeconfig, configClient), nil
	}
}

// defaultRepositoryFactory is a RepositoryClientFactory func the uses the default client provided by the repository low level library.
func defaultRepositoryFactory(configClient config.Client) func(providerConfig config.Provider) (repository.Client, error) {
	return func(providerConfig config.Provider) (repository.Client, error) {
		return repository.New(providerConfig, configClient.Variables())
	}
}
