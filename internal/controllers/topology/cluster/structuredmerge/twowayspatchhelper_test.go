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

package structuredmerge

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/test/builder"
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
		// Create

		{
			name:     "Create if original does not exists",
			original: nil,
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"apiVersion": builder.BootstrapGroupVersion.String(),
					"kind":       builder.GenericBootstrapConfigKind,
					"metadata": map[string]interface{}{
						"namespace": metav1.NamespaceDefault,
						"name":      "foo",
					},
					"spec": map[string]interface{}{
						"foo": "foo",
					},
				},
			},
			options:            []HelperOption{},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte(fmt.Sprintf("{\"apiVersion\":%q,\"kind\":%q,\"metadata\":{\"name\":\"foo\",\"namespace\":%q},\"spec\":{\"foo\":\"foo\"}}", builder.BootstrapGroupVersion.String(), builder.GenericBootstrapConfigKind, metav1.NamespaceDefault)),
		},

		// Ignore fields

		{
			name: "Ignore fields are removed from the patch",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"controlPlaneEndpoint": map[string]interface{}{
							"host": "",
							"port": int64(0),
						},
					},
				},
			},
			options:            []HelperOption{IgnorePaths{contract.Path{"spec", "controlPlaneEndpoint"}}},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},

		// Allowed Path fields

		{
			name: "Not allowed fields are removed from the patch",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"status": map[string]interface{}{
						"foo": "foo",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},

		// Field both in original and in modified --> align to modified if different

		{
			name: "Field (spec.foo) both in original and in modified, no-op when equal",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},
		{
			name: "Field (metadata.label) both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo-modified",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{\"metadata\":{\"labels\":{\"foo\":\"foo-modified\"}}}"),
		},
		{
			name: "Field (spec.template.spec.foo) both in original and in modified, no-op when equal",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"foo": "foo",
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
								"foo": "foo",
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
			name: "Field (spec.foo) both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo",
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo-changed",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"foo\":\"foo-changed\"}}"),
		},
		{
			name: "Field (metadata.label) both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo-changed",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{\"metadata\":{\"labels\":{\"foo\":\"foo-changed\"}}}"),
		},
		{
			name: "Field (spec.template.spec.foo) both in original and in modified, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"foo": "foo",
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
								"foo": "foo-changed",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"template\":{\"spec\":{\"foo\":\"foo-changed\"}}}}"),
		},

		{
			name: "Value of type Array or Slice both in original and in modified,, align to modified when different", // Note: fake treats all the slice as atomic (false positive)
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []interface{}{
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
						"slice": []interface{}{
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
			name: "Field (spec.foo) in modified only, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo-changed",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"foo\":\"foo-changed\"}}"),
		},
		{
			name: "Field (metadata.label) in modified only, align to modified",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo-changed",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{\"metadata\":{\"labels\":{\"foo\":\"foo-changed\"}}}"),
		},
		{
			name: "Field (spec.template.spec.foo) in modified only, align to modified when different",
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{ // desired
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"foo": "foo-changed",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
			wantPatch:          []byte("{\"spec\":{\"template\":{\"spec\":{\"foo\":\"foo-changed\"}}}}"),
		},

		{
			name: "Value of type Array or Slice in modified only, align to modified when different",
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []interface{}{
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

		// Field only in original (not existing in modified) --> preserve original

		{
			name: "Field (spec.foo) in original only, preserve", // Note: fake can't detect if has been originated from templates or from external controllers, so it assumes  (false negative)
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "foo",
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
			name: "Field (metadata.label) in original only, preserve", // Note: fake can't detect if has been originated from templates or from external controllers (false negative)
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo",
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
		{
			name: "Field (spec.template.spec.foo) in original only, preserve", // Note: fake can't detect if has been originated from templates or from external controllers (false negative)
			original: &unstructured.Unstructured{ // current
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"foo": "foo",
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

		{
			name: "Value of type Array or Slice in original only, preserve", // Note: fake can't detect if has been originated from templates or from external controllers (false negative)
			original: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"spec": map[string]interface{}{
						"slice": []interface{}{
							"D",
							"C",
							"B",
						},
					},
				},
			},
			modified: &unstructured.Unstructured{
				Object: map[string]interface{}{},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
			wantPatch:          []byte("{}"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			patch, err := NewTwoWaysPatchHelper(tt.original, tt.modified, env.GetClient(), tt.options...)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(patch.patch).To(Equal(tt.wantPatch))
			g.Expect(patch.HasChanges()).To(Equal(tt.wantHasChanges))
			g.Expect(patch.HasSpecChanges()).To(Equal(tt.wantHasSpecChanges))
		})
	}
}
