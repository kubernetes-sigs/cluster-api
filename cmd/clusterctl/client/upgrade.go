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
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterv1alpha4 "sigs.k8s.io/cluster-api/api/v1alpha4"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// PlanUpgradeOptions carries the options supported by upgrade plan.
type PlanUpgradeOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty, default discovery rules apply.
	Kubeconfig Kubeconfig
}

func (c *clusterctlClient) PlanCertManagerUpgrade(options PlanUpgradeOptions) (CertManagerUpgradePlan, error) {
	// Get the client for interacting with the management cluster.
	cluster, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return CertManagerUpgradePlan{}, err
	}

	certManager := cluster.CertManager()
	plan, err := certManager.PlanUpgrade()
	return CertManagerUpgradePlan(plan), err
}

func (c *clusterctlClient) PlanUpgrade(options PlanUpgradeOptions) ([]UpgradePlan, error) {
	// Get the client for interacting with the management cluster.
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return nil, err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract (default) or the previous one.
	// NOTE: given that v1beta1 (current) and v1alpha4 (previous) does not have breaking changes, we support also upgrades from v1alpha3 to v1beta1;
	// this is an exception and support for skipping releases should be removed in future releases.
	if err := clusterClient.ProviderInventory().CheckCAPIContract(
		cluster.AllowCAPIContract{Contract: clusterv1alpha3.GroupVersion.Version},
		cluster.AllowCAPIContract{Contract: clusterv1alpha4.GroupVersion.Version},
	); err != nil {
		return nil, err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	upgradePlans, err := clusterClient.ProviderUpgrader().Plan()
	if err != nil {
		return nil, err
	}

	// UpgradePlan is an alias for cluster.UpgradePlan; this makes the conversion
	aliasUpgradePlan := make([]UpgradePlan, len(upgradePlans))
	for i, plan := range upgradePlans {
		aliasUpgradePlan[i] = UpgradePlan{
			Contract:  plan.Contract,
			Providers: plan.Providers,
		}
	}

	return aliasUpgradePlan, nil
}

// ApplyUpgradeOptions carries the options supported by upgrade apply.
type ApplyUpgradeOptions struct {
	// Kubeconfig to use for accessing the management cluster. If empty, default discovery rules apply.
	Kubeconfig Kubeconfig

	// Contract defines the API Version of Cluster API (contract e.g. v1alpha4) the management cluster should upgrade to.
	// When upgrading by contract, the latest versions available will be used for all the providers; if you want
	// a more granular control on upgrade, use CoreProvider, BootstrapProviders, ControlPlaneProviders, InfrastructureProviders.
	Contract string

	// CoreProvider instance and version (e.g. capi-system/cluster-api:v0.3.0) to upgrade to. This field can be used as alternative to Contract.
	CoreProvider string

	// BootstrapProviders instance and versions (e.g. capi-kubeadm-bootstrap-system/kubeadm:v0.3.0) to upgrade to. This field can be used as alternative to Contract.
	BootstrapProviders []string

	// ControlPlaneProviders instance and versions (e.g. capi-kubeadm-control-plane-system/kubeadm:v0.3.0) to upgrade to. This field can be used as alternative to Contract.
	ControlPlaneProviders []string

	// InfrastructureProviders instance and versions (e.g. capa-system/aws:v0.5.0) to upgrade to. This field can be used as alternative to Contract.
	InfrastructureProviders []string
}

func (c *clusterctlClient) ApplyUpgrade(options ApplyUpgradeOptions) error {
	if options.Contract != "" && options.Contract != clusterv1.GroupVersion.Version {
		return errors.Errorf("current version of clusterctl could only upgrade to %s contract, requested %s", clusterv1.GroupVersion.Version, options.Contract)
	}

	// Get the client for interacting with the management cluster.
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract (default) or the previous one.
	// NOTE: given that v1beta1 (current) and v1alpha4 (previous) does not have breaking changes, we support also upgrades from v1alpha3 to v1beta1;
	// this is an exception and support for skipping releases should be removed in future releases.
	if err := clusterClient.ProviderInventory().CheckCAPIContract(
		cluster.AllowCAPIContract{Contract: clusterv1alpha3.GroupVersion.Version},
		cluster.AllowCAPIContract{Contract: clusterv1alpha4.GroupVersion.Version},
	); err != nil {
		return err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return err
	}

	// Ensures the latest version of cert-manager.
	// NOTE: it is safe to upgrade to latest version of cert-manager given that it provides
	// conversion web-hooks around Issuer/Certificate kinds, so installing an older versions of providers
	// should continue to work with the latest cert-manager.
	certManager := clusterClient.CertManager()
	if err := certManager.EnsureLatestVersion(); err != nil {
		return err
	}

	// Check if the user want a custom upgrade
	isCustomUpgrade := options.CoreProvider != "" ||
		len(options.BootstrapProviders) > 0 ||
		len(options.ControlPlaneProviders) > 0 ||
		len(options.InfrastructureProviders) > 0

	// If we are upgrading a specific set of providers only, process the providers and call ApplyCustomPlan.
	if isCustomUpgrade {
		// Converts upgrade references back into an UpgradeItem.
		upgradeItems := []cluster.UpgradeItem{}

		if options.CoreProvider != "" {
			upgradeItems, err = addUpgradeItems(upgradeItems, clusterctlv1.CoreProviderType, options.CoreProvider)
			if err != nil {
				return err
			}
		}
		upgradeItems, err = addUpgradeItems(upgradeItems, clusterctlv1.BootstrapProviderType, options.BootstrapProviders...)
		if err != nil {
			return err
		}
		upgradeItems, err = addUpgradeItems(upgradeItems, clusterctlv1.ControlPlaneProviderType, options.ControlPlaneProviders...)
		if err != nil {
			return err
		}
		upgradeItems, err = addUpgradeItems(upgradeItems, clusterctlv1.InfrastructureProviderType, options.InfrastructureProviders...)
		if err != nil {
			return err
		}

		// Execute the upgrade using the custom upgrade items
		return clusterClient.ProviderUpgrader().ApplyCustomPlan(upgradeItems...)
	}

	// Otherwise we are upgrading a whole management cluster according to a clusterctl generated upgrade plan.
	return clusterClient.ProviderUpgrader().ApplyPlan(options.Contract)
}

func addUpgradeItems(upgradeItems []cluster.UpgradeItem, providerType clusterctlv1.ProviderType, providers ...string) ([]cluster.UpgradeItem, error) {
	for _, upgradeReference := range providers {
		providerUpgradeItem, err := parseUpgradeItem(upgradeReference, providerType)
		if err != nil {
			return nil, err
		}
		if providerUpgradeItem.NextVersion == "" {
			return nil, errors.Errorf("invalid provider name %q. Provider name should be in the form namespace/name:version and version cannot be empty", upgradeReference)
		}
		upgradeItems = append(upgradeItems, *providerUpgradeItem)
	}
	return upgradeItems, nil
}

func parseUpgradeItem(ref string, providerType clusterctlv1.ProviderType) (*cluster.UpgradeItem, error) {
	refSplit := strings.Split(strings.ToLower(ref), "/")
	if len(refSplit) != 2 {
		return nil, errors.Errorf("invalid provider name %q. Provider name should be in the form namespace/provider[:version]", ref)
	}

	if refSplit[0] == "" {
		return nil, errors.Errorf("invalid provider name %q. Provider name should be in the form namespace/name[:version] and namespace cannot be empty", ref)
	}
	namespace := refSplit[0]

	name, version, err := parseProviderName(refSplit[1])
	if err != nil {
		return nil, errors.Wrapf(err, "invalid provider name %q. Provider name should be in the form namespace/name[:version] and the namespace should be valid", ref)
	}

	return &cluster.UpgradeItem{
		Provider: clusterctlv1.Provider{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      clusterctlv1.ManifestLabel(name, providerType),
			},
			ProviderName: name,
			Type:         string(providerType),
			// The value for the following fields will be retrieved while
			// creating the custom upgrade plan.
			WatchedNamespace: "",
		},
		NextVersion: version,
	}, nil
}
