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

package repository

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/homedir"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func TestOverrides(t *testing.T) {
	tests := []struct {
		name            string
		configVarClient config.VariablesClient
		expectedPath    string
	}{
		{
			name:            "returns default overrides path if no config provided",
			configVarClient: test.NewFakeVariableClient(),
			expectedPath:    filepath.Join(homedir.HomeDir(), config.ConfigFolder, overrideFolder, "infrastructure-myinfra", "v1.0.1", "infra-comp.yaml"),
		},
		{
			name:            "returns default overrides path if config variable is empty",
			configVarClient: test.NewFakeVariableClient().WithVar(overrideFolderKey, ""),
			expectedPath:    filepath.Join(homedir.HomeDir(), config.ConfigFolder, overrideFolder, "infrastructure-myinfra", "v1.0.1", "infra-comp.yaml"),
		},
		{
			name:            "returns default overrides path if config variable is whitespace",
			configVarClient: test.NewFakeVariableClient().WithVar(overrideFolderKey, "   "),
			expectedPath:    filepath.Join(homedir.HomeDir(), config.ConfigFolder, overrideFolder, "infrastructure-myinfra", "v1.0.1", "infra-comp.yaml"),
		},
		{
			name:            "uses overrides folder from the config variables",
			configVarClient: test.NewFakeVariableClient().WithVar(overrideFolderKey, "/Users/foobar/workspace/releases"),
			expectedPath:    "/Users/foobar/workspace/releases/infrastructure-myinfra/v1.0.1/infra-comp.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			provider := config.NewProvider("myinfra", "", clusterctlv1.InfrastructureProviderType)
			override := newOverride(&newOverrideInput{
				configVariablesClient: tt.configVarClient,
				provider:              provider,
				version:               "v1.0.1",
				filePath:              "infra-comp.yaml",
			})

			g.Expect(override.Path()).To(Equal(tt.expectedPath))
		})
	}
}

func TestGetLocalOverrides(t *testing.T) {
	t.Run("returns contents of file successfully", func(t *testing.T) {
		g := NewWithT(t)
		tmpDir := createTempDir(t)
		defer os.RemoveAll(tmpDir)

		createLocalTestProviderFile(t, tmpDir, "infrastructure-myinfra/v1.0.1/infra-comp.yaml", "foo: bar")

		info := &newOverrideInput{
			configVariablesClient: test.NewFakeVariableClient().WithVar(overrideFolderKey, tmpDir),
			provider:              config.NewProvider("myinfra", "", clusterctlv1.InfrastructureProviderType),
			version:               "v1.0.1",
			filePath:              "infra-comp.yaml",
		}

		b, err := getLocalOverride(info)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(string(b)).To(Equal("foo: bar"))
	})

	t.Run("doesn't return error if file does not exist", func(t *testing.T) {
		g := NewWithT(t)

		info := &newOverrideInput{
			configVariablesClient: test.NewFakeVariableClient().WithVar(overrideFolderKey, "do-not-exist"),
			provider:              config.NewProvider("myinfra", "", clusterctlv1.InfrastructureProviderType),
			version:               "v1.0.1",
			filePath:              "infra-comp.yaml",
		}

		_, err := getLocalOverride(info)
		g.Expect(err).ToNot(HaveOccurred())
	})
}
