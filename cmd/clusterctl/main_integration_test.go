// +build integration

/*
Copyright 2018 The Kubernetes Authors.

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

package main_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"testing"
)

const (
	clusterctlBinaryName = "clusterctl"
	workingDirectoryName = "clusterctl"
)

// run these tests with the flag "-update" to update the values stored in all of the golden files
var update = flag.Bool("update", false, "update .golden files")

func TestMain(m *testing.M) {
	flag.Parse()
	err := setupPrerequisites()
	if err != nil {
		fmt.Printf("unable to setup prerequisites: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestEmptyAndInvalidArgs(t *testing.T) {
	testCases := []struct {
		name            string
		args            []string
		exitCode        int
		fixtureFilename string
	}{
		{"no arguments", []string{}, 0, "no-args.golden"},
		{"no arguments with invalid flag", []string{"--invalid-flag"}, 1, "no-args-invalid-flag.golden"},
		{"create with no arguments", []string{"create"}, 0, "create-no-args.golden"},
		{"create with no arguments with invalid flag", []string{"create", "--invalid-flag"}, 1, "create-no-args-invalid-flag.golden"},
		{"create cluster with no arguments", []string{"create", "cluster"}, 1, "create-cluster-no-args.golden"},
		{"create cluster with no arguments with invalid flag", []string{"create", "cluster", "--invalid-flag"}, 1, "create-cluster-no-args-invalid-flag.golden"},
		{"delete with no arguments", []string{"delete"}, 0, "delete-no-args.golden"},
		{"delete with no arguments with invalid flag", []string{"delete", "--invalid-flag"}, 1, "delete-no-args-invalid-flag.golden"},
		{"delete cluster with no arguments with invalid flag", []string{"delete", "cluster", "--invalid-flag"}, 1, "delete-cluster-no-args-invalid-flag.golden"},
		{"validate with no arguments", []string{"validate"}, 0, "validate-no-args.golden"},
		{"validate with no arguments with invalid flag", []string{"validate", "--invalid-flag"}, 1, "validate-no-args-invalid-flag.golden"},
		{"validate cluster with no arguments with invalid flag", []string{"validate", "cluster", "--invalid-flag"}, 1, "validate-cluster-no-args-invalid-flag.golden"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := runClusterctl(t, tc.args...)
			compareExitCode(t, err, tc.exitCode)
			compareOutput(t, output, tc.fixtureFilename)
		})
	}
}

func compareExitCode(t *testing.T, err error, expectedExitCode int) {
	if expectedExitCode == 0 {
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	} else {
		actualExitCode := extractExitCode(t, err)
		if actualExitCode != expectedExitCode {
			t.Errorf("unexpected exit code: want %v, got %v", expectedExitCode, actualExitCode)
		}
	}
}

func extractExitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Errorf("expected error exit code, got nil")
	} else if exitError, ok := err.(*exec.ExitError); ok {
		ws := exitError.Sys().(syscall.WaitStatus)
		return ws.ExitStatus()
	} else {
		t.Fatalf("unable to retrieve exit code from error: %v", err)
	}
	return 0
}

func compareOutput(t *testing.T, actualOutput string, goldenFileName string) {
	t.Helper()
	if *update {
		writeFixture(t, goldenFileName, actualOutput)
	}
	expectedOutput := loadFixture(t, goldenFileName)
	if actualOutput != expectedOutput {
		// TODO: format this using a diff tool / library (issue 255)
		t.Errorf("unexpected output, want %v, got %v", expectedOutput, actualOutput)
	}
}

func writeFixture(t *testing.T, fixtureFilename string, value string) {
	t.Helper()
	path := getFixturePath(fixtureFilename)
	err := ioutil.WriteFile(path, []byte(value), 0644)
	if err != nil {
		t.Fatalf("unable to update value of fixture '%v': %v", path, err)
	}
}

func loadFixture(t *testing.T, fixtureFilename string) string {
	t.Helper()
	path := getFixturePath(fixtureFilename)
	output, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("unable to read fixture file '%v': %v", path, err)
	}
	return string(output)
}

func getFixturePath(filename string) string {
	return path.Join("testdata", filename)
}

func setupPrerequisites() error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to retrieve working directory: %v", err)
	}
	_, dir := path.Split(workDir)
	if dir != workingDirectoryName {
		return fmt.Errorf("invalid working directory name: want %v, got %v", workingDirectoryName, dir)
	}
	err = cleanAndBuildClusterctl()
	if err != nil {
		return fmt.Errorf("unable to build clusterctl: %v", err)
	}
	return nil
}

func cleanAndBuildClusterctl() error {
	err := exec.Command("go", "clean", "./...").Run()
	if err != nil {
		return fmt.Errorf("unable to run go clean: %v", err)
	}
	err = exec.Command("go", "build", ".").Run()
	if err != nil {
		return fmt.Errorf("unable to run go build: %v", err)
	}
	return nil
}

func runClusterctl(t *testing.T, args ...string) (string, error) {
	workDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("unable to retrieve working directory: %v", err)
	}
	return runCmd("", path.Join(workDir, clusterctlBinaryName), args...)
}

func runCmd(workDir string, cmd string, args ...string) (string, error) {
	command := exec.Command(cmd, args...)
	command.Dir = workDir
	cmdStr := fmt.Sprintf("%v %v", cmd, strings.Join(args, " "))
	fmt.Printf("Running command: %v\n", cmdStr)
	output, err := command.CombinedOutput()
	return string(output), err
}
