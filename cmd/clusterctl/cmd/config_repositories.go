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
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/yaml"
)

const (
	// RepositoriesOutputYaml is an option used to print the repository list in yaml format.
	RepositoriesOutputYaml = "yaml"
	// RepositoriesOutputText is an option used to print the repository list in text format.
	RepositoriesOutputText = "text"
	// RepositoriesOutputName is an option used to print the repository list by name only.
	RepositoriesOutputName = "name"

	// RepositoriesProviderBootstrap is an option used to filter the repository list by bootstrap provider.
	RepositoriesProviderBootstrap = "bootstrap"
	// RepositoriesProviderControlPlane is an option used to filter the repository list by controle-plane provider.
	RepositoriesProviderControlPlane = "control-plane"
	// RepositoriesProviderCore is an option used to filter the repository list by core provider.
	RepositoriesProviderCore = "core"
	// RepositoriesProviderInfrastructure is an option used to print the repository list by infrastructure provider.
	RepositoriesProviderInfrastructure = "infrastructure"
)

var (
	// RepositoriesOutputs is a list of valid repository list outputs.
	RepositoriesOutputs = map[string]interface{}{
		RepositoriesOutputYaml: nil,
		RepositoriesOutputText: nil,
		RepositoriesOutputName: nil,
	}

	// RepositoriesProviders is a list of valid repository list provider types.
	RepositoriesProviders = map[string]clusterctlv1.ProviderType{
		RepositoriesProviderBootstrap:      clusterctlv1.BootstrapProviderType,
		RepositoriesProviderControlPlane:   clusterctlv1.ControlPlaneProviderType,
		RepositoriesProviderCore:           clusterctlv1.CoreProviderType,
		RepositoriesProviderInfrastructure: clusterctlv1.InfrastructureProviderType,
	}
)

func configRepositoriesGetSupportedOutputs() []string {
	outputs := []string{}
	for output, _ := range RepositoriesOutputs {
		outputs = append(outputs, output)
	}
	return outputs
}

func configRepositoriesGetSupportedProviders() []string {
	providers := []string{}
	for provider, _ := range RepositoriesProviders {
		providers = append(providers, provider)
	}
	return providers
}

type configRepositoriesOptions struct {
	output   string
	provider string
}

var cro = &configRepositoriesOptions{}

var configRepositoryCmd = &cobra.Command{
	Use:   "repositories",
	Args:  cobra.NoArgs,
	Short: "Display the list of providers and their repository configurations.",
	Long: LongDesc(`
		Display the list of providers and their repository configurations.

		clusterctl ships with a list of known providers; if necessary, edit
		$HOME/.cluster-api/clusterctl.yaml file to add new provider or to customize existing ones.`),

	Example: Examples(`
		# Displays the list of available providers.
		clusterctl config repositories
		
		# Print the list of available providers in yaml format.
		clusterctl config repositories -o yaml

		# Displays the list of available infrastructure providers.
		clusterctl config repositories --provider infrastructure
`),

	RunE: func(cmd *cobra.Command, args []string) error {
		return runGetRepositories(cfgFile, os.Stdout)
	},
}

func init() {
	configRepositoryCmd.Flags().StringVarP(&cro.output, "output", "o", RepositoriesOutputText,
		fmt.Sprintf("Output format. Valid values: %v.", configRepositoriesGetSupportedOutputs()))
	configRepositoryCmd.Flags().StringVar(&cro.provider, "provider", "",
		fmt.Sprintf("Provider type. Valid values: %v.", configRepositoriesGetSupportedProviders()))
	configCmd.AddCommand(configRepositoryCmd)
}

func repositoriesFilterByType(repositoriesList []client.Provider, providerType clusterctlv1.ProviderType) []client.Provider {
	var ret []client.Provider

	for _, r := range repositoriesList {
		if r.Type() == providerType {
			ret = append(ret, r)
		}
	}

	return ret
}

func runGetRepositories(cfgFile string, out io.Writer) error {
	if _, ok := RepositoriesOutputs[cro.output]; !ok {
		return errors.Errorf("Invalid output format %q. Valid values: %v.", cro.output, configRepositoriesGetSupportedOutputs())
	}

	if cro.provider != "" {
		if _, ok := RepositoriesProviders[cro.provider]; !ok {
			return errors.Errorf("Invalid provider type %q. Valid values: %v.", cro.provider, configRepositoriesGetSupportedProviders())
		}
	}

	if out == nil {
		return errors.New("unable to print to nil output writer")
	}

	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	repositoryList, err := c.GetProvidersConfig()
	if err != nil {
		return err
	}

	if cro.provider != "" {
		repositoryList = repositoriesFilterByType(repositoryList, RepositoriesProviders[cro.provider])
	}

	w := tabwriter.NewWriter(out, 10, 4, 3, ' ', 0)

	switch cro.output {
	case RepositoriesOutputText:
		fmt.Fprintln(w, "NAME\tTYPE\tURL\tFILE")
		for _, r := range repositoryList {
			dir, file := filepath.Split(r.URL())
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name(), r.Type(), dir, file)
		}
	case RepositoriesOutputYaml:
		y, err := yaml.Marshal(repositoryList)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, string(y))
	case RepositoriesOutputName:
		for _, r := range repositoryList {
			fmt.Fprintf(w, "%s\n", r.Name())
		}
	}
	w.Flush()
	return nil
}
