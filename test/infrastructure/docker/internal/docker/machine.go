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

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/util/taints"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1beta2"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker/types"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning/cloudinit"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning/ignition"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
	"sigs.k8s.io/cluster-api/util/patch"
)

var (
	cloudProviderTaint = corev1.Taint{Key: "node.cloudprovider.kubernetes.io/uninitialized", Effect: corev1.TaintEffectNoSchedule}
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

// Name returns the name of the machine.
func (m *Machine) Name() string {
	return m.machine
}

// ContainerName return the name of the container for this machine.
func (m *Machine) ContainerName() string {
	return MachineContainerName(m.cluster, m.machine)
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
	return nil, errors.New("unknown ipFamily")
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
func (m *Machine) Create(ctx context.Context, image string, role string, version *string, labels map[string]string, mounts []infrav1.Mount) error {
	log := ctrl.LoggerFrom(ctx)

	// Create if not exists.
	if m.container == nil {
		var err error

		// Get the KindMapping for the target K8s version.
		// NOTE: The KindMapping allows to select the most recent kindest/node image available, if any, as well as
		// provide info about the mode to be used when starting the kindest/node image itself.
		if version == nil {
			return errors.New("cannot create a DockerMachine for a nil version")
		}

		semVer, err := semver.Parse(strings.TrimPrefix(*version, "v"))
		if err != nil {
			return errors.Wrap(err, "failed to parse DockerMachine version")
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
				return errors.WithStack(err)
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
				return errors.WithStack(err)
			}
		default:
			return errors.Errorf("unable to create machine for role %s", role)
		}
		// After creating a node we need to wait a small amount of time until crictl does not return an error.
		// This fixes an issue where we try to kubeadm init too quickly after creating the container.
		err = wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, 4*time.Second, true, func(ctx context.Context) (bool, error) {
			ps := m.container.Commander.Command("crictl", "ps")
			return ps.Run(ctx) == nil, nil
		})
		if err != nil {
			log.Info("Failed running command", "command", "crictl ps")
			logContainerDebugInfo(ctx, log, m.ContainerName())
			return errors.Wrap(err, "failed to run crictl ps")
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

// PreloadLoadImages takes a list of container images and imports them into a machine.
func (m *Machine) PreloadLoadImages(ctx context.Context, images []string) error {
	// Save the image into a tar
	dir, err := os.MkdirTemp("", "image-tar")
	if err != nil {
		return errors.Wrap(err, "failed to create tempdir")
	}
	defer os.RemoveAll(dir)

	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	for i, image := range images {
		imageTarPath := filepath.Clean(filepath.Join(dir, fmt.Sprintf("image-%d.tar", i)))

		err = containerRuntime.SaveContainerImage(ctx, image, imageTarPath)
		if err != nil {
			return errors.Wrapf(err, "failed to save image %q to %q", image, imageTarPath)
		}

		f, err := os.Open(imageTarPath)
		if err != nil {
			return errors.Wrapf(err, "failed to open image %q from %q", image, imageTarPath)
		}
		defer f.Close() //nolint:gocritic // No resource leak.

		ps := m.container.Commander.Command("ctr", "--namespace=k8s.io", "images", "import", "-")
		ps.SetStdin(f)
		if err := ps.Run(ctx); err != nil {
			return errors.Wrapf(err, "failed to load image %q", image)
		}
	}
	return nil
}

// ExecBootstrap runs bootstrap on a node, this is generally `kubeadm <init|join>`.
func (m *Machine) ExecBootstrap(ctx context.Context, data string, format bootstrapv1.Format, version *string, image string) error {
	log := ctrl.LoggerFrom(ctx)

	if m.container == nil {
		return errors.New("unable to set ExecBootstrap. the container hosting this machine does not exists")
	}

	// Get the kindMapping for the target K8s version.
	// NOTE: The kindMapping allows to select the most recent kindest/node image available, if any, as well as
	// provide info about the mode to be used when starting the kindest/node image itself.
	if version == nil {
		return errors.New("cannot create a DockerMachine for a nil version")
	}

	semVer, err := semver.Parse(strings.TrimPrefix(*version, "v"))
	if err != nil {
		return errors.Wrap(err, "failed to parse DockerMachine version")
	}

	kindMapping := kind.GetMapping(semVer, image)

	// Decode the cloud config
	cloudConfig, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errors.Wrap(err, "failed to decode machine's bootstrap data")
	}

	var commands []provisioning.Cmd

	switch format {
	case bootstrapv1.CloudConfig:
		commands, err = cloudinit.RawCloudInitToProvisioningCommands(cloudConfig, kindMapping)
	case bootstrapv1.Ignition:
		commands, err = ignition.RawIgnitionToProvisioningCommands(cloudConfig)
	default:
		return fmt.Errorf("unknown provisioning format %q", format)
	}

	if err != nil {
		log.Info("provisioning code failed to parse", "bootstrap data", data)
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
			log.Info("Failed running command", "instance", m.Name(), "command", command, "stdout", outStd.String(), "stderr", outErr.String(), "bootstrap data", data)
			logContainerDebugInfo(ctx, log, m.ContainerName())
			return errors.Wrapf(err, "failed to run cloud config: stdout: %s stderr: %s", outStd.String(), outErr.String())
		}
	}

	return nil
}

// CheckForBootstrapSuccess checks if bootstrap was successful by checking for existence of the sentinel file.
func (m *Machine) CheckForBootstrapSuccess(ctx context.Context, logResult bool) error {
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
		if logResult {
			log.Info("Failed running command", "command", "test -f /run/cluster-api/bootstrap-success.complete", "stdout", outStd.String(), "stderr", outErr.String())
		}
		return errors.Wrap(err, "failed to run bootstrap check")
	}
	return nil
}

