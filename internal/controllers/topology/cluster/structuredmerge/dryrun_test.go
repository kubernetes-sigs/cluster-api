/*
Copyright 2022 The Kubernetes Authors.

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
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/internal/contract"
)

func Test_dryRunPatch(t *testing.T) {
	tests := []struct {
		name               string
		ctx                *dryRunInput
		wantHasChanges     bool
		wantHasSpecChanges bool
	}{
		{
			name: "DryRun detects no changes on managed fields",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:metadata": map[string]interface{}{
						"f:labels": map[string]interface{}{
							"f:foo": map[string]interface{}{},
						},
					},
					"f:spec": map[string]interface{}{
						"f:foo": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "apiVersion, kind, metadata.name and metadata.namespace fields in modified are not detected as changes",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					// apiVersion, kind, metadata.name and metadata.namespace are not tracked in managedField.
					// NOTE: We are simulating a real object with something in spec and labels, so both
					// the top level object and metadata are considered as granular maps.
					"f:metadata": map[string]interface{}{
						"f:labels": map[string]interface{}{
							"f:foo": map[string]interface{}{},
						},
					},
					"f:spec": map[string]interface{}{
						"f:foo": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
				modified: map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "apiVersion, kind, metadata.name and metadata.namespace fields in modified are not detected as changes (edge case)",
			ctx: &dryRunInput{
				path:     contract.Path{},
				fieldsV1: map[string]interface{}{
					// apiVersion, kind, metadata.name and metadata.namespace are not tracked in managedField.
					// NOTE: we are simulating an edge case where we are not tracking managed fields
					// in metadata or in spec; this could lead to edge case because server side applies required
					// apiVersion, kind, metadata.name and metadata.namespace but those are not tracked in managedFields.
					// If this case is not properly handled, dryRun could report false positives assuming those field
					// have been added to modified.
				},
				original: map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
				},
				modified: map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "Foo",
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "DryRun detects metadata only change on managed fields",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:metadata": map[string]interface{}{
						"f:labels": map[string]interface{}{
							"f:foo": map[string]interface{}{},
						},
					},
					"f:spec": map[string]interface{}{
						"f:foo": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar-changed",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
		},
		{
			name: "DryRun spec only change on managed fields",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:metadata": map[string]interface{}{
						"f:labels": map[string]interface{}{
							"f:foo": map[string]interface{}{},
						},
					},
					"f:spec": map[string]interface{}{
						"f:foo": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"foo": "bar-changed",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "identifies changes when modified has a value not previously managed",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:foo": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
						"bar": "baz", // new value not previously managed
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "identifies changes when modified drops a value previously managed",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:foo": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"foo": "bar",
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						// foo (previously managed) has been dropped
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "No changes in an atomic map",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:atomicMap": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "identifies changes in an atomic map",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:atomicMap": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "bar-changed",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "No changes on managed atomic list",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:atomicList": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "Identifies changes on managed atomic list",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:atomicList": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							map[string]interface{}{
								"foo": "bar",
							},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							map[string]interface{}{
								"foo": "bar-changed",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "No changes on managed listMap",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listMap": map[string]interface{}{
							"k:{\"foo\":\"id1\"}": map[string]interface{}{
								"f:foo": map[string]interface{}{},
								"f:bar": map[string]interface{}{},
							},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
								"bar": "baz",
							},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
								"bar": "baz",
							},
						},
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "Identified value added on a empty managed listMap",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listMap": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified value added on a managed listMap",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listMap": map[string]interface{}{
							"k:{\"foo\":\"id1\"}": map[string]interface{}{
								"f:foo": map[string]interface{}{},
							},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
							},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
							},
							map[string]interface{}{
								"foo": "id2",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified value removed on a managed listMap",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listMap": map[string]interface{}{
							"k:{\"foo\":\"id1\"}": map[string]interface{}{
								"f:foo": map[string]interface{}{},
							},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
							},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified changes on a managed listMap",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listMap": map[string]interface{}{
							"k:{\"foo\":\"id1\"}": map[string]interface{}{
								"f:foo": map[string]interface{}{},
								"f:bar": map[string]interface{}{},
							},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
								"bar": "baz",
							},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
								"baz": "baz-changed",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified changes on a managed listMap (same number of items, different keys)",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listMap": map[string]interface{}{
							"k:{\"foo\":\"id1\"}": map[string]interface{}{
								"f:foo": map[string]interface{}{},
								"f:bar": map[string]interface{}{},
							},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id1",
								"bar": "baz",
							},
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"foo": "id2",
								"bar": "baz",
							},
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "no changes on a managed listSet",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listSet": map[string]interface{}{
							"v:foo": map[string]interface{}{},
							"v:bar": map[string]interface{}{},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
							"bar",
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
							"bar",
						},
					},
				},
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "Identified value added on a empty managed listSet",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listSet": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified value added on a managed listSet",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listSet": map[string]interface{}{
							"v:foo": map[string]interface{}{},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
							"bar",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified value removed on a managed listSet",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listSet": map[string]interface{}{
							"v:foo": map[string]interface{}{},
							"v:bar": map[string]interface{}{},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
							"bar",
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
							// bar removed
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified changes on a managed listSet",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:listSet": map[string]interface{}{
							"v:foo": map[string]interface{}{},
							"v:bar": map[string]interface{}{},
						},
					},
				},
				original: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
							"bar",
						},
					},
				},
				modified: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"foo",
							"bar-changed",
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Identified nested field got added",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					// apiVersion, kind, metadata.name and metadata.namespace are not tracked in managedField.
					// NOTE: We are simulating a real object with something in spec and labels, so both
					// the top level object and metadata are considered as granular maps.
					"f:metadata": map[string]interface{}{
						"f:labels": map[string]interface{}{
							"f:foo": map[string]interface{}{},
						},
					},
					"f:spec": map[string]interface{}{
						"f:another": map[string]interface{}{},
					},
				},
				original: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"another": "value",
					},
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
						"labels": map[string]interface{}{
							"foo": "bar",
						},
					},
					"spec": map[string]interface{}{
						"another": "value",
						"foo": map[string]interface{}{
							"bar": true,
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Nested type gets changed",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"f:foo": map[string]interface{}{
							"v:bar": map[string]interface{}{},
						},
					},
				},
				original: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
					"spec": map[string]interface{}{
						"foo": []interface{}{
							"bar",
						},
					},
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
					"spec": map[string]interface{}{
						"foo": map[string]interface{}{
							"bar": true,
						},
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Nested field is getting removed",
			ctx: &dryRunInput{
				path: contract.Path{},
				fieldsV1: map[string]interface{}{
					"f:spec": map[string]interface{}{
						"v:keep": map[string]interface{}{},
						"f:foo": map[string]interface{}{
							"v:bar": map[string]interface{}{},
						},
					},
				},
				original: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
					"spec": map[string]interface{}{
						"keep": "me",
						"foo": []interface{}{
							"bar",
						},
					},
				},
				modified: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":      "foo",
						"namespace": "bar",
					},
					"spec": map[string]interface{}{
						"keep": "me",
					},
				},
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotHasChanges, gotHasSpecChanges := dryRunPatch(tt.ctx)

			g.Expect(gotHasChanges).To(Equal(tt.wantHasChanges))
			g.Expect(gotHasSpecChanges).To(Equal(tt.wantHasSpecChanges))
		})
	}
}
