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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/cloudinit"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
	clusterapicontainer "sigs.k8s.io/cluster-api/util/container"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

const (
	defaultImageName = "kindest/node"
	defaultImageTag  = "v1.22.0"
)

type nodeCreator interface {
	CreateControlPlaneNode(ctx context.Context, name, image, clusterName, listenAddress string, port int32, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string, ipFamily clusterv1.ClusterIPFamily) (node *types.Node, err error)
	CreateWorkerNode(ctx context.Context, name, image, clusterName string, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string, ipFamily clusterv1.ClusterIPFamily) (node *types.Node, err error)
}

// Machine implement a service for managing the docker containers hosting a kubernetes nodes.
type Machine struct {
	cluster   string
	machine   string
	image     string
	ipFamily  clusterv1.ClusterIPFamily
	labels    map[string]string
	container *types.Node

	nodeCreator nodeCreator
}

// NewMachine returns a new Machine service for the given Cluster/DockerCluster pair.
func NewMachine(cluster *clusterv1.Cluster, machine, image string, labels map[string]string) (*Machine, error) {
	if cluster == nil {
		return nil, errors.New("cluster is required when creating a docker.Machine")
	}
	if cluster.Name == "" {
		return nil, errors.New("cluster name is required when creating a docker.Machine")
	}
	if machine == "" {
		return nil, errors.New("machine is required when creating a docker.Machine")
	}

	filters := container.FilterBuilder{}
	filters.AddKeyNameValue(filterLabel, clusterLabelKey, cluster.Name)
	filters.AddKeyValue(filterName, fmt.Sprintf("^%s$", machineContainerName(cluster.Name, machine)))
	for key, val := range labels {
		filters.AddKeyNameValue(filterLabel, key, val)
	}

	newContainer, err := getContainer(filters)
	if err != nil {
		return nil, err
	}

	ipFamily, err := cluster.GetIPFamily()
	if err != nil {
		return nil, fmt.Errorf("create docker machine: %s", err)
	}

	return &Machine{
		cluster:     cluster.Name,
		machine:     machine,
		image:       image,
		ipFamily:    ipFamily,
		container:   newContainer,
		labels:      labels,
		nodeCreator: &Manager{},
	}, nil
}

func ListMachinesByCluster(cluster *clusterv1.Cluster, labels map[string]string) ([]*Machine, error) {
	if cluster == nil {
		return nil, errors.New("cluster is required when listing machines in the cluster")
	}
	if cluster.Name == "" {
		return nil, errors.New("cluster name is required when listing machines in the cluster")
	}

	filters := container.FilterBuilder{}
	filters.AddKeyNameValue(filterLabel, clusterLabelKey, cluster.Name)
	for key, val := range labels {
		filters.AddKeyNameValue(filterLabel, key, val)
	}

	containers, err := listContainers(filters)
	if err != nil {
		return nil, err
	}

	ipFamily, err := cluster.GetIPFamily()
	if err != nil {
		return nil, fmt.Errorf("list docker machines by cluster: %s", err)
	}

	machines := make([]*Machine, len(containers))
	for i, containerNode := range containers {
		machines[i] = &Machine{
			cluster:     cluster.Name,
			machine:     machineFromContainerName(cluster.Name, containerNode.Name),
			image:       containerNode.Image,
			ipFamily:    ipFamily,
			labels:      labels,
			container:   containerNode,
			nodeCreator: &Manager{},
		}
	}

	return machines, nil
}

// IsControlPlane returns true if the container for this machine is a control plane node.
func (m *Machine) IsControlPlane() bool {
	if !m.Exists() {
		return false
	}
	return m.container.ClusterRole == constants.ControlPlaneNodeRoleValue
}

// ImageVersion returns the version of the image used or nil if not specified
// NOTE: Image version might be different from the Kubernetes version, because some characters
// allowed by semver (e.g. +) can't be used for image tags, so they are replaced with "_".
func (m *Machine) ImageVersion() string {
	if m.image == "" {
		return defaultImageTag
	}

	return m.image[strings.LastIndex(m.image, ":")+1 : len(m.image)]
}

// Exists returns true if the container for this machine exists.
func (m *Machine) Exists() bool {
	return m.container != nil
}

// Name returns the name of the machine.
func (m *Machine) Name() string {
	return m.machine
}

// ContainerName return the name of the container for this machine.
func (m *Machine) ContainerName() string {
	return machineContainerName(m.cluster, m.machine)
}

