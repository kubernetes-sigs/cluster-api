/*
Copyright 2020 The Kubernetes Authors.

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
	"net"

	"github.com/pkg/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

const KubeadmContainerPort = 6443
const ControlPlanePort = 6443
const DefaultNetwork = "kind"

type Manager struct{}

type nodeCreateOpts struct {
	Name         string
	Image        string
	ClusterName  string
	Role         string
	Mounts       []v1alpha4.Mount
	PortMappings []v1alpha4.PortMapping
	Labels       map[string]string
	IPFamily     clusterv1.ClusterIPFamily
}

func (m *Manager) CreateControlPlaneNode(ctx context.Context, name, image, clusterName, listenAddress string, port int32, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string, ipFamily clusterv1.ClusterIPFamily) (*types.Node, error) {
	// gets a random host port for the API server
	if port == 0 {
		p, err := getPort()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get port for API server")
		}
		port = p
	}

	// add api server port mapping
	portMappingsWithAPIServer := append(portMappings, v1alpha4.PortMapping{
		ListenAddress: listenAddress,
		HostPort:      port,
		ContainerPort: KubeadmContainerPort,
		Protocol:      v1alpha4.PortMappingProtocolTCP,
	})
	createOpts := &nodeCreateOpts{
		Name:         name,
		Image:        image,
		ClusterName:  clusterName,
		Role:         constants.ControlPlaneNodeRoleValue,
		PortMappings: portMappingsWithAPIServer,
		Mounts:       mounts,
		IPFamily:     ipFamily,
	}
	node, err := createNode(ctx, createOpts)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func (m *Manager) CreateWorkerNode(ctx context.Context, name, image, clusterName string, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string, ipFamily clusterv1.ClusterIPFamily) (*types.Node, error) {
	createOpts := &nodeCreateOpts{
		Name:         name,
		Image:        image,
		ClusterName:  clusterName,
		Role:         constants.WorkerNodeRoleValue,
		PortMappings: portMappings,
		Mounts:       mounts,
		Labels:       labels,
		IPFamily:     ipFamily,
	}
	return createNode(ctx, createOpts)
}

func (m *Manager) CreateExternalLoadBalancerNode(ctx context.Context, name, image, clusterName, listenAddress string, port int32, ipFamily clusterv1.ClusterIPFamily) (*types.Node, error) {
	// gets a random host port for control-plane load balancer
	// gets a random host port for the API server
	if port == 0 {
		p, err := getPort()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get port for API server")
		}
		port = p
	}

	// load balancer port mapping
	portMappings := []v1alpha4.PortMapping{{
		ListenAddress: listenAddress,
		HostPort:      port,
		ContainerPort: ControlPlanePort,
		Protocol:      v1alpha4.PortMappingProtocolTCP,
	}}
	createOpts := &nodeCreateOpts{
		Name:         name,
		Image:        image,
		ClusterName:  clusterName,
		Role:         constants.ExternalLoadBalancerNodeRoleValue,
		PortMappings: portMappings,
	}
	node, err := createNode(ctx, createOpts)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func createNode(ctx context.Context, opts *nodeCreateOpts) (*types.Node, error) {
	log := ctrl.LoggerFrom(ctx)

	// Collect the labels to apply to the container
	containerLabels := map[string]string{
		clusterLabelKey:  opts.ClusterName,
		nodeRoleLabelKey: opts.Role,
	}
	for name, value := range opts.Labels {
		containerLabels[name] = value
	}

	runOptions := &container.RunContainerInput{
		Name:   opts.Name, // make hostname match container name
		Image:  opts.Image,
		Labels: containerLabels,
		// runtime persistent storage
		// this ensures that E.G. pods, logs etc. are not on the container
		// filesystem, which is not only better for performance, but allows
		// running kind in kind for "party tricks"
		// (please don't depend on doing this though!)
		Volumes:      map[string]string{"/var": ""},
		Mounts:       generateMountInfo(opts.Mounts),
		PortMappings: generatePortMappings(opts.PortMappings),
		Network:      DefaultNetwork,
		Tmpfs: map[string]string{
			"/tmp": "", // various things depend on working /tmp
			"/run": "", // systemd wants a writable /run
		},
		IPFamily: opts.IPFamily,
	}
	log.V(6).Info("Container run options: %+v", runOptions)

	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to container runtime: %v", err)
	}

	err = containerRuntime.RunContainer(ctx, runOptions, nil)
	if err != nil {
		return nil, err
	}

	return types.NewNode(opts.Name, opts.Image, opts.Role), nil
}

// helper used to get a free TCP port for the API server.
func getPort() (int32, error) {
	listener, err := net.Listen("tcp", ":0") //nolint:gosec
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return int32(port), nil
}

func generateMountInfo(mounts []v1alpha4.Mount) []container.Mount {
	mountInfo := []container.Mount{}
	for _, mount := range mounts {
		mountInfo = append(mountInfo, container.Mount{
			Source:   mount.HostPath,
			Target:   mount.ContainerPath,
			ReadOnly: mount.Readonly,
		})
	}
	// some k8s things want to read /lib/modules
	mountInfo = append(mountInfo, container.Mount{
		Source:   "/lib/modules",
		Target:   "/lib/modules",
		ReadOnly: true,
	})
	return mountInfo
}

func generatePortMappings(portMappings []v1alpha4.PortMapping) []container.PortMapping {
	result := make([]container.PortMapping, 0, len(portMappings))
	for _, pm := range portMappings {
		portMapping := container.PortMapping{
			ContainerPort: pm.ContainerPort,
			HostPort:      pm.HostPort,
			ListenAddress: pm.ListenAddress,
			Protocol:      capiProtocolToCommonProtocol(pm.Protocol),
		}
		result = append(result, portMapping)
	}
	return result
}

func capiProtocolToCommonProtocol(protocol v1alpha4.PortMappingProtocol) string {
	switch protocol {
	case v1alpha4.PortMappingProtocolUDP:
		return "udp"
	case v1alpha4.PortMappingProtocolSCTP:
		return "sctp"
	default:
		return "tcp"
	}
}
