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
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
	"sigs.k8s.io/cluster-api/test/infrastructure/kind"
)

const (
	// Supported cloud config modules.
	writefiles = "write_files"
	runcmd     = "runcmd"
)

type actionFactory struct{}

func (a *actionFactory) action(name string) action {
	switch name {
	case writefiles:
		return newWriteFilesAction()
	case runcmd:
		return newRunCmdAction()
	default:
		// TODO Add a logger during the refactor and log this unknown module
		return newUnknown(name)
	}
}

type action interface {
	Unmarshal(userData []byte, mapping kind.Mapping) error
	Commands() ([]provisioning.Cmd, error)
}

// RawCloudInitToProvisioningCommands converts a cloudconfig to a list of commands to run in sequence on the node.
func RawCloudInitToProvisioningCommands(config []byte, mapping kind.Mapping) ([]provisioning.Cmd, error) {
	// validate cloudConfigScript is a valid yaml, as required by the cloud config specification
	if err := yaml.Unmarshal(config, &map[string]interface{}{}); err != nil {
		return nil, errors.Wrapf(err, "cloud-config is not valid yaml")
	}

	// parse the cloud config yaml into a slice of cloud config actions.
	actions, err := getActions(config, mapping)
	if err != nil {
		return nil, err
	}

	commands := []provisioning.Cmd{}
	for _, action := range actions {
		cmds, err := action.Commands()
		if err != nil {
			return commands, err
		}
		commands = append(commands, cmds...)
	}

	return commands, nil
}

// getActions parses the cloud config yaml into a slice of actions to run.
// Parsing manually is required because the order of the cloud config's actions must be maintained.
func getActions(userData []byte, mapping kind.Mapping) ([]action, error) {
	actionRegEx := regexp.MustCompile(`^[a-zA-Z_]*:`)
	lines := make([]string, 0)
	actions := make([]action, 0)
	actionFactory := &actionFactory{}

	var act action

	// scans the file searching for keys/top level actions.
	scanner := bufio.NewScanner(bytes.NewReader(userData))
	for scanner.Scan() {
		line := scanner.Text()
		// if the line is key/top level action
		if actionRegEx.MatchString(line) {
			// converts the file fragment scanned up to now into the current action, if any
			if act != nil {
				actionBlock := strings.Join(lines, "\n")
				if err := act.Unmarshal([]byte(actionBlock), mapping); err != nil {
					return nil, errors.WithStack(err)
				}
				actions = append(actions, act)
				lines = lines[:0]
			}

			// creates the new action
			actionName := strings.TrimSuffix(line, ":")
			act = actionFactory.action(actionName)
		}

		lines = append(lines, line)
	}

	// converts the last file fragment scanned into the current action, if any
	if act != nil {
		actionBlock := strings.Join(lines, "\n")
		if err := act.Unmarshal([]byte(actionBlock), mapping); err != nil {
			return nil, errors.WithStack(err)
		}
		actions = append(actions, act)
	}

	return actions, scanner.Err()
}
