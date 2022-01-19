/*
Copyright 2022 The Kubernetes Authors.

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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// TopologyPlanOptions define options for TopologyPlan.
type TopologyPlanOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// Objs is the list of objects that are input to the topology plan (dry run) operation.
	// The objects can be among new/modified clusters, new/modifed ClusterClasses and new/modified templates.
	Objs []*unstructured.Unstructured

	// Cluster is the name of the cluster to dryrun reconcile if multiple clusters are affected by the input.
	Cluster string

	// Namespace is the target namespace for the operation.
	// This namespace is used as default for objects with missing namespaces.
	// If the namespace of any of the input objects conflicts with Namespace an error is returned.
	Namespace string
}

// TopologyPlanOutput defines the output of the topology plan operation.
type TopologyPlanOutput = cluster.TopologyPlanOutput

// TopologyPlan performs a dry run execution of the topology reconciler using the given inputs.
// It returns a summary of the changes observed during the execution.
func (c *clusterctlClient) TopologyPlan(options TopologyPlanOptions) (*TopologyPlanOutput, error) {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return nil, err
	}

	out, err := clusterClient.Topology().Plan(&cluster.TopologyPlanInput{
		Objs:              options.Objs,
		TargetClusterName: options.Cluster,
		TargetNamespace:   options.Namespace,
	})

	return out, err
}
