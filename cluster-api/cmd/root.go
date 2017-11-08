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
	"log"
	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
	"io/ioutil"
	machinev1 "k8s.io/kube-deploy/cluster-api/api/machines/v1alpha1"
	clusterv1 "k8s.io/kube-deploy/cluster-api/api/cluster/v1alpha1"
)

var RootCmd = &cobra.Command{
	Use:   "cluster-api",
	Short: "cluster management",
	Long:  `Simple kubernetes cluster management`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
		cmd.Help()
	},
}

// Kubernetes cluster config file.
var KubeConfig string;

func init() {
	RootCmd.Flags().StringVarP(&KubeConfig, "kubecofig", "k", "", "location for the kubernetes config file")
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func parseClusterYaml(file string) (*clusterv1.Cluster, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	cluster := &clusterv1.Cluster{}
	err = yaml.Unmarshal(bytes, cluster)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}


func parseMachinesYaml(file string) ([]machinev1.Machine, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	machines := &machinev1.MachineList{}
	err = yaml.Unmarshal(bytes, &machines)
	if err != nil {
		return nil, err
	}
	return machines.Items, nil
}