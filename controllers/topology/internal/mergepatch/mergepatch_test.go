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
		name           string
		original       *unstructured.Unstructured // current
		modified       *unstructured.Unstructured // desired
		options        []HelperOption
		wantHasChanges bool
		wantPatch      []byte
	}{
		// Field both in original and in modified --> align to modified

		{
			name: "Field both in original and in modified, no-op when equal",
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
			wantHasChanges: false,
			wantPatch:      []byte("{}"),
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"spec\":{\"foo\":\"bar\"}}"),
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
			wantHasChanges: false,
			wantPatch:      []byte("{}"),
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"spec\":{\"template\":{\"spec\":{\"A\":\"A\"}}}}"),
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"spec\":{\"map\":{\"A\":\"A\",\"C\":\"C\"}}}"),
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"spec\":{\"slice\":[\"A\",\"B\",\"C\"]}}"),
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"spec\":{\"foo\":\"bar\"}}"),
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"spec\":{\"template\":{\"spec\":{\"A\":\"A\"}}}}"),
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
			wantHasChanges: false,
			wantPatch:      []byte("{}"),
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
			wantHasChanges: false,
			wantPatch:      []byte("{}"),
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
			wantHasChanges: false,
			wantPatch:      []byte("{}"),
		},
		{
			name: "Relevant Diff are preserved",
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"metadata\":{\"annotations\":{\"foo\":\"bar\"},\"labels\":{\"foo\":\"bar\"}}}"),
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
			options:        []HelperOption{IgnorePaths{contract.Path{"spec", "controlPlaneEndpoint"}}},
			wantHasChanges: false,
			wantPatch:      []byte("{}"),
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
			wantHasChanges: false,
			wantPatch:      []byte("{}"),
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
			wantHasChanges: true,
			wantPatch:      []byte("{\"spec\":{\"B\":\"B\"}}"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			patch, err := NewHelper(tt.original, tt.modified, nil, tt.options...)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(patch.HasChanges()).To(Equal(tt.wantHasChanges))
			g.Expect(patch.patch).To(Equal(tt.wantPatch))
		})
	}
}
