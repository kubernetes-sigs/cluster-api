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
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/cloudinit"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/cluster/nodes"
	"sigs.k8s.io/kind/pkg/exec"
)

const (
	defaultImageName = "kindest/node"
	defaultImageTag  = "v1.15.3"
)

// Machine implement a service for managing the docker containers hosting a kubernetes nodes.
type Machine struct {
	log       logr.Logger
	cluster   string
	machine   string
	image     string
	container *nodes.Node
}

// NewMachine returns a new Machine service for the given Cluster/DockerCluster pair.
func NewMachine(cluster, machine, image string, logger logr.Logger) (*Machine, error) {
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
		image:     image,
		container: container,
		log:       logger,
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

func machineContainerName(cluster, machine string) string {
	return fmt.Sprintf("%s-%s", cluster, machine)
}

// Create creates a docker container hosting a Kubernetes node.
func (m *Machine) Create(role string, version *string) error {
	// Create if not exists.
	if m.container == nil {
		var err error

		machineImage := m.machineImage(version)
		if m.image != "" {
			machineImage = m.image
		}

		switch role {
		case constants.ControlPlaneNodeRoleValue:
			m.log.Info("Creating control plane machine container")
			m.container, err = nodes.CreateControlPlaneNode(
				m.ContainerName(),
				machineImage,
				clusterLabel(m.cluster),
				"127.0.0.1",
				0,
				nil,
				nil,
			)
		case constants.WorkerNodeRoleValue:
			m.log.Info("Creating worker machine container")
			m.container, err = nodes.CreateWorkerNode(
				m.ContainerName(),
				machineImage,
				clusterLabel(m.cluster),
				nil,
				nil,
			)
		default:
			return errors.Errorf("unable to create machine for role %s", role)
		}
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// ExecBootstrap runs bootstrap on a node, this is generally `kubeadm <init|join>`
func (m *Machine) ExecBootstrap(data string) error {
	if m.container == nil {
		return errors.New("unable to set ExecBootstrap. the container hosting this machine does not exists")
	}

	cloudConfig, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errors.Wrap(err, "failed to decode machine's bootstrap data")
	}

	m.log.Info("Running machine bootstrap scripts")
	lines, err := cloudinit.Run(cloudConfig, m.container.Cmder())
	if err != nil {
		m.log.Error(err, strings.Join(lines, "\n"))
		return errors.Wrap(err, "failed to join a control plane node with kubeadm")
	}

	return nil
}

// SetNodeProviderID sets the docker provider ID for the kubernetes node
func (m *Machine) SetNodeProviderID() error {
	kubectlNode, err := m.getKubectlNode()
	if err != nil {
		return errors.Wrapf(err, "unable to set NodeProviderID. error getting a kubectl node")
	}
	if kubectlNode == nil {
		return errors.New("unable to set NodeProviderID. there are no kubectl node available")
	}

	m.log.Info("Setting Kubernetes node providerID")
	patch := fmt.Sprintf(`{"spec": {"providerID": "%s"}}`, m.ProviderID())
	cmd := kubectlNode.Command(
		"kubectl",
		"--kubeconfig", "/etc/kubernetes/admin.conf",
		"patch",
		"node", m.ContainerName(),
		"--patch", patch,
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		m.log.Error(err, strings.Join(lines, "\n"))
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
		m.log.Info("Running kubeadm reset on the machine")
		cmd := m.container.Command("kubeadm", "reset", "--force")
		lines, err := exec.CombinedOutputLines(cmd)
		if err != nil {
			m.log.Error(err, strings.Join(lines, "\n"))
			return errors.Wrap(err, "failed to reset node")
		}
	}

	return nil
}

// Delete deletes a docker container hosting a Kubernetes node.
func (m *Machine) Delete() error {
	// Delete if exists.
	if m.container != nil {
		m.log.Info("Deleting machine container")
		if err := nodes.Delete(*m.container); err != nil {
			return err
		}
	}
	return nil
}

// machineImage is the image of the container node with the machine
func (m *Machine) machineImage(version *string) string {
	if version == nil {
		defaultImage := fmt.Sprintf("%s:%s", defaultImageName, defaultImageTag)
		m.log.Info("Image for machine container not specified, using default comtainer image", defaultImage)
		return defaultImage
	}

	//TODO(fp) make this smarter
	// - allows usage of custom docker repository & image names
	// - add v only for semantic versions
	versionString := *version
	if !strings.HasPrefix(versionString, "v") {
		versionString = fmt.Sprintf("v%s", versionString)
	}

	return fmt.Sprintf("%s:%s", defaultImageName, versionString)
}
