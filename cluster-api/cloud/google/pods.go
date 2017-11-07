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
	"bytes"
	"fmt"
	"os/exec"
	"text/template"
)

func CreateMachineControllerPod(token string) error {
	tmpl, err := template.ParseFiles("cloud/google/pods/machine-controller.yaml")
	if err != nil {
		return err
	}

	type params struct {
		Token string
	}

	var tmplBuf bytes.Buffer

	err = tmpl.Execute(&tmplBuf, params{
		Token: token,
	})
	if err != nil {
		return err
	}

	cmd := exec.Command("kubectl", "create", "-f", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write(tmplBuf.Bytes())
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't create machine controller pod: %v, output: %v", err, out)
	}
	return nil
}
