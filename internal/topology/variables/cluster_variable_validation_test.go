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

func Test_ValidateClusterVariables(t *testing.T) {
	tests := []struct {
		name             string
		definitions      []clusterv1.ClusterClassStatusVariable
		values           []clusterv1.ClusterVariable
		validateRequired bool
		wantErr          bool
	}{
		{
			name: "Pass for a number of valid values.",
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
									Minimum: pointer.Int64(1),
								},
							},
						},
					},
				},
				{
					Name: "zone",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:      "string",
									MinLength: pointer.Int64(1),
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
									Type: "string",
									Enum: []apiextensionsv1.JSON{
										{Raw: []byte(`"us-east-1"`)},
										{Raw: []byte(`"us-east-2"`)},
									},
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
						Raw: []byte(`1`),
					},
				},
				{
					Name: "zone",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"longerThanOneCharacter"`),
					},
				},
				{
					Name: "location",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"us-east-1"`),
					},
				},
			},
			validateRequired: true,
		},
		{
			name:    "Error when no value for required definition.",
			wantErr: true,
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
									Minimum: pointer.Int64(1),
								},
							},
						},
					},
				},
				{
					Name: "zone",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:      "string",
									MinLength: pointer.Int64(1),
								},
							},
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				// cpu is missing in the values but is required in definition.
				{
					Name: "zone",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"longerThanOneCharacter"`),
					},
				},
			},
			validateRequired: true,
		},
		{
			name: "Pass if validateRequired='false' and no value for required definition.",
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
									Minimum: pointer.Int64(1),
								},
							},
						},
					},
				},
				{
					Name: "zone",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:      "string",
									MinLength: pointer.Int64(1),
								},
							},
						},
					},
				},
			},

			values: []clusterv1.ClusterVariable{
				// cpu is missing in the values and is required in definitions,
				// but required validation is disabled.
				{
					Name: "zone",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"longerThanOneCharacter"`),
					},
				},
			},
			validateRequired: false,
		},
		{
			name:        "Error if value has no definition.",
			wantErr:     true,
			definitions: []clusterv1.ClusterClassStatusVariable{},
			values: []clusterv1.ClusterVariable{
				// location has a value but no definition.
				{
					Name: "location",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"us-east-1"`),
					},
				},
			},
			validateRequired: true,
		},
		// Non-conflicting definition tests.
		{
			name: "Pass if a value with empty definitionFrom set for a non-conflicting definition",
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
							From:     clusterv1.VariableDefinitionFromInline,
							Required: true,
						},
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From:     "somepatch",
							Required: true,
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
				},
			},
			validateRequired: true,
		},
		{
			name: "Pass when there is a separate value for each required definition with no definition conflicts.",
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
							From:     clusterv1.VariableDefinitionFromInline,
							Required: true,
						},
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From:     "somepatch",
							Required: true,
						},
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				// Each value is set individually.
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
					DefinitionFrom: "somepatch",
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: "inline",
				},
			},
			validateRequired: true,
		},
		{
			name:    "Fail if value DefinitionFrom field does not match any definition.",
			wantErr: true,
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
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
						Raw: []byte(`1`),
					},
					// This definition does not exist.
					DefinitionFrom: "non-existent-patch",
				},
			},
			validateRequired: true,
		},
		{
			name:    "Fail if a value is set twice with the same definitionFrom.",
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
					},
				},
			},
			values: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
					DefinitionFrom: "somepatch",
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					DefinitionFrom: "somepatch",
				},
			},
			validateRequired: true,
		},
		{
			name:    "Fail if a value is set with empty and non-empty definitionFrom.",
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
			validateRequired: true,
		},
		{
			name:    "Fail when values invalid by their definition schema.",
			wantErr: true,
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: true,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
						{
							From: "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
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
						Raw: []byte(`1`),
					},
					DefinitionFrom: "inline",
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"one"`),
					},
					DefinitionFrom: "somepatch",
				},
			},
			validateRequired: true,
		},
		// Conflicting definition tests.
		{
			name: "Pass with a value provided for each conflicting definition.",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: true,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
						{
							From: "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
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
						Raw: []byte(`"one"`),
					},
					DefinitionFrom: "inline",
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
					DefinitionFrom: "somepatch",
				},
			},
			validateRequired: true,
		},
		{
			name: "Pass if non-required definition value doesn't include definitionFrom for each required definition when definitions conflict.",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: true,
					// There are conflicting definitions which means values should include a `definitionFrom` field.
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From:     "somepatch",
							Required: true,
						},
						// This variable is not required so it does not need a value.
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From:     "anotherpatch",
							Required: false,
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
					DefinitionFrom: "somepatch",
				},
			},
			validateRequired: true,
		},
		{
			name:    "Fail if value doesn't include definitionFrom when definitions conflict.",
			wantErr: true,
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					// There are conflicting definitions which means values should include a `definitionFrom` field.
					DefinitionsConflict: true,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
						{
							From: "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
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
						Raw: []byte(`1`),
					},
					// No definitionFrom
				},
			},
			validateRequired: true,
		},
		{
			name:    "Fail if value doesn't include definitionFrom for each required definition when definitions conflict.",
			wantErr: true,
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: true,
					// There are conflicting definitions which means values should include a `definitionFrom` field.
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{

							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
							From:     clusterv1.VariableDefinitionFromInline,
							Required: true,
						},
						{
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
							From:     "somepatch",
							Required: true,
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
					DefinitionFrom: "somepatch",
				},
			},
			validateRequired: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := validateClusterVariables(tt.values, tt.definitions,
				tt.validateRequired, field.NewPath("spec", "topology", "variables"))

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
		})
	}
}

