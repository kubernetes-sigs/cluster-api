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
	"strings"

	"github.com/pkg/errors"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
)

func (p *inventoryClient) GetManagementGroups() ([]ManagementGroup, error) {
	providerList, err := p.list()
	if err != nil {
		return nil, err
	}

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
	managementGroups := []ManagementGroup{}
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
				return errors.Errorf("Unable to identify management groups: core providers %s/%s and %s/%s have overlapping watching namespaces",
					provider.Namespace, provider.Name,
					other.Namespace, other.Name,
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
				overlappingCoreProviders = append(overlappingCoreProviders, fmt.Sprintf("%s/%s", coreProvider.Namespace, coreProvider.Name))
			}
		}

		// if the provider does not overlap with any core provider, return error (it will not be part of any management group)
		if len(overlappingCoreProviders) == 0 {
			return errors.Errorf("Unable to identify management groups: provider %s/%s can't be combined with any core provider",
				provider.Namespace, provider.Name,
			)
		}

		// if the provider overlaps with more than one core provider, return error (it is part of two management groups --> e.g. there could be potential upgrade conflicts)
		if len(overlappingCoreProviders) > 1 {
			return errors.Errorf("Unable to identify management groupss: provider %s/%s is watching for objects in namespaces controlled by more than one core provider (%s)",
				provider.Namespace, provider.Name,
				strings.Join(overlappingCoreProviders, " ,"),
			)
		}
	}
	return nil
}
