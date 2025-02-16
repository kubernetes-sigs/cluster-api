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

package variables

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func Test_convertToAPIExtensionsJSONSchemaProps(t *testing.T) {
	defaultJSON := apiextensions.JSON(`defaultValue`)

	tests := []struct {
		name    string
		schema  *clusterv1.JSONSchemaProps
		want    *apiextensions.JSONSchemaProps
		wantErr bool
	}{
		{
			name: "pass for basic schema validation",
			schema: &clusterv1.JSONSchemaProps{
				Type:             "integer",
				Format:           "uri",
				MaxLength:        ptr.To[int64](4),
				MinLength:        ptr.To[int64](2),
				Pattern:          "abc.*",
				Maximum:          ptr.To[int64](43),
				ExclusiveMaximum: true,
				Minimum:          ptr.To[int64](1),
				ExclusiveMinimum: false,
				OneOf: []clusterv1.JSONSchemaProps{{
					Required: []string{"property1", "property2"},
				}, {
					Required: []string{"property3", "property4"},
				}},
				AnyOf: []clusterv1.JSONSchemaProps{{
					Required: []string{"property1", "property2"},
				}, {
					Required: []string{"property3", "property4"},
				}},
				AllOf: []clusterv1.JSONSchemaProps{{
					Required: []string{"property1", "property2"},
				}, {
					Required: []string{"property3", "property4"},
				}},
				Not: &clusterv1.JSONSchemaProps{
					Required: []string{"property1", "property2"},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Type:             "integer",
				Format:           "uri",
				MaxLength:        ptr.To[int64](4),
				MinLength:        ptr.To[int64](2),
				Pattern:          "abc.*",
				Maximum:          ptr.To[float64](43),
				ExclusiveMaximum: true,
				Minimum:          ptr.To[float64](1),
				ExclusiveMinimum: false,
				OneOf: []apiextensions.JSONSchemaProps{{
					Required: []string{"property1", "property2"},
				}, {
					Required: []string{"property3", "property4"},
				}},
				AnyOf: []apiextensions.JSONSchemaProps{{
					Required: []string{"property1", "property2"},
				}, {
					Required: []string{"property3", "property4"},
				}},
				AllOf: []apiextensions.JSONSchemaProps{{
					Required: []string{"property1", "property2"},
				}, {
					Required: []string{"property3", "property4"},
				}},
				Not: &apiextensions.JSONSchemaProps{
					Required: []string{"property1", "property2"},
				},
			},
		},
		{
			name: "pass for schema validation with enum & default",
			schema: &clusterv1.JSONSchemaProps{
				Example: &apiextensionsv1.JSON{
					Raw: []byte(`"defaultValue"`),
				},
				Default: &apiextensionsv1.JSON{
					Raw: []byte(`"defaultValue"`),
				},
				Enum: []apiextensionsv1.JSON{
					{Raw: []byte(`"enumValue1"`)},
					{Raw: []byte(`"enumValue2"`)},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Default: &defaultJSON,
				Example: &defaultJSON,
				Enum: []apiextensions.JSON{
					`enumValue1`,
					`enumValue2`,
				},
			},
		},
		{
			name: "fail for schema validation with default value with invalid JSON",
			schema: &clusterv1.JSONSchemaProps{
				Default: &apiextensionsv1.JSON{
					Raw: []byte(`defaultValue`), // missing quotes
				},
			},
			wantErr: true,
		},
		{
			name: "pass for schema validation with object",
			schema: &clusterv1.JSONSchemaProps{
				Properties: map[string]clusterv1.JSONSchemaProps{
					"property1": {
						Type:    "integer",
						Minimum: ptr.To[int64](1),
					},
					"property2": {
						Type:          "string",
						Format:        "uri",
						MinProperties: ptr.To[int64](2),
						MaxProperties: ptr.To[int64](4),
						OneOf: []clusterv1.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						AnyOf: []clusterv1.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						AllOf: []clusterv1.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						Not: &clusterv1.JSONSchemaProps{
							Required: []string{"property1", "property2"},
						},
					},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Properties: map[string]apiextensions.JSONSchemaProps{
					"property1": {
						Type:    "integer",
						Minimum: ptr.To[float64](1),
					},
					"property2": {
						Type:          "string",
						Format:        "uri",
						MinProperties: ptr.To[int64](2),
						MaxProperties: ptr.To[int64](4),
						OneOf: []apiextensions.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						AnyOf: []apiextensions.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						AllOf: []apiextensions.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						Not: &apiextensions.JSONSchemaProps{
							Required: []string{"property1", "property2"},
						},
					},
				},
			},
		},
		{
			name: "pass for schema validation with map",
			schema: &clusterv1.JSONSchemaProps{
				AdditionalProperties: &clusterv1.JSONSchemaProps{
					Properties: map[string]clusterv1.JSONSchemaProps{
						"property1": {
							Type:    "integer",
							Minimum: ptr.To[int64](1),
						},
						"property2": {
							Type:          "string",
							Format:        "uri",
							MinProperties: ptr.To[int64](2),
							MaxProperties: ptr.To[int64](4),
							OneOf: []clusterv1.JSONSchemaProps{{
								Required: []string{"property1", "property2"},
							}, {
								Required: []string{"property3", "property4"},
							}},
							AnyOf: []clusterv1.JSONSchemaProps{{
								Required: []string{"property1", "property2"},
							}, {
								Required: []string{"property3", "property4"},
							}},
							AllOf: []clusterv1.JSONSchemaProps{{
								Required: []string{"property1", "property2"},
							}, {
								Required: []string{"property3", "property4"},
							}},
							Not: &clusterv1.JSONSchemaProps{
								Required: []string{"property1", "property2"},
							},
						},
					},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				AdditionalProperties: &apiextensions.JSONSchemaPropsOrBool{
					Allows: true,
					Schema: &apiextensions.JSONSchemaProps{
						Properties: map[string]apiextensions.JSONSchemaProps{
							"property1": {
								Type:    "integer",
								Minimum: ptr.To[float64](1),
							},
							"property2": {
								Type:          "string",
								Format:        "uri",
								MinProperties: ptr.To[int64](2),
								MaxProperties: ptr.To[int64](4),
								OneOf: []apiextensions.JSONSchemaProps{{
									Required: []string{"property1", "property2"},
								}, {
									Required: []string{"property3", "property4"},
								}},
								AnyOf: []apiextensions.JSONSchemaProps{{
									Required: []string{"property1", "property2"},
								}, {
									Required: []string{"property3", "property4"},
								}},
								AllOf: []apiextensions.JSONSchemaProps{{
									Required: []string{"property1", "property2"},
								}, {
									Required: []string{"property3", "property4"},
								}},
								Not: &apiextensions.JSONSchemaProps{
									Required: []string{"property1", "property2"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "pass for schema validation with array",
			schema: &clusterv1.JSONSchemaProps{
				Items: &clusterv1.JSONSchemaProps{
					Type:      "integer",
					Minimum:   ptr.To[int64](1),
					Format:    "uri",
					MinLength: ptr.To[int64](2),
					MaxLength: ptr.To[int64](4),
					OneOf: []clusterv1.JSONSchemaProps{{
						Required: []string{"property1", "property2"},
					}, {
						Required: []string{"property3", "property4"},
					}},
					AnyOf: []clusterv1.JSONSchemaProps{{
						Required: []string{"property1", "property2"},
					}, {
						Required: []string{"property3", "property4"},
					}},
					AllOf: []clusterv1.JSONSchemaProps{{
						Required: []string{"property1", "property2"},
					}, {
						Required: []string{"property3", "property4"},
					}},
					Not: &clusterv1.JSONSchemaProps{
						Required: []string{"property1", "property2"},
					},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Items: &apiextensions.JSONSchemaPropsOrArray{
					Schema: &apiextensions.JSONSchemaProps{
						Type:      "integer",
						Minimum:   ptr.To[float64](1),
						Format:    "uri",
						MinLength: ptr.To[int64](2),
						MaxLength: ptr.To[int64](4),
						OneOf: []apiextensions.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						AnyOf: []apiextensions.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						AllOf: []apiextensions.JSONSchemaProps{{
							Required: []string{"property1", "property2"},
						}, {
							Required: []string{"property3", "property4"},
						}},
						Not: &apiextensions.JSONSchemaProps{
							Required: []string{"property1", "property2"},
						},
					},
				},
			},
		},
		{
			name: "pass for schema validation with CEL validation rules",
			schema: &clusterv1.JSONSchemaProps{
				Items: &clusterv1.JSONSchemaProps{
					Type:      "integer",
					Minimum:   ptr.To[int64](1),
					Format:    "uri",
					MinLength: ptr.To[int64](2),
					MaxLength: ptr.To[int64](4),
					XValidations: []clusterv1.ValidationRule{{
						Rule:              "self > 0",
						Message:           "value must be greater than 0",
						MessageExpression: "value must be greater than 0",
						FieldPath:         "a.field.path",
					}, {
						Rule:              "self > 0",
						Message:           "value must be greater than 0",
						MessageExpression: "value must be greater than 0",
						FieldPath:         "a.field.path",
						Reason:            clusterv1.FieldValueErrorReason("a reason"),
					}},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Items: &apiextensions.JSONSchemaPropsOrArray{
					Schema: &apiextensions.JSONSchemaProps{
						Type:      "integer",
						Minimum:   ptr.To[float64](1),
						Format:    "uri",
						MinLength: ptr.To[int64](2),
						MaxLength: ptr.To[int64](4),
						XValidations: apiextensions.ValidationRules{{
							Rule:              "self > 0",
							Message:           "value must be greater than 0",
							MessageExpression: "value must be greater than 0",
							FieldPath:         "a.field.path",
						}, {
							Rule:              "self > 0",
							Message:           "value must be greater than 0",
							MessageExpression: "value must be greater than 0",
							FieldPath:         "a.field.path",
							Reason:            ptr.To(apiextensions.FieldValueErrorReason("a reason")),
						}},
					},
				},
			},
		}, {
			name: "pass for schema validation with XIntOrString",
			schema: &clusterv1.JSONSchemaProps{
				Items: &clusterv1.JSONSchemaProps{
					XIntOrString: true,
					AnyOf: []clusterv1.JSONSchemaProps{{
						Type: "integer",
					}, {
						Type: "string",
					}},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Items: &apiextensions.JSONSchemaPropsOrArray{
					Schema: &apiextensions.JSONSchemaProps{
						XIntOrString: true,
						AnyOf: []apiextensions.JSONSchemaProps{{
							Type: "integer",
						}, {
							Type: "string",
						}},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errs := convertToAPIExtensionsJSONSchemaProps(tt.schema, field.NewPath(""))
			if tt.wantErr {
				if len(errs) == 0 {
					t.Errorf("convertToAPIExtensionsJSONSchemaProps() error = %v, wantErr %v", errs, tt.wantErr)
				}
				return
			}
			if !cmp.Equal(got, tt.want) {
				t.Errorf("convertToAPIExtensionsJSONSchemaProps() got = %v, want %v", got, tt.want)
			}
		})
	}
}
