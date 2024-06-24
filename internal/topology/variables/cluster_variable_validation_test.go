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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var ctx = ctrl.SetupSignalHandler()

func Test_ValidateClusterVariables(t *testing.T) {
	tests := []struct {
		name             string
		definitions      []clusterv1.ClusterClassStatusVariable
		values           []clusterv1.ClusterVariable
		validateRequired bool
		wantErrs         []validationMatch
	}{
		// Basic cases
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
									Minimum: ptr.To[int64](1),
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
									MinLength: ptr.To[int64](1),
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
			name: "Error when no value for required definition.",
			wantErrs: []validationMatch{
				required("Required value: required variable with name \"cpu\" must be defined",
					"spec.topology.variables"),
			},
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
									Minimum: ptr.To[int64](1),
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
									MinLength: ptr.To[int64](1),
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
									Minimum: ptr.To[int64](1),
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
									MinLength: ptr.To[int64](1),
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
			name: "Error if value has no definition.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"us-east-1\\\"\": no definitions found for variable \"location\"",
					"spec.topology.variables[location]"),
			},
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
			name: "Fail if value DefinitionFrom field does not match any definition.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"1\": no definitions found for variable \"cpu\" from \"non-existent-patch\"",
					"spec.topology.variables[cpu]"),
			},
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
			name: "Fail if a value is set twice with the same definitionFrom.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"[Name: cpu DefinitionFrom: somepatch,Name: cpu DefinitionFrom: somepatch]\": cluster variables not valid: variable \"cpu\" from \"somepatch\" is defined more than once",
					"spec.topology.variables"),
			},
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
			name: "Fail if a value is set with empty and non-empty definitionFrom.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"[Name: cpu DefinitionFrom: ,Name: cpu DefinitionFrom: somepatch]\": cluster variables not valid: variable \"cpu\" is defined with a mix of empty and non-empty values for definitionFrom",
					"spec.topology.variables"),
			},
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
			name: "Fail when values invalid by their definition schema.",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"1\": must be of type string: \"integer\"",
					"spec.topology.variables[cpu].value"),
				invalidType("Invalid value: \"\\\"one\\\"\": must be of type integer: \"string\"",
					"spec.topology.variables[cpu].value"),
			},
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
			name: "Fail if value doesn't include definitionFrom when definitions conflict.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"1\": variable \"cpu\" has conflicting definitions. It requires a non-empty `definitionFrom`",
					"spec.topology.variables[cpu]"),
			},
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
			name: "Fail if value doesn't include definitionFrom for each required definition when definitions conflict.",
			wantErrs: []validationMatch{
				required("Required value: required variable with name \"cpu\" from \"inline\" must be defined",
					"spec.topology.variables"),
			},
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
			gotErrs := validateClusterVariables(ctx, tt.values, nil, tt.definitions,
				tt.validateRequired, field.NewPath("spec", "topology", "variables"))

			checkErrors(t, tt.wantErrs, gotErrs)
		})
	}
}