// SetNodeProviderID sets the docker provider ID for the kubernetes node.
func (m *Machine) SetNodeProviderID(ctx context.Context, c client.Client) error {
	log := ctrl.LoggerFrom(ctx)

	dockerNode, err := m.getDockerNode(ctx)
	if err != nil {
		return errors.Wrapf(err, "unable to set NodeProviderID. error getting a kubectl node")
	}
	if !dockerNode.IsRunning() {
		return errors.Wrapf(ContainerNotRunningError{Name: dockerNode.Name}, "unable to set NodeProviderID")
	}

	node := &corev1.Node{}
	if err = c.Get(ctx, apimachinerytypes.NamespacedName{Name: m.ContainerName()}, node); err != nil {
		return errors.Wrap(err, "failed to retrieve node")
	}

	log.Info("Setting Kubernetes node providerID")

	patchHelper, err := patch.NewHelper(node, c)
	if err != nil {
		return err
	}

	node.Spec.ProviderID = m.ProviderID()

	if err = patchHelper.Patch(ctx, node); err != nil {
		return errors.Wrap(err, "failed to set providerID")
	}

	return nil
}

// CloudProviderNodePatch performs the tasks that would normally be down by an external cloud provider.
// 1) For all CAPD Nodes it sets the ProviderID on the Kubernetes Node.
// 2) If the cloudProviderTaint is set it updates the addresses in the Kubernetes Node `.status.addresses`.
// 3) If the cloudProviderTaint is set it removes it to inform Kubernetes that this Node is now initialized.
func (m *Machine) CloudProviderNodePatch(ctx context.Context, c client.Client, addresses []clusterv1.MachineAddress) error {
	log := ctrl.LoggerFrom(ctx)

	dockerNode, err := m.getDockerNode(ctx)
	if err != nil {
		return errors.Wrapf(err, "unable to complete Docker Cloud Provider tasks. Error getting a docker node")
	}
	if !dockerNode.IsRunning() {
		return errors.Wrapf(ContainerNotRunningError{Name: dockerNode.Name}, "unable to complete Docker Cloud Provider tasks")
	}

	node := &corev1.Node{}
	if err = c.Get(ctx, apimachinerytypes.NamespacedName{Name: m.ContainerName()}, node); err != nil {
		return errors.Wrap(err, "unable to complete Docker Cloud Provider tasks: failed to retrieve node")
	}

	patchHelper, err := patch.NewHelper(node, c)
	if err != nil {
		return err
	}

	// 1) Set the providerID on the node.
	log.Info("Setting Kubernetes node providerID")
	node.Spec.ProviderID = m.ProviderID()

	// If the node is managed by an external cloud provider - e.g. in dualstack tests - add the
	// machine addresses on the node and remove the cloudProviderTaint.
	if taints.HasTaint(node.Spec.Taints, cloudProviderTaint) {
		// The machine addresses must retain their order - i.e. new addresses should only be appended to the list.
		// This is what Kubelet expects when setting new IPs for pods using the host network.
		nodeAddressMap := map[corev1.NodeAddress]bool{}
		for _, addr := range node.Status.Addresses {
			nodeAddressMap[addr] = true
		}
		log.Info("Setting Kubernetes node IP Addresses")
		for _, addr := range addresses {
			if _, ok := nodeAddressMap[corev1.NodeAddress{Address: addr.Address, Type: corev1.NodeAddressType(addr.Type)}]; ok {
				continue
			}
			// Set the addresses in the Node `.status.addresses`
			// Only add "InternalIP" type addresses.
			// Node "ExternalIP" addresses are not well defined in Kubernetes across different cloud providers.
			// This keeps parity with what is done for dualstack nodes in Kind.
			if addr.Type != clusterv1.MachineInternalIP {
				continue
			}
			node.Status.Addresses = append(node.Status.Addresses, corev1.NodeAddress{
				Type:    corev1.NodeAddressType(addr.Type),
				Address: addr.Address,
			})
		}
		// R3) emove the cloud provider taint on the node - if it exists - to initialize it.
		if taints.RemoveNodeTaint(node, cloudProviderTaint) {
			log.Info("Removing the cloudprovider taint to initialize node")
		}
	}

	return patchHelper.Patch(ctx, node)
}

func (m *Machine) getDockerNode(ctx context.Context) (*types.Node, error) {
	// collect info about the existing nodes
	filters := container.FilterBuilder{}
	filters.AddKeyNameValue(filterLabel, clusterLabelKey, m.cluster)

	dockerNodes, err := listContainers(ctx, filters)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Return the node matching the current machine, required to patch itself using its kubelet config
	for _, node := range dockerNodes {
		if node.Name == m.container.Name {
			return node, nil
		}
	}

	return nil, fmt.Errorf("there are no Docker nodes matching the container name")
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

func logContainerDebugInfo(ctx context.Context, log logr.Logger, name string) {
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
	err = containerRuntime.ContainerDebugInfo(debugCtx, name, &buffer)
	if err != nil {
		log.Error(err, "Failed to get logs from the machine container")
		return
	}
	log.Info("Got logs from the machine container", "output", strings.ReplaceAll(buffer.String(), "\\n", "\n"))
}
