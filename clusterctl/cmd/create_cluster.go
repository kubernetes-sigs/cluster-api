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
	"github.com/ghodss/yaml"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"io/ioutil"
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer"
	"sigs.k8s.io/cluster-api/clusterctl/clusterdeployer/minikube"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
)

type CreateOptions struct {
	Cluster string
	Machine string
	CleanupExternalCluster bool
	VmDriver string
}

var co = &CreateOptions{}

var createClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Create kubernetes cluster",
	Long:  `Create a kubernetes cluster with one command`,
	Run: func(cmd *cobra.Command, args []string) {
		if co.Cluster == "" {
			exitWithHelp(cmd, "Please provide yaml file for cluster definition.")
		}
		if co.Machine == "" {
			exitWithHelp(cmd, "Please provide yaml file for machine definition.")
		}
		if err := RunCreate(co); err != nil {
			glog.Exit(err)
		}
	},
}

func RunCreate(co *CreateOptions) error {
	c, err := parseClusterYaml(co.Cluster)
	if err != nil {
		return err
	}
	m, err := parseMachinesYaml(co.Machine)
	if err != nil {
		return err
	}

	mini := minikube.New(co.VmDriver)
	d := clusterdeployer.New(mini, co.CleanupExternalCluster)
	err = d.Create(c, m)
	return err
}

func init() {
	// Required flags
	createClusterCmd.Flags().StringVarP(&co.Cluster, "cluster", "c", "", "A yaml file containing cluster object definition")
	createClusterCmd.Flags().StringVarP(&co.Machine, "machines", "m", "", "A yaml file containing machine object definition(s)")

	// Optional flags
	createClusterCmd.Flags().BoolVarP(&co.CleanupExternalCluster, "cleanup-external-cluster", "", true, "Whether to cleanup the external cluster after bootstrap")
	createClusterCmd.Flags().StringVarP(&co.VmDriver, "vm-driver", "", "", "Which vm driver to use for minikube")
	createCmd.AddCommand(createClusterCmd)
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

func parseMachinesYaml(file string) ([]*clusterv1.Machine, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	list := &clusterv1.MachineList{}
	err = yaml.Unmarshal(bytes, &list)
	if err != nil {
		return nil, err
	}

	if list == nil {
		return []*clusterv1.Machine{}, nil
	}

	return util.MachineP(list.Items), nil
}
