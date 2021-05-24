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

package exec

import (
	"bytes"
	"context"
	"fmt"
)

// KubectlApply shells out to kubectl apply.
//
// TODO: Remove this usage of kubectl and replace with a function from apply.go using the controller-runtime client.
func KubectlApply(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) error {
	aargs := append([]string{"apply", "--kubeconfig", kubeconfigPath, "-f", "-"}, args...)
	rbytes := bytes.NewReader(resources)
	applyCmd := NewCommand(
		WithCommand("kubectl"),
		WithArgs(aargs...),
		WithStdin(rbytes),
	)
	stdout, stderr, err := applyCmd.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}
	fmt.Println(string(stdout))
	return nil
}

// KubectlWait shells out to kubectl wait.
func KubectlWait(ctx context.Context, kubeconfigPath string, args ...string) error {
	wargs := append([]string{"wait", "--kubeconfig", kubeconfigPath}, args...)
	wait := NewCommand(
		WithCommand("kubectl"),
		WithArgs(wargs...),
	)
	_, stderr, err := wait.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}
	return nil
}
