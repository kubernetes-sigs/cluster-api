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
	"bufio"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
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
func (n *Node) String() string {
	return n.Name
}

// Role returns the role of the node.
func (n *Node) Role() (string, error) {
	return n.ClusterRole, nil
}

// IP gets the docker ipv4 and ipv6 of the node.
func (n *Node) IP(ctx context.Context) (ipv4 string, ipv6 string, err error) {
	// retrieve the IP address of the node using docker inspect
	containerRuntime, err := container.NewDockerClient()
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
	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	err = containerRuntime.DeleteContainer(ctx, n.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to delete container %q", n.Name)
	}

	return nil
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
	containerRuntime, err := container.NewDockerClient()
	if err != nil {
		return errors.Wrap(err, "failed to connect to container runtime")
	}

	err = containerRuntime.KillContainer(ctx, n.Name, signal)
	if err != nil {
		return errors.Wrapf(err, "failed to kill container %q", n.Name)
	}

	return nil
}

type ContainerCmder struct {
	nameOrID string
}

func GetContainerCmder(containerNameOrID string) *ContainerCmder {
	return &ContainerCmder{
		nameOrID: containerNameOrID,
	}
}

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

// RunLoggingOutputOnFail runs the cmd, logging error output if Run returns an error.
func (c *ContainerCmd) RunLoggingOutputOnFail(ctx context.Context) ([]string, error) {
	var buff bytes.Buffer
	c.SetStdout(&buff)
	c.SetStderr(&buff)
	err := c.Run(ctx)
	out := make([]string, 0)
	if err != nil {
		scanner := bufio.NewScanner(&buff)
		for scanner.Scan() {
			out = append(out, scanner.Text())
		}
	}
	return out, errors.WithStack(err)
}

func (c *ContainerCmd) Run(ctx context.Context) error {
	containerRuntime, err := container.NewDockerClient()
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

func (c *ContainerCmd) SetEnv(env ...string) {
	c.env = env
}

func (c *ContainerCmd) SetStdin(r io.Reader) {
	c.stdin = r
}

func (c *ContainerCmd) SetStdout(w io.Writer) {
	c.stdout = w
}

func (c *ContainerCmd) SetStderr(w io.Writer) {
	c.stderr = w
}
