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

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
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
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Minimum: pointer.Int64(1),
						},
					},
				},
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Minimum: pointer.Int64(1),
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
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Minimum: pointer.Int64(1),
						},
					},
				},
				{
					Name:     "validNumber",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "number",
							Maximum: pointer.Int64(1),
						},
					},
				},

				{
					Name:     "validVariable",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:      "string",
							MinLength: pointer.Int64(1),
						},
					},
				},
				{
					Name:     "location",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:      "string",
							MinLength: pointer.Int64(1),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := ValidateClusterClassVariables(tt.clusterClassVariables,
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
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Minimum: pointer.Int64(1),
					},
				},
			},
		},
		{
			name: "Valid string schema",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: pointer.Int64(1),
					},
				},
			},
		},
		{
			name: "Valid variable name",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "validVariable",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: pointer.Int64(1),
					},
				},
			},
		},
		{
			name: "fail on variable name is builtin",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "builtin",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: pointer.Int64(1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on empty variable name",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: pointer.Int64(1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on variable name containing dot (.)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "path.tovariable",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: pointer.Int64(1),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "fail on variable type is null",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
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
				Name:     "var",
				Required: true,
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
				Name:     "var",
				Required: true,
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
			name: "Invalid object schema",
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
								Type: "invalidType",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := validateClusterClassVariable(tt.clusterClassVariable,
				field.NewPath("spec", "variables").Index(0))

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
		})
	}
}
