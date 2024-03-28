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
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v53/github"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/internal/goproxy"
	goproxytest "sigs.k8s.io/cluster-api/internal/goproxy/test"
)

func Test_gitHubRepository_GetVersions(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second

	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	// Setup an handler for returning 5 fake releases.
	mux.HandleFunc("/repos/o/r1/releases", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `[`)
		fmt.Fprint(w, `{"id":1, "tag_name": "v0.4.0"},`)
		fmt.Fprint(w, `{"id":2, "tag_name": "v0.4.1"},`)
		fmt.Fprint(w, `{"id":3, "tag_name": "v0.4.2"},`)
		fmt.Fprint(w, `{"id":4, "tag_name": "v0.4.3-alpha"}`) // Pre-release
		fmt.Fprint(w, `]`)
	})

	scheme, host, muxGoproxy, teardownGoproxy := goproxytest.NewFakeGoproxy()
	clientGoproxy := goproxy.NewClient(scheme, host)
	defer teardownGoproxy()

	// Setup a handler for returning 4 fake releases.
	muxGoproxy.HandleFunc("/github.com/o/r2/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v0.5.0\n")
		fmt.Fprint(w, "v0.4.0\n")
		fmt.Fprint(w, "v0.3.2\n")
		fmt.Fprint(w, "v0.3.1\n")
	})

	// Setup a handler for returning 3 different major fake releases.
	muxGoproxy.HandleFunc("/github.com/o/r3/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v1.0.0\n")
		fmt.Fprint(w, "v0.1.0\n")
	})
	muxGoproxy.HandleFunc("/github.com/o/r3/v2/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v2.0.0\n")
	})
	muxGoproxy.HandleFunc("/github.com/o/r3/v3/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v3.0.0\n")
	})

	configVariablesClient := test.NewFakeVariableClient()

	tests := []struct {
		name           string
		providerConfig config.Provider
		want           []string
		wantErr        bool
	}{
		{
			name:           "fallback to github",
			providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.0/path", clusterctlv1.CoreProviderType),
			want:           []string{"v0.4.0", "v0.4.1", "v0.4.2", "v0.4.3-alpha"},
			wantErr:        false,
		},
		{
			name:           "use goproxy",
			providerConfig: config.NewProvider("test", "https://github.com/o/r2/releases/v0.4.0/path", clusterctlv1.CoreProviderType),
			want:           []string{"v0.3.1", "v0.3.2", "v0.4.0", "v0.5.0"},
			wantErr:        false,
		},
		{
			name:           "use goproxy having multiple majors",
			providerConfig: config.NewProvider("test", "https://github.com/o/r3/releases/v3.0.0/path", clusterctlv1.CoreProviderType),
			want:           []string{"v0.1.0", "v1.0.0", "v2.0.0", "v3.0.0"},
			wantErr:        false,
		},
		{
			name:           "failure",
			providerConfig: config.NewProvider("test", "https://github.com/o/unknown/releases/v0.4.0/path", clusterctlv1.CoreProviderType),
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			resetCaches()

			gRepo, err := NewGitHubRepository(ctx, tt.providerConfig, configVariablesClient, injectGithubClient(client), injectGoproxyClient(clientGoproxy))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := gRepo.GetVersions(ctx)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_githubRepository_newGitHubRepository(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	type field struct {
		providerConfig config.Provider
		variableClient config.VariablesClient
	}
	tests := []struct {
		name    string
		field   field
		want    *gitHubRepository
		wantErr bool
	}{
		{
			name: "can create a new GitHub repo",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.1/path", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want: &gitHubRepository{
				providerConfig:           config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.1/path", clusterctlv1.CoreProviderType),
				configVariablesClient:    test.NewFakeVariableClient(),
				authenticatingHTTPClient: nil,
				owner:                    "o",
				repository:               "r1",
				defaultVersion:           "v0.4.1",
				rootPath:                 ".",
				componentsPath:           "path",
				injectClient:             nil,
			},
			wantErr: false,
		},
		{
			name: "missing variableClient",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.1/path", clusterctlv1.CoreProviderType),
				variableClient: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "provider url is not valid",
			field: field{
				providerConfig: config.NewProvider("test", "%gh&%ij", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "provider url should be in https",
			field: field{
				providerConfig: config.NewProvider("test", "http://github.com/blabla", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "provider url should be in github",
			field: field{
				providerConfig: config.NewProvider("test", "http://gitlab.com/blabla", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:    &gitHubRepository{},
			wantErr: true,
		},
		{
			name: "provider url should be in https://github.com/{owner}/{Repository}/%s/{latest|version-tag}/{componentsClient.yaml} format",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/dd/", clusterctlv1.CoreProviderType),
				variableClient: test.NewFakeVariableClient(),
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			resetCaches()

			gitHub, err := NewGitHubRepository(context.Background(), tt.field.providerConfig, tt.field.variableClient)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gitHub).To(Equal(tt.want))
		})
	}
}

func Test_githubRepository_getComponentsPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		rootPath string
		want     string
	}{
		{
			name:     "get the file name",
			path:     "github.com/o/r/releases/v0.4.1/file.yaml",
			rootPath: "github.com/o/r/releases/v0.4.1/",
			want:     "file.yaml",
		},
		{
			name:     "trim github.com",
			path:     "github.com/o/r/releases/v0.4.1/file.yaml",
			rootPath: "github.com",
			want:     "o/r/releases/v0.4.1/file.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			resetCaches()

			g.Expect(getComponentsPath(tt.path, tt.rootPath)).To(Equal(tt.want))
		})
	}
}

func Test_githubRepository_getFile(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	providerConfig := config.NewProvider("test", "https://github.com/o/r/releases/v0.4.1/file.yaml", clusterctlv1.CoreProviderType)

	// Setup a handler for returning a fake release.
	mux.HandleFunc("/repos/o/r/releases/tags/v0.4.1", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":13, "tag_name": "v0.4.1", "assets": [{"id": 1, "name": "file.yaml"}] }`)
	})

	// Setup a handler for returning a fake release asset.
	mux.HandleFunc("/repos/o/r/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=file.yaml")
		fmt.Fprint(w, "content")
	})

	configVariablesClient := test.NewFakeVariableClient()

	tests := []struct {
		name     string
		release  string
		fileName string
		want     []byte
		wantErr  bool
	}{
		{
			name:     "Release and file exist",
			release:  "v0.4.1",
			fileName: "file.yaml",
			want:     []byte("content"),
			wantErr:  false,
		},
		{
			name:     "Release does not exist",
			release:  "not-a-release",
			fileName: "file.yaml",
			want:     nil,
			wantErr:  true,
		},
		{
			name:     "File does not exist",
			release:  "v0.4.1",
			fileName: "404.file",
			want:     nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			resetCaches()

			gitHub, err := NewGitHubRepository(context.Background(), providerConfig, configVariablesClient, injectGithubClient(client))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := gitHub.GetFile(context.Background(), tt.release, tt.fileName)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_gitHubRepository_getVersions(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	// Setup a handler for returning fake releases in a paginated manner
	// Each response contains a link to the next page (if available) which
	// is parsed by the handler to navigate through all pages
	mux.HandleFunc("/repos/o/r1/releases", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			// Page 1
			w.Header().Set("Link", `<https://api.github.com/repositories/12345/releases?page=2>; rel="next"`) // Link to page 2
			fmt.Fprint(w, `[`)
			fmt.Fprint(w, `{"id":1, "tag_name": "v0.4.0"},`)
			fmt.Fprint(w, `{"id":2, "tag_name": "v0.4.1"}`)
			fmt.Fprint(w, `]`)
		case "2":
			// Page 2
			w.Header().Set("Link", `<https://api.github.com/repositories/12345/releases?page=3>; rel="next"`) // Link to page 3
			fmt.Fprint(w, `[`)
			fmt.Fprint(w, `{"id":3, "tag_name": "v0.4.2"},`)
			fmt.Fprint(w, `{"id":4, "tag_name": "v0.4.3-alpha"}`) // Pre-release
			fmt.Fprint(w, `]`)
		case "3":
			// Page 3 (last page)
			fmt.Fprint(w, `[`)
			fmt.Fprint(w, `{"id":4, "tag_name": "v0.4.4-beta"},`) // Pre-release
			fmt.Fprint(w, `{"id":5, "tag_name": "foo"}`)          // No semantic version tag
			fmt.Fprint(w, `]`)
		default:
			t.Fatalf("unexpected page requested")
		}
	})

	configVariablesClient := test.NewFakeVariableClient()

	type field struct {
		providerConfig config.Provider
	}
	tests := []struct {
		name    string
		field   field
		want    []string
		wantErr bool
	}{
		{
			name: "Get versions with all releases",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.1/path", clusterctlv1.CoreProviderType),
			},
			want:    []string{"v0.4.0", "v0.4.1", "v0.4.2", "v0.4.3-alpha", "v0.4.4-beta"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			resetCaches()

			gitHub, err := NewGitHubRepository(ctx, tt.field.providerConfig, configVariablesClient, injectGithubClient(client))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := gitHub.(*gitHubRepository).getVersions(ctx)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(ConsistOf(tt.want))
		})
	}
}

