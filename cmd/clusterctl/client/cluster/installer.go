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

package cluster

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/version"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

// ProviderInstaller defines methods for enforcing consistency rules for provider installation.
type ProviderInstaller interface {
	// Add adds a provider to the install queue.
	// NB. By deferring the installation, the installer service can perform validation of the target state of the management cluster
	// before actually starting the installation of new providers.
	Add(repository.Components)

	// Install performs the installation of the providers ready in the install queue.
	Install() ([]repository.Components, error)

	// Validate performs steps to validate a management cluster by looking at the current state and the providers in the queue.
	// The following checks are performed in order to ensure a fully operational cluster:
	// - There must be only one instance of the same provider per namespace
	// - Instances of the same provider must not be fighting for objects (no watching overlap)
	// - Providers must combine in valid management groups
	//   - All the providers must belong to one/only one management groups
	//   - All the providers in a management group must support the same API Version of Cluster API (contract)
	Validate() error

	// Images returns the list of images required for installing the providers ready in the install queue.
	Images() []string
}

// providerInstaller implements ProviderInstaller
type providerInstaller struct {
	configClient            config.Client
	repositoryClientFactory RepositoryClientFactory
	proxy                   Proxy
	providerComponents      ComponentsClient
	providerInventory       InventoryClient
	installQueue            []repository.Components
}

var _ ProviderInstaller = &providerInstaller{}

func (i *providerInstaller) Add(components repository.Components) {
	i.installQueue = append(i.installQueue, components)
}

func (i *providerInstaller) Install() ([]repository.Components, error) {
	ret := make([]repository.Components, 0, len(i.installQueue))
	for _, components := range i.installQueue {
		if err := installComponentsAndUpdateInventory(components, i.providerComponents, i.providerInventory); err != nil {
			return nil, err
		}

		ret = append(ret, components)
	}
	return ret, nil
}

func installComponentsAndUpdateInventory(components repository.Components, providerComponents ComponentsClient, providerInventory InventoryClient) error {
	log := logf.Log
	log.Info("Installing", "Provider", components.ManifestLabel(), "Version", components.Version(), "TargetNamespace", components.TargetNamespace())

	inventoryObject := components.InventoryObject()

	// Check the list of providers currently in the cluster and decide if to install shared components (CRDs, web-hooks) or not.
	// We are required to install shared components in two cases:
	// - when this is the first instance of the provider being installed.
	// - when the version of the provider being installed is newer than the max version already installed in the cluster.
	// Nb. this assumes the newer version of shared components are fully retro-compatible.
	providerList, err := providerInventory.List()
	if err != nil {
		return err
	}

	installSharedComponents, err := shouldInstallSharedComponents(providerList, inventoryObject)
	if err != nil {
		return err
	}
	if installSharedComponents {
		log.V(1).Info("Creating shared objects", "Provider", components.ManifestLabel(), "Version", components.Version())
		// TODO: currently shared components overrides existing shared components. As a future improvement we should
		//  consider if to delete (preserving CRDs) before installing so there will be no left-overs in case the list of resources changes
		if err := providerComponents.Create(components.SharedObjs()); err != nil {
			return err
		}
	} else {
		log.V(1).Info("Shared objects already up to date", "Provider", components.ManifestLabel())
	}

	// Then always install the instance specific objects and the then inventory item for the provider

	log.V(1).Info("Creating instance objects", "Provider", components.ManifestLabel(), "Version", components.Version(), "TargetNamespace", components.TargetNamespace())
	if err := providerComponents.Create(components.InstanceObjs()); err != nil {
		return err
	}

	log.V(1).Info("Creating inventory entry", "Provider", components.ManifestLabel(), "Version", components.Version(), "TargetNamespace", components.TargetNamespace())
	if err := providerInventory.Create(inventoryObject); err != nil {
		return err
	}

	return nil
}

// shouldInstallSharedComponents checks if it is required to install shared components for a provider.
func shouldInstallSharedComponents(providerList *clusterctlv1.ProviderList, provider clusterctlv1.Provider) (bool, error) {
	// Get the max version of the provider already installed in the cluster.
	var maxVersion *version.Version
	for _, other := range providerList.FilterByProviderNameAndType(provider.ProviderName, provider.GetProviderType()) {
		otherVersion, err := version.ParseSemantic(other.Version)
		if err != nil {
			return false, errors.Wrapf(err, "failed to parse version for the %s provider", other.InstanceName())
		}
		if maxVersion == nil || otherVersion.AtLeast(maxVersion) {
			maxVersion = otherVersion
		}
	}
	// If there is no max version, this is the first instance of the provider being installed, so it is required
	// to install the shared components.
	if maxVersion == nil {
		return true, nil
	}

	// If the installed version is newer or equal than than the version of the provider being installed,
	// return false because we should not down grade the shared components.
	providerVersion, err := version.ParseSemantic(provider.Version)
	if err != nil {
		return false, errors.Wrapf(err, "failed to parse version for the %s provider", provider.InstanceName())
	}
	if maxVersion.AtLeast(providerVersion) {
		return false, nil
	}

	// Otherwise, the version of the provider being installed is newer that the current max version, so it is
	// required to install also the new version of shared components.
	return true, nil
}

