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
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework/exec"
	. "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
)

const (
	fileURIScheme  = "file"
	httpURIScheme  = "http"
	httpsURIScheme = "https"
)

// RepositoryFileTransformation is a helpers for managing a clusterctl
// local repository to be used for running e2e tests in isolation.
type RepositoryFileTransformation func([]byte) ([]byte, error)

// CreateRepositoryInput is the input for CreateRepository.
type CreateRepositoryInput struct {
	RepositoryFolder    string
	E2EConfig           *E2EConfig
	FileTransformations []RepositoryFileTransformation
}

// RegisterClusterResourceSetConfigMapTransformation registers a FileTransformations that injects a manifests file into
// a ConfigMap that defines a ClusterResourceSet resource.
//
// NOTE: this transformation is specifically designed for replacing "data: ${envSubstVar}".
func (i *CreateRepositoryInput) RegisterClusterResourceSetConfigMapTransformation(manifestPath, envSubstVar string) {
	Byf("Reading the ClusterResourceSet manifest %s", manifestPath)
	manifestData, err := os.ReadFile(manifestPath) //nolint:gosec
	Expect(err).ToNot(HaveOccurred(), "Failed to read the ClusterResourceSet manifest file")
	Expect(manifestData).ToNot(BeEmpty(), "ClusterResourceSet manifest file should not be empty")

	i.FileTransformations = append(i.FileTransformations, func(template []byte) ([]byte, error) {
		oldData := fmt.Sprintf("data: ${%s}", envSubstVar)
		newData := "data:\n"
		newData += "  resources: |\n"
		for _, l := range strings.Split(string(manifestData), "\n") {
			newData += strings.Repeat(" ", 4) + l + "\n"
		}
		return bytes.ReplaceAll(template, []byte(oldData), []byte(newData)), nil
	})
}

const clusterctlConfigFileName = "clusterctl-config.yaml"
const clusterctlConfigV1_2FileName = "clusterctl-config.v1.2.yaml"

// CreateRepository creates a clusterctl local repository based on the e2e test config, and the returns the path
// to a clusterctl config file to be used for working with such repository.
func CreateRepository(ctx context.Context, input CreateRepositoryInput) string {
	Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling CreateRepository")
	Expect(os.MkdirAll(input.RepositoryFolder, 0750)).To(Succeed(), "Failed to create the clusterctl local repository folder %s", input.RepositoryFolder)

	providers := []providerConfig{}
	providersV1_2 := []providerConfig{}
	for _, provider := range input.E2EConfig.Providers {
		providerLabel := clusterctlv1.ManifestLabel(provider.Name, clusterctlv1.ProviderType(provider.Type))
		providerURL := filepath.Join(input.RepositoryFolder, providerLabel, "latest", "components.yaml")
		for _, version := range provider.Versions {
			manifest, err := YAMLForComponentSource(ctx, version)
			Expect(err).ToNot(HaveOccurred(), "Failed to generate the manifest for %q / %q", providerLabel, version.Name)

			sourcePath := filepath.Join(input.RepositoryFolder, providerLabel, version.Name)
			Expect(os.MkdirAll(sourcePath, 0750)).To(Succeed(), "Failed to create the clusterctl local repository folder for %q / %q", providerLabel, version.Name)

			filePath := filepath.Join(sourcePath, "components.yaml")
			Expect(os.WriteFile(filePath, manifest, 0600)).To(Succeed(), "Failed to write manifest in the clusterctl local repository for %q / %q", providerLabel, version.Name)

			destinationPath := filepath.Join(input.RepositoryFolder, providerLabel, version.Name, "components.yaml")
			allFiles := append(provider.Files, version.Files...)
			for _, file := range allFiles {
				data, err := os.ReadFile(file.SourcePath)
				Expect(err).ToNot(HaveOccurred(), "Failed to read file %q / %q", provider.Name, file.SourcePath)

				// Applies FileTransformations if defined
				for _, t := range input.FileTransformations {
					data, err = t(data)
					Expect(err).ToNot(HaveOccurred(), "Failed to apply transformation func template %q", file)
				}

				destinationFile := filepath.Join(filepath.Dir(destinationPath), file.TargetName)
				Expect(os.WriteFile(destinationFile, data, 0600)).To(Succeed(), "Failed to write clusterctl local repository file %q / %q", provider.Name, file.TargetName)
			}
		}
		p := providerConfig{
			Name: provider.Name,
			URL:  providerURL,
			Type: provider.Type,
		}
		providers = append(providers, p)
		if !(clusterctlv1.ProviderType(provider.Type) == clusterctlv1.IPAMProviderType || clusterctlv1.ProviderType(provider.Type) == clusterctlv1.RuntimeExtensionProviderType || clusterctlv1.ProviderType(provider.Type) == clusterctlv1.AddonProviderType) {
			providersV1_2 = append(providersV1_2, p)
		}
	}

	// set this path to an empty file under the repository path, so test can run in isolation without user's overrides kicking in
	overridePath := filepath.Join(input.RepositoryFolder, "overrides")
	Expect(os.MkdirAll(overridePath, 0750)).To(Succeed(), "Failed to create the clusterctl overrides folder %q", overridePath)

	// creates a clusterctl config file to be used for working with such repository
	clusterctlConfigFile := &clusterctlConfig{
		Path: filepath.Join(input.RepositoryFolder, clusterctlConfigFileName),
		Values: map[string]interface{}{
			"providers":       providers,
			"overridesFolder": overridePath,
		},
	}
	for key := range input.E2EConfig.Variables {
		clusterctlConfigFile.Values[key] = input.E2EConfig.GetVariable(key)
	}
	Expect(clusterctlConfigFile.write()).To(Succeed(), "Failed to write clusterctlConfigFile")

	// creates a clusterctl config file to be used for working with such repository with only the providers supported in clusterctl < v1.3
	clusterctlConfigFileV1_2 := &clusterctlConfig{
		Path: filepath.Join(input.RepositoryFolder, clusterctlConfigV1_2FileName),
		Values: map[string]interface{}{
			"providers":       providersV1_2,
			"overridesFolder": overridePath,
		},
	}
	for key := range input.E2EConfig.Variables {
		clusterctlConfigFileV1_2.Values[key] = input.E2EConfig.GetVariable(key)
	}
	Expect(clusterctlConfigFileV1_2.write()).To(Succeed(), "Failed to write v1.2 clusterctlConfigFile")

	return clusterctlConfigFile.Path
}

