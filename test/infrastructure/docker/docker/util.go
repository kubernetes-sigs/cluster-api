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

	"github.com/pkg/errors"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
)

// clusterLabel returns the label applied to all the containers in a cluster
func clusterLabel(name string) string {
	return fmt.Sprintf("%s=%s", constants.ClusterLabelKey, name)
}

// roleLabel returns the label applied to all the containers with a specific role
func roleLabel(role string) string {
	return fmt.Sprintf("%s=%s", constants.NodeRoleKey, role)
}

// withName returns a filter on name for listContainers & getContainer
func withName(name string) string {
	return fmt.Sprintf("name=^%s$", name)
}

// withLabel returns a filter on labels for listContainers & getContainer
func withLabel(label string) string {
	return fmt.Sprintf("label=%s", label)
}

// listContainers returns the list of docker containers matching filters
func listContainers(filters ...string) ([]nodes.Node, error) {
	n, err := nodes.List(filters...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list containers")
	}
	return n, nil
}

// getContainer returns the docker container matching filters
func getContainer(filters ...string) (*nodes.Node, error) {
	n, err := listContainers(filters...)
	if err != nil {
		return nil, err
	}

	switch len(n) {
	case 0:
		return nil, nil
	case 1:
		return &n[0], nil
	default:
		return nil, errors.Errorf("expected 0 or 1 container, got %d", len(n))
	}
}
