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

package execer

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// Client gives access to the `kind` program
type Client struct {
	Stdout   io.Writer
	Stderr   io.Writer
	Command  string
	ExtraEnv []string
}

func NewClient(command string) *Client {
	return &Client{
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		Command:  command,
		ExtraEnv: []string{},
	}
}

func (c *Client) PipeToCommand(stdin io.Reader, args ...string) error {
	cmd := exec.Command(c.Command, args...)
	cmd.Env = append(os.Environ(), c.ExtraEnv...)
	in, err := cmd.StdinPipe()
	if err != nil {
		return errors.WithStack(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.WithStack(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.WithStack(err)
	}
	io.Copy(in, stdin)

	if err := cmd.Start(); err != nil {
		return errors.WithStack(err)
	}
	in.Close()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		c.Stdout.Write(scanner.Bytes())
		c.Stdout.Write([]byte("\n"))
	}
	scannerr := bufio.NewScanner(stderr)
	for scannerr.Scan() {
		c.Stderr.Write(scannerr.Bytes())
		c.Stderr.Write([]byte("\n"))
	}

	if err := cmd.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (c *Client) RunCommandReturnOutput(args ...string) (string, error) {
	cmd := exec.Command(c.Command, args...)
	cmd.Env = append(os.Environ(), c.ExtraEnv...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.WithStack(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", errors.WithStack(err)
	}

	if err := cmd.Start(); err != nil {
		return "", errors.WithStack(err)
	}
	var b bytes.Buffer
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		b.Write(scanner.Bytes())
		b.Write([]byte("\n"))
	}
	if err := scanner.Err(); err != nil {
		return "", errors.WithStack(err)
	}

	out := b.String()
	scannerr := bufio.NewScanner(stderr)
	for scannerr.Scan() {
		c.Stderr.Write(scannerr.Bytes())
		c.Stderr.Write([]byte("\n"))
	}
	if err := scannerr.Err(); err != nil {
		return "", errors.WithStack(err)
	}

	if err := cmd.Wait(); err != nil {
		return "", errors.WithStack(err)
	}

	return strings.TrimSpace(out), nil

}

func (c *Client) RunCommand(args ...string) error {
	cmd := exec.Command(c.Command, args...)
	cmd.Env = append(os.Environ(), c.ExtraEnv...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.WithStack(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.WithStack(err)
	}

	if err := cmd.Start(); err != nil {
		return errors.WithStack(err)
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		c.Stdout.Write(scanner.Bytes())
		c.Stdout.Write([]byte("\n"))
	}
	scannerr := bufio.NewScanner(stderr)
	for scannerr.Scan() {
		c.Stderr.Write(scannerr.Bytes())
		c.Stderr.Write([]byte("\n"))
	}

	if err := cmd.Wait(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
