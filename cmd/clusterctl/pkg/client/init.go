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
	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// Init initializes a management cluster by adding the requested list of providers.
func (c *clusterctlClient) Init(options InitOptions) ([]Components, bool, error) {
	// gets access to the management cluster
	cluster, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return nil, false, err
	}

	// Ensure the cert-manager WebHook is in place.
	if err := cluster.CertManger().EnsureWebHook(); err != nil {
		return nil, false, err
	}

	// ensure the custom resource definitions required by clusterctl are in place
	if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return nil, false, err
	}

	// checks if the cluster already contains a Core provider.
	// if not we consider this the first time init is executed, and thus we enforce the installation of a core provider (if not already explicitly requested by the user)
	firstRun := false
	currentCoreProvider, err := cluster.ProviderInventory().GetDefaultProviderName(clusterctlv1.CoreProviderType)
	if err != nil {
		return nil, false, err
	}
	if currentCoreProvider == "" {
		firstRun = true
		if options.CoreProvider == "" {
			options.CoreProvider = config.ClusterAPIName
		}
		if len(options.BootstrapProviders) == 0 {
			options.BootstrapProviders = append(options.BootstrapProviders, config.KubeadmBootstrapProviderName)
		}
	}

	// create an installer service, add the requested providers to the install queue (thus performing validation of the target state of the management cluster
	// before starting the installation), and then perform the installation.
	installer := cluster.ProviderInstaller()

	addOptions := addToInstallerOptions{
		installer:         installer,
		targetNameSpace:   options.TargetNameSpace,
		watchingNamespace: options.WatchingNamespace,
		force:             options.Force,
	}

	if options.CoreProvider != "" {
		if err := c.addToInstaller(addOptions, clusterctlv1.CoreProviderType, options.CoreProvider); err != nil {
			return nil, false, err
		}
	}

	if err := c.addToInstaller(addOptions, clusterctlv1.BootstrapProviderType, options.BootstrapProviders...); err != nil {
		return nil, false, err
	}

	if err := c.addToInstaller(addOptions, clusterctlv1.InfrastructureProviderType, options.InfrastructureProviders...); err != nil {
		return nil, false, err
	}

	components, err := installer.Install()
	if err != nil {
		return nil, false, err
	}

	// Components is an alias for repository.Components; this makes the conversion from the two types
	aliasComponents := make([]Components, len(components))
	for i, components := range components {
		aliasComponents[i] = components
	}
	return aliasComponents, firstRun, nil
}

type addToInstallerOptions struct {
	installer         cluster.ProviderInstaller
	targetNameSpace   string
	watchingNamespace string
	force             bool
}

// addToInstaller adds the components to the install queue and checks that the actual provider type match the target group
func (c *clusterctlClient) addToInstaller(options addToInstallerOptions, targetGroup clusterctlv1.ProviderType, providers ...string) error {
	for _, provider := range providers {
		components, err := c.getComponentsByName(provider, options.targetNameSpace, options.watchingNamespace)
		if err != nil {
			return err
		}

		if components.Type() != targetGroup {
			return errors.Errorf("can't use %q provider as an %q, it is a %q", provider, targetGroup, components.Type())
		}

		if err := options.installer.Add(components, options.force); err != nil {
			return err
		}
	}
	return nil
}
