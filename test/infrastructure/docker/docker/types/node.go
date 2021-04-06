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

package types

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	dockerTypes "github.com/docker/docker/api/types"
	dockerClient "github.com/docker/docker/client"
	"github.com/moby/moby/pkg/stdcopy"
	"github.com/pkg/errors"
)

// GetDockerClient returns a Docker engine API client created with expected values.
func GetDockerClient() (*dockerClient.Client, error) {
	return dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
}

// Node can be thought of as a logical component of Kubernetes.
// A node is either a control plane node, a worker node, or a load balancer node.
type Node struct {
	Name        string
	ClusterRole string
	InternalIP  string
	Image       string
	status      string
	Commander   *containerCmder
}

// NewNode returns a Node with defaults.
func NewNode(name, image, role string) *Node {
	return &Node{
		Name:        name,
		Image:       image,
		ClusterRole: role,
		Commander:   ContainerCmder(name),
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
	cli, err := GetDockerClient()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get container client")
	}

	containerInfo, err := cli.ContainerInspect(ctx, n.Name)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get container details")
	}

	ip := ""
	ip6 := ""
	for _, net := range containerInfo.NetworkSettings.Networks {
		ip = net.IPAddress
		ip6 = net.GlobalIPv6Address
		break
	}
	return ip, ip6, nil
}

// IsRunning returns if the container is running.
func (n *Node) IsRunning() bool {
	return strings.HasPrefix(n.status, "Up")
}

// Delete removes the container.
func (n *Node) Delete(ctx context.Context) error {
	cli, err := GetDockerClient()
	if err != nil {
		return errors.WithStack(err)
	}

	return cli.ContainerRemove(ctx, n.Name, dockerTypes.ContainerRemoveOptions{
		Force:         true, // force the container to be delete now
		RemoveVolumes: true, // delete volumes
	})
}

// WriteFile puts a file inside a running container.
func (n *Node) WriteFile(ctx context.Context, dest, content string) error {
	// create destination directory
	cmd := n.Commander.Command("mkdir", "-p", filepath.Dir(dest))
	if err := cmd.Run(ctx); err != nil {
		return errors.WithStack(err)
	}

	command := n.Commander.Command("cp", "/dev/stdin", dest)
	command.SetStdin(strings.NewReader(content))
	return command.Run(ctx)
}

// Kill sends the named signal to the container.
func (n *Node) Kill(ctx context.Context, signal string) error {
	cli, err := GetDockerClient()
	if err != nil {
		return errors.WithStack(err)
	}

	return cli.ContainerKill(ctx, n.Name, signal)
}

type containerCmder struct {
	nameOrID string
}

func ContainerCmder(containerNameOrID string) *containerCmder {
	return &containerCmder{
		nameOrID: containerNameOrID,
	}
}

func (c *containerCmder) Command(command string, args ...string) *containerCmd {
	return &containerCmd{
		nameOrID: c.nameOrID,
		command:  command,
		args:     args,
	}
}

// containerCmd implements exec.Cmd for docker containers.
type containerCmd struct {
	nameOrID string // the container name or ID
	command  string
	args     []string
	env      []string
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
}

// RunLoggingOutputOnFail runs the cmd, logging error output if Run returns an error.
func (c *containerCmd) RunLoggingOutputOnFail(ctx context.Context) ([]string, error) {
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

func (c *containerCmd) Run(ctx context.Context) error {
	cli, err := GetDockerClient()
	if err != nil {
		return errors.WithStack(err)
	}

	execConfig := &dockerTypes.ExecConfig{
		// run with privileges so we can remount etc..
		// this might not make sense in the most general sense, but it is
		// important to many kind commands
		Privileged:   true,
		Cmd:          append([]string{c.command}, c.args...), // with the command specified
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  c.stdin != nil, // interactive so we can supply input
	}

	// set env
	execConfig.Env = append(execConfig.Env, c.env...)

	response, err := cli.ContainerExecCreate(ctx, c.nameOrID, *execConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	execID := response.ID
	if execID == "" {
		return errors.New("exec ID empty")
	}

	resp, err := cli.ContainerExecAttach(ctx, execID, dockerTypes.ExecStartCheck{})
	if err != nil {
		return errors.WithStack(err)
	}
	defer resp.Close()

	// If there is input, send it through to its stdin
	inputDone := make(chan struct{})
	go func() {
		if c.stdin != nil {
			_, _ = io.Copy(resp.Conn, c.stdin)
			_ = resp.CloseWrite()
		}
		close(inputDone)
	}()

	// Read out any output from the call
	outputDone := make(chan error)
	go func() {
		if c.stdout != nil && c.stderr != nil {
			// StdCopy demultiplexes the stream into two buffers
			_, err = stdcopy.StdCopy(c.stdout, c.stderr, resp.Reader)
			outputDone <- err
		}
		close(outputDone)
	}()

	select {
	case err := <-outputDone:
		if err != nil {
			return errors.WithStack(err)
		}
		break

	case <-ctx.Done():
		return errors.WithStack(ctx.Err())
	}

	inspect, err := cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		return errors.WithStack(err)
	}
	status := inspect.ExitCode
	if status != 0 {
		return errors.New(fmt.Sprintf("exited with status: %d", status))
	}
	return nil
}

func (c *containerCmd) SetEnv(env ...string) {
	c.env = env
}

func (c *containerCmd) SetStdin(r io.Reader) {
	c.stdin = r
}

func (c *containerCmd) SetStdout(w io.Writer) {
	c.stdout = w
}

func (c *containerCmd) SetStderr(w io.Writer) {
	c.stderr = w
}
