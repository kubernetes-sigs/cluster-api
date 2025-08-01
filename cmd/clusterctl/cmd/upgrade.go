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
	"slices"

	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	GroupID: groupManagement,
	Short:   "Upgrade core and provider components in a management cluster",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

func init() {
	upgradeCmd.AddCommand(upgradePlanCmd)
	upgradeCmd.AddCommand(upgradeApplyCmd)
	RootCmd.AddCommand(upgradeCmd)
}

func sortUpgradeItems(plan client.UpgradePlan) {
	slices.SortFunc(plan.Providers, func(i, j cluster.UpgradeItem) int {
		if i.Type < j.Type ||
			(i.Type == j.Type && i.Name < j.Name) ||
			(i.Type == j.Type && i.Name == j.Name && i.Namespace < j.Namespace) {
			return -1
		}
		return 1
	})
}

func sortUpgradePlans(upgradePlans []client.UpgradePlan) {
	slices.SortFunc(upgradePlans, func(i, j client.UpgradePlan) int {
		if i.Contract < j.Contract {
			return -1
		}
		return 1
	})
}

func prettifyTargetVersion(version string) string {
	if version == "" {
		return "Already up to date"
	}
	return version
}
