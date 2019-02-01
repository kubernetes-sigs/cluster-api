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

package cmdrunner

import (
	"os/exec"
)

// Runner has one method that executes a command and returns stdout and stderr.
type Runner interface {
	CombinedOutput(cmd string, args ...string) (output string, err error)
}

type realRunner struct {
}

// New returns a command runner.
func New() *realRunner { // nolint
	return &realRunner{}
}

func (runner *realRunner) CombinedOutput(cmd string, args ...string) (string, error) {
	output, err := exec.Command(cmd, args...).CombinedOutput()
	return string(output), err
}
