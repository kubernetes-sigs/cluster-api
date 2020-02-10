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
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/cluster"
)

// PlanUpgradeOptions carries the options supported by upgrade plan.
type PlanUpgradeOptions struct {
	// Kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig discovery will be used.
	Kubeconfig string
}

func (c *clusterctlClient) PlanUpgrade(options PlanUpgradeOptions) ([]UpgradePlan, error) {
	// Get the client for interacting with the management cluster.
	cluster, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return nil, err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	upgradePlan, err := cluster.ProviderUpgrader().Plan()
	if err != nil {
		return nil, err
	}

	// UpgradePlan is an alias for cluster.UpgradePlan; this makes the conversion
	aliasUpgradePlan := make([]UpgradePlan, len(upgradePlan))
	for i, plan := range upgradePlan {
		aliasUpgradePlan[i] = UpgradePlan{
			Contract:     plan.Contract,
			CoreProvider: plan.CoreProvider,
			Providers:    plan.Providers,
		}
	}

	return aliasUpgradePlan, nil
}

// ApplyUpgradeOptions carries the options supported by upgrade apply.
type ApplyUpgradeOptions struct {
	// Kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig discovery will be used.
	Kubeconfig string

	// ManagementGroup that should be upgraded.
	ManagementGroup string

	// Contract defines the API Version of Cluster API (contract) the management group should upgrade to.
	Contract string
}

func (c *clusterctlClient) ApplyUpgrade(options ApplyUpgradeOptions) error {
	// Get the client for interacting with the management cluster.
	clusterClient, err := c.clusterClientFactory(options.Kubeconfig)
	if err != nil {
		return err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := clusterClient.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return err
	}

	// The management group name is derived from the core provider name, so now
	// convert the reference back into a coreProvider.
	coreUpgradeItem, err := parseUpgradeItem(options.ManagementGroup)
	if err != nil {
		return err
	}
	coreProvider := coreUpgradeItem.Provider

	// Otherwise we are upgrading a whole management group according to a clusterctl generated upgrade plan.
	if err := clusterClient.ProviderUpgrader().ApplyPlan(coreProvider, options.Contract); err != nil {
		return err
	}

	return nil
}

func parseUpgradeItem(ref string) (*cluster.UpgradeItem, error) {
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
				Name:      name,
			},
		},
		NextVersion: version,
	}, nil
}
