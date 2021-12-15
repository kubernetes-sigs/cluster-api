/*
Copyright 2020 The Kubernetes Authors.

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

package framework

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"

	. "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
)

// GatherJUnitReports will move JUnit files from one directory to another,
// renaming them in a format expected by Prow.
func GatherJUnitReports(srcDir string, destDir string) error {
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		return err
	}

	return filepath.Walk(srcDir, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() && p != srcDir {
			return filepath.SkipDir
		}
		if filepath.Ext(p) != ".xml" {
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, "junit") {
			newName := strings.ReplaceAll(base, "_", ".")
			if err := os.Rename(p, path.Join(destDir, newName)); err != nil {
				return err
			}
		}

		return nil
	})
}

// ResolveArtifactsDirectory attempts to resolve a directory to store test
// outputs, using either that provided by Prow, or defaulting to _artifacts.
func ResolveArtifactsDirectory(input string) string {
	if input != "" {
		return input
	}
	if dir, ok := os.LookupEnv("ARTIFACTS"); ok {
		return dir
	}

	findRootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := findRootCmd.Output()
	if err != nil {
		return "_artifacts"
	}
	rootDir := strings.TrimSpace(string(out))

	return path.Join(rootDir, "_artifacts")
}

// CreateJUnitReporterForProw sets up Ginkgo to create JUnit outputs compatible
// with Prow.
func CreateJUnitReporterForProw(artifactsDirectory string) *reporters.JUnitReporter {
	junitPath := filepath.Join(artifactsDirectory, fmt.Sprintf("junit.e2e_suite.%d.xml", config.GinkgoConfig.ParallelNode))

	return reporters.NewJUnitReporter(junitPath)
}

// CompleteCommand prints a command before running it. Acts as a helper function.
// privateArgs when true will not print arguments.
func CompleteCommand(cmd *exec.Cmd, desc string, privateArgs bool) *exec.Cmd {
	cmd.Stderr = TestOutput
	cmd.Stdout = TestOutput
	if privateArgs {
		Byf("%s: dir=%s, command=%s", desc, cmd.Dir, cmd)
	} else {
		Byf("%s: dir=%s, command=%s", desc, cmd.Dir, cmd.String())
	}

	return cmd
}
