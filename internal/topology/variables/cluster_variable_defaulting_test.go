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
		name            string
		definitions     []clusterv1.ClusterClassStatusVariable
		values          []clusterv1.ClusterVariable
		createVariables bool
		want            []clusterv1.ClusterVariable
		wantErr         bool
	}{
		{
			name:        "Return error if variable is not defined in ClusterClass",
			definitions: []clusterv1.ClusterClassStatusVariable{},
			values: []clusterv1.ClusterVariable{
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
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
					},
				},
				{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
					},
				},

				{
					Name: "count",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "number",
									Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
								},
							},
						},
					},
				},
				{
					Name: "correct",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "boolean",
									Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
								},
							},
						},
					},
				},
			},
			values:          []clusterv1.ClusterVariable{},
			createVariables: true,
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
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
					},
				},
			},
			values:          []clusterv1.ClusterVariable{},
			createVariables: false,
			want:            []clusterv1.ClusterVariable{},
		},
		{
			name: "Don't default variables that are set",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
					},
				},
				{
					Name: "correct",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "boolean",
									Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
								},
							},
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
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
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
					},
				},
				{
					Name: "correct",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "boolean",
								},
							},
						},
					},
				},
			},
			values:          []clusterv1.ClusterVariable{},
			createVariables: true,
			want: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
				},
			},
		},
		// Variables with multiple non-conflicting definitions.
		{
			name: "Don't default if a value is set with empty definitionFrom",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
						{
							Required: true,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					// This variable does not have definitionConflicts. An unset definitionFrom is valid.
				},
			},
			createVariables: true,
			want: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						// Expect value set in Cluster.
						Raw: []byte(`2`),
					},
				},
			},
		},
		{
			name: "Default many non-conflicting variables to one value in Cluster",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
						{

							Required: true,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
						{

							Required: true,
							From:     "otherpatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
					},
				},
			},
			values:          []clusterv1.ClusterVariable{},
			createVariables: true,
			want: []clusterv1.ClusterVariable{
				{
					// As this variable is non-conflicting it can be set only once in the Cluster through defaulting.
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
				},
			},
		},
		{
			name: "Default many non-conflicting definitions to many values in Cluster when some values are set with definitionFrom",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
						{
							Required: true,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				// The variable is set with definitionFrom for one of two definitions.
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: clusterv1.VariableDefinitionFromInline,
				},
			},
			createVariables: true,
			want: []clusterv1.ClusterVariable{
				// Expect the value in the Cluster to be retained.
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: clusterv1.VariableDefinitionFromInline,
				},
				// Expect defaulting for the definition with no value in the Cluster.
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
					DefinitionFrom: "somepatch",
				},
			},
		},
		// Variables with conflicting definitions.
		{
			name: "Default many conflicting definitions to many values in Cluster",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: true,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
						{
							// Variable isn't required, but should still be defaulted according to its schema.
							Required: false,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`2`)},
								},
							},
						},
					},
				},
			},
			values:          []clusterv1.ClusterVariable{},
			createVariables: true,
			want: []clusterv1.ClusterVariable{
				// As this variable has conflicting definitions expect one value in the Cluster
				// for each "DefinitionFrom".
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
					DefinitionFrom: clusterv1.VariableDefinitionFromInline,
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: "somepatch",
				},
			},
		},
		{
			name: "Default many conflicting definitions to many values in Cluster when some values are set with definitionFrom",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: true,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
								},
							},
						},
						{
							// Variable isn't required, but should still be defaulted according to its schema.
							Required: false,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "integer",
									Default: &apiextensionsv1.JSON{Raw: []byte(`2`)},
								},
							},
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`3`),
					},
					DefinitionFrom: clusterv1.VariableDefinitionFromInline,
				},
			},
			createVariables: true,
			want: []clusterv1.ClusterVariable{
				// As this variable has conflicting definitions expect one value in the Cluster
				// for each "DefinitionFrom" and retain values set in Cluster.
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`3`),
					},
					DefinitionFrom: clusterv1.VariableDefinitionFromInline,
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: "somepatch",
				},
			},
		},
		{
			name:    "Error if a value is set with empty and non-empty definitionFrom.",
			wantErr: true,
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From: "somepatch",
						},
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From: clusterv1.VariableDefinitionFromInline,
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
					// Mix of empty and non-empty definitionFrom is not valid.
					DefinitionFrom: "",
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: "somepatch",
				},
			},
		},
		{
			name:    "Error if a value is set twice with the same definitionFrom.",
			wantErr: true,
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From: "somepatch",
						},
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From: clusterv1.VariableDefinitionFromInline,
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				// Identical definitionFrom and name for two different values.
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
					DefinitionFrom: "",
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: "",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			vars, errList := defaultClusterVariables(tt.values, tt.definitions, tt.createVariables,
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
		clusterClassVariable *statusVariableDefinition
		createVariable       bool
		want                 *clusterv1.ClusterVariable
		wantErr              bool
	}{
		{
			name: "Default new integer variable",
			clusterClassVariable: &statusVariableDefinition{
				Name: "cpu",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "cpu",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
					},
				},
			},
			createVariable: false,
			want:           nil,
		},
		{
			name: "Don't default existing integer variable",
			clusterClassVariable: &statusVariableDefinition{
				Name: "cpu",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "integer",
							Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "location",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "string",
							Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "location",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "string",
							Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "cpu",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "number",
							Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "cpu",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "number",
							Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "correct",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "boolean",
							Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "correct",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "boolean",
							Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
						},
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "httpProxy",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "httpProxy",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
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
			createVariable: true,
			want:           nil,
		},
		{
			name: "Don't default existing object variable",
			clusterClassVariable: &statusVariableDefinition{
				Name: "httpProxy",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "httpProxy",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{

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
			name: "Default new map variable",
			clusterClassVariable: &statusVariableDefinition{
				Name: "httpProxy",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "object",
							Default: &apiextensionsv1.JSON{Raw: []byte(`{"proxy":{"enabled":false}}`)},
							AdditionalProperties: &clusterv1.JSONSchemaProps{
								Type: "object",
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
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"proxy":{"enabled":false,"url":"https://example.com"}}`),
				},
			},
		},
		{
			name: "Default nested fields of existing map variable",
			clusterClassVariable: &statusVariableDefinition{
				Name: "httpProxy",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "object",
							AdditionalProperties: &clusterv1.JSONSchemaProps{
								Type: "object",
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
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"proxy1":{"enabled":false},"proxy2":{"enabled":false}}`),
				},
			},
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"proxy1":{"enabled":false,"url":"https://example.com"},"proxy2":{"enabled":false,"url":"https://example.com"}}`),
				},
			},
		},
		{
			name: "Default new array variable",
			clusterClassVariable: &statusVariableDefinition{
				Name: "testVariable",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
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
			clusterClassVariable: &statusVariableDefinition{
				Name: "testVariable",
				ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{

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

			defaultedVariable, errList := defaultValue(tt.clusterVariable, tt.clusterClassVariable,
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

func Test_getAllVariables(t *testing.T) {
	g := NewWithT(t)
	t.Run("Expect values to be correctly consolidated in allVariables", func(t *testing.T) {
		expectedValues := []clusterv1.ClusterVariable{
			// var1 has a value with no DefinitionFrom set and only one definition. It should be retained as is.
			{
				Name: "var1",
			},

			// var2 has a value with DefinitionFrom "inline". It has a second definition with DefinitionFrom "somepatch".
			// The value for the first definition should be retained and a value for the second definition should be added.
			{
				Name:           "var2",
				DefinitionFrom: clusterv1.VariableDefinitionFromInline,
			},
			{
				Name:           "var2",
				DefinitionFrom: "somepatch",
			},

			// var3 had no values. It has two conflicting definitions. A value for each definition should be added.
			{
				Name:           "var3",
				DefinitionFrom: clusterv1.VariableDefinitionFromInline,
			},
			{
				Name:           "var3",
				DefinitionFrom: "somepatch",
			},

			// var4 had no values. It has two non-conflicting definitions. A single value with emptyDefinitionFrom should
			// be added.
			{
				Name:           "var4",
				DefinitionFrom: emptyDefinitionFrom,
			},
		}

		values := []clusterv1.ClusterVariable{
			{
				Name: "var1",
			},
			{
				Name:           "var2",
				DefinitionFrom: clusterv1.VariableDefinitionFromInline,
			},
		}
		definitions := []clusterv1.ClusterClassStatusVariable{
			{
				Name: "var1",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: false,
						From:     clusterv1.VariableDefinitionFromInline,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				},
			},
			{
				Name: "var2",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						From:     clusterv1.VariableDefinitionFromInline,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					{
						Required: true,
						From:     "somepatch",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				},
			},
			{
				Name:                "var3",
				DefinitionsConflict: true,
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: false,
						From:     clusterv1.VariableDefinitionFromInline,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					{
						Required: true,
						From:     "somepatch",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				},
			},
			{
				Name: "var4",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						From:     clusterv1.VariableDefinitionFromInline,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					{
						Required: true,
						From:     "somepatch",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				},
			},
		}

		valuesIndex, err := newValuesIndex(values)
		g.Expect(err).To(BeNil())
		got := getAllVariables(values, valuesIndex, definitions)
		g.Expect(got).To(Equal(expectedValues))
	})
}
