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

	"github.com/spf13/cobra"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
)

type AlphaPhaseGetKubeconfigOptions struct {
	ClusterName      string
	Kubeconfig       string
	KubeconfigOutput string
	Namespace        string
	Provider         string
}

var pgko = &AlphaPhaseGetKubeconfigOptions{}

var alphaPhaseGetKubeconfigCmd = &cobra.Command{
	Use:   "get-kubeconfig",
	Short: "Get Kubeconfig",
	Long:  `Get Kubeconfig`,
	Run: func(cmd *cobra.Command, args []string) {
		if pgko.Kubeconfig == "" {
			exitWithHelp(cmd, "Please provide a kubeconfig file.")
		}

		if pgko.Provider == "" {
			exitWithHelp(cmd, "Please specify a provider.")
		}

		if pgko.ClusterName == "" {
			exitWithHelp(cmd, "Please specify a cluster name.")
		}

		if err := RunAlphaPhaseGetKubeconfig(pgko); err != nil {
			klog.Exit(err)
		}
	},
}

func RunAlphaPhaseGetKubeconfig(pgko *AlphaPhaseGetKubeconfigOptions) error {
	kubeconfig, err := ioutil.ReadFile(pgko.Kubeconfig)
	if err != nil {
		return err
	}

	clientFactory := clusterclient.NewFactory()
	client, err := clientFactory.NewClientFromKubeconfig(string(kubeconfig))
	if err != nil {
		return fmt.Errorf("unable to create cluster client: %v", err)
	}

	provider, err := getProvider(pgko.Provider)
	if err != nil {
		return err
	}

	if _, err := phases.GetKubeconfig(client, provider, pgko.KubeconfigOutput, pgko.ClusterName, pgko.Namespace); err != nil {
		return fmt.Errorf("unable to get kubeconfig: %v", err)
	}

	return nil
}

func init() {
	// Required flags
	alphaPhaseGetKubeconfigCmd.Flags().StringVarP(&pgko.Kubeconfig, "kubeconfig", "", "", "Path for the kubeconfig file to use")
	alphaPhaseGetKubeconfigCmd.Flags().StringVarP(&pgko.ClusterName, "cluster-name", "", "", "Cluster Name")
	// TODO: Remove as soon as code allows https://github.com/kubernetes-sigs/cluster-api/issues/157
	alphaPhaseGetKubeconfigCmd.Flags().StringVarP(&pgko.Provider, "provider", "", "", "Which provider deployment logic to use")

	// Optional flags
	alphaPhaseGetKubeconfigCmd.Flags().StringVarP(&pgko.KubeconfigOutput, "kubeconfig-out", "", "kubeconfig", "Where to output the kubeconfig for the provisioned cluster")
	alphaPhaseGetKubeconfigCmd.Flags().StringVarP(&pgko.Namespace, "namespace", "n", "", "Namespace")
	alphaPhasesCmd.AddCommand(alphaPhaseGetKubeconfigCmd)
}
