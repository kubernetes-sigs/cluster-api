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
	"os"

	"fmt"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"sigs.k8s.io/cluster-api/tf-deployer/deploy"
)

type CreateOptions struct {
	Cluster          string
	Machine          string
	NamedMachinePath string
}

var co = &CreateOptions{}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create kubernetes cluster",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if co.Cluster == "" {
			glog.Error("Please provide yaml file for cluster definition.")
			cmd.Help()
			os.Exit(1)
		}
		if co.Machine == "" {
			glog.Error("Please provide yaml file for machine definition.")
			cmd.Help()
			os.Exit(1)
		}
		if co.NamedMachinePath == "" {
			glog.Error("Please provide a yaml file for machine HCL configs.")
			cmd.Help()
			os.Exit(1)
		}
		if err := RunCreate(co); err != nil {
			glog.Exit(err)
		}
	},
}

func RunCreate(co *CreateOptions) error {
	cluster, err := parseClusterYaml(co.Cluster)
	if err != nil {
		return fmt.Errorf("Error parsing cluster yaml: %+v", err)
	}

	machines, err := parseMachinesYaml(co.Machine)
	if err != nil {
		return fmt.Errorf("Error parsing machines yaml: %+v", err)
	}

	d := deploy.NewDeployer(kubeConfig, co.NamedMachinePath)

	return d.CreateCluster(cluster, machines)
}
func init() {
	createCmd.Flags().StringVarP(&co.Cluster, "cluster", "c", "", "cluster yaml file")
	createCmd.Flags().StringVarP(&co.Machine, "machines", "m", "", "machine yaml file")
	createCmd.Flags().StringVarP(&co.NamedMachinePath, "namedmachines", "n", "", "named machines yaml file")

	RootCmd.AddCommand(createCmd)
}
