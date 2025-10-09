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

package migrate

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestYAMLParser_ParseYAMLStream(t *testing.T) {
	scheme := runtime.NewScheme()
	parser := NewYAMLParser(scheme)

	testYAML := `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: test-cluster
  namespace: default
spec:
  clusterNetwork:
    pods:
      cidrBlocks: ["192.168.0.0/16"]
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  key: value
---
apiVersion: cluster.x-k8s.io/v1beta2
kind: Machine
metadata:
  name: test-machine
spec:
  clusterName: test-cluster`

	reader := strings.NewReader(testYAML)
	docs, err := parser.ParseYAMLStream(reader)
	if err != nil {
		t.Fatalf("ParseYAMLStream failed: %v", err)
	}

	if len(docs) != 3 {
		t.Fatalf("Expected 3 documents, got %d", len(docs))
	}

	// Test first document (v1beta1 CAPI resource)
	if docs[0].Type != ResourceTypeCoreV1Beta1 {
		t.Errorf("Expected first document to be ResourceTypeCoreV1Beta1, got %v", docs[0].Type)
	}
	expectedGVK := schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	}
	if docs[0].GVK != expectedGVK {
		t.Errorf("Expected GVK %v, got %v", expectedGVK, docs[0].GVK)
	}

	// Test second document (non-CAPI resource)
	if docs[1].Type != ResourceTypeNonCAPI {
		t.Errorf("Expected second document to be ResourceTypeNonCAPI, got %v", docs[1].Type)
	}

	// Test third document (v1beta2 CAPI resource)
	if docs[2].Type != ResourceTypeOtherCAPI {
		t.Errorf("Expected third document to be ResourceTypeOtherCAPI, got %v", docs[2].Type)
	}
}

func TestYAMLParser_SerializeYAMLStream(t *testing.T) {
	scheme := runtime.NewScheme()
	parser := NewYAMLParser(scheme)

	// Create test documents
	docs := []Document{
		{
			Object: nil,
			Raw:    []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\ndata:\n  key: value"),
			GVK:    schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"},
			Type:   ResourceTypeNonCAPI,
			Index:  0,
		},
		{
			Object: nil,
			Raw:    []byte("apiVersion: cluster.x-k8s.io/v1beta1\nkind: Cluster\nmetadata:\n  name: test-cluster"),
			GVK:    schema.GroupVersionKind{Group: "cluster.x-k8s.io", Version: "v1beta1", Kind: "Cluster"},
			Type:   ResourceTypeCoreV1Beta1,
			Index:  1,
		},
	}

	var buf bytes.Buffer
	err := parser.SerializeYAMLStream(docs, &buf)
	if err != nil {
		t.Fatalf("SerializeYAMLStream failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "---\n") {
		t.Error("Expected document separator '---' in output")
	}

	// Check that both documents are present
	if !strings.Contains(output, "ConfigMap") {
		t.Error("Expected ConfigMap in output")
	}
	if !strings.Contains(output, "Cluster") {
		t.Error("Expected Cluster in output")
	}
}

func TestYAMLParser_EmptyInput(t *testing.T) {
	scheme := runtime.NewScheme()
	parser := NewYAMLParser(scheme)

	reader := strings.NewReader("")
	docs, err := parser.ParseYAMLStream(reader)
	if err != nil {
		t.Fatalf("ParseYAMLStream failed on empty input: %v", err)
	}

	if len(docs) != 0 {
		t.Fatalf("Expected 0 documents for empty input, got %d", len(docs))
	}
}

func TestYAMLParser_InvalidYAML(t *testing.T) {
	scheme := runtime.NewScheme()
	parser := NewYAMLParser(scheme)

	invalidYAML := `this is not valid yaml: [unclosed bracket`
	reader := strings.NewReader(invalidYAML)

	docs, err := parser.ParseYAMLStream(reader)
	if err != nil {
		t.Fatalf("ParseYAMLStream should handle invalid YAML gracefully: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("Expected 1 document for invalid YAML, got %d", len(docs))
	}

	if docs[0].Type != ResourceTypeUnsupported {
		t.Errorf("Expected invalid YAML to be classified as ResourceTypeUnsupported, got %v", docs[0].Type)
	}

	expectedRaw := invalidYAML + "\n"
	if string(docs[0].Raw) != expectedRaw {
		t.Errorf("Expected raw content to be preserved with newline, got %q", string(docs[0].Raw))
	}
}
