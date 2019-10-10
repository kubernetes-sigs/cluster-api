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
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/third_party/forked/loadbalancer"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/container/docker"
)

// LoadBalancer manages the load balancer for a specific docker cluster.
type LoadBalancer struct {
	log       logr.Logger
	name      string
	container *nodes.Node
}

// NewLoadBalancer returns a new helper for managing a docker loadbalancer with a given name.
func NewLoadBalancer(name string, logger logr.Logger) (*LoadBalancer, error) {
	if name == "" {
		return nil, errors.New("name is required when creating a docker.LoadBalancer")
	}
	if logger == nil {
		return nil, errors.New("logger is required when creating a docker.LoadBalancer")
	}

	container, err := getContainer(
		withLabel(clusterLabel(name)),
		withLabel(roleLabel(constants.ExternalLoadBalancerNodeRoleValue)),
	)
	if err != nil {
		return nil, err
	}

	return &LoadBalancer{
		name:      name,
		container: container,
		log:       logger,
	}, nil
}

// ContainerName is the name of the docker container with the load balancer
func (s *LoadBalancer) containerName() string {
	return fmt.Sprintf("%s-lb", s.name)
}

// Create creates a docker container hosting a load balancer for the cluster.
func (s *LoadBalancer) Create() error {
	// Create if not exists.
	if s.container == nil {
		var err error
		s.log.Info("Creating load balancer container")
		s.container, err = nodes.CreateExternalLoadBalancerNode(
			s.containerName(),
			loadbalancer.Image,
			clusterLabel(s.name),
			"0.0.0.0",
			0,
		)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// UpdateConfiguration updates the external load balancer configuration with new control plane nodes.
func (s *LoadBalancer) UpdateConfiguration() error {
	if s.container == nil {
		return errors.New("unable to configure load balancer: load balancer container does not exists")
	}

	// collect info about the existing controlplane nodes
	controlPlaneNodes, err := listContainers(
		withLabel(clusterLabel(s.name)),
		withLabel(roleLabel(constants.ControlPlaneNodeRoleValue)),
	)
	if err != nil {
		return errors.WithStack(err)
	}

	var backendServers = map[string]string{}
	for _, n := range controlPlaneNodes {
		controlPlaneIPv4, _, err := n.IP()
		if err != nil {
			return errors.Wrapf(err, "failed to get IP for container %s", n.Name())
		}
		backendServers[n.Name()] = fmt.Sprintf("%s:%d", controlPlaneIPv4, 6443)
	}

	// create loadbalancer config data
	loadbalancerConfig, err := loadbalancer.Config(&loadbalancer.ConfigData{
		ControlPlanePort: loadbalancer.ControlPlanePort,
		BackendServers:   backendServers,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	s.log.Info("Updating load balancer configuration")
	if err := s.container.WriteFile(loadbalancer.ConfigPath, loadbalancerConfig); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(docker.Kill("SIGHUP", s.containerName()))
}

// IP returns the load balancer IP address
func (s *LoadBalancer) IP() (string, error) {
	lbip4, _, err := s.container.IP()
	if err != nil {
		return "", errors.WithStack(err)
	}
	return lbip4, nil
}

// Delete the docker containers hosting a loadbalancer for the cluster.
func (s *LoadBalancer) Delete() error {
	// Delete if exists.
	if s.container != nil {
		s.log.Info("Deleting load balancer container")
		if err := nodes.Delete(*s.container); err != nil {
			return err
		}
	}

	return nil
}
