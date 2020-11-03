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

package clusterctl

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
)

// Provides helpers for managing a clusterctl local repository to be used for running e2e tests in isolation.
type RepositoryFileTransformation func([]byte) ([]byte, error)

// CreateRepositoryInput is the input for CreateRepository.
type CreateRepositoryInput struct {
	RepositoryFolder    string
	E2EConfig           *E2EConfig
	FileTransformations []RepositoryFileTransformation
}

// RegisterClusterResourceSetConfigMapTransformation registers a FileTransformations that injects a CNI file into
// a ConfigMap that defines a ClusterResourceSet resource.
//
// NOTE: this transformation is specifically designed for replacing "data: ${envSubstVar}".
func (i *CreateRepositoryInput) RegisterClusterResourceSetConfigMapTransformation(cniManifestPath, envSubstVar string) {
	By(fmt.Sprintf("Reading the CNI manifest %s", cniManifestPath))
	cniData, err := ioutil.ReadFile(cniManifestPath)
	Expect(err).ToNot(HaveOccurred(), "Failed to read the e2e test CNI file")
	Expect(cniData).ToNot(BeEmpty(), "CNI file should not be empty")

	i.FileTransformations = append(i.FileTransformations, func(template []byte) ([]byte, error) {
		old := fmt.Sprintf("data: ${%s}", envSubstVar)
		new := "data:\n"
		new += "  resources: |\n"
		for _, l := range strings.Split(string(cniData), "\n") {
			new += strings.Repeat(" ", 4) + l + "\n"
		}
		return bytes.Replace(template, []byte(old), []byte(new), -1), nil
	})
}

// CreateRepository creates a clusterctl local repository based on the e2e test config, and the returns the path
// to a clusterctl config file to be used for working with such repository.
func CreateRepository(ctx context.Context, input CreateRepositoryInput) string {
	Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling CreateRepository")
	Expect(os.MkdirAll(input.RepositoryFolder, 0755)).To(Succeed(), "Failed to create the clusterctl local repository folder %s", input.RepositoryFolder)

	providers := []providerConfig{}
	for _, provider := range input.E2EConfig.Providers {
		providerURL := ""
		for _, version := range provider.Versions {
			providerLabel := clusterctlv1.ManifestLabel(provider.Name, clusterctlv1.ProviderType(provider.Type))

			generator := framework.ComponentGeneratorForComponentSource(version)
			manifest, err := generator.Manifests(ctx)
			Expect(err).ToNot(HaveOccurred(), "Failed to generate the manifest for %q / %q", providerLabel, version.Name)

			sourcePath := filepath.Join(input.RepositoryFolder, providerLabel, version.Name)
			Expect(os.MkdirAll(sourcePath, 0755)).To(Succeed(), "Failed to create the clusterctl local repository folder for %q / %q", providerLabel, version.Name)

			filePath := filepath.Join(sourcePath, "components.yaml")
			Expect(ioutil.WriteFile(filePath, manifest, 0600)).To(Succeed(), "Failed to write manifest in the clusterctl local repository for %q / %q", providerLabel, version.Name)

			if providerURL == "" {
				providerURL = filePath
			}
		}
		providers = append(providers, providerConfig{
			Name: provider.Name,
			URL:  providerURL,
			Type: provider.Type,
		})

		for _, file := range provider.Files {
			data, err := ioutil.ReadFile(file.SourcePath)
			Expect(err).ToNot(HaveOccurred(), "Failed to read file %q / %q", provider.Name, file.SourcePath)

			// Applies FileTransformations if defined
			for _, t := range input.FileTransformations {
				data, err = t(data)
				Expect(err).ToNot(HaveOccurred(), "Failed to apply transformation func template %q", file)
			}

			destinationFile := filepath.Join(filepath.Dir(providerURL), file.TargetName)
			Expect(ioutil.WriteFile(destinationFile, data, 0600)).To(Succeed(), "Failed to write clusterctl local repository file %q / %q", provider.Name, file.TargetName)
		}
	}

	// set this path to an empty file under the repository path, so test can run in isolation without user's overrides kicking in
	overridePath := filepath.Join(input.RepositoryFolder, "overrides")
	Expect(os.MkdirAll(overridePath, 0755)).To(Succeed(), "Failed to create the clusterctl overrides folder %q", overridePath)

	// creates a clusterctl config file to be used for working with such repository
	clusterctlConfigFile := &clusterctlConfig{
		Path: filepath.Join(input.RepositoryFolder, "clusterctl-config.yaml"),
		Values: map[string]interface{}{
			"providers":       providers,
			"overridesFolder": overridePath,
		},
	}
	for key, value := range input.E2EConfig.Variables {
		clusterctlConfigFile.Values[key] = value
	}
	clusterctlConfigFile.write()

	return clusterctlConfigFile.Path
}
