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
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/docker/types"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"sigs.k8s.io/kind/pkg/exec"
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
	runArgs := []string{
		"--detach", // run the container detached
		"--tty",    // allocate a tty for entrypoint logs
		// running containers in a container requires privileged
		// NOTE: we could try to replicate this with --cap-add, and use less
		// privileges, but this flag also changes some mounts that are necessary
		// including some ones docker would otherwise do by default.
		// for now this is what we want. in the future we may revisit this.
		"--privileged",
		"--security-opt", "seccomp=unconfined", // also ignore seccomp
		// runtime temporary storage
		"--tmpfs", "/tmp", // various things depend on working /tmp
		"--tmpfs", "/run", // systemd wants a writable /run
		// runtime persistent storage
		// this ensures that E.G. pods, logs etc. are not on the container
		// filesystem, which is not only better for performance, but allows
		// running kind in kind for "party tricks"
		// (please don't depend on doing this though!)
		"--volume", "/var",
		// some k8s things want to read /lib/modules
		"--volume", "/lib/modules:/lib/modules:ro",
		"--hostname", name, // make hostname match container name
		"--name", name, // ... and set the container name
		// label the node with the cluster ID
		"--label", clusterLabel,
		// label the node with the role ID
		"--label", fmt.Sprintf("%s=%s", nodeRoleLabelKey, role),
	}

	// pass proxy environment variables to be used by node's docker daemon
	proxyDetails, err := getProxyDetails()
	if err != nil || proxyDetails == nil {
		return nil, errors.Wrap(err, "proxy setup error")
	}
	for key, val := range proxyDetails.Envs {
		runArgs = append(runArgs, "-e", fmt.Sprintf("%s=%s", key, val))
	}

	// adds node specific args
	runArgs = append(runArgs, extraArgs...)

	if usernsRemap() {
		// We need this argument in order to make this command work
		// in systems that have userns-remap enabled on the docker daemon
		runArgs = append(runArgs, "--userns=host")
	}

	if err := run(
		image,
		withRunArgs(runArgs...),
		withMounts(mounts),
		withPortMappings(portMappings),
	); err != nil {
		return nil, err
	}

	return types.NewNode(name, image, role), nil
}

// labelsAsArgs transforms a map of labels into extraArgs
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

// helper used to get a free TCP port for the API server
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

// proxyDetails contains proxy settings discovered on the host
type proxyDetails struct {
	Envs map[string]string
}

const (
	// Docker default bridge network is named "bridge" (https://docs.docker.com/network/bridge/#use-the-default-bridge-network)
	defaultNetwork = "bridge"
	httpProxy      = "HTTP_PROXY"
	httpsProxy     = "HTTPS_PROXY"
	noProxy        = "NO_PROXY"
)

// networkInspect displays detailed information on one or more networks
func networkInspect(networkNames []string, format string) ([]string, error) {
	cmd := exec.Command("docker", "network", "inspect",
		"-f", format,
		strings.Join(networkNames, " "),
	)
	return exec.CombinedOutputLines(cmd)
}

// getSubnets returns a slice of subnets for a specified network
func getSubnets(networkName string) ([]string, error) {
	format := `{{range (index (index . "IPAM") "Config")}}{{index . "Subnet"}} {{end}}`
	lines, err := networkInspect([]string{networkName}, format)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSpace(lines[0]), " "), nil
}

// getProxyDetails returns a struct with the host environment proxy settings
// that should be passed to the nodes
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

// usernsRemap checks if userns-remap is enabled in dockerd
func usernsRemap() bool {
	cmd := exec.Command("docker", "info", "--format", "'{{json .SecurityOptions}}'")
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		return false
	}
	if len(lines) > 0 {
		if strings.Contains(lines[0], "name=userns") {
			return true
		}
	}
	return false
}