func Test_ValidateClusterVariable(t *testing.T) {
	tests := []struct {
		name                 string
		clusterClassVariable *clusterv1.ClusterClassVariable
		clusterVariable      *clusterv1.ClusterVariable
		wantErrs             []validationMatch
	}{
		// Scalars
		{
			name: "Valid integer",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Minimum: ptr.To[int64](1),
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
			name: "Error if integer is above Maximum",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"99\": should be less than or equal to 10",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Maximum: ptr.To[int64](10),
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
			name: "Error if integer is below Minimum",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"0\": should be greater than or equal to 1",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Minimum: ptr.To[int64](1),
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
			name: "Fails, expected integer got string",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"\\\"1\\\"\": must be of type integer: \"string\"",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:    "integer",
						Minimum: ptr.To[int64](1),
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
						MinLength: ptr.To[int64](1),
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
			name: "Error if string doesn't match pattern ",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"000000a\\\"\": should match '^[0-9]+$'",
					"spec.topology.variables[location].value"),
			},
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
			name: "Error if string doesn't match format ",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"\\\"not a URI\\\"\": must be of type uri: \"not a URI\"",
					"spec.topology.variables[location].value"),
			},
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
			name: "Fails, value does not match one of the enum string values",
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"\\\"us-east-invalid\\\"\": supported values: \"us-east-1\", \"us-east-2\"",
					"spec.topology.variables[location].value"),
			},
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
			name: "Fails, value does not match one of the enum integer values",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"3\": must be of type string: \"integer\"",
					"spec.topology.variables[location].value"),
				unsupported("Unsupported value: \"3\": supported values: \"1\", \"2\"",
					"spec.topology.variables[location].value"),
			},
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
		// Objects
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
			name: "Error if nested field is invalid",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"{\\\"enabled\\\":\\\"not-a-bool\\\"}\": enabled in body must be of type boolean: \"string\"",
					"spec.topology.variables[httpProxy].value.enabled"),
			},
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
			name: "Error if object is a bool instead",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"\\\"not-a-object\\\"\": must be of type object: \"string\"",
					"spec.topology.variables[httpProxy].value"),
			},
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
			name: "Error if object is missing required field",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"{\\\"enabled\\\":\\\"true\\\"}\": enabled in body must be of type boolean: \"string\"",
					"spec.topology.variables[httpProxy].value.enabled"),
				required("Required value",
					"spec.topology.variables[httpProxy].value.url"),
			},
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
			name: "Error if object has too many properties",
			wantErrs: []validationMatch{
				toomany("Too many: \"{\\\"requiredProperty\\\":false,\\\"boolProperty\\\":true,\\\"integerProperty\\\":1,\\\"enumProperty\\\":\\\"enumValue2\\\"}\": must have at most 2 items",
					"spec.topology.variables[testObject].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testObject",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:          "object",
						MaxProperties: ptr.To[int64](2),
						Properties: map[string]clusterv1.JSONSchemaProps{
							"requiredProperty": {
								Type: "boolean",
							},
							"boolProperty": {
								Type: "boolean",
							},
							"integerProperty": {
								Type:    "integer",
								Minimum: ptr.To[int64](1),
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
					// Object has 4 properties, allowed are up to 2.
					Raw: []byte(`{"requiredProperty":false,"boolProperty":true,"integerProperty":1,"enumProperty":"enumValue2"}`),
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
								Minimum: ptr.To[int64](1),
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
			name: "Fails, value does not match one of the enum object values",
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"{\\\"location\\\": \\\"us-east-2\\\",\\\"url\\\":\\\"wrong-url\\\"}\": supported values: \"{\\\"location\\\":\\\"us-east-1\\\",\\\"url\\\":\\\"us-east-1-url\\\"}\", \"{\\\"location\\\":\\\"us-east-2\\\",\\\"url\\\":\\\"us-east-2-url\\\"}\"",
					"spec.topology.variables[enumObject].value"),
			},
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
		// Maps
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
			name: "Error if map is missing a required field",
			wantErrs: []validationMatch{
				required("Required value",
					"spec.topology.variables[httpProxy].value.proxy.url"),
			},
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
			name: "Error if map has too many entries",
			wantErrs: []validationMatch{
				toomany("Too many: \"{\\\"proxy\\\":{\\\"enabled\\\":false},\\\"proxy2\\\":{\\\"enabled\\\":false},\\\"proxy3\\\":{\\\"enabled\\\":false}}\": must have at most 2 items",
					"spec.topology.variables[httpProxy].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "httpProxy",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:          "object",
						MaxProperties: ptr.To[int64](2),
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
					// Map has 3 entries, allowed are up to 2.
					Raw: []byte(`{"proxy":{"enabled":false},"proxy2":{"enabled":false},"proxy3":{"enabled":false}}`),
				},
			},
		},
		// Arrays
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
			name: "Error if array element is invalid",
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"[\\\"enumValue1\\\",\\\"enumValueInvalid\\\"]\": supported values: \"enumValue1\", \"enumValue2\"",
					"spec.topology.variables[testArray].value.[1]"),
			},
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
			name: "Error if array is too large",
			wantErrs: []validationMatch{
				toomany("Too many: \"[\\\"value1\\\",\\\"value2\\\",\\\"value3\\\",\\\"value4\\\"]\": must have at most 3 items",
					"spec.topology.variables[testArray].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
						},
						MaxItems: ptr.To[int64](3),
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
			name: "Error if array is too small",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"[\\\"value1\\\",\\\"value2\\\"]\": should have at least 3 items",
					"spec.topology.variables[testArray].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "testArray",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
						},
						MinItems: ptr.To[int64](3),
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
			name: "Error if array contains duplicate values",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"[\\\"value1\\\",\\\"value1\\\"]\": shouldn't contain duplicates",
					"spec.topology.variables[testArray].value"),
			},
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
			name: "Fails, value does not match one of the enum array values",
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"[\\\"7\\\",\\\"8\\\",\\\"9\\\"]\": supported values: \"[\\\"1\\\",\\\"2\\\",\\\"3\\\"]\", \"[\\\"4\\\",\\\"5\\\",\\\"6\\\"]\"",
					"spec.topology.variables[enumArray].value"),
			},
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
		// x-kubernetes-preserve-unknown-fields
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
			name: "Error if undefined field",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"{\\\"knownProperty\\\":false,\\\"unknownProperty\\\":true}\": failed validation: \"unknownProperty\" field(s) are not specified in the variable schema of variable \"testObject\"",
					"spec.topology.variables[testObject]"),
			},
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
			name: "Error if undefined field with different casing",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"{\\\"KnownProperty\\\":false}\": failed validation: \"KnownProperty\" field(s) are not specified in the variable schema of variable \"testObject\"",
					"spec.topology.variables[testObject]"),
			},
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
			name: "Error if undefined field nested",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"{\\\"test\\\": {\\\"knownProperty\\\":false,\\\"unknownProperty\\\":true}}\": failed validation: \"test.unknownProperty\" field(s) are not specified in the variable schema of variable \"testObject\"",
					"spec.topology.variables[testObject]"),
			},
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
			name: "Error if undefined field nested and x-kubernetes-preserve-unknown-fields one level above",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"{\\\"test\\\": {\\\"knownProperty\\\":false,\\\"unknownProperty\\\":true}}\": failed validation: \"test.unknownProperty\" field(s) are not specified in the variable schema of variable \"testObject\"",
					"spec.topology.variables[testObject]"),
			},
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
		// CEL
		{
			name: "Valid CEL expression: scalar: using self",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self <= 1",
						}},
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
			name: "Valid CEL expression: special characters",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self.__namespace__",
						}, {
							Rule: "self.x__dash__prop",
						}, {
							Rule: "self.redact__underscores__d",
						}},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"namespace": { // keyword
								Type: "boolean",
							},
							"x-prop": {
								Type: "boolean",
							},
							"redact__d": {
								Type: "boolean",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"namespace":true,"x-prop":true,"redact__d":true}`),
				},
			},
		},
		{
			name: "Valid CEL expression: objects: using self.field, has(self.field)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self.field <= 1",
						}, {
							Rule: "has(self.field)",
						}, {
							Rule: "!has(self.field2)", // field2 is absent in the value
						}},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"field": {
								Type: "integer",
							},
							"field2": {
								Type: "integer",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"field": 1}`),
				},
			},
		},
		{
			name: "Valid CEL expression: maps: using self[mapKey], mapKey in self, self.all, equality",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self['key1'] == 1",
						}, {
							Rule: "'key1' in self",
						}, {
							Rule: "self.all(value, self[value] >= 1)",
						}, {
							Rule: "self == {'key1':1,'key2':2}",
						}, {
							Rule: "self == {'key2':2,'key1':1}", // order does not matter
						}},
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"key1":1,"key2":2}`),
				},
			},
		},
		{
			name: "Valid CEL expression: arrays: using self[i], self.all, equality",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self[0] == 1",
						}, {
							Rule: "self[1] == 2",
						}, {
							Rule: "self.all(value, value >= 1)",
						}, {
							Rule: "self == [1,2]",
						}, {
							Rule: "self != [2,1]", // order matters
						}, {
							Rule: "self + [3] == [1,2,3]",
						}, {
							Rule: "self + [3] == [1,2] + [3]",
						}, {
							Rule: "self + [3] != [3,1,2]",
						}, {
							Rule: "[3] + self == [3,1,2]",
						}, {
							Rule: "[3] + self == [3] + [1,2]",
						}, {
							Rule: "[3] + self  != [1,2,3]",
						}},
						Items: &clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`[1,2]`),
				},
			},
		},
		{
			name: "Error if integer is above maximum via with CEL expression",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"99\": failed rule: self <= 1",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self <= 1",
						}},
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
			name: "Error if integer is below minimum via CEL expression",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"0\": failed rule: self >= 1",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self >= 1",
						}},
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
			name: "Error if integer is below minimum via CEL expression with custom error message",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"0\": new value must be greater than or equal to 1",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule:    "self >= 1",
							Message: "new value must be greater than or equal to 1",
						}},
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
			name: "Error if integer is below minimum via CEL expression with custom error message (messageExpression preferred)",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"0\": new value must be greater than or equal to 1, but got 0",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule:              "self >= 1",
							Message:           "new value must be greater than or equal to 1",
							MessageExpression: "'new value must be greater than or equal to 1, but got %d'.format([self])",
						}},
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
			name: "Error if integer is below minimum via CEL expression with custom error message (fallback to message if messageExpression fails)",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"0\": new value must be greater than or equal to 1",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule:              "self >= 1",
							Message:           "new value must be greater than or equal to 1",
							MessageExpression: "''", // This evaluates to an empty string, and thus message is used instead.
						}},
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
			name: "Invalid CEL expression: objects (nested)",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"{\\\"objectField\\\": {\\\"field\\\": 2}}\": failed rule: self.field <= 1",
					"spec.topology.variables[cpu].value.objectField"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:                   "object",
						XPreserveUnknownFields: true,
						Properties: map[string]clusterv1.JSONSchemaProps{
							"objectField": {
								Type:                   "object",
								XPreserveUnknownFields: true,
								XValidations: []clusterv1.ValidationRule{{
									Rule: "self.field <= 1",
								}},
								Properties: map[string]clusterv1.JSONSchemaProps{
									"field": {
										Type: "integer",
									},
								},
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"objectField": {"field": 2}}`),
				},
			},
		},
		{
			name: "Valid CEL expression: objects: defined field can be accessed",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:                   "object",
						XPreserveUnknownFields: true,
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self.field <= 1",
						}},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"field": {
								Type: "integer",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"field": 1,"field3": null}`), // unknown field3 is preserved, but not used in CEL
				},
			},
		},
		{
			name: "Invalid CEL expression: objects: unknown field cannot be accessed",
			wantErrs: []validationMatch{
				invalid("rule compile error: compilation failed: ERROR: <input>:1:5: undefined field 'field3'",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:                   "object",
						XPreserveUnknownFields: true,
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self.field <= 1",
						}, {
							Rule: "self.field3 <= 1",
						}},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"field": {
								Type: "integer",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"field": 1,"field3": null}`), // unknown field3 is preserved, but checking it via CEL fails
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErrs := ValidateClusterVariable(ctx, tt.clusterVariable, nil, tt.clusterClassVariable,
				field.NewPath("spec", "topology", "variables").Key(tt.clusterClassVariable.Name))

			checkErrors(t, tt.wantErrs, gotErrs)
		})
	}
}

