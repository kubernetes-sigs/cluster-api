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
	"sort"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/version"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

// upgradeInfo holds all the information required for taking upgrade decisions for a provider.
type upgradeInfo struct {
	// metadata holds the information about releaseSeries and the link between release series and the API Version of Cluster API (contract).
	// e.g. release series 0.5.x for the AWS provider --> v1alpha3
	metadata *clusterctlv1.Metadata

	// currentVersion of the provider
	currentVersion *version.Version

	// currentContract of the provider
	currentContract string

	// nextVersions return the list of versions available for upgrades, defined as the list of version available in the provider repository
	// greater than the currentVersion.
	nextVersions []version.Version
}

// getUpgradeInfo returns all the info required for taking upgrade decisions for a provider.
// NOTE: This could contain also versions for the previous or next Cluster API contract (not supported in current clusterctl release, but upgrade plan should report this options).
func (u *providerUpgrader) getUpgradeInfo(provider clusterctlv1.Provider) (*upgradeInfo, error) {
	// Gets the list of versions available in the provider repository.
	configRepository, err := u.configClient.Providers().Get(provider.ProviderName, provider.GetProviderType())
	if err != nil {
		return nil, err
	}

	providerRepository, err := u.repositoryClientFactory(configRepository, u.configClient)
	if err != nil {
		return nil, err
	}

	repositoryVersions, err := providerRepository.GetVersions()
	if err != nil {
		return nil, err
	}

	if len(repositoryVersions) == 0 {
		return nil, errors.Errorf("failed to get available versions for the %s provider", provider.InstanceName())
	}

	//  Pick the provider's latest version available in the repository and use it to get the most recent metadata for the provider.
	var latestVersion *version.Version
	for _, availableVersion := range repositoryVersions {
		availableSemVersion, err := version.ParseSemantic(availableVersion)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse available version for the %s provider", provider.InstanceName())
		}

		if latestVersion == nil || latestVersion.LessThan(availableSemVersion) {
			latestVersion = availableSemVersion
		}
	}

	latestMetadata, err := providerRepository.Metadata(versionTag(latestVersion)).Get()
	if err != nil {
		return nil, err
	}

	// Get current provider version and check if the releaseSeries defined in metadata includes it.
	currentVersion, err := version.ParseSemantic(provider.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse current version for the %s provider", provider.InstanceName())
	}

	if latestMetadata.GetReleaseSeriesForVersion(currentVersion) == nil {
		return nil, errors.Errorf("invalid provider metadata: version %s (the current version) for the provider %s does not match any release series", provider.Version, provider.InstanceName())
	}

	// Filters the versions to be considered for upgrading the provider (next
	// versions) and checks if the releaseSeries defined in metadata includes
	// all of them.
	// NOTE: This could contain also versions for the previous or next Cluster API contract (not supported in current clusterctl release, but upgrade plan should report this options).
	nextVersions := []version.Version{}
	for _, repositoryVersion := range repositoryVersions {
		// we are ignoring the conversion error here because a first check already passed above
		repositorySemVersion, _ := version.ParseSemantic(repositoryVersion)

		// Drop the nextVersion version if older or equal that the current version
		// NB. Using !LessThan because version does not implement a GreaterThan method.
		if !currentVersion.LessThan(repositorySemVersion) {
			continue
		}

		if latestMetadata.GetReleaseSeriesForVersion(repositorySemVersion) == nil {
			return nil, errors.Errorf("invalid provider metadata: version %s (one of the available versions) for the provider %s does not match any release series", repositoryVersion, provider.InstanceName())
		}

		nextVersions = append(nextVersions, *repositorySemVersion)
	}

	return newUpgradeInfo(latestMetadata, currentVersion, nextVersions), nil
}

func newUpgradeInfo(metadata *clusterctlv1.Metadata, currentVersion *version.Version, nextVersions []version.Version) *upgradeInfo {
	// Sorts release series; this ensures also an implicit ordering of API Version of Cluster API (contract).
	sort.Slice(metadata.ReleaseSeries, func(i, j int) bool {
		return metadata.ReleaseSeries[i].Major < metadata.ReleaseSeries[j].Major ||
			(metadata.ReleaseSeries[i].Major == metadata.ReleaseSeries[j].Major && metadata.ReleaseSeries[i].Minor < metadata.ReleaseSeries[j].Minor)
	})

	// Sorts nextVersions.
	sort.Slice(nextVersions, func(i, j int) bool {
		return nextVersions[i].LessThan(&nextVersions[j])
	})

	// Gets the current contract for the provider
	// Please note this should never be empty, because getUpgradeInfo ensures the releaseSeries defined in metadata includes the current version.
	currentContract := ""
	if currentReleaseSeries := metadata.GetReleaseSeriesForVersion(currentVersion); currentReleaseSeries != nil {
		currentContract = currentReleaseSeries.Contract
	}

	return &upgradeInfo{
		metadata:        metadata,
		currentVersion:  currentVersion,
		currentContract: currentContract,
		nextVersions:    nextVersions,
	}
}

// getContractsForUpgrade return the list of API Version of Cluster API (contract) version available for a provider upgrade.
func (i *upgradeInfo) getContractsForUpgrade() []string {
	contractsForUpgrade := sets.NewString()
	for _, releaseSeries := range i.metadata.ReleaseSeries {
		// Drop the release series if older than the current version, because not relevant for upgrade.
		if i.currentVersion.Major() > releaseSeries.Major || (i.currentVersion.Major() == releaseSeries.Major && i.currentVersion.Minor() > releaseSeries.Minor) {
			continue
		}
		contractsForUpgrade.Insert(releaseSeries.Contract)
	}

	return contractsForUpgrade.List()
}

// getLatestNextVersion returns the next available version for a provider within the target API Version of Cluster API (contract).
// the next available version is the latest version available in the for the target contract version.
func (i *upgradeInfo) getLatestNextVersion(contract string) *version.Version {
	var latestNextVersion *version.Version
	for _, releaseSeries := range i.metadata.ReleaseSeries {
		// Skip the release series if not linked with the target contract version
		if releaseSeries.Contract != contract {
			continue
		}

		for j := range i.nextVersions {
			nextVersion := &i.nextVersions[j]

			// Drop the nextVersion version if not linked with the current
			// release series or if it is a pre-release.
			if nextVersion.Major() != releaseSeries.Major ||
				nextVersion.Minor() != releaseSeries.Minor ||
				nextVersion.PreRelease() != "" {
				continue
			}

			// Drop the nextVersion if older that the latestNextVersion selected so far
			if latestNextVersion == nil || latestNextVersion.LessThan(nextVersion) {
				latestNextVersion = nextVersion
			}
		}
	}

	return latestNextVersion
}

// versionTag converts a version to a RepositoryTag.
func versionTag(version *version.Version) string {
	if version == nil {
		return ""
	}

	return fmt.Sprintf("v%s", version.String())
}
