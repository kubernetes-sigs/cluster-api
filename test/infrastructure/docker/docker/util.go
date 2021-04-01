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

	dockerTypes "github.com/docker/docker/api/types"
	dockerFilters "github.com/docker/docker/api/types/filters"
	dockerClient "github.com/docker/docker/client"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
)

const clusterLabelKey = "io.x-k8s.kind.cluster"
const nodeRoleLabelKey = "io.x-k8s.kind.role"

// clusterLabel returns the label applied to all the containers in a cluster.
func clusterLabel(name string) string {
	return toLabel(clusterLabelKey, name)
}

// roleLabel returns the label applied to all the containers with a specific role.
func roleLabel(role string) string {
	return toLabel(nodeRoleLabelKey, role)
}

func toLabel(key, val string) string {
	return fmt.Sprintf("%s=%s", key, val)
}

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

// withName returns a filter on name for listContainers & getContainer.
func withName(name string) string {
	return fmt.Sprintf("name=^%s$", name)
}

// withLabel returns a filter on labels for listContainers & getContainer.
func withLabel(label string) string {
	return fmt.Sprintf("label=%s", label)
}

// listContainers returns the list of docker containers matching filters.
func listContainers(filters ...string) ([]*types.Node, error) {
	n, err := List(filters...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list containers")
	}
	return n, nil
}

// getContainer returns the docker container matching filters.
func getContainer(filters ...string) (*types.Node, error) {
	n, err := listContainers(filters...)
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
func List(filters ...string) ([]*types.Node, error) {
	res := []*types.Node{}
	visit := func(cluster string, node *types.Node) {
		res = append(res, node)
	}
	return res, list(visit, filters...)
}

func list(visit func(string, *types.Node), filters ...string) error {
	ctx := context.Background()
	cli, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		return errors.Wrap(err, "failed to connect to docker daemon")
	}

	options := dockerTypes.ContainerListOptions{
		All:     true,
		Limit:   -1,
		Filters: dockerFilters.NewArgs(dockerFilters.Arg("label", clusterLabelKey)),
	}

	for _, f := range filters {
		label := strings.Split(f, "=")
		if len(label) == 2 {
			options.Filters.Add(label[0], label[1])
		} else if len(label) == 3 {
			options.Filters.Add(label[0], fmt.Sprintf("%s=%s", label[1], label[2]))
		}
	}

	containers, err := cli.ContainerList(ctx, options)
	if err != nil {
		return errors.Wrap(err, "failed to list nodes")
	}

	for _, container := range containers {
		names := container.Names
		cluster := clusterLabelKey
		image := container.Image
		status := container.Status
		visit(cluster, types.NewNode(strings.Trim(names[0], "/"), image, "undetermined").WithStatus(status))
	}
	return nil
}
