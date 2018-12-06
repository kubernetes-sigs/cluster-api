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

package testcmdrunner

import (
	"testing"
)

type TestRunner struct {
	callback func(cmd string, args ...string) (string, error)
}

// TestRunner mocks out command execution.
func NewTestRunner(callback func(cmd string, args ...string) (string, error)) (*TestRunner, error) {
	return &TestRunner{
		callback: callback,
	}, nil
}

func NewTestRunnerFailOnErr(t *testing.T, callback func(cmd string, args ...string) (string, error)) *TestRunner {
	runner, err := NewTestRunner(callback)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func (runner *TestRunner) CombinedOutput(cmd string, args ...string) (string, error) {
	return runner.callback(cmd, args...)
}
