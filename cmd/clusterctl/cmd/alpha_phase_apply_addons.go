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
	"fmt"
	"io/ioutil"

	"github.com/spf13/cobra"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
)

type AlphaPhaseApplyAddonsOptions struct {
	Kubeconfig string
	Addons     string
}

var paao = &AlphaPhaseApplyAddonsOptions{}

var alphaPhaseApplyAddonsCmd = &cobra.Command{
	Use:   "apply-addons",
	Short: "Apply Addons",
	Long:  `Apply Addons`,
	Run: func(cmd *cobra.Command, args []string) {
		if paao.Addons == "" {
			exitWithHelp(cmd, "Please provide yaml file for addons definition.")
		}

		if paao.Kubeconfig == "" {
			exitWithHelp(cmd, "Please provide a kubeconfig file.")
		}

		if err := RunAlphaPhaseApplyAddons(paao); err != nil {
			klog.Exit(err)
		}
	},
}

func RunAlphaPhaseApplyAddons(paao *AlphaPhaseApplyAddonsOptions) error {
	kubeconfig, err := ioutil.ReadFile(paao.Kubeconfig)
	if err != nil {
		return err
	}

	addons, err := ioutil.ReadFile(paao.Addons)
	if err != nil {
		return fmt.Errorf("error loading addons file '%v': %v", paao.Addons, err)
	}

	clientFactory := clusterclient.NewFactory()
	client, err := clientFactory.NewClientFromKubeconfig(string(kubeconfig))
	if err != nil {
		return fmt.Errorf("unable to create cluster client: %v", err)
	}

	if err := phases.ApplyAddons(client, string(addons)); err != nil {
		return fmt.Errorf("unable to apply addons: %v", err)
	}

	return nil
}

func init() {
	// Required flags
	alphaPhaseApplyAddonsCmd.Flags().StringVarP(&paao.Kubeconfig, "kubeconfig", "", "", "Path for the kubeconfig file to use")
	alphaPhaseApplyAddonsCmd.Flags().StringVarP(&paao.Addons, "addon-components", "a", "", "A yaml file containing cluster addons to apply to the cluster")
	alphaPhasesCmd.AddCommand(alphaPhaseApplyAddonsCmd)
}