func (i *providerInstaller) Validate() error {
	// Get the list of providers currently in the cluster.
	providerList, err := i.providerInventory.List()
	if err != nil {
		return err
	}

	// Starts simulating what will be the resulting management cluster by adding to the list the providers in the installQueue.
	// During this operation following checks are performed:
	// - There must be only one instance of the same provider per namespace
	// - Instances of the same provider must not be fighting for objects (no watching overlap)
	for _, components := range i.installQueue {
		if providerList, err = simulateInstall(providerList, components); err != nil {
			return errors.Wrapf(err, "installing provider %q can lead to a non functioning management cluster", components.ManifestLabel())
		}
	}

	// Now that the provider list contains all the providers that are scheduled for install, gets the resulting management groups.
	// During this operation following check is performed:
	// - Providers must combine in valid management groups
	//   - All the providers must belong to one/only one management group
	managementGroups, err := deriveManagementGroups(providerList)
	if err != nil {
		return err
	}

	// Checks if all the providers supports the same API Version of Cluster API (contract) of the corresponding management group.
	providerInstanceContracts := map[string]string{}
	for _, components := range i.installQueue {
		provider := components.InventoryObject()

		// Gets the management group the providers belongs to, and then retrieve the API Version of Cluster API (contract)
		// all the providers in the management group must support.
		managementGroup := managementGroups.FindManagementGroupByProviderInstanceName(provider.InstanceName())
		managementGroupContract, err := i.getProviderContract(providerInstanceContracts, managementGroup.CoreProvider)
		if err != nil {
			return err
		}

		// Gets the API Version of Cluster API (contract) the provider support and compare it with the  management group contract.
		providerContract, err := i.getProviderContract(providerInstanceContracts, provider)
		if err != nil {
			return err
		}
		if providerContract != managementGroupContract {
			return errors.Errorf("installing provider %q can lead to a non functioning management cluster: the target version for the provider supports the %s API Version of Cluster API (contract), while the management group is using %s", components.ManifestLabel(), providerContract, managementGroupContract)
		}
	}
	return nil
}

// getProviderContract returns the API Version of Cluster API (contract) for a provider instance.
func (i *providerInstaller) getProviderContract(providerInstanceContracts map[string]string, provider clusterctlv1.Provider) (string, error) {
	// If the contract for the provider instance is already known, return it.
	if contract, ok := providerInstanceContracts[provider.InstanceName()]; ok {
		return contract, nil
	}

	// Otherwise get the contract for the providers instance.

	// Gets the providers metadata.
	configRepository, err := i.configClient.Providers().Get(provider.ProviderName, provider.GetProviderType())
	if err != nil {
		return "", err
	}

	providerRepository, err := i.repositoryClientFactory(configRepository, i.configClient)
	if err != nil {
		return "", err
	}

	latestMetadata, err := providerRepository.Metadata(provider.Version).Get()
	if err != nil {
		return "", err
	}

	// Gets the contract for the current release.
	currentVersion, err := version.ParseSemantic(provider.Version)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse current version for the %s provider", provider.InstanceName())
	}

	releaseSeries := latestMetadata.GetReleaseSeriesForVersion(currentVersion)
	if releaseSeries == nil {
		return "", errors.Errorf("invalid provider metadata: version %s for the provider %s does not match any release series", provider.Version, provider.InstanceName())
	}

	if releaseSeries.Contract != clusterv1.GroupVersion.Version {
		return "", errors.Errorf("current version of clusterctl could install only %s providers, detected %s for provider %s", clusterv1.GroupVersion.Version, releaseSeries.Contract, provider.ManifestLabel())
	}

	providerInstanceContracts[provider.InstanceName()] = releaseSeries.Contract
	return releaseSeries.Contract, nil
}

// simulateInstall adds a provider to the list of providers in a cluster (without installing it).
func simulateInstall(providerList *clusterctlv1.ProviderList, components repository.Components) (*clusterctlv1.ProviderList, error) {
	provider := components.InventoryObject()

	existingInstances := providerList.FilterByProviderNameAndType(provider.ProviderName, provider.GetProviderType())

	// Target Namespace check
	// Installing two instances of the same provider in the same namespace won't be supported
	for _, i := range existingInstances {
		if i.Namespace == provider.Namespace {
			return providerList, errors.Errorf("there is already an instance of the %q provider installed in the %q namespace", provider.ManifestLabel(), provider.Namespace)
		}
	}

	// Watching Namespace check:
	// If we are going to install an instance of a provider watching objects in namespaces already controlled by other providers
	// then there will be providers fighting for objects...
	for _, i := range existingInstances {
		if i.HasWatchingOverlapWith(provider) {
			return providerList, errors.Errorf("the new instance of the %q provider is going to watch for objects in the namespace %q that is already controlled by other instances of the same provider", provider.ManifestLabel(), provider.WatchedNamespace)
		}
	}

	providerList.Items = append(providerList.Items, provider)

	return providerList, nil
}

func (i *providerInstaller) Images() []string {
	ret := sets.NewString()
	for _, components := range i.installQueue {
		ret = ret.Insert(components.Images()...)
	}
	return ret.List()
}

func newProviderInstaller(configClient config.Client, repositoryClientFactory RepositoryClientFactory, proxy Proxy, providerMetadata InventoryClient, providerComponents ComponentsClient) *providerInstaller {
	return &providerInstaller{
		configClient:            configClient,
		repositoryClientFactory: repositoryClientFactory,
		proxy:                   proxy,
		providerComponents:      providerComponents,
		providerInventory:       providerMetadata,
	}
}
