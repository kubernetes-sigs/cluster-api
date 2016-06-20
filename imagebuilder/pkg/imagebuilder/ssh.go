/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

// TODO: This probably should be moved to / shared with nodeup, because then it would allow action on a remote target

package imagebuilder

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"

	"github.com/golang/glog"
)

// NewSSH constructs a new SSH helper
func NewSSH(sshClient *ssh.Client) *SSH {
	return &SSH{
		sshClient: sshClient,
	}
}

// SSH holds an SSH client, and adds utilities like SCP functionality
type SSH struct {
	sshClient *ssh.Client
}

// SCPMkdir executes a mkdir against the SSH target, using SCP
func (s *SSH) SCPMkdir(dest string, mode os.FileMode) error {
	glog.Infof("Doing SSH SCP mkdir: %q", dest)
	session, err := s.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("error establishing SSH session: %v", err)
	}
	defer session.Close()

	name := filepath.Base(dest)
	scpBase := filepath.Dir(dest)
	//scpBase = "." + scpBase

	var stdinErr error
	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		_, stdinErr = fmt.Fprintln(w, "D0"+toOctal(mode), 0, name)
		if stdinErr != nil {
			return
		}
	}()
	output, err := session.CombinedOutput("/usr/bin/scp -tr " + scpBase)
	if err != nil {
		glog.Warningf("Error output from SCP: %s", output)
		return fmt.Errorf("error doing SCP mkdir: %v", err)
	}
	if stdinErr != nil {
		glog.Warningf("Error output from SCP: %s", output)
		return fmt.Errorf("error doing SCP mkdir (writing to stdin): %v", stdinErr)
	}

	return nil
}

func toOctal(mode os.FileMode) string {
	return strconv.FormatUint(uint64(mode), 8)
}

// SCPPut copies a file to the SSH target, using SCP
func (s *SSH) SCPPut(dest string, length int, content io.Reader, mode os.FileMode) error {
	glog.Infof("Doing SSH SCP upload: %q", dest)
	session, err := s.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("error establishing SSH session: %v", err)
	}
	defer session.Close()

	name := filepath.Base(dest)
	scpBase := filepath.Dir(dest)
	//scpBase = "." + scpBase

	var stdinErr error
	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		_, stdinErr = fmt.Fprintln(w, "C0"+toOctal(mode), length, name)
		if stdinErr != nil {
			return
		}
		_, stdinErr = io.Copy(w, content)
		if stdinErr != nil {
			return
		}
		_, stdinErr = fmt.Fprint(w, "\x00")
		if stdinErr != nil {
			return
		}
	}()
	output, err := session.CombinedOutput("/usr/bin/scp -tr " + scpBase)
	if err != nil {
		glog.Warningf("Error output from SCP: %s", output)
		return fmt.Errorf("error doing SCP put: %v", err)
	}
	if stdinErr != nil {
		glog.Warningf("Error output from SCP: %s", output)
		return fmt.Errorf("error doing SCP put (writing to stdin): %v", stdinErr)
	}

	return nil
}

// CommandExecution helps us build a command for running
type CommandExecution struct {
	Command string
	Cwd     string
	Env     map[string]string
	Sudo    bool
	ssh     *SSH
}

// WithSudo indicates that the command should be executed with sudo
func (c *CommandExecution) WithSudo() *CommandExecution {
	c.Sudo = true
	return c
}

// WithCwd sets the directory in which the command will execute
func (c *CommandExecution) WithCwd(cwd string) *CommandExecution {
	c.Cwd = cwd
	return c
}

// Setenv sets an environment variable for the command execution
func (c *CommandExecution) Setenv(k, v string) *CommandExecution {
	c.Env[k] = v
	return c
}

// Run executes the command
func (c *CommandExecution) Run() error {
	return c.ssh.run(c)
}

// Command builds a CommandExecution bound to the current SSH target
func (s *SSH) Command(cmd string) *CommandExecution {
	c := &CommandExecution{
		ssh:     s,
		Command: cmd,
		Env:     make(map[string]string),
	}
	return c
}

// Exec executes a command against the SSH target
func (s *SSH) Exec(cmd string) error {
	c := s.Command(cmd)
	return c.Run()
}

// TODO: I bet we could refactor this nicely using a fluent calling style...
func (s *SSH) run(cmd *CommandExecution) error {
	// Warn if the caller is doing something dumb
	if cmd.Sudo && strings.HasPrefix(cmd.Command, "sudo ") {
		glog.Warningf("sudo used with command that includes sudo (%q)", cmd.Command)
	}

	var script bytes.Buffer

	needScript := false

	script.WriteString("#!/bin/bash -e\n")
	if cmd.Cwd != "" {
		script.WriteString("cd " + cmd.Cwd + "\n")
		needScript = true
	}
	if cmd.Env != nil && len(cmd.Env) != 0 {
		// Most SSH servers are configured not to accept arbitrary env vars
		for k, v := range cmd.Env {
			/*			err := session.Setenv(k, v)
						if err != nil {
							return fmt.Errorf("error setting env var in SSH session: %v", err)
						}
			*/
			script.WriteString("export " + k + "='" + v + "'\n")
			needScript = true
		}
	}
	script.WriteString(cmd.Command + "\n")

	session, err := s.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("error establishing SSH session: %v", err)
	}
	defer session.Close()

	cmdToRun := cmd.Command
	if needScript {
		tmpScript := fmt.Sprintf("/tmp/ssh-exec-%d", rand.Int63())
		scriptBytes := script.Bytes()
		err := s.SCPPut(tmpScript, len(scriptBytes), bytes.NewReader(scriptBytes), 0755)
		if err != nil {
			return fmt.Errorf("error uploading temporary script: %v", err)
		}
		defer s.Exec("rm -f " + tmpScript)
		cmdToRun = tmpScript
		if cmd.Sudo {
			cmdToRun = "sudo " + cmdToRun
		}
	} else {
		cmdToRun = cmd.Command
		if cmd.Sudo {
			cmdToRun = "sudo " + cmdToRun
		}
	}

	// We "lie" about the command we're running when we're using a script
	glog.Infof("Executing SSH command: %q", cmd.Command)
	output, err := session.CombinedOutput(cmdToRun)
	if err != nil {
		glog.Infof("Error from SSH command %q: %v", cmd.Command, err)
		glog.Infof("Output was: %s", output)
		return fmt.Errorf("error executing SSH command %q: %v", cmd.Command, err)
	}

	glog.V(2).Infof("Output was: %s", output)
	return nil
}
