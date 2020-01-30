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

var configProviderCmd = &cobra.Command{
	Use:   "provider",
	Args:  cobra.ExactArgs(1),
	Short: "Display information about a Cluster API provider",
	Long: LongDesc(`
		Display information about a Cluster API provider.

		clusterctl fetch the provider components yaml from the provider repository; required variables
		and the default namespace where the provider should be deployed are derived from this file.`),

	Example: Examples(`
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
		if !(cpo.output == "" || cpo.output == "yaml" || cpo.output == "text") { //nolint goconst
			return errors.New("please provide a valid output. Supported values are [ text, yaml ]")
		}

		if len(args) == 0 {
			return runGetRepositories()
		}
		return runGetComponents(args[0], cpo.targetNamespace, cpo.watchingNamespace, cpo.output)
	},
}

func init() {
	configProviderCmd.Flags().StringVarP(&cpo.output, "output", "o", "text", "Output format. One of [yaml, text]")
	configProviderCmd.Flags().StringVarP(&cpo.targetNamespace, "target-namespace", "", "", "The target namespace where the provider should be deployed. If not specified, a default namespace will be used")
	configProviderCmd.Flags().StringVarP(&cpo.watchingNamespace, "watching-namespace", "", "", "Namespace that the provider should watch to reconcile Cluster API objects. If unspecified, the provider watches for Cluster API objects across all namespaces")

	configCmd.AddCommand(configProviderCmd)
}

func runGetComponents(providerName, targetNamespace, watchingNamespace, output string) error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	components, err := c.GetProviderComponents(providerName, targetNamespace, watchingNamespace)
	if err != nil {
		return err
	}

	if output == "yaml" {
		return componentsYAMLOutput(components)
	}
	return componentsDefaultOutput(components)
}

func componentsYAMLOutput(c client.Components) error {
	yaml, err := c.Yaml()
	if err != nil {
		return err
	}

	fmt.Println(string(yaml))
	return err
}

func componentsDefaultOutput(c client.Components) error {
	fmt.Printf("Name:               %s\n", c.Name())
	fmt.Printf("Type:               %s\n", c.Type())
	fmt.Printf("URL:                %s\n", c.URL())
	fmt.Printf("Version:            %s\n", c.Version())
	fmt.Printf("TargetNamespace:    %s\n", c.TargetNamespace())
	fmt.Printf("WatchingNamespace:  %s\n", c.WatchingNamespace())
	if len(c.Variables()) > 0 {
		fmt.Println("Variables:")
		for _, v := range c.Variables() {
			fmt.Printf("  - %s\n", v)
		}
	}
	if len(c.Images()) > 0 {
		fmt.Println("Images:")
		for _, v := range c.Images() {
			fmt.Printf("  - %s\n", v)
		}
	}
	fmt.Println()

	return nil
}
