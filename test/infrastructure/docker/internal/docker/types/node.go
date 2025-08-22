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

// Package types implements type functionality.
package types

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/cluster-api/test/infrastructure/container"
)

// Node can be thought of as a logical component of Kubernetes.
// A node is either a control plane node, a worker node, or a load balancer node.
type Node struct {
	Name        string
	ClusterRole string
	InternalIP  string
	Image       string
	status      string
	Commander   *ContainerCmder
}

// NewNode returns a Node with defaults.
func NewNode(name, image, role string) *Node {
	return &Node{
		Name:        name,
		Image:       image,
		ClusterRole: role,
		Commander:   GetContainerCmder(name),
	}
}

// WithStatus sets the status of the container and returns the node.
func (n *Node) WithStatus(status string) *Node {
	n.status = status
	return n
}

// String returns the name of the node.
func (n Node) String() string {
	return n.Name
}

// Role returns the role of the node.
func (n *Node) Role() (string, error) {
	return n.ClusterRole, nil
}

// IP gets the docker ipv4 and ipv6 of the node.
func (n *Node) IP(ctx context.Context) (ipv4 string, ipv6 string, err error) {
	// retrieve the IP address of the node using docker inspect
	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to connect to container runtime")
	}

	// retrieve the IP address of the node's container from the runtime
	ipv4, ipv6, err = containerRuntime.GetContainerIPs(ctx, n.Name)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get node IPs from runtime")
	}

	return ipv4, ipv6, nil
}

// IsRunning returns if the container is running.
func (n *Node) IsRunning() bool {
	return strings.HasPrefix(n.status, "Up")
}

// Delete removes the container.
func (n *Node) Delete(ctx context.Context) error {
	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	err = containerRuntime.DeleteContainer(ctx, n.Name)
	if err != nil {
		log := ctrl.LoggerFrom(ctx)

		// Use our own context, so we are able to get debug information even
		// when the context used in the layers above is already timed out.
		debugCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var buffer bytes.Buffer
		err = containerRuntime.ContainerDebugInfo(debugCtx, n.Name, &buffer)
		if err != nil {
			log.Error(err, "Failed to get logs from the machine container")
		} else {
			log.Info("Got logs from the machine container", "output", strings.ReplaceAll(buffer.String(), "\\n", "\n"))
		}

		return errors.Wrapf(err, "failed to delete container %q", n.Name)
	}

	return nil
}

// ReadFile reads a file from a running container.
func (n *Node) ReadFile(ctx context.Context, dest string) ([]byte, error) {
	command := n.Commander.Command("cp", dest, "/dev/stdout")
	stdout := bytes.Buffer{}

	command.SetStdout(&stdout)
	// Also set stderr so it does not pollute stdout.
	command.SetStderr(&bytes.Buffer{})

	if err := command.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to read file %s", dest)
	}
	return stdout.Bytes(), nil
}

// WriteFile puts a file inside a running container.
func (n *Node) WriteFile(ctx context.Context, dest, content string) error {
	// create destination directory
	cmd := n.Commander.Command("mkdir", "-p", filepath.Dir(dest))
	if err := cmd.Run(ctx); err != nil {
		return errors.Wrapf(err, "failed to create directory %s", dest)
	}

	command := n.Commander.Command("cp", "/dev/stdin", dest)
	command.SetStdin(strings.NewReader(content))
	return command.Run(ctx)
}

// Kill sends the named signal to the container.
func (n *Node) Kill(ctx context.Context, signal string) error {
	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	err = containerRuntime.KillContainer(ctx, n.Name, signal)
	if err != nil {
		return errors.Wrapf(err, "failed to kill container %q", n.Name)
	}

	return nil
}

// ContainerCmder is used for running commands within a container.
type ContainerCmder struct {
	nameOrID string
}

// GetContainerCmder gets a new ContainerCmder instance used for running commands within a container.
func GetContainerCmder(containerNameOrID string) *ContainerCmder {
	return &ContainerCmder{
		nameOrID: containerNameOrID,
	}
}

// Command is the command to be run in a container.
func (c *ContainerCmder) Command(command string, args ...string) *ContainerCmd {
	return &ContainerCmd{
		nameOrID: c.nameOrID,
		command:  command,
		args:     args,
	}
}

// ContainerCmd implements exec.Cmd for docker containers.
type ContainerCmd struct {
	nameOrID string // the container name or ID
	command  string
	args     []string
	env      []string
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
}

// Run will run a configured ContainerCmd inside a container instance.
func (c *ContainerCmd) Run(ctx context.Context) error {
	containerRuntime, err := container.RuntimeFrom(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	execConfig := container.ExecContainerInput{
		OutputBuffer:    c.stdout,
		ErrorBuffer:     c.stderr,
		InputBuffer:     c.stdin,
		EnvironmentVars: c.env,
	}

	err = containerRuntime.ExecContainer(ctx, c.nameOrID, &execConfig, c.command, c.args...)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// SetEnv sets environment variable settings to define in a node.
func (c *ContainerCmd) SetEnv(env ...string) {
	c.env = env
}

// SetStdin sets the io.Reader to use for receiving stdin input.
func (c *ContainerCmd) SetStdin(r io.Reader) {
	c.stdin = r
}

// SetStdout sets the io.Writer to use for stdout output.
func (c *ContainerCmd) SetStdout(w io.Writer) {
	c.stdout = w
}

// SetStderr sets the io.Writer to use for stderr output.
func (c *ContainerCmd) SetStderr(w io.Writer) {
	c.stderr = w
}
