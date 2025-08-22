//go:build tools && integration

/*
Copyright 2023 The Kubernetes Authors.

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

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/gomega"
)

func TestReleaseNotesIntegration(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			// This tests a patch release computing the PR list from previous patch tag
			// to HEAD. Since v1.3 is out of support, we won't be backporting new PRs
			// so the branch should remain untouched and the test valid.
			name:     "new patch",
			args:     []string{"--release", "v1.3.10"},
			expected: "test/golden/v1.3.10.md",
		},
		{
			// By default when using the `--release` options, notes command computes
			// everything from last tag to HEAD. Hence if we use the head of release-1.5,
			// this test will become invalid every time we backport some PR to release
			// branch release-1.5.
			// Instead, to simulate the v1.5.0 release, we manually set the `--to` flag,
			// which sets the upper boundary for the PR search.
			name:     "new minor",
			args:     []string{"--from", "tags/v1.4.0", "--to", "tags/v1.5.0", "--branch", "release-1.5"},
			expected: "test/golden/v1.5.0.md",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			expectedOutput, err := os.ReadFile(tc.expected)
			g.Expect(err).ToNot(HaveOccurred())

			orgCurrentDir, err := os.Getwd()
			g.Expect(err).To(Succeed())
			t.Cleanup(func() {
				g.Expect(os.Chdir(orgCurrentDir)).To(Succeed())
			})

			// a two workers config is slow but it guarantees no rate limiting
			os.Args = append([]string{os.Args[0]}, tc.args...)

			old := os.Stdout // keep backup of the real stdout to restore later
			r, w, err := os.Pipe()
			g.Expect(err).To(Succeed())
			os.Stdout = w

			t.Cleanup(func() {
				// Reset defined flags so we can can call cmd.run() again
				flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			})

			g.Expect(runReleaseNotesCmd()).To(Succeed())

			w.Close()
			output, err := io.ReadAll(r)
			g.Expect(err).NotTo(HaveOccurred())
			os.Stdout = old

			g.Expect(string(output)).To(BeComparableTo(string(expectedOutput)))
		})
	}
}

func runReleaseNotesCmd() error {
	cmd := newNotesCmd()
	if err := cmd.run(); err != nil {
		return fmt.Errorf("release notes command failed: %w", err)
	}

	return nil
}

func runCommand(tb testing.TB, cmd *exec.Cmd) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("Command %s failed: %s", cmd.Args, string(out))
	}
}
