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
	"strconv"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/version"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
)

func (c *clusterctlClient) GetProvidersConfig() ([]Provider, error) {
	r, err := c.configClient.Providers().List()
	if err != nil {
		return nil, err
	}

	// Provider is an alias for config.Provider; this makes the conversion
	rr := make([]Provider, len(r))
	for i, provider := range r {
		rr[i] = provider
	}

	return rr, nil
}

func (c *clusterctlClient) GetProviderComponents(provider, targetNameSpace, watchingNamespace string) (Components, error) {
	components, err := c.getComponentsByName(provider, targetNameSpace, watchingNamespace)
	if err != nil {
		return nil, err
	}
	return components, nil
}

// GetClusterTemplateOptions carries the options supported by GetClusterTemplate.
type GetClusterTemplateOptions struct {
	// Kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig
	// discovery will be used.
	Kubeconfig string

	// ProviderRepositorySource to be used for reading the workload cluster template from a provider repository;
	// only one template source can be used at time; if not other source will be set, a ProviderRepositorySource
	// will be generated inferring values from the cluster.
	ProviderRepositorySource *ProviderRepositorySourceOptions

	// URLSource to be used for reading the workload cluster template; only one template source can be used at time.
	URLSource *URLSourceOptions

	// ConfigMapSource to be used for reading the workload cluster template; only one template source can be used at time.
	ConfigMapSource *ConfigMapSourceOptions

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

	// listVariablesOnly sets the GetClusterTemplate method to return the list of variables expected by the template
	// without executing any further processing.
	ListVariablesOnly bool
}

// numSources return the number of template sources currently set on a GetClusterTemplateOptions.
func (o *GetClusterTemplateOptions) numSources() int {
	numSources := 0
	if o.ProviderRepositorySource != nil {
		numSources++
	}
	if o.ConfigMapSource != nil {
		numSources++
	}
	if o.URLSource != nil {
		numSources++
	}
	return numSources
}

// ProviderRepositorySourceOptions defines the options to be used when reading a workload cluster template
// from a provider repository.
type ProviderRepositorySourceOptions struct {
	// InfrastructureProvider to read the workload cluster template from. By default (empty), the default
	// infrastructure provider will be used if no other sources are specified.
	InfrastructureProvider string

	// Flavor defines The workload cluster template variant to be used when reading from the infrastructure
	// provider repository. By default (empty), the default cluster template will be used.
	Flavor string
}

// URLSourceOptions defines the options to be used when reading a workload cluster template from an URL.
type URLSourceOptions struct {
	// URL to read the workload cluster template from.
	URL string
}

// DefaultCustomTemplateConfigMapKey  where the workload cluster template is hosted.
const DefaultCustomTemplateConfigMapKey = "template"

// ConfigMapSourceOptions defines the options to be used when reading a workload cluster template from a ConfigMap.
type ConfigMapSourceOptions struct {
	// Namespace where the ConfigMap exists. By default (empty), the current namespace will be used.
	Namespace string

	// Name to read the workload cluster template from.
	Name string

	// DataKey where the workload cluster template is hosted. By default (empty), the
	// DefaultCustomTemplateConfigMapKey will be used.
	DataKey string
}

func (c *clusterctlClient) GetClusterTemplate(options GetClusterTemplateOptions) (Template, error) {
	// Checks that no more than on source is set
	numsSource := options.numSources()
	if numsSource > 1 {
		return nil, errors.New("invalid cluster template source: only one template can be used at time")
	}

	// If no source is set, defaults to using an empty ProviderRepositorySource so values will be
	// inferred from the cluster inventory.
	if numsSource == 0 {
		options.ProviderRepositorySource = &ProviderRepositorySourceOptions{}
	}

	// Gets  the client for the current management cluster
	cluster, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return nil, err
	}

	// If the option specifying the targetNamespace is empty, try to detect it.
	if options.TargetNamespace == "" {
		currentNamespace, err := cluster.Proxy().CurrentNamespace()
		if err != nil {
			return nil, err
		}
		options.TargetNamespace = currentNamespace
	}

	// Inject some of the templateOptions into the configClient so they can be consumed as a variables from the template.
	if err := c.templateOptionsToVariables(options); err != nil {
		return nil, err
	}

	// Gets the workload cluster template from the selected source
	if options.ProviderRepositorySource != nil {
		return c.getTemplateFromRepository(cluster, *options.ProviderRepositorySource, options.TargetNamespace, options.ListVariablesOnly)
	}
	if options.ConfigMapSource != nil {
		return c.getTemplateFromConfigMap(cluster, *options.ConfigMapSource, options.TargetNamespace, options.ListVariablesOnly)
	}
	if options.URLSource != nil {
		return c.getTemplateFromURL(cluster, *options.URLSource, options.TargetNamespace, options.ListVariablesOnly)
	}

	return nil, errors.New("unable to read custom template. Please specify a template source")
}

