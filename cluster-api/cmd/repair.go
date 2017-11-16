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

	"github.com/spf13/cobra"
	"k8s.io/kube-deploy/cluster-api/deploy"
	"github.com/golang/glog"
)

type RepairOptions struct {
	dryRun bool
}

var ro = &RepairOptions{}

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Repair node",
	Long:  `Repairs given node`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunRepair(ro); err != nil {
			glog.Exit(err)
		}
	},
}

func RunRepair(ro *RepairOptions) error {
	d := deploy.NewDeployer(provider, kubeConfig)
	return d.RepairNode(ro.dryRun)
}

func init() {
	repairCmd.Flags().BoolVarP(&ro.dryRun, "dryrun", "", true, "dry run mode. Defaults to true")
	RootCmd.AddCommand(repairCmd)
}

