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

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client"
)

type configProvidersOptions struct {
	coreProvider           string
	bootstrapProvider      string
	controlPlaneProvider   string
	infrastructureProvider string
	output                 string
	targetNamespace        string
	watchingNamespace      string
}

var cpo = &configProvidersOptions{}

var configProviderCmd = &cobra.Command{
	Use:   "provider",
	Args:  cobra.NoArgs,
	Short: "Display information about a Cluster API provider",
	Long: LongDesc(`
		Display information about a Cluster API provider.

		clusterctl fetch the provider components yaml from the provider repository; required variables
		and the default namespace where the provider should be deployed are derived from this file.`),

	Example: Examples(`
		# Displays relevant information about the AWS infrastructure provider, including also the list of 
		# required env variables, if any.
		clusterctl config provider --infrastructure aws

		# Displays relevant information about a specific version of the AWS infrastructure provider.
		clusterctl config provider --infrastructure aws:v0.4.1

		# Displays the yaml for creating provider components for the AWS infrastructure provider.
		clusterctl config provider --infrastructure aws -o yaml

		# Displays the yaml for creating provider components for a specific version of the AWS infrastructure provider.
		clusterctl config provider --infrastructure aws:v0.4.1 -o yaml`),

	RunE: func(cmd *cobra.Command, args []string) error {
		return runGetComponents()
	},
}

func init() {
	configProviderCmd.Flags().StringVarP(&cpo.coreProvider, "core", "", "", "Core provider version (e.g. cluster-api:v0.3.0)")
	configProviderCmd.Flags().StringVarP(&cpo.infrastructureProvider, "infrastructure", "i", "", "Infrastructure provider and version (e.g. aws:v0.5.0)")
	configProviderCmd.Flags().StringVarP(&cpo.bootstrapProvider, "bootstrap", "b", "", "Bootstrap provider and version (e.g. kubeadm:v0.3.0)")
	configProviderCmd.Flags().StringVarP(&cpo.controlPlaneProvider, "control-plane", "c", "", "ControlPlane provider and version (e.g. kubeadm:v0.3.0)")

	configProviderCmd.Flags().StringVarP(&cpo.output, "output", "o", "text", "Output format. One of [yaml, text]")
	configProviderCmd.Flags().StringVarP(&cpo.targetNamespace, "target-namespace", "", "", "The target namespace where the provider should be deployed. If not specified, a default namespace will be used")
	configProviderCmd.Flags().StringVarP(&cpo.watchingNamespace, "watching-namespace", "", "", "Namespace that the provider should watch to reconcile Cluster API objects. If unspecified, the provider watches for Cluster API objects across all namespaces")

	configCmd.AddCommand(configProviderCmd)
}

func runGetComponents() error {
	if !(cpo.output == "" || cpo.output == "yaml" || cpo.output == "text") { //nolint goconst
		return errors.New("please provide a valid output. Supported values are [ text, yaml ]")
	}

	providerName := cpo.coreProvider
	providerType := clusterctlv1.CoreProviderType
	if cpo.bootstrapProvider != "" {
		if providerName != "" {
			return errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure should be set")
		}
		providerName = cpo.bootstrapProvider
		providerType = clusterctlv1.BootstrapProviderType
	}
	if cpo.controlPlaneProvider != "" {
		if providerName != "" {
			return errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure should be set")
		}
		providerName = cpo.controlPlaneProvider
		providerType = clusterctlv1.ControlPlaneProviderType
	}
	if cpo.infrastructureProvider != "" {
		if providerName != "" {
			return errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure should be set")
		}
		providerName = cpo.infrastructureProvider
		providerType = clusterctlv1.InfrastructureProviderType
	}
	if providerName == "" {
		return errors.New("at least one of --core, --bootstrap, --control-plane, --infrastructure should be set")
	}

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	components, err := c.GetProviderComponents(providerName, providerType, cpo.targetNamespace, cpo.watchingNamespace)
	if err != nil {
		return err
	}

	if cpo.output == "yaml" {
		return componentsYAMLOutput(components)
	}
	return componentsDefaultOutput(components)
}

func componentsYAMLOutput(c client.Components) error {
	yaml, err := c.Yaml()
	if err != nil {
		return err
	}

	if _, err := os.Stdout.Write(yaml); err != nil {
		return errors.Wrap(err, "failed to write yaml to Stdout")
	}
	os.Stdout.WriteString("\n")
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
