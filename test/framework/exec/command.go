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

// Package exec implements command execution functionality.
package exec

import (
	"context"
	"io"
	"os/exec"

	"github.com/pkg/errors"
)

// Command wraps exec.Command with specific functionality.
// This differentiates itself from the standard library by always collecting stdout and stderr.
// Command improves the UX of exec.Command for our specific use case.
type Command struct {
	Cmd   string
	Args  []string
	Stdin io.Reader
}

// Option is a functional option type that modifies a Command.
type Option func(*Command)

// NewCommand returns a configured Command.
func NewCommand(opts ...Option) *Command {
	cmd := &Command{
		Stdin: nil,
	}
	for _, option := range opts {
		option(cmd)
	}
	return cmd
}

// WithStdin sets up the command to read from this io.Reader.
func WithStdin(stdin io.Reader) Option {
	return func(cmd *Command) {
		cmd.Stdin = stdin
	}
}

// WithCommand defines the command to run such as `kubectl` or `kind`.
func WithCommand(command string) Option {
	return func(cmd *Command) {
		cmd.Cmd = command
	}
}

// WithArgs sets the arguments for the command such as `get pods -n kube-system` to the command `kubectl`.
func WithArgs(args ...string) Option {
	return func(cmd *Command) {
		cmd.Args = args
	}
}

// Run executes the command and returns stdout, stderr and the error if there is any.
func (c *Command) Run(ctx context.Context) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, c.Cmd, c.Args...) //nolint:gosec
	if c.Stdin != nil {
		cmd.Stdin = c.Stdin
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	output, err := io.ReadAll(stdout)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	errout, err := io.ReadAll(stderr)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if err := cmd.Wait(); err != nil {
		return output, errout, errors.WithStack(err)
	}
	return output, errout, nil
}
