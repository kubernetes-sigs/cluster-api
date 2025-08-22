/*
Copyright 2024 The Kubernetes Authors.

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

package kubernetesversions

import (
	"testing"
)

func Test_calculateURL(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{
			name:    "Stable version",
			version: "stable-1.29",
			want:    "https://dl.k8s.io/release/stable-1.29.txt",
		},
		{
			name:    "CI version",
			version: "ci/latest-1.30",
			want:    "https://dl.k8s.io/ci/latest-1.30.txt",
		},
		{
			name:    "Invalid version",
			version: "abc",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calculateURL(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("calculateURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("calculateURL() got = %v, want %v", got, tt.want)
			}
		})
	}
}
