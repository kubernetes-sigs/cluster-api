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
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

func TestParseYAMLStream(t *testing.T) {
	sourceGVs := []schema.GroupVersion{
		{Group: "cluster.x-k8s.io", Version: "v1beta1"},
	}

	knownGroups := []string{"cluster.x-k8s.io"}

	matcher := newGVKMatcher(sourceGVs, knownGroups)

	tests := []struct {
		name          string
		input         string
		wantDocCount  int
		wantFirstType resourceType
		wantErr       bool
	}{
		{
			name: "single v1beta1 cluster",
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
			wantDocCount:  1,
			wantFirstType: resourceTypeConvertible,
			wantErr:       false,
		},
		{
			name: "multi-document YAML",
			input: `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Machine
metadata:
  name: test-machine
  namespace: default
`,
			wantDocCount:  2,
			wantFirstType: resourceTypeConvertible,
			wantErr:       false,
		},
		{
			name: "non-CAPI resource",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: default
data:
  key: value
`,
			wantDocCount:  1,
			wantFirstType: resourceTypePassThrough,
			wantErr:       false,
		},
		{
			name: "empty document",
			input: `


`,
			wantDocCount: 0,
			wantErr:      false,
		},
		{
			name: "invalid YAML - missing apiVersion",
			input: `kind: Cluster
metadata:
  name: test-cluster
`,
			wantDocCount:  0,
			wantFirstType: resourceTypePassThrough,
			wantErr:       true,
		},
		{
			name: "invalid YAML - missing kind",
			input: `apiVersion: cluster.x-k8s.io/v1beta1
metadata:
  name: test-cluster
`,
			wantDocCount: 0,
			wantErr:      true,
		},
		{
			name: "invalid YAML - not a kubernetes object",
			input: `just: some
random: yaml
that: is
not: a k8s object
`,
			wantDocCount: 0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := parseYAMLStream([]byte(tt.input), scheme.Scheme, matcher)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseYAMLStream() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(docs) != tt.wantDocCount {
				t.Errorf("parseYAMLStream() got %d documents, want %d", len(docs), tt.wantDocCount)
				return
			}
			if tt.wantDocCount > 0 && docs[0].typ != tt.wantFirstType {
				t.Errorf("parseYAMLStream() first doc type = %v, want %v", docs[0].typ, tt.wantFirstType)
			}
		})
	}
}

func TestSerializeYAMLStream(t *testing.T) {
	sourceGVs := []schema.GroupVersion{
		{Group: "cluster.x-k8s.io", Version: "v1beta1"},
	}
	knownGroups := []string{"cluster.x-k8s.io"}
	matcher := newGVKMatcher(sourceGVs, knownGroups)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "single document",
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
			wantErr: false,
		},
		{
			name: "multi-document YAML",
			input: `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Machine
metadata:
  name: test-machine
  namespace: default
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the input.
			docs, err := parseYAMLStream([]byte(tt.input), scheme.Scheme, matcher)
			if err != nil {
				t.Fatalf("parseYAMLStream() error = %v", err)
			}

			// Serialize back.
			output, err := serializeYAMLStream(docs, scheme.Scheme)
			if (err != nil) != tt.wantErr {
				t.Errorf("serializeYAMLStream() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(output) == 0 {
				t.Error("serializeYAMLStream() returned empty output")
			}
		})
	}
}

func TestClassifyResource(t *testing.T) {
	sourceGVs := []schema.GroupVersion{
		{Group: "cluster.x-k8s.io", Version: "v1beta1"},
	}
	knownGroups := []string{"cluster.x-k8s.io"}
	matcher := newGVKMatcher(sourceGVs, knownGroups)

	tests := []struct {
		name string
		gvk  schema.GroupVersionKind
		want resourceType
	}{
		{
			name: "v1beta1 cluster - convertible",
			gvk: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "Cluster",
			},
			want: resourceTypeConvertible,
		},
		{
			name: "v1beta2 cluster - known but not convertible",
			gvk: schema.GroupVersionKind{
				Group:   "cluster.x-k8s.io",
				Version: "v1beta2",
				Kind:    "Cluster",
			},
			want: resourceTypeKnown,
		},
		{
			name: "non-CAPI resource - pass through",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
			want: resourceTypePassThrough,
		},
		{
			name: "bootstrap CAPI resource - pass through",
			gvk: schema.GroupVersionKind{
				Group:   "bootstrap.cluster.x-k8s.io",
				Version: "v1beta1",
				Kind:    "KubeadmConfig",
			},
			want: resourceTypePassThrough,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyResource(tt.gvk, matcher)
			if got != tt.want {
				t.Errorf("classifyResource() = %v, want %v", got, tt.want)
			}
		})
	}
}