// ProviderID return the provider identifier for this machine.
func (m *Machine) ProviderID() string {
	return fmt.Sprintf("docker:////%s", m.ContainerName())
}

func (m *Machine) Address(ctx context.Context) (string, error) {
	ipv4, ipv6, err := m.container.IP(ctx)
	if err != nil {
		return "", err
	}

	if m.ipFamily == clusterv1.IPv6IPFamily {
		return ipv6, nil
	}
	return ipv4, nil
}

// Create creates a docker container hosting a Kubernetes node.
func (m *Machine) Create(ctx context.Context, role string, version *string, mounts []infrav1.Mount) error {
	log := ctrl.LoggerFrom(ctx)

	// Create if not exists.
	if m.container == nil {
		var err error

		machineImage := m.machineImage(version)
		if m.image != "" {
			machineImage = m.image
		}

		switch role {
		case constants.ControlPlaneNodeRoleValue:
			log.Info("Creating control plane machine container")
			m.container, err = m.nodeCreator.CreateControlPlaneNode(
				ctx,
				m.ContainerName(),
				machineImage,
				m.cluster,
				"127.0.0.1",
				0,
				kindMounts(mounts),
				nil,
				m.labels,
				m.ipFamily,
			)
			if err != nil {
				return errors.WithStack(err)
			}
		case constants.WorkerNodeRoleValue:
			log.Info("Creating worker machine container")
			m.container, err = m.nodeCreator.CreateWorkerNode(
				ctx,
				m.ContainerName(),
				machineImage,
				m.cluster,
				kindMounts(mounts),
				nil,
				m.labels,
				m.ipFamily,
			)
			if err != nil {
				return errors.WithStack(err)
			}
		default:
			return errors.Errorf("unable to create machine for role %s", role)
		}
		// After creating a node we need to wait a small amount of time until crictl does not return an error.
		// This fixes an issue where we try to kubeadm init too quickly after creating the container.
		err = wait.PollImmediate(500*time.Millisecond, 4*time.Second, func() (bool, error) {
			ps := m.container.Commander.Command("crictl", "ps")
			return ps.Run(ctx) == nil, nil
		})
		if err != nil {
			log.Info("Failed running command", "command", "crictl ps")
			logContainerDebugInfo(log, m.ContainerName())
			return errors.Wrap(errors.WithStack(err), "failed to run crictl ps")
		}
		return nil
	}
	return nil
}

func kindMounts(mounts []infrav1.Mount) []v1alpha4.Mount {
	if len(mounts) == 0 {
		return nil
	}

	ret := make([]v1alpha4.Mount, 0, len(mounts))
	for _, m := range mounts {
		ret = append(ret, v1alpha4.Mount{
			ContainerPath: m.ContainerPath,
			HostPath:      m.HostPath,
			Readonly:      m.Readonly,
			Propagation:   v1alpha4.MountPropagationNone,
		})
	}
	return ret
}

func (m *Machine) PreloadLoadImages(ctx context.Context, images []string) error {
	// Save the image into a tar
	dir, err := os.MkdirTemp("", "image-tar")
	if err != nil {
		return errors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)

	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	for i, image := range images {
		imageTarPath := filepath.Clean(filepath.Join(dir, fmt.Sprintf("image-%d.tar", i)))

		err = containerRuntime.SaveContainerImage(ctx, image, imageTarPath)
		if err != nil {
			return errors.Wrap(err, "failed to save image")
		}

		f, err := os.Open(imageTarPath)
		if err != nil {
			return errors.Wrap(err, "failed to open image")
		}
		defer f.Close()

		ps := m.container.Commander.Command("ctr", "--namespace=k8s.io", "images", "import", "-")
		ps.SetStdin(f)
		if err := ps.Run(ctx); err != nil {
			return errors.Wrap(err, "failed to load image")
		}
	}
	return nil
}

