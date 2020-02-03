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

func (c *clusterctlClient) GetClusterTemplate(options GetClusterTemplateOptions) (Template, error) {

	cluster, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return nil, err
	}

	if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	// If the option specifying the name of the infrastructure provider to get templates from is empty, try to detect it.
	provider := options.InfrastructureProvider
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

	// Get the template from the template repository.
	providerConfig, err := c.configClient.Providers().Get(name)
	if err != nil {
		return nil, err
	}

	repo, err := c.repositoryClientFactory(providerConfig)
	if err != nil {
		return nil, err
	}

	template, err := repo.Templates(version).Get(options.Flavor, options.BootstrapProvider, options.TargetNamespace)
	if err != nil {
		return nil, err
	}
	return template, nil
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
