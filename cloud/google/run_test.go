/*
Copyright 2017 The Kubernetes Authors.
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

// White box testing for functionality in run.go
package google

import (
	"fmt"
	"testing"
)

func TestRun(t *testing.T) {
	oldRunOutput := runOutput
	defer func() { runOutput = oldRunOutput }()

	tests := []struct {
		name        string
		cmd         string
		args        []string
		runErr      error
		expectErr   bool
	}{
		{
			name: "success",
		},
		{
			name:      "error",
			runErr:    fmt.Errorf("404"),
			expectErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runOutput = func(cmd string, args ...string) (string, error) {
				if cmd != test.cmd {
					t.Errorf("Unexpected command. Got: %v, Want: %v", cmd, test.cmd)
				}
				if len(args) != len(test.args) {
					t.Fatalf("Unexpected number of arguments. Got: %v, Want: %v", len(args), len(test.args))
				}
				for idx := range test.args {
					if args[idx] != test.args[idx] {
						t.Errorf("Unexpected argument at index %v. Got: %v, Want: %v", idx, args[idx], test.args[idx])
					}
				}
				return "", test.runErr
			}

			err := run(test.cmd, test.args...)
			if (test.expectErr && err == nil) || (!test.expectErr && err != nil) {
				t.Fatalf("Error not as expected. Got: %v, Want Err: %v", err, test.expectErr)
			}
		})
	}
}