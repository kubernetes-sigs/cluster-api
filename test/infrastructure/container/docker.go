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
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	httpProxy  = "HTTP_PROXY"
	httpsProxy = "HTTPS_PROXY"
	noProxy    = "NO_PROXY"
)

type docker struct {
	dockerClient *client.Client
}

// NewDockerClient gets a client for interacting with a Docker container runtime.
func NewDockerClient() (RuntimeInterface, error) {
	dockerClient, err := getDockerClient()
	if err != nil {
		return nil, fmt.Errorf("failed to created docker runtime client")
	}
	return &docker{
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
func (d *docker) SaveContainerImage(ctx context.Context, image, dest string) error {
	reader, err := d.dockerClient.ImageSave(ctx, []string{image})
	if err != nil {
		return fmt.Errorf("unable to read image data: %v", err)
	}
	defer reader.Close()

	tar, err := os.Create(dest)
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

// PullContainerImage triggers the Docker engine to pull an image if it is not
// already present.
func (d *docker) PullContainerImage(ctx context.Context, image string) error {
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

// GetHostPort looks up the host port bound for the port and protocol (e.g. "6443/tcp").
func (d *docker) GetHostPort(ctx context.Context, name, portAndProtocol string) (string, error) {
	// Get details about the container
	containerInfo, err := d.dockerClient.ContainerInspect(ctx, name)
	if err != nil {
		return "", fmt.Errorf("error getting container information for %q: %v", name, err)
	}

	// Loop through the container port bindings and return the first HostPort
	for port, bindings := range containerInfo.NetworkSettings.Ports {
		if string(port) == portAndProtocol {
			for _, binding := range bindings {
				return binding.HostPort, nil
			}
		}
	}

	return "", fmt.Errorf("no host port found for load balancer %q", name)
}

// ExecToFile executes a command in a running container and writes any output to the provided fileOnHost.
func (d *docker) ExecToFile(ctx context.Context, containerName string, fileOnHost *os.File, command string, args ...string) error {
	execConfig := types.ExecConfig{
		Privileged:   true,
		Cmd:          append([]string{command}, args...),
		AttachStdout: true,
		AttachStderr: true,
	}

	response, err := d.dockerClient.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return fmt.Errorf("error creating container exec: %v", err)
	}

	execID := response.ID
	if execID == "" {
		return fmt.Errorf("exec ID empty")
	}

	resp, err := d.dockerClient.ContainerExecAttach(ctx, execID, types.ExecStartCheck{})
	if err != nil {
		return fmt.Errorf("error attaching to container exec: %v", err)
	}
	defer resp.Close()

	// Read out any output from the call
	outputErrors := make(chan error)
	go func() {
		// Send the output to the host file
		_, err = stdcopy.StdCopy(fileOnHost, fileOnHost, resp.Reader)
		outputErrors <- err
	}()

	select {
	case err := <-outputErrors:
		if err != nil {
			return fmt.Errorf("error reading output from container exec: %v", err)
		}

	case <-ctx.Done():
		return err
	}

	return nil
}

// ownerAndGroup gets the user configuration for the container (user:group).
func (crc *RunContainerInput) ownerAndGroup() string {
	if crc.User != "" {
		if crc.Group != "" {
			return fmt.Sprintf("%s:%s", crc.User, crc.Group)
		}

		return crc.User
	}

	return ""
}

// environmentVariables gets the collection of environment variables for the container.
func (crc *RunContainerInput) environmentVariables() []string {
	envVars := []string{}
	for key, val := range crc.EnvironmentVars {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, val))
	}
	return envVars
}

// bindings gets the volume mount bindings for the container.
func (crc *RunContainerInput) bindings() []string {
	bindings := []string{}
	for src, dest := range crc.Volumes {
		bindings = append(bindings, volumeMount(src, dest))
	}
	return bindings
}

// RunContainer will run a docker container with the given settings and arguments, returning any errors.
func (d *docker) RunContainer(ctx context.Context, runConfig *RunContainerInput, output io.Writer) error {
	containerConfig := dockerContainer.Config{
		Tty:          true, // allocate a tty for entrypoint logs
		Image:        runConfig.Image,
		Cmd:          runConfig.CommandArgs,
		User:         runConfig.ownerAndGroup(),
		AttachStdout: true,
		AttachStderr: true,
		Entrypoint:   runConfig.Entrypoint,
	}

	hostConfig := dockerContainer.HostConfig{
		SecurityOpt:  []string{"seccomp=unconfined"}, // ignore seccomp
		Binds:        runConfig.bindings(),
		NetworkMode:  dockerContainer.NetworkMode(runConfig.Network),
		PortBindings: nat.PortMap{},
	}
	networkConfig := network.NetworkingConfig{}

	envVars := runConfig.environmentVariables()

	// pass proxy environment variables to be used by node's docker daemon
	proxyDetails := getProxyDetails()
	for key, val := range proxyDetails.Envs {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, val))
	}
	containerConfig.Env = envVars

	if d.usernsRemap(ctx) {
		// We need this argument in order to make this command work
		// in systems that have userns-remap enabled on the docker daemon
		hostConfig.UsernsMode = "host"
	}

	// Make sure we have the image
	if err := d.PullContainerImage(ctx, runConfig.Image); err != nil {
		return err
	}

	// Create the container using our settings
	resp, err := d.dockerClient.ContainerCreate(
		ctx,
		&containerConfig,
		&hostConfig,
		&networkConfig,
		nil,
		"",
	)
	if err != nil {
		return fmt.Errorf("error creating container: %v", err)
	}

	// Read out any output from the container
	attachOpts := types.ContainerAttachOptions{
		Stream: true,
		Stdin:  false,
		Stdout: true,
		Stderr: true,
	}

	// Attach to the container so we can capture the output
	containerOutput, err := d.dockerClient.ContainerAttach(ctx, resp.ID, attachOpts)
	if err != nil {
		return fmt.Errorf("failed to attach to container: %v", err)
	}

	// Actually start the container
	if err := d.dockerClient.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("error starting container: %v", err)
	}

	outputErrors := make(chan error)
	go func() {
		// Send the output to the host file
		_, err = io.Copy(output, containerOutput.Reader)
		outputErrors <- err
	}()
	defer containerOutput.Close()

	// Wait for the run to complete
	statusCh, errCh := d.dockerClient.ContainerWait(ctx, resp.ID, dockerContainer.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error waiting for container run: %v", err)
		}
	case err := <-outputErrors:
		if err != nil {
			return fmt.Errorf("error reading output from container run: %v", err)
		}
	case <-statusCh:
	case <-ctx.Done():
		return err
	}

	return nil
}

// proxyDetails contains proxy settings discovered on the host.
type proxyDetails struct {
	Envs map[string]string
}

// getProxyDetails returns a struct with the host environment proxy settings
// that should be passed to the nodes.
func getProxyDetails() *proxyDetails {
	var val string
	details := proxyDetails{Envs: make(map[string]string)}
	proxyEnvs := []string{httpProxy, httpsProxy, noProxy}

	for _, name := range proxyEnvs {
		val = os.Getenv(name)
		if val == "" {
			val = os.Getenv(strings.ToLower(name))
		}
		if val == "" {
			continue
		}
		details.Envs[name] = val
		details.Envs[strings.ToLower(name)] = val
	}

	return &details
}

// usernsRemap checks if userns-remap is enabled in dockerd.
func (d *docker) usernsRemap(ctx context.Context) bool {
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

func volumeMount(src, dest string) string {
	volumeArg := src + ":" + dest
	if isSELinuxEnforcing() {
		return volumeArg + ":z"
	}
	return volumeArg
}
