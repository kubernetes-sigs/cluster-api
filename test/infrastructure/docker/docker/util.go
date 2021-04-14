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
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
	"sigs.k8s.io/kind/pkg/exec"
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
	args := []string{
		"ps",
		"-q",         // quiet output for parsing
		"-a",         // show stopped nodes
		"--no-trunc", // don't truncate
		// filter for nodes with the cluster label
		"--filter", "label=" + clusterLabelKey,
		// format to include friendly name and the cluster name
		"--format", fmt.Sprintf(`{{.Names}}\t{{.Label "%s"}}\t{{.Image}}\t{{.Status}}`, clusterLabelKey),
	}
	for _, filter := range filters {
		args = append(args, "--filter", filter)
	}
	cmd := exec.Command("docker", args...)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to list nodes. Output: %s", lines)
	}
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) != 4 {
			return errors.Errorf("invalid output when listing nodes: %s", line)
		}
		names := strings.Split(parts[0], ",")
		cluster := parts[1]
		image := parts[2]
		status := parts[3]
		visit(cluster, types.NewNode(names[0], image, "undetermined").WithStatus(status))
	}
	return nil
}
