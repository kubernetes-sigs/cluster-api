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
	"context"
	"sort"
	"time"

	"github.com/pkg/errors"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

// NoopProvider determines if a provider passed in should behave as a no-op.
const NoopProvider = "-"

// InitOptions carries the options supported by Init.
type InitOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// CoreProvider version (e.g. cluster-api:v1.1.5) to add to the management cluster. If unspecified, the
	// cluster-api core provider's latest release is used.
	CoreProvider string

	// BootstrapProviders and versions (e.g. kubeadm:v1.1.5) to add to the management cluster.
	// If unspecified, the kubeadm bootstrap provider's latest release is used.
	BootstrapProviders []string

	// InfrastructureProviders and versions (e.g. aws:v0.5.0) to add to the management cluster.
	InfrastructureProviders []string

	// ControlPlaneProviders and versions (e.g. kubeadm:v1.1.5) to add to the management cluster.
	// If unspecified, the kubeadm control plane provider latest release is used.
	ControlPlaneProviders []string

	// IPAMProviders and versions (e.g. infoblox:v0.0.1) to add to the management cluster.
	IPAMProviders []string

	// RuntimeExtensionProviders and versions (e.g. test:v0.0.1) to add to the management cluster.
	RuntimeExtensionProviders []string

	// AddonProviders and versions (e.g. helm:v0.1.0) to add to the management cluster.
	AddonProviders []string

	// TargetNamespace defines the namespace where the providers should be deployed. If unspecified, each provider
	// will be installed in a provider's default namespace.
	TargetNamespace string

	// LogUsageInstructions instructs the init command to print the usage instructions in case of first run.
	LogUsageInstructions bool

	// WaitProviders instructs the init command to wait till the providers are installed.
	WaitProviders bool

	// WaitProviderTimeout sets the timeout per provider wait installation
	WaitProviderTimeout time.Duration

	// SkipTemplateProcess allows for skipping the call to the template processor, including also variable replacement in the component YAML.
	// NOTE this works only if the rawYaml is a valid yaml by itself, like e.g when using envsubst/the simple processor.
	skipTemplateProcess bool

	// IgnoreValidationErrors allows for skipping the validation of provider installs.
	// NOTE this should only be used for development
	IgnoreValidationErrors bool

	// allowMissingProviderCRD is used to allow for a missing provider CRD when listing images.
	// It is set to false to enforce that provider CRD is available when performing the standard init operation.
	allowMissingProviderCRD bool
}

// Init initializes a management cluster by adding the requested list of providers.
func (c *clusterctlClient) Init(ctx context.Context, options InitOptions) ([]Components, error) {
	log := logf.Log

	// Default WaitProviderTimeout as we cannot rely on defaulting in the CLI
	// when clusterctl is used as a library.
	if options.WaitProviderTimeout.Nanoseconds() == 0 {
		options.WaitProviderTimeout = time.Duration(5*60) * time.Second
	}

	// gets access to the management cluster
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return nil, err
	}

	// ensure the custom resource definitions required by clusterctl are in place
	if err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions(ctx); err != nil {
		return nil, err
	}

	// Ensure this command only runs against v1beta1 management clusters
	if err := clusterClient.ProviderInventory().CheckCAPIContract(ctx, cluster.AllowCAPINotInstalled{}); err != nil {
		return nil, err
	}

	// checks if the cluster already contains a Core provider.
	// if not we consider this the first time init is executed, and thus we enforce the installation of a core provider,
	// a bootstrap provider and a control-plane provider (if not already explicitly requested by the user)
	log.Info("Fetching providers")
	firstRun := c.addDefaultProviders(ctx, clusterClient, &options)

	// create an installer service, add the requested providers to the install queue and then perform validation
	// of the target state of the management cluster before starting the installation.
	installer, err := c.setupInstaller(ctx, clusterClient, options)
	if err != nil {
		return nil, err
	}

	// Before installing the providers, validates the management cluster resulting by the planned installation. The following checks are performed:
	// - There should be only one instance of the same provider.
	// - All the providers must support the same API Version of Cluster API (contract)
	// - All provider CRDs that are referenced in core Cluster API CRDs must comply with the CRD naming scheme,
	//   otherwise a warning is logged.
	if err := installer.Validate(ctx); err != nil {
		if !options.IgnoreValidationErrors {
			return nil, err
		}
		log.Error(err, "Ignoring validation errors")
	}

	// Before installing the providers, ensure the cert-manager Webhook is in place.
	certManager := clusterClient.CertManager()
	if err := certManager.EnsureInstalled(ctx); err != nil {
		return nil, err
	}

	installOpts := cluster.InstallOptions{
		WaitProviders:       options.WaitProviders,
		WaitProviderTimeout: options.WaitProviderTimeout,
	}
	components, err := installer.Install(ctx, installOpts)
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
		log.Info("  clusterctl generate cluster [name] --kubernetes-version [version] | kubectl apply -f -")
		log.Info("")
	}

	// Components is an alias for repository.Components; this makes the conversion from the two types
	aliasComponents := make([]Components, len(components))
	for i, components := range components {
		aliasComponents[i] = components
	}
	return aliasComponents, nil
}

