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

// TestRunner mocks out command execution.
type TestRunner struct {
	callback func(cmd string, args ...string) (string, error)
}

// New builds a TestRunner to mock out command execution.
func New(callback func(cmd string, args ...string) (string, error)) (*TestRunner, error) {
	return &TestRunner{
		callback: callback,
	}, nil
}

// NewOrDie builds a TestRunner or calls t.Fatal on error
func NewOrDie(t *testing.T, callback func(cmd string, args ...string) (string, error)) *TestRunner {
	runner, err := New(callback)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func (runner *TestRunner) CombinedOutput(cmd string, args ...string) (string, error) {
	return runner.callback(cmd, args...)
}