func Test_gitHubRepository_getLatestContractRelease(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	// Setup a handler for returning a fake release.
	mux.HandleFunc("/repos/o/r1/releases/tags/v0.5.0", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":13, "tag_name": "v0.5.0", "assets": [{"id": 1, "name": "metadata.yaml"}] }`)
	})

	mux.HandleFunc("/repos/o/r1/releases/tags/v0.3.2", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":14, "tag_name": "v0.3.2", "assets": [{"id": 2, "name": "metadata.yaml"}] }`)
	})

	// Setup a handler for returning a fake release metadata file.
	mux.HandleFunc("/repos/o/r1/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=metadata.yaml")
		fmt.Fprint(w, "apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3\nreleaseSeries:\n  - major: 0\n    minor: 4\n    contract: v1alpha4\n  - major: 0\n    minor: 5\n    contract: v1alpha4\n  - major: 0\n    minor: 3\n    contract: v1alpha3\n")
	})

	mux.HandleFunc("/repos/o/r1/releases/assets/2", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=metadata.yaml")
		fmt.Fprint(w, "apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3\nreleaseSeries:\n  - major: 0\n    minor: 4\n    contract: v1alpha4\n  - major: 0\n    minor: 5\n    contract: v1alpha4\n  - major: 0\n    minor: 3\n    contract: v1alpha3\n")
	})

	scheme, host, muxGoproxy, teardownGoproxy := goproxytest.NewFakeGoproxy()
	clientGoproxy := goproxy.NewClient(scheme, host)

	defer teardownGoproxy()

	// Setup a handler for returning 4 fake releases.
	muxGoproxy.HandleFunc("/github.com/o/r1/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v0.5.0\n")
		fmt.Fprint(w, "v0.4.0\n")
		fmt.Fprint(w, "v0.3.2\n")
		fmt.Fprint(w, "v0.3.1\n")
	})

	// setup an handler for returning 4 fake releases but no actual tagged release
	muxGoproxy.HandleFunc("/github.com/o/r2/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v0.5.0\n")
		fmt.Fprint(w, "v0.4.0\n")
		fmt.Fprint(w, "v0.3.2\n")
		fmt.Fprint(w, "v0.3.1\n")
	})

	configVariablesClient := test.NewFakeVariableClient()

	type field struct {
		providerConfig config.Provider
	}
	tests := []struct {
		name     string
		field    field
		contract string
		want     string
		wantErr  bool
	}{
		{
			name: "Get latest release if it matches the contract",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/latest/path", clusterctlv1.CoreProviderType),
			},
			contract: "v1alpha4",
			want:     "v0.5.0",
			wantErr:  false,
		},
		{
			name: "Get previous release if the latest doesn't match the contract",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/latest/path", clusterctlv1.CoreProviderType),
			},
			contract: "v1alpha3",
			want:     "v0.3.2",
			wantErr:  false,
		},
		{
			name: "Return the latest release if the contract doesn't exist",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/latest/path", clusterctlv1.CoreProviderType),
			},
			want:     "v0.5.0",
			contract: "foo",
			wantErr:  false,
		},
		{
			name: "Return 404 if there is no release for the tag",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r2/releases/v0.99.0/path", clusterctlv1.CoreProviderType),
			},
			want:     "0.99.0",
			contract: "v1alpha4",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			resetCaches()

			gRepo, err := NewGitHubRepository(context.Background(), tt.field.providerConfig, configVariablesClient, injectGithubClient(client), injectGoproxyClient(clientGoproxy))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := latestContractRelease(context.Background(), gRepo, tt.contract)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_gitHubRepository_getLatestRelease(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	scheme, host, muxGoproxy, teardownGoproxy := goproxytest.NewFakeGoproxy()
	clientGoproxy := goproxy.NewClient(scheme, host)
	defer teardownGoproxy()

	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	// Setup a handler for returning 4 fake releases.
	muxGoproxy.HandleFunc("/github.com/o/r1/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v0.4.1\n")
		fmt.Fprint(w, "v0.4.2\n")
		fmt.Fprint(w, "v0.4.3-alpha\n") // prerelease
		fmt.Fprint(w, "foo\n")          // no semantic version tag
	})
	// And also expose a release for them
	mux.HandleFunc("/repos/o/r1/releases/tags/v0.4.2", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":13, "tag_name": "v0.4.2", "assets": [{"id": 1, "name": "metadata.yaml"}] }`)
	})
	mux.HandleFunc("/repos/o/r3/releases/tags/v0.1.0-alpha.2", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":14, "tag_name": "v0.1.0-alpha.2", "assets": [{"id": 2, "name": "metadata.yaml"}] }`)
	})

	// Setup a handler for returning no releases.
	muxGoproxy.HandleFunc("/github.com/o/r2/@v/list", func(_ http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		// no releases
	})

	// Setup a handler for returning fake prereleases only.
	muxGoproxy.HandleFunc("/github.com/o/r3/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v0.1.0-alpha.0\n")
		fmt.Fprint(w, "v0.1.0-alpha.1\n")
		fmt.Fprint(w, "v0.1.0-alpha.2\n")
	})

	// Setup a handler for returning a fake release metadata file.
	mux.HandleFunc("/repos/o/r1/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=metadata.yaml")
		fmt.Fprint(w, "apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3\nreleaseSeries:\n  - major: 0\n    minor: 4\n    contract: v1alpha4\n  - major: 0\n    minor: 5\n    contract: v1alpha4\n  - major: 0\n    minor: 3\n    contract: v1alpha3\n")
	})

	mux.HandleFunc("/repos/o/r3/releases/assets/2", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=metadata.yaml")
		fmt.Fprint(w, "apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3\nreleaseSeries:\n  - major: 0\n    minor: 4\n    contract: v1alpha4\n  - major: 0\n    minor: 5\n    contract: v1alpha4\n  - major: 0\n    minor: 3\n    contract: v1alpha3\n")
	})

	configVariablesClient := test.NewFakeVariableClient()

	type field struct {
		providerConfig config.Provider
	}
	tests := []struct {
		name    string
		field   field
		want    string
		wantErr bool
	}{
		{
			name: "Get latest release, ignores pre-release version",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.2/path", clusterctlv1.CoreProviderType),
			},
			want:    "v0.4.2",
			wantErr: false,
		},
		{
			name: "Fails, when no release found",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r2/releases/v0.4.1/path", clusterctlv1.CoreProviderType),
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Falls back to latest prerelease when no official release present",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r3/releases/v0.1.0-alpha.2/path", clusterctlv1.CoreProviderType),
			},
			want:    "v0.1.0-alpha.2",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			resetCaches()

			gRepo, err := NewGitHubRepository(ctx, tt.field.providerConfig, configVariablesClient, injectGoproxyClient(clientGoproxy), injectGithubClient(client))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := latestRelease(ctx, gRepo)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
			g.Expect(gRepo.(*gitHubRepository).defaultVersion).To(Equal(tt.want))
		})
	}
}

