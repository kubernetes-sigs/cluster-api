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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/cloudinit"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

const (
	defaultImageName = "kindest/node"
	defaultImageTag  = "v1.16.3"
)

type nodeCreator interface {
	CreateControlPlaneNode(name, image, clusterLabel, listenAddress string, port int32, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping) (node *types.Node, err error)
	CreateWorkerNode(name, image, clusterLabel string, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping) (node *types.Node, err error)
}

// Machine implement a service for managing the docker containers hosting a kubernetes nodes.
type Machine struct {
	log       logr.Logger
	cluster   string
	machine   string
	image     string
	container *types.Node

	nodeCreator nodeCreator
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
		cluster:     cluster,
		machine:     machine,
		image:       image,
		container:   container,
		log:         logger,
		nodeCreator: &Manager{},
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

// Create creates a docker container hosting a Kubernetes node.
func (m *Machine) Create(ctx context.Context, role string, version *string) error {
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
			m.container, err = m.nodeCreator.CreateControlPlaneNode(
				m.ContainerName(),
				machineImage,
				clusterLabel(m.cluster),
				"127.0.0.1",
				0,
				nil,
				nil,
			)
			if err != nil {
				return errors.WithStack(err)
			}
		case constants.WorkerNodeRoleValue:
			m.log.Info("Creating worker machine container")
			m.container, err = m.nodeCreator.CreateWorkerNode(
				m.ContainerName(),
				machineImage,
				clusterLabel(m.cluster),
				nil,
				nil,
			)
			if err != nil {
				return errors.WithStack(err)
			}
		default:
			return errors.Errorf("unable to create machine for role %s", role)
		}
		// After creating a node we need to wait a small amount of time until crictl does not return an error.
		// This fixes an issue where we try to kubeadm init too quickly after creating the container.
		err = wait.PollImmediate(500*time.Millisecond, 2*time.Second, func() (bool, error) {
			ps := m.container.Commander.Command("crictl", "ps")
			return ps.Run(ctx) == nil, nil
		})
		if err != nil {
			return errors.WithStack(err)
		}
		return nil
	}
	return nil
}

// ExecBootstrap runs bootstrap on a node, this is generally `kubeadm <init|join>`
func (m *Machine) ExecBootstrap(ctx context.Context, data string) error {
	if m.container == nil {
		return errors.New("unable to set ExecBootstrap. the container hosting this machine does not exists")
	}

	cloudConfig, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errors.Wrap(err, "failed to decode machine's bootstrap data")
	}

	commands, err := cloudinit.Commands(cloudConfig)
	if err != nil {
		m.log.Info("cloud config failed to parse", "cloud-config", string(cloudConfig))
		return errors.Wrap(err, "failed to join a control plane node with kubeadm")
	}

	m.log.Info("Running machine bootstrap scripts")
	var out bytes.Buffer
	for _, command := range commands {
		cmd := m.container.Commander.Command(command.Cmd, command.Args...)
		cmd.SetStderr(&out)
		if command.Stdin != "" {
			cmd.SetStdin(strings.NewReader(command.Stdin))
		}
		err := cmd.Run(ctx)
		if err != nil {
			m.log.Info("Failed running command", "command", command, "stderr", out.String())
			return errors.Wrap(err, "failed to run cloud conifg")
		}
	}

	return nil
}

// SetNodeProviderID sets the docker provider ID for the kubernetes node
func (m *Machine) SetNodeProviderID(ctx context.Context) error {
	kubectlNode, err := m.getKubectlNode()
	if err != nil {
		return errors.Wrapf(err, "unable to set NodeProviderID. error getting a kubectl node")
	}
	if kubectlNode == nil {
		return errors.New("unable to set NodeProviderID. there are no kubectl node available")
	}

	m.log.Info("Setting Kubernetes node providerID")
	patch := fmt.Sprintf(`{"spec": {"providerID": "%s"}}`, m.ProviderID())
	cmd := kubectlNode.Commander.Command(
		"kubectl",
		"--kubeconfig", "/etc/kubernetes/admin.conf",
		"patch",
		"node", m.ContainerName(),
		"--patch", patch,
	)
	lines, err := cmd.RunLoggingOutputOnFail(ctx)
	if err != nil {
		for _, line := range lines {
			m.log.Info(line)
		}
		return errors.Wrap(err, "failed update providerID")
	}

	return nil
}

func (m *Machine) getKubectlNode() (*types.Node, error) {
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
	// Return the first node that is not the current machine.
	// The assumption being made is that the existing control planes will already be ready.
	// This is true when we are using kubeadm control plane.
	for _, node := range kubectlNodes {
		if node.Name != m.container.Name {
			return node, nil
		}
	}
	// This will happen when the current machine is the only machine.
	return kubectlNodes[0], nil
}

// Delete deletes a docker container hosting a Kubernetes node.
func (m *Machine) Delete(ctx context.Context) error {
	// Delete if exists.
	if m.container != nil {
		m.log.Info("Deleting machine container")
		if err := m.container.Delete(ctx); err != nil {
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
