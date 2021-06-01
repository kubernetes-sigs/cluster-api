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

package util

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// Cmd implements a wrapper on os/exec.cmd.
type Cmd struct {
	command string
	args    []string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

// NewCmd returns a new Cmd with the given arguments.
func NewCmd(command string, args ...string) *Cmd {
	return &Cmd{
		command: command,
		args:    args,
	}
}

// Run runs the command.
func (c *Cmd) Run() error {
	return c.runInnerCommand()
}

// RunWithEcho runs the command and redirects its output to stdout and stderr.
func (c *Cmd) RunWithEcho() error {
	c.stdout = os.Stderr
	c.stderr = os.Stdout
	return c.runInnerCommand()
}

// RunAndCapture runs the command and captures any output.
func (c *Cmd) RunAndCapture() (lines []string, err error) {
	var buff bytes.Buffer
	c.stdout = &buff
	c.stderr = &buff
	err = c.runInnerCommand()

	scanner := bufio.NewScanner(&buff)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, err
}

// Stdin sets the stdin for the command.
func (c *Cmd) Stdin(in io.Reader) *Cmd {
	c.stdin = in
	return c
}

func (c *Cmd) runInnerCommand() error {
	cmd := exec.Command(c.command, c.args...) //nolint:gosec

	if c.stdin != nil {
		cmd.Stdin = c.stdin
	}
	if c.stdout != nil {
		cmd.Stdout = c.stdout
	}
	var b1 strings.Builder
	cmd.Stderr = &b1
	if c.stderr != nil {
		cmd.Stderr = io.MultiWriter(&b1, c.stderr)
	}

	if err := cmd.Run(); err != nil {
		return errors.Wrapf(err, "failed to run: %s %s\n%s", c.command, strings.Join(c.args, " "), b1.String())
	}

	return nil
}
