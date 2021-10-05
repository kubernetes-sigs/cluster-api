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
	"io"
	"strconv"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/utils/pointer"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
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

func (c *clusterctlClient) GetProviderComponents(provider string, providerType clusterctlv1.ProviderType, options ComponentsOptions) (Components, error) {
	components, err := c.getComponentsByName(provider, providerType, repository.ComponentsOptions(options))
	if err != nil {
		return nil, err
	}

	return components, nil
}

// ReaderSourceOptions define the options to be used when reading a template
// from an arbitrary reader.
type ReaderSourceOptions struct {
	Reader io.Reader
}

// ProcessYAMLOptions are the options supported by ProcessYAML.
type ProcessYAMLOptions struct {
	ReaderSource *ReaderSourceOptions
	// URLSource to be used for reading the template
	URLSource *URLSourceOptions

	// SkipTemplateProcess return the list of variables expected by the template
	// without executing any further processing.
	SkipTemplateProcess bool
}

func (c *clusterctlClient) ProcessYAML(options ProcessYAMLOptions) (YamlPrinter, error) {
	if options.ReaderSource != nil {
		// NOTE: Beware of potentially reading in large files all at once
		// since this is inefficient and increases memory utilziation.
		content, err := io.ReadAll(options.ReaderSource.Reader)
		if err != nil {
			return nil, err
		}
		return repository.NewTemplate(repository.TemplateInput{
			RawArtifact:           content,
			ConfigVariablesClient: c.configClient.Variables(),
			Processor:             yaml.NewSimpleProcessor(),
			TargetNamespace:       "",
			SkipTemplateProcess:   options.SkipTemplateProcess,
		})
	}

	// Technically we do not need to connect to the cluster. However, we are
	// leveraging the template client which exposes GetFromURL() is available
	// on the cluster client so we create a cluster client with default
	// configs to access it.
	clstr, err := c.clusterClientFactory(
		ClusterClientFactoryInput{
			// use the default kubeconfig
			Kubeconfig: Kubeconfig{},
		},
	)
	if err != nil {
		return nil, err
	}

	if options.URLSource != nil {
		return c.getTemplateFromURL(clstr, *options.URLSource, "", options.SkipTemplateProcess)
	}

	return nil, errors.New("unable to read custom template. Please specify a template source")
}

// GetClusterTemplateOptions carries the options supported by GetClusterTemplate.
type GetClusterTemplateOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// ProviderRepositorySource to be used for reading the workload cluster template from a provider repository;
	// only one template source can be used at time; if not other source will be set, a ProviderRepositorySource
	// will be generated inferring values from the cluster.
	ProviderRepositorySource *ProviderRepositorySourceOptions

	// URLSource to be used for reading the workload cluster template; only one template source can be used at time.
	URLSource *URLSourceOptions

	// ConfigMapSource to be used for reading the workload cluster template; only one template source can be used at time.
	ConfigMapSource *ConfigMapSourceOptions

	// TargetNamespace where the objects describing the workload cluster should be deployed. If unspecified,
	// the current namespace will be used.
	TargetNamespace string

	// ClusterName to be used for the workload cluster.
	ClusterName string

	// KubernetesVersion to use for the workload cluster. If unspecified, the value from os env variables
	// or the .cluster-api/clusterctl.yaml config file will be used.
	KubernetesVersion string

	// ControlPlaneMachineCount defines the number of control plane machines to be added to the workload cluster.
	// It can be set through the cli flag, CONTROL_PLANE_MACHINE_COUNT environment variable or will default to 1
	ControlPlaneMachineCount *int64

	// WorkerMachineCount defines number of worker machines to be added to the workload cluster.
	// It can be set through the cli flag, WORKER_MACHINE_COUNT environment variable or will default to 0
	WorkerMachineCount *int64

	// ListVariablesOnly sets the GetClusterTemplate method to return the list of variables expected by the template
	// without executing any further processing.
	ListVariablesOnly bool

	// YamlProcessor defines the yaml processor to use for the cluster
	// template processing. If not defined, SimpleProcessor will be used.
	YamlProcessor Processor
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
	// InfrastructureProvider to read the workload cluster template from. If unspecified, the default
	// infrastructure provider will be used if no other sources are specified.
	InfrastructureProvider string

	// Flavor defines The workload cluster template variant to be used when reading from the infrastructure
	// provider repository. If unspecified, the default cluster template will be used.
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
	// Namespace where the ConfigMap exists. If unspecified, the current namespace will be used.
	Namespace string

	// Name to read the workload cluster template from.
	Name string

	// DataKey where the workload cluster template is hosted. If unspecified, the
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
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{options.Kubeconfig, options.YamlProcessor})
	if err != nil {
		return nil, err
	}

	// If the option specifying the targetNamespace is empty, try to detect it.
	if options.TargetNamespace == "" {
		if err := clusterClient.Proxy().CheckClusterAvailable(); err != nil {
			return nil, errors.Wrap(err, "management cluster not available. Cannot auto-discover target namespace. Please specify a target namespace")
		}
		currentNamespace, err := clusterClient.Proxy().CurrentNamespace()
		if err != nil {
			return nil, err
		}
		if currentNamespace == "" {
			return nil, errors.New("failed to identify the current namespace. Please specify a target namespace")
		}
		options.TargetNamespace = currentNamespace
	}

	// Inject some of the templateOptions into the configClient so they can be consumed as a variables from the template.
	if err := c.templateOptionsToVariables(options); err != nil {
		return nil, err
	}

	// Gets the workload cluster template from the selected source
	if options.ProviderRepositorySource != nil {
		// Ensure this command only runs against management clusters with the current Cluster API contract.
		// NOTE: This command tolerates also not existing cluster (Kubeconfig.Path=="") or clusters not yet initialized in order to allow
		// users to dry-run the command and take a look at what the cluster will look like; in both scenarios, it is required
		// to pass provider:version given that auto-discovery can't work without a provider inventory installed in a cluster.
		if options.Kubeconfig.Path != "" {
			if err := clusterClient.ProviderInventory().CheckCAPIContract(cluster.AllowCAPINotInstalled{}); err != nil {
				return nil, err
			}
		}
		return c.getTemplateFromRepository(clusterClient, options)
	}
	if options.ConfigMapSource != nil {
		return c.getTemplateFromConfigMap(clusterClient, *options.ConfigMapSource, options.TargetNamespace, options.ListVariablesOnly)
	}
	if options.URLSource != nil {
		return c.getTemplateFromURL(clusterClient, *options.URLSource, options.TargetNamespace, options.ListVariablesOnly)
	}

	return nil, errors.New("unable to read custom template. Please specify a template source")
}

