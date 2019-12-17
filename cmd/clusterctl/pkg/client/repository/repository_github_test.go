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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/google/go-github/github"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

//TODO: test newGitHubRepository

//TODO: test GetFile

//TODO: test getComponentsPath

func Test_gitHubRepository_getVersions(t *testing.T) {
	client, mux, teardown := setup()
	defer teardown()

	// setup an handler for returning 5 fake releases
	mux.HandleFunc("/repos/o/r1/releases", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `[`)
		fmt.Fprint(w, `{"id":1, "tag_name": "v0.4.0"},`)
		fmt.Fprint(w, `{"id":2, "tag_name": "v0.4.1"},`)
		fmt.Fprint(w, `{"id":3, "tag_name": "v0.4.2"},`)
		fmt.Fprint(w, `{"id":4, "tag_name": "v0.4.3-alpha"},`) // prerelease
		fmt.Fprint(w, `{"id":5, "tag_name": "foo"}`)           // no semantic version tag
		fmt.Fprint(w, `]`)
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
			name: "Get versions",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.1/path", clusterctlv1.CoreProviderType),
			},
			want:    []string{"v0.4.0", "v0.4.1", "v0.4.2"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := newGitHubRepository(tt.field.providerConfig, configVariablesClient)
			if err != nil {
				t.Fatalf("newGitHubRepository() error = %v", err)
			}
			g.injectClient = client

			got, err := g.getVersions()
			if (err != nil) != tt.wantErr {
				t.Errorf("getVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("getVersions() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_gitHubRepository_getLatestRelease(t *testing.T) {
	client, mux, teardown := setup()
	defer teardown()

	// setup an handler for returning 4 fake releases
	mux.HandleFunc("/repos/o/r1/releases", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `[`)
		fmt.Fprint(w, `{"id":1, "tag_name": "v0.4.1"},`)
		fmt.Fprint(w, `{"id":2, "tag_name": "v0.4.2"},`)
		fmt.Fprint(w, `{"id":3, "tag_name": "v0.4.3-alpha"},`) // prerelease
		fmt.Fprint(w, `{"id":4, "tag_name": "foo"}`)           // no semantic version tag
		fmt.Fprint(w, `]`)
	})

	// setup an handler for returning no releases
	mux.HandleFunc("/repos/o/r2/releases", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		//no releases
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
			name: "Get release v0.4.1",
			field: field{
				providerConfig: config.NewProvider("test", "https://github.com/o/r1/releases/v0.4.1/path", clusterctlv1.CoreProviderType),
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := newGitHubRepository(tt.field.providerConfig, configVariablesClient)
			if err != nil {
				t.Fatalf("newGitHubRepository() error = %v", err)
			}
			g.injectClient = client

			got, err := g.getLatestRelease()
			if (err != nil) != tt.wantErr {
				t.Errorf("getLatestRelease() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getLatestRelease() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_gitHubRepository_getReleaseByTag(t *testing.T) {
	client, mux, teardown := setup()
	defer teardown()

	providerConfig := config.NewProvider("test", "https://github.com/o/r/releases/v0.4.1/path", clusterctlv1.CoreProviderType)

	// setup and handler for returning a fake release
	mux.HandleFunc("/repos/o/r/releases/tags/foo", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		fmt.Fprint(w, `{"id":13, "tag_name": "v0.4.1"}`)
	})

	configVariablesClient := test.NewFakeVariableClient()

	type args struct {
		tag string
	}
	tests := []struct {
		name        string
		args        args
		wantTagName string
		wantErr     bool
	}{
		{
			name: "Return existing version",
			args: args{
				tag: "foo",
			},
			wantTagName: "v0.4.1",
			wantErr:     false,
		},
		{
			name: "Fails if version does not exists",
			args: args{
				tag: "bar",
			},
			wantTagName: "",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := newGitHubRepository(providerConfig, configVariablesClient)
			if err != nil {
				t.Fatalf("newGitHubRepository() error = %v", err)
			}
			g.injectClient = client

			got, err := g.getReleaseByTag(tt.args.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("getReleaseByTag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantTagName == "" && got == nil {
				return
			}

			if tt.wantTagName != "" && got != nil {
				if *got.TagName != tt.wantTagName {
					t.Errorf("getReleaseByTag().TagName got = %v, want %v", *got.TagName, tt.wantTagName)
				}
				return
			}

			t.Errorf("getReleaseByTag() got = %v, want.TagName %v", got, tt.wantTagName)
		})
	}
}

func Test_gitHubRepository_downloadFilesFromRelease(t *testing.T) {
	client, mux, teardown := setup()
	defer teardown()

	providerConfig := config.NewProvider("test", "https://github.com/o/r/releases/v0.4.1/file.yaml", clusterctlv1.CoreProviderType) //tree/master/path not relevant for the test

	// setup an handler for returning a fake release asset
	mux.HandleFunc("/repos/o/r/releases/assets/1", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=file.yaml")
		fmt.Fprint(w, "content")
	})

	configVariablesClient := test.NewFakeVariableClient()

	var id1 int64 = 1
	var id2 int64 = 2
	tagName := "file.yaml"
	file := "file.yaml"

	type args struct {
		release  *github.RepositoryRelease
		fileName string
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Pass if file exists",
			args: args{
				release: &github.RepositoryRelease{
					TagName: &tagName,
					Assets: []github.ReleaseAsset{
						{
							ID:   &id1,
							Name: &file,
						},
					},
				},
				fileName: file,
			},
			want:    []byte("content"),
			wantErr: false,
		},
		{
			name: "Fails if file does not exists",
			args: args{
				release: &github.RepositoryRelease{
					TagName: &tagName,
					Assets: []github.ReleaseAsset{
						{
							ID:   &id1,
							Name: &file,
						},
					},
				},
				fileName: "another file",
			},
			wantErr: true,
		},
		{
			name: "Fails if file does not exists",
			args: args{
				release: &github.RepositoryRelease{
					TagName: &tagName,
					Assets: []github.ReleaseAsset{
						{
							ID:   &id2, //id does not match any file (this should not happen)
							Name: &file,
						},
					},
				},
				fileName: file,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := newGitHubRepository(providerConfig, configVariablesClient)
			if err != nil {
				t.Fatalf("newGitHubRepository() error = %v", err)
			}
			g.injectClient = client

			got, err := g.downloadFilesFromRelease(tt.args.release, tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("downloadFilesFromRelease() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("downloadFilesFromRelease() got = %v, want %v", got, tt.want)
			}
		})
	}
}

const baseURLPath = "/api-v3"

// setup sets up a test HTTP server along with a github.Client that is
// configured to talk to that test server. Tests should register handlers on
// mux which provide mock responses for the API method being tested.
func setup() (client *github.Client, mux *http.ServeMux, teardown func()) {
	// mux is the HTTP request multiplexer used with the test server.
	mux = http.NewServeMux()

	apiHandler := http.NewServeMux()
	apiHandler.Handle(baseURLPath+"/", http.StripPrefix(baseURLPath, mux))

	// server is a test HTTP server used to provide mock API responses.
	server := httptest.NewServer(apiHandler)

	// client is the GitHub client being tested and is configured to use test server.
	client = github.NewClient(nil)
	url, _ := url.Parse(server.URL + baseURLPath + "/")
	client.BaseURL = url
	client.UploadURL = url

	return client, mux, server.Close
}

func testMethod(t *testing.T, r *http.Request, want string) {
	if got := r.Method; got != want {
		t.Errorf("Request method: %v, want %v", got, want)
	}
}