// getTemplateFromRepository returns a workload cluster template from a provider repository.
func (c *clusterctlClient) getTemplateFromRepository(cluster cluster.Client, source ProviderRepositorySourceOptions, targetNamespace string, listVariablesOnly bool) (Template, error) {
	// ensure the custom resource definitions required by clusterctl are in place
	if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	// If the option specifying the name of the infrastructure provider to get templates from is empty, try to detect it.
	provider := source.InfrastructureProvider
	if provider == "" {
		defaultProviderName, err := cluster.ProviderInventory().GetDefaultProviderName(clusterctlv1.InfrastructureProviderType)
		if err != nil {
			return nil, err
		}

		if defaultProviderName == "" {
			return nil, errors.New("failed to identify the default infrastructure provider. Please specify an infrastructure provider")
		}
		provider = defaultProviderName
	}

	// parse the abbreviated syntax for name[:version]
	name, version, err := parseProviderName(provider)
	if err != nil {
		return nil, err
	}

	// If the version of the infrastructure provider to get templates from is empty, try to detect it.
	if version == "" {
		defaultProviderVersion, err := cluster.ProviderInventory().GetDefaultProviderVersion(name)
		if err != nil {
			return nil, err
		}

		if defaultProviderVersion == "" {
			return nil, errors.Errorf("failed to identify the default version for the provider %q. Please specify a version", name)
		}
		version = defaultProviderVersion
	}

	// Get the template from the template repository.
	providerConfig, err := c.configClient.Providers().Get(name)
	if err != nil {
		return nil, err
	}

	repo, err := c.repositoryClientFactory(providerConfig)
	if err != nil {
		return nil, err
	}

	template, err := repo.Templates(version).Get(source.Flavor, targetNamespace, listVariablesOnly)
	if err != nil {
		return nil, err
	}
	return template, nil
}

// getTemplateFromConfigMap returns a workload cluster template from a ConfigMap.
func (c *clusterctlClient) getTemplateFromConfigMap(cluster cluster.Client, source ConfigMapSourceOptions, targetNamespace string, listVariablesOnly bool) (Template, error) {
	// If the option specifying the configMapNamespace is empty, default it to the current namespace.
	if source.Namespace == "" {
		currentNamespace, err := cluster.Proxy().CurrentNamespace()
		if err != nil {
			return nil, err
		}
		source.Namespace = currentNamespace
	}

	// If the option specifying the configMapDataKey is empty, default it.
	if source.DataKey == "" {
		source.DataKey = DefaultCustomTemplateConfigMapKey
	}

	return cluster.Template().GetFromConfigMap(source.Namespace, source.Name, source.DataKey, targetNamespace, listVariablesOnly)
}

// getTemplateFromURL returns a workload cluster template from an URL.
func (c *clusterctlClient) getTemplateFromURL(cluster cluster.Client, source URLSourceOptions, targetNamespace string, listVariablesOnly bool) (Template, error) {
	return cluster.Template().GetFromURL(source.URL, targetNamespace, listVariablesOnly)
}

// templateOptionsToVariables injects some of the templateOptions to the configClient so they can be consumed as a variables from the template.
func (c *clusterctlClient) templateOptionsToVariables(options GetClusterTemplateOptions) error {

	// the TargetNamespace, if valid, can be used in templates using the ${ NAMESPACE } variable.
	if err := validateDNS1123Label(options.TargetNamespace); err != nil {
		return errors.Wrapf(err, "invalid target-namespace")
	}
	c.configClient.Variables().Set("NAMESPACE", options.TargetNamespace)

	// the ClusterName, if valid, can be used in templates using the ${ CLUSTER_NAME } variable.
	if err := validateDNS1123Label(options.ClusterName); err != nil {
		return errors.Wrapf(err, "invalid cluster name")
	}
	c.configClient.Variables().Set("CLUSTER_NAME", options.ClusterName)

	// the KubernetesVersion, if valid, can be used in templates using the ${ KUBERNETES_VERSION } variable.
	// NB. in case the KubernetesVersion from the templateOptions is empty, we are not setting any values so the
	// configClient is going to search into os env variables/the clusterctl config file as a fallback options.
	if options.KubernetesVersion != "" {
		if _, err := version.ParseSemantic(options.KubernetesVersion); err != nil {
			return errors.Errorf("invalid KubernetesVersion. Please use a semantic version number")
		}
		c.configClient.Variables().Set("KUBERNETES_VERSION", options.KubernetesVersion)
	}

	// the ControlPlaneMachineCount, if valid, can be used in templates using the ${ CONTROL_PLANE_MACHINE_COUNT } variable.
	if options.ControlPlaneMachineCount < 1 {
		return errors.Errorf("invalid ControlPlaneMachineCount. Please use a number greater or equal than 1")
	}
	c.configClient.Variables().Set("CONTROL_PLANE_MACHINE_COUNT", strconv.Itoa(options.ControlPlaneMachineCount))

	// the WorkerMachineCount, if valid, can be used in templates using the ${ WORKER_MACHINE_COUNT } variable.
	if options.WorkerMachineCount < 0 {
		return errors.Errorf("invalid WorkerMachineCount. Please use a number greater or equal than 0")
	}
	c.configClient.Variables().Set("WORKER_MACHINE_COUNT", strconv.Itoa(options.WorkerMachineCount))

	return nil
}
