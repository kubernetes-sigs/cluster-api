/*
Copyright 2020 The Kubernetes Authors.

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

package client

import (
	"context"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
)

// DescribeClusterOptions carries the options supported by DescribeCluster.
type DescribeClusterOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// Namespace where the workload cluster is located. If unspecified, the current namespace will be used.
	Namespace string

	// ClusterName to be used for the workload cluster.
	ClusterName string

	// ShowOtherConditions is a list of comma separated kind or kind/name for which we should add the ShowObjectConditionsAnnotation
	// to signal to the presentation layer to show all the conditions for the objects.
	ShowOtherConditions string

	// ShowMachineSets instructs the discovery process to include machine sets in the ObjectTree.
	ShowMachineSets bool

	// Echo displays MachineInfrastructure or BootstrapConfig objects if the object's ready condition is true
	// or it has the same Status, Severity and Reason of the parent's object ready condition (it is an echo)
	Echo bool

	// Grouping groups machines objects in case the ready conditions
	// have the same Status, Severity and Reason.
	Grouping bool
}

// DescribeCluster returns the object tree representing the status of a Cluster API cluster.
func (c *clusterctlClient) DescribeCluster(options DescribeClusterOptions) (*tree.ObjectTree, error) {
	// gets access to the management cluster
	cluster, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return nil, err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract.
	if err := cluster.ProviderInventory().CheckCAPIContract(); err != nil {
		return nil, err
	}

	// If the option specifying the Namespace is empty, try to detect it.
	if options.Namespace == "" {
		currentNamespace, err := cluster.Proxy().CurrentNamespace()
		if err != nil {
			return nil, err
		}
		options.Namespace = currentNamespace
	}

	// Fetch the Cluster client.
	client, err := cluster.Proxy().NewClient()
	if err != nil {
		return nil, err
	}

	// Gets the object tree representing the status of a Cluster API cluster.
	return tree.Discovery(context.TODO(), client, options.Namespace, options.ClusterName, tree.DiscoverOptions{
		ShowOtherConditions: options.ShowOtherConditions,
		ShowMachineSets:     options.ShowMachineSets,
		Echo:                options.Echo,
		Grouping:            options.Grouping,
	})
}
