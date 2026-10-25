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
	"regexp"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	pkgerrors "github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker/types"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning/cloudinit"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning/ignition"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
)

type nodeCreator interface {
	CreateControlPlaneNode(ctx context.Context, name, clusterName, listenAddress string, port int32, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string, ipFamily container.ClusterIPFamily, kindMapping kind.Mapping) (node *types.Node, err error)
	CreateWorkerNode(ctx context.Context, name, clusterName string, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string, ipFamily container.ClusterIPFamily, kindMapping kind.Mapping) (node *types.Node, err error)
}

// Machine implement a service for managing the docker containers hosting a kubernetes nodes.
type Machine struct {
	cluster     string
	machine     string
	ipFamily    container.ClusterIPFamily
	container   *types.Node
	nodeCreator nodeCreator
}

// NewMachine returns a new Machine service for the given Cluster/DockerCluster pair.
func NewMachine(ctx context.Context, cluster *clusterv1.Cluster, machine string, filterLabels map[string]string) (*Machine, error) {
	if cluster == nil {
		return nil, pkgerrors.New("cluster is required when creating a docker.Machine")
	}
	if cluster.Name == "" {
		return nil, pkgerrors.New("cluster name is required when creating a docker.Machine")
	}
	if machine == "" {
		return nil, pkgerrors.New("machine is required when creating a docker.Machine")
	}

	filters := container.FilterBuilder{}
	filters.AddKeyNameValue(filterLabel, clusterLabelKey, cluster.Name)
	filters.AddKeyValue(filterName, fmt.Sprintf("^%s$", MachineContainerName(cluster.Name, machine)))
	for key, val := range filterLabels {
		filters.AddKeyNameValue(filterLabel, key, val)
	}

	newContainer, err := getContainer(ctx, filters)
	if err != nil {
		return nil, err
	}

	ipFamily, err := container.GetClusterIPFamily(cluster)
	if err != nil {
		return nil, fmt.Errorf("create docker machine: %s", err)
	}

	return &Machine{
		cluster:     cluster.Name,
		machine:     machine,
		ipFamily:    ipFamily,
		container:   newContainer,
		nodeCreator: &Manager{},
	}, nil
}

