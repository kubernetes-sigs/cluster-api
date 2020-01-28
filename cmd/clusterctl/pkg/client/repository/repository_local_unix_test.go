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
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

func Test_localRepository_newLocalRepository(t *testing.T) {
	type fields struct {
		provider              config.Provider
		configVariablesClient config.VariablesClient
	}
	type want struct {
		basepath       string
		providerName   string
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
				provider:              config.NewProvider("provider-foo", "/base/path/provider-foo/v1.0.0/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want: want{
				basepath:       "/base/path",
				providerName:   "provider-foo",
				defaultVersion: "v1.0.0",
				rootPath:       "",
				componentsPath: "bootstrap-components.yaml",
			},
			wantErr: false,
		},
		{
			name: "successfully creates new local repository object with a single version and no basepath",
			fields: fields{
				provider:              config.NewProvider("provider-foo", "/provider-foo/v1.0.0/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want: want{
				basepath:       "/",
				providerName:   "provider-foo",
				defaultVersion: "v1.0.0",
				rootPath:       "",
				componentsPath: "bootstrap-components.yaml",
			},
			wantErr: false,
		},
		{
			name: "fails if an absolute path not specified",
			fields: fields{
				provider:              config.NewProvider("provider-foo", "./provider-foo/v1/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want:    want{},
			wantErr: true,
		},
		{
			name: "fails if provider name does not match in the path",
			fields: fields{
				provider:              config.NewProvider("provider-foo", "/foo/bar/provider-bar/v1/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want:    want{},
			wantErr: true,
		},
		{
			name: "fails if malformed path: invalid version directory",
			fields: fields{
				provider:              config.NewProvider("provider-foo", "/foo/bar/provider-foo/v.a.b.c/bootstrap-components.yaml", clusterctlv1.BootstrapProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
			},
			want:    want{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newLocalRepository(tt.fields.provider, tt.fields.configVariablesClient)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}
			if got.basepath != tt.want.basepath {
				t.Errorf("got.basepath = %v, want = %v ", got.basepath, tt.want.basepath)
			}
			if got.providerName != tt.want.providerName {
				t.Errorf("got.providerName = %v, want = %v ", got.providerName, tt.want.providerName)
			}
			if got.DefaultVersion() != tt.want.defaultVersion {
				t.Errorf("got.DefaultVersion() = %v, want = %v ", got.DefaultVersion(), tt.want.defaultVersion)
			}
			if got.RootPath() != tt.want.rootPath {
				t.Errorf("got.RootPath() = %v, want = %v ", got.RootPath(), tt.want.rootPath)
			}
			if got.ComponentsPath() != tt.want.componentsPath {
				t.Errorf("got.ComponentsPath() = %v, want = %v ", got.ComponentsPath(), tt.want.componentsPath)
			}
		})
	}
}

func createTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "cc")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	return dir
}

func createLocalTestProviderFile(t *testing.T, tmpDir, path, msg string) string {
	dst := filepath.Join(tmpDir, path)
	// Create all directories in the standard layout
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatalf("err: %s", err)
	}

	if err := ioutil.WriteFile(dst, []byte(msg), 0644); err != nil {
		t.Fatalf("err: %s", err)
	}
	return dst
}

func Test_localRepository_newLocalRepository_Latest(t *testing.T) {
	tmpDir := createTempDir(t)
	defer os.RemoveAll(tmpDir)

	// Create several release directories
	createLocalTestProviderFile(t, tmpDir, "provider-2/v1.0.0/bootstrap-components.yaml", "foo: bar")
	createLocalTestProviderFile(t, tmpDir, "provider-2/v1.0.1/bootstrap-components.yaml", "foo: bar")
	createLocalTestProviderFile(t, tmpDir, "provider-2/Foo.Bar/bootstrap-components.yaml", "foo: bar")
	createLocalTestProviderFile(t, tmpDir, "provider-2/foo.file", "foo: bar")

	// Provider URL for the latest release
	p2URLLatest := "provider-2/latest/bootstrap-components.yaml"
	p2URLLatestAbs := filepath.Join(tmpDir, p2URLLatest)
	p2 := config.NewProvider("provider-2", p2URLLatestAbs, clusterctlv1.BootstrapProviderType)

	got, err := newLocalRepository(p2, test.NewFakeVariableClient())
	if err != nil {
		t.Fatalf("got error %v when none was expected", err)
	}

	if got.basepath != tmpDir {
		t.Errorf("got.basepath = %v, want = %v ", got.basepath, tmpDir)
	}
	if got.providerName != "provider-2" {
		t.Errorf("got.providerName = %v, want = provider-2 ", got.providerName)
	}
	if got.DefaultVersion() != "v1.0.1" {
		t.Errorf("got.DefaultVersion() = %v, want = v1.0.1 ", got.DefaultVersion())
	}
	if got.RootPath() != "" {
		t.Errorf("got.RootPath() = %v, want = \"\" ", got.RootPath())
	}
	if got.ComponentsPath() != "bootstrap-components.yaml" {
		t.Errorf("got.ComponentsPath() = %v, want = bootstrap-components.yaml ", got.ComponentsPath())
	}

}

func Test_localRepository_GetFile(t *testing.T) {
	tmpDir := createTempDir(t)
	defer os.RemoveAll(tmpDir)

	// Provider 1: URL is for the only release available
	dst1 := createLocalTestProviderFile(t, tmpDir, "provider-1/v1.0.0/bootstrap-components.yaml", "foo: bar")
	p1 := config.NewProvider("provider-1", dst1, clusterctlv1.BootstrapProviderType)

	// Provider 2: URL is for the latest release
	createLocalTestProviderFile(t, tmpDir, "provider-2/v1.0.0/bootstrap-components.yaml", "version: v1.0.0")
	createLocalTestProviderFile(t, tmpDir, "provider-2/v1.0.1/bootstrap-components.yaml", "version: v1.0.1")
	createLocalTestProviderFile(t, tmpDir, "provider-2/Foo.Bar/bootstrap-components.yaml", "version: Foo.Bar")
	createLocalTestProviderFile(t, tmpDir, "provider-2/foo.file", "foo: bar")
	p2URLLatest := "provider-2/latest/bootstrap-components.yaml"
	p2URLLatestAbs := filepath.Join(tmpDir, p2URLLatest)
	p2 := config.NewProvider("provider-2", p2URLLatestAbs, clusterctlv1.BootstrapProviderType)

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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := newLocalRepository(tt.fields.provider, tt.fields.configVariablesClient)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, err := r.GetFile(tt.args.version, tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if string(got) != tt.want.contents {
				t.Errorf("got %s expected %s", got, tt.want.contents)
			}
		})
	}
}

