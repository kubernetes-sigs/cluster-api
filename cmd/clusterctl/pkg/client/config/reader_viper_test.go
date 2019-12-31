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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func Test_viperReader_Get(t *testing.T) {
	dir, err := ioutil.TempDir("", "clusterctl")
	if err != nil {
		t.Fatalf("ioutil.TempDir() error = %v", err)
	}
	defer os.RemoveAll(dir)

	os.Setenv("FOO", "foo")

	configFile := filepath.Join(dir, ".clusterctl.yaml")

	if err := ioutil.WriteFile(configFile, []byte("bar: bar"), 0640); err != nil {
		t.Fatalf("ioutil.WriteFile() error = %v", err)
	}

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
			v := &viperReader{}

			err := v.Init(configFile)
			if err != nil {
				t.Fatalf("Init() error = %v", err)
			}

			got, err := v.Get(tt.args.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Get() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_viperReader_Set(t *testing.T) {
	dir, err := ioutil.TempDir("", "clusterctl")
	if err != nil {
		t.Fatalf("ioutil.TempDir() error = %v", err)
	}
	defer os.RemoveAll(dir)

	os.Setenv("FOO", "foo")

	configFile := filepath.Join(dir, ".clusterctl.yaml")

	if err := ioutil.WriteFile(configFile, []byte("bar: bar"), 0640); err != nil {
		t.Fatalf("ioutil.WriteFile() error = %v", err)
	}

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
			v := &viperReader{}

			err := v.Init(configFile)
			if err != nil {
				t.Fatalf("Init() error = %v", err)
			}

			v.Set(tt.args.key, tt.args.value)

			got, err := v.Get(tt.args.key)
			if err != nil {
				t.Errorf("Get() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("Get() got = %v, want %v (Set() did not worked)", got, tt.want)
			}
		})
	}
}
