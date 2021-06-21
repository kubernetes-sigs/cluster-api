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

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func Test_localRepository_newLocalRepository(t *testing.T) {
	type fields struct {
		provider              config.Provider
		configVariablesClient config.VariablesClient
	}
	type want struct {
		basepath       string
		providerLabel  string
		defaultVersion string
		rootPath       string
		componentsPath string
	}
	tests := []struct {
		name    string
		fields  fields
		want    want
		wantErr bool
	}{
		{
			name: "successfully creates new local repository object with a single version",
			fields: fields{
				provider:              config.NewProvider("foo", "/base/path/bootstrap-foo/v1.0.0/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want: want{
				basepath:       "/base/path",
				providerLabel:  "bootstrap-foo",
				defaultVersion: "v1.0.0",
				rootPath:       "",
				componentsPath: "bootstrap-components.yaml",
			},
			wantErr: false,
		},
		{
			name: "successfully creates new local repository object with a single version and no basepath",
			fields: fields{
				provider:              config.NewProvider("foo", "/bootstrap-foo/v1.0.0/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want: want{
				basepath:       "/",
				providerLabel:  "bootstrap-foo",
				defaultVersion: "v1.0.0",
				rootPath:       "",
				componentsPath: "bootstrap-components.yaml",
			},
			wantErr: false,
		},
		{
			name: "fails if an absolute path not specified",
			fields: fields{
				provider:              config.NewProvider("foo", "./bootstrap-foo/v1/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want:    want{},
			wantErr: true,
		},
		{
			name: "fails if provider id does not match in the path",
			fields: fields{
				provider:              config.NewProvider("foo", "/foo/bar/bootstrap-bar/v1/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want:    want{},
			wantErr: true,
		},
		{
			name: "fails if malformed path: invalid version directory",
			fields: fields{
				provider:              config.NewProvider("foo", "/foo/bar/bootstrap-foo/v.a.b.c/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want:    want{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := newLocalRepository(tt.fields.provider, tt.fields.configVariablesClient)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got.basepath).To(Equal(tt.want.basepath))
			g.Expect(got.providerLabel).To(Equal(tt.want.providerLabel))
			g.Expect(got.DefaultVersion()).To(Equal(tt.want.defaultVersion))
			g.Expect(got.RootPath()).To(Equal(tt.want.rootPath))
			g.Expect(got.ComponentsPath()).To(Equal(tt.want.componentsPath))
		})
	}
}

func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "cc")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	return dir
}

func createLocalTestProviderFile(t *testing.T, tmpDir, path, msg string) string {
	g := NewWithT(t)

	dst := filepath.Join(tmpDir, path)
	// Create all directories in the standard layout
	g.Expect(os.MkdirAll(filepath.Dir(dst), 0750)).To(Succeed())
	g.Expect(os.WriteFile(dst, []byte(msg), 0600)).To(Succeed())

	return dst
}

func Test_localRepository_newLocalRepository_Latest(t *testing.T) {
	g := NewWithT(t)

	tmpDir := createTempDir(t)
	defer os.RemoveAll(tmpDir)

	// Create several release directories
	createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/v1.0.0/bootstrap-components.yaml", "foo: bar")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/v1.0.1/bootstrap-components.yaml", "foo: bar")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/v2.0.0-alpha.0/bootstrap-components.yaml", "foo: bar")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/Foo.Bar/bootstrap-components.yaml", "foo: bar")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/foo.file", "foo: bar")

	// Provider URL for the latest release
	p2URLLatest := "bootstrap-foo/latest/bootstrap-components.yaml"
	p2URLLatestAbs := filepath.Join(tmpDir, p2URLLatest)
	p2 := config.NewProvider("foo", p2URLLatestAbs, clusterctlv1.BootstrapProviderType)

	got, err := newLocalRepository(p2, test.NewFakeVariableClient())
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(got.basepath).To(Equal(tmpDir))
	g.Expect(got.providerLabel).To(Equal("bootstrap-foo"))
	g.Expect(got.DefaultVersion()).To(Equal("v1.0.1"))
	g.Expect(got.RootPath()).To(BeEmpty())
	g.Expect(got.ComponentsPath()).To(Equal("bootstrap-components.yaml"))
}

func Test_localRepository_GetFile(t *testing.T) {
	tmpDir := createTempDir(t)
	defer os.RemoveAll(tmpDir)

	// Provider 1: URL is for the only release available
	dst1 := createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/v1.0.0/bootstrap-components.yaml", "foo: bar")
	p1 := config.NewProvider("foo", dst1, clusterctlv1.BootstrapProviderType)

	// Provider 2: URL is for the latest release
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v1.0.0/bootstrap-components.yaml", "version: v1.0.0")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v1.0.1/bootstrap-components.yaml", "version: v1.0.1")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v2.0.0-alpha.0/bootstrap-components.yaml", "version: v2.0.0-alpha.0")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/Foo.Bar/bootstrap-components.yaml", "version: Foo.Bar")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/foo.file", "foo: bar")
	p2URLLatest := "bootstrap-bar/latest/bootstrap-components.yaml"
	p2URLLatestAbs := filepath.Join(tmpDir, p2URLLatest)
	p2 := config.NewProvider("bar", p2URLLatestAbs, clusterctlv1.BootstrapProviderType)

	// Provider 3: URL is for only prerelease available
	dst3 := createLocalTestProviderFile(t, tmpDir, "bootstrap-baz/v1.0.0-alpha.0/bootstrap-components.yaml", "version: v1.0.0-alpha.0")
	p3 := config.NewProvider("baz", dst3, clusterctlv1.BootstrapProviderType)

	type fields struct {
		provider              config.Provider
		configVariablesClient config.VariablesClient
	}
	type args struct {
		version  string
		fileName string
	}
	type want struct {
		contents string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "Get file from release directory",
			fields: fields{
				provider:              p1,
				configVariablesClient: test.NewFakeVariableClient(),
			},
			args: args{
				version:  "v1.0.0",
				fileName: "bootstrap-components.yaml",
			},
			want: want{
				contents: "foo: bar",
			},
			wantErr: false,
		},
		{
			name: "Get file from latest release directory",
			fields: fields{
				provider:              p2,
				configVariablesClient: test.NewFakeVariableClient(),
			},
			args: args{
				version:  "latest",
				fileName: "bootstrap-components.yaml",
			},
			want: want{
				contents: "version: v1.0.1", // We use the file contents to determine data was read from latest release
			},
			wantErr: false,
		},
		{
			name: "Get file from default version release directory",
			fields: fields{
				provider:              p2,
				configVariablesClient: test.NewFakeVariableClient(),
			},
			args: args{
				version:  "",
				fileName: "bootstrap-components.yaml",
			},
			want: want{
				contents: "version: v1.0.1", // We use the file contents to determine data was read from latest release
			},
			wantErr: false,
		},
		{
			name: "Get file from pre-release version release directory",
			fields: fields{
				provider:              p2,
				configVariablesClient: test.NewFakeVariableClient(),
			},
			args: args{
				version:  "v2.0.0-alpha.0",
				fileName: "bootstrap-components.yaml",
			},
			want: want{
				contents: "version: v2.0.0-alpha.0", // We use the file contents to determine data was read from latest release
			},
			wantErr: false,
		},
		{
			name: "Get file from latest prerelease directory if no releases",
			fields: fields{
				provider:              p3,
				configVariablesClient: test.NewFakeVariableClient(),
			},
			args: args{
				version:  "latest",
				fileName: "bootstrap-components.yaml",
			},
			want: want{
				contents: "version: v1.0.0-alpha.0", // We use the file contents to determine data was read from latest prerelease
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r, err := newLocalRepository(tt.fields.provider, tt.fields.configVariablesClient)
			g.Expect(err).NotTo(HaveOccurred())

			got, err := r.GetFile(tt.args.version, tt.args.fileName)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(got)).To(Equal(tt.want.contents))
		})
	}
}

