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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client"
)

type configProvidersOptions struct {
	output            string
	targetNamespace   string
	watchingNamespace string
}

var cpo = &configProvidersOptions{}

var configProvidersCmd = &cobra.Command{
	Use:     "providers",
	Aliases: []string{"provider"},
	Args:    cobra.MaximumNArgs(1),
	Short:   "Display Cluster API provider configuration",
	Long: LongDesc(`
		clusterctl ships with a list of well-known providers; if necessary, edit
		the $HOME/cluster-api/.clusterctl.yaml file to add new provider configurations or to customize existing ones.
		
		Each provider configuration links to a repository, and clusterctl will fetch the provider
		components yaml from there; required variables and the default namespace where the
		provider should be deployed are derived from this file.`),

	Example: Examples(`
		# Displays the list of available providers.
		clusterctl config providers

		# Displays relevant information about the AWS provider, including also the list of 
		# required env variables, if any.
		clusterctl config provider aws

		# Displays relevant information about a specific version of the AWS provider.
		clusterctl config provider aws:v0.4.1

		# Displays the yaml for creating provider components for the AWS provider.
		clusterctl config provider aws -o yaml

		# Displays the yaml for creating provider components for a specific version of the AWS provider.
		clusterctl config provider aws:v0.4.1 -o yaml`),

	RunE: func(cmd *cobra.Command, args []string) error {
		if !(cpo.output == "" || cpo.output == "yaml" || cpo.output == "text") {
			return errors.New("please provide a valid output. Supported values are [ text, yaml ]")
		}

		if len(args) == 0 {
			return runGetRepositories()
		}
		return runGetRepository(args[0], cpo.targetNamespace, cpo.watchingNamespace, cpo.output)
	},
}

func init() {
	configProvidersCmd.Flags().StringVarP(&cpo.output, "output", "o", "text", "Output format. One of [yaml, text]")
	configProvidersCmd.Flags().StringVarP(&cpo.targetNamespace, "target-namespace", "", "", "The target namespace where the provider should be deployed. If not specified, a default namespace will be used")
	configProvidersCmd.Flags().StringVarP(&cpo.watchingNamespace, "watching-namespace", "", "", "Namespace that the provider should watch to reconcile Cluster API objects. If unspecified, the provider watches for Cluster API objects across all namespaces")

	configCmd.AddCommand(configProvidersCmd)
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

func runGetRepository(providerName, targetNamespace, watchingNamespace, output string) error {
	return nil
}
