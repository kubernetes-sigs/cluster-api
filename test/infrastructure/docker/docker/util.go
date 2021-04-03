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
const filterLabel = "label"
const filterName = "name"

type Filter map[string]map[string]string

// AddKeyValue adds a filter with a single name.
func (f Filter) AddKeyValue(key, value string) {
	if _, ok := f[key]; !ok {
		f[key] = map[string]string{}
	}

	f[key][value] = ""
}

// AddKeyNameValue adds a filter with a name=value.
func (f Filter) AddKeyNameValue(key, name, value string) {
	f.AddKeyValue(key, name)
	f[key][name] = value
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

// listContainers returns the list of docker containers matching filters.
func listContainers(filters Filter) ([]*types.Node, error) {
	n, err := List(filters)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list containers")
	}
	return n, nil
}

// getContainer returns the docker container matching filters.
func getContainer(filters Filter) (*types.Node, error) {
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
func List(filters Filter) ([]*types.Node, error) {
	res := []*types.Node{}
	visit := func(cluster string, node *types.Node) {
		res = append(res, node)
	}
	return res, list(visit, filters)
}

func list(visit func(string, *types.Node), filters Filter) error {
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

	for key, values := range filters {
		for subkey, subvalue := range values {
			if subvalue == "" {
				options.Filters.Add(key, subkey)
			} else {
				options.Filters.Add(key, fmt.Sprintf("%s=%s", subkey, subvalue))
			}
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
