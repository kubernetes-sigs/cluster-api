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
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type initOptions struct {
	kubeconfig              string
	kubeconfigContext       string
	coreProvider            string
	bootstrapProviders      []string
	controlPlaneProviders   []string
	infrastructureProviders []string
	targetNamespace         string
	listImages              bool
	waitProviders           bool
	waitProviderTimeout     int
}

var initOpts = &initOptions{}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a management cluster.",
	Long: LongDesc(`
		Initialize a management cluster.

		Installs Cluster API core components, the kubeadm bootstrap provider,
		and the selected bootstrap and infrastructure providers.

		The management cluster must be an existing Kubernetes cluster, make sure
		to have enough privileges to install the desired components.

		Use 'clusterctl config repositories' to get a list of available providers; if necessary, edit
		$HOME/.cluster-api/clusterctl.yaml file to add new provider or to customize existing ones.

		Some providers require environment variables to be set before running clusterctl init.
		Refer to the provider documentation, or use 'clusterctl config provider [name]' to get a list of required variables.

		See https://cluster-api.sigs.k8s.io for more details.`),

	Example: Examples(`
		# Initialize a management cluster, by installing the given infrastructure provider.
		#
		# Note: when this command is executed on an empty management cluster,
 		#       it automatically triggers the installation of the Cluster API core provider.
		clusterctl init --infrastructure=aws

		# Initialize a management cluster with a specific version of the given infrastructure provider.
		clusterctl init --infrastructure=aws:v0.4.1

		# Initialize a management cluster with a custom kubeconfig path and the given infrastructure provider.
		clusterctl init --kubeconfig=foo.yaml  --infrastructure=aws

		# Initialize a management cluster with multiple infrastructure providers.
		clusterctl init --infrastructure=aws,vsphere

		# Initialize a management cluster with a custom target namespace for the provider resources.
		clusterctl init --infrastructure aws --target-namespace foo

		# Lists the container images required for initializing the management cluster.
		#
		# Note: This command is a dry-run; it won't perform any action other than printing to screen.
		clusterctl init --infrastructure aws --list-images`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

func init() {
	initCmd.Flags().StringVar(&initOpts.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig for the management cluster. If unspecified, default discovery rules apply.")
	initCmd.Flags().StringVar(&initOpts.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	initCmd.Flags().StringVar(&initOpts.coreProvider, "core", "",
		"Core provider version (e.g. cluster-api:v0.3.0) to add to the management cluster. If unspecified, Cluster API's latest release is used.")
	initCmd.Flags().StringSliceVarP(&initOpts.infrastructureProviders, "infrastructure", "i", nil,
		"Infrastructure providers and versions (e.g. aws:v0.5.0) to add to the management cluster.")
	initCmd.Flags().StringSliceVarP(&initOpts.bootstrapProviders, "bootstrap", "b", nil,
		"Bootstrap providers and versions (e.g. kubeadm:v0.3.0) to add to the management cluster. If unspecified, Kubeadm bootstrap provider's latest release is used.")
	initCmd.Flags().StringSliceVarP(&initOpts.controlPlaneProviders, "control-plane", "c", nil,
		"Control plane providers and versions (e.g. kubeadm:v0.3.0) to add to the management cluster. If unspecified, the Kubeadm control plane provider's latest release is used.")
	initCmd.Flags().StringVar(&initOpts.targetNamespace, "target-namespace", "",
		"The target namespace where the providers should be deployed. If unspecified, the provider components' default namespace is used.")
	initCmd.Flags().BoolVar(&initOpts.waitProviders, "wait-providers", false,
		"Wait for providers to be installed.")
	initCmd.Flags().IntVar(&initOpts.waitProviderTimeout, "wait-provider-timeout", 5*60,
		"Wait timeout per provider installation in seconds. This value is ignored if --wait-providers is false")

	// TODO: Move this to a sub-command or similar, it shouldn't really be a flag.
	initCmd.Flags().BoolVar(&initOpts.listImages, "list-images", false,
		"Lists the container images required for initializing the management cluster (without actually installing the providers)")

	RootCmd.AddCommand(initCmd)
}

func runInit() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	options := client.InitOptions{
		Kubeconfig:              client.Kubeconfig{Path: initOpts.kubeconfig, Context: initOpts.kubeconfigContext},
		CoreProvider:            initOpts.coreProvider,
		BootstrapProviders:      initOpts.bootstrapProviders,
		ControlPlaneProviders:   initOpts.controlPlaneProviders,
		InfrastructureProviders: initOpts.infrastructureProviders,
		TargetNamespace:         initOpts.targetNamespace,
		LogUsageInstructions:    true,
		WaitProviders:           initOpts.waitProviders,
		WaitProviderTimeout:     time.Duration(initOpts.waitProviderTimeout) * time.Second,
	}

	if initOpts.listImages {
		images, err := c.InitImages(options)
		if err != nil {
			return err
		}

		for _, i := range images {
			fmt.Println(i)
		}
		return nil
	}

	if _, err := c.Init(options); err != nil {
		return err
	}
	return nil
}
