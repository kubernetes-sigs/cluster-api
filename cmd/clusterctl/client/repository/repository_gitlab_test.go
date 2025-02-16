/*
Copyright 2022 The Kubernetes Authors.

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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	goproxytest "sigs.k8s.io/cluster-api/internal/goproxy/test"
)

func Test_gitLabRepository_newGitLabRepository(t *testing.T) {
	type field struct {
		providerConfig config.Provider
		variableClient config.VariablesClient
	}
	tests := []struct {
		name      string
		field     field
		want      *gitLabRepository
		wantedErr string
	}{
		{
			name: "can create a new GitLab repo",
			field: field{
				providerConfig: config.NewProvider("test", "https://gitlab.example.org/api/v4/projects/group%2Fproject/packages/generic/my-package/v1.0/path", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want: &gitLabRepository{
				providerConfig:        config.NewProvider("test", "https://gitlab.example.org/api/v4/projects/group%2Fproject/packages/generic/my-package/v1.0/path", clusterctlv1.CoreProviderType),
				configVariablesClient: test.NewFakeVariableClient(),
				httpClient:            http.DefaultClient,
				host:                  "gitlab.example.org",
				projectSlug:           "group%2Fproject",
				packageName:           "my-package",
				defaultVersion:        "v1.0",
				rootPath:              ".",
				componentsPath:        "path",
			},
			wantedErr: "",
		},
		{
			name: "missing variableClient",
			field: field{
				providerConfig: config.NewProvider("test", "https://gitlab.example.org/api/v4/projects/group%2Fproject/packages/generic/my-package/v1.0/path", clusterctlv1.CoreProviderType),
				variableClient: nil,
			},
			want:      nil,
			wantedErr: "invalid arguments: configVariablesClient can't be nil",
		},
		{
			name: "provider url is not valid",
			field: field{
				providerConfig: config.NewProvider("test", "%gh&%ij", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:      nil,
			wantedErr: "invalid url: parse \"%gh&%ij\": invalid URL escape \"%gh\"",
		},
		{
			name: "provider url should be in https",
			field: field{
				providerConfig: config.NewProvider("test", "http://gitlab.example.org/api/v4/projects/group%2Fproject/packages/generic/my-package/v1.0/path", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:      nil,
			wantedErr: "invalid url: a GitLab repository url should be in the form https://{host}/api/v4/projects/{projectSlug}/packages/generic/{packageName}/{defaultVersion}/{componentsPath}",
		},
		{
			name: "provider url should have the correct number of parts",
			field: field{
				providerConfig: config.NewProvider("test", "https://gitlab.example.org/api/v4/projects/group%2Fproject/packages/generic/my-package/v1.0/path/grrr", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:      &gitLabRepository{},
			wantedErr: "invalid url: a GitLab repository url should be in the form https://{host}/api/v4/projects/{projectSlug}/packages/generic/{packageName}/{defaultVersion}/{componentsPath}",
		},
		{
			name: "provider url should have the correct prefix",
			field: field{
				providerConfig: config.NewProvider("test", "https://gitlab.example.org/subpath/api/v4/projects/group%2Fproject/packages/generic/my-package/v1.0/path", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:      &gitLabRepository{},
			wantedErr: "invalid url: a GitLab repository url should be in the form https://{host}/api/v4/projects/{projectSlug}/packages/generic/{packageName}/{defaultVersion}/{componentsPath}",
		},
		{
			name: "provider url should have the packages part",
			field: field{
				providerConfig: config.NewProvider("test", "https://gitlab.example.org/api/v4/projects/group%2Fproject/packages-invalid/generic/my-package/v1.0/path", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:      nil,
			wantedErr: "invalid url: a GitLab repository url should be in the form https://{host}/api/v4/projects/{projectSlug}/packages/generic/{packageName}/{defaultVersion}/{componentsPath}",
		},
		{
			name: "provider url should have the generic part",
			field: field{
				providerConfig: config.NewProvider("test", "https://gitlab.example.org/api/v4/projects/group%2Fproject/packages/generic-invalid/my-package/v1.0/path", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:      nil,
			wantedErr: "invalid url: a GitLab repository url should be in the form https://{host}/api/v4/projects/{projectSlug}/packages/generic/{packageName}/{defaultVersion}/{componentsPath}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			resetCaches()

			gitLab, err := NewGitLabRepository(tt.field.providerConfig, tt.field.variableClient)
			if tt.wantedErr != "" {
				g.Expect(err).To(MatchError(tt.wantedErr))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gitLab).To(Equal(tt.want))
		})
	}
}

func Test_gitLabRepository_getFile(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	client := server.Client()

	providerURL := fmt.Sprintf("%s/api/v4/projects/group%%2Fproject/packages/generic/my-package/v0.4.1/file.yaml", server.URL)
	providerConfig := config.NewProvider("test", providerURL, clusterctlv1.CoreProviderType)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		if r.URL.EscapedPath() == "/api/v4/projects/group%2Fproject/packages/generic/my-package/v0.4.1/file.yaml" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename=file.yaml")
			fmt.Fprint(w, "content")
			return
		}
		http.NotFound(w, r)
	})

	configVariablesClient := test.NewFakeVariableClient()

	tests := []struct {
		name     string
		version  string
		fileName string
		want     []byte
		wantErr  bool
	}{
		{
			name:     "Release and file exist",
			version:  "v0.4.1",
			fileName: "file.yaml",
			want:     []byte("content"),
			wantErr:  false,
		},
		{
			name:     "File does not exist",
			version:  "v0.4.1",
			fileName: "404.file",
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			resetCaches()

			gitLab, err := NewGitLabRepository(providerConfig, configVariablesClient)
			gitLab.(*gitLabRepository).httpClient = client
			g.Expect(err).ToNot(HaveOccurred())

			got, err := gitLab.GetFile(context.Background(), tt.version, tt.fileName)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
