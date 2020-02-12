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
	"sort"

	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/log"
)

const NoopProvider = "-"

// Init initializes a management cluster by adding the requested list of providers.
func (c *clusterctlClient) Init(options InitOptions) ([]Components, error) {
	log := logf.Log

	// gets access to the management cluster
	cluster, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return nil, err
	}

	// ensure the custom resource definitions required by clusterctl are in place
	if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	// checks if the cluster already contains a Core provider.
	// if not we consider this the first time init is executed, and thus we enforce the installation of a core provider,
	// a bootstrap provider and a control-plane provider (if not already explicitly requested by the user)
	log.Info("Fetching providers")
	firstRun := c.addDefaultProviders(cluster, &options)

	// create an installer service, add the requested providers to the install queue and then perform validation
	// of the target state of the management cluster before starting the installation.
	installer, err := c.setupInstaller(cluster, options)
	if err != nil {
		return nil, err
	}

	// Before installing the providers, validates the management cluster resulting by the planned installation. The following checks are performed:
	// - There should be only one instance of the same provider per namespace.
	// - Instances of the same provider should not be fighting for objects (no watching overlap).
	// - Providers combines in valid management groups
	//   - All the providers should belong to one/only one management groups
	//   - All the providers in a management group must support the same API Version of Cluster API (contract)
	if err := installer.Validate(); err != nil {
		return nil, err
	}

	// Before installing the providers, ensure the cert-manager WebHook is in place.
	if err := cluster.CertManager().EnsureWebHook(); err != nil {
		return nil, err
	}

	components, err := installer.Install()
	if err != nil {
		return nil, err
	}

	// If this is the firstRun, then log the usage instructions.
	if firstRun && options.LogUsageInstructions {
		log.Info("")
		log.Info("Your management cluster has been initialized successfully!")
		log.Info("")
		log.Info("You can now create your first workload cluster by running the following:")
		log.Info("")
		log.Info("  clusterctl config cluster [name] --kubernetes-version [version] | kubectl apply -f -")
		log.Info("")
	}

	// Components is an alias for repository.Components; this makes the conversion from the two types
	aliasComponents := make([]Components, len(components))
	for i, components := range components {
		aliasComponents[i] = components
	}
	return aliasComponents, nil
}

// Init returns the list of images required for init.
func (c *clusterctlClient) InitImages(options InitOptions) ([]string, error) {
	// gets access to the management cluster
	cluster, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return nil, err
	}

	// checks if the cluster already contains a Core provider.
	// if not we consider this the first time init is executed, and thus we enforce the installation of a core provider,
	// a bootstrap provider and a control-plane provider (if not already explicitly requested by the user)
	c.addDefaultProviders(cluster, &options)

	// create an installer service, add the requested providers to the install queue and then perform validation
	// of the target state of the management cluster before starting the installation.
	installer, err := c.setupInstaller(cluster, options)
	if err != nil {
		return nil, err
	}

	// Gets the list of container images required for the cert-manager (if not already installed).
	images, err := cluster.CertManager().Images()
	if err != nil {
		return nil, err
	}

	// Appends the list of container images required for the selected providers.
	images = append(images, installer.Images()...)

	sort.Strings(images)
	return images, nil
}

func (c *clusterctlClient) setupInstaller(cluster cluster.Client, options InitOptions) (cluster.ProviderInstaller, error) {
	installer := cluster.ProviderInstaller()

	addOptions := addToInstallerOptions{
		installer:         installer,
		targetNamespace:   options.TargetNamespace,
		watchingNamespace: options.WatchingNamespace,
	}

	if options.CoreProvider != "" {
		if err := c.addToInstaller(addOptions, clusterctlv1.CoreProviderType, options.CoreProvider); err != nil {
			return nil, err
		}
	}

	if err := c.addToInstaller(addOptions, clusterctlv1.BootstrapProviderType, options.BootstrapProviders...); err != nil {
		return nil, err
	}

	if err := c.addToInstaller(addOptions, clusterctlv1.ControlPlaneProviderType, options.ControlPlaneProviders...); err != nil {
		return nil, err
	}

	if err := c.addToInstaller(addOptions, clusterctlv1.InfrastructureProviderType, options.InfrastructureProviders...); err != nil {
		return nil, err
	}

	return installer, nil
}

func (c *clusterctlClient) addDefaultProviders(cluster cluster.Client, options *InitOptions) bool {
	firstRun := false
	// Check if there is already a core provider installed in the cluster
	// Nb. we are ignoring the error so this operation can support listing images even if there is no an existing management cluster;
	// in case there is no an existing management cluster, we assume there are no core providers installed in the cluster.
	currentCoreProvider, _ := cluster.ProviderInventory().GetDefaultProviderName(clusterctlv1.CoreProviderType)

	// If there are no core providers installed in the cluster, consider this a first run and add default providers to the list
	// of providers to be installed.
	if currentCoreProvider == "" {
		firstRun = true
		if options.CoreProvider == "" {
			options.CoreProvider = config.ClusterAPIProviderName
		}
		if len(options.BootstrapProviders) == 0 {
			options.BootstrapProviders = append(options.BootstrapProviders, config.KubeadmBootstrapProviderName)
		}
		if len(options.ControlPlaneProviders) == 0 {
			options.ControlPlaneProviders = append(options.ControlPlaneProviders, config.KubeadmControlPlaneProviderName)
		}
	}
	return firstRun
}

type addToInstallerOptions struct {
	installer         cluster.ProviderInstaller
	targetNamespace   string
	watchingNamespace string
}

// addToInstaller adds the components to the install queue and checks that the actual provider type match the target group
func (c *clusterctlClient) addToInstaller(options addToInstallerOptions, targetGroup clusterctlv1.ProviderType, providers ...string) error {
	for _, provider := range providers {
		// It is possible to opt-out from automatic installation of bootstrap/controlPlane providers using '-' as a provider name (NoopProvider).
		if provider == NoopProvider {
			if targetGroup == clusterctlv1.CoreProviderType {
				return errors.New("the '-' value can not be used for the core provider")
			}
			continue
		}

		components, err := c.getComponentsByName(provider, options.targetNamespace, options.watchingNamespace)
		if err != nil {
			return errors.Wrapf(err, "failed to get provider components for the %q provider", provider)
		}

		if components.Type() != targetGroup {
			return errors.Errorf("can't use %q provider as an %q, it is a %q", provider, targetGroup, components.Type())
		}

		options.installer.Add(components)
	}
	return nil
}