// ListMachinesByCluster will retrieve a list of all machines that are part of the given cluster.
func ListMachinesByCluster(ctx context.Context, cluster *clusterv1.Cluster, labels map[string]string) ([]*Machine, error) {
	if cluster == nil {
		return nil, pkgerrors.New("cluster is required when listing machines in the cluster")
	}
	if cluster.Name == "" {
		return nil, pkgerrors.New("cluster name is required when listing machines in the cluster")
	}

	filters := container.FilterBuilder{}
	filters.AddKeyNameValue(filterLabel, clusterLabelKey, cluster.Name)
	for key, val := range labels {
		filters.AddKeyNameValue(filterLabel, key, val)
	}

	containers, err := listContainers(ctx, filters)
	if err != nil {
		return nil, err
	}

	ipFamily, err := container.GetClusterIPFamily(cluster)
	if err != nil {
		return nil, fmt.Errorf("list docker machines by cluster: %s", err)
	}

	machines := make([]*Machine, len(containers))
	for i, containerNode := range containers {
		machines[i] = &Machine{
			cluster:     cluster.Name,
			machine:     machineFromContainerName(cluster.Name, containerNode.Name),
			ipFamily:    ipFamily,
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

// Exists returns true if the container for this machine exists.
func (m *Machine) Exists() bool {
	return m.container != nil
}

// IsRunning returns true if the container for this machine is running.
func (m *Machine) IsRunning() bool {
	if !m.Exists() {
		return false
	}

	return m.container.IsRunning()
}

// Name returns the name of the machine.
func (m *Machine) Name() string {
	return m.machine
}

// ContainerName return the name of the container for this machine.
func (m *Machine) ContainerName() string {
	return MachineContainerName(m.cluster, m.machine)
}

// Command return a "docker exec" command for the container for this machine.
func (m *Machine) Command(command string, args ...string) *types.ContainerCmd {
	return m.container.Commander.Command(command, args...)
}

// ProviderID return the provider identifier for this machine.
func (m *Machine) ProviderID() string {
	return fmt.Sprintf("docker:////%s", m.ContainerName())
}

// Address will get the IP address of the machine. It can return
// a single IPv4 address, a single IPv6 address or one of each depending on the machine.ipFamily.
func (m *Machine) Address(ctx context.Context) ([]string, error) {
	ipv4, ipv6, err := m.container.IP(ctx)
	if err != nil {
		return nil, err
	}
	switch m.ipFamily {
	case container.IPv6IPFamily:
		return []string{ipv6}, nil
	case container.IPv4IPFamily:
		return []string{ipv4}, nil
	case container.DualStackIPFamily:
		return []string{ipv4, ipv6}, nil
	}
	return nil, pkgerrors.New("unknown ipFamily")
}

// ContainerImage return the image of the container for this machine
// or empty string if the container does not exist yet.
func (m *Machine) ContainerImage() string {
	if m.container == nil {
		return ""
	}
	return m.container.Image
}

// Create creates a docker container hosting a Kubernetes node.
func (m *Machine) Create(ctx context.Context, image string, role string, version string, labels map[string]string, mounts []infrav1.Mount) error {
	log := ctrl.LoggerFrom(ctx)

	// Create if not exists.
	if m.container == nil {
		var err error

		// Get the KindMapping for the target K8s version.
		// NOTE: The KindMapping allows to select the most recent kindest/node image available, if any, as well as
		// provide info about the mode to be used when starting the kindest/node image itself.
		if version == "" {
			return pkgerrors.New("cannot create a DockerMachine for a nil version")
		}

		semVer, err := semver.ParseTolerant(version)
		if err != nil {
			return pkgerrors.Wrap(err, "failed to parse DockerMachine version")
		}

		kindMapping := kind.GetMapping(semVer, image)

		switch role {
		case constants.ControlPlaneNodeRoleValue:
			log.Info(fmt.Sprintf("Creating control plane machine container with image %s, mode %s", kindMapping.Image, kindMapping.Mode))
			m.container, err = m.nodeCreator.CreateControlPlaneNode(
				ctx,
				m.ContainerName(),
				m.cluster,
				"127.0.0.1",
				0,
				kindMounts(mounts),
				nil,
				labels,
				m.ipFamily,
				kindMapping,
			)
			if err != nil {
				return pkgerrors.WithStack(err)
			}
		case constants.WorkerNodeRoleValue:
			log.Info(fmt.Sprintf("Creating worker machine container with image %s, mode %s", kindMapping.Image, kindMapping.Mode))
			m.container, err = m.nodeCreator.CreateWorkerNode(
				ctx,
				m.ContainerName(),
				m.cluster,
				kindMounts(mounts),
				nil,
				labels,
				m.ipFamily,
				kindMapping,
			)
			if err != nil {
				return pkgerrors.WithStack(err)
			}
		default:
			return pkgerrors.Errorf("unable to create machine for role %s", role)
		}
	}
	return nil
}

// WaitForCrictlPs wait a small amount of time until crictl does not return an error.
// Note: This was an initial try to avoid issues that might happen when we try to kubeadm init too quickly after creating the container.
// We now have a better solution that also checks for cgroups ready.
func (m *Machine) WaitForCrictlPs(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)
	err := wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 4*time.Second, true, func(ctx context.Context) (bool, error) {
		ps := m.Command("crictl", "ps")
		return ps.Run(ctx) == nil, nil
	})
	if err != nil {
		log.Info("Failed running command", "command", "crictl ps")
		m.LogContainerDebugInfo(ctx)
		return pkgerrors.Wrap(err, "failed to run crictl ps")
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

// PreloadLoadImage import an image into a machine.
func (m *Machine) PreloadLoadImage(ctx context.Context, image string) error {
	// Save the image into a tar
	dir, err := os.MkdirTemp("", "image-tar")
	if err != nil {
		return pkgerrors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)

	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to connect to container runtime")
	}

	imageTarPath := filepath.Clean(filepath.Join(dir, "image.tar"))

	err = containerRuntime.SaveContainerImage(ctx, image, imageTarPath)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to save image %q to %q", image, imageTarPath)
	}

	f, err := os.Open(imageTarPath)
	if err != nil {
		return pkgerrors.Wrapf(err, "failed to open image %q from %q", image, imageTarPath)
	}
	defer func() {
		_ = f.Close()
	}()

	ps := m.Command("ctr", "--namespace=k8s.io", "images", "import", "-")
	ps.SetStdin(f)
	if err := ps.Run(ctx); err != nil {
		return pkgerrors.Wrapf(err, "failed to load image %q", image)
	}
	return nil
}

// Note: See https://github.com/kubernetes-sigs/kind/pull/2421 for more context.
var waitUntilLogRegExp = regexp.MustCompile("Reached target .*Multi-User System.*")

// WaitForMultiUserTarget checks if the multi-user target is reached to figure out if the container is ready for bootstrap exec.
func (m *Machine) WaitForMultiUserTarget(ctx context.Context, containerRuntime container.Runtime) error {
	if m.container == nil {
		return pkgerrors.New("the container hosting this machine does not exist")
	}

	logs, err := containerRuntime.GetContainerLogs(ctx, m.container.Name)
	if err != nil {
		return err
	}

	if !waitUntilLogRegExp.MatchString(logs) {
		return pkgerrors.New("multi-user target not reached yet")
	}

	return m.WaitForCrictlPs(ctx)
}

// GetBootstrapCommands return bootstrap commands for a Machine, this is generally `kubeadm <init|join>` plus additional commands.
func (m *Machine) GetBootstrapCommands(_ context.Context, data string, format bootstrapv1.Format, version string, image string) ([]provisioning.Cmd, error) {
	if m.container == nil {
		return nil, pkgerrors.New("unable to set ExecBootstrap. the container hosting this machine does not exist")
	}

	// Get the kindMapping for the target K8s version.
	// NOTE: The kindMapping allows to select the most recent kindest/node image available, if any, as well as
	// provide info about the mode to be used when starting the kindest/node image itself.
	if version == "" {
		return nil, pkgerrors.New("cannot create a DockerMachine for a nil version")
	}

	semVer, err := semver.ParseTolerant(version)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to parse DockerMachine version")
	}

	kindMapping := kind.GetMapping(semVer, image)

	// Decode the cloud config
	cloudConfig, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to decode machine's bootstrap data")
	}

	var commands []provisioning.Cmd

	switch format {
	case bootstrapv1.CloudConfig:
		commands, err = cloudinit.RawCloudInitToProvisioningCommands(cloudConfig, kindMapping)
	case bootstrapv1.Ignition:
		commands, err = ignition.RawIgnitionToProvisioningCommands(cloudConfig)
	default:
		return nil, fmt.Errorf("unknown provisioning format %q", format)
	}

	if err != nil {
		return nil, pkgerrors.Wrap(err, "failed to join a control plane node with kubeadm")
	}
	return commands, nil
}

