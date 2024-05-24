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
	"context"
	"testing"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func Test_ValidateClusterClassVariables(t *testing.T) {
	tests := []struct {
		name                  string
		clusterClassVariables []clusterv1.ClusterClassVariable
		wantErr               bool
	}{
		{
			name: "Error if multiple variables share a name",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "cpu",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Minimum: ptr.To[int64](1),
						},
					},
				},
				{
					Name: "cpu",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Minimum: ptr.To[int64](1),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass multiple variable validation",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "cpu",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Minimum: ptr.To[int64](1),
						},
					},
				},
				{
					Name: "validNumber",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "number",
							Maximum: ptr.To[int64](1),
						},
					},
				},

				{
					Name: "validVariable",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:      "string",
							MinLength: ptr.To[int64](1),
						},
					},
				},
				{
					Name: "location",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:      "string",
							MinLength: ptr.To[int64](1),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := ValidateClusterClassVariables(context.TODO(),
				tt.clusterClassVariables,
				field.NewPath("spec", "variables"))

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
		})
	}
}

func Test_ValidateClusterClassVariable(t *testing.T) {
	tests := []struct {
		name                 string
		clusterClassVariable *clusterv1.ClusterClassVariable
		wantErr              bool
	}{
		{
			name: "Valid integer schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Minimum: ptr.To[int64](1),
					},
				},
			},
		},
		{
			name: "Valid string schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "location",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
		},
		{
			name: "Valid variable name",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "validVariable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
		},
		{
			name: "fail on variable name is builtin",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "builtin",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on empty variable name",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on variable name containing dot (.)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "path.tovariable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid variable metadata",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "validVariable",
				Metadata: clusterv1.ClusterClassVariableMetadata{
					Labels: map[string]string{
						"label-key": "label-value",
					},
					Annotations: map[string]string{
						"annotation-key": "annotation-value",
					},
				},
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
		},
		{
			name: "fail on invalid variable label: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "path.tovariable",
				Metadata: clusterv1.ClusterClassVariableMetadata{
					Labels: map[string]string{
						".label-key": "label-value",
					},
				},
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on invalid variable annotation: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "path.tovariable",
				Metadata: clusterv1.ClusterClassVariableMetadata{
					Annotations: map[string]string{
						".annotation-key": "annotation-value",
					},
				},
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid default value regular string",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`"defaultValue"`),
						},
					},
				},
			},
		},
		{
			name: "fail on default value with invalid JSON",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`"defaultValue": "value"`), // invalid JSON
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid example value regular string",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Example: &apiextensionsv1.JSON{
							Raw: []byte(`"exampleValue"`),
						},
					},
				},
			},
		},
		{
			name: "fail on example value with invalid JSON",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Example: &apiextensionsv1.JSON{
							Raw: []byte(`"exampleValue": "value"`), // invalid JSON
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid enum values",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`"enumValue1"`)},
							{Raw: []byte(`"enumValue2"`)},
						},
					},
				},
			},
		},
		{
			name: "fail on enum value with invalid JSON",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`"defaultValue": "value"`)}, // invalid JSON
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on variable type is null",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "null",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on variable type is not valid",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "invalidVariableType",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on variable type length zero",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid object schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type:    "boolean",
								Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
							},
							"url": {
								Type: "string",
							},
							"noProxy": {
								Type: "string",
							},
						},
					},
				},
			},
		},
		{
			name: "fail on invalid object schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type:    "boolean",
								Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
							},
							"url": {
								Type: "string",
							},
							"noProxy": {
								Type: "invalidType", // invalid type.
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid map schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]clusterv1.JSONSchemaProps{
								"enabled": {
									Type:    "boolean",
									Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
								},
								"url": {
									Type: "string",
								},
								"noProxy": {
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail on invalid map schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]clusterv1.JSONSchemaProps{
								"enabled": {
									Type:    "boolean",
									Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
								},
								"url": {
									Type: "string",
								},
								"noProxy": {
									Type: "invalidType", // invalid type.
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on object (properties) and map (additionalProperties) both set",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]clusterv1.JSONSchemaProps{
								"enabled": {
									Type:    "boolean",
									Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
								},
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type:    "boolean",
								Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid array schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "arrayVariable",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type:    "boolean",
							Default: &apiextensionsv1.JSON{Raw: []byte(`false`)},
						},
					},
				},
			},
		},
		{
			name: "fail on invalid array schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "arrayVariable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type:    "string",
							Default: &apiextensionsv1.JSON{Raw: []byte(`invalidString`)}, // missing quotes.
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass on variable with required set true with a default defined",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "string",
						Default: &apiextensionsv1.JSON{Raw: []byte(`"defaultValue"`)},
					},
				},
			},
		},
		{
			name: "pass on variable with a default that is valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MaxLength: ptr.To[int64](6),
						Default:   &apiextensionsv1.JSON{Raw: []byte(`"short"`)},
					},
				},
			},
		},
		{
			name: "fail on variable with a default that is invalid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MaxLength: ptr.To[int64](6),
						Default:   &apiextensionsv1.JSON{Raw: []byte(`"veryLongValueIsInvalid"`)},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if variable is an object with default value valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Default: &apiextensionsv1.JSON{Raw: []byte(`100`)},
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail if variable is an object with default value invalidated by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Default: &apiextensionsv1.JSON{Raw: []byte(`-100`)},
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail if variable is an object with a top level default value invalidated by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`{"spec":{"replicas": -100}}`),
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if variable is an object with a top level default value valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`{"spec":{"replicas": 100}}`),
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail if field is required below properties and sets a default that misses the field",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type:     "object",
								Required: []string{"replicas"},
								Default: &apiextensionsv1.JSON{
									// replicas missing results in failure
									Raw: []byte(`{"value": 100}`),
								},
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Default: &apiextensionsv1.JSON{Raw: []byte(`100`)},
										Minimum: ptr.To[int64](1),
									},
									"value": {
										Type:    "integer",
										Default: &apiextensionsv1.JSON{Raw: []byte(`100`)},
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if field is required below properties and sets a default",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type:     "object",
								Required: []string{"replicas"},
								Default: &apiextensionsv1.JSON{
									// replicas is set here so the `required` property is met.
									Raw: []byte(`{"replicas": 100}`),
								},
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Default: &apiextensionsv1.JSON{Raw: []byte(`100`)},
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
		},

		{
			name: "pass on variable with an example that is valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MaxLength: ptr.To[int64](6),
						Example:   &apiextensionsv1.JSON{Raw: []byte(`"short"`)},
					},
				},
			},
		},
		{
			name: "fail on variable with an example that is invalid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MaxLength: ptr.To[int64](6),
						Example:   &apiextensionsv1.JSON{Raw: []byte(`"veryLongValueIsInvalid"`)},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if variable is an object with an example valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](0),
										Example: &apiextensionsv1.JSON{Raw: []byte(`100`)},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail if variable is an object with an example invalidated by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](0),
										Example: &apiextensionsv1.JSON{Raw: []byte(`-100`)},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail if variable is an object with a top level example value invalidated by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Example: &apiextensionsv1.JSON{
							Raw: []byte(`{"spec":{"replicas": -100}}`),
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if variable is an object with a top level default value valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Example: &apiextensionsv1.JSON{
							Raw: []byte(`{"spec":{"replicas": 100}}`),
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "pass on variable with an enum with all variables valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MaxLength: ptr.To[int64](6),
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`"short1"`)},
							{Raw: []byte(`"short2"`)},
						},
					},
				},
			},
		},
		{
			name: "fail on variable with an enum with a value that is invalid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MaxLength: ptr.To[int64](6),
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`"veryLongValueIsInvalid"`)},
							{Raw: []byte(`"short"`)},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if variable is an object with an enum value that is valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](0),
										Enum: []apiextensionsv1.JSON{
											{Raw: []byte(`100`)},
											{Raw: []byte(`5`)},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail if variable is an object with an enum value invalidated by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](0),
										Enum: []apiextensionsv1.JSON{
											{Raw: []byte(`100`)},
											{Raw: []byte(`-100`)},
										},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail if variable is an object with a top level enum value invalidated by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Enum: []apiextensionsv1.JSON{
							{
								Raw: []byte(`{"spec":{"replicas": 100}}`),
							},
							{
								Raw: []byte(`{"spec":{"replicas": -100}}`),
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "pass if variable is an object with a top level enum value that is valid by the given schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Enum: []apiextensionsv1.JSON{
							{
								Raw: []byte(`{"spec":{"replicas": 100}}`),
							},
							{
								Raw: []byte(`{"spec":{"replicas": 200}}`),
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"spec": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"replicas": {
										Type:    "integer",
										Minimum: ptr.To[int64](1),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := validateClusterClassVariable(context.TODO(),
				tt.clusterClassVariable,
				field.NewPath("spec", "variables").Index(0))

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
		})
	}
}
