/*
Copyright 2021 The Kubernetes Authors.

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

package mergepatch

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
)

func TestNewHelper(t *testing.T) {
	tests := []struct {
		name               string
		original           *unstructured.Unstructured // current
		modified           *unstructured.Unstructured // desired
		options            []HelperOption
		wantHasChanges     bool
		wantHasSpecChanges bool
		wantPatch          []byte
	}{
		// Field both in original and in modified --> align to modified

		{
			name: "Field (spec) both in original and in modified, no-op when equal",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},
		{
			name: "Field both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar-changed",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"foo\":\"bar\"}}"),
		},
		{
			name: "Field (metadata.label) both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{\"metadata\":{\"labels\":{\"foo\":\"bar\"}}}"),
		},
		{
			name: "Nested field both in original and in modified, no-op when equal",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},
		{
			name: "Nested field both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A-Changed",
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"template\":{\"spec\":{\"A\":\"A\"}}}}"),
		},
		{
			name: "Value of type map, enforces entries from modified, preserve entries only in original",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"map": map[string]string{
							"A": "A-changed",
							"B": "B",
							// C missing
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"map": map[string]string{
							"A": "A",
							// B missing
							"C": "C",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"map\":{\"A\":\"A\",\"C\":\"C\"}}}"),
		},
		{
			name: "Value of type Array or Slice, align to modified",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []string{
							"D",
							"C",
							"B",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []string{
							"A",
							"B",
							"C",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"slice\":[\"A\",\"B\",\"C\"]}}"),
		},

		// Field only in modified (not existing in original) --> align to modified

		{
			name: "Field only in modified, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"foo\":\"bar\"}}"),
		},
		{
			name: "Nested field only in modified, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"template\":{\"spec\":{\"A\":\"A\"}}}}"),
		},

		// Field only in original (not existing in modified) --> preserve original

		{
			name: "Field only in original, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},
		{
			name: "Nested field only in original, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"A": "A",
							},
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},

		// Diff for metadata fields computed by the system or in status are discarded

		{
			name: "Diff for metadata fields computed by the system or in status are discarded",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"selfLink":        "foo",
						"uid":             "foo",
						"resourceVersion": "foo",
						"generation":      "foo",
						"managedFields":   "foo",
					},
					"status": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},
		{
			name: "Relevant Diff for metadata (labels and annotations) are preserved",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
						"annotations": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{\"metadata\":{\"annotations\":{\"foo\":\"bar\"},\"labels\":{\"foo\":\"bar\"}}}"),
		},

		// Ignore fields

		{
			name: "Ignore fields are removed from the diff",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": 0,
						},
					},
				},
			},
			options:            []HelperOption{IgnorePaths{contract.Path{"spec", "controlPlaneEndpoint"}}},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},

		// More tests
		{
			name: "No changes",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						"B": "B",
						"C": "C", // C only in modified
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						"B": "B",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},
		{
			name: "Many changes",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						// B missing
						"C": "C", // C only in modified
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"A": "A",
						"B": "B",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"B\":\"B\"}}"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			patch, err := NewHelper(tt.original, tt.modified, nil, tt.options...)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(patch.HasChanges()).To(Equal(tt.wantHasChanges))
			g.Expect(patch.HasSpecChanges()).To(Equal(tt.wantHasSpecChanges))
			g.Expect(patch.patch).To(Equal(tt.wantPatch))
		})
	}
}

func Test_filterPatchMap(t *testing.T) {
	tests := []struct {
		name     string
		patchMap map[string]interface{}
		paths    []contract.Path
		want     map[string]interface{}
	}{
		{
			name: "Allow values",
			patchMap: map[string]interface{}{
				"foo": "123",
				"bar": 123,
				"baz": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
			},
			paths: []contract.Path{
				[]string{"foo"},
				[]string{"baz", "foo"},
			},
			want: map[string]interface{}{
				"foo": "123",
				"baz": map[string]interface{}{
					"foo": "123",
				},
			},
		},
		{
			name: "Allow maps",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
				"bar": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
				"baz": map[string]interface{}{
					"foo": map[string]interface{}{
						"foo": "123",
						"bar": 123,
					},
					"bar": map[string]interface{}{
						"foo": "123",
						"bar": 123,
					},
				},
			},
			paths: []contract.Path{
				[]string{"foo"},
				[]string{"baz", "foo"},
			},
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"foo": "123",
					"bar": 123,
				},
				"baz": map[string]interface{}{
					"foo": map[string]interface{}{
						"foo": "123",
						"bar": 123,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			filterPatchMap(tt.patchMap, tt.paths)

			g.Expect(tt.patchMap).To(Equal(tt.want))
		})
	}
}

func Test_removePath(t *testing.T) {
	tests := []struct {
		name     string
		patchMap map[string]interface{}
		path     contract.Path
		want     map[string]interface{}
	}{
		{
			name: "Remove value",
			patchMap: map[string]interface{}{
				"foo": "123",
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{},
		},
		{
			name: "Remove map",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{},
		},
		{
			name: "Remove nested value",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{},
			},
		},
		{
			name: "Remove nested map",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": map[string]interface{}{
						"baz": "123",
					},
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{},
			},
		},
		{
			name: "Ignore partial match",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			path: contract.Path([]string{"foo", "bar", "baz"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			removePath(tt.patchMap, tt.path)

			g.Expect(tt.patchMap).To(Equal(tt.want))
		})
	}
}

func Test_cleanupEmptyMaps(t *testing.T) {
	tests := []struct {
		name string
		get  map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "Preserve values",
			get: map[string]interface{}{
				"foo": "bar",
			},
			want: map[string]interface{}{
				"foo": "bar",
			},
		},
		{
			name: "Preserve nested values",
			get: map[string]interface{}{
				"spec": map[string]interface{}{
					"foo": "bar",
				},
			},
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"foo": "bar",
				},
			},
		},
		{
			name: "Drop empty maps",
			get: map[string]interface{}{
				"spec": map[string]interface{}{},
			},
			want: map[string]interface{}{},
		},
		{
			name: "Drop empty nested maps",
			get: map[string]interface{}{
				"spec": map[string]interface{}{
					"map": map[string]interface{}{},
				},
			},
			want: map[string]interface{}{},
		},
		{
			name: "mixed case",
			get: map[string]interface{}{
				"m1": map[string]interface{}{
					"mm2": map[string]interface{}{},
					"foo": "bar",
				},
				"m2":  map[string]interface{}{},
				"foo": "bar",
			},
			want: map[string]interface{}{
				"m1": map[string]interface{}{
					"foo": "bar",
				},
				"foo": "bar",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cleanupEmptyMaps(tt.get)

			g.Expect(tt.get).To(Equal(tt.want))
		})
	}
}
