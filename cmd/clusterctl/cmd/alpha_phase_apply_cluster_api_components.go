/*
Copyright 2018 The Kubernetes Authors.

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
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
)

type AlphaPhaseApplyClusterAPIComponentsOptions struct {
	Kubeconfig         string
	ProviderComponents string
}

var pacaso = &AlphaPhaseApplyClusterAPIComponentsOptions{}

var alphaPhaseApplyClusterAPIComponentsCmd = &cobra.Command{
	Use:   "apply-cluster-api-components",
	Short: "Apply Cluster API components",
	Long:  `Apply Cluster API components`,
	Run: func(cmd *cobra.Command, args []string) {
		if pacaso.ProviderComponents == "" {
			exitWithHelp(cmd, "Please provide yaml file for provider component definition.")
		}

		if pacaso.Kubeconfig == "" {
			exitWithHelp(cmd, "Please provide a kubeconfig file.")
		}

		if err := RunAlphaPhaseApplyClusterAPIComponents(pacaso); err != nil {
			klog.Exit(err)
		}
	},
}

func RunAlphaPhaseApplyClusterAPIComponents(pacaso *AlphaPhaseApplyClusterAPIComponentsOptions) error {
	kubeconfig, err := ioutil.ReadFile(pacaso.Kubeconfig)
	if err != nil {
		return err
	}

	pc, err := ioutil.ReadFile(pacaso.ProviderComponents)
	if err != nil {
		return errors.Wrapf(err, "error loading provider components file %q", pacaso.ProviderComponents)
	}

	clientFactory := clusterclient.NewFactory()
	client, err := clientFactory.NewClientFromKubeconfig(string(kubeconfig))
	if err != nil {
		return errors.Wrap(err, "unable to create cluster client")
	}

	return phases.ApplyClusterAPIComponents(client, string(pc))
}

func init() {
	// Required flags
	alphaPhaseApplyClusterAPIComponentsCmd.Flags().StringVarP(&pacaso.Kubeconfig, "kubeconfig", "", "", "Path for the kubeconfig file to use")
	alphaPhaseApplyClusterAPIComponentsCmd.Flags().StringVarP(&pacaso.ProviderComponents, "provider-components", "p", "", "A yaml file containing cluster api provider controllers and supporting objects")
	alphaPhasesCmd.AddCommand(alphaPhaseApplyClusterAPIComponentsCmd)
}
