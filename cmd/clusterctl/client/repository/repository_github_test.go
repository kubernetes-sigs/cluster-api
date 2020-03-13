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
	"reflect"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/google/go-github/github"
	"k8s.io/utils/pointer"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

//TODO: test newGitHubRepository

//TODO: test GetFile

//TODO: test getComponentsPath

func Test_gitHubRepository_getVersions(t *testing.T) {
	g := NewWithT(t)

	client, mux, teardown := test.NewFakeGitHub()
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
			want:    []string{"v0.4.0", "v0.4.1", "v0.4.2", "v0.4.3-alpha"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gitHub, err := newGitHubRepository(tt.field.providerConfig, configVariablesClient)
			g.Expect(err).NotTo(HaveOccurred())

			gitHub.injectClient = client

			got, err := gitHub.getVersions()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(ConsistOf(tt.want))
		})
	}
}

func Test_gitHubRepository_getLatestRelease(t *testing.T) {
	g := NewWithT(t)

	client, mux, teardown := test.NewFakeGitHub()
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
			want:    "v0.4.3-alpha", // prerelease/build releaese are considered as well
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
			gRepo, err := newGitHubRepository(tt.field.providerConfig, configVariablesClient)
			g.Expect(err).NotTo(HaveOccurred())

			gRepo.injectClient = client

			got, err := gRepo.getLatestRelease()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_gitHubRepository_getReleaseByTag(t *testing.T) {
	g := NewWithT(t)

	client, mux, teardown := test.NewFakeGitHub()
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
		wantTagName *string
		wantErr     bool
	}{
		{
			name: "Return existing version",
			args: args{
				tag: "foo",
			},
			wantTagName: pointer.StringPtr("v0.4.1"),
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
			gRepo, err := newGitHubRepository(providerConfig, configVariablesClient)
			g.Expect(err).NotTo(HaveOccurred())

			gRepo.injectClient = client

			got, err := gRepo.getReleaseByTag(tt.args.tag)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			if tt.wantTagName == nil {
				g.Expect(got).To(BeNil())
				return
			}

			g.Expect(got.TagName).To(Equal(tt.wantTagName))
		})
	}
}

func Test_gitHubRepository_downloadFilesFromRelease(t *testing.T) {
	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	providerConfig := config.NewProvider("test", "https://github.com/o/r/releases/v0.4.1/file.yaml", clusterctlv1.CoreProviderType) //tree/master/path not relevant for the test

	// test.NewFakeGitHub an handler for returning a fake release asset
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
				t.Fatal(err)
			}
			g.injectClient = client

			got, err := g.downloadFilesFromRelease(tt.args.release, tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got = %v, want %v", got, tt.want)
			}
		})
	}
}

func testMethod(t *testing.T, r *http.Request, want string) {
	if got := r.Method; got != want {
		t.Errorf("Request method: %v, want %v", got, want)
	}
}