func Test_localRepository_GetVersions(t *testing.T) {
	tmpDir := createTempDir(t)
	defer os.RemoveAll(tmpDir)

	// Provider 1: has a single release available
	dst1 := createLocalTestProviderFile(t, tmpDir, "bootstrap-foo/v1.0.0/bootstrap-components.yaml", "foo: bar")
	p1 := config.NewProvider("foo", dst1, clusterctlv1.BootstrapProviderType)

	// Provider 2: Has multiple releases available
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v1.0.0/bootstrap-components.yaml", "version: v1.0.0")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v1.0.1/bootstrap-components.yaml", "version: v1.0.1")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v2.0.1/bootstrap-components.yaml", "version: v2.0.1")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v2.0.2+exp.sha.5114f85/bootstrap-components.yaml", "version: v2.0.2+exp.sha.5114f85")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/v2.0.3-alpha/bootstrap-components.yaml", "version: v2.0.3-alpha")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/Foo.Bar/bootstrap-components.yaml", "version: Foo.Bar")
	createLocalTestProviderFile(t, tmpDir, "bootstrap-bar/foo.file", "foo: bar")
	p2URLLatest := "bootstrap-bar/latest/bootstrap-components.yaml"
	p2URLLatestAbs := filepath.Join(tmpDir, p2URLLatest)
	p2 := config.NewProvider("bar", p2URLLatestAbs, clusterctlv1.BootstrapProviderType)

	type fields struct {
		provider              config.Provider
		configVariablesClient config.VariablesClient
	}
	type want struct {
		versions []string
	}
	tests := []struct {
		name    string
		fields  fields
		want    want
		wantErr bool
	}{
		{
			name: "Get the only release available from release directory",
			fields: fields{
				provider:              p1,
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want: want{
				versions: []string{"v1.0.0"},
			},
			wantErr: false,
		},
		{
			name: "Get all valid releases available from release directory",
			fields: fields{
				provider:              p2,
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want: want{
				versions: []string{"v1.0.0", "v1.0.1", "v2.0.1", "v2.0.2+exp.sha.5114f85", "v2.0.3-alpha"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r, err := newLocalRepository(tt.fields.provider, tt.fields.configVariablesClient)
			g.Expect(err).NotTo(HaveOccurred())

			got, err := r.GetVersions()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(ConsistOf(tt.want.versions))
		})
	}
}
