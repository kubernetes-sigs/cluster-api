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

package cluster

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v53/github"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/config"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	yaml "sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

var template = `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine`

func Test_templateClient_GetFromConfigMap(t *testing.T) {
	g := NewWithT(t)

	configClient, err := config.New(context.Background(), "", config.InjectReader(test.NewFakeReader()))
	g.Expect(err).ToNot(HaveOccurred())

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "my-template",
		},
		Data: map[string]string{
			"prod": template,
		},
	}

	type fields struct {
		proxy        Proxy
		configClient config.Client
	}
	type args struct {
		configMapNamespace  string
		configMapName       string
		configMapDataKey    string
		targetNamespace     string
		skipTemplateProcess bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Return template",
			fields: fields{
				proxy:        test.NewFakeProxy().WithObjs(configMap),
				configClient: configClient,
			},
			args: args{
				configMapNamespace:  "ns1",
				configMapName:       "my-template",
				configMapDataKey:    "prod",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    template,
			wantErr: false,
		},
		{
			name: "Config map does not exists",
			fields: fields{
				proxy:        test.NewFakeProxy().WithObjs(configMap),
				configClient: configClient,
			},
			args: args{
				configMapNamespace:  "ns1",
				configMapName:       "something-else",
				configMapDataKey:    "prod",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Config map key does not exists",
			fields: fields{
				proxy:        test.NewFakeProxy().WithObjs(configMap),
				configClient: configClient,
			},
			args: args{
				configMapNamespace:  "ns1",
				configMapName:       "my-template",
				configMapDataKey:    "something-else",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			processor := yaml.NewSimpleProcessor()
			tc := newTemplateClient(TemplateClientInput{tt.fields.proxy, tt.fields.configClient, processor})
			got, err := tc.GetFromConfigMap(ctx, tt.args.configMapNamespace, tt.args.configMapName, tt.args.configMapDataKey, tt.args.targetNamespace, tt.args.skipTemplateProcess)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			wantTemplate, err := repository.NewTemplate(repository.TemplateInput{
				RawArtifact:           []byte(tt.want),
				ConfigVariablesClient: configClient.Variables(),
				Processor:             processor,
				TargetNamespace:       tt.args.targetNamespace,
				SkipTemplateProcess:   tt.args.skipTemplateProcess,
			})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(wantTemplate))
		})
	}
}

