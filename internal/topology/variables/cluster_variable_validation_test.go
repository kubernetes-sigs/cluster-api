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
		name                  string
		clusterClassVariables []clusterv1.ClusterClassVariable
		clusterVariables      []clusterv1.ClusterVariable
		validateRequired      bool
		wantErr               bool
	}{
		{
			name: "Pass for a number of valid variables.",
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
					Name:     "zone",
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
							Type: "string",
							Enum: []apiextensionsv1.JSON{
								{Raw: []byte(`"us-east-1"`)},
								{Raw: []byte(`"us-east-2"`)},
							},
						},
					},
				},
			},

			clusterVariables: []clusterv1.ClusterVariable{
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
			name: "Error if required ClusterClassVariable is not defined in ClusterVariables.",
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
					Name:     "zone",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:      "string",
							MinLength: pointer.Int64(1),
						},
					},
				},
			},

			clusterVariables: []clusterv1.ClusterVariable{
				// cpu is missing in the ClusterVariables but is required in ClusterClassVariables.
				{
					Name: "zone",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"longerThanOneCharacter"`),
					},
				},
			},
			validateRequired: true,
			wantErr:          true,
		},
		{
			name: "Pass if required ClusterClassVariable is not defined in ClusterVariables but required validation is disabled.",
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
					Name:     "zone",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:      "string",
							MinLength: pointer.Int64(1),
						},
					},
				},
			},

			clusterVariables: []clusterv1.ClusterVariable{
				// cpu is missing in the ClusterVariables and is required in ClusterClassVariables,
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
			name: "Error if ClusterVariable defined which has no ClusterClassVariable definition.",
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
					Name:     "zone",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:      "string",
							MinLength: pointer.Int64(1),
						},
					},
				},
			},

			clusterVariables: []clusterv1.ClusterVariable{
				// location is defined here but not in the ClusterClassVariables
				{
					Name: "location",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"us-east-1"`),
					},
				},
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
			},
			validateRequired: true,
			wantErr:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := validateClusterVariables(tt.clusterVariables, tt.clusterClassVariables,
				tt.validateRequired, field.NewPath("spec", "topology", "variables"))

			if tt.wantErr {
				g.Expect(errList).NotTo(BeEmpty())
				return
			}
			g.Expect(errList).To(BeEmpty())
		})
	}
}

func Test_ValidateTopLevelClusterVariablesExist(t *testing.T) {
	tests := []struct {
		name                      string
		clusterVariablesOverrides []clusterv1.ClusterVariable
		clusterVariables          []clusterv1.ClusterVariable
		wantErr                   bool
	}{
		{
			name: "Pass if all variable overrides have corresponding top-level variables.",
			clusterVariablesOverrides: []clusterv1.ClusterVariable{
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
			},
			clusterVariables: []clusterv1.ClusterVariable{
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
			},
		},
		{
			name:                      "Pass if there are no variable overrides and no top-level variables.",
			clusterVariablesOverrides: []clusterv1.ClusterVariable{},
			clusterVariables:          []clusterv1.ClusterVariable{},
		},
		{
			name:                      "Pass if there are no variable overrides.",
			clusterVariablesOverrides: []clusterv1.ClusterVariable{},
			clusterVariables: []clusterv1.ClusterVariable{
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
			},
		},
		{
			name: "Error if a variable override is missing the corresponding top-level variables.",
			clusterVariablesOverrides: []clusterv1.ClusterVariable{
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
			},
			clusterVariables: []clusterv1.ClusterVariable{
				{
					Name: "zone",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"longerThanOneCharacter"`),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass if not every top-level variable has an override.",
			clusterVariablesOverrides: []clusterv1.ClusterVariable{
				{
					Name: "zone",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`"longerThanOneCharacter"`),
					},
				},
			},
			clusterVariables: []clusterv1.ClusterVariable{
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
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			errList := ValidateTopLevelClusterVariablesExist(tt.clusterVariablesOverrides, tt.clusterVariables,
				field.NewPath("spec", "topology", "workers", "machineDeployments").Index(0).Child("variables", "overrides"))

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
			name: "Error if integer is above Maximum",
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
			wantErr: true,
		},
		{
			name: "Error if integer is below Minimum",
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
			wantErr: true,
		},

		{
			name: "Fails, expected integer got string",
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
			wantErr: true,
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
			name: "Error if string doesn't match pattern ",
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
			wantErr: true,
		},
		{
			name: "Error if string doesn't match format ",
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
			wantErr: true,
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
			wantErr: true,
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
			wantErr: true,
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
			name: "Error if nested field is invalid",
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
			wantErr: true,
		},
		{
			name: "Error if object is a bool instead",
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
			wantErr: true,
		},
		{
			name: "Error if object is missing required field",
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
			wantErr: true,
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
			name: "Fails, value does not match one of the enum object values",
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
			wantErr: true,
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
			name: "Error if array element is invalid",
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
			wantErr: true,
		},
		{
			name: "Error if array is too large",
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
			wantErr: true,
		},
		{
			name: "Error if array is too small",
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
			wantErr: true,
		},
		{
			name: "Error if array contains duplicate values",
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
			wantErr: true,
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
			wantErr: true,
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
