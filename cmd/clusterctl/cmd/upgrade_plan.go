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
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type upgradePlanOptions struct {
	kubeconfig        string
	kubeconfigContext string
}

var up = &upgradePlanOptions{}

var upgradePlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Provide a list of recommended target versions for upgrading Cluster API providers in a management cluster",
	Long: LongDesc(`
		The upgrade plan command provides a list of recommended target versions for upgrading the
        Cluster API providers in a management cluster.

		All the providers should be supporting the same API Version of Cluster API (contract) in order
        to guarantee the proper functioning of the management cluster.

		Then, for each provider, the following upgrade options are provided:
		- The latest patch release for the current API Version of Cluster API (contract).
		- The latest patch release for the next API Version of Cluster API (contract), if available.`),

	Example: Examples(`
		# Gets the recommended target versions for upgrading Cluster API providers.
		clusterctl upgrade plan`),

	RunE: func(*cobra.Command, []string) error {
		return runUpgradePlan()
	},
}

func init() {
	upgradePlanCmd.Flags().StringVar(&up.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig file to use for accessing the management cluster. If empty, default discovery rules apply.")
	upgradePlanCmd.Flags().StringVar(&up.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
}

func runUpgradePlan() error {
	ctx := context.Background()

	c, err := client.New(ctx, cfgFile)
	if err != nil {
		return err
	}

	certManUpgradePlan, err := c.PlanCertManagerUpgrade(ctx, client.PlanUpgradeOptions{
		Kubeconfig: client.Kubeconfig{Path: up.kubeconfig, Context: up.kubeconfigContext},
	})
	if err != nil {
		return err
	}
	if !certManUpgradePlan.ExternallyManaged {
		if certManUpgradePlan.ShouldUpgrade {
			fmt.Printf("Cert-Manager will be upgraded from %q to %q\n\n", certManUpgradePlan.From, certManUpgradePlan.To)
		} else {
			fmt.Printf("Cert-Manager is already up to date\n\n")
		}
	}

	upgradePlans, err := c.PlanUpgrade(ctx, client.PlanUpgradeOptions{
		Kubeconfig: client.Kubeconfig{Path: up.kubeconfig, Context: up.kubeconfigContext},
	})

	if err != nil {
		return err
	}

	if len(upgradePlans) == 0 {
		fmt.Println("There are no providers in the cluster. Please use clusterctl init to initialize a Cluster API management cluster.")
		return nil
	}

	// ensure upgrade plans are sorted consistently (by CoreProvider.Namespace, Contract).
	sortUpgradePlans(upgradePlans)

	for _, plan := range upgradePlans {
		// ensure provider are sorted consistently (by Type, Name, Namespace).
		sortUpgradeItems(plan)

		upgradeAvailable := false

		fmt.Println("")
		fmt.Printf("Latest release available for the %s API Version of Cluster API (contract):\n", plan.Contract)
		fmt.Println("")
		w := tabwriter.NewWriter(os.Stdout, 10, 4, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tNAMESPACE\tTYPE\tCURRENT VERSION\tNEXT VERSION")
		for _, upgradeItem := range plan.Providers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", upgradeItem.Provider.Name, upgradeItem.Provider.Namespace, upgradeItem.Provider.Type, upgradeItem.Provider.Version, prettifyTargetVersion(upgradeItem.NextVersion))
			if upgradeItem.NextVersion != "" {
				upgradeAvailable = true
			}
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Println("")

		if upgradeAvailable {
			if plan.Contract == clusterv1.GroupVersion.Version {
				fmt.Println("You can now apply the upgrade by executing the following command:")
				fmt.Println("")
				fmt.Printf("clusterctl upgrade apply --contract %s\n", plan.Contract)
			} else {
				fmt.Printf("The current version of clusterctl could not upgrade to %s contract (only %s supported).\n", plan.Contract, clusterv1.GroupVersion.Version)
			}
		} else {
			fmt.Println("You are already up to date!")
		}
		fmt.Println("")
	}

	return nil
}
