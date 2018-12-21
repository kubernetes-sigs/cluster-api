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

	"github.com/spf13/cobra"
	"k8s.io/klog"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/bootstrap/minikube"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/clusterdeployer/clusterclient"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/phases"
)

type AlphaPhaseCreateBootstrapClusterOptions struct {
	MiniKube         []string
	VmDriver         string
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
	if pcbco.VmDriver != "" {
		pcbco.MiniKube = append(pcbco.MiniKube, fmt.Sprintf("vm-driver=%s", pcbco.VmDriver))
	}

	bootstrapProvider := minikube.WithOptionsAndKubeConfigPath(pcbco.MiniKube, pcbco.KubeconfigOutput)

	_, _, err := phases.CreateBootstrapCluster(bootstrapProvider, false, clusterclient.NewFactory())
	if err != nil {
		return fmt.Errorf("failed to create bootstrap cluster: %v", err)
	}

	klog.Infof("Created bootstrap cluster, path to kubeconfig: %q", pcbco.KubeconfigOutput)
	return nil
}

func init() {
	// Optional flags
	alphaPhaseCreateBootstrapClusterCmd.Flags().StringSliceVarP(&pcbco.MiniKube, "minikube", "", []string{}, "Minikube options")
	alphaPhaseCreateBootstrapClusterCmd.Flags().StringVarP(&pcbco.VmDriver, "vm-driver", "", "", "Which vm driver to use for minikube")
	alphaPhaseCreateBootstrapClusterCmd.Flags().StringVarP(&pcbco.KubeconfigOutput, "kubeconfig-out", "", "minikube.kubeconfig", "Where to output the kubeconfig for the bootstrap cluster")

	alphaPhasesCmd.AddCommand(alphaPhaseCreateBootstrapClusterCmd)
}
