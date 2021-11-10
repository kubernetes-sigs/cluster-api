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
			name: "Field (metadata.label) preserve instance specific values when path is not authoritative",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"a": "a",
							"b": "b-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							// a missing
							"b": "b",
							"c": "c",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{\"metadata\":{\"labels\":{\"b\":\"b\",\"c\":\"c\"}}}"),
		},
		{
			name: "Field (metadata.label) align to modified when path is authoritative",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"a": "a",
							"b": "b-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							// a missing
							"b": "b",
							"c": "c",
						},
					},
				},
			},
			options:            []HelperOption{AuthoritativePaths{contract.Path{"metadata", "labels"}}},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{\"metadata\":{\"labels\":{\"a\":null,\"b\":\"b\",\"c\":\"c\"}}}"),
		},
		{
			name: "IgnorePaths supersede AuthoritativePaths",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"a": "a",
							"b": "b-changed",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							// a missing
							"b": "b",
							"c": "c",
						},
					},
				},
			},
			options:            []HelperOption{AuthoritativePaths{contract.Path{"metadata", "labels"}}, IgnorePaths{contract.Path{"metadata", "labels"}}},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
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
			name: "Value of type map, enforces entries from modified if the path is authoritative",
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
			options:            []HelperOption{AuthoritativePaths{contract.Path{"spec", "map"}}},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"map\":{\"A\":\"A\",\"B\":null,\"C\":\"C\"}}}"),
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

func Test_filterPaths(t *testing.T) {
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
		{
			name: "Cleanup empty maps",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
					"baz": map[string]interface{}{
						"bar": "123",
					},
				},
			},
			paths: []contract.Path{},
			want:  map[string]interface{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			filterPaths(tt.patchMap, tt.paths)

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
					"baz": "123",
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"baz": "123",
				},
			},
		},
		{
			name: "Remove nested map",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": map[string]interface{}{
						"baz": "123",
					},
					"baz": "123",
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"baz": "123",
				},
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
		{
			name: "Cleanup empty maps",
			patchMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"baz": map[string]interface{}{
						"bar": "123",
					},
				},
			},
			path: contract.Path([]string{"foo", "baz", "bar"}),
			want: map[string]interface{}{},
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

func Test_enforcePath(t *testing.T) {
	tests := []struct {
		name             string
		authoritativeMap map[string]interface{}
		twoWaysMap       map[string]interface{}
		path             contract.Path
		want             map[string]interface{}
	}{
		{
			name: "Keep value not enforced",
			authoritativeMap: map[string]interface{}{
				"foo": nil,
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123",
				},
			},
			// no enforcing path
		},
		{
			name: "Enforce value",
			authoritativeMap: map[string]interface{}{
				"foo": nil,
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123", // value enforced, it should be overridden.
				},
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{
				"foo": nil,
			},
		},
		{
			name: "Enforce nested value",
			authoritativeMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": "123", // value enforced, it should be overridden.
					"baz": "345", // value not enforced, it should be preserved.
				},
			},
			path: contract.Path([]string{"foo", "bar"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
					"baz": "345",
				},
			},
		},
		{
			name: "Enforce nested value",
			authoritativeMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
			twoWaysMap: map[string]interface{}{
				"foo": map[string]interface{}{ // value enforced, it should be overridden.
					"bar": "123",
					"baz": "345",
				},
			},
			path: contract.Path([]string{"foo"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
		},
		{
			name: "Enforce nested value rebuilding struct if missing",
			authoritativeMap: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
			twoWaysMap: map[string]interface{}{}, // foo enforced, it should be rebuilt/overridden.
			path:       contract.Path([]string{"foo"}),
			want: map[string]interface{}{
				"foo": map[string]interface{}{
					"bar": nil,
				},
			},
		},
		{
			name: "Ignore partial match",
			authoritativeMap: map[string]interface{}{
				"foo": "a",
			},
			twoWaysMap: map[string]interface{}{},
			path:       contract.Path([]string{"foo", "bar", "baz"}),
			want:       map[string]interface{}{},
		},

		{
			name: "authoritative has no changes, twoWays has no changes, no changes",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": nil,
					},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": nil,
					},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": nil,
					},
				},
			},
		},
		{
			name: "authoritative has no changes, twoWays has no changes, no changes",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{},
		},
		{
			name: "authoritative has changes, twoWays has no changes, authoritative apply",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		{
			name: "authoritative has changes, twoWays has changes, authoritative apply",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "baz",
							},
						},
					},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
		},
		{
			name: "authoritative has no changes, twoWays has changes, twoWays changes blanked out",
			authoritativeMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{},
				},
			},
			twoWaysMap: map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"foo": "baz",
							},
						},
					},
				},
			},
			path: contract.Path([]string{"spec", "template", "metadata"}),
			want: map[string]interface{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			enforcePath(tt.authoritativeMap, tt.twoWaysMap, tt.path)

			g.Expect(tt.twoWaysMap).To(Equal(tt.want))
		})
	}
}
