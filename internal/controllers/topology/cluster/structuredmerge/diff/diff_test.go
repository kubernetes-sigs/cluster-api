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

package diff

import (
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/internal/contract"
)

var testSchema = CRDSchema{
	"":         TypeDef{Struct: &StructDef{Type: GranularStructType}}, // Root object schema
	"metadata": TypeDef{Struct: &StructDef{Type: GranularStructType}},
	"spec":     TypeDef{Struct: &StructDef{Type: GranularStructType}},

	"spec.struct":       TypeDef{Struct: &StructDef{Type: GranularStructType}},
	"spec.struct.foo":   TypeDef{Scalar: &ScalarDef{}},
	"spec.struct.bar":   TypeDef{Scalar: &ScalarDef{}},
	"spec.struct.baz":   TypeDef{Scalar: &ScalarDef{}},
	"spec.atomicStruct": TypeDef{Struct: &StructDef{Type: AtomicStructType}},

	"metadata.labels":        TypeDef{Map: &MapDef{Type: GranularMapType}}, // mapOfScalars
	"metadata.labels[]":      TypeDef{Scalar: &ScalarDef{}},
	"spec.mapOfStruct":       TypeDef{Map: &MapDef{Type: GranularMapType}},
	"spec.mapOfStruct[]":     TypeDef{Struct: &StructDef{Type: GranularStructType}},
	"spec.mapOfStruct[].foo": TypeDef{Scalar: &ScalarDef{}},
	"spec.mapOfMap":          TypeDef{Map: &MapDef{Type: GranularMapType}},
	"spec.mapOfMap[]":        TypeDef{Map: &MapDef{Type: GranularMapType}},
	"spec.mapOfMap[][]":      TypeDef{Struct: &StructDef{Type: GranularStructType}},
	"spec.mapOfMap[][].foo":  TypeDef{Scalar: &ScalarDef{}},
	"spec.mapOfList":         TypeDef{Map: &MapDef{Type: GranularMapType}},
	"spec.mapOfList[]":       TypeDef{List: &ListDef{Type: AtomicListType}},
	"spec.atomicMap":         TypeDef{Map: &MapDef{Type: AtomicMapType}},

	"spec.listMap":       TypeDef{List: &ListDef{Type: ListMapType, Keys: []string{"id1", "id2"}}},
	"spec.listMap[]":     TypeDef{Struct: &StructDef{Type: GranularStructType}}, // List item schema
	"spec.listMap[].id1": TypeDef{Scalar: &ScalarDef{}},
	"spec.listMap[].id2": TypeDef{Scalar: &ScalarDef{}},
	"spec.listMap[].foo": TypeDef{Scalar: &ScalarDef{}},
	"spec.listMap[].bar": TypeDef{Scalar: &ScalarDef{}},
	"spec.listMap[].baz": TypeDef{Scalar: &ScalarDef{}},
	"spec.listSet":       TypeDef{List: &ListDef{Type: ListSetType}},
	"spec.atomicList":    TypeDef{List: &ListDef{Type: AtomicListType}},
}

