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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type moveOptions struct {
	fromKubeconfig string
	namespace      string
	toKubeconfig   string
}

var mo = &moveOptions{}

var moveCmd = &cobra.Command{
	Use:   "move",
	Short: "Moves Cluster API objects (e.g. Cluster, Machines) from a management cluster to another management cluster",
	Long: LongDesc(`
		Moves Cluster API objects (e.g. Cluster, Machines) from a management cluster to another management cluster.

		The target cluster must have all the required provider components already installed.`),

	Example: Examples(`
		# Moves Cluster API objects from cluster to the target cluster.
		clusterctl move --to-kubeconfig=target-kubeconfig.yaml`),

	RunE: func(cmd *cobra.Command, args []string) error {
		if mo.toKubeconfig == "" {
			return errors.New("please specify a target cluster using the --to flag")
		}

		return runMove()
	},
}

func init() {
	moveCmd.Flags().StringVarP(&mo.fromKubeconfig, "kubeconfig", "", "", "Path to the kubeconfig file to use for accessing the originating management cluster. If empty, default rules for kubeconfig discovery will be used")
	moveCmd.Flags().StringVarP(&mo.toKubeconfig, "to-kubeconfig", "", "", "Path to the kubeconfig file to use for accessing the target management cluster")
	moveCmd.Flags().StringVarP(&mo.namespace, "namespace", "n", "", "The namespace where the objects describing the workload cluster exists. If not specified, the current namespace will be used")

	RootCmd.AddCommand(moveCmd)
}

func runMove() error {
	return nil
}
