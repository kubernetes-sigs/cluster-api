/*
Copyright 2022 The Kubernetes Authors.

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

	"github.com/spf13/cobra"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client"
)

var initListImagesCmd = &cobra.Command{
	Use:   "list-images",
	Short: "Lists the container images required for initializing the management cluster",
	Long: LongDesc(`
		Lists the container images required for initializing the management cluster.

		See https://cluster-api.sigs.k8s.io for more details.`),

	Example: Examples(`
		# Lists the container images required for initializing the management cluster.
		#
		# Note: This command is a dry-run; it won't perform any action other than printing to screen.
		clusterctl init list-images --infrastructure aws

		# List infrastructure, bootstrap, control-plane and core images
		clusterctl init list-images --infrastructure vcd --bootstrap kubeadm --control-plane nested --core cluster-api:v1.2.0
	`),
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInitListImages()
	},
}

func runInitListImages() error {
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
		LogUsageInstructions:      false,
	}

	images, err := c.InitImages(options)
	if err != nil {
		return err
	}

	for _, i := range images {
		fmt.Println(i)
	}
	return nil
}
