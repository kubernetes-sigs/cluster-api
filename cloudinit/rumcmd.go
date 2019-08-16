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

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
)

// Cmd defines a runcmd command
type Cmd struct {
	Cmd  string
	Args []string
}

// UnmarshalJSON a runcmd command
// It can be either a list or a string. If the item is a
// list, it will be properly executed (with the first arg as the command).
// If the item is a string, it will be written to a file and interpreted using ``sh``.
func (c *Cmd) UnmarshalJSON(data []byte) error {
	// try to decode the command into a list
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

	// if decode into a list didn't worked,
	// try to decode the command into a string
	var s2 string
	if err := json.Unmarshal(data, &s2); err != nil {
		return errors.WithStack(err)
	}
	c.Cmd = "/bin/sh"
	c.Args = []string{"-c", s2}

	return nil
}

// runCmdAction defines a cloud init action that replicates the behavior of the cloud init rundcmd module
type runCmdAction struct {
	Cmds []Cmd `json:"runcmd,"`
}

var _ cloudCongfigAction = &runCmdAction{}

func newRunCmdAction() cloudCongfigAction {
	return &runCmdAction{}
}

// Unmarshal the runCmdAction
func (a *runCmdAction) Unmarshal(userData []byte) error {
	if err := yaml.Unmarshal(userData, a); err != nil {
		return errors.Wrapf(errors.WithStack(err), "error parsing write_files action: %s", userData)
	}
	return nil
}

const prompt = "capd@docker$"
const errorPrefix = "ERROR!"

// Run the runCmdAction
func (a *runCmdAction) Run(cmder exec.Cmder) ([]string, error) {
	var lines []string
	for _, c := range a.Cmds {
		// Add a line in the output that mimics the command being issues at the command line
		lines = append(lines, fmt.Sprintf("%s %s %s", prompt, c.Cmd, strings.Join(c.Args, " ")))

		// Run the command
		cmd := cmder.Command(c.Cmd, c.Args...)
		cmdLines, err := exec.CombinedOutputLines(cmd)

		// Add The output lines received
		lines = append(lines, cmdLines...)

		// If the command failed
		if err != nil {
			// Add a line in the output with the error message and exit
			lines = append(lines, fmt.Sprintf("%s %v", errorPrefix, err))
			return lines, errors.Wrapf(errors.WithStack(err), "error running %+v", c)
		}
	}
	return lines, nil
}
