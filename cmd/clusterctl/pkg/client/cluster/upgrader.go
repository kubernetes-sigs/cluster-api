/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"

	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
)

// ProviderUpgrader defines methods for supporting provider upgrade.
type ProviderUpgrader interface {
	// Plan returns a set of suggested Upgrade plans for the cluster, and more specifically:
	// - Each management group gets separated upgrade plans.
	// - For each management group, an upgrade plan will be generated for each ClusterAPIVersion available, e.g.
	//   - Upgrade to the latest version in the the v1alpha2 series: ....
	//   - Upgrade to the latest version in the the v1alpha3 series: ....
	Plan() ([]UpgradePlan, error)
}

// UpgradePlan defines a list of possible upgrade targets for a management group.
type UpgradePlan struct {
	ClusterAPIVersion string
	CoreProvider      clusterctlv1.Provider
	Providers         []UpgradeItem
}

// UpgradeRef returns a string identifying the upgrade plan; this string is derived by the core provider which is
// unique for each management group.
func (u *UpgradePlan) UpgradeRef() string {
	return fmt.Sprintf("%s/%s", u.CoreProvider.Namespace, u.CoreProvider.Name)
}

// UpgradeItem defines a possible upgrade target for a provider in the management group.
type UpgradeItem struct {
	clusterctlv1.Provider
	NextVersion string
}

// UpgradeRef returns a string identifying the upgrade item; this string is derived by the provider.
func (u *UpgradeItem) UpgradeRef() string {
	return fmt.Sprintf("%s/%s", u.Namespace, u.Name)
}

type providerUpgrader struct {
	configClient            config.Client
	repositoryClientFactory RepositoryClientFactory
	providerInventory       InventoryClient
}

var _ ProviderUpgrader = &providerUpgrader{}

func (u *providerUpgrader) Plan() ([]UpgradePlan, error) {
	managementGroups, err := u.providerInventory.GetManagementGroups()
	if err != nil {
		return nil, err
	}

	var ret []UpgradePlan
	for _, managementGroup := range managementGroups {
		// The core provider is driving all the plan logic for each management group, because all the providers
		// in a management group are expected to support the same Cluster API/API version supported by the core provider.
		// e.g if the core provider supports v1alpha3, all the providers in the same management group should support v1alpha3 as well;
		// all the providers in the management group can upgrade to the latest release supporting v1alpha3, or if available,
		// or if available, all the providers in the management group can upgrade to the latest release supporting v1alpha4.

		// Gets the upgrade info for the core provider.
		coreUpgradeInfo, err := u.getUpgradeInfo(managementGroup.CoreProvider)
		if err != nil {
			return nil, err
		}

		// Identifies the Cluster API/API version that we should consider for the management group update (Nb. the core provider is driving the entire management group).
		// This includes the current API version (e.g. v1alpha3) and the new one available, if any.
		apiVersionsForUpgrade := coreUpgradeInfo.getAPIVersionsForUpgrade()
		if len(apiVersionsForUpgrade) == 0 {
			return nil, errors.Wrapf(err, "Invalid metadata: unable to find current ClusterAPIVersion for the %s/%s provider", managementGroup.CoreProvider.Namespace, managementGroup.CoreProvider.Name)
		}

		// Creates an UpgradePlan for each API version considered for upgrades; each upgrade plans contains
		// an UpgradeItem for each provider defining the next available version with the target API version, if available.
		// e.g. v1alpha3, cluster-api --> v0.3.2, kubeadm bootstrap --> v0.3.2, aws --> v0.5.4
		// e.g. v1alpha4, cluster-api --> v0.4.1, kubeadm bootstrap --> v0.4.1, aws --> v0.6.2
		for _, apiVersion := range apiVersionsForUpgrade {
			upgradeItems := []UpgradeItem{}
			for _, provider := range managementGroup.Providers {
				// Gets the upgrade info for the provider.
				providerUpgradeInfo, err := u.getUpgradeInfo(provider)
				if err != nil {
					return nil, err
				}

				// Identifies the next available version with the target API version for the provider, if available.
				nextVersion := providerUpgradeInfo.getLatestNextVersion(apiVersion)

				// Append the upgrade item for the provider/with the target API version.
				upgradeItems = append(upgradeItems, UpgradeItem{
					Provider:    provider,
					NextVersion: versionTag(nextVersion),
				})
			}

			ret = append(ret, UpgradePlan{
				ClusterAPIVersion: apiVersion,
				CoreProvider:      managementGroup.CoreProvider,
				Providers:         upgradeItems,
			})
		}
	}

	return ret, nil
}

func newProviderUpgrader(configClient config.Client, repositoryClientFactory RepositoryClientFactory, providerInventory InventoryClient) *providerUpgrader {
	return &providerUpgrader{
		configClient:            configClient,
		repositoryClientFactory: repositoryClientFactory,
		providerInventory:       providerInventory,
	}
}
