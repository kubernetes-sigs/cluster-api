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
	"os"
	"strings"

	dockerTypes "github.com/docker/docker/api/types"
	dockerContainer "github.com/docker/docker/api/types/container"
	dockerMount "github.com/docker/docker/api/types/mount"
	dockerNetwork "github.com/docker/docker/api/types/network"
	dockerClient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"
)

const KubeadmContainerPort = 6443
const ControlPlanePort = 6443

type Manager struct{}

func (m *Manager) CreateControlPlaneNode(name, image, clusterLabel, listenAddress string, port int32, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string) (*types.Node, error) {
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
	})
	node, err := createNode(
		name, image, clusterLabel, constants.ControlPlaneNodeRoleValue, mounts, portMappingsWithAPIServer,
		// publish selected port for the API server
		append([]string{"--expose", fmt.Sprintf("%d", port)}, labelsAsArgs(labels)...)...,
	)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func (m *Manager) CreateWorkerNode(name, image, clusterLabel string, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, labels map[string]string) (*types.Node, error) {
	return createNode(name, image, clusterLabel, constants.WorkerNodeRoleValue, mounts, portMappings, labelsAsArgs(labels)...)
}

func (m *Manager) CreateExternalLoadBalancerNode(name, image, clusterLabel, listenAddress string, port int32) (*types.Node, error) {
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
	}}
	node, err := createNode(name, image, clusterLabel, constants.ExternalLoadBalancerNodeRoleValue,
		nil, portMappings,
		// publish selected port for the control plane
		"--expose", fmt.Sprintf("%d", port),
	)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func createNode(name, image, clusterLabel, role string, mounts []v1alpha4.Mount, portMappings []v1alpha4.PortMapping, extraArgs ...string) (*types.Node, error) {
	clusterLabelParts := strings.Split(clusterLabel, "=")
	if len(clusterLabelParts) != 2 {
		return nil, errors.New(fmt.Sprintf("invalid cluster label: %s", clusterLabel))
	}
	labels := map[string]string{
		clusterLabelParts[0]: clusterLabelParts[1],
		nodeRoleLabelKey:     role,
	}

	containerConfig := dockerContainer.Config{
		Tty:      true, // allocate a tty for entrypoint logs
		Hostname: name, // make hostname match container name
		Labels:   labels,
		Image:    image,
		// runtime persistent storage
		// this ensures that E.G. pods, logs etc. are not on the container
		// filesystem, which is not only better for performance, but allows
		// running kind in kind for "party tricks"
		// (please don't depend on doing this though!)
		Volumes: map[string]struct{}{"/var": {}},
	}
	hostConfig := dockerContainer.HostConfig{
		// running containers in a container requires privileged
		// NOTE: we could try to replicate this with --cap-add, and use less
		// privileges, but this flag also changes some mounts that are necessary
		// including some ones docker would otherwise do by default.
		// for now this is what we want. in the future we may revisit this.
		Privileged:  true,
		SecurityOpt: []string{"seccomp=unconfined"}, // also ignore seccomp
		// some k8s things want to read /lib/modules
		Binds:       []string{"/lib/modules:/lib/modules:ro"},
		NetworkMode: defaultNetwork,
		Tmpfs: map[string]string{
			"/tmp": "", // various things depend on working /tmp
			"/run": "", // systemd wants a writable /run
		},
		PortBindings: nat.PortMap{},
	}
	networkConfig := dockerNetwork.NetworkingConfig{}

	// pass proxy environment variables to be used by node's docker daemon
	envVars := []string{}
	proxyDetails, err := getProxyDetails()
	if err != nil || proxyDetails == nil {
		return nil, errors.Wrap(err, "proxy setup error")
	}
	for key, val := range proxyDetails.Envs {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, val))
	}
	containerConfig.Env = envVars

	configureMounts(mounts, &hostConfig)
	configurePortMappings(portMappings, &hostConfig)

	// We control these, so for now just parse out the expected format
	ports := nat.PortSet{}
	for i := 0; i < len(extraArgs); i++ {
		if extraArgs[i] == "--expose" {
			i++
			ports[nat.Port(fmt.Sprintf("%s/tcp", extraArgs[i]))] = struct{}{}
		} else if extraArgs[i] == "--label" {
			i++
			label := strings.Split(extraArgs[i], "=")
			labels[label[0]] = label[1]
		}
	}

	// We need to make sure any PortMappings are also included in the expose list
	for port := range hostConfig.PortBindings {
		ports[port] = struct{}{}
	}
	containerConfig.ExposedPorts = ports

	if usernsRemap() {
		// We need this argument in order to make this command work
		// in systems that have userns-remap enabled on the docker daemon
		hostConfig.UsernsMode = "host"
	}

	ctx := context.Background()
	cli, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.Wrap(err, "container client error")
	}

	resp, err := cli.ContainerCreate(
		ctx,
		&containerConfig,
		&hostConfig,
		&networkConfig,
		nil,
		name,
	)

	if err != nil {
		return nil, errors.Wrap(err, "container creation error")
	}

	if err := cli.ContainerStart(ctx, resp.ID, dockerTypes.ContainerStartOptions{}); err != nil {
		return nil, errors.Wrap(err, "container start error")
	}

	return types.NewNode(name, image, role), nil
}