// getTemplateFromRepository returns a workload cluster template from a provider repository.
func (c *clusterctlClient) getTemplateFromRepository(cluster cluster.Client, options GetClusterTemplateOptions) (Template, error) {
	source := *options.ProviderRepositorySource
	targetNamespace := options.TargetNamespace
	listVariablesOnly := options.ListVariablesOnly
	processor := options.YamlProcessor

	// If the option specifying the name of the infrastructure provider to get templates from is empty, try to detect it.
	provider := source.InfrastructureProvider
	ensureCustomResourceDefinitions := false
	if provider == "" {
		if err := cluster.Proxy().CheckClusterAvailable(); err != nil {
			return nil, errors.Wrap(err, "management cluster not available. Cannot auto-discover default infrastructure provider. Please specify an infrastructure provider")
		}
		// ensure the custom resource definitions required by clusterctl are in place
		if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
			return nil, errors.Wrapf(err, "provider custom resource definitions (CRDs) are not installed")
		}
		ensureCustomResourceDefinitions = true

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
		if err := cluster.Proxy().CheckClusterAvailable(); err != nil {
			return nil, errors.Wrapf(err, "management cluster not available. Cannot auto-discover version for the provider %q automatically. Please specify a version", name)
		}
		// ensure the custom resource definitions required by clusterctl are in place (if not already done)
		if !ensureCustomResourceDefinitions {
			if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
				return nil, errors.Wrapf(err, "failed to identify the default version for the provider %q. Please specify a version", name)
			}
		}

		inventoryVersion, err := cluster.ProviderInventory().GetProviderVersion(name, clusterctlv1.InfrastructureProviderType)
		if err != nil {
			return nil, err
		}

		if inventoryVersion == "" {
			return nil, errors.Errorf("Unable to identify version for the provider %q automatically. Please specify a version", name)
		}
		version = inventoryVersion
	}

	// Get the template from the template repository.
	providerConfig, err := c.configClient.Providers().Get(name, clusterctlv1.InfrastructureProviderType)
	if err != nil {
		return nil, err
	}

	repo, err := c.repositoryClientFactory(RepositoryClientFactoryInput{Provider: providerConfig, Processor: processor})
	if err != nil {
		return nil, err
	}

	template, err := repo.Templates(version).Get(source.Flavor, targetNamespace, listVariablesOnly)
	if err != nil {
		return nil, err
	}

	clusterClassClient := repo.ClusterClasses(version)

	template, err = addClusterClassIfMissing(template, clusterClassClient, cluster, targetNamespace, listVariablesOnly)
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
	if err := validateDNS1123Domanin(options.ClusterName); err != nil {
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
	if options.ControlPlaneMachineCount == nil {
		// Check if set through env variable and default to 1 otherwise
		if v, err := c.configClient.Variables().Get("CONTROL_PLANE_MACHINE_COUNT"); err != nil {
			options.ControlPlaneMachineCount = pointer.Int64Ptr(1)
		} else {
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return errors.Errorf("invalid value for CONTROL_PLANE_MACHINE_COUNT set")
			}
			options.ControlPlaneMachineCount = &i
		}
	}
	if *options.ControlPlaneMachineCount < 1 {
		return errors.Errorf("invalid ControlPlaneMachineCount. Please use a number greater or equal than 1")
	}
	c.configClient.Variables().Set("CONTROL_PLANE_MACHINE_COUNT", strconv.FormatInt(*options.ControlPlaneMachineCount, 10))

	// the WorkerMachineCount, if valid, can be used in templates using the ${ WORKER_MACHINE_COUNT } variable.
	if options.WorkerMachineCount == nil {
		// Check if set through env variable and default to 0 otherwise
		if v, err := c.configClient.Variables().Get("WORKER_MACHINE_COUNT"); err != nil {
			options.WorkerMachineCount = pointer.Int64Ptr(0)
		} else {
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return errors.Errorf("invalid value for WORKER_MACHINE_COUNT set")
			}
			options.WorkerMachineCount = &i
		}
	}
	if *options.WorkerMachineCount < 0 {
		return errors.Errorf("invalid WorkerMachineCount. Please use a number greater or equal than 0")
	}
	c.configClient.Variables().Set("WORKER_MACHINE_COUNT", strconv.FormatInt(*options.WorkerMachineCount, 10))

	return nil
}
