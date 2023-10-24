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
	"testing"

	. "github.com/onsi/gomega"
)

func TestReleaseNotes(t *testing.T) {
	g := NewWithT(t)
	folder := os.Getenv("NOTES_TEST_FOLDER")
	goldenFile := os.Getenv("NOTES_TEST_GOLDEN_FILE")
	prevTag := os.Getenv("NOTES_TEST_PREVIOUS_RELEASE_TAG")

	g.Expect(folder).ToNot(BeEmpty())
	g.Expect(goldenFile).ToNot(BeEmpty())
	g.Expect(prevTag).ToNot(BeEmpty())

	t.Logf("Running release notes for prev tag %s", prevTag)

	expectedOutput, err := os.ReadFile(goldenFile)
	g.Expect(err).ToNot(HaveOccurred())

	// two workers is slow but is guarantees no rate limiting
	os.Args = []string{os.Args[0], "--from", prevTag, "--workers", "2"}
	g.Expect(os.Chdir(folder)).To(Succeed())

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
}

func runReleaseNotesCmd() error {
	// we replicate the main function here so we don't get os.Exit
	flag.Parse()
	if code := run(); code != 0 {
		return fmt.Errorf("release notes command exited with code %d", code)
	}

	return nil
}
