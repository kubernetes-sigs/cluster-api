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
	"strconv"
	"testing"

	"github.com/Masterminds/goutils"
	. "github.com/onsi/gomega"
)

var baseArgs = []string{"clusterctl", "foo"}

const (
	forkEnvVar          = "FORK"
	pluginCommandEnvVar = "PLUGIN_COMMAND"
)

func Test_plugin(t *testing.T) {
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
			stdout, _, err := runForkTest("TestExecutePlugin", fmt.Sprintf("%s=%s", pluginCommandEnvVar, tc.command))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(stdout).To(ContainSubstring(tc.expected))
		})
	}
}

func TestExecutePlugin(t *testing.T) {
	g := NewWithT(t)
	if goutils.DefaultString(os.Getenv(forkEnvVar), "false") == strconv.FormatBool(false) {
		t.Skip("FORK environment variable isn't set. Skipping test")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "clusterctl-foo")
	g.Expect(os.WriteFile(path, []byte(pluginCode), 0755)).To(Succeed()) //nolint:gosec
	g.Expect(os.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"), tmpDir))).ToNot(HaveOccurred())

	os.Args = append(baseArgs, os.Getenv(pluginCommandEnvVar))
	Execute()
}

const pluginCode = `
#!/bin/bash

# optional argument handling
if [[ "$1" == "version" ]]
then
echo "1.0.0"
exit 0
fi

echo "I am a plugin named clusterctl-foo"
`

func runForkTest(testName string, option string) (string, string, error) {
	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%v", testName)) //nolint:gosec
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%v", forkEnvVar, true), option)

	var stdoutB, stderrB bytes.Buffer
	cmd.Stdout = &stdoutB
	cmd.Stderr = &stderrB

	err := cmd.Run()

	return stdoutB.String(), stderrB.String(), err
}