// InitImages returns the list of images required for init.
func (c *clusterctlClient) InitImages(ctx context.Context, options InitOptions) ([]string, error) {
	// gets access to the management cluster
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return nil, err
	}

	// Ensure this command only runs against empty management clusters or v1beta1 management clusters.
	if err := clusterClient.ProviderInventory().CheckCAPIContract(ctx, cluster.AllowCAPINotInstalled{}); err != nil {
		return nil, err
	}

	// checks if the cluster already contains a Core provider.
	// if not we consider this the first time init is executed, and thus we enforce the installation of a core provider,
	// a bootstrap provider and a control-plane provider (if not already explicitly requested by the user)
	c.addDefaultProviders(ctx, clusterClient, &options)

	// skip variable parsing when listing images
	options.skipTemplateProcess = true

	options.allowMissingProviderCRD = true

	// create an installer service, add the requested providers to the install queue and then perform validation
	// of the target state of the management cluster before starting the installation.
	installer, err := c.setupInstaller(ctx, clusterClient, options)
	if err != nil {
		return nil, err
	}

	// Gets the list of container images required for the cert-manager (if not already installed).
	certManager := clusterClient.CertManager()
	images, err := certManager.Images(ctx)
	if err != nil {
		return nil, err
	}

	// Appends the list of container images required for the selected providers.
	images = append(images, installer.Images()...)

	sort.Strings(images)
	return images, nil
}

func (c *clusterctlClient) setupInstaller(ctx context.Context, cluster cluster.Client, options InitOptions) (cluster.ProviderInstaller, error) {
	installer := cluster.ProviderInstaller()

	providerList := &clusterctlv1.ProviderList{}

	addOptions := addToInstallerOptions{
		installer:           installer,
		targetNamespace:     options.TargetNamespace,
		skipTemplateProcess: options.skipTemplateProcess,
		providerList:        providerList,
	}

	if !options.allowMissingProviderCRD {
		providerList, err := cluster.ProviderInventory().List(ctx)
		if err != nil {
			return nil, err
		}

		addOptions.providerList = providerList
	}

	if options.CoreProvider != "" {
		if err := c.addToInstaller(ctx, addOptions, clusterctlv1.CoreProviderType, options.CoreProvider); err != nil {
			return nil, err
		}
	}

	if err := c.addToInstaller(ctx, addOptions, clusterctlv1.BootstrapProviderType, options.BootstrapProviders...); err != nil {
		return nil, err
	}

	if err := c.addToInstaller(ctx, addOptions, clusterctlv1.ControlPlaneProviderType, options.ControlPlaneProviders...); err != nil {
		return nil, err
	}

	if err := c.addToInstaller(ctx, addOptions, clusterctlv1.InfrastructureProviderType, options.InfrastructureProviders...); err != nil {
		return nil, err
	}

	if err := c.addToInstaller(ctx, addOptions, clusterctlv1.IPAMProviderType, options.IPAMProviders...); err != nil {
		return nil, err
	}

	if err := c.addToInstaller(ctx, addOptions, clusterctlv1.RuntimeExtensionProviderType, options.RuntimeExtensionProviders...); err != nil {
		return nil, err
	}

	if err := c.addToInstaller(ctx, addOptions, clusterctlv1.AddonProviderType, options.AddonProviders...); err != nil {
		return nil, err
	}

	return installer, nil
}

func (c *clusterctlClient) addDefaultProviders(ctx context.Context, cluster cluster.Client, options *InitOptions) bool {
	firstRun := false
	// Check if there is already a core provider installed in the cluster
	// Nb. we are ignoring the error so this operation can support listing images even if there is no an existing management cluster;
	// in case there is no an existing management cluster, we assume there are no core providers installed in the cluster.
	currentCoreProvider, _ := cluster.ProviderInventory().GetDefaultProviderName(ctx, clusterctlv1.CoreProviderType)

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
	installer           cluster.ProviderInstaller
	targetNamespace     string
	skipTemplateProcess bool
	providerList        *clusterctlv1.ProviderList
}

// addToInstaller adds the components to the install queue and checks that the actual provider type match the target group.
func (c *clusterctlClient) addToInstaller(ctx context.Context, options addToInstallerOptions, providerType clusterctlv1.ProviderType, providers ...string) error {
	for _, provider := range providers {
		// It is possible to opt-out from automatic installation of bootstrap/control-plane providers using '-' as a provider name (NoopProvider).
		if provider == NoopProvider {
			if providerType == clusterctlv1.CoreProviderType {
				return errors.New("the '-' value can not be used for the core provider")
			}
			continue
		}
		componentsOptions := repository.ComponentsOptions{
			TargetNamespace:     options.targetNamespace,
			SkipTemplateProcess: options.skipTemplateProcess,
		}
		components, err := c.getComponentsByName(ctx, provider, providerType, componentsOptions)
		if err != nil {
			return errors.Wrapf(err, "failed to get provider components for the %q provider", provider)
		}

		if components.Type() != providerType {
			return errors.Errorf("can't use %q provider as an %q, it is a %q", provider, providerType, components.Type())
		}

		// If a provider of the same name, type and version already exists in the Cluster skip adding it to the installer.
		matchingProviders := options.providerList.FilterByProviderNameNamespaceTypeVersion(
			components.Name(),
			components.TargetNamespace(),
			components.Type(),
			components.Version(),
		)
		if len(matchingProviders) != 0 {
			continue
		}

		options.installer.Add(components)
	}
	return nil
}
