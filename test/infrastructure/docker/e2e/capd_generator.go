/*
Copyright 2019 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/framework/exec"
)

type provider struct{}

// GetName returns the name of the components being generated.
func (g *provider) GetName() string {
	return "Cluster API Provider Docker version: Local files"
}

func (g *provider) kustomizePath(path string) string {
	return "../config/" + path
}

// Manifests return the generated components and any error if there is one.
func (g *provider) Manifests(ctx context.Context) ([]byte, error) {
	kustomize := exec.NewCommand(
		exec.WithCommand("kustomize"),
		exec.WithArgs("build", g.kustomizePath("default")),
	)
	stdout, stderr, err := kustomize.Run(ctx)
	if err != nil {
		fmt.Println(string(stderr))
		return nil, errors.WithStack(err)
	}
	return stdout, nil
}
