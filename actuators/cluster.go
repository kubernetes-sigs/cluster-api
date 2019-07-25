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
	"github.com/go-logr/logr"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	capiv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
)

// Cluster defines a cluster actuator object
type Cluster struct {
	Log logr.Logger
}

// Reconcile setups an external load balancer for the cluster if needed
func (c *Cluster) Reconcile(cluster *capiv1alpha2.Cluster) error {
	c.Log.Info("Reconciling cluster", "cluster-namespace", cluster.Namespace, "cluster-name", cluster.Name)
	elb, err := getExternalLoadBalancerNode(cluster.Name, c.Log)
	if err != nil {
		c.Log.Error(err, "Error getting external load balancer node")
		return err
	}
	if elb != nil {
		c.Log.Info("External Load Balancer already exists. Nothing to do for this cluster.")
		return nil
	}
	c.Log.Info("Cluster has been created! Setting up some infrastructure", "cluster-name", cluster.Name)
	_, err = actions.SetUpLoadBalancer(cluster.Name)
	return err
}

// Delete can be used to delete a cluster
func (c *Cluster) Delete(cluster *capiv1alpha2.Cluster) error {
	c.Log.Info("Cluster delete is not implemented.")
	return nil
}
