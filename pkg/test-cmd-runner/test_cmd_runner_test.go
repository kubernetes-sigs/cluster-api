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

package test_cmd_runner_test

import (
	"fmt"
	"os"
	"sigs.k8s.io/cluster-api/pkg/test-cmd-runner"
	"strings"
	"testing"
)

func init() {
	test_cmd_runner.RegisterCallback(callbackWithArgs)
	test_cmd_runner.RegisterCallback(callbackWithoutArgs)
}

var (
	callbackOutput = "**** this is test output ****"
	commandName    = "my-command"
	argOne         = "arg1"
	argTwo         = "arg2"
	argThree       = "arg3"
	allArgs        = []string{argOne, argTwo, argThree}
)

func TestUnregisteredFunctionShouldError(t *testing.T) {
	_, err := test_cmd_runner.NewTestRunner(unregisteredFunction)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	expectedContains := "RegisterCallback(unregisteredFunction)"
	if !strings.Contains(err.Error(), expectedContains) {
		t.Errorf("expected error to contain '%v', error contents: %v", expectedContains, err)
	}
}

func TestCallbackFunctionShouldExecute(t *testing.T) {
	runner := test_cmd_runner.NewTestRunnerFailOnErr(t, callbackWithArgs)
	output, err := runner.CombinedOutput(commandName, allArgs...)
	if err != nil {
		t.Errorf("invalid error: expected 'nil', got '%v'", err)
	}
	if output != callbackOutput {
		t.Errorf("invalid output: expected '%v' got '%v'", callbackOutput, output)
	}
}

func TestNoArgsShouldNotError(t *testing.T) {
	runner := test_cmd_runner.NewTestRunnerFailOnErr(t, callbackWithoutArgs)
	output, err := runner.CombinedOutput(commandName)
	if err != nil {
		t.Errorf("invalid error: expected 'nil', got '%v'", err)
	}
	if output != callbackOutput {
		t.Errorf("invalid output: expected '%v' got '%v'", callbackOutput, output)
	}
}

func TestMain(m *testing.M) {
	test_cmd_runner.TestMain(m)
}

func callbackWithoutArgs(cmd string, args ...string) int {
	return verifyCmdAndArgsAndPrint(commandName, []string{}, cmd, args...)
}

func callbackWithArgs(cmd string, args ...string) int {
	return verifyCmdAndArgsAndPrint(commandName, allArgs, cmd, args...)
}

func verifyCmdAndArgsAndPrint(expectedCmd string, expectedArgs []string, cmd string, args ...string) (exitCode int) {
	if cmd != expectedCmd {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("incorrect cmd name, expected '%v', got '%v'", commandName, cmd))
		return 1
	}
	if !stringSlicesEqual(args, expectedArgs) {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("incorrect args, expected: '%v', got '%v'", allArgs, args))
		return 1
	}
	fmt.Print(callbackOutput)
	return 0
}

func stringSlicesEqual(s1 []string, s2 []string) bool {
	if s1 == nil && s2 == nil {
		return true
	}
	if s1 == nil && s2 != nil || s1 != nil && s2 == nil {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func unregisteredFunction(cmd string, args ...string) int {
	return 0
}
