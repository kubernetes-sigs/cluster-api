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
	"fmt"
	"strings"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
)

// RolloutOptions carries the base set of options supported by rollout command.
type RolloutOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// Resources for the rollout command
	Resources []string

	// Namespace where the resource(s) live. If unspecified, the namespace name will be inferred
	// from the current configuration.
	Namespace string
}

func (c *clusterctlClient) RolloutRestart(options RolloutOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}
	tuples, err := getResourceTuples(clusterClient, options)
	if err != nil {
		return err
	}
	for _, t := range tuples {
		if err := c.alphaClient.Rollout().ObjectRestarter(clusterClient.Proxy(), t, options.Namespace); err != nil {
			return err
		}
	}
	return nil
}

func (c *clusterctlClient) RolloutPause(options RolloutOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}
	tuples, err := getResourceTuples(clusterClient, options)
	if err != nil {
		return err
	}
	for _, t := range tuples {
		if err := c.alphaClient.Rollout().ObjectPauser(clusterClient.Proxy(), t, options.Namespace); err != nil {
			return err
		}
	}
	return nil
}

func (c *clusterctlClient) RolloutResume(options RolloutOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}
	tuples, err := getResourceTuples(clusterClient, options)
	if err != nil {
		return err
	}
	for _, t := range tuples {
		if err := c.alphaClient.Rollout().ObjectResumer(clusterClient.Proxy(), t, options.Namespace); err != nil {
			return err
		}
	}
	return nil
}

func getResourceTuples(clusterClient cluster.Client, options RolloutOptions) ([]util.ResourceTuple, error) {
	// If the option specifying the Namespace is empty, try to detect it.
	if options.Namespace == "" {
		currentNamespace, err := clusterClient.Proxy().CurrentNamespace()
		if err != nil {
			return []util.ResourceTuple{}, err
		}
		options.Namespace = currentNamespace
	}

	if len(options.Resources) == 0 {
		return []util.ResourceTuple{}, fmt.Errorf("required resource not specified")
	}
	normalized := normalizeResources(options.Resources)
	tuples, err := util.ResourceTypeAndNameArgs(normalized...)
	if err != nil {
		return []util.ResourceTuple{}, err
	}
	return tuples, nil
}

func normalizeResources(input []string) []string {
	normalized := make([]string, 0, len(input))
	for _, in := range input {
		normalized = append(normalized, strings.ToLower(in))
	}
	return normalized
}
