/*
Copyright 2019 The Kubernetes Authors.

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
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client"
)

var configRepositoryCmd = &cobra.Command{
	Use:   "repositories",
	Args:  cobra.NoArgs,
	Short: "Display the list of Cluster API providers and their repository configuration",
	Long: LongDesc(`
		Displays the list of the Cluster API provider's and their repository configuration.
		
		clusterctl ships with a list of well-known providers; if necessary, edit
		the $HOME/.cluster-api/clusterctl.yaml file to add new provider configurations or to customize existing ones.`),

	Example: Examples(`
		# Displays the list of available providers.
		clusterctl config repositories`),

	RunE: func(cmd *cobra.Command, args []string) error {
		return runGetRepositories()
	},
}

func init() {
	configCmd.AddCommand(configRepositoryCmd)
}

func runGetRepositories() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	repositoryList, err := c.GetProvidersConfig()
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 10, 4, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tURL")
	for _, r := range repositoryList {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name(), r.Type(), r.URL())
	}
	w.Flush()

	return nil
}
