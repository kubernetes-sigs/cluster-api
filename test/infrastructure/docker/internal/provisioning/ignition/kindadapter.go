/*
Copyright 2021 The Kubernetes Authors.

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

package ignition

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	ignitionTypes "github.com/flatcar/ignition/config/v2_3/types"
	"github.com/pkg/errors"
	"github.com/vincent-petithory/dataurl"
	"sigs.k8s.io/yaml"

	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/provisioning"
)

// RawIgnitionToProvisioningCommands converts an Ignition YAML document to a slice of commands.
func RawIgnitionToProvisioningCommands(config []byte) ([]provisioning.Cmd, error) {
	// Ensure Ignition is a valid YAML document.
	if err := yaml.Unmarshal(config, &map[string]interface{}{}); err != nil {
		return nil, errors.Wrapf(err, "invalid YAML")
	}

	// Parse the Ignition YAML into a slice of Ignition actions.
	actions, err := getActions(config)
	if err != nil {
		return nil, err
	}

	return actions, nil
}

// getActions parses the cloud config YAML into a slice of actions to run.
// Parsing manually is required because the order of the cloud config's actions must be maintained.
func getActions(userData []byte) ([]provisioning.Cmd, error) {
	var commands []provisioning.Cmd

	var ignition ignitionTypes.Config
	if err := json.Unmarshal(userData, &ignition); err != nil {
		return nil, fmt.Errorf("unmarshalling Ignition JSON: %w", err)
	}

	// Generate commands for files.
	for _, f := range ignition.Storage.Files {
		raw := strings.TrimSpace(f.Contents.Source)
		contents, err := decodeFileContents(raw)
		if err != nil {
			return nil, fmt.Errorf("decoding file contents: %w", err)
		}

		mode := strconv.FormatInt(int64(*f.Mode), 8)
		if len(mode) == 3 {
			// Sticky bit isn't specified - pad with a zero.
			mode = "0" + mode
		}

		if f.Path == "/etc/kubeadm.sh" {
			contents = hackKubeadmIgnoreErrors(contents)
		}

		commands = append(commands, []provisioning.Cmd{
			// Idempotently create the directory.
			{Cmd: "mkdir", Args: []string{"-p", filepath.Dir(f.Path)}},
			// Write the file.
			{Cmd: "/bin/sh", Args: []string{"-c", fmt.Sprintf("cat > %s /dev/stdin", f.Path)}, Stdin: contents},
			// Set file permissions.
			{Cmd: "chmod", Args: []string{mode, f.Path}},
		}...)
	}

	for _, u := range ignition.Systemd.Units {
		contents := strings.TrimSpace(u.Contents)
		path := fmt.Sprintf("/etc/systemd/system/%s", u.Name)

		commands = append(commands, []provisioning.Cmd{
			{Cmd: "/bin/sh", Args: []string{"-c", fmt.Sprintf("cat > %s /dev/stdin", path)}, Stdin: contents},
			{Cmd: "systemctl", Args: []string{"daemon-reload"}},
		}...)

		if u.Enable || (u.Enabled != nil && *u.Enabled) {
			commands = append(commands, provisioning.Cmd{Cmd: "systemctl", Args: []string{"enable", "--now", u.Name}})
		}
	}

	return commands, nil
}

// Add `--ignore-preflight-errors=all` to `kubeadm init` and `kubeadm join`.
func hackKubeadmIgnoreErrors(s string) string {
	lines := strings.Split(s, "\n")

	for idx, line := range lines {
		if !(strings.Contains(line, "kubeadm init") || strings.Contains(line, "kubeadm join")) {
			continue
		}

		lines[idx] = line + " --ignore-preflight-errors=all"
	}

	return strings.Join(lines, "\n")
}

// decodeFileContents accepts a string representing the contents of a file encoded in Ignition
// format and returns a decoded version of the string.
func decodeFileContents(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}

	if u.Scheme != "data" {
		return s, nil
	}

	rendered, err := dataurl.DecodeString(s)
	if err != nil {
		return "", err
	}

	return string(rendered.Data), nil
}
