/*
Copyright 2025 The Kubernetes Authors.

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

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

var baseArgs = []string{"clusterctl", "foo"}

const (
	forkEnvVar          = "CLUSTERCTL_PLUGIN_TEST_FORK"
	pluginCommandEnvVar = "CLUSTERCTL_PLUGIN_COMMAND"
)

func Test_plugin(t *testing.T) {
	// If CLUSTERCTL_PLUGIN_TEST_FORK is set then just execute the command.
	if os.Getenv(forkEnvVar) != "" {
		os.Args = append(baseArgs, os.Getenv(pluginCommandEnvVar))
		Execute()
		return
	}
	g := NewWithT(t)
	tmpDir := t.TempDir()

	// Create a bash based plugin used for the test which gets added to the PATH variable.
	pluginPath := filepath.Join(tmpDir, "clusterctl-foo")
	pathVar := os.Getenv("PATH")
	g.Expect(os.WriteFile(pluginPath, []byte(pluginCode), 0755)).To(Succeed()) //nolint:gosec

	tt := []struct {
		name     string
		command  string
		expected string
	}{
		{
			name:     "base plugin command test",
			expected: "I am a plugin named clusterctl-foo",
		},
		{
			name:     "plugin version command test",
			command:  "version",
			expected: "1.0.0",
		},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			stdout, _, err := runForkTest(t.Name(), fmt.Sprintf("%s=%s", pluginCommandEnvVar, tc.command), fmt.Sprintf("PATH=%s:%s", tmpDir, pathVar))
			g.Expect(err).To(Succeed())
			g.Expect(stdout).To(ContainSubstring(tc.expected))
		})
	}
}

var pluginCode = `#!/bin/bash

# optional argument handling
if [[ "$1" == "version" ]]
then
echo "1.0.0"
exit 0
fi

echo "I am a plugin named clusterctl-foo"
`

func runForkTest(testName string, options ...string) (string, string, error) {
	cmd := exec.Command(os.Args[0], "-test.run", testName) //nolint:gosec
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%v", forkEnvVar, true))
	cmd.Env = append(cmd.Env, options...)

	var stdoutB, stderrB bytes.Buffer
	cmd.Stdout = &stdoutB
	cmd.Stderr = &stderrB

	err := cmd.Run()

	return stdoutB.String(), stderrB.String(), err
}