// Test_isChangingAnything_basic checks if isChangingAnything handles basic combination of JSONSchemaProps that can be found in a CRD definition.
func Test_isChangingAnything(t *testing.T) {
	tests := []struct {
		name               string
		ctx                *isChangingAnythingInput
		wantHasChanges     bool
		wantHasSpecChanges bool
	}{
		// Test struct & struct variants (atomic struct)

		{
			name: "No changes in struct",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
							"bar": "bar", // field existing only in current object should not trigger change.
						},
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
						},
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "Struct field changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo-changed", // foo has been changed in the current intent.
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Struct field changed in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo-changed", // foo has been changed in the current object.
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "Field does not exists in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							// foo has been deleted from current object.
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "AtomicStruct changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo-changed",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "AtomicStruct changed in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
							"bar": "bar", // bar has been added in the current object.
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "(Atomic) Struct does not exists in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},

		// Test map & map variants (map of scalar, map of struct, map of map, map of list, atomic map)

		{
			name: "No map changes",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo",
							"bar": "bar", // items existing only in current object should not trigger change.
						},
					},
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
							"bar": map[string]interface{}{ // items existing only in current object should not trigger change.
								"foo": "bar",
							},
						},
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
							"bar": map[string]interface{}{ // items existing only in current object should not trigger change.
								"foo": map[string]interface{}{
									"foo": "bar",
								},
							},
						},
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
							"bar": []interface{}{ // items existing only in current object should not trigger change.
								"a",
							},
						},
						"atomicMap": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"foo": "foo",
						},
					},
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
						},
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
						},
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
						},
						"atomicMap": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},
		{
			name: "item in map of scalar item/metadata only changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo-changed", // foo has been changed in the current intent.
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
		},
		{
			name: "item in map of scalar item/metadata only added in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
							"bar": "bar", // bar has been added in the current intent.
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
		},
		{
			name: "item in map of scalar item/metadata only changed in the object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo-changed", // foo has been changed in the current object.
						},
					},
				},
				currentIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: false,
		},
		{
			name: "item in map of struct changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo-changed", // foo has been changed in the current intent.
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of struct added in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
							"bar": map[string]interface{}{ // bar has been added in the current intent.
								"foo": "bar",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of struct changed in the object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo-changed", // foo has been changed in the current object.
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of maps changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{ // foo has been changed in the current intent.
								"foo": map[string]interface{}{
									"foo": "foo-changed",
								},
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of maps added in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
							"bar": map[string]interface{}{ // bar has been added in the current intent.
								"foo": map[string]interface{}{
									"foo": "bar",
								},
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of maps changed in the object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{ // foo has been changed in the current object.
								"foo": map[string]interface{}{
									"foo": "foo-changed",
								},
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of list changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{ // foo has been changed in the current intent.
								"a-changed",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of list added in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
							"bar": []interface{}{ // bar has been changed in the current intent.
								"a",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "item in map of list changed in the object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{ // foo has been changed in the current object.
								"a-changed",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "AtomicMap changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo-changed",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "AtomicMap changed in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo-changed",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "(Atomic) Map does not exists in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},

		// Test list & list variants (atomic list, list set, list map)

		{
			name: "No list changes",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
								"bar": "bar", // field existing only in current object should not trigger change.
							},
							map[string]interface{}{ // item existing only in current object should not trigger change.
								"id1": "B1",
								"id2": "B2",
								"foo": "foo",
							},
						},
						"listSet": []interface{}{
							"a",
							"b",
							"c", // item existing only in current object should not trigger change.
						},
						"atomicList": []interface{}{
							"a",
							"b",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
						"listSet": []interface{}{
							"a",
							"b",
						},
						"atomicList": []interface{}{
							"a",
							"b",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     false,
			wantHasSpecChanges: false,
		},

		{
			name: "List map item changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo-changed", // foo has been changed in the current intent.
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "List map item changed in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo-changed", // foo has been changed in the current intent.
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "List map item does not exists in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							// item with key id1:A1, id2:A2 does not exist in the current object.
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "List set item does not exists in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							// item "b" does not exist in the current object.
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							"b",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "AtomicList changed in the intent",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
							"b", // b added in the current intent.
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "AtomicList changed in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
							"b", // b added in the current object.
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
		{
			name: "(Atomic) List does not exists in the current object",
			ctx: &isChangingAnythingInput{
				path: contract.Path{},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						// atomic list does not exist in the current object.
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
						},
					},
				},
				schema: testSchema,
			},
			wantHasChanges:     true,
			wantHasSpecChanges: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotHasChanges, gotHasSpecChanges := isChangingAnything(tt.ctx)

			g.Expect(gotHasChanges).To(Equal(tt.wantHasChanges))
			g.Expect(gotHasSpecChanges).To(Equal(tt.wantHasSpecChanges))
		})
	}
}

// Test_isDroppingAnyIntent_basic checks if isDroppingAnyIntent handles basic combination of JSONSchemaProps that can be found in a CRD definition.
func Test_isDroppingAnyIntent_basic(t *testing.T) {
	tests := []struct {
		name                string
		ctx                 *isDroppingAnyIntentInput
		wantHasDeletion     bool
		wantHasSpecDeletion bool
	}{
		// Test struct & struct variants (atomic struct)

		{
			name: "No deletion in struct",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
							"bar": "bar-changed", // field changed should not be detected as a deletion.
							"baz": "baz",         // field added should not be detected as a deletion.
						},
						"atomicStruct": map[string]interface{}{
							// field deleted should not be detected as a deletion
							"bar": "bar-changed", // field changed should not be detected as a deletion.
							"baz": "baz",         // field added should not be detected as a deletion.
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detects field deletion in struct",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							// foo has been deleted.
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore field deletion in struct if the field has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"struct": map[string]interface{}{
						"foo": "foo",
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							// foo has been deleted.
						},
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"struct": map[string]interface{}{
							// The field has been already deleted from the current object.
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detects (atomic) struct deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						// struct has been deleted.
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore (atomic) struct deletion if the struct has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicStruct": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						// struct has been deleted.
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						// The struct has been already deleted from the current object.
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},

		// Test map & map variants (map of scalar, map of struct, map of map, map of list, atomic map)

		{
			name: "No deletion in map",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
							"bar": "bar",
						},
					},
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
							"bar": map[string]interface{}{
								"foo": "bar",
							},
						},
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
							"bar": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "bar",
								},
							},
						},
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
							"bar": []interface{}{
								"a",
							},
						},
						"atomicMap": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
							"bar": "bar-changed", // field changed should not be detected as a deletion.
							"baz": "baz",         // field added should not be detected as a deletion.
						},
					},
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
							"bar": map[string]interface{}{ // item changed should not be detected as a deletion.
								"foo": "bar-changed",
							},
							"baz": map[string]interface{}{ // item added should not be detected as a deletion.
								"foo": "foo",
							},
						},
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
							"bar": map[string]interface{}{ // item changed should not be detected as a deletion.
								"foo": map[string]interface{}{
									"foo": "bar-changed",
								},
							},
							"baz": map[string]interface{}{ // item added should not be detected as a deletion.
								"foo": map[string]interface{}{
									"foo": "baz",
								},
							},
						},
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
							"bar": []interface{}{ // item changed should not be detected as a deletion.
								"a-changed",
							},
							"baz": []interface{}{ // item added should not be detected as a deletion.
								"a",
							},
						},
						"atomicMap": map[string]interface{}{
							// item deleted should not be detected as a deletion.
							"bar": "bar-changed", // item changed should not be detected as a deletion.
							"baz": "baz",         // item added should not be detected as a deletion.
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
							"bar": "bar",
						},
					},
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
							"bar": map[string]interface{}{
								"foo": "bar",
							},
						},
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
							"bar": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "bar",
								},
							},
						},
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
							"bar": []interface{}{
								"a",
							},
						},
						"atomicMap": map[string]interface{}{
							"foo": "foo",
							"bar": "bar",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detect map of scalar item/metadata only deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: false,
		},
		{
			name: "Ignore map of scalar item/metadata only deletion if the field has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{ // mapOfScalars
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detect map of struct deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore map of struct deletion if the field has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfStruct": map[string]interface{}{
							// foo item has been already removed from current object
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detect map of map deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore map of map deletion if the field has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							"foo": map[string]interface{}{
								"foo": map[string]interface{}{
									"foo": "foo",
								},
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfMap": map[string]interface{}{
							// foo item has been already removed from current object
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detect map of list deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore map of map list if the field has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							"foo": []interface{}{
								"a",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							// foo item has been removed
						},
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"mapOfList": map[string]interface{}{
							// foo item has been already removed from current object
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detects (atomic) map deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						// map has been deleted.
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore (atomic) map deletion if the map has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicMap": map[string]interface{}{
							"foo": "foo",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						// map has been deleted.
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						// The struct map been already deleted from the current object.
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},

		// Test list & list variants (atomic list, list set, list map)

		{
			name: "No deletion in lists",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
								"bar": "bar",
							},
						},
						"listSet": []interface{}{
							"a",
							"b",
						},
						"atomicList": []interface{}{
							"a",
							"b",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
								"bar": "bar-changed", // field changed should not be detected as a deletion.
								"baz": "baz",         // field added should not be detected as a deletion.
							},
							map[string]interface{}{ // item added should not be detected as a deletion.
								"id1": "B1",
								"id2": "B2",
								"foo": "foo",
							},
						},
						"listSet": []interface{}{
							"a",
							"b",
							"c", // item added should not be detected as a deletion.
						},
						"atomicList": []interface{}{
							// item deleted should not be detected as a deletion
							"b",
							"c", // item added should not be detected as a deletion.
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
								"bar": "bar",
							},
						},
						"listSet": []interface{}{
							"a",
							"b",
						},
						"atomicList": []interface{}{
							"a",
							"b",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detects list map item deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							// item with key A1,A2 has been deleted.
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore list map item deletion if the list map item has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							// item with key A1,A2 has been deleted.
						},
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							// The  item with key A1,A2 has been already deleted from the current object
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detects field deletion inside list map item",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								// field foo in the item with key A1,A2 has been deleted.
							},
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore field deletion inside list map item if the field has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								"foo": "foo",
							},
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								// field foo in the item with key A1,A2 has been deleted.
							},
						},
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						"listMap": []interface{}{
							map[string]interface{}{
								"id1": "A1",
								"id2": "A2",
								// field foo in the item with key A1,A2 has been already deleted.
							},
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detects list set item deleted",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							"b",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							// item b has been deleted.
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							"b",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore list set item deletion if the item has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							"b",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							// item b has been deleted.
						},
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"listSet": []interface{}{
							"a",
							// item b has been already deleted.
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
		{
			name: "Detects (atomic) list deletion",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						// listMap has been deleted.
					},
				},
				currentObject: map[string]interface{}{
					// NOTE: It is required to keep the current object intentionally in sync with the previous intent (see note above).
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
						},
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     true,
			wantHasSpecDeletion: true,
		},
		{
			name: "Ignore (atomic) list deletion if the list has been already deleted from the current object",
			ctx: &isDroppingAnyIntentInput{
				path: contract.Path{},
				previousIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						"atomicList": []interface{}{
							"a",
						},
					},
				},
				currentIntent: map[string]interface{}{
					"spec": map[string]interface{}{
						// listMap has been deleted.
					},
				},
				currentObject: map[string]interface{}{
					"spec": map[string]interface{}{
						// The listMap has been already deleted from the current object.
					},
				},
				schema: testSchema,
			},
			wantHasDeletion:     false,
			wantHasSpecDeletion: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gotHasDeletion, gotHasSpecDeletion := isDroppingAnyIntent(tt.ctx)

			g.Expect(gotHasDeletion).To(Equal(tt.wantHasDeletion))
			g.Expect(gotHasSpecDeletion).To(Equal(tt.wantHasSpecDeletion))
		})
	}
}
