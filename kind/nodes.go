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

package kind

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-docker/kind/actions"
	"sigs.k8s.io/cluster-api-provider-docker/third_party/forked/loadbalancer"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

// LoadBalancer can do things with load balancers
type LoadBalancer struct {
	Cluster string
	Node    *nodes.Node
	Logger  logr.Logger
}

// NewLoadBalancer is a node balancer initializer
func NewLoadBalancer(cluster string, log logr.Logger) (*LoadBalancer, error) {
	allNodes, err := nodes.List()
	if err != nil {
		log.Error(err, "Failed to list all nodes in cluster")
		return nil, err
	}
	node, err := nodes.ExternalLoadBalancerNode(allNodes)
	if err != nil {
		log.Error(err, "Failed to get external load balancer node")
		return nil, err
	}
	return &LoadBalancer{
		Cluster: cluster,
		Node:    node,
		Logger:  log,
	}, nil
}

// Create creates a load balancer but does not configure it.
func (l *LoadBalancer) Create() error {
	name := l.Name()
	log := l.Logger.WithName("load-balancer-create").WithValues("cluster", l.Cluster, "name", name)
	clusterLabel := fmt.Sprintf("%s=%s", constants.ClusterLabelKey, l.Cluster)

	if err := l.Cleanup(); err != nil {
		log.Error(err, "Failed to cleanup environment")
		return err
	}
	// Now that the environment is clean, create
	log.Info("Creating load balancer")
	lb, err := nodes.CreateExternalLoadBalancerNode(
		name,
		// TODO: make this configurable
		loadbalancer.Image,
		clusterLabel,
		"0.0.0.0",
		0,
	)
	if err != nil {
		return err
	}
	l.Node = lb
	return nil
}

// Cleanup cleans up exited containers
func (l *LoadBalancer) Cleanup() error {
	name := l.Name()
	log := l.Logger.WithName("cleanup").WithValues("cluster", l.Cluster, "name", name)
	// if there is already a container with our name but it's exited then remove it
	exitedLB, err := nodes.List(
		fmt.Sprintf("label=%s=%s", constants.NodeRoleKey, constants.ExternalLoadBalancerNodeRoleValue),
		fmt.Sprintf("name=%s", name),
		fmt.Sprintf("status=exited"),
	)
	if err != nil {
		log.Error(err, "Failed to list exited external load balancer nodes")
		return err
	}

	if len(exitedLB) > 0 {
		// only one container with given name should exist, if any.
		log.Info("Cleaning up exited load balancer")
		return nodes.Delete(exitedLB[0])
	}
	return nil
}

// Name is the name of the load balancer
func (l *LoadBalancer) Name() string {
	return fmt.Sprintf("%s-%s", l.Cluster, constants.ExternalLoadBalancerNodeRoleValue)
}

// TODO: Delete load balancer?

// Node is a node initializer. It knows how to build every kind of node
type Node struct {
	Cluster, Machine, Role, Version string
	Nodes                           []nodes.Node
	MachineActions                  *actions.MachineActions
	logr.Logger
}

// NewNode creates a node initializer
func NewNode(cluster, machine, role, version string, log logr.Logger) (*Node, error) {
	clusterNodes, err := nodes.List()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list nodes")
	}
	return &Node{
		Logger:         log.WithName("new-node").WithValues("cluster", cluster, "machine", machine, "role", role, "version", version),
		Cluster:        cluster,
		Machine:        machine,
		Role:           role,
		Version:        version,
		Nodes:          clusterNodes,
		MachineActions: &actions.MachineActions{Logger: log.WithName("machine-actions")},
	}, nil
}

// Create figures out what kind of node to make and does the right thing
func (n *Node) Create() (*nodes.Node, error) {
	log := n.Logger.WithName("node-create")
	switch n.Role {
	case "control-plane":
		// Node length includes ELB which must already exist
		if len(n.Nodes) == 1 {
			log.Info("Creating the first control plane node")
			node, err := nodes.ExternalLoadBalancerNode(n.Nodes)
			if err != nil {
				log.Error(err, "Failed to get external load balancer node")
				return nil, err
			}
			ipv4, _, err := node.IP()
			if err != nil {
				log.Error(err, "Failed to get node's IP", "node-name", node.Name())
				return nil, err
			}
			return n.MachineActions.CreateControlPlane(n.Cluster, n.Machine, ipv4, n.Version, nil)
		}
		log.Info("Adding an additional control plane node")
		return n.MachineActions.AddControlPlane(n.Cluster, n.Machine, n.Version)
	case "worker":
		log.Info("Adding a worker")
		return n.MachineActions.AddWorker(n.Cluster, n.Machine, n.Version)
	default:
		log.Info("Unknown role", "role", n.Role)
		return nil, errors.Errorf("Unknown role: %q", n.Role)
	}
}

// Delete removes the underlying infrastructure for a given Node.
func (n *Node) Delete() error {
	log := n.Logger.WithName("node-delete")
	switch n.Role {
	case "control-plane":
		return n.MachineActions.DeleteControlPlane(n.Cluster, n.Machine)
	case "worker":
		return n.MachineActions.DeleteWorker(n.Cluster, n.Machine)
	default:
		log.Info("Unknown role", "role", n.Role)
		return errors.Errorf("Unknown role: %q", n.Role)
	}
}
