/*
Copyright 2019 The Kubernetes Authors.

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
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-docker/cloudinit"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/exec"
)

const (
	defaultImage = "kindest/node:v1.15.0"
)

// Machine implement a service for managing the docker containers hosting a kubernetes nodes.
type Machine struct {
	logr.Logger
	cluster   string
	machine   string
	container *nodes.Node
}

// NewMachine returns a new Machine service for the given Cluster/DockerCluster pair.
func NewMachine(cluster, machine string, logger logr.Logger) (*Machine, error) {
	if cluster == "" {
		return nil, errors.New("cluster is required when creating a docker.Machine")
	}
	if machine == "" {
		return nil, errors.New("machine is required when creating a docker.Machine")
	}
	if logger == nil {
		return nil, errors.New("logger is required when creating a docker.Machine")
	}

	container, err := getContainer(
		withLabel(clusterLabel(cluster)),
		withName(machineContainerName(cluster, machine)),
	)
	if err != nil {
		return nil, err
	}

	return &Machine{
		cluster:   cluster,
		machine:   machine,
		Logger:    logger,
		container: container,
	}, nil
}

// ContainerName return the name of the container for this machine
func (m *Machine) ContainerName() string {
	return machineContainerName(m.cluster, m.machine)
}

// ProviderID return the provider identifier for this machine
func (m *Machine) ProviderID() string {
	return fmt.Sprintf("docker:////%s", m.ContainerName())
}

// NodeName return the name of the Kubernetes node corresponding to this machine
func (m *Machine) NodeName() string {
	return m.ContainerName()
}

func machineContainerName(cluster, machine string) string {
	return fmt.Sprintf("%s-%s", cluster, machine)
}

// CreateControlPlane creates a docker container hosting a Kubernetes node.
func (m *Machine) CreateControlPlane(version *string) error {
	// Create if not exists.
	if m.container == nil {
		var err error
		m.Info("Creating controlplane machine container")
		m.container, err = nodes.CreateControlPlaneNode(
			m.ContainerName(),
			machineImage(version),
			clusterLabel(m.cluster),
			"127.0.0.1",
			0,
			nil,
			nil,
		)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// CreateWorker creates a docker container hosting a Kubernetes node.
func (m *Machine) CreateWorker(version *string) error {
	// Create if not exists.
	if m.container == nil {
		var err error
		m.Info("Creating worker machine container")
		m.container, err = nodes.CreateWorkerNode(
			m.ContainerName(),
			machineImage(version),
			clusterLabel(m.cluster),
			nil,
			nil,
		)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// ExecBootstrap runs bootstrap on a node, this is generally `kubeadm <init|join>`
func (m *Machine) ExecBootstrap(data string) error {
	if m.container == nil {

	}

	cloudConfig, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errors.Wrap(err, "failed to decode machine's bootstrap data")
	}

	m.Info("Running machine bootstrap scripts")
	lines, err := cloudinit.Run(cloudConfig, m.container.Cmder())
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed to join a control plane node with kubeadm")
	}

	return nil
}

// SetNodeProviderID patches a node with docker://node-name as the providerID
func (m *Machine) SetNodeProviderID() error {
	kubectlNode, err := m.getKubectlNode()
	if err != nil {
		return errors.Wrapf(err, "unable to set NodeProviderID. error getting a kubectl node")
	}
	if kubectlNode == nil {
		return errors.New("unable to set NodeProviderID. there are no kubectl node available")
	}

	m.Info("Setting Kubernetes node providerID")
	patch := fmt.Sprintf(`{"spec": {"providerID": "%s"}}`, m.ProviderID())
	cmd := kubectlNode.Command(
		"kubectl",
		"--kubeconfig", "/etc/kubernetes/admin.conf",
		"patch",
		"node", m.NodeName(),
		"--patch", patch,
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		for _, line := range lines {
			fmt.Println(line)
		}
		return errors.Wrap(err, "failed update providerID")
	}

	return nil
}

func (m *Machine) getKubectlNode() (*nodes.Node, error) {
	// collect info about the existing controlplane nodes
	kubectlNodes, err := listContainers(
		withLabel(clusterLabel(m.cluster)),
		withLabel(roleLabel(constants.ControlPlaneNodeRoleValue)),
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(kubectlNodes) == 0 {
		return nil, nil
	}

	return &kubectlNodes[0], nil
}

// KubeadmReset will run `kubeadm reset` on the machine.
func (m *Machine) KubeadmReset() error {
	if m.container != nil {
		m.Info("Running kubeadm reset on the machine")
		cmd := m.container.Command("kubeadm", "reset", "--force")
		lines, err := exec.CombinedOutputLines(cmd)
		if err != nil {
			for _, line := range lines {
				fmt.Println(line)
			}
			return errors.Wrap(err, "failed to reset node")
		}
	}

	return nil
}

// Delete deletes a docker container hosting a Kubernetes node.
func (m *Machine) Delete() error {
	// Delete if exists.
	if m.container != nil {
		m.Info("Deleting machine container")
		if err := nodes.Delete(*m.container); err != nil {
			return err
		}
	}
	return nil
}

// machineImage is the image of the container node with the machine
func machineImage(version *string) string {
	if version == nil {
		return defaultImage
	}

	//TODO(fp) make this smarter
	// - allows usage of custom docker repository & image names
	// - add v only for semantic versions
	versionString := *version
	if !strings.HasPrefix(versionString, "v") {
		versionString = fmt.Sprintf("v%s", versionString)
	}

	return fmt.Sprintf("kindest/node:%s", versionString)
}
