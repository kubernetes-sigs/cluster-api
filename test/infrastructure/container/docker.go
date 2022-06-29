/*
Copyright 2021 The Kubernetes Authors.

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

// Package container provides an interface for interacting with Docker and potentially
// other container runtimes.
package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerfilters "github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/pkg/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	httpProxy  = "HTTP_PROXY"
	httpsProxy = "HTTPS_PROXY"
	noProxy    = "NO_PROXY"
)

type dockerRuntime struct {
	dockerClient *client.Client
}

// NewDockerClient gets a client for interacting with a Docker container runtime.
func NewDockerClient() (Runtime, error) {
	dockerClient, err := getDockerClient()
	if err != nil {
		return nil, fmt.Errorf("failed to created docker runtime client")
	}
	return &dockerRuntime{
		dockerClient: dockerClient,
	}, nil
}

// getDockerClient returns a new client connection for interacting with the Docker engine.
func getDockerClient() (*client.Client, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %v", err)
	}

	return dockerClient, nil
}

// SaveContainerImage saves a Docker image to the file specified by dest.
func (d *dockerRuntime) SaveContainerImage(ctx context.Context, image, dest string) error {
	reader, err := d.dockerClient.ImageSave(ctx, []string{image})
	if err != nil {
		return fmt.Errorf("unable to read image data: %v", err)
	}
	defer reader.Close()

	tar, err := os.Create(dest) //nolint:gosec // No security issue: dest is safe.
	if err != nil {
		return fmt.Errorf("failed to create destination file %q: %v", dest, err)
	}
	defer tar.Close()

	_, err = io.Copy(tar, reader)
	if err != nil {
		return fmt.Errorf("failure writing image data to file: %v", err)
	}

	return nil
}

// PullContainerImageIfNotExists triggers the Docker engine to pull an image, but only if it doesn't
// already exist. This is important when we're using locally build images in CI which
// do not exist remotely.
func (d *dockerRuntime) PullContainerImageIfNotExists(ctx context.Context, image string) error {
	imageExistsLocally, err := d.ImageExistsLocally(ctx, image)
	if err != nil {
		return errors.Wrapf(err, "failure determining if the image exists in local cache: %s", image)
	}
	if imageExistsLocally {
		return nil
	}

	return d.PullContainerImage(ctx, image)
}

// PullContainerImage triggers the Docker engine to pull an image.
func (d *dockerRuntime) PullContainerImage(ctx context.Context, image string) error {
	pullResp, err := d.dockerClient.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failure pulling container image: %v", err)
	}
	defer pullResp.Close()

	// Clients must read the ImagePull response to EOF to complete the pull
	// operation or errors can occur.
	if _, err = io.ReadAll(pullResp); err != nil {
		return fmt.Errorf("error while reading container image: %v", err)
	}

	return nil
}

// ImageExistsLocally returns if the specified image exists in local container image cache.
func (d *dockerRuntime) ImageExistsLocally(ctx context.Context, image string) (bool, error) {
	filters := dockerfilters.NewArgs()
	filters.Add("reference", image)
	images, err := d.dockerClient.ImageList(ctx, types.ImageListOptions{
		Filters: filters,
	})
	if err != nil {
		return false, errors.Wrapf(err, "failure listing container image: %s", image)
	}
	if len(images) > 0 {
		return true, nil
	}
	return false, nil
}

// GetHostPort looks up the host port bound for the port and protocol (e.g. "6443/tcp").
func (d *dockerRuntime) GetHostPort(ctx context.Context, containerName, portAndProtocol string) (string, error) {
	// Get details about the container
	containerInfo, err := d.dockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", fmt.Errorf("error getting container information for %q: %v", containerName, err)
	}

	// Loop through the container port bindings and return the first HostPort
	for port, bindings := range containerInfo.NetworkSettings.Ports {
		if string(port) == portAndProtocol {
			for _, binding := range bindings {
				return binding.HostPort, nil
			}
		}
	}

	return "", fmt.Errorf("no host port found for load balancer %q", containerName)
}

// ExecContainer executes a command in a running container and writes any output to the provided writer.
func (d *dockerRuntime) ExecContainer(ctxd context.Context, containerName string, config *ExecContainerInput, command string, args ...string) error {
	ctx := context.Background() // Let the command finish, even if it takes longer than the default timeout
	execConfig := types.ExecConfig{
		// Run with privileges so we can remount etc..
		// This might not make sense in the most general sense, but it is
		// important to many kind commands.
		Privileged:   true,
		Cmd:          append([]string{command}, args...),
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  config.InputBuffer != nil,
		Env:          config.EnvironmentVars,
	}

	response, err := d.dockerClient.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return errors.Wrap(err, "error creating container exec")
	}

	execID := response.ID
	if execID == "" {
		return errors.Wrap(err, "exec ID empty")
	}

	resp, err := d.dockerClient.ContainerExecAttach(ctx, execID, types.ExecStartCheck{})
	if err != nil {
		return errors.Wrap(err, "error attaching to container exec")
	}
	defer resp.Close()

	// If there is input, send it through to its stdin
	inputErrors := make(chan error)
	if config.InputBuffer != nil {
		go func() {
			_, err := io.Copy(resp.Conn, config.InputBuffer)
			inputErrors <- err
			_ = resp.CloseWrite()
		}()
	}

	if config.OutputBuffer == nil {
		// We always want to read whatever output the command sends
		config.OutputBuffer = &bytes.Buffer{}
	}

	outputErrors := make(chan error)
	go func() {
		// Send the output to the output writer
		var err error
		if config.ErrorBuffer != nil {
			_, err = stdcopy.StdCopy(config.OutputBuffer, config.ErrorBuffer, resp.Reader)
		} else {
			_, err = io.Copy(config.OutputBuffer, resp.Reader)
		}
		outputErrors <- err
		close(outputErrors)
	}()

	select {
	case err := <-inputErrors:
		if err != nil {
			return errors.Wrap(err, "error providing execution input")
		}

	case err := <-outputErrors:
		if err != nil {
			return errors.Wrap(err, "error getting execution output")
		}

	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "operation cancelled")
	}

	retry := 0
	for retry < 600 {
		inspect, err := d.dockerClient.ContainerExecInspect(ctx, execID)
		if err != nil {
			return errors.Wrap(err, "failed to get exec status")
		}

		if !inspect.Running {
			if status := inspect.ExitCode; status != 0 {
				return errors.Errorf("exited with status: %d, %s", status, config.OutputBuffer)
			}
			break
		}

		time.Sleep(time.Millisecond * 500)
		retry++
	}

	return nil
}

// ListContainers returns a list of all containers.
func (d *dockerRuntime) ListContainers(ctx context.Context, filters FilterBuilder) ([]Container, error) {
	listOptions := types.ContainerListOptions{
		All:     true,
		Limit:   -1,
		Filters: dockerfilters.NewArgs(),
	}

	// Construct our filtering options
	for key, values := range filters {
		for subkey, subvalues := range values {
			for _, v := range subvalues {
				if v == "" {
					listOptions.Filters.Add(key, subkey)
				} else {
					listOptions.Filters.Add(key, fmt.Sprintf("%s=%s", subkey, v))
				}
			}
		}
	}

	dockerContainers, err := d.dockerClient.ContainerList(ctx, listOptions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list containers")
	}

	containers := []Container{}
	for i := range dockerContainers {
		container := dockerContainerToContainer(&dockerContainers[i])
		containers = append(containers, container)
	}

	return containers, nil
}

// DeleteContainer will remove a container, forcing removal if still running.
func (d *dockerRuntime) DeleteContainer(ctx context.Context, containerName string) error {
	return d.dockerClient.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{
		Force:         true, // force the container to be delete now
		RemoveVolumes: true, // delete volumes
	})
}

// KillContainer will kill a running container with the specified signal.
func (d *dockerRuntime) KillContainer(ctx context.Context, containerName, signal string) error {
	return d.dockerClient.ContainerKill(ctx, containerName, signal)
}

// GetContainerIPs inspects a container to get its IPv4 and IPv6 IP addresses.
// Will not error if there is no IP address assigned. Calling code will need to
// determine whether that is an issue or not.
func (d *dockerRuntime) GetContainerIPs(ctx context.Context, containerName string) (string, string, error) {
	containerInfo, err := d.dockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get container details")
	}

	for _, net := range containerInfo.NetworkSettings.Networks {
		return net.IPAddress, net.GlobalIPv6Address, nil
	}

	return "", "", nil
}

// ContainerDebugInfo gets the container metadata and logs from the runtime (docker inspect, docker logs).
func (d *dockerRuntime) ContainerDebugInfo(ctx context.Context, containerName string, w io.Writer) error {
	containerInfo, err := d.dockerClient.ContainerInspect(ctx, containerName)
	if err != nil {
		return errors.Wrapf(err, "failed to inspect container %q", containerName)
	}

	fmt.Fprintln(w, "Inspected the container:")
	fmt.Fprintf(w, "%+v\n", containerInfo)

	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	}
	responseBody, err := d.dockerClient.ContainerLogs(ctx, containerInfo.ID, options)
	if err != nil {
		return errors.Wrapf(err, "error getting container logs for %q", containerName)
	}
	defer responseBody.Close()

	fmt.Fprintln(w, "Got logs from the container:")
	_, err = io.Copy(w, responseBody)
	if err != nil {
		return errors.Wrapf(err, "error reading logs from container %q", containerName)
	}
	return nil
}

// dockerContainerToContainer converts a Docker API container instance to our local
// generic container type.
func dockerContainerToContainer(container *types.Container) Container {
	return Container{
		Name:   strings.Trim(container.Names[0], "/"),
		Image:  container.Image,
		Status: container.Status,
	}
}

// RunContainer will run a docker container with the given settings and arguments, returning any errors.
func (d *dockerRuntime) RunContainer(ctx context.Context, runConfig *RunContainerInput, output io.Writer) error {
	containerConfig := dockercontainer.Config{
		Tty:          true,           // allocate a tty for entrypoint logs
		Hostname:     runConfig.Name, // make hostname match container name
		Labels:       runConfig.Labels,
		Image:        runConfig.Image,
		Cmd:          runConfig.CommandArgs,
		User:         ownerAndGroup(runConfig),
		AttachStdout: output != nil,
		AttachStderr: output != nil,
		Entrypoint:   runConfig.Entrypoint,
		Volumes:      map[string]struct{}{},
	}

	hostConfig := dockercontainer.HostConfig{
		// Running containers in a container requires privileges.
		// NOTE: we could try to replicate this with --cap-add, and use less
		// privileges, but this flag also changes some mounts that are necessary
		// including some ones docker would otherwise do by default.
		// for now this is what we want. in the future we may revisit this.
		Privileged:    true,
		SecurityOpt:   []string{"seccomp=unconfined"}, // ignore seccomp
		NetworkMode:   dockercontainer.NetworkMode(runConfig.Network),
		Tmpfs:         runConfig.Tmpfs,
		PortBindings:  nat.PortMap{},
		RestartPolicy: dockercontainer.RestartPolicy{Name: "unless-stopped"},
	}
	networkConfig := network.NetworkingConfig{}

	if runConfig.IPFamily == clusterv1.IPv6IPFamily {
		hostConfig.Sysctls = map[string]string{
			"net.ipv6.conf.all.disable_ipv6": "0",
			"net.ipv6.conf.all.forwarding":   "1",
		}
	}

	// mount /dev/mapper if docker storage driver if Btrfs or ZFS
	// https://github.com/kubernetes-sigs/kind/pull/1464
	needed, err := d.needsDevMapper(ctx)
	if err != nil {
		return errors.Wrapf(err, "unable to get Docker engine info, failed to create container %q", runConfig.Name)
	}

	if needed {
		hostConfig.Binds = append(hostConfig.Binds, "/dev/mapper:/dev/mapper:ro")
	}

	envVars := environmentVariables(runConfig)

	// pass proxy environment variables to be used by node's docker daemon
	proxyDetails, err := d.getProxyDetails(ctx, runConfig.Network)
	if err != nil {
		return errors.Wrapf(err, "error getting subnets for %q", runConfig.Network)
	}
	for key, val := range proxyDetails.Envs {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, val))
	}
	containerConfig.Env = envVars

	configureVolumes(runConfig, &containerConfig, &hostConfig)
	configurePortMappings(runConfig.PortMappings, &containerConfig, &hostConfig)

	if d.usernsRemap(ctx) {
		// We need this argument in order to make this command work
		// in systems that have userns-remap enabled on the docker daemon
		hostConfig.UsernsMode = "host"
	}

	// Make sure we have the image
	if err := d.PullContainerImageIfNotExists(ctx, runConfig.Image); err != nil {
		return errors.Wrapf(err, "error pulling container image %s", runConfig.Image)
	}

	// Create the container using our settings
	resp, err := d.dockerClient.ContainerCreate(
		ctx,
		&containerConfig,
		&hostConfig,
		&networkConfig,
		nil,
		runConfig.Name,
	)
	if err != nil {
		return errors.Wrapf(err, "error creating container %q", runConfig.Name)
	}

	var containerOutput types.HijackedResponse
	if output != nil {
		// Read out any output from the container
		attachOpts := types.ContainerAttachOptions{
			Stream: true,
			Stdin:  false,
			Stdout: true,
			Stderr: true,
		}

		// Attach to the container so we can capture the output
		containerOutput, err = d.dockerClient.ContainerAttach(ctx, resp.ID, attachOpts)
		if err != nil {
			return errors.Wrapf(err, "failed to attach to container %q", runConfig.Name)
		}
	}

	// Actually start the container
	if err := d.dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return errors.Wrapf(err, "error starting container %q", runConfig.Name)
	}

	if output != nil {
		outputErrors := make(chan error)
		go func() {
			// Send the output to the host file
			_, err = io.Copy(output, containerOutput.Reader)
			outputErrors <- err
		}()
		defer containerOutput.Close()

		// Wait for the run to complete
		statusCh, errCh := d.dockerClient.ContainerWait(ctx, resp.ID, dockercontainer.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				return errors.Wrap(err, "error waiting for container run")
			}
		case err := <-outputErrors:
			if err != nil {
				return errors.Wrap(err, "error reading output from container run")
			}
		case <-statusCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	containerJSON, err := d.dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return fmt.Errorf("error inspecting container %s: %v", resp.ID, err)
	}

	if containerJSON.ContainerJSONBase.State.ExitCode != 0 {
		return fmt.Errorf("error container run failed with exit code %d", containerJSON.ContainerJSONBase.State.ExitCode)
	}

	return nil
}

// needsDevMapper checks whether we need to mount /dev/mapper.
// This is required when the docker storage driver is Btrfs or ZFS.
// https://github.com/kubernetes-sigs/kind/pull/1464
func (d *dockerRuntime) needsDevMapper(ctx context.Context) (bool, error) {
	info, err := d.dockerClient.Info(ctx)
	if err != nil {
		return false, err
	}

	return info.Driver == "btrfs" || info.Driver == "zfs", nil
}

// ownerAndGroup gets the user configuration for the container (user:group).
func ownerAndGroup(crc *RunContainerInput) string {
	if crc.User != "" {
		if crc.Group != "" {
			return fmt.Sprintf("%s:%s", crc.User, crc.Group)
		}

		return crc.User
	}

	return ""
}

// environmentVariables gets the collection of environment variables for the container.
func environmentVariables(crc *RunContainerInput) []string {
	envVars := []string{}
	for key, val := range crc.EnvironmentVars {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, val))
	}
	return envVars
}

func configureVolumes(crc *RunContainerInput, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig) {
	seLinux := isSELinuxEnforcing()

	for source, dest := range crc.Volumes {
		if dest == "" {
			config.Volumes[source] = struct{}{}
		} else {
			if seLinux {
				hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s:z", source, dest))
			} else {
				hostConfig.Binds = append(hostConfig.Binds, fmt.Sprintf("%s:%s", source, dest))
			}
		}
	}

	for _, containerMount := range crc.Mounts {
		opts := []string{}
		if seLinux {
			// Only request relabeling if the pod provides an SELinux context. If the pod
			// does not provide an SELinux context relabeling will label the volume with
			// the container's randomly allocated MCS label. This would restrict access
			// to the volume to the container which mounts it first.
			opts = append(opts, "Z")
		}
		if containerMount.ReadOnly {
			opts = append(opts, "ro")
		}
		appendStr := ""
		if len(opts) != 0 {
			appendStr = fmt.Sprintf(":%s", strings.Join(opts, ","))
		}

		bindString := fmt.Sprintf("%s:%s%s", containerMount.Source, containerMount.Target, appendStr)
		hostConfig.Binds = append(hostConfig.Binds, bindString)
	}
}

// getSubnets returns a slice of subnets for a specified network.
func (d *dockerRuntime) getSubnets(ctx context.Context, networkName string) ([]string, error) {
	subnets := []string{}
	networkInfo, err := d.dockerClient.NetworkInspect(ctx, networkName, types.NetworkInspectOptions{})
	if err != nil {
		return subnets, errors.Wrapf(err, "failed to inspect network %q", networkName)
	}

	for _, network := range networkInfo.IPAM.Config {
		subnets = append(subnets, network.Subnet)
	}

	return subnets, nil
}

// proxyDetails contains proxy settings discovered on the host.
type proxyDetails struct {
	Envs map[string]string
}

// getProxyDetails returns a struct with the host environment proxy settings
// that should be passed to the nodes.
func (d *dockerRuntime) getProxyDetails(ctx context.Context, network string) (*proxyDetails, error) {
	var val string
	details := proxyDetails{Envs: make(map[string]string)}
	proxyEnvs := []string{httpProxy, httpsProxy, noProxy}
	proxySupport := false

	for _, name := range proxyEnvs {
		val = os.Getenv(name)
		if val == "" {
			val = os.Getenv(strings.ToLower(name))
		}
		if val == "" {
			continue
		}
		proxySupport = true
		details.Envs[name] = val
		details.Envs[strings.ToLower(name)] = val
	}

	// Specifically add the docker network subnets to NO_PROXY if we are using proxies
	if proxySupport {
		subnets, err := d.getSubnets(ctx, network)
		if err != nil {
			return &details, err
		}
		noProxyList := strings.Join(append(subnets, details.Envs[noProxy]), ",")
		details.Envs[noProxy] = noProxyList
		details.Envs[strings.ToLower(noProxy)] = noProxyList
	}

	return &details, nil
}

// usernsRemap checks if userns-remap is enabled in dockerd.
func (d *dockerRuntime) usernsRemap(ctx context.Context) bool {
	info, err := d.dockerClient.Info(ctx)
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

func isSELinuxEnforcing() bool {
	dat, err := os.ReadFile("/sys/fs/selinux/enforce")
	if err != nil {
		return false
	}
	return string(dat) == "1"
}

func configurePortMappings(portMappings []PortMapping, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig) {
	exposedPorts := nat.PortSet{}
	for _, pm := range portMappings {
		protocol := pm.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		port := nat.Port(fmt.Sprintf("%d/%s", pm.ContainerPort, protocol))
		mapping := nat.PortBinding{
			HostIP:   pm.ListenAddress,
			HostPort: fmt.Sprintf("%d", pm.HostPort),
		}
		hostConfig.PortBindings[port] = append(hostConfig.PortBindings[port], mapping)
		exposedPorts[port] = struct{}{}
		exposedPorts[nat.Port(fmt.Sprintf("%d/tcp", pm.HostPort))] = struct{}{}
	}

	config.ExposedPorts = exposedPorts
}