func Test_ValidateClusterVariable_CELTransitions(t *testing.T) {
	tests := []struct {
		name                 string
		clusterClassVariable *clusterv1.ClusterClassVariable
		clusterVariable      *clusterv1.ClusterVariable
		oldClusterVariable   *clusterv1.ClusterVariable
		wantErrs             []validationMatch
	}{
		{
			name: "Valid transition if old value is not set",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self > oldSelf",
						}},
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
			name: "Valid transition if old value is less than new value via CEL expression",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self > oldSelf",
						}},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`1`),
				},
			},
			oldClusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`0`),
				},
			},
		},
		{
			name: "Error if integer is not greater than old value via CEL expression",
			wantErrs: []validationMatch{
				invalid("failed rule: self > oldSelf",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self > oldSelf",
						}},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`0`),
				},
			},
			oldClusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`1`),
				},
			},
		},
		{
			name: "Error if integer is not greater than old value via CEL expression with custom error message",
			wantErrs: []validationMatch{
				invalid("new value must be greater than old value",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule:    "self > oldSelf",
							Message: "new value must be greater than old value",
						}},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`0`),
				},
			},
			oldClusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`1`),
				},
			},
		},
		{
			name: "Pass immutability check if value did not change",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{{
							Rule:    "self.field == oldSelf.field",
							Message: "field is immutable",
						}},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"field": {
								Type: "string",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"field":"value1"}`),
				},
			},
			oldClusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"field":"value1"}`),
				},
			},
		},
		{
			name: "Fail immutability check if value changes",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"{\\\"field\\\":\\\"value2\\\"}\": field is immutable, but field was changed from \"value1\" to \"value2\"",
					"spec.topology.variables[cpu].value"),
			},
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "cpu",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{{
							Rule:              "self.field == oldSelf.field",
							MessageExpression: "'field is immutable, but field was changed from \"%s\" to \"%s\"'.format([oldSelf.field,self.field])",
						}},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"field": {
								Type: "string",
							},
						},
					},
				},
			},
			clusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"field":"value2"}`),
				},
			},
			oldClusterVariable: &clusterv1.ClusterVariable{
				Name: "cpu",
				Value: apiextensionsv1.JSON{
					Raw: []byte(`{"field":"value1"}`),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErrs := ValidateClusterVariable(ctx, tt.clusterVariable, tt.oldClusterVariable, tt.clusterClassVariable,
				field.NewPath("spec", "topology", "variables").Key(tt.clusterVariable.Name))

			checkErrors(t, tt.wantErrs, gotErrs)
		})
	}
}

func Test_ValidateMachineVariables(t *testing.T) {
	tests := []struct {
		name        string
		definitions []clusterv1.ClusterClassStatusVariable
		values      []clusterv1.ClusterVariable
		oldValues   []clusterv1.ClusterVariable
		wantErrs    []validationMatch
	}{
		// Basic cases
		{
			name: "Pass when no value for required definition (required variables are not required for overrides)",
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
									Minimum: ptr.To[int64](1),
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
									MinLength: ptr.To[int64](1),
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
		},
		{
			name: "Error if value has no definition.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"us-east-1\\\"\": no definitions found for variable \"location\"",
					"spec.topology.workers.machineDeployments[0].variables.overrides[location]"),
			},
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
		},
		{
			name: "Fail if value DefinitionFrom field does not match any definition.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"1\": no definitions found for variable \"cpu\" from \"non-existent-patch\"",
					"spec.topology.workers.machineDeployments[0].variables.overrides[cpu]"),
			},
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
		},
		{
			name: "Fail when values invalid by their definition schema.",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"1\": must be of type string: \"integer\"",
					"spec.topology.workers.machineDeployments[0].variables.overrides[cpu].value"),
				invalidType("Invalid value: \"\\\"one\\\"\": must be of type integer: \"string\"",
					"spec.topology.workers.machineDeployments[0].variables.overrides[cpu].value"),
			},
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
						Raw: []byte(`1`), // not a string
					},
					DefinitionFrom: "inline",
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"one"`), // not an integer
					},
					DefinitionFrom: "somepatch",
				},
			},
		},
		// CEL
		{
			name: "Valid integer with CEL expression",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self <= 1",
									}},
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
			},
		},
		{
			name: "Error if integer is above maximum via with CEL expression",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"99\": failed rule: self <= 1",
					"spec.topology.workers.machineDeployments[0].variables.overrides[cpu].value"),
			},
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self <= 1",
									}},
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
						Raw: []byte(`99`),
					},
				},
			},
		},
		{
			name: "Error if integer is below minimum via CEL expression",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"0\": failed rule: self >= 1",
					"spec.topology.workers.machineDeployments[0].variables.overrides[cpu].value"),
			},
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self >= 1",
									}},
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
						Raw: []byte(`0`),
					},
				},
			},
		},
		{
			name: "Valid transition if old value is not set",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self > oldSelf",
									}},
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
			},
		},
		{
			name: "Valid transition if old value is less than new value via CEL expression",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self > oldSelf",
									}},
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
			},
			oldValues: []clusterv1.ClusterVariable{
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`0`),
					},
				},
			},
		},
		{
			name: "Error if integer is not greater than old value via CEL expression",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"0\": failed rule: self > oldSelf",
					"spec.topology.workers.machineDeployments[0].variables.overrides[cpu].value"),
			},
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self > oldSelf",
									}},
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
						Raw: []byte(`0`),
					},
				},
			},
			oldValues: []clusterv1.ClusterVariable{
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
			gotErrs := ValidateMachineVariables(ctx, tt.values, tt.oldValues, tt.definitions,
				field.NewPath("spec", "topology", "workers", "machineDeployments").Index(0).Child("variables", "overrides"))

			checkErrors(t, tt.wantErrs, gotErrs)
		})
	}
}
