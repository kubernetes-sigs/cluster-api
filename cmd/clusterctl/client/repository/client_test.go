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

package repository

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_newRepositoryClient_LocalFileSystemRepository(t *testing.T) {
	g := NewWithT(t)

	tmpDir := createTempDir(t)
	defer os.RemoveAll(tmpDir)

	dst1 := createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/v1.0.0/bootstrap-components.yaml", "")
	dst2 := createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v2.0.0/bootstrap-components.yaml", "")

	configClient, err := config.New("", config.InjectReader(test.NewFakeReader()))
	g.Expect(err).NotTo(HaveOccurred())

	type fields struct {
		provider config.Provider
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "successfully creates repository client with local filesystem backend and scheme == \"\"",
			fields: fields{
				provider: config.NewProvider("foo", dst1, clusterctlv1.BootstrapProviderType),
			},
		},
		{
			name: "successfully creates repository client with local filesystem backend and scheme == \"file\"",
			fields: fields{
				provider: config.NewProvider("bar", "file://"+dst2, clusterctlv1.BootstrapProviderType),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			repoClient, err := newRepositoryClient(tt.fields.provider, configClient)
			gs.Expect(err).NotTo(HaveOccurred())

			var expected *localRepository
			gs.Expect(repoClient.repository).To(BeAssignableToTypeOf(expected))
		})
	}
}

func Test_newRepositoryClient_YamlProcessor(t *testing.T) {
	tests := []struct {
		name   string
		opts   []Option
		assert func(*WithT, yaml.Processor)
	}{
		{
			name: "it creates a repository client with simple yaml processor by default",
			assert: func(g *WithT, p yaml.Processor) {
				_, ok := (p).(*yaml.SimpleProcessor)
				g.Expect(ok).To(BeTrue())
			},
		},
		{
			name: "it creates a repository client with specified yaml processor",
			opts: []Option{InjectYamlProcessor(test.NewFakeProcessor())},
			assert: func(g *WithT, p yaml.Processor) {
				_, ok := (p).(*yaml.SimpleProcessor)
				g.Expect(ok).To(BeFalse())
				_, ok = (p).(*test.FakeProcessor)
				g.Expect(ok).To(BeTrue())
			},
		},
		{
			name: "it creates a repository with simple yaml processor even if injected with nil processor",
			opts: []Option{InjectYamlProcessor(nil)},
			assert: func(g *WithT, p yaml.Processor) {
				g.Expect(p).ToNot(BeNil())
				_, ok := (p).(*yaml.SimpleProcessor)
				g.Expect(ok).To(BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			configProvider := config.NewProvider("fakeProvider", "", clusterctlv1.CoreProviderType)
			configClient, err := config.New("", config.InjectReader(test.NewFakeReader()))
			g.Expect(err).NotTo(HaveOccurred())

			tt.opts = append(tt.opts, InjectRepository(NewMemoryRepository()))

			repoClient, err := newRepositoryClient(
				configProvider,
				configClient,
				tt.opts...,
			)
			g.Expect(err).NotTo(HaveOccurred())
			tt.assert(g, repoClient.processor)
		})
	}
}
