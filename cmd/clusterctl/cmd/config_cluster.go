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
	"github.com/spf13/cobra"
)

type configClusterOptions struct {
	kubeconfig             string
	flavor                 string
	bootstrapProvider      string
	infrastructureProvider string

	targetNamespace   string
	kubernetesVersion string
	controlplaneCount int
	workerCount       int
}

var cc = &configClusterOptions{}

var configClusterClusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Generate templates for creating a Cluster API workload clusters",
	Long: LongDesc(`
		clusterctl ships with a list of well know providers; if necessary, edit
		the $HOME/cluster-api/.clusterctl.yaml file to add new provider configurations or to customize existing ones.
		
		Each provider configuration links to a repository, and clusterctl will fetch the template
		from creating a new cluster from there.`),

	Example: Examples(`
		# Generates a yaml file for creating a Cluster API workload cluster using
		# default infrastructure provider and default bootstrap provider installed in the cluster.
		clusterctl config cluster my-cluster
 
		# Generates a yaml file for creating a Cluster API workload cluster using
		# specified infrastructure and bootstrap provider
		clusterctl config cluster my-cluster --infrastructure=aws --bootstrap=kubeadm
 
		# Generates a yaml file for creating a Cluster API workload cluster using
		# specified version of the AWS infrastructure provider
		clusterctl config cluster my-cluster --infrastructure=aws:v0.4.1

		# Generates a yaml file for creating a Cluster API workload cluster in the "foo" namespace.
		clusterctl config cluster my-cluster --target-namespace=foo
		
		# Generates a yaml file for creating a Cluster API workload cluster with
		# nodes using Kubernetes v1.16.0
		clusterctl config cluster my-cluster --kubernetes-version=v1.16.0

		# Generates a yaml file for creating a Cluster API workload cluster with
		# custom number of nodes (if supported by provider's templates)
		clusterctl config cluster my-cluster --controlplane-machine-count=3 --worker-machine-count=10`),

	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGenerateCluster(args[0])
	},
}

func init() {
	configClusterClusterCmd.Flags().StringVarP(&cc.kubeconfig, "kubeconfig", "", "", "Path to the kubeconfig file to use for accessing the management cluster. If empty, default rules for kubeconfig discovery will be used")

	configClusterClusterCmd.Flags().StringVarP(&cc.infrastructureProvider, "infrastructure", "i", "", "The infrastructure provider that should be used for creating the workload cluster")
	configClusterClusterCmd.Flags().StringVarP(&cc.bootstrapProvider, "bootstrap", "b", "kubeadm", "The provider that should be used for bootstrapping Kubernetes nodes in the workload cluster")

	configClusterClusterCmd.Flags().StringVarP(&cc.flavor, "flavor", "f", "", "The template variant to be used for creating the workload cluster")
	configClusterClusterCmd.Flags().StringVarP(&cc.targetNamespace, "namespace", "n", "", "The namespace where the objects describing the workload cluster should be deployed. If not specified, the current namespace will be used")
	configClusterClusterCmd.Flags().StringVarP(&cc.kubernetesVersion, "kubernetes-version", "", "", "The Kubernetes version to use for the workload cluster")
	configClusterClusterCmd.Flags().IntVarP(&cc.controlplaneCount, "controlplane-machine-count", "", 1, "The number of control plane machines to be added to the workload cluster")
	configClusterClusterCmd.Flags().IntVarP(&cc.workerCount, "worker-machine-count", "", 1, "The number of worker machines to be added to the workload cluster")

	configCmd.AddCommand(configClusterClusterCmd)
}

func runGenerateCluster(name string) error {
	return nil
}
