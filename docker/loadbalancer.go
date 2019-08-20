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

package docker

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-docker/third_party/forked/loadbalancer"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

// LoadBalancer manages the load balancer for a specific docker cluster.
type LoadBalancer struct {
	logr.Logger
	Name string
}

// NewLoadBalancer returns a new LoadBalancer with a given name.
func NewLoadBalancer(name string, logger logr.Logger) (*LoadBalancer, error) {
	if name == "" {
		return nil, errors.New("name is required when creating a LoadBalancer")
	}
	if logger == nil {
		return nil, errors.New("logger is required when creating a LoadBalancer")
	}

	return &LoadBalancer{
		Name:   name,
		Logger: logger,
	}, nil
}

// Create creates a docker container hosting a load balancer for the cluster.
func (s *LoadBalancer) Create() (*nodes.Node, error) {
	// Describe and create if not exists.
	lb, err := s.container()
	if err != nil {
		return nil, err
	}

	if lb == nil {
		s.Info("Creating load balancer container")
		lb, err = nodes.CreateExternalLoadBalancerNode(
			s.containerName(),
			loadbalancer.Image,
			s.ClusterLabel(),
			"0.0.0.0",
			0,
		)
		if err != nil {
			return nil, err
		}
	}

	return lb, nil
}

// Delete the docker containers hosting a loadbalancer for the cluster.
func (s *LoadBalancer) Delete() error {
	// Describe and delete if exists.
	lb, err := s.container()
	if err != nil {
		return err
	}

	if lb != nil {
		s.Info("Deleting load balancer container")
		if err := nodes.Delete(*lb); err != nil {
			return err
		}
	}

	return nil
}

// containerName is the name of the docker container with the load balancer
func (s *LoadBalancer) containerName() string {
	return fmt.Sprintf("%s-lb", s.Name)
}

// ClusterLabel is the label of the docker containers for this cluster
func (s *LoadBalancer) ClusterLabel() string {
	return fmt.Sprintf("%s=%s", constants.ClusterLabelKey, s.Name)
}

// roleLabel is the label of the docker container with the load balancer
func (s *LoadBalancer) roleLabel() string {
	return fmt.Sprintf("%s=%s", constants.NodeRoleKey, constants.ExternalLoadBalancerNodeRoleValue)
}

// container returns the docker container with the load balancer
func (s *LoadBalancer) container() (*nodes.Node, error) {
	n, err := nodes.List(
		fmt.Sprintf("label=%s", s.ClusterLabel()),
		fmt.Sprintf("label=%s", s.roleLabel()),
		fmt.Sprintf("name=%s", s.containerName()),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list load balancer containers")
	}

	switch len(n) {
	case 0:
		return nil, nil
	case 1:
		return &n[0], nil
	default:
		return nil, errors.Errorf("expected 0 or 1 load balancer container, got %d", len(n))
	}
}
