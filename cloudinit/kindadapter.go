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

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"sigs.k8s.io/kind/pkg/exec"
)

var supportedCloudCongfigActions = map[string]cloudCongfigActionBuilder{
	"write_files": newWriteFilesAction,
	"runcmd":      newRunCmdAction,
}

type cloudCongfigActionBuilder func() cloudCongfigAction

type cloudCongfigAction interface {
	Unmarshal(userData []byte) error
	Run(cmder exec.Cmder) ([]string, error)
}

func cloudCongfigActionFactory(actionName string) (cloudCongfigAction, error) {
	if actionBuilder, ok := supportedCloudCongfigActions[actionName]; ok {
		action := actionBuilder()
		return action, nil
	}

	return nil, errors.Errorf("cloud config module %q is not supported", actionName)
}

// Run the given userData (a cloud config script) on the given node
func Run(userData []byte, cmder exec.Cmder) ([]string, error) {
	// validate userData is a valid yaml, as required by the cloud config specification
	var m map[string]interface{}
	if err := yaml.Unmarshal(userData, &m); err != nil {
		return nil, errors.Wrapf(errors.WithStack(err), "userData should be a valid yaml, as required by the cloud config specification")
	}

	// parse the cloud config yaml into a slice of cloud config actions.
	actions, err := getCloudCongfigActionSlices(userData)
	if err != nil {
		return nil, err
	}

	// executes all the actions in order
	var lines []string
	for _, a := range actions {
		actionLines, err := a.Run(cmder)
		if err != nil {
			return actionLines, err
		}
		lines = append(lines, actionLines...)
	}

	return lines, nil
}

// getCloudCongfigActionSlices parse the cloud config yaml into a slice of cloud config actions.
// NB. it is necessary to parse manually because a cloud config is a map of unstructured elements;
// using a standard yaml parser and make it UnMarshal to map[string]interface{} can't work because
// [1] map does not guarantee the order of items, while the adapter should preserve the order of actions
// [2] map does not allow to repeat the same action module in two point of the cloud init sequence, while
// this might happen in real cloud inits
func getCloudCongfigActionSlices(userData []byte) ([]cloudCongfigAction, error) {
	var cloudConfigActionRegEx = regexp.MustCompile(`^[a-zA-Z_]*:`)
	var err error
	var lines []string
	var action cloudCongfigAction
	var actionSlice []cloudCongfigAction

	// scans the file searching for keys/top level actions.
	scanner := bufio.NewScanner(bytes.NewReader(userData))
	for scanner.Scan() {
		line := scanner.Text()
		// if the line is key/top level action
		if cloudConfigActionRegEx.MatchString(line) {
			// converts the file fragment scanned up to now into the current action, if any
			if action != nil {
				actionBlock := strings.Join(lines, "\n")
				if err := action.Unmarshal([]byte(actionBlock)); err != nil {
					return nil, errors.WithStack(err)
				}
				actionSlice = append(actionSlice, action)
				lines = lines[:0]
			}

			// creates the new action
			actionName := strings.TrimSuffix(line, ":")
			action, err = cloudCongfigActionFactory(actionName)
			if err != nil {
				return nil, err
			}
		}

		lines = append(lines, line)
	}

	// converts the last file fragment scanned into the current action, if any
	if action != nil {
		actionBlock := strings.Join(lines, "\n")
		if err := action.Unmarshal([]byte(actionBlock)); err != nil {
			return nil, errors.WithStack(err)
		}
		actionSlice = append(actionSlice, action)
	}

	return actionSlice, scanner.Err()
}
