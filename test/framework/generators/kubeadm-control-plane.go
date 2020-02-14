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

package generators

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/test/framework/exec"
)

// KubeadmControlPlaneGitHubManifestsFormat is a convenience string to get Cluster API manifests at an exact revision.
// Set KubeadmControlPlane.KustomizePath = fmt.Sprintf(KubeadmControlPlaneGitHubManifestsFormat, <some git ref>).
var KubeadmControlPlaneGitHubManifestsFormat = "https://github.com/kubernetes-sigs/cluster-api//controlplane/kubeadm/config?ref=%s"

// KubeadmControlPlane generates provider components for the Kubeadm Control Plane provider.
type KubeadmControlPlane struct {
	// KustomizePath is a URL, relative or absolute filesystem path to a kustomize file that generates the Kubeadm Control Plane provider manifests.
	// KustomizePath takes precedence over Version.
	KustomizePath string
	// Version defines the release version. If GitRef is not set Version must be set and will not use kustomize.
	Version string
}

// GetName returns the name of the components being generated.
func (g *KubeadmControlPlane) GetName() string {
	if g.KustomizePath != "" {
		return fmt.Sprintf("Using Kubeadm control plane provider manifests from: %q", g.KustomizePath)
	}
	return fmt.Sprintf("Kubeadm control plane provider manifests from Cluster API release version %s", g.Version)
}

func (g *KubeadmControlPlane) releaseYAMLPath() string {
	return fmt.Sprintf("https://github.com/kubernetes-sigs/cluster-api/releases/download/%s/cluster-api-components.yaml", g.Version)
}

// Manifests return the generated components and any error if there is one.
func (g *KubeadmControlPlane) Manifests(ctx context.Context) ([]byte, error) {
	if g.KustomizePath != "" {
		kustomize := exec.NewCommand(
			exec.WithCommand("kustomize"),
			exec.WithArgs("build", g.KustomizePath),
		)
		stdout, stderr, err := kustomize.Run(ctx)
		if err != nil {
			fmt.Println(string(stderr))
			return nil, errors.WithStack(err)
		}
		stdout = bytes.Replace(stdout, []byte("imagePullPolicy: Always"), []byte("imagePullPolicy: IfNotPresent"), -1)
		return stdout, nil
	}
	resp, err := http.Get(g.releaseYAMLPath())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	return out, nil
}
