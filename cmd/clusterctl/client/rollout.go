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

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/util"
)

// RolloutRestartOptions carries the options supported by RolloutRestart.
type RolloutRestartOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// Resources for the rollout command
	Resources []string

	// Namespace where the resource(s) live. If unspecified, the namespace name will be inferred
	// from the current configuration.
	Namespace string
}

// RolloutPauseOptions carries the options supported by RolloutPause.
type RolloutPauseOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// Resources for the rollout command
	Resources []string

	// Namespace where the resource(s) live. If unspecified, the namespace name will be inferred
	// from the current configuration.
	Namespace string
}

// RolloutResumeOptions carries the options supported by RolloutResume.
type RolloutResumeOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// Resources for the rollout command
	Resources []string

	// Namespace where the resource(s) live. If unspecified, the namespace name will be inferred
	// from the current configuration.
	Namespace string
}

// RolloutUndoOptions carries the options supported by RolloutUndo.
type RolloutUndoOptions struct {
	// Kubeconfig defines the kubeconfig to use for accessing the management cluster. If empty,
	// default rules for kubeconfig discovery will be used.
	Kubeconfig Kubeconfig

	// Resources for the rollout command
	Resources []string

	// Namespace where the resource(s) live. If unspecified, the namespace name will be inferred
	// from the current configuration.
	Namespace string

	// Revision number to rollback to when issuing the undo command.
	ToRevision int64
}

func (c *clusterctlClient) RolloutRestart(options RolloutRestartOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}
	objRefs, err := getObjectRefs(clusterClient, options.Namespace, options.Resources)
	if err != nil {
		return err
	}
	for _, ref := range objRefs {
		if err := c.alphaClient.Rollout().ObjectRestarter(clusterClient.Proxy(), ref); err != nil {
			return err
		}
	}
	return nil
}

func (c *clusterctlClient) RolloutPause(options RolloutPauseOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}
	objRefs, err := getObjectRefs(clusterClient, options.Namespace, options.Resources)
	if err != nil {
		return err
	}
	for _, ref := range objRefs {
		if err := c.alphaClient.Rollout().ObjectPauser(clusterClient.Proxy(), ref); err != nil {
			return err
		}
	}
	return nil
}

func (c *clusterctlClient) RolloutResume(options RolloutResumeOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}
	objRefs, err := getObjectRefs(clusterClient, options.Namespace, options.Resources)
	if err != nil {
		return err
	}
	for _, ref := range objRefs {
		if err := c.alphaClient.Rollout().ObjectResumer(clusterClient.Proxy(), ref); err != nil {
			return err
		}
	}
	return nil
}

func (c *clusterctlClient) RolloutUndo(options RolloutUndoOptions) error {
	clusterClient, err := c.clusterClientFactory(ClusterClientFactoryInput{Kubeconfig: options.Kubeconfig})
	if err != nil {
		return err
	}
	objRefs, err := getObjectRefs(clusterClient, options.Namespace, options.Resources)
	if err != nil {
		return err
	}
	for _, ref := range objRefs {
		if err := c.alphaClient.Rollout().ObjectRollbacker(clusterClient.Proxy(), ref, options.ToRevision); err != nil {
			return err
		}
	}
	return nil
}

func getObjectRefs(clusterClient cluster.Client, namespace string, resources []string) ([]corev1.ObjectReference, error) {
	// If the option specifying the Namespace is empty, try to detect it.
	if namespace == "" {
		currentNamespace, err := clusterClient.Proxy().CurrentNamespace()
		if err != nil {
			return []corev1.ObjectReference{}, err
		}
		namespace = currentNamespace
	}

	if len(resources) == 0 {
		return []corev1.ObjectReference{}, fmt.Errorf("required resource not specified")
	}
	normalized := normalizeResources(resources)
	objRefs, err := util.GetObjectReferences(namespace, normalized...)
	if err != nil {
		return []corev1.ObjectReference{}, err
	}
	return objRefs, nil
}

func normalizeResources(input []string) []string {
	normalized := make([]string, 0, len(input))
	for _, in := range input {
		normalized = append(normalized, strings.ToLower(in))
	}
	return normalized
}
