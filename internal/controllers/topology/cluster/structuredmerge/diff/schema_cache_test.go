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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// Test_addToSchema_basic test add to schema handles basic combination of JSONSchemaProps that can be found in a CRD definition.
// TODO: generate CRD definition from go types and load them, so we cover the generation process vs making assumption on the generated JSONSchemaProps.
func Test_addToSchema_basic(t *testing.T) {
	tests := []struct {
		name       string
		crd        *apiextensionsv1.JSONSchemaProps
		wantSchema CRDSchema
	}{
		// Test struct & struct variants (atomic struct)

		{
			name: "Adds struct, scalar and nested structs",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"apiVersion": {Type: "string"},
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooScalar": {Type: "string"},
							"fooStruct": {
								Type: "object",
								// Struct are granular by default
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"fooScalar": {Type: "string"},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                         TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":               TypeDef{Scalar: &ScalarDef{}},
				"kind":                     TypeDef{Scalar: &ScalarDef{}},
				"spec":                     TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooScalar":           TypeDef{Scalar: &ScalarDef{}},
				"spec.fooStruct":           TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooStruct.fooScalar": TypeDef{Scalar: &ScalarDef{}},
			},
		},
		{
			name: "Adds atomic struct",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooAtomicStruct": {
								Type:     "object",
								XMapType: pointer.StringPtr("atomic"), // XMapType applies to struct as well.
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"fooScalar": {Type: "string"},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                     TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":           TypeDef{Scalar: &ScalarDef{}},
				"kind":                 TypeDef{Scalar: &ScalarDef{}},
				"spec":                 TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooAtomicStruct": TypeDef{Struct: &StructDef{Type: AtomicStructType}},
				// no further detail are processed for fooAtomicStruct.
			},
		},

		// Test map & map variants (map of scalar, map of struct, map of map, map of list, atomic map)

		{
			name: "Adds map with scalar items",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooMap": {
								Type: "object",
								// Map are granular by default
								AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "bool",
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":              TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":    TypeDef{Scalar: &ScalarDef{}},
				"kind":          TypeDef{Scalar: &ScalarDef{}},
				"spec":          TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooMap":   TypeDef{Map: &MapDef{Type: GranularMapType}},
				"spec.fooMap[]": TypeDef{Scalar: &ScalarDef{}},
			},
		},
		{
			name: "Adds map with struct items",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooMap": {
								Type: "object",
								// Map are granular by default
								AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "object",
										Properties: map[string]apiextensionsv1.JSONSchemaProps{
											"fooScalar": {Type: "string"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                        TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":              TypeDef{Scalar: &ScalarDef{}},
				"kind":                    TypeDef{Scalar: &ScalarDef{}},
				"spec":                    TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooMap":             TypeDef{Map: &MapDef{Type: GranularMapType}},
				"spec.fooMap[]":           TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooMap[].fooScalar": TypeDef{Scalar: &ScalarDef{}},
			},
		},
		{
			name: "Adds map with map items",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooMap": {
								Type: "object",
								// Map are granular by default
								AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "object",
										AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
											Schema: &apiextensionsv1.JSONSchemaProps{
												Type: "object",
												Properties: map[string]apiextensionsv1.JSONSchemaProps{
													"fooScalar": {Type: "string"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                          TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":                TypeDef{Scalar: &ScalarDef{}},
				"kind":                      TypeDef{Scalar: &ScalarDef{}},
				"spec":                      TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooMap":               TypeDef{Map: &MapDef{Type: GranularMapType}},
				"spec.fooMap[]":             TypeDef{Map: &MapDef{Type: GranularMapType}},
				"spec.fooMap[][]":           TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooMap[][].fooScalar": TypeDef{Scalar: &ScalarDef{}},
			},
		},
		{
			name: "Adds map with list items",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooMap": {
								Type: "object",
								// Map are granular by default
								AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "array",
										Items: &apiextensionsv1.JSONSchemaPropsOrArray{
											Schema: &apiextensionsv1.JSONSchemaProps{
												Type: "bool",
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":              TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":    TypeDef{Scalar: &ScalarDef{}},
				"kind":          TypeDef{Scalar: &ScalarDef{}},
				"spec":          TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooMap":   TypeDef{Map: &MapDef{Type: GranularMapType}},
				"spec.fooMap[]": TypeDef{List: &ListDef{Type: AtomicListType}},
				// no further detail are processed for spec.fooMap[] (it is an atomic list).
			},
		},
		{
			name: "Adds atomic map",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooAtomicMap": {
								Type:     "object",
								XMapType: pointer.StringPtr("atomic"),
								AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "bool",
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                  TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":        TypeDef{Scalar: &ScalarDef{}},
				"kind":              TypeDef{Scalar: &ScalarDef{}},
				"spec":              TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooAtomicMap": TypeDef{Map: &MapDef{Type: AtomicMapType}},
				// no further detail are processed for fooAtomicMap.
			},
		},

		// Test list & list variants (atomic list, list set, list map)

		{
			name: "Adds atomic list",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooAtomicList": {
								Type: "array",
								// List are atomic by default
								Items: &apiextensionsv1.JSONSchemaPropsOrArray{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "bool",
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                   TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":         TypeDef{Scalar: &ScalarDef{}},
				"kind":               TypeDef{Scalar: &ScalarDef{}},
				"spec":               TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooAtomicList": TypeDef{List: &ListDef{Type: AtomicListType}},
				// no further detail are processed for fooAtomicList.
			},
		},
		{
			name: "Adds list set",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooListSet": {
								Type:      "array",
								XListType: pointer.String("set"),
								Items: &apiextensionsv1.JSONSchemaPropsOrArray{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "string",
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":      TypeDef{Scalar: &ScalarDef{}},
				"kind":            TypeDef{Scalar: &ScalarDef{}},
				"spec":            TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooListSet": TypeDef{List: &ListDef{Type: ListSetType}},
				// no further detail are processed for fooListSet, they are scalar by default.
			},
		},
		{
			name: "Adds list map",
			crd: &apiextensionsv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"spec": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"fooListMap": {
								Type:         "array",
								XListType:    pointer.String("map"),
								XListMapKeys: []string{"fooScalar"},
								Items: &apiextensionsv1.JSONSchemaPropsOrArray{
									Schema: &apiextensionsv1.JSONSchemaProps{
										Type: "object",
										Properties: map[string]apiextensionsv1.JSONSchemaProps{
											"fooScalar": {Type: "string"},
											"barScalar": {Type: "string"},
										},
									},
								},
							},
						},
					},
				},
			},
			wantSchema: CRDSchema{
				"":                            TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"apiVersion":                  TypeDef{Scalar: &ScalarDef{}},
				"kind":                        TypeDef{Scalar: &ScalarDef{}},
				"spec":                        TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooListMap":             TypeDef{List: &ListDef{Type: ListMapType, Keys: []string{"fooScalar"}}},
				"spec.fooListMap[]":           TypeDef{Struct: &StructDef{Type: GranularStructType}},
				"spec.fooListMap[].fooScalar": TypeDef{Scalar: &ScalarDef{}},
				"spec.fooListMap[].barScalar": TypeDef{Scalar: &ScalarDef{}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := CRDSchema{}
			addToSchema(contract.Path{}, scheme, tt.crd)

			g.Expect(scheme).To(Equal(tt.wantSchema))
		})
	}
}
