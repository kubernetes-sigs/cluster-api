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
	"io/ioutil"

	"github.com/openshift/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"github.com/openshift/cluster-api/cmd/clusterctl/phases"
	"github.com/spf13/cobra"
	"k8s.io/klog"
)

type AlphaPhasePivotOptions struct {
	SourceKubeconfig   string
	TargetKubeconfig   string
	ProviderComponents string
}

var ppo = &AlphaPhasePivotOptions{}

var alphaPhasePivotCmd = &cobra.Command{
	Use:   "pivot",
	Short: "Pivot",
	Long:  `Pivot`,
	Run: func(cmd *cobra.Command, args []string) {
		if ppo.ProviderComponents == "" {
			exitWithHelp(cmd, "Please provide yaml file for provider components definition.")
		}

		if ppo.SourceKubeconfig == "" {
			exitWithHelp(cmd, "Please provide a source kubeconfig file.")
		}

		if ppo.TargetKubeconfig == "" {
			exitWithHelp(cmd, "Please provide a target kubeconfig file.")
		}

		if err := RunAlphaPhasePivot(ppo); err != nil {
			klog.Exit(err)
		}
	},
}

func RunAlphaPhasePivot(ppo *AlphaPhasePivotOptions) error {
	sourceKubeconfig, err := ioutil.ReadFile(ppo.SourceKubeconfig)
	if err != nil {
		return err
	}

	targetKubeconfig, err := ioutil.ReadFile(ppo.TargetKubeconfig)
	if err != nil {
		return err
	}

	providerComponents, err := ioutil.ReadFile(ppo.ProviderComponents)
	if err != nil {
		return fmt.Errorf("error loading addons file '%v': %v", ppo.ProviderComponents, err)
	}

	clientFactory := clusterclient.NewFactory()
	sourceClient, err := clientFactory.NewClientFromKubeconfig(string(sourceKubeconfig))
	if err != nil {
		return fmt.Errorf("unable to create source cluster client: %v", err)
	}

	targetClient, err := clientFactory.NewClientFromKubeconfig(string(targetKubeconfig))
	if err != nil {
		return fmt.Errorf("unable to create target cluster client: %v", err)
	}

	if err := phases.Pivot(sourceClient, targetClient, string(providerComponents)); err != nil {
		return fmt.Errorf("unable to pivot Cluster API Components: %v", err)
	}

	return nil
}

func init() {
	// Required flags
	alphaPhasePivotCmd.Flags().StringVarP(&ppo.SourceKubeconfig, "source-kubeconfig", "s", "", "Path for the source kubeconfig file to use")
	alphaPhasePivotCmd.Flags().StringVarP(&ppo.TargetKubeconfig, "target-kubeconfig", "t", "", "Path for the target kubeconfig file to use")
	alphaPhasePivotCmd.Flags().StringVarP(&ppo.ProviderComponents, "provider-components", "p", "", "A yaml file containing provider components to apply to the cluster")
	alphaPhasesCmd.AddCommand(alphaPhasePivotCmd)
}