func Test_localRepository_GetVersions(t *testing.T) {
	tmpDir := createTempDir(t)
	defer os.RemoveAll(tmpDir)

	// Provider 1: has a single release available
	dst1 := createLocalTestProviderFile(t, tmpDir, "provider-1/v1.0.0/bootstrap-components.yaml", "foo: bar")
	p1 := config.NewProvider("provider-1", dst1, clusterctlv1.BootstrapProviderType)

	// Provider 2: Has multiple releases available
	createLocalTestProviderFile(t, tmpDir, "provider-2/v1.0.0/bootstrap-components.yaml", "version: v1.0.0")
	createLocalTestProviderFile(t, tmpDir, "provider-2/v1.0.1/bootstrap-components.yaml", "version: v1.0.1")
	createLocalTestProviderFile(t, tmpDir, "provider-2/v2.0.1/bootstrap-components.yaml", "version: v2.0.1")
	createLocalTestProviderFile(t, tmpDir, "provider-2/Foo.Bar/bootstrap-components.yaml", "version: Foo.Bar")
	createLocalTestProviderFile(t, tmpDir, "provider-2/foo.file", "foo: bar")
	p2URLLatest := "provider-2/latest/bootstrap-components.yaml"
	p2URLLatestAbs := filepath.Join(tmpDir, p2URLLatest)
	p2 := config.NewProvider("provider-2", p2URLLatestAbs, clusterctlv1.BootstrapProviderType)

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
				versions: []string{"v1.0.0", "v1.0.1", "v2.0.1"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := newLocalRepository(tt.fields.provider, tt.fields.configVariablesClient)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
				return
			}
			got, err := r.GetVersions()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want.versions) {
				t.Fatalf("got %v, expected %v versions", len(tt.want.versions), len(got))
			}
			sort.Strings(tt.want.versions)
			sort.Strings(got)
			for i := range got {
				if got[i] != tt.want.versions[i] {
					t.Errorf("got %s expected %s", got, tt.want.versions)
				}

			}
		})
	}
}
