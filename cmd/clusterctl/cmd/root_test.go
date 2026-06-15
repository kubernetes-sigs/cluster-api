/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"

	. "github.com/onsi/gomega"
)

func TestExecuteRoot_PrintsHelpForArgumentValidationErrors(t *testing.T) {
	g := NewWithT(t)

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	RootCmd.SetOut(stdout)
	RootCmd.SetErr(stderr)
	RootCmd.SetArgs([]string{"describe", "cluster"})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
	})

	err := executeRoot(RootCmd)
	g.Expect(err).To(HaveOccurred())
	g.Expect(stderr.String()).To(ContainSubstring("Error: please specify a cluster name"))
	g.Expect(stdout.String()).To(ContainSubstring("Usage:"))
	g.Expect(stdout.String()).To(ContainSubstring("clusterctl describe cluster NAME [flags]"))
}

func TestExecuteRoot_DoesNotPrintHelpForRuntimeErrors(t *testing.T) {
	g := NewWithT(t)

	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	RootCmd.SetOut(stdout)
	RootCmd.SetErr(stderr)
	RootCmd.SetArgs([]string{"convert", "/tmp/clusterctl-does-not-exist.yaml"})
	t.Cleanup(func() {
		RootCmd.SetArgs(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
	})

	err := executeRoot(RootCmd)
	g.Expect(err).To(HaveOccurred())
	g.Expect(stderr.String()).To(ContainSubstring("failed to read input file"))
	g.Expect(stdout.String()).To(BeEmpty())
}
