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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"
)

// Cmd
type Cmd struct {
	Cmd   string
	Args  []string
	Stdin string
}

// UnmarshalJSON a runcmd command
// It can be either a list or a string.
// If the item is a list, the head of the list is the command and the tail are the args.
// If the item is a string, the whole command will be wrapped in `/bin/sh -c`.
func (c *Cmd) UnmarshalJSON(data []byte) error {
	// First, try to decode the input as a list
	var s1 []string
	if err := json.Unmarshal(data, &s1); err != nil {
		if _, ok := err.(*json.UnmarshalTypeError); !ok {
			return errors.WithStack(err)
		}
	} else {
		c.Cmd = s1[0]
		c.Args = s1[1:]
		return nil
	}

	// If it's not a list, it must be a string
	var s2 string
	if err := json.Unmarshal(data, &s2); err != nil {
		return errors.WithStack(err)
	}

	c.Cmd = "/bin/sh"
	c.Args = []string{"-c", s2}

	return nil
}

// runCmd defines parameters of a shell command that is equivalent to an action found in the cloud init rundcmd module.
type runCmd struct {
	Cmds []Cmd `json:"runcmd,"`
}

func newRunCmdAction() action {
	return &runCmd{}
}

// Unmarshal the runCmd
func (a *runCmd) Unmarshal(userData []byte) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(err, "error parsing run_cmd action: %s", userData)
	}
	return nil
}

// Commands returns a list of commands to run on the node
func (a *runCmd) Commands() ([]Cmd, error) {
	cmds := make([]Cmd, 0)
	for _, c := range a.Cmds {
		// kubeadm in docker requires to ignore some errors, and this requires to modify the cmd generate by CABPK by default...
		c = hackKubeadmIgnoreErrors(c)
		cmds = append(cmds, c)
	}
	return cmds, nil
}

func hackKubeadmIgnoreErrors(c Cmd) Cmd {
	// case kubeadm commands are defined as a string
	if c.Cmd == "/bin/sh" && len(c.Args) >= 2 {
		if c.Args[0] == "-c" && (strings.Contains(c.Args[1], "kubeadm init") || strings.Contains(c.Args[1], "kubeadm join")) {
			c.Args[1] = fmt.Sprintf("%s %s", c.Args[1], "--ignore-preflight-errors=all")
		}
	}

	// case kubeadm commands are defined as a list
	if c.Cmd == "kubeadm" && len(c.Args) >= 1 {
		if c.Args[0] == "init" || c.Args[0] == "join" {
			c.Args = append(c.Args, "--ignore-preflight-errors=all")
		}
	}

	return c
}
