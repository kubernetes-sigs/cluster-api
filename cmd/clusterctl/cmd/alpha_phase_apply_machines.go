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
	"sigs.k8s.io/cluster-api/pkg/util"
)

type AlphaPhaseApplyMachinesOptions struct {
	Kubeconfig string
	Machines   string
	Namespace  string
}

var pamo = &AlphaPhaseApplyMachinesOptions{}

var alphaPhaseApplyMachinesCmd = &cobra.Command{
	Use:   "apply-machines",
	Short: "Apply Machines",
	Long:  `Apply Machines`,
	Run: func(cmd *cobra.Command, args []string) {
		if pamo.Machines == "" {
			exitWithHelp(cmd, "Please provide yaml file for machines definition.")
		}

		if pamo.Kubeconfig == "" {
			exitWithHelp(cmd, "Please provide a kubeconfig file.")
		}

		if err := RunAlphaPhaseApplyMachines(pamo); err != nil {
			klog.Exit(err)
		}
	},
}

func RunAlphaPhaseApplyMachines(pamo *AlphaPhaseApplyMachinesOptions) error {
	kubeconfig, err := ioutil.ReadFile(pamo.Kubeconfig)
	if err != nil {
		return err
	}

	machines, err := util.ParseMachinesYaml(pamo.Machines)
	if err != nil {
		return err
	}

	clientFactory := clusterclient.NewFactory()
	client, err := clientFactory.NewClientFromKubeconfig(string(kubeconfig))
	if err != nil {
		return errors.Wrap(err, "unable to create cluster client")
	}

	if err := phases.ApplyMachines(client, pamo.Namespace, machines); err != nil {
		return errors.Wrap(err, "unable to apply machines")
	}

	return nil
}

func init() {
	// Required flags
	alphaPhaseApplyMachinesCmd.Flags().StringVarP(&pamo.Kubeconfig, "kubeconfig", "", "", "Path for the kubeconfig file to use")
	alphaPhaseApplyMachinesCmd.Flags().StringVarP(&pamo.Machines, "machines", "m", "", "A yaml file containing machine object definitions")

	// Optional flags
	alphaPhaseApplyMachinesCmd.Flags().StringVarP(&pamo.Namespace, "namespace", "n", "", "Namespace")
	alphaPhasesCmd.AddCommand(alphaPhaseApplyMachinesCmd)
}
