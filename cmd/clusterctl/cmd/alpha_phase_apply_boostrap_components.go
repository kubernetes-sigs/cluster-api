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
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
)

type AlphaPhaseApplyBootstrapComponentsOptions struct {
	Kubeconfig          string
	BootstrapComponents string
}

var pabco = &AlphaPhaseApplyBootstrapComponentsOptions{}

var alphaPhaseApplyBootstrapComponentsCmd = &cobra.Command{
	Use:   "apply-bootstrap-components",
	Short: "Apply boostrap components",
	Long:  `Apply bootstrap components`,
	Run: func(cmd *cobra.Command, args []string) {
		if pabco.BootstrapComponents == "" {
			exitWithHelp(cmd, "Please provide yaml file for bootstrap component definition.")
		}

		if pabco.Kubeconfig == "" {
			exitWithHelp(cmd, "Please provide a kubeconfig file.")
		}

		if err := RunAlphaPhaseApplyBootstrapComponents(pabco); err != nil {
			klog.Exit(err)
		}
	},
}

func RunAlphaPhaseApplyBootstrapComponents(pabco *AlphaPhaseApplyBootstrapComponentsOptions) error {
	kubeconfig, err := ioutil.ReadFile(pabco.Kubeconfig)
	if err != nil {
		return err
	}

	pc, err := ioutil.ReadFile(pabco.BootstrapComponents)
	if err != nil {
		return errors.Wrapf(err, "error loading provider components file %q", pabco.BootstrapComponents)
	}

	clientFactory := clusterclient.NewFactory()
	client, err := clientFactory.NewClientFromKubeconfig(string(kubeconfig))
	if err != nil {
		return errors.Wrap(err, "unable to create cluster client")
	}

	return phases.ApplyBootstrapComponents(client, string(pc))
}

func init() {
	// Required flags
	alphaPhaseApplyBootstrapComponentsCmd.Flags().StringVarP(&pabco.Kubeconfig, "kubeconfig", "", "", "Path for the kubeconfig file to use")
	alphaPhaseApplyBootstrapComponentsCmd.Flags().StringVarP(&pabco.BootstrapComponents, "bootstrap-components", "b", "", "A yaml file containing bootstrap cluster components")
	alphaPhasesCmd.AddCommand(alphaPhaseApplyBootstrapComponentsCmd)
}