// CheckForSentinelFile checks if bootstrap was already started by checking for existence of the sentinel file.
func (m *Machine) CheckForSentinelFile(ctx context.Context) (bool, error) {
	if m.container == nil {
		return false, pkgerrors.New("unable to set CheckForBootstrapSuccess. the container hosting this machine does not exists")
	}

	var outErr bytes.Buffer
	var outStd bytes.Buffer
	cmd := m.container.Commander.Command("/bin/sh", "-c", "test -f /run/cluster-api/capd.bootstrap.started && echo \"true\" || echo \"false\"")
	cmd.SetStderr(&outErr)
	cmd.SetStdout(&outStd)
	if err := cmd.Run(ctx); err != nil {
		return false, pkgerrors.Wrap(err, "failed to run bootstrap check")
	}
	return strings.Contains(outStd.String(), "true"), nil
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

		m.container = nil
	}
	return nil
}

// LogContainerDebugInfo logs additional debug info for the container.
func (m *Machine) LogContainerDebugInfo(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)

	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		log.Error(err, "Failed to connect to container runtime")
		return
	}

	// Use our own context, so we are able to get debug information even
	// when the context used in the layers above is already timed out.
	debugCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	debugCtx = container.RuntimeInto(debugCtx, containerRuntime)

	var buffer bytes.Buffer
	err = containerRuntime.ContainerDebugInfo(debugCtx, m.Name(), &buffer)
	if err != nil {
		log.Error(err, "Failed to get logs from container")
		return
	}
	log.Info("Debug info from the container", "info", strings.ReplaceAll(buffer.String(), "\\n", "\n"))
}
