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
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

/*
NOTE: This command is deprecated in favor of `clusterctl generate provider`. The source code is located at `cmd/clusterctl/cmd/generate_provider.go`.
This file will be removed in 0.5.x. Do not make any changes to this file.
*/

const (
	// ComponentsOutputYaml is an option used to print the components in yaml format.
	ComponentsOutputYaml = "yaml"
	// ComponentsOutputText is an option used to print the components in text format.
	ComponentsOutputText = "text"
)

var (
	// ComponentsOutputs is a list of valid components outputs.
	ComponentsOutputs = []string{ComponentsOutputText, ComponentsOutputYaml}
)

type configProvidersOptions struct {
	coreProvider           string
	bootstrapProvider      string
	controlPlaneProvider   string
	infrastructureProvider string
	output                 string
	targetNamespace        string
}

var cpo = &configProvidersOptions{}

var configProviderCmd = &cobra.Command{
	Use:   "provider",
	Args:  cobra.NoArgs,
	Short: "Display information about a provider.",
	Long: LongDesc(`
		Display information about a provider.

		clusterctl fetches the provider components from the provider repository;
		required template variables and the default namespace where the provider should be deployed are parsed from this file.`),

	Example: Examples(`
		# Displays information about a specific infrastructure provider.
		# If applicable, prints out the list of required environment variables.
		clusterctl config provider --infrastructure aws

		# Displays information about a specific version of the AWS infrastructure provider.
		clusterctl config provider --infrastructure aws:v0.4.1

		# Prints out the component file in yaml format for the given infrastructure provider.
		clusterctl config provider --infrastructure aws -o yaml

		# Prints out the component file in yaml format for the given infrastructure provider and version.
		clusterctl config provider --infrastructure aws:v0.4.1 -o yaml`),

	RunE: func(cmd *cobra.Command, args []string) error {
		return runGetComponents()
	},
	Deprecated: "use `clusterctl generate provider` instead",
}

func init() {
	configProviderCmd.Flags().StringVar(&cpo.coreProvider, "core", "",
		"Core provider and version (e.g. cluster-api:v0.3.0)")
	configProviderCmd.Flags().StringVarP(&cpo.infrastructureProvider, "infrastructure", "i", "",
		"Infrastructure provider and version (e.g. aws:v0.5.0)")
	configProviderCmd.Flags().StringVarP(&cpo.bootstrapProvider, "bootstrap", "b", "",
		"Bootstrap provider and version (e.g. kubeadm:v0.3.0)")
	configProviderCmd.Flags().StringVarP(&cpo.controlPlaneProvider, "control-plane", "c", "",
		"ControlPlane provider and version (e.g. kubeadm:v0.3.0)")

	configProviderCmd.Flags().StringVarP(&cpo.output, "output", "o", "text",
		fmt.Sprintf("Output format. Valid values: %v.", ComponentsOutputs))
	configProviderCmd.Flags().StringVar(&cpo.targetNamespace, "target-namespace", "",
		"The target namespace where the provider should be deployed. If unspecified, the components default namespace is used.")

	configCmd.AddCommand(configProviderCmd)
}

func runGetComponents() error {
	if cpo.output != ComponentsOutputYaml && cpo.output != ComponentsOutputText {
		return errors.Errorf("Invalid output format %q. Valid values: %v.", cpo.output, ComponentsOutputs)
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

	options := client.ComponentsOptions{
		TargetNamespace:     cpo.targetNamespace,
		SkipTemplateProcess: true,
	}
	components, err := c.GetProviderComponents(providerName, providerType, options)
	if err != nil {
		return err
	}
	return printComponents(components, cpo.output)
}

func printComponents(c client.Components, output string) error {
	switch output {
	case ComponentsOutputText:
		dir, file := filepath.Split(c.URL())
		// Remove the version suffix from the URL since we already display it
		// separately.
		baseURL, _ := filepath.Split(strings.TrimSuffix(dir, "/"))
		fmt.Printf("Name:               %s\n", c.Name())
		fmt.Printf("Type:               %s\n", c.Type())
		fmt.Printf("URL:                %s\n", baseURL)
		fmt.Printf("Version:            %s\n", c.Version())
		fmt.Printf("File:               %s\n", file)
		fmt.Printf("TargetNamespace:    %s\n", c.TargetNamespace())
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
	case ComponentsOutputYaml:
		yaml, err := c.Yaml()
		if err != nil {
			return err
		}

		if _, err := os.Stdout.Write(yaml); err != nil {
			return errors.Wrap(err, "failed to write yaml to Stdout")
		}
		if _, err := os.Stdout.WriteString("\n"); err != nil {
			return errors.Wrap(err, "failed to write trailing new line of yaml to Stdout")
		}
		return nil
	}
	return nil
}
