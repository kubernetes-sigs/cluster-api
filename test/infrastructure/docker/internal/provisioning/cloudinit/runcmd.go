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

package cloudinit

import (
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
)

// runCmd defines parameters of a shell command that is equivalent to an action found in the cloud init rundcmd module.
type runCmd struct {
	Cmds []provisioning.Cmd `json:"runcmd,"`
}

func newRunCmdAction() action {
	return &runCmd{}
}

// Unmarshal the runCmd.
func (a *runCmd) Unmarshal(userData []byte) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(err, "error parsing run_cmd action: %s", userData)
	}
	return nil
}

// Commands returns a list of commands to run on the node.
func (a *runCmd) Commands() ([]provisioning.Cmd, error) {
	cmds := make([]provisioning.Cmd, 0)
	for _, c := range a.Cmds {
		// kubeadm in docker requires to ignore some errors, and this requires to modify the cmd generate by CABPK by default...
		c = hackKubeadmIgnoreErrors(c)
		cmds = append(cmds, c)
	}
	return cmds, nil
}

func hackKubeadmIgnoreErrors(c provisioning.Cmd) provisioning.Cmd {
	// case kubeadm commands are defined as a string
	if c.Cmd == "/bin/sh" && len(c.Args) >= 2 {
		if c.Args[0] == "-c" {
			c.Args[1] = strings.Replace(c.Args[1], "kubeadm init", "kubeadm init --ignore-preflight-errors=all", 1)
			c.Args[1] = strings.Replace(c.Args[1], "kubeadm join", "kubeadm join --ignore-preflight-errors=all", 1)
		}
	}

	// case kubeadm commands are defined as a list
	if c.Cmd == "kubeadm" && len(c.Args) >= 1 {
		if c.Args[0] == "init" || c.Args[0] == "join" {
			c.Args = append(c.Args, "")                 // make space
			copy(c.Args[2:], c.Args[1:])                // shift elements
			c.Args[1] = "--ignore-preflight-errors=all" // insert the additional arg
		}
	}

	return c
}
