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
	"time"

	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

type initOptions struct {
	kubeconfig                string
	kubeconfigContext         string
	coreProvider              string
	bootstrapProviders        []string
	controlPlaneProviders     []string
	infrastructureProviders   []string
	ipamProviders             []string
	runtimeExtensionProviders []string
	targetNamespace           string
	validate                  bool
	waitProviders             bool
	waitProviderTimeout       int
}

var initOpts = &initOptions{}

var initCmd = &cobra.Command{
	Use:     "init",
	GroupID: groupManagement,
	Short:   "Initialize a management cluster",
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
		clusterctl init --infrastructure aws --target-namespace foo`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

func init() {
	initCmd.PersistentFlags().StringVar(&initOpts.kubeconfig, "kubeconfig", "",
		"Path to the kubeconfig for the management cluster. If unspecified, default discovery rules apply.")
	initCmd.PersistentFlags().StringVar(&initOpts.kubeconfigContext, "kubeconfig-context", "",
		"Context to be used within the kubeconfig file. If empty, current context will be used.")
	initCmd.PersistentFlags().StringVar(&initOpts.coreProvider, "core", "",
		"Core provider version (e.g. cluster-api:v1.1.5) to add to the management cluster. If unspecified, Cluster API's latest release is used.")
	initCmd.PersistentFlags().StringSliceVarP(&initOpts.infrastructureProviders, "infrastructure", "i", nil,
		"Infrastructure providers and versions (e.g. aws:v0.5.0) to add to the management cluster.")
	initCmd.PersistentFlags().StringSliceVarP(&initOpts.bootstrapProviders, "bootstrap", "b", nil,
		"Bootstrap providers and versions (e.g. kubeadm:v1.1.5) to add to the management cluster. If unspecified, Kubeadm bootstrap provider's latest release is used.")
	initCmd.PersistentFlags().StringSliceVarP(&initOpts.controlPlaneProviders, "control-plane", "c", nil,
		"Control plane providers and versions (e.g. kubeadm:v1.1.5) to add to the management cluster. If unspecified, the Kubeadm control plane provider's latest release is used.")
	initCmd.PersistentFlags().StringSliceVar(&initOpts.ipamProviders, "ipam", nil,
		"IPAM providers and versions (e.g. infoblox:v0.0.1) to add to the management cluster.")
	initCmd.PersistentFlags().StringSliceVar(&initOpts.runtimeExtensionProviders, "runtime-extension", nil,
		"Runtime extension providers and versions (e.g. test:v0.0.1) to add to the management cluster.")
	initCmd.Flags().StringVarP(&initOpts.targetNamespace, "target-namespace", "n", "",
		"The target namespace where the providers should be deployed. If unspecified, the provider components' default namespace is used.")
	initCmd.Flags().BoolVar(&initOpts.waitProviders, "wait-providers", false,
		"Wait for providers to be installed.")
	initCmd.Flags().IntVar(&initOpts.waitProviderTimeout, "wait-provider-timeout", 5*60,
		"Wait timeout per provider installation in seconds. This value is ignored if --wait-providers is false")
	initCmd.Flags().BoolVar(&initOpts.validate, "validate", true,
		"If true, clusterctl will validate that the deployments will succeed on the management cluster.")

	initCmd.AddCommand(initListImagesCmd)
	RootCmd.AddCommand(initCmd)
}

func runInit() error {
	c, err := client.New(cfgFile)
	if err != nil {
		return err
	}

	options := client.InitOptions{
		Kubeconfig:                client.Kubeconfig{Path: initOpts.kubeconfig, Context: initOpts.kubeconfigContext},
		CoreProvider:              initOpts.coreProvider,
		BootstrapProviders:        initOpts.bootstrapProviders,
		ControlPlaneProviders:     initOpts.controlPlaneProviders,
		InfrastructureProviders:   initOpts.infrastructureProviders,
		IPAMProviders:             initOpts.ipamProviders,
		RuntimeExtensionProviders: initOpts.runtimeExtensionProviders,
		TargetNamespace:           initOpts.targetNamespace,
		LogUsageInstructions:      true,
		WaitProviders:             initOpts.waitProviders,
		WaitProviderTimeout:       time.Duration(initOpts.waitProviderTimeout) * time.Second,
		IgnoreValidationErrors:    !initOpts.validate,
	}

	if _, err := c.Init(options); err != nil {
		return err
	}
	return nil
}
