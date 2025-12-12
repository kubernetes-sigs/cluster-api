/*
Copyright 2025 The Kubernetes Authors.

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

package convert

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestConverter_Convert(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		toVersion     string
		wantErr       bool
		wantConverted bool
	}{
		{
			name: "convert v1beta1 cluster to v1beta2",
			input: `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
`,
			toVersion:     "v1beta2",
			wantErr:       false,
			wantConverted: true,
		},
		{
			name: "pass through v1beta2 cluster unchanged",
			input: `apiVersion: cluster.x-k8s.io/v1beta2
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
`,
			toVersion:     "v1beta2",
			wantErr:       false,
			wantConverted: false,
		},
		{
			name: "pass through non-CAPI resource",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: default
data:
  key: value
`,
			toVersion:     "v1beta2",
			wantErr:       false,
			wantConverted: false,
		},
		{
			name: "convert multi-document YAML",
			input: `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: Machine
metadata:
  name: test-machine
  namespace: default
`,
			toVersion:     "v1beta2",
			wantErr:       false,
			wantConverted: true,
		},
		{
			name: "invalid YAML",
			input: `this is not valid yaml
kind: Cluster
`,
			toVersion: "v1beta2",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceGroupVersions := []schema.GroupVersion{clusterv1beta1.GroupVersion}
			knownAPIGroups := []string{clusterv1.GroupVersion.Group}
			converter := NewConverter("cluster.x-k8s.io", clusterv1.GroupVersion, sourceGroupVersions, knownAPIGroups)
			output, messages, err := converter.Convert([]byte(tt.input), tt.toVersion)

			if (err != nil) != tt.wantErr {
				t.Errorf("Convert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if len(output) == 0 {
				t.Error("Convert() returned empty output")
			}

			// Verify output contains expected version if conversion happened.
			if tt.wantConverted {
				outputStr := string(output)
				if !strings.Contains(outputStr, "cluster.x-k8s.io/v1beta2") {
					t.Errorf("Convert() output does not contain v1beta2 version: %s", outputStr)
				}
			}

			// Messages should be non-nil (even if empty).
			if messages == nil {
				t.Error("Convert() returned nil messages slice")
			}
		})
	}
}