// labelsAsArgs transforms a map of labels into extraArgs.
func labelsAsArgs(labels map[string]string) []string {
	args := make([]string, len(labels)*2)
	i := 0
	for key, val := range labels {
		args[i] = "--label"
		args[i+1] = fmt.Sprintf("%s=%s", key, val)
		i++
	}
	return args
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

// proxyDetails contains proxy settings discovered on the host.
type proxyDetails struct {
	Envs map[string]string
}

const (
	defaultNetwork = "kind"
	httpProxy      = "HTTP_PROXY"
	httpsProxy     = "HTTPS_PROXY"
	noProxy        = "NO_PROXY"
)

// getSubnets returns a slice of subnets for a specified network.
func getSubnets(networkName string) ([]string, error) {
	ctx := context.Background()
	cli, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	networkInfo, err := cli.NetworkInspect(ctx, networkName, dockerTypes.NetworkInspectOptions{})
	if err != nil {
		return nil, err
	}

	subnets := []string{}
	for _, network := range networkInfo.IPAM.Config {
		subnets = append(subnets, network.Subnet)
	}

	return subnets, err
}

// getProxyDetails returns a struct with the host environment proxy settings
// that should be passed to the nodes.
func getProxyDetails() (*proxyDetails, error) {
	var val string
	details := proxyDetails{Envs: make(map[string]string)}
	proxyEnvs := []string{httpProxy, httpsProxy, noProxy}
	proxySupport := false

	for _, name := range proxyEnvs {
		val = os.Getenv(name)
		if val != "" {
			proxySupport = true
			details.Envs[name] = val
			details.Envs[strings.ToLower(name)] = val
		} else {
			val = os.Getenv(strings.ToLower(name))
			if val != "" {
				proxySupport = true
				details.Envs[name] = val
				details.Envs[strings.ToLower(name)] = val
			}
		}
	}

	// Specifically add the docker network subnets to NO_PROXY if we are using proxies
	if proxySupport {
		subnets, err := getSubnets(defaultNetwork)
		if err != nil {
			return nil, err
		}
		noProxyList := strings.Join(append(subnets, details.Envs[noProxy]), ",")
		details.Envs[noProxy] = noProxyList
		details.Envs[strings.ToLower(noProxy)] = noProxyList
	}

	return &details, nil
}

// usernsRemap checks if userns-remap is enabled in dockerd.
func usernsRemap() bool {
	ctx := context.Background()
	cli, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}

	info, err := cli.Info(ctx)
	if err != nil {
		return false
	}

	for _, secOpt := range info.SecurityOptions {
		if strings.Contains(secOpt, "name=userns") {
			return true
		}
	}
	return false
}

// configureMounts sets the container mounts.
func configureMounts(mounts []v1alpha4.Mount, hostConfig *dockerContainer.HostConfig) {
	for _, mount := range mounts {
		hostConfig.Mounts = append(hostConfig.Mounts,
			dockerMount.Mount{
				Type:     dockerMount.TypeBind,
				Source:   mount.HostPath,
				Target:   mount.ContainerPath,
				ReadOnly: mount.Readonly,
				BindOptions: &dockerMount.BindOptions{
					Propagation: capiPropagationToDockerPropagation(mount.Propagation),
				},
			},
		)
	}
}

// capiPropagationToDockerPropagation translates the CAPI propagation type to the type to use
// with Docker.
func capiPropagationToDockerPropagation(prop v1alpha4.MountPropagation) dockerMount.Propagation {
	// "private" is default
	switch prop {
	case v1alpha4.MountPropagationBidirectional:
		return dockerMount.PropagationRShared
	case v1alpha4.MountPropagationHostToContainer:
		return dockerMount.PropagationRSlave
	default:
		return dockerMount.PropagationPrivate
	}
}

func configurePortMappings(portMappings []v1alpha4.PortMapping, hostConfig *dockerContainer.HostConfig) {
	for _, pm := range portMappings {
		port := nat.Port(fmt.Sprintf("%d/%s", pm.ContainerPort, capiProtocolToDockerProtocol(pm.Protocol)))
		mapping := nat.PortBinding{
			HostIP:   pm.ListenAddress,
			HostPort: fmt.Sprintf("%d", pm.HostPort),
		}
		hostConfig.PortBindings[port] = append(hostConfig.PortBindings[port], mapping)
	}
}

// capiProtocolToDockerProtocol translates the CAPI port protocol to the type to use
// with Docker.
func capiProtocolToDockerProtocol(protocol v1alpha4.PortMappingProtocol) string {
	switch protocol {
	case v1alpha4.PortMappingProtocolUDP:
		return "udp"
	case v1alpha4.PortMappingProtocolSCTP:
		return "sctp"
	default:
		return "tcp"
	}
}
