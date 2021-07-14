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

	// DryRun means the move action is a dry run, no real action will be performed
	DryRun bool
}

// BackupOptions holds options supported by backup.
type BackupOptions struct {
	// FromKubeconfig defines the kubeconfig to use for accessing the source management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	FromKubeconfig Kubeconfig

	// Namespace where the objects describing the workload cluster exists. If unspecified, the current
	// namespace will be used.
	Namespace string

	// Directory defines the local directory to store the cluster objects
	Directory string
}

// RestoreOptions holds options supported by restore.
type RestoreOptions struct {
	// FromKubeconfig defines the kubeconfig to use for accessing the target management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	ToKubeconfig Kubeconfig

	// Directory defines the local directory to restore cluster objects from
	Directory string
}

func (c *clusterctlClient) Move(options MoveOptions) error {
	// Get the client for interacting with the source management cluster.
	fromCluster, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.FromKubeconfig})
	if err != nil {
		return err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract.
	if err := fromCluster.ProviderInventory().CheckCAPIContract(); err != nil {
		return err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := fromCluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return err
	}

	var toCluster cluster.Client
	if !options.DryRun {
		// Get the client for interacting with the target management cluster.
		toCluster, err = c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.ToKubeconfig})
		if err != nil {
			return err
		}

		// Ensure this command only runs against management clusters with the current Cluster API contract.
		if err := toCluster.ProviderInventory().CheckCAPIContract(); err != nil {
			return err
		}

		// Ensures the custom resource definitions required by clusterctl are in place
		if err := toCluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
			return err
		}
	}

	// If the option specifying the Namespace is empty, try to detect it.
	if options.Namespace == "" {
		currentNamespace, err := fromCluster.Proxy().CurrentNamespace()
		if err != nil {
			return err
		}
		options.Namespace = currentNamespace
	}

	return fromCluster.ObjectMover().Move(options.Namespace, toCluster, options.DryRun)
}

func (c *clusterctlClient) Backup(options BackupOptions) error {
	// Get the client for interacting with the source management cluster.
	fromCluster, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.FromKubeconfig})
	if err != nil {
		return err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract.
	if err := fromCluster.ProviderInventory().CheckCAPIContract(); err != nil {
		return err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := fromCluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
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

	if _, err := os.Stat(options.Directory); os.IsNotExist(err) {
		return err
	}

	return fromCluster.ObjectMover().Backup(options.Namespace, options.Directory)
}

func (c *clusterctlClient) Restore(options RestoreOptions) error {
	// Get the client for interacting with the source management cluster.
	toCluster, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.ToKubeconfig})
	if err != nil {
		return err
	}

	// Ensure this command only runs against management clusters with the current Cluster API contract.
	if err := toCluster.ProviderInventory().CheckCAPIContract(); err != nil {
		return err
	}

	// Ensures the custom resource definitions required by clusterctl are in place.
	if err := toCluster.ProviderInventory().EnsureCustomResourceDefinitions(); err != nil {
		return err
	}

	if _, err := os.Stat(options.Directory); os.IsNotExist(err) {
		return err
	}

	return toCluster.ObjectMover().Restore(toCluster, options.Directory)
}
