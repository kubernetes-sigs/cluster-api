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

package cmd

import (
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client"
)

type upgradeApplyOptions struct {
	kubeconfig      string
	managementGroup string
	contract        string
}

var ua = &upgradeApplyOptions{}

var upgradeApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply new versions of Cluster API core and providers in a management cluster",
	Long: LongDesc(`
		The upgrade apply command applies new versions of Cluster API providers as defined by clusterctl upgrade plan.

		New version should be applied for each management groups, ensuring all the providers on the same cluster API version
		in order to guarantee the proper functioning of the management cluster.`),

	Example: Examples(`
		# Upgrades all the providers in the capi-system/cluster-api to the latest version available which is compliant
		# to the v1alpha3 API Version of Cluster API (contract).
		clusterctl upgrade apply --management-group capi-system/cluster-api  --contract v1alpha3`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpgradeApply()
	},
}

func init() {
	upgradeApplyCmd.Flags().StringVar(&ua.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If unspecified, default discovery rules apply.")
	upgradeApplyCmd.Flags().StringVar(&ua.managementGroup, "management-group", "",
		"The management group that should be upgraded")
	upgradeApplyCmd.Flags().StringVar(&ua.contract, "contract", "",
		"The API Version of Cluster API (contract) the management group should upgrade to")
}

func runUpgradeApply() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	if err := c.ApplyUpgrade(client.ApplyUpgradeOptions{
		Kubeconfig:      ua.kubeconfig,
		ManagementGroup: ua.managementGroup,
		Contract:        ua.contract,
	}); err != nil {
		return err
	}
	return nil
}
