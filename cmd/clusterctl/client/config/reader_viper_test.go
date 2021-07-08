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

package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_viperReader_Init(t *testing.T) {
	g := NewWithT(t)

	// Change HOME dir and do not specify config file
	// (.cluster-api/clusterctl) in it.
	clusterctlHomeDir, err := os.MkdirTemp("", "clusterctl-default")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(clusterctlHomeDir)

	dir, err := os.MkdirTemp("", "clusterctl")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "clusterctl.yaml")
	g.Expect(os.WriteFile(configFile, []byte("bar: bar"), 0600)).To(Succeed())

	configFileBadContents := filepath.Join(dir, "clusterctl-bad.yaml")
	g.Expect(os.WriteFile(configFileBadContents, []byte("bad-contents"), 0600)).To(Succeed())

	// To test the remote config file
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, err := w.Write([]byte("bar: bar"))
		g.Expect(err).NotTo(HaveOccurred())
	}))
	defer ts.Close()

	// To test the remote config file when fails to fetch
	tsFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer tsFail.Close()

	tests := []struct {
		name       string
		configPath string
		configDirs []string
		expectErr  bool
	}{
		{
			name:       "reads in config successfully",
			configPath: configFile,
			configDirs: []string{clusterctlHomeDir},
			expectErr:  false,
		},
		{
			name:       "returns error for invalid config file path",
			configPath: "do-not-exist.yaml",
			configDirs: []string{clusterctlHomeDir},
			expectErr:  true,
		},
		{
			name:       "does not return error if default file doesn't exist",
			configPath: "",
			configDirs: []string{clusterctlHomeDir},
			expectErr:  false,
		},
		{
			name:       "returns error for malformed config",
			configPath: configFileBadContents,
			configDirs: []string{clusterctlHomeDir},
			expectErr:  true,
		},
		{
			name:       "reads in config from remote successfully",
			configPath: ts.URL,
			configDirs: []string{clusterctlHomeDir},
			expectErr:  false,
		},
		{
			name:       "fail to read remote config",
			configPath: tsFail.URL,
			configDirs: []string{clusterctlHomeDir},
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gg := NewWithT(t)
			v := newViperReader(injectConfigPaths(tt.configDirs))
			if tt.expectErr {
				gg.Expect(v.Init(tt.configPath)).ToNot(Succeed())
				return
			}
			gg.Expect(v.Init(tt.configPath)).To(Succeed())
		})
	}
}

func Test_viperReader_Get(t *testing.T) {
	g := NewWithT(t)

	dir, err := os.MkdirTemp("", "clusterctl")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(dir)

	_ = os.Setenv("FOO", "foo")

	configFile := filepath.Join(dir, "clusterctl.yaml")
	g.Expect(os.WriteFile(configFile, []byte("bar: bar"), 0600)).To(Succeed())

	type args struct {
		key string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Read from env",
			args: args{
				key: "FOO",
			},
			want:    "foo",
			wantErr: false,
		},
		{
			name: "Read from file",
			args: args{
				key: "BAR",
			},
			want:    "bar",
			wantErr: false,
		},
		{
			name: "Fails if missing",
			args: args{
				key: "BAZ",
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			v := newViperReader(injectConfigPaths([]string{dir}))

			gs.Expect(v.Init(configFile)).To(Succeed())

			got, err := v.Get(tt.args.key)
			if tt.wantErr {
				gs.Expect(err).To(HaveOccurred())
				return
			}

			gs.Expect(err).NotTo(HaveOccurred())
			gs.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_viperReader_GetWithoutDefaultConfig(t *testing.T) {
	g := NewWithT(t)
	dir, err := os.MkdirTemp("", "clusterctl")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(dir)

	_ = os.Setenv("FOO_FOO", "bar")

	v := newViperReader(injectConfigPaths([]string{dir}))
	g.Expect(v.Init("")).To(Succeed())

	got, err := v.Get("FOO_FOO")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).To(Equal("bar"))
}

func Test_viperReader_Set(t *testing.T) {
	g := NewWithT(t)

	dir, err := os.MkdirTemp("", "clusterctl")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(dir)

	_ = os.Setenv("FOO", "foo")

	configFile := filepath.Join(dir, "clusterctl.yaml")

	g.Expect(os.WriteFile(configFile, []byte("bar: bar"), 0600)).To(Succeed())

	type args struct {
		key   string
		value string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "",
			args: args{
				key:   "FOO",
				value: "bar",
			},
			want: "bar",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			v := &viperReader{}

			gs.Expect(v.Init(configFile)).To(Succeed())

			v.Set(tt.args.key, tt.args.value)

			got, err := v.Get(tt.args.key)
			gs.Expect(err).NotTo(HaveOccurred())
			gs.Expect(got).To(Equal(tt.want))
		})
	}
}

func Test_viperReader_checkDefaultConfig(t *testing.T) {
	g := NewWithT(t)
	dir, err := os.MkdirTemp("", "clusterctl")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(dir)
	dir = strings.TrimSuffix(dir, "/")

	configFile := filepath.Join(dir, "clusterctl.yaml")
	g.Expect(os.WriteFile(configFile, []byte("bar: bar"), 0600)).To(Succeed())

	type fields struct {
		configPaths []string
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "tmp path without final /",
			fields: fields{
				configPaths: []string{dir},
			},
			want: true,
		},
		{
			name: "tmp path with final /",
			fields: fields{
				configPaths: []string{fmt.Sprintf("%s/", dir)},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := NewWithT(t)

			v := &viperReader{
				configPaths: tt.fields.configPaths,
			}
			gs.Expect(v.checkDefaultConfig()).To(Equal(tt.want))
		})
	}
}
