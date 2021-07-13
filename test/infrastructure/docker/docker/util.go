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
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
)

const clusterLabelKey = "io.x-k8s.kind.cluster"
const nodeRoleLabelKey = "io.x-k8s.kind.role"
const filterLabel = "label"
const filterName = "name"

func machineContainerName(cluster, machine string) string {
	if strings.HasPrefix(machine, cluster) {
		return machine
	}
	return fmt.Sprintf("%s-%s", cluster, machine)
}

func machineFromContainerName(cluster, containerName string) string {
	machine := strings.TrimPrefix(containerName, cluster)
	return strings.TrimPrefix(machine, "-")
}

// listContainers returns the list of docker containers matching filters.
func listContainers(filters container.FilterBuilder) ([]*types.Node, error) {
	n, err := List(filters)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list containers")
	}
	return n, nil
}

// getContainer returns the docker container matching filters.
func getContainer(filters container.FilterBuilder) (*types.Node, error) {
	n, err := listContainers(filters)
	if err != nil {
		return nil, err
	}

	switch len(n) {
	case 0:
		return nil, nil
	case 1:
		return n[0], nil
	default:
		return nil, errors.Errorf("expected 0 or 1 container, got %d", len(n))
	}
}

// List returns the list of container IDs for the kind "nodes", optionally
// filtered by docker ps filters
// https://docs.docker.com/engine/reference/commandline/ps/#filtering
func List(filters container.FilterBuilder) ([]*types.Node, error) {
	res := []*types.Node{}
	visit := func(cluster string, node *types.Node) {
		res = append(res, node)
	}
	return res, list(visit, filters)
}

func list(visit func(string, *types.Node), filters container.FilterBuilder) error {
	ctx := context.TODO()
	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	// We also need our cluster label key to the list of filter
	filters.AddKeyValue("label", clusterLabelKey)

	containers, err := containerRuntime.ListContainers(ctx, filters)
	if err != nil {
		return errors.Wrap(err, "failed to list containers")
	}

	for _, cntr := range containers {
		name := cntr.Name
		cluster := clusterLabelKey
		image := cntr.Image
		status := cntr.Status
		visit(cluster, types.NewNode(name, image, "undetermined").WithStatus(status))
	}

	return nil
}
