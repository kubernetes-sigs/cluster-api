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

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/scheme"
)

func TestParseYAMLStream(t *testing.T) {
	sourceGVs := map[schema.GroupVersion]bool{
		{Group: "cluster.x-k8s.io", Version: "v1beta1"}: true,
	}

	tests := []struct {
		name                 string
		input                string
		wantDocCount         int
		wantFirstConvertible bool
		wantErr              bool
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
			wantDocCount:         1,
			wantFirstConvertible: true,
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
			wantDocCount:         2,
			wantFirstConvertible: true,
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
			wantDocCount:         1,
			wantFirstConvertible: false,
		},
		{
			name: "empty document",
			input: `


`,
			wantDocCount: 0,
		},
		{
			name: "invalid YAML - missing apiVersion",
			input: `kind: Cluster
metadata:
  name: test-cluster
`,
			wantErr: true,
		},
		{
			name: "invalid YAML - missing kind",
			input: `apiVersion: cluster.x-k8s.io/v1beta1
metadata:
  name: test-cluster
`,
			wantErr: true,
		},
		{
			name: "invalid YAML - not a kubernetes object",
			input: `just: some
random: yaml
that: is
not: a k8s object
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			docs, err := parseYAMLStream([]byte(tt.input), scheme.Scheme, sourceGVs)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(docs).To(HaveLen(tt.wantDocCount))

			if tt.wantDocCount > 0 {
				g.Expect(docs[0].convertible).To(Equal(tt.wantFirstConvertible))
			}
		})
	}
}

func TestSerializeYAMLStream(t *testing.T) {
	sourceGVs := map[schema.GroupVersion]bool{
		{Group: "cluster.x-k8s.io", Version: "v1beta1"}: true,
	}

	tests := []struct {
		name  string
		input string
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			docs, err := parseYAMLStream([]byte(tt.input), scheme.Scheme, sourceGVs)
			g.Expect(err).ToNot(HaveOccurred())

			output, err := serializeYAMLStream(docs, scheme.Scheme)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(output).ToNot(BeEmpty())
		})
	}
}
