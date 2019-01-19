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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
)

type AlphaPhaseCreateBootstrapClusterOptions struct {
	Bootstrap        bootstrap.Options
	KubeconfigOutput string
}

var pcbco = &AlphaPhaseCreateBootstrapClusterOptions{}

var alphaPhaseCreateBootstrapClusterCmd = &cobra.Command{
	Use:   "create-bootstrap-cluster",
	Short: "Create a bootstrap cluster",
	Long:  `Create a bootstrap cluster`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunAlphaPhaseCreateBootstrapCluster(pcbco); err != nil {
			klog.Exit(err)
		}
	},
}

func RunAlphaPhaseCreateBootstrapCluster(pcbco *AlphaPhaseCreateBootstrapClusterOptions) error {
	bootstrapProvider, err := bootstrap.Get(pcbco.Bootstrap)
	if err != nil {
		return err
	}

	_, _, err = phases.CreateBootstrapCluster(bootstrapProvider, false, clusterclient.NewFactory())
	if err != nil {
		return errors.Wrap(err, "failed to create bootstrap cluster")
	}

	klog.Infof("Created bootstrap cluster, path to kubeconfig: %q", pcbco.KubeconfigOutput)
	return nil
}

func init() {
	// Optional flags
	alphaPhaseCreateBootstrapClusterCmd.Flags().StringVarP(&pcbco.KubeconfigOutput, "kubeconfig-out", "", "minikube.kubeconfig", "Where to output the kubeconfig for the bootstrap cluster")
	pcbco.Bootstrap.AddFlags(alphaPhasesCmd.Flags())
	alphaPhasesCmd.AddCommand(alphaPhaseCreateBootstrapClusterCmd)
}
