/*
Copyright 2017 The Kubernetes Authors.

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
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"github.com/spf13/cobra"
	"k8s.io/kube-deploy/cluster-api/deploy"
	"github.com/golang/glog"
)

type UpgradeOptions struct {
	KubernetesVersion string
}

var uo = &UpgradeOptions{}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade Kubernetes cluster.",
	Long:  `Upgrade the kubernetes control plan and nodes to the specified version.`,
	Run: func(cmd *cobra.Command, args []string) {
		if uo.KubernetesVersion == "" {
			glog.Exit("Please provide new kubernetes version.")
		}

		if err := RunUpgrade(uo); err != nil {
			glog.Exit(err.Error())
		}
	},
}

func RunUpgrade(uo *UpgradeOptions) error {
	err := deploy.UpgradeCluster(uo.KubernetesVersion, kubeConfig)
	if err != nil {
		glog.Errorf("Failed to upgrade cluster with error : %v", err)
	}
	return err
}

func init() {
	upgradeCmd.Flags().StringVarP(&uo.KubernetesVersion, "version", "v", "", "Kubernetes Version")

	RootCmd.AddCommand(upgradeCmd)
}

