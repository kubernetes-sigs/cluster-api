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
	"os"
	"strings"
)

// KubectlApply shells out to kubectl apply.
//
// TODO: Remove this usage of kubectl and replace with a function from apply.go using the controller-runtime client.
func KubectlApply(ctx context.Context, kubeconfigPath string, resources []byte, args ...string) error {
	aargs := append([]string{"apply", "--kubeconfig", kubeconfigPath, "-f", "-"}, args...)
	rbytes := bytes.NewReader(resources)
	applyCmd := NewCommand(
		WithCommand(kubectlPath()),
		WithArgs(aargs...),
		WithStdin(rbytes),
	)

	fmt.Printf("Running kubectl %s\n", strings.Join(aargs, " "))
	stdout, stderr, err := applyCmd.Run(ctx)
	fmt.Printf("stderr:\n%s\n", string(stderr))
	fmt.Printf("stdout:\n%s\n", string(stdout))
	return err
}

// KubectlWait shells out to kubectl wait.
func KubectlWait(ctx context.Context, kubeconfigPath string, args ...string) error {
	wargs := append([]string{"wait", "--kubeconfig", kubeconfigPath}, args...)
	wait := NewCommand(
		WithCommand(kubectlPath()),
		WithArgs(wargs...),
	)
	_, stderr, err := wait.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return err
	}
	return nil
}

func kubectlPath() string {
	if kubectlPath, ok := os.LookupEnv("CAPI_KUBECTL_PATH"); ok {
		return kubectlPath
	}
	return "kubectl"
}
