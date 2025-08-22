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
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// DeleteOptions carries the options supported by Delete.
type DeleteOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// CoreProvider version (e.g. cluster-api:v1.1.5) to delete from the management cluster.
	CoreProvider string

	// BootstrapProviders and versions (e.g. kubeadm:v1.1.5) to delete from the management cluster.
	BootstrapProviders []string

	// InfrastructureProviders and versions (e.g. aws:v0.5.0) to delete from the management cluster.
	InfrastructureProviders []string

	// ControlPlaneProviders and versions (e.g. kubeadm:v1.1.5) to delete from the management cluster.
	ControlPlaneProviders []string

	// IPAMProviders and versions (e.g. infoblox:v0.0.1) to delete from the management cluster.
	IPAMProviders []string

	// RuntimeExtensionProviders and versions (e.g. test:v0.0.1) to delete from the management cluster.
	RuntimeExtensionProviders []string

	// AddonProviders and versions (e.g. helm:v0.1.0) to delete from the management cluster.
	AddonProviders []string

	// DeleteAll set for deletion of all the providers.
	DeleteAll bool

	// IncludeNamespace forces the deletion of the namespace where the providers are hosted
	// (and of all the contained objects).
	IncludeNamespace bool

	// IncludeCRDs forces the deletion of the provider's CRDs (and of all the related objects).
	IncludeCRDs bool

	// SkipInventory forces the deletion of the inventory items used by clusterctl to track providers.
	SkipInventory bool
}

func (c *clusterctlClient) Delete(ctx context.Context, options DeleteOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract.
	if err := clusterClient.ProviderInventory().CheckCAPIContract(ctx); err != nil {
		return err
	}

	// Ensure the custom resource definitions required by clusterctl are in place.
	if err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions(ctx); err != nil {
		return err
	}

	// Get the list of installed providers.
	installedProviders, err := clusterClient.ProviderInventory().List(ctx)
	if err != nil {
		return err
	}

	// Prepare the list of providers to delete.
	var providersToDelete []clusterctlv1.Provider

	if options.DeleteAll {
		providersToDelete = installedProviders.Items
	} else {
		// Otherwise we are deleting only a subset of providers.
		var providers []clusterctlv1.Provider
		providers, err = appendProviders(providers, clusterctlv1.CoreProviderType, options.CoreProvider)
		if err != nil {
			return err
		}

		providers, err = appendProviders(providers, clusterctlv1.BootstrapProviderType, options.BootstrapProviders...)
		if err != nil {
			return err
		}

		providers, err = appendProviders(providers, clusterctlv1.ControlPlaneProviderType, options.ControlPlaneProviders...)
		if err != nil {
			return err
		}

		providers, err = appendProviders(providers, clusterctlv1.InfrastructureProviderType, options.InfrastructureProviders...)
		if err != nil {
			return err
		}

		providers, err = appendProviders(providers, clusterctlv1.IPAMProviderType, options.IPAMProviders...)
		if err != nil {
			return err
		}

		providers, err = appendProviders(providers, clusterctlv1.RuntimeExtensionProviderType, options.RuntimeExtensionProviders...)
		if err != nil {
			return err
		}

		providers, err = appendProviders(providers, clusterctlv1.AddonProviderType, options.AddonProviders...)
		if err != nil {
			return err
		}

		for _, provider := range providers {
			// Try to detect the namespace where the provider lives
			provider.Namespace, err = clusterClient.ProviderInventory().GetProviderNamespace(ctx, provider.ProviderName, provider.GetProviderType())
			if err != nil {
				return err
			}
			if provider.Namespace == "" {
				return errors.Errorf("failed to identify the namespace for the %q provider", provider.ProviderName)
			}

			if provider.Version != "" {
				version, err := clusterClient.ProviderInventory().GetProviderVersion(ctx, provider.ProviderName, provider.GetProviderType())
				if err != nil {
					return err
				}
				if provider.Version != version {
					return errors.Errorf("failed to identify the provider %q with version %q", provider.ProviderName, provider.Version)
				}
			}

			providersToDelete = append(providersToDelete, provider)
		}
	}

	if options.IncludeCRDs {
		errList := []error{}
		for _, provider := range providersToDelete {
			err = clusterClient.ProviderComponents().ValidateNoObjectsExist(ctx, provider)
			if err != nil {
				errList = append(errList, err)
			}
		}
		if len(errList) > 0 {
			return kerrors.NewAggregate(errList)
		}
	}

	// Delete the selected providers.
	for _, provider := range providersToDelete {
		if err := clusterClient.ProviderComponents().Delete(ctx, cluster.DeleteOptions{Provider: provider, IncludeNamespace: options.IncludeNamespace, IncludeCRDs: options.IncludeCRDs, SkipInventory: options.SkipInventory}); err != nil {
			return err
		}
	}

	return nil
}

func appendProviders(list []clusterctlv1.Provider, providerType clusterctlv1.ProviderType, names ...string) ([]clusterctlv1.Provider, error) {
	for _, name := range names {
		if name == "" {
			continue
		}

		// Parse the abbreviated syntax for name[:version]
		name, version, err := parseProviderName(name)
		if err != nil {
			return nil, err
		}

		list = append(list, clusterctlv1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterctlv1.ManifestLabel(name, providerType),
			},
			ProviderName: name,
			Type:         string(providerType),
			Version:      version,
		})
	}
	return list, nil
}