// CopyAndAmendClusterctlConfigInput is the input for copyAndAmendClusterctlConfig.
type CopyAndAmendClusterctlConfigInput struct {
	ClusterctlConfigPath string
	OutputPath           string
	Variables            map[string]string
}

// CopyAndAmendClusterctlConfig copies the clusterctl-config from ClusterctlConfigPath to
// OutputPath and adds the given Variables.
func CopyAndAmendClusterctlConfig(_ context.Context, input CopyAndAmendClusterctlConfigInput) error {
	// Read clusterctl config from ClusterctlConfigPath.
	clusterctlConfigFile := &clusterctlConfig{
		Path: input.ClusterctlConfigPath,
	}
	if err := clusterctlConfigFile.read(); err != nil {
		return err
	}

	// Overwrite variables.
	if clusterctlConfigFile.Values == nil {
		clusterctlConfigFile.Values = map[string]interface{}{}
	}
	for key, value := range input.Variables {
		clusterctlConfigFile.Values[key] = value
	}

	// Write clusterctl config to OutputPath.
	clusterctlConfigFile.Path = input.OutputPath
	return clusterctlConfigFile.write()
}

// AdjustConfigPathForBinary adjusts the clusterctlConfigPath in case the clusterctl version v1.3.
func AdjustConfigPathForBinary(clusterctlBinaryPath, clusterctlConfigPath string) string {
	version, err := getClusterCtlVersion(clusterctlBinaryPath)
	Expect(err).ToNot(HaveOccurred())

	if version.LT(semver.MustParse("1.3.0")) {
		return strings.Replace(clusterctlConfigPath, clusterctlConfigFileName, clusterctlConfigV1_2FileName, -1)
	}
	return clusterctlConfigPath
}

func getClusterCtlVersion(clusterctlBinaryPath string) (*semver.Version, error) {
	clusterctl := exec.NewCommand(
		exec.WithCommand(clusterctlBinaryPath),
		exec.WithArgs("version", "--output", "short"),
	)
	stdout, stderr, err := clusterctl.Run(context.Background())
	if err != nil {
		Expect(err).ToNot(HaveOccurred(), "failed to run clusterctl version:\nstdout:\n%s\nstderr:\n%s", string(stdout), string(stderr))
	}
	data := stdout
	version, err := semver.ParseTolerant(string(data))
	if err != nil {
		return nil, fmt.Errorf("clusterctl version returned an invalid version: %s", string(data))
	}
	return &version, nil
}

// YAMLForComponentSource returns the YAML for the provided component source.
func YAMLForComponentSource(ctx context.Context, source ProviderVersionSource) ([]byte, error) {
	var data []byte

	switch source.Type {
	case URLSource:
		buf, err := getComponentSourceFromURL(ctx, source)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get component source YAML from URL")
		}
		data = buf
	case KustomizeSource:
		// Set Path of kustomize binary using CAPI_KUSTOMIZE_PATH env
		kustomizePath, ok := os.LookupEnv("CAPI_KUSTOMIZE_PATH")
		if !ok {
			kustomizePath = "kustomize"
		}
		kustomize := exec.NewCommand(
			exec.WithCommand(kustomizePath),
			exec.WithArgs("build", source.Value))
		stdout, stderr, err := kustomize.Run(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to execute kustomize: %s", stderr)
		}
		data = stdout
	default:
		return nil, errors.Errorf("invalid type: %q", source.Type)
	}

	for _, replacement := range source.Replacements {
		rx, err := regexp.Compile(replacement.Old)
		if err != nil {
			return nil, err
		}
		data = rx.ReplaceAll(data, []byte(replacement.New))
	}

	return data, nil
}

// getComponentSourceFromURL fetches contents of component source YAML file from provided URL source.
func getComponentSourceFromURL(ctx context.Context, source ProviderVersionSource) ([]byte, error) {
	var buf []byte

	u, err := url.Parse(source.Value)
	if err != nil {
		return nil, err
	}

	// url.Parse always lower cases scheme
	switch u.Scheme {
	case "", fileURIScheme:
		buf, err = os.ReadFile(u.Path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read file")
		}
	case httpURIScheme, httpsURIScheme:
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.Value, http.NoBody)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %s: failed to create request", source.Value)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %s", source.Value)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, errors.Errorf("failed to get %s: got status code %d", source.Value, resp.StatusCode)
		}
		defer resp.Body.Close()
		buf, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %s: failed to read body", source.Value)
		}
	default:
		return nil, errors.Errorf("unknown scheme for component source %q: allowed values are file, http, https", u.Scheme)
	}

	return buf, nil
}