func Test_gitHubRepository_getLatestPatchRelease(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	scheme, host, muxGoproxy, teardownGoproxy := goproxytest.NewFakeGoproxy()
	clientGoproxy := goproxy.NewClient(scheme, host)
	defer teardownGoproxy()

	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	// Setup a handler for returning 4 fake releases.
	muxGoproxy.HandleFunc("/github.com/o/r1/@v/list", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, "v0.4.0\n")
		fmt.Fprint(w, "v0.3.2\n")
		fmt.Fprint(w, "v1.3.2\n")
	})

	// Setup a handler for returning a fake release.
	mux.HandleFunc("/repos/o/r1/releases/tags/v0.4.0", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":13, "tag_name": "v0.4.0", "assets": [{"id": 1, "name": "metadata.yaml"}] }`)
	})

	mux.HandleFunc("/repos/o/r1/releases/tags/v0.3.2", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":14, "tag_name": "v0.3.2", "assets": [{"id": 1, "name": "metadata.yaml"}] }`)
	})

	mux.HandleFunc("/repos/o/r1/releases/tags/v1.3.2", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":15, "tag_name": "v1.3.2", "assets": [{"id": 1, "name": "metadata.yaml"}] }`)
	})

	// Setup a handler for returning a fake release metadata file.
	mux.HandleFunc("/repos/o/r1/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=metadata.yaml")
		fmt.Fprint(w, "apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3\nreleaseSeries:\n  - major: 0\n    minor: 4\n    contract: v1alpha4\n  - major: 0\n    minor: 5\n    contract: v1alpha4\n  - major: 0\n    minor: 3\n    contract: v1alpha3\n")
	})

	major0 := uint(0)
	minor3 := uint(3)
	minor4 := uint(4)

	configVariablesClient := test.NewFakeVariableClient()

	type field struct {
		providerConfig config.Provider
	}
	tests := []struct {
		name    string
		field   field
		major   *uint
		minor   *uint
		want    string
		wantErr bool
	}{
		{
			name: "Get latest patch release, no Major/Minor specified",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v1.3.2/path", clusterctlv1.CoreProviderType),
			},
			minor:   nil,
			major:   nil,
			want:    "v1.3.2",
			wantErr: false,
		},
		{
			name: "Get latest patch release, for Major 0 and Minor 3",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.3.2/path", clusterctlv1.CoreProviderType),
			},
			major:   &major0,
			minor:   &minor3,
			want:    "v0.3.2",
			wantErr: false,
		},
		{
			name: "Get latest patch release, for Major 0 and Minor 4",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.0/path", clusterctlv1.CoreProviderType),
			},
			major:   &major0,
			minor:   &minor4,
			want:    "v0.4.0",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			resetCaches()

			gRepo, err := NewGitHubRepository(ctx, tt.field.providerConfig, configVariablesClient, injectGoproxyClient(clientGoproxy), injectGithubClient(client))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := latestPatchRelease(ctx, gRepo, tt.major, tt.minor)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_gitHubRepository_getReleaseByTag(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	providerConfig := config.NewProvider("test", "https://github.com/o/r/releases/v0.4.1/path", clusterctlv1.CoreProviderType)

	// Setup a handler for returning a fake release.
	mux.HandleFunc("/repos/o/r/releases/tags/foo", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":13, "tag_name": "v0.4.1"}`)
	})

	configVariablesClient := test.NewFakeVariableClient()

	type args struct {
		tag string
	}
	tests := []struct {
		name        string
		args        args
		wantTagName *string
		wantErr     bool
	}{
		{
			name: "Return existing version",
			args: args{
				tag: "foo",
			},
			wantTagName: ptr.To("v0.4.1"),
			wantErr:     false,
		},
		{
			name: "Fails if version does not exists",
			args: args{
				tag: "bar",
			},
			wantTagName: nil,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			resetCaches()

			gRepo, err := NewGitHubRepository(ctx, providerConfig, configVariablesClient, injectGithubClient(client))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := gRepo.(*gitHubRepository).getReleaseByTag(ctx, tt.args.tag)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			if tt.wantTagName == nil {
				g.Expect(got).To(BeNil())
				return
			}

			g.Expect(got.TagName).To(Equal(tt.wantTagName))
		})
	}
}