func Test_ValidateClusterVariable(t *testing.T) {
	tests := []struct {
		name                 string
		clusterClassVariable *clusterv1.ClusterClassVariable
		clusterVariable      *clusterv1.ClusterVariable
		wantErr              bool
	}{
		{
			name: "Valid integer",
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
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`1`),
				},
			},
		},
		{
			name:    "Error if integer is above Maximum",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Maximum: pointer.Int64(10),
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`99`),
				},
			},
		},
		{
			name:    "Error if integer is below Minimum",
			wantErr: true,
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
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`0`),
				},
			},
		},

		{
			name:    "Fails, expected integer got string",
			wantErr: true,
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
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"1"`),
				},
			},
		},
		{
			name: "Valid string",
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
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"us-east"`),
				},
			},
		},
		{
			name:    "Error if string doesn't match pattern ",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "string",
						Pattern: "^[0-9]+$",
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"000000a"`),
				},
			},
		},
		{
			name:    "Error if string doesn't match format ",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:   "string",
						Format: "uri",
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"not a URI"`),
				},
			},
		},
		{
			name: "Valid enum string",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`"us-east-1"`)},
							{Raw: []byte(`"us-east-2"`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"us-east-1"`),
				},
			},
		},
		{
			name:    "Fails, value does not match one of the enum string values",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`"us-east-1"`)},
							{Raw: []byte(`"us-east-2"`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"us-east-invalid"`),
				},
			},
		},
		{
			name: "Valid enum integer",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`1`)},
							{Raw: []byte(`2`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`1`),
				},
			},
		},
		{
			name:    "Fails, value does not match one of the enum integer values",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "location",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`1`)},
							{Raw: []byte(`2`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "location",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`3`),
				},
			},
		},
		{
			name: "Valid object",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "boolean",
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
		},
		{
			name:    "Error if nested field is invalid",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "boolean",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"enabled":"not-a-bool"}`),
				},
			},
		},
		{
			name:    "Error if object is a bool instead",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "boolean",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`"not-a-object"`),
				},
			},
		},
		{
			name:    "Error if object is missing required field",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"url": {
								Type: "string",
							},
							"enabled": {
								Type: "boolean",
							},
						},
						Required: []string{
							"url",
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"enabled":"true"}`),
				},
			},
		},
		{
			name: "Valid object",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"requiredProperty": {
								Type: "boolean",
							},
							"boolProperty": {
								Type: "boolean",
							},
							"integerProperty": {
								Type:    "integer",
								Minimum: pointer.Int64(1),
							},
							"enumProperty": {
								Type: "string",
								Enum: []apiextensionsv1.JSON{
									{Raw: []byte(`"enumValue1"`)},
									{Raw: []byte(`"enumValue2"`)},
								},
							},
						},
						Required: []string{"requiredProperty"},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"requiredProperty":false,"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`),
				},
			},
		},
		{
			name: "Valid enum object",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "enumObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"location": {
								Type: "string",
							},
							"url": {
								Type: "string",
							},
						},
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`{"location": "us-east-1","url":"us-east-1-url"}`)},
							{Raw: []byte(`{"location": "us-east-2","url":"us-east-2-url"}`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "enumObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"location": "us-east-2","url":"us-east-2-url"}`),
				},
			},
		},
		{
			name:    "Fails, value does not match one of the enum object values",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "enumObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"location": {
								Type: "string",
							},
							"url": {
								Type: "string",
							},
						},
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`{"location": "us-east-1","url":"us-east-1-url"}`)},
							{Raw: []byte(`{"location": "us-east-2","url":"us-east-2-url"}`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "enumObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"location": "us-east-2","url":"wrong-url"}`),
				},
			},
		},
		{
			name: "Valid map",
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
									Type: "boolean",
								},
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"proxy":{"enabled":false}}`),
				},
			},
		},
		{
			name:    "Error if map is missing a required field",
			wantErr: true,
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
									Type: "boolean",
								},
								"url": {
									Type: "string",
								},
							},
							Required: []string{"url"},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"proxy":{"enabled":false}}`),
				},
			},
		},
		{
			name: "Valid array",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
							Enum: []apiextensionsv1.JSON{
								{Raw: []byte(`"enumValue1"`)},
								{Raw: []byte(`"enumValue2"`)},
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testArray",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`["enumValue1","enumValue2"]`),
				},
			},
		},
		{
			name:    "Error if array element is invalid",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
							Enum: []apiextensionsv1.JSON{
								{Raw: []byte(`"enumValue1"`)},
								{Raw: []byte(`"enumValue2"`)},
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testArray",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`["enumValue1","enumValueInvalid"]`),
				},
			},
		},
		{
			name:    "Error if array is too large",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
						},
						MaxItems: pointer.Int64(3),
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testArray",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`["value1","value2","value3","value4"]`),
				},
			},
		},
		{
			name:    "Error if array is too small",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
						},
						MinItems: pointer.Int64(3),
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testArray",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`["value1","value2"]`),
				},
			},
		},
		{
			name:    "Error if array contains duplicate values",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
						},
						UniqueItems: true,
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testArray",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`["value1","value1"]`),
				},
			},
		},
		{
			name: "Valid array object",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "enumArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
						},
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`["1","2","3"]`)},
							{Raw: []byte(`["4","5","6"]`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "enumArray",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`["1","2","3"]`),
				},
			},
		},
		{
			name:    "Fails, value does not match one of the enum array values",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "enumArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
						},
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`["1","2","3"]`)},
							{Raw: []byte(`["4","5","6"]`)},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "enumArray",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`["7","8","9"]`),
				},
			},
		},
		{
			name: "Valid object with x-kubernetes-preserve-unknown-fields",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"knownProperty": {
								Type: "boolean",
							},
						},
						// Preserves fields for the current object (in this case unknownProperty).
						XPreserveUnknownFields: true,
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"knownProperty":false,"unknownProperty":true}`),
				},
			},
		},
		{
			name:    "Error if undefined field",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"knownProperty": {
								Type: "boolean",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					// unknownProperty is not defined in the schema.
					Raw: []byte(`{"knownProperty":false,"unknownProperty":true}`),
				},
			},
		},
		{
			name:    "Error if undefined field with different casing",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"knownProperty": {
								Type: "boolean",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					// KnownProperty is only defined with lower case in the schema.
					Raw: []byte(`{"KnownProperty":false}`),
				},
			},
		},
		{
			name: "Valid nested object with x-kubernetes-preserve-unknown-fields",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						// XPreserveUnknownFields preservers recursively if the object has nested fields
						// as no nested Properties are defined.
						XPreserveUnknownFields: true,
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"test": {"unknownProperty":false}}`),
				},
			},
		},
		{
			name: "Valid object with nested fields and x-kubernetes-preserve-unknown-fields",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"test": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"knownProperty": {
										Type: "boolean",
									},
								},
								// Preserves fields on the current level (in this case unknownProperty).
								XPreserveUnknownFields: true,
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"test": {"knownProperty":false,"unknownProperty":true}}`),
				},
			},
		},
		{
			name:    "Error if undefined field nested",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"test": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"knownProperty": {
										Type: "boolean",
									},
								},
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					// unknownProperty is not defined in the schema.
					Raw: []byte(`{"test": {"knownProperty":false,"unknownProperty":true}}`),
				},
			},
		},
		{
			name:    "Error if undefined field nested and x-kubernetes-preserve-unknown-fields one level above",
			wantErr: true,
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"test": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"knownProperty": {
										Type: "boolean",
									},
								},
							},
						},
						// Preserves only on the current level as nested Properties are defined.
						XPreserveUnknownFields: true,
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"test": {"knownProperty":false,"unknownProperty":true}}`),
				},
			},
		},
		{
			name: "Valid object with mid-level unknown fields",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"test": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"knownProperty": {
										Type: "boolean",
									},
								},
							},
						},
						// Preserves only on the current level as nested Properties are defined.
						XPreserveUnknownFields: true,
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "testObject",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"test": {"knownProperty":false},"unknownProperty":true}`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := ValidateClusterVariable(tt.clusterVariable, tt.clusterClassVariable,
				field.NewPath("spec", "topology", "variables"))

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
		})
	}
}
