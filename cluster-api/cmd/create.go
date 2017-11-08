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
	"log"
	"os"
)

type CreateOptions struct {
	Cluster                 string
	Machine                 string
	EnableMachineController bool
}

var co = &CreateOptions{}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Simple kubernetes cluster creator",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if co.Cluster == "" {
			log.Print("Please provide yaml file for cluster definition." )
			cmd.Help()
			os.Exit(1)
		}
		if co.Machine == "" {
			log.Print("Please provide yaml file for machine definition.")
			cmd.Help()
			os.Exit(1)
		}
		if err := RunCreate(co); err != nil {
			log.Fatal(err)
		}
		//log.Print("Cluster creation successful")
	},
}

func RunCreate(co *CreateOptions) error {
	cluster, err := parseClusterYaml(co.Cluster)
	if err != nil {
		return err
	}
	//log.Printf("Parsing done cluster: [%s]", cluster)

	machines, err := parseMachinesYaml(co.Machine)
	if err != nil {
		return err
	}
	//log.Printf("Parsing done [%s]", machines)

	d := deploy.NewDeployer()

	return d.CreateCluster(cluster, machines, co.EnableMachineController)
}
func init() {
	createCmd.Flags().StringVarP(&co.Cluster, "cluster", "c", "", "cluster yaml file")
	createCmd.Flags().StringVarP(&co.Machine, "machines", "m", "", "machine yaml file")
	createCmd.Flags().BoolVarP(&co.EnableMachineController, "enable-machine-controller", "e", false, "whether or not to start the machine controller")

	RootCmd.AddCommand(createCmd)
}