func Test_gitHubRepository_downloadFilesFromRelease(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second
	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	providerConfig := config.NewProvider("test", "https://github.com/o/r/releases/v0.4.1/file.yaml", clusterctlv1.CoreProviderType)                           // tree/main/path not relevant for the test
	providerConfigWithRedirect := config.NewProvider("test", "https://github.com/o/r-with-redirect/releases/v0.4.1/file.yaml", clusterctlv1.CoreProviderType) // tree/main/path not relevant for the test

	// Setup a handler for returning a fake release asset.
	mux.HandleFunc("/repos/o/r/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=file.yaml")
		fmt.Fprint(w, "content")
	})
	// Setup a handler which redirects to a different location.
	mux.HandleFunc("/repos/o/r-with-redirect/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
		goproxytest.HTTPTestMethod(t, r, "GET")
		http.Redirect(w, r, "/api-v3/repos/o/r/releases/assets/1", http.StatusFound)
	})

	configVariablesClient := test.NewFakeVariableClient()

	var id1 int64 = 1
	var id2 int64 = 2
	tagName := "vO.3.3"
	file := "file.yaml"

	type args struct {
		release  *github.RepositoryRelease
		fileName string
	}
	tests := []struct {
		name           string
		args           args
		providerConfig config.Provider
		want           []byte
		wantErr        bool
	}{
		{
			name: "Pass if file exists",
			args: args{
				release: &github.RepositoryRelease{
					TagName: &tagName,
					Assets: []*github.ReleaseAsset{
						{
							ID:   &id1,
							Name: &file,
						},
					},
				},
				fileName: file,
			},
			providerConfig: providerConfig,
			want:           []byte("content"),
			wantErr:        false,
		},
		{
			name: "Pass if file exists with redirect",
			args: args{
				release: &github.RepositoryRelease{
					TagName: &tagName,
					Assets: []*github.ReleaseAsset{
						{
							ID:   &id1,
							Name: &file,
						},
					},
				},
				fileName: file,
			},
			providerConfig: providerConfigWithRedirect,
			want:           []byte("content"),
			wantErr:        false,
		},
		{
			name: "Fails if file does not exists",
			args: args{
				release: &github.RepositoryRelease{
					TagName: &tagName,
					Assets: []*github.ReleaseAsset{
						{
							ID:   &id1,
							Name: &file,
						},
					},
				},
				fileName: "another file",
			},
			providerConfig: providerConfig,
			wantErr:        true,
		},
		{
			name: "Fails if file does not exists",
			args: args{
				release: &github.RepositoryRelease{
					TagName: &tagName,
					Assets: []*github.ReleaseAsset{
						{
							ID:   &id2, // id does not match any file (this should not happen)
							Name: &file,
						},
					},
				},
				fileName: "another file",
			},
			providerConfig: providerConfig,
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			resetCaches()

			gRepo, err := NewGitHubRepository(context.Background(), tt.providerConfig, configVariablesClient, injectGithubClient(client))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := gRepo.(*gitHubRepository).downloadFilesFromRelease(context.Background(), tt.args.release, tt.args.fileName)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

// resetCaches is called repeatedly throughout tests to help avoid cross-test pollution.
func resetCaches() {
	cacheVersions = map[string][]string{}
	cacheReleases = map[string]*github.RepositoryRelease{}
	cacheFiles = map[string][]byte{}
}

func Test_gitHubRepository_releaseNotFound(t *testing.T) {
	retryableOperationInterval = 200 * time.Millisecond
	retryableOperationTimeout = 1 * time.Second

	tests := []struct {
		name        string
		releaseTags []string
		ghReleases  []string
		want        string
		wantErr     bool
	}{
		{
			name:        "One release",
			releaseTags: []string{"v0.4.2"},
			ghReleases:  []string{"v0.4.2"},
			want:        "v0.4.2",
			wantErr:     false,
		},
		{
			name:        "Latest tag without a release",
			releaseTags: []string{"v0.5.0", "v0.4.2"},
			ghReleases:  []string{"v0.4.2"},
			want:        "v0.4.2",
			wantErr:     false,
		},
		{
			name:        "Two tags without releases",
			releaseTags: []string{"v0.6.0", "v0.5.0", "v0.4.2"},
			ghReleases:  []string{"v0.4.2"},
			want:        "v0.4.2",
			wantErr:     false,
		},
		{
			name:        "Five tags without releases",
			releaseTags: []string{"v0.9.0", "v0.8.0", "v0.7.0", "v0.6.0", "v0.5.0", "v0.4.2"},
			ghReleases:  []string{"v0.4.2"},
			wantErr:     true,
		},
		{
			name:        "Pre-releases have lower priority",
			releaseTags: []string{"v0.7.0-alpha", "v0.6.0-alpha", "v0.5.0-alpha", "v0.4.2"},
			ghReleases:  []string{"v0.4.2"},
			want:        "v0.4.2",
			wantErr:     false,
		},
		{
			name:        "Two Github releases",
			releaseTags: []string{"v0.7.0", "v0.6.0", "v0.5.0", "v0.4.2"},
			ghReleases:  []string{"v0.5.0", "v0.4.2"},
			want:        "v0.5.0",
			wantErr:     false,
		},
		{
			name:        "Github release and prerelease",
			releaseTags: []string{"v0.6.0", "v0.5.0-alpha", "v0.4.2"},
			ghReleases:  []string{"v0.5.0-alpha", "v0.4.2"},
			want:        "v0.4.2",
			wantErr:     false,
		},
		{
			name:        "No Github releases",
			releaseTags: []string{"v0.6.0", "v0.5.0", "v0.4.2"},
			ghReleases:  []string{},
			wantErr:     true,
		},
		{
			name:        "Pre-releases only",
			releaseTags: []string{"v0.6.0-alpha", "v0.5.0-alpha", "v0.4.2-alpha"},
			ghReleases:  []string{"v0.5.0-alpha"},
			want:        "v0.5.0-alpha",
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			configVariablesClient := test.NewFakeVariableClient()

			resetCaches()

			client, mux, teardown := test.NewFakeGitHub()
			defer teardown()

			providerConfig := config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.1/file.yaml", clusterctlv1.CoreProviderType)

			scheme, host, muxGoproxy, teardownGoproxy := goproxytest.NewFakeGoproxy()
			clientGoproxy := goproxy.NewClient(scheme, host)

			defer teardownGoproxy()

			// First, register tags within goproxy.
			muxGoproxy.HandleFunc("/github.com/o/r1/@v/list", func(w http.ResponseWriter, r *http.Request) {
				goproxytest.HTTPTestMethod(t, r, "GET")
				for _, release := range tt.releaseTags {
					fmt.Fprint(w, release+"\n")
				}
			})

			// Second, register releases in GitHub.
			for _, release := range tt.ghReleases {
				mux.HandleFunc(fmt.Sprintf("/repos/o/r1/releases/tags/%s", release), func(w http.ResponseWriter, r *http.Request) {
					goproxytest.HTTPTestMethod(t, r, "GET")
					parts := strings.Split(r.RequestURI, "/")
					version := parts[len(parts)-1]
					fmt.Fprintf(w, "{\"id\":13, \"tag_name\": %q, \"assets\": [{\"id\": 1, \"name\": \"metadata.yaml\"}] }", version)
				})
			}

			// Third, setup a handler for returning a fake release metadata file.
			mux.HandleFunc("/repos/o/r1/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
				goproxytest.HTTPTestMethod(t, r, "GET")
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", "attachment; filename=metadata.yaml")
				fmt.Fprint(w, "apiVersion: clusterctl.cluster.x-k8s.io/v1alpha3\nreleaseSeries:\n  - major: 0\n    minor: 4\n    contract: v1alpha4\n  - major: 0\n    minor: 5\n    contract: v1alpha4\n  - major: 0\n    minor: 3\n    contract: v1alpha3\n")
			})

			gRepo, err := NewGitHubRepository(ctx, providerConfig, configVariablesClient, injectGithubClient(client), injectGoproxyClient(clientGoproxy))
			g.Expect(err).ToNot(HaveOccurred())

			got, err := latestRelease(ctx, gRepo)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