// ExecBootstrap runs bootstrap on a node, this is generally `kubeadm <init|join>`.
func (m *Machine) ExecBootstrap(ctx context.Context, data string) error {
	log := ctrl.LoggerFrom(ctx)

	if m.container == nil {
		return errors.New("unable to set ExecBootstrap. the container hosting this machine does not exists")
	}

	cloudConfig, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errors.Wrap(err, "failed to decode machine's bootstrap data")
	}

	commands, err := cloudinit.Commands(cloudConfig)
	if err != nil {
		log.Info("cloud config failed to parse", "bootstrap data", data)
		return errors.Wrap(err, "failed to join a control plane node with kubeadm")
	}

	var outErr bytes.Buffer
	var outStd bytes.Buffer
	for _, command := range commands {
		cmd := m.container.Commander.Command(command.Cmd, command.Args...)
		cmd.SetStderr(&outErr)
		cmd.SetStdout(&outStd)
		if command.Stdin != "" {
			cmd.SetStdin(strings.NewReader(command.Stdin))
		}
		err := cmd.Run(ctx)
		if err != nil {
			log.Info("Failed running command", "command", command, "stdout", outStd.String(), "stderr", outErr.String(), "bootstrap data", data)
			logContainerDebugInfo(log, m.ContainerName())
			return errors.Wrap(errors.WithStack(err), "failed to run cloud config")
		}
	}

	return nil
}

// CheckForBootstrapSuccess checks if bootstrap was successful by checking for existence of the sentinel file.
func (m *Machine) CheckForBootstrapSuccess(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	if m.container == nil {
		return errors.New("unable to set CheckForBootstrapSuccess. the container hosting this machine does not exists")
	}

	var outErr bytes.Buffer
	var outStd bytes.Buffer
	cmd := m.container.Commander.Command("test", "-f", "/run/cluster-api/bootstrap-success.complete")
	cmd.SetStderr(&outErr)
	cmd.SetStdout(&outStd)
	if err := cmd.Run(ctx); err != nil {
		log.Info("Failed running command", "command", "test -f /run/cluster-api/bootstrap-success.complete", "stdout", outStd.String(), "stderr", outErr.String())
		return errors.Wrap(errors.WithStack(err), "failed to run bootstrap check")
	}
	return nil
}

// SetNodeProviderID sets the docker provider ID for the kubernetes node.
func (m *Machine) SetNodeProviderID(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	kubectlNode, err := m.getKubectlNode()
	if err != nil {
		return errors.Wrapf(err, "unable to set NodeProviderID. error getting a kubectl node")
	}
	if kubectlNode == nil {
		return errors.New("unable to set NodeProviderID. there are no kubectl node available")
	}
	if !kubectlNode.IsRunning() {
		return errors.Wrapf(ContainerNotRunningError{Name: kubectlNode.Name}, "unable to set NodeProviderID")
	}

	log.Info("Setting Kubernetes node providerID")
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
			log.Info(line)
		}
		return errors.Wrap(err, "failed update providerID")
	}

	return nil
}

func (m *Machine) getKubectlNode() (*types.Node, error) {
	// collect info about the existing controlplane nodes
	filters := container.FilterBuilder{}
	filters.AddKeyNameValue(filterLabel, clusterLabelKey, m.cluster)
	filters.AddKeyNameValue(filterLabel, nodeRoleLabelKey, constants.ControlPlaneNodeRoleValue)

	kubectlNodes, err := listContainers(filters)
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
	log := ctrl.LoggerFrom(ctx)

	// Delete if exists.
	if m.container != nil {
		log.Info("Deleting machine container")
		if err := m.container.Delete(ctx); err != nil {
			return err
		}
	}
	return nil
}

// machineImage is the image of the container node with the machine.
func (m *Machine) machineImage(version *string) string {
	if version == nil {
		defaultImage := fmt.Sprintf("%s:%s", defaultImageName, defaultImageTag)
		return defaultImage
	}

	// TODO(fp) make this smarter
	// - allows usage of custom docker repository & image names
	// - add v only for semantic versions
	versionString := *version
	if !strings.HasPrefix(versionString, "v") {
		versionString = fmt.Sprintf("v%s", versionString)
	}

	versionString = clusterapicontainer.SemverToOCIImageTag(versionString)

	return fmt.Sprintf("%s:%s", defaultImageName, versionString)
}

func logContainerDebugInfo(log logr.Logger, name string) {
	// let's use our own context, so we are able to get debug information even
	// when the context used in the layers above is already timed out
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		log.Error(err, "failed to connect to container runtime")
		return
	}

	var buffer bytes.Buffer
	err = containerRuntime.ContainerDebugInfo(ctx, name, &buffer)
	if err != nil {
		log.Error(err, "failed to get logs from the machine container")
		return
	}
	log.Info("Got logs from the machine container", "output", strings.ReplaceAll(buffer.String(), "\\n", "\n"))
}
