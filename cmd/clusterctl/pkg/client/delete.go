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

package client

import (
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
)

func (c *clusterctlClient) Delete(options DeleteOptions) error {
	clusterClient, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return err
	}

	if err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return err
	}

	// Get the list of installed providers.
	installedProviders, err := clusterClient.ProviderInventory().List()
	if err != nil {
		return err
	}

	// If the list of providers to delete is empty, delete all the providers.
	// Prepare the list of providers to delete.
	var providers []clusterctlv1.Provider

	if len(options.Providers) == 0 {
		providers = installedProviders.Items
	} else {
		// Otherwise we are deleting only a subset of providers.
		for _, provider := range options.Providers {
			// Parse the abbreviated syntax for name[:version]
			name, _, err := parseProviderName(provider)
			if err != nil {
				return err
			}

			// If the namespace where the provider is installed is not provided, try to detect it
			namespace := options.Namespace
			if namespace == "" {
				namespace, err = clusterClient.ProviderInventory().GetDefaultProviderNamespace(name)
				if err != nil {
					return err
				}

				// if there are more instance of a providers, it is not possible to get a default namespace for the provider,
				// so we should return and ask for it.
				if namespace == "" {
					return errors.Errorf("Unable to find default namespace for the %q provider. Please specify the provider's namespace", name)
				}
			}

			// Check the provider/namespace tuple actually matches one of the installed providers.
			found := false
			for _, ip := range installedProviders.Items {
				if ip.Name == name && ip.Namespace == namespace {
					found = true
					providers = append(providers, ip)
					break
				}
			}
			if found {
				break
			}

			// In case the provider does not match any installed providers, we still force deletion
			// so the user can do 'delete' without removing CRD and after some time 'delete --delete-crd' (same for the namespace).
			providers = append(providers, clusterctlv1.Provider{ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			}})
		}
	}

	// Delete the selected providers
	for _, provider := range providers {
		if err := clusterClient.ProviderComponents().Delete(cluster.DeleteOptions{Provider: provider, ForceDeleteNamespace: options.ForceDeleteNamespace, ForceDeleteCRD: options.ForceDeleteCRD}); err != nil {
			return err
		}
	}

	return nil
}
