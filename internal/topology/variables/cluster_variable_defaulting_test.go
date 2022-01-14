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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func Test_DefaultClusterVariables(t *testing.T) {
	tests := []struct {
		name                  string
		clusterClassVariables []clusterv1.ClusterClassVariable
		clusterVariables      []clusterv1.ClusterVariable
		createVariables       bool
		want                  []clusterv1.ClusterVariable
		wantErr               bool
	}{
		{
			name:                  "Return error if variable is not defined in ClusterClass",
			clusterClassVariables: []clusterv1.ClusterClassVariable{},
			clusterVariables: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
				},
			},
			createVariables: true,
			wantErr:         true,
		},
		{
			name: "Default one variable of each valid type",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
				{
					Name:     "location",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "string",
							Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
						},
					},
				},

				{
					Name:     "count",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "number",
							Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
						},
					},
				},
				{
					Name:     "correct",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "boolean",
							Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
						},
					},
				},
			},
			clusterVariables: []clusterv1.ClusterVariable{},
			createVariables:  true,
			want: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
				},
				{
					Name: "location",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"us-east"`),
					},
				},
				{
					Name: "count",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`0.1`),
					},
				},
				{
					Name: "correct",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`true`),
					},
				},
			},
		},
		{
			name: "Don't default variable if variable creation is disabled",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
			clusterVariables: []clusterv1.ClusterVariable{},
			createVariables:  false,
			want:             []clusterv1.ClusterVariable{},
		},
		{
			name: "Don't default variables that are set",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
				{
					Name:     "correct",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "boolean",
							Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
						},
					},
				},
			},
			clusterVariables: []clusterv1.ClusterVariable{
				{
					Name: "correct",

					// Value is set here and shouldn't be defaulted.
					Value: apiextensionsv1.JSON{
						Raw: []byte(`false`),
					},
				},
			},
			createVariables: true,
			want: []clusterv1.ClusterVariable{
				{
					Name: "correct",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`false`),
					},
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
				},
			},
		},
		{
			name: "Don't add variables that have no default schema",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
				{
					Name:     "correct",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "boolean",
						},
					},
				},
			},
			clusterVariables: []clusterv1.ClusterVariable{},
			createVariables:  true,
			want: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			vars, errList := defaultClusterVariables(tt.clusterVariables, tt.clusterClassVariables, tt.createVariables,
				field.NewPath("spec", "topology", "variables"))

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
			g.Expect(vars).To(Equal(tt.want))
		})
	}
}

func Test_DefaultClusterVariable(t *testing.T) {
	tests := []struct {
		name                 string
		clusterVariable      *clusterv1.ClusterVariable
		clusterClassVariable *clusterv1.ClusterClassVariable
		createVariable       bool
		want                 *clusterv1.ClusterVariable
		wantErr              bool
	}{
		{
			name: "Default new integer variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
					},
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`1`),
				},
			},
		},
		{
			name: "Don't default new integer variable if variable creation is disabled",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
					},
				},
			},
			createVariable: false,
			want:           nil,
		},
		{
			name: "Don't default existing integer variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`2`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`2`),
				},
			},
		},
		{
			name: "Default new string variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "string",
						Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
					},
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"us-east"`),
				},
			},
		},
		{
			name: "Don't default existing string variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "string",
						Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"us-west"`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"us-west"`),
				},
			},
		},
		{
			name: "Default new number variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "number",
						Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
					},
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`0.1`),
				},
			},
		},
		{
			name: "Don't default existing number variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "number",
						Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`2.1`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`2.1`),
				},
			},
		},
		{
			name: "Default new boolean variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "correct",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "boolean",
						Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
					},
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "correct",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`true`),
				},
			},
		},
		{
			name: "Don't default existing boolean variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "correct",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "boolean",
						Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "correct",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`false`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "correct",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`false`),
				},
			},
		},
		{
			name: "Default new object variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "object",
						Default: &apiextensionsv1.JSON{Raw: []byte(`{"enabled": false}`)},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "boolean",
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
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"enabled":false}`),
				},
			},
		},
		{
			name: "Don't default new object variable if there is no top-level default",
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
			createVariable: true,
			want:           nil,
		},
		{
			name: "Don't default existing object variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "object",
						Default: &apiextensionsv1.JSON{Raw: []byte(`{"enabled": false}`)},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "boolean",
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
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"enabled":false,"url":"https://example.com"}`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"enabled":false,"url":"https://example.com"}`),
				},
			},
		},
		{
			name: "Default nested fields of existing object variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "object",
						Default: &apiextensionsv1.JSON{Raw: []byte(`{"enabled": false}`)},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "boolean",
							},
							"url": {
								Type:    "string",
								Default: &apiextensionsv1.JSON{Raw: []byte(`"https://example.com"`)},
							},
							"noProxy": {
								Type: "string",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"enabled":false}`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"enabled":false,"url":"https://example.com"}`),
				},
			},
		},
		{
			name: "Default new array variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testVariable",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]clusterv1.JSONSchemaProps{
								"enabled": {
									Type: "boolean",
								},
								"url": {
									Type: "string",
								},
							},
						},
						Default: &apiextensionsv1.JSON{Raw: []byte(`[{"enabled":false,"url":"123"},{"enabled":false,"url":"456"}]`)},
					},
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "testVariable",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`[{"enabled":false,"url":"123"},{"enabled":false,"url":"456"}]`),
				},
			},
		},
		{
			name: "Don't default existing array variable",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testVariable",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]clusterv1.JSONSchemaProps{
								"enabled": {
									Type: "boolean",
								},
								"url": {
									Type: "string",
								},
							},
						},
						Default: &apiextensionsv1.JSON{Raw: []byte(`[{"enabled":false,"url":"123"},{"enabled":false,"url":"456"}]`)},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testVariable",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`[]`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "testVariable",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`[]`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			defaultedVariable, errList := defaultClusterVariable(tt.clusterVariable, tt.clusterClassVariable,
				field.NewPath("spec", "topology", "variables").Index(0), tt.createVariable)

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())

			g.Expect(defaultedVariable).To(Equal(tt.want))
		})
	}
}
