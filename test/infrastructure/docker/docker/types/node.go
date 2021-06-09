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
	"sigs.k8s.io/kind/pkg/exec"
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
	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}},{{.GlobalIPv6Address}}{{end}}",
		n.Name, // ... against the "node" container
	)
	lines, err := exec.CombinedOutputLines(cmd)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get container details")
	}
	if len(lines) != 1 {
		return "", "", errors.Errorf("file should only be one line, got %d lines", len(lines))
	}
	ips := strings.Split(lines[0], ",")
	if len(ips) != 2 {
		return "", "", errors.Errorf("container addresses should have 2 values, got %d values", len(ips))
	}
	return ips[0], ips[1], nil
}

// IsRunning returns if the container is running.
func (n *Node) IsRunning() bool {
	return strings.HasPrefix(n.status, "Up")
}

// Delete removes the container.
func (n *Node) Delete(ctx context.Context) error {
	cmd := exec.CommandContext(ctx,
		"docker",
		append(
			[]string{
				"rm",
				"-f", // force the container to be delete now
				"-v", // delete volumes
			},
			n.Name,
		)...,
	)
	return cmd.Run()
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
	cmd := exec.CommandContext(ctx,
		"docker", "kill",
		"-s", signal,
		n.Name,
	)
	return errors.WithStack(cmd.Run())
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
	args := []string{
		"exec",
		// run with privileges so we can remount etc..
		// this might not make sense in the most general sense, but it is
		// important to many kind commands
		"--privileged",
	}
	if c.stdin != nil {
		args = append(args,
			"-i", // interactive so we can supply input
		)
	}
	// set env
	for _, env := range c.env {
		args = append(args, "-e", env)
	}
	// specify the container and command, after this everything will be
	// args the the command in the container rather than to docker
	args = append(
		args,
		c.nameOrID, // ... against the container
		c.command,  // with the command specified
	)
	args = append(
		args,
		// finally, with the caller args
		c.args...,
	)
	cmd := exec.CommandContext(ctx, "docker", args...)
	if c.stdin != nil {
		cmd.SetStdin(c.stdin)
	}
	if c.stderr != nil {
		cmd.SetStderr(c.stderr)
	}
	if c.stdout != nil {
		cmd.SetStdout(c.stdout)
	}
	return errors.WithStack(cmd.Run())
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
