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
	"sort"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade core and provider components in a management cluster.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	upgradeCmd.AddCommand(upgradePlanCmd)
	upgradeCmd.AddCommand(upgradeApplyCmd)
	RootCmd.AddCommand(upgradeCmd)
}

func sortUpgradeItems(plan client.UpgradePlan) {
	sort.Slice(plan.Providers, func(i, j int) bool {
		return plan.Providers[i].Provider.Type < plan.Providers[j].Provider.Type ||
			(plan.Providers[i].Provider.Type == plan.Providers[j].Provider.Type && plan.Providers[i].Provider.Name < plan.Providers[j].Provider.Name) ||
			(plan.Providers[i].Provider.Type == plan.Providers[j].Provider.Type && plan.Providers[i].Provider.Name == plan.Providers[j].Provider.Name && plan.Providers[i].Provider.Namespace < plan.Providers[j].Provider.Namespace)
	})
}

func sortUpgradePlans(upgradePlans []client.UpgradePlan) {
	sort.Slice(upgradePlans, func(i, j int) bool {
		return upgradePlans[i].Contract < upgradePlans[j].Contract
	})
}

func prettifyTargetVersion(version string) string {
	if version == "" {
		return "Already up to date"
	}
	return version
}
