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
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type upgradePlanOptions struct {
	kubeconfig string
}

var up = &upgradePlanOptions{}

var upgradePlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Provide a list of recommended target versions for upgrading Cluster API providers in a management cluster",
	Long: LongDesc(`
		The upgrade plan command provides a list of recommended target versions for upgrading Cluster API providers in a management cluster.

		The providers are grouped into management groups, each one defining a set of providers that should be supporting
		the same API Version of Cluster API (contract) in order to guarantee the proper functioning of the management cluster.

		Then, for each provider in a management group, the following upgrade options are provided:
		- The latest patch release for the current API Version of Cluster API (contract).
		- The latest patch release for the next API Version of Cluster API (contract), if available.`),

	Example: Examples(`
		# Gets the recommended target versions for upgrading Cluster API providers.
		clusterctl upgrade plan`),

	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpgradePlan()
	},
}

func init() {
	upgradePlanCmd.Flags().StringVar(&up.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If empty, default discovery rules apply.")
}

func runUpgradePlan() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	upgradePlans, err := c.PlanUpgrade(client.PlanUpgradeOptions{
		Kubeconfig: up.kubeconfig,
	})
	if err != nil {
		return err
	}

	// ensure upgrade plans are sorted consistently (by CoreProvider.Namespace, Contract).
	sortUpgradePlans(upgradePlans)

	if len(upgradePlans) == 0 {
		fmt.Println("There are no management groups in the cluster. Please use clusterctl init to initialize a Cluster API management cluster.")
		return nil
	}

	for _, plan := range upgradePlans {
		// ensure provider are sorted consistently (by Type, Name, Namespace).
		sortUpgradeItems(plan)

		upgradeAvailable := false

		fmt.Println("")
		fmt.Printf("Management group: %s, latest release available for the %s API Version of Cluster API (contract):\n", plan.CoreProvider.InstanceName(), plan.Contract)
		fmt.Println("")
		w := tabwriter.NewWriter(os.Stdout, 10, 4, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tNAMESPACE\tTYPE\tCURRENT VERSION\tNEXT VERSION")
		for _, upgradeItem := range plan.Providers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", upgradeItem.Provider.Name, upgradeItem.Provider.Namespace, upgradeItem.Provider.Type, upgradeItem.Provider.Version, prettifyTargetVersion(upgradeItem.NextVersion))
			if upgradeItem.NextVersion != "" {
				upgradeAvailable = true
			}
		}
		w.Flush()
		fmt.Println("")

		if upgradeAvailable {
			fmt.Println("You can now apply the upgrade by executing the following command:")
			fmt.Println("")
			fmt.Println(fmt.Sprintf("   upgrade apply --management-group %s --contract %s", plan.CoreProvider.InstanceName(), plan.Contract))
		} else {
			fmt.Println("You are already up to date!")
		}
		fmt.Println("")

	}

	return nil
}