func run(image string, opts ...RunOpt) error {
	o := &runOpts{}
	for _, opt := range opts {
		o = opt(o)
	}
	// convert mounts to container run args
	runArgs := o.RunArgs
	for _, mount := range o.Mounts {
		runArgs = append(runArgs, generateMountBindings(mount)...)
	}
	for _, portMapping := range o.PortMappings {
		runArgs = append(runArgs, generatePortMappings(portMapping)...)
	}
	// construct the actual docker run argv
	args := []string{"run"}
	args = append(args, runArgs...)
	args = append(args, image)
	args = append(args, o.ContainerArgs...)
	cmd := exec.Command("docker", args...)
	output, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		// log error output if there was any
		for _, line := range output {
			fmt.Println(line)
		}
		return err
	}
	return nil
}

// RunOpt is an option for run
type RunOpt func(*runOpts) *runOpts

// actual options struct
type runOpts struct {
	RunArgs       []string
	ContainerArgs []string
	Mounts        []v1alpha4.Mount
	PortMappings  []v1alpha4.PortMapping
}

// withRunArgs sets the args for docker run
// as in the args portion of `docker run args... image containerArgs...`
func withRunArgs(args ...string) RunOpt {
	return func(r *runOpts) *runOpts {
		r.RunArgs = args
		return r
	}
}

// withMounts sets the container mounts
func withMounts(mounts []v1alpha4.Mount) RunOpt {
	return func(r *runOpts) *runOpts {
		r.Mounts = mounts
		return r
	}
}

// withPortMappings sets the container port mappings to the host
func withPortMappings(portMappings []v1alpha4.PortMapping) RunOpt {
	return func(r *runOpts) *runOpts {
		r.PortMappings = portMappings
		return r
	}
}

func generateMountBindings(mounts ...v1alpha4.Mount) []string {
	result := make([]string, 0, len(mounts))
	for _, m := range mounts {
		bind := fmt.Sprintf("%s:%s", m.HostPath, m.ContainerPath)
		var attrs []string
		if m.Readonly {
			attrs = append(attrs, "ro")
		}
		// Only request relabeling if the pod provides an SELinux context. If the pod
		// does not provide an SELinux context relabeling will label the volume with
		// the container's randomly allocated MCS label. This would restrict access
		// to the volume to the container which mounts it first.
		if m.SelinuxRelabel {
			attrs = append(attrs, "Z")
		}
		switch m.Propagation {
		case v1alpha4.MountPropagationNone:
			// noop, private is default
		case v1alpha4.MountPropagationBidirectional:
			attrs = append(attrs, "rshared")
		case v1alpha4.MountPropagationHostToContainer:
			attrs = append(attrs, "rslave")
		default:
			// Falls back to "private"
		}

		if len(attrs) > 0 {
			bind = fmt.Sprintf("%s:%s", bind, strings.Join(attrs, ","))
		}
		// our specific modification is the following line: make this a docker flag
		bind = fmt.Sprintf("--volume=%s", bind)
		result = append(result, bind)
	}
	return result
}

func generatePortMappings(portMappings ...v1alpha4.PortMapping) []string {
	result := make([]string, 0, len(portMappings))
	for _, pm := range portMappings {
		var hostPortBinding string
		if pm.ListenAddress != "" {
			hostPortBinding = net.JoinHostPort(pm.ListenAddress, fmt.Sprintf("%d", pm.HostPort))
		} else {
			hostPortBinding = fmt.Sprintf("%d", pm.HostPort)
		}
		var protocol string
		switch pm.Protocol {
		case v1alpha4.PortMappingProtocolTCP:
			protocol = "TCP"
		case v1alpha4.PortMappingProtocolUDP:
			protocol = "UDP"
		case v1alpha4.PortMappingProtocolSCTP:
			protocol = "SCTP"
		default:
			protocol = "TCP"
		}
		publish := fmt.Sprintf("--publish=%s:%d/%s", hostPortBinding, pm.ContainerPort, protocol)
		result = append(result, publish)
	}
	return result
}
