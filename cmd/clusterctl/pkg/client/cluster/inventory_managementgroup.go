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
	"strings"

	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

// ManagementGroup is a group of providers composed by a CoreProvider and a set of Bootstrap/ControlPlane/Infrastructure providers
// watching objects in the same namespace. For example, a management group can be used for upgrades, in order to ensure all the providers
// in a management group support the same API Version of Cluster API (contract).
type ManagementGroup struct {
	CoreProvider clusterctlv1.Provider
	Providers    []clusterctlv1.Provider
}

// Equals return true if two management groups have the same core provider.
func (mg *ManagementGroup) Equals(other *ManagementGroup) bool {
	return mg.CoreProvider.Equals(other.CoreProvider)
}

// GetProviderByInstanceName returns a specific provider instance.
func (mg *ManagementGroup) GetProviderByInstanceName(instanceName string) *clusterctlv1.Provider {
	for _, provider := range mg.Providers {
		if provider.InstanceName() == instanceName {
			return &provider
		}
	}
	return nil
}

// ManagementGroupList defines a list of management groups
type ManagementGroupList []ManagementGroup

// FindManagementGroupByProviderInstanceName return the management group that hosts a given provider.
func (ml *ManagementGroupList) FindManagementGroupByProviderInstanceName(instanceName string) *ManagementGroup {
	for _, managementGroup := range *ml {
		if p := managementGroup.GetProviderByInstanceName(instanceName); p != nil {
			return &managementGroup
		}
	}
	return nil
}

// deriveManagementGroups derives the management groups from a list of providers.
func deriveManagementGroups(providerList *clusterctlv1.ProviderList) (ManagementGroupList, error) {
	// If any of the core providers watch the same namespace, we cannot define the management group.
	if err := checkOverlappingCoreProviders(providerList); err != nil {
		return nil, err
	}

	// If any of the  Bootstrap/ControlPlane/Infrastructure providers can't be combined with a core provider,
	// or if any of the Bootstrap/ControlPlane/Infrastructure providers is watching objects controlled by more than one core provider
	// we can't define a management group.
	if err := checkOverlappingProviders(providerList); err != nil {
		return nil, err
	}

	// Composes the management group
	managementGroups := ManagementGroupList{}
	for _, coreProvider := range providerList.FilterCore() {
		group := ManagementGroup{CoreProvider: coreProvider}
		for _, provider := range providerList.Items {
			if coreProvider.HasWatchingOverlapWith(provider) {
				group.Providers = append(group.Providers, provider)
			}
		}
		managementGroups = append(managementGroups, group)
	}

	return managementGroups, nil
}

// checkOverlappingCoreProviders checks if there are core providers with overlapping watching namespaces, if yes, return error e.g.
// cluster-api in capi-system watching all namespaces and another cluster-api in capi-system2 watching capi-system2 (both are watching capi-system2)
// NB. This should not happen because init prevent the users to do so, but nevertheless we are double checking this before upgrades.
func checkOverlappingCoreProviders(providerList *clusterctlv1.ProviderList) error {
	for _, provider := range providerList.FilterCore() {
		for _, other := range providerList.FilterCore() {
			// if the provider to compare is the same of the other provider, skip it
			if provider.Equals(other) {
				continue
			}

			// check for overlapping namespaces
			if provider.HasWatchingOverlapWith(other) {
				return errors.Errorf("Unable to identify management groups: core providers %s and %s have overlapping watching namespaces",
					provider.InstanceName(),
					other.InstanceName(),
				)
			}
		}
	}
	return nil
}

// checkOverlappingProviders checks if Bootstrap/ControlPlane/Infrastructure providers:
// 1) can't be combined with any core provider
//    e.g. cluster-api in capi-system watching capi-system and aws in capa-system watching capa-system (they are watching different namespaces)
// 2) can be combined with more than one core provider
//    e.g. cluster-api in capi-system1 watching all capi-system1, cluster-api in capi-system2 watching all capi-system2,  aws in capa-system watching all namespaces (aws is working with both CAPI instances, but this is not a configuration supported by clusterctl)
func checkOverlappingProviders(providerList *clusterctlv1.ProviderList) error {
	for _, provider := range providerList.FilterNonCore() {
		// check for the core providers watching objects in the same namespace of the provider
		var overlappingCoreProviders []string
		for _, coreProvider := range providerList.FilterCore() {
			if provider.HasWatchingOverlapWith(coreProvider) {
				overlappingCoreProviders = append(overlappingCoreProviders, coreProvider.InstanceName())
			}
		}

		// if the provider does not overlap with any core provider, return error (it will not be part of any management group)
		if len(overlappingCoreProviders) == 0 {
			return errors.Errorf("Unable to identify management groups: provider %s can't be combined with any core provider",
				provider.InstanceName(),
			)
		}

		// if the provider overlaps with more than one core provider, return error (it is part of two management groups --> e.g. there could be potential upgrade conflicts)
		if len(overlappingCoreProviders) > 1 {
			return errors.Errorf("Unable to identify management groups: provider %s is watching for objects in namespaces controlled by more than one core provider (%s)",
				provider.InstanceName(),
				strings.Join(overlappingCoreProviders, " ,"),
			)
		}
	}
	return nil
}

func (p *inventoryClient) GetManagementGroups() (ManagementGroupList, error) {
	providerList, err := p.List()
	if err != nil {
		return nil, err
	}

	return deriveManagementGroups(providerList)
}
