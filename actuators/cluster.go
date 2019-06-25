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

package actuators

import (
	"fmt"

	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// Cluster defines a cluster actuator object
type Cluster struct {
}

// NewClusterActuator returns a new cluster actuator object
func NewClusterActuator() *Cluster {
	return &Cluster{}
}

// Reconcile setups an external load balancer for the cluster if needed
func (c *Cluster) Reconcile(cluster *clusterv1.Cluster) error {
	elb, err := getExternalLoadBalancerNode(cluster.Name)
	if err != nil {
		fmt.Printf("%+v\n", err)
		return err
	}
	if elb != nil {
		fmt.Println("External Load Balancer already exists. Nothing to do for this cluster.")
		return nil
	}
	fmt.Printf("The cluster named %q has been created! Setting up some infrastructure.\n", cluster.Name)
	_, err = actions.SetUpLoadBalancer(cluster.Name)
	return err
}

// Delete can be used to delete a cluster
func (c *Cluster) Delete(cluster *clusterv1.Cluster) error {
	fmt.Println("Cluster delete is not implemented.")
	return nil
}
