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

package client

import "testing"

func Test_parseProviderName(t *testing.T) {
	type args struct {
		provider string
	}
	tests := []struct {
		name        string
		args        args
		wantName    string
		wantVersion string
		wantErr     bool
	}{
		{
			name: "simple name",
			args: args{
				provider: "provider",
			},
			wantName:    "provider",
			wantVersion: "",
			wantErr:     false,
		},
		{
			name: "name & version",
			args: args{
				provider: "provider:version",
			},
			wantName:    "provider",
			wantVersion: "version",
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion, err := parseProviderName(tt.args.provider)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotName != tt.wantName {
				t.Errorf("gotName = %v, want %v", gotName, tt.wantName)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("gotVersion = %v, want %v", gotVersion, tt.wantVersion)
			}
		})
	}
}
