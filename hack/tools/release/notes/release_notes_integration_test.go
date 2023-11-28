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
	"k8s.io/kubectl/pkg/cmd"
)

func TestReleaseNotesIntegration(t *testing.T) {
	testCases := []struct {
		name                 string
		previousRelease      string
		releaseBranchForTest string
		expected             string
	}{
		{
			// This tests a patch release computing the PR list from previous patch tag
			// to HEAD. Since v1.3 is out of support, we won't be backporting new PRs
			// so the branch should remain untouched and the test valid.
			name:                 "new patch",
			previousRelease:      "v1.3.9",
			releaseBranchForTest: "release-1.3",
			expected:             "test/golden/v1.3.10.md",
		},
		{
			// The release notes command computes everything from last tag
			// to HEAD. Hence if we use the head of release-1.5, this test will
			// become invalid everytime we backport some PR to release branch release-1.5.
			// Here we cheat a little by poiting to worktree to v1.5.0, which is the
			// release that this test is simulating, so it should not exist yet. But
			// it represents accurately the HEAD of release-1.5 when we released v1.5.0.
			name:                 "new minor",
			previousRelease:      "v1.4.0",
			releaseBranchForTest: "v1.5.0",
			expected:             "test/golden/v1.5.0.md",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setupNotesTest(t, tc.previousRelease, tc.releaseBranchForTest)
			g := NewWithT(t)

			expectedOutput, err := os.ReadFile(tc.expected)
			g.Expect(err).ToNot(HaveOccurred())

			orgCurrentDir, err := os.Getwd()
			g.Expect(err).To(Succeed())
			t.Cleanup(func() {
				g.Expect(os.Chdir(orgCurrentDir)).To(Succeed())
			})
			g.Expect(os.Chdir(tc.releaseBranchForTest)).To(Succeed())

			// a two workers config is slow but it guarantees no rate limiting
			os.Args = []string{os.Args[0], "--from", tc.previousRelease, "--workers", "2"}

			old := os.Stdout // keep backup of the real stdout to restore later
			r, w, err := os.Pipe()
			g.Expect(err).To(Succeed())
			os.Stdout = w

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
	// we replicate the main function here so we don't get os.Exit
	flag.Parse()
	if code := run(); code != 0 {
		return fmt.Errorf("release notes command exited with code %d", code)
	}

	return nil
}

func setupNotesTest(tb testing.TB, previousRelease, releaseBranchForTest string) {
	g := NewWithT(tb)

	_, err := os.Stat(releaseBranchForTest)
	if os.IsNotExist(err) {
		runCommand(tb, exec.Command("git", "worktree", "add", releaseBranchForTest))
	} else {
		g.Expect(err).To(Succeed())
	}

	pull := exec.Command("git", "pull", "upstream", releaseBranchForTest)
	pull.Dir = releaseBranchForTest
	runCommand(tb, pull)

	tb.Cleanup(func() {
		runCommand(tb, cmd.Command("git", "worktree", "remove", releaseBranchForTest))
	})
}

func runCommand(tb testing.TB, cmd *exec.Cmd) {
	out, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("Command %s failed: %s", cmd.Args, string(out))
	}
}
