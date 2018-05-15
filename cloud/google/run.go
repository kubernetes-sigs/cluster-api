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

package google

import (
	"os/exec"
	"fmt"
)

func run(cmd string, args ...string) error {
	_, err := runOutput(cmd, args...)
	return err
}

// runOutput is declared as function variable to allow for testing hooks.
var runOutput = func(cmd string, args ...string) (string, error) {
	c := exec.Command(cmd, args...)
	if out, err := c.CombinedOutput(); err != nil {
		return string(out), fmt.Errorf("error: %v, output: %s", err, string(out))
	}
	return "", nil
}