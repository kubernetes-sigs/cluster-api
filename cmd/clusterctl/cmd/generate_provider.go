/*
Copyright 2021 The Kubernetes Authors.

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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type generateProvidersOptions struct {
	coreProvider             string
	bootstrapProvider        string
	controlPlaneProvider     string
	infrastructureProvider   string
	ipamProvider             string
	runtimeExtensionProvider string
	addonProvider            string
	targetNamespace          string
	textOutput               bool
	raw                      bool
	outputFile               string
}

var gpo = &generateProvidersOptions{}

var generateProviderCmd = &cobra.Command{
	Use:   "provider",
	Args:  cobra.NoArgs,
	Short: "Generate templates for provider components",
	Long: LongDesc(`
		Generate templates for provider components.

		clusterctl fetches the provider components from the provider repository and performs variable substitution.
		
		Variable values are either sourced from the clusterctl config file or
		from environment variables`),

	Example: Examples(`
		# Generates a yaml file for creating provider with variable values using
        # components defined in the provider repository.
		clusterctl generate provider --infrastructure aws

		# Generates a yaml file for creating provider for a specific version with variable values using
        # components defined in the provider repository.
		clusterctl generate provider --infrastructure aws:v0.4.1

		# Displays information about a specific infrastructure provider.
		# If applicable, prints out the list of required environment variables.
		clusterctl generate provider --infrastructure aws --describe

		# Displays information about a specific version of the infrastructure provider.
		clusterctl generate provider --infrastructure aws:v0.4.1 --describe

		# Generates a yaml file for creating provider for a specific version.
		# No variables will be processed and substituted using this flag
		clusterctl generate provider --infrastructure aws:v0.4.1 --raw`),

	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenerateProviderComponents()
	},
}

func init() {
	generateProviderCmd.Flags().StringVar(&gpo.coreProvider, "core", "",
		"Core provider and version (e.g. cluster-api:v1.1.5)")
	generateProviderCmd.Flags().StringVarP(&gpo.infrastructureProvider, "infrastructure", "i", "",
		"Infrastructure provider and version (e.g. aws:v0.5.0)")
	generateProviderCmd.Flags().StringVarP(&gpo.bootstrapProvider, "bootstrap", "b", "",
		"Bootstrap provider and version (e.g. kubeadm:v1.1.5)")
	generateProviderCmd.Flags().StringVarP(&gpo.controlPlaneProvider, "control-plane", "c", "",
		"ControlPlane provider and version (e.g. kubeadm:v1.1.5)")
	generateProviderCmd.Flags().StringVar(&gpo.ipamProvider, "ipam", "",
		"IPAM provider and version (e.g. infoblox:v0.0.1)")
	generateProviderCmd.Flags().StringVar(&gpo.runtimeExtensionProvider, "runtime-extension", "",
		"Runtime extension provider and version (e.g. test:v0.0.1)")
	generateProviderCmd.Flags().StringVar(&gpo.addonProvider, "addon", "",
		"Add-on provider and version (e.g. helm:v0.1.0)")
	generateProviderCmd.Flags().StringVarP(&gpo.targetNamespace, "target-namespace", "n", "",
		"The target namespace where the provider should be deployed. If unspecified, the components default namespace is used.")
	generateProviderCmd.Flags().BoolVar(&gpo.textOutput, "describe", false,
		"Generate configuration without variable substitution.")
	generateProviderCmd.Flags().BoolVar(&gpo.raw, "raw", false,
		"Generate configuration without variable substitution in a yaml format.")

	generateProviderCmd.Flags().StringVar(&gpo.outputFile, "write-to", "", "Specify the output file to write the template to, defaults to STDOUT if the flag is not set")

	generateCmd.AddCommand(generateProviderCmd)
}

func runGenerateProviderComponents() error {
	providerName, providerType, err := parseProvider()
	if err != nil {
		return err
	}
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	options := client.ComponentsOptions{
		TargetNamespace:     gpo.targetNamespace,
		SkipTemplateProcess: gpo.raw || gpo.textOutput,
	}

	components, err := c.GenerateProvider(providerName, providerType, options)
	if err != nil {
		return err
	}

	if gpo.textOutput {
		return printComponentsAsText(components)
	}

	return printYamlOutput(components, gpo.outputFile)
}

// parseProvider parses command line flags and returns the provider name and type.
func parseProvider() (string, clusterctlv1.ProviderType, error) {
	providerName := gpo.coreProvider
	providerType := clusterctlv1.CoreProviderType
	if gpo.bootstrapProvider != "" {
		if providerName != "" {
			return "", "", errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure, --ipam, --extension, --addon should be set")
		}
		providerName = gpo.bootstrapProvider
		providerType = clusterctlv1.BootstrapProviderType
	}
	if gpo.controlPlaneProvider != "" {
		if providerName != "" {
			return "", "", errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure, --ipam, --extension, --addon should be set")
		}
		providerName = gpo.controlPlaneProvider
		providerType = clusterctlv1.ControlPlaneProviderType
	}
	if gpo.infrastructureProvider != "" {
		if providerName != "" {
			return "", "", errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure, --ipam, --extension, --addon should be set")
		}
		providerName = gpo.infrastructureProvider
		providerType = clusterctlv1.InfrastructureProviderType
	}
	if gpo.ipamProvider != "" {
		if providerName != "" {
			return "", "", errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure, --ipam, --extension, --addon should be set")
		}
		providerName = gpo.ipamProvider
		providerType = clusterctlv1.IPAMProviderType
	}
	if gpo.runtimeExtensionProvider != "" {
		if providerName != "" {
			return "", "", errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure, --ipam, --extension, --addon should be set")
		}
		providerName = gpo.runtimeExtensionProvider
		providerType = clusterctlv1.RuntimeExtensionProviderType
	}
	if gpo.addonProvider != "" {
		if providerName != "" {
			return "", "", errors.New("only one of --core, --bootstrap, --control-plane, --infrastructure, --ipam, --extension, --addon should be set")
		}
		providerName = gpo.addonProvider
		providerType = clusterctlv1.AddonProviderType
	}
	if providerName == "" {
		return "", "", errors.New("at least one of --core, --bootstrap, --control-plane, --infrastructure, --ipam, --extension, --addon should be set")
	}

	return providerName, providerType, nil
}
