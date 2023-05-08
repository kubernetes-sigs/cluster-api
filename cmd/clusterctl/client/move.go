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
	"os"

	"github.com/pkg/errors"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
)

// MoveOptions carries the options supported by move.
type MoveOptions struct {
	// FromKubeconfig defines the kubeconfig to use for accessing the source management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	FromKubeconfig Kubeconfig

	// ToKubeconfig defines the kubeconfig to use for accessing the target management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	ToKubeconfig Kubeconfig

	// Namespace where the objects describing the workload cluster exists. If unspecified, the current
	// namespace will be used.
	Namespace string

	// ExperimentalResourceMutatorFn accepts any number of resource mutator functions that are applied on all resources being moved.
	// This is an experimental feature and is exposed only from the library and not (yet) through the CLI.
	ExperimentalResourceMutators []cluster.ResourceMutatorFunc

	// FromDirectory apply configuration from directory.
	FromDirectory string

	// ToDirectory save configuration to directory.
	ToDirectory string

	// DryRun means the move action is a dry run, no real action will be performed.
	DryRun bool
}

func (c *clusterctlClient) Move(options MoveOptions) error {
	// Both backup and restore makes no sense. It's a complete move.
	if options.FromDirectory != "" && options.ToDirectory != "" {
		return errors.Errorf("can't set both FromDirectory and ToDirectory")
	}

	if !options.DryRun &&
		options.FromDirectory == "" &&
		options.ToDirectory == "" &&
		options.ToKubeconfig == (Kubeconfig{}) {
		return errors.Errorf("at least one of FromDirectory, ToDirectory and ToKubeconfig must be set")
	}

	if options.ToDirectory != "" {
		return c.toDirectory(options)
	} else if options.FromDirectory != "" {
		return c.fromDirectory(options)
	} else {
		return c.move(options)
	}
}

func (c *clusterctlClient) move(options MoveOptions) error {
	// Get the client for interacting with the source management cluster.
	fromCluster, err := c.getClusterClient(options.FromKubeconfig)
	if err != nil {
		return err
	}

	// If the option specifying the Namespace is empty, try to detect it.
	if options.Namespace == "" {
		currentNamespace, err := fromCluster.Proxy().CurrentNamespace()
		if err != nil {
			return err
		}
		options.Namespace = currentNamespace
	}

	var toCluster cluster.Client
	if !options.DryRun {
		// Get the client for interacting with the target management cluster.
		if toCluster, err = c.getClusterClient(options.ToKubeconfig); err != nil {
			return err
		}
	}

	return fromCluster.ObjectMover().Move(options.Namespace, toCluster, options.DryRun, options.ExperimentalResourceMutators...)
}

func (c *clusterctlClient) fromDirectory(options MoveOptions) error {
	toCluster, err := c.getClusterClient(options.ToKubeconfig)
	if err != nil {
		return err
	}

	if _, err := os.Stat(options.FromDirectory); os.IsNotExist(err) {
		return err
	}

	return toCluster.ObjectMover().FromDirectory(toCluster, options.FromDirectory)
}

func (c *clusterctlClient) toDirectory(options MoveOptions) error {
	fromCluster, err := c.getClusterClient(options.FromKubeconfig)
	if err != nil {
		return err
	}

	// If the option specifying the Namespace is empty, try to detect it.
	if options.Namespace == "" {
		currentNamespace, err := fromCluster.Proxy().CurrentNamespace()
		if err != nil {
			return err
		}
		options.Namespace = currentNamespace
	}

	if _, err := os.Stat(options.ToDirectory); os.IsNotExist(err) {
		return err
	}

	return fromCluster.ObjectMover().ToDirectory(options.Namespace, options.ToDirectory)
}

func (c *clusterctlClient) getClusterClient(kubeconfig Kubeconfig) (cluster.Client, error) {
	cluster, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: kubeconfig})
	if err != nil {
		return nil, err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract.
	if err := cluster.ProviderInventory().CheckCAPIContract(); err != nil {
		return nil, err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := cluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return nil, err
	}
	return cluster, nil
}