func Test_templateClient_getGitHubFileContent(t *testing.T) {
	g := NewWithT(t)

	client, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	configClient, err := config.New(context.Background(), "", config.InjectReader(test.NewFakeReader()))
	g.Expect(err).ToNot(HaveOccurred())

	mux.HandleFunc("/repos/kubernetes-sigs/cluster-api/contents/config/default/cluster-template.yaml", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
		  "type": "file",
		  "encoding": "base64",
		  "content": "`+base64.StdEncoding.EncodeToString([]byte(template))+`",
		  "sha": "f5f369044773ff9c6383c087466d12adb6fa0828",
		  "size": 12,
		  "name": "cluster-template.yaml",
		  "path": "config/default/cluster-template.yaml"
		}`)
	})

	type args struct {
		rURL *url.URL
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Return custom template",
			args: args{
				rURL: mustParseURL("https://github.com/kubernetes-sigs/cluster-api/blob/main/config/default/cluster-template.yaml"),
			},
			want:    []byte(template),
			wantErr: false,
		},
		{
			name: "Wrong url",
			args: args{
				rURL: mustParseURL("https://github.com/kubernetes-sigs/cluster-api/blob/main/config/default/something-else.yaml"),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			c := &templateClient{
				configClient: configClient,
				gitHubClientFactory: func(context.Context, config.VariablesClient) (*github.Client, error) {
					return client, nil
				},
			}
			got, err := c.getGitHubFileContent(ctx, tt.args.rURL)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_templateClient_getRawUrlFileContent(t *testing.T) {
	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, template)
	}))

	defer fakeServer.Close()

	type args struct {
		rURL string
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Return custom template",
			args: args{
				rURL: fakeServer.URL,
			},
			want:    []byte(template),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			c := newTemplateClient(TemplateClientInput{})
			got, err := c.getRawURLFileContent(ctx, tt.args.rURL)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_templateClient_getLocalFileContent(t *testing.T) {
	g := NewWithT(t)

	tmpDir, err := os.MkdirTemp("", "cc")
	g.Expect(err).ToNot(HaveOccurred())
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "cluster-template.yaml")
	g.Expect(os.WriteFile(path, []byte(template), 0600)).To(Succeed())

	type args struct {
		rURL *url.URL
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Return custom template",
			args: args{
				rURL: mustParseURL(path),
			},
			want:    []byte(template),
			wantErr: false,
		},
		{
			name: "Wrong path",
			args: args{
				rURL: mustParseURL(filepath.Join(tmpDir, "something-else.yaml")),
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := &templateClient{}
			got, err := c.getLocalFileContent(tt.args.rURL)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_templateClient_GetFromURL(t *testing.T) {
	g := NewWithT(t)

	tmpDir, err := os.MkdirTemp("", "cc")
	g.Expect(err).ToNot(HaveOccurred())
	defer os.RemoveAll(tmpDir)

	configClient, err := config.New(context.Background(), "", config.InjectReader(test.NewFakeReader()))
	g.Expect(err).ToNot(HaveOccurred())

	fakeGithubClient, mux, teardown := test.NewFakeGitHub()
	defer teardown()

	mux.HandleFunc("/repos/kubernetes-sigs/cluster-api/contents/config/default/cluster-template.yaml", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
		  "type": "file",
		  "encoding": "base64",
		  "content": "`+base64.StdEncoding.EncodeToString([]byte(template))+`",
		  "sha": "f5f369044773ff9c6383c087466d12adb6fa0828",
		  "size": 12,
		  "name": "cluster-template.yaml",
		  "path": "config/default/cluster-template.yaml"
		}`)
	})

	mux.HandleFunc("/repos/some-owner/some-repo/releases/tags/v1.0.0", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
		  "tag_name": "v1.0.0",
		  "name": "v1.0.0",
		  "id": 12345678,
		  "url": "https://api.github.com/repos/some-owner/some-repo/releases/12345678",
		  "assets": [
			{
			  "id": 87654321,
			  "name": "cluster-template.yaml"
			}
		  ]
		}`)
	})

	mux.HandleFunc("/repos/some-owner/some-repo/releases/assets/87654321", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, template)
	})

	mux.HandleFunc("/repos/some-owner/some-repo/releases/tags/v2.0.0", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{
		  "tag_name": "v2.0.0",
		  "name": "v2.0.0",
		  "id": 12345678,
		  "url": "https://api.github.com/repos/some-owner/some-repo/releases/12345678",
		  "assets": [
			{
			  "id": 22222222,
			  "name": "cluster-template.yaml"
			}
		  ]
		}`)
	})

	// redirect asset
	mux.HandleFunc("/repos/some-owner/some-repo/releases/assets/22222222", func(w http.ResponseWriter, _ *http.Request) {
		// add the "/api-v3" prefix to match the prefix of the fake github server
		w.Header().Add("Location", "/api-v3/redirected/22222222")
		w.WriteHeader(http.StatusFound)
	})

	// redirect location
	mux.HandleFunc("/redirected/22222222", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, template)
	})

	path := filepath.Join(tmpDir, "cluster-template.yaml")
	g.Expect(os.WriteFile(path, []byte(template), 0600)).To(Succeed())

	// redirect stdin
	saveStdin := os.Stdin
	defer func() { os.Stdin = saveStdin }()
	os.Stdin, err = os.Open(path) //nolint:gosec
	g.Expect(err).ToNot(HaveOccurred())

	type args struct {
		templateURL         string
		targetNamespace     string
		skipTemplateProcess bool
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Get from local file system",
			args: args{
				templateURL:         path,
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    template,
			wantErr: false,
		},
		{
			name: "Get from GitHub",
			args: args{
				templateURL:         "https://github.com/kubernetes-sigs/cluster-api/blob/main/config/default/cluster-template.yaml",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    template,
			wantErr: false,
		},
		{
			name: "Get asset from GitHub release",
			args: args{
				templateURL:         "https://github.com/some-owner/some-repo/releases/download/v1.0.0/cluster-template.yaml",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    template,
			wantErr: false,
		},
		{
			name: "Get asset from GitHub release + redirect",
			args: args{
				templateURL:         "https://github.com/some-owner/some-repo/releases/download/v2.0.0/cluster-template.yaml",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    template,
			wantErr: false,
		},
		{
			name: "Get asset from GitHub release with a wrong URL",
			args: args{
				templateURL:         "https://github.com/some-owner/some-repo/releases/wrong/v1.0.0/cluster-template.yaml",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Get from stdin",
			args: args{
				templateURL:         "-",
				targetNamespace:     "",
				skipTemplateProcess: false,
			},
			want:    template,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			gitHubClientFactory := func(context.Context, config.VariablesClient) (*github.Client, error) {
				return fakeGithubClient, nil
			}
			processor := yaml.NewSimpleProcessor()
			c := newTemplateClient(TemplateClientInput{nil, configClient, processor})
			// override the github client factory
			c.gitHubClientFactory = gitHubClientFactory

			got, err := c.GetFromURL(ctx, tt.args.templateURL, tt.args.targetNamespace, tt.args.skipTemplateProcess)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			wantTemplate, err := repository.NewTemplate(repository.TemplateInput{
				RawArtifact:           []byte(tt.want),
				ConfigVariablesClient: configClient.Variables(),
				Processor:             processor,
				TargetNamespace:       tt.args.targetNamespace,
				SkipTemplateProcess:   tt.args.skipTemplateProcess,
			})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(wantTemplate))
		})
	}
}

func mustParseURL(rawURL string) *url.URL {
	rURL, err := url.Parse(rawURL)
	if err != nil {
		panic(err)
	}
	return rURL
}
