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
	"fmt"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func Test_ValidateClusterClassVariables(t *testing.T) {
	tests := []struct {
		name                     string
		clusterClassVariables    []clusterv1.ClusterClassVariable
		oldClusterClassVariables []clusterv1.ClusterClassVariable
		wantErrs                 []validationMatch
	}{
		// Basics
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
			wantErrs: []validationMatch{
				invalid("variable name must be unique. Variable with name \"cpu\" is defined more than once",
					"spec.variables[cpu].name"),
			},
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
		{
			name: "pass if x-kubernetes-validations has valid rule",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "cpu",
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
		// CEL
		{
			name: "pass if x-kubernetes-validations has valid messageExpression",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "cpu",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
							XValidations: []clusterv1.ValidationRule{{
								Rule:              "true",
								MessageExpression: "'Expected integer greater or equal to 1, got %d'.format([self])",
							}},
						},
					},
				},
			},
		},
		// CEL compatibility version
		{
			name: "fail if x-kubernetes-validations has invalid rule: " +
				"new rule that uses opts that are not available with the current compatibility version",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("compilation failed: ERROR: <input>:1:3: undeclared reference to 'ip' (in container '')",
					"spec.variables[someIP].schema.openAPIV3Schema.x-kubernetes-validations[0].rule"),
			},
		},
		{
			name: "pass if x-kubernetes-validations has valid rule: " +
				"pre-existing rule that uses opts that are not available with the current compatibility version, but with the \"max\" env",
			oldClusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				},
			},
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								// but because this rule already existed before, the "max" env is used
								// and there the IP func is available.
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid rule: " +
				"pre-existing rule that uses opts that are not available with the current compatibility version, but with the \"max\" env" +
				"this would work if the pre-existing rule would be for the same variable",
			oldClusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someOtherIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				},
			},
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								// Note: IP will be only available if the compatibility version is 1.30
								// this rule already existed before, but the "max" env is not used
								// because the rule didn't exist for the same variable.
								Rule: "ip(self).family() == 6",
							}},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("compilation failed: ERROR: <input>:1:3: undeclared reference to 'ip' (in container '')",
					"spec.variables[someIP].schema.openAPIV3Schema.x-kubernetes-validations[0].rule"),
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid messageExpression: " +
				"new messageExpression that uses opts that are not available with the current compatibility version",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								Rule: "true",
								// Note: IP will be only available if the compatibility version is 1.30
								MessageExpression: "'IP family %d'.format([ip(self).family()])",
							}},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("messageExpression compilation failed: ERROR: <input>:1:26: undeclared reference to 'ip' (in container '')",
					"spec.variables[someIP].schema.openAPIV3Schema.x-kubernetes-validations[0].messageExpression"),
			},
		},
		{
			name: "pass if x-kubernetes-validations has valid messageExpression: " +
				"pre-existing messageExpression that uses opts that are not available with the current compatibility version, but with the \"max\" env",
			oldClusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								Rule: "true",
								// Note: IP will be only available if the compatibility version is 1.30
								MessageExpression: "'IP family %d'.format([ip(self).family()])",
							}},
						},
					},
				},
			},
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								Rule: "true",
								// Note: IP will be only available if the compatibility version is 1.30
								// but because this messageExpression already existed before, the "max" env is used
								// and there the IP func is available.
								MessageExpression: "'IP family %d'.format([ip(self).family()])",
							}},
						},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid messageExpression: " +
				"pre-existing messageExpression that uses opts that are not available with the current compatibility version, but with the \"max\" env" +
				"this would work if the pre-existing messageExpression would be for the same variable",
			oldClusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someOtherIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								Rule: "true",
								// Note: IP will be only available if the compatibility version is 1.30
								MessageExpression: "'IP family %d'.format([ip(self).family()])",
							}},
						},
					},
				},
			},
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name: "someIP",
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								Rule: "true",
								// Note: IP will be only available if the compatibility version is 1.30
								// this rule already existed before, but the "max" env is not used
								// because the rule didn't exist for the same variable.
								MessageExpression: "'IP family %d'.format([ip(self).family()])",
							}},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("messageExpression compilation failed: ERROR: <input>:1:26: undeclared reference to 'ip' (in container '')",
					"spec.variables[someIP].schema.openAPIV3Schema.x-kubernetes-validations[0].messageExpression"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//g := NewWithT(t)

			// Pin the compatibility version used in variable CEL validation to 1.29, so we don't have to continuously refactor
			// the unit tests that verify that compatibility is handled correctly.
			// FIXME(sbueringer)
			//effectiveVer := utilversion.DefaultComponentGlobalsRegistry.EffectiveVersionFor(utilversion.DefaultKubeComponent)
			//if effectiveVer != nil {
			//	g.Expect(effectiveVer.MinCompatibilityVersion()).To(Equal(version.MustParse("v1.29")))
			//} else {
			//	v := utilversion.DefaultKubeEffectiveVersion()
			//	v.SetMinCompatibilityVersion(version.MustParse("v1.29"))
			//	g.Expect(utilversion.DefaultComponentGlobalsRegistry.Register(utilversion.DefaultKubeComponent, v, nil)).To(Succeed())
			//}

			gotErrs := ValidateClusterClassVariables(ctx,
				tt.oldClusterClassVariables,
				tt.clusterClassVariables,
				field.NewPath("spec", "variables"))

			checkErrors(t, tt.wantErrs, gotErrs)
		})
	}
}

func Test_ValidateClusterClassVariable(t *testing.T) {
	tests := []struct {
		name                 string
		clusterClassVariable *clusterv1.ClusterClassVariable
		wantErrs             []validationMatch
	}{
		// Scalars
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
		// Variable names
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
			wantErrs: []validationMatch{
				invalid("\"builtin\" is a reserved variable name",
					"spec.variables[builtin].name"),
			},
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
			wantErrs: []validationMatch{
				required("variable name must be defined",
					"spec.variables[].name"),
			},
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
			wantErrs: []validationMatch{
				invalid("\"path.tovariable\": variable name cannot contain \".\"",
					"spec.variables[path.tovariable].name"),
			},
		},
		// Metadata
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
				Name: "variable",
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
			wantErrs: []validationMatch{
				invalid("\".label-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].metadata.labels"),
			},
		},
		{
			name: "fail on invalid variable annotation: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
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
			wantErrs: []validationMatch{
				invalid("\".annotation-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].metadata.annotations"),
			},
		},
		// Defaults
		{
			name: "Valid variable XMetadata",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
						XMetadata: &clusterv1.VariableSchemaMetadata{
							Labels: map[string]string{
								"label-key": "label-value",
							},
							Annotations: map[string]string{
								"annotation-key": "annotation-value",
							},
						},
					},
				},
			},
		},
		{
			name: "fail on invalid XMetadata label: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
						XMetadata: &clusterv1.VariableSchemaMetadata{
							Labels: map[string]string{
								".label-key": "label-value",
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".label-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.x-metadata.labels"),
			},
		},
		{
			name: "fail on invalid XMetadata annotation: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:      "string",
						MinLength: ptr.To[int64](1),
						XMetadata: &clusterv1.VariableSchemaMetadata{
							Annotations: map[string]string{
								".annotation-key": "annotation-value",
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".annotation-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.x-metadata.annotations"),
			},
		},
		{
			name: "Valid variable XMetadata (map)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "validVariable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "string",
							XMetadata: &clusterv1.VariableSchemaMetadata{
								Labels: map[string]string{
									"label-key": "label-value",
								},
								Annotations: map[string]string{
									"annotation-key": "annotation-value",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail on invalid XMetadata (map) label: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "string",
							XMetadata: &clusterv1.VariableSchemaMetadata{
								Labels: map[string]string{
									".label-key": "label-value",
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".label-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.additionalProperties.x-metadata.labels"),
			},
		},
		{
			name: "fail on invalid XMetadata (map) annotation: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "string",
							XMetadata: &clusterv1.VariableSchemaMetadata{
								Annotations: map[string]string{
									".annotation-key": "annotation-value",
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".annotation-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.additionalProperties.x-metadata.annotations"),
			},
		},
		{
			name: "Valid variable XMetadata (object)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "validVariable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "string",
								XMetadata: &clusterv1.VariableSchemaMetadata{
									Labels: map[string]string{
										"label-key": "label-value",
									},
									Annotations: map[string]string{
										"annotation-key": "annotation-value",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail on invalid XMetadata (object) label: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "string",
								XMetadata: &clusterv1.VariableSchemaMetadata{
									Labels: map[string]string{
										".label-key": "label-value",
									},
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".label-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.properties[enabled].x-metadata.labels"),
			},
		},
		{
			name: "fail on invalid XMetadata (object) annotation: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"enabled": {
								Type: "string",
								XMetadata: &clusterv1.VariableSchemaMetadata{
									Annotations: map[string]string{
										".annotation-key": "annotation-value",
									},
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".annotation-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.properties[enabled].x-metadata.annotations"),
			},
		},
		{
			name: "Valid variable XMetadata (array)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "validVariable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
							XMetadata: &clusterv1.VariableSchemaMetadata{
								Labels: map[string]string{
									"label-key": "label-value",
								},
								Annotations: map[string]string{
									"annotation-key": "annotation-value",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "fail on invalid XMetadata (array) label: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
							XMetadata: &clusterv1.VariableSchemaMetadata{
								Labels: map[string]string{
									".label-key": "label-value",
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".label-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.items.x-metadata.labels"),
			},
		},
		{
			name: "fail on invalid XMetadata (array) annotation: key does not start with alphanumeric character",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variable",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
							XMetadata: &clusterv1.VariableSchemaMetadata{
								Annotations: map[string]string{
									".annotation-key": "annotation-value",
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".annotation-key\": name part must consist of alphanumeric characters",
					"spec.variables[variable].schema.openAPIV3Schema.items.x-metadata.annotations"),
			},
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
						Type: "object",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`"defaultValue": "value"`), // invalid JSON
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"defaultValue\\\": \\\"value\\\"\": failed to convert schema definition for variable \"var\"",
					"spec.variables[var].schema.openAPIV3Schema.default"),
			},
		},
		{
			name: "fail on default value with invalid JSON (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "object",
							Default: &apiextensionsv1.JSON{
								Raw: []byte(`"defaultValue": "value"`), // invalid JSON
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"defaultValue\\\": \\\"value\\\"\": failed to convert schema definition for variable \"var\"",
					"spec.variables[var].schema.openAPIV3Schema.items.default"),
			},
		},
		// Examples
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
						Type: "object",
						Example: &apiextensionsv1.JSON{
							Raw: []byte(`"exampleValue": "value"`), // invalid JSON
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"exampleValue\\\": \\\"value\\\"\": failed to convert schema definition for variable \"var\"",
					"spec.variables[var].schema.openAPIV3Schema.example"),
			},
		},
		{
			name: "fail on example value with invalid JSON (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "object",
							Example: &apiextensionsv1.JSON{
								Raw: []byte(`"exampleValue": "value"`), // invalid JSON
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"exampleValue\\\": \\\"value\\\"\": failed to convert schema definition for variable \"var\"",
					"spec.variables[var].schema.openAPIV3Schema.items.example"),
			},
		},
		// Enums
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
						Type: "object",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`"enumValue": "value"`)}, // invalid JSON
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"enumValue\\\": \\\"value\\\"\": failed to convert schema definition for variable \"var\"",
					"spec.variables[var].schema.openAPIV3Schema.enum[0]"),
			},
		},
		{
			name: "fail on enum value with invalid JSON (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name:     "var",
				Required: true,
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "object",
							Enum: []apiextensionsv1.JSON{
								{Raw: []byte(`{"enumValue": "value"}`)}, // valid JSON
								{Raw: []byte(`"enumValue": "value"`)},   // invalid JSON
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"\\\"enumValue\\\": \\\"value\\\"\": failed to convert schema definition for variable \"var\"",
					"spec.variables[var].schema.openAPIV3Schema.items.enum[1]"),
			},
		},
		// Variable types
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
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"null\"",
					"spec.variables[var].schema.openAPIV3Schema.type"),
			},
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
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"invalidVariableType\"",
					"spec.variables[var].schema.openAPIV3Schema.type"),
			},
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
			wantErrs: []validationMatch{
				required("Required value: must not be empty for specified object fields",
					"spec.variables[var].schema.openAPIV3Schema.type"),
			},
		},
		{
			name: "fail on variable type length zero (object)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"nestedField": {
								Type: "",
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				required("Required value: must not be empty for specified object fields",
					"spec.variables[var].schema.openAPIV3Schema.properties[nestedField].type"),
			},
		},
		{
			name: "fail on variable type length zero (array)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type: "",
						},
					},
				},
			},
			wantErrs: []validationMatch{
				required("Required value: must not be empty for specified array items",
					"spec.variables[var].schema.openAPIV3Schema.items.type"),
			},
		},
		{
			name: "fail on variable type length zero (map)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						AdditionalProperties: &clusterv1.JSONSchemaProps{
							Type: "",
						},
					},
				},
			},
			wantErrs: []validationMatch{
				required("Required value: must not be empty for specified object fields",
					"spec.variables[var].schema.openAPIV3Schema.additionalProperties.type"),
			},
		},
		// Objects
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
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"invalidType\"",
					"spec.variables[httpProxy].schema.openAPIV3Schema.properties[noProxy].type"),
			},
		},
		// Maps
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
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"invalidType\"",
					"spec.variables[httpProxy].schema.openAPIV3Schema.additionalProperties.properties[noProxy].type"),
			},
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
			wantErrs: []validationMatch{
				forbidden("Forbidden: additionalProperties and properties are mutually exclusive",
					"spec.variables[httpProxy].schema.openAPIV3Schema"),
			},
		},
		// Arrays
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
			wantErrs: []validationMatch{
				invalid("default is not valid JSON: invalid character 'i' looking for beginning of value",
					"spec.variables[arrayVariable].schema.openAPIV3Schema.items.default"),
			},
		},
		// Required
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
		// Defaults
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
			wantErrs: []validationMatch{
				toolong("Too long: may not be longer than 6",
					"spec.variables[var].schema.openAPIV3Schema.default"),
			},
		},
		{
			name: "fail on variable with a default that is invalid by the given schema (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "array",
						Items: &clusterv1.JSONSchemaProps{
							Type:      "string",
							MaxLength: ptr.To[int64](6),
							Default:   &apiextensionsv1.JSON{Raw: []byte(`"veryLongValueIsInvalid"`)},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				toolong("Too long: may not be longer than 6",
					"spec.variables[var].schema.openAPIV3Schema.items.default"),
			},
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
			wantErrs: []validationMatch{
				invalid("should be greater than or equal to 1",
					"spec.variables[var].schema.openAPIV3Schema.properties[spec].properties[replicas].default"),
			},
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
			wantErrs: []validationMatch{
				invalid("should be greater than or equal to 1",
					"spec.variables[var].schema.openAPIV3Schema.default.spec.replicas"),
			},
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
			wantErrs: []validationMatch{
				required("Required value",
					"spec.variables[var].schema.openAPIV3Schema.properties[spec].default.replicas"),
			},
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
		// Examples
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
			wantErrs: []validationMatch{
				toolong("Too long: may not be longer than 6",
					"spec.variables[var].schema.openAPIV3Schema.example"),
			},
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
			wantErrs: []validationMatch{
				invalid("should be greater than or equal to 0",
					"spec.variables[var].schema.openAPIV3Schema.properties[spec].properties[replicas].example"),
			},
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
			wantErrs: []validationMatch{
				invalid("should be greater than or equal to 1",
					"spec.variables[var].schema.openAPIV3Schema.example.spec.replicas"),
			},
		},
		{
			name: "pass if variable is an object with a top level example value valid by the given schema",
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
		// Enums
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
			wantErrs: []validationMatch{
				toolong("may not be longer than 6",
					"spec.variables[var].schema.openAPIV3Schema.enum[0]"),
			},
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
			wantErrs: []validationMatch{
				invalid("should be greater than or equal to 0",
					"spec.variables[var].schema.openAPIV3Schema.properties[spec].properties[replicas].enum[1]"),
			},
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
			wantErrs: []validationMatch{
				invalid("should be greater than or equal to 1",
					"spec.variables[var].schema.openAPIV3Schema.enum[1].spec.replicas"),
			},
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
		// CEL
		{
			name: "pass if x-kubernetes-validations has valid rule",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
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
		{
			name: "fail if x-kubernetes-validations has invalid rule: invalid CEL expression",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule: "this does not compile",
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("compilation failed: ERROR: <input>:1:6: Syntax error: mismatched input 'does' expecting <EOF>\n | this does not compile\n | .....^",
					"spec.variables[cpu].schema.openAPIV3Schema.x-kubernetes-validations[0].rule"),
			},
		},
		// CEL: uncorrelatable paths (in arrays)
		{
			name: "fail if x-kubernetes-validations has invalid rule: oldSef cannot be used in arrays",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:     "array",
						MaxItems: ptr.To[int64](20),
						Items: &clusterv1.JSONSchemaProps{
							Type: "string",
							XValidations: []clusterv1.ValidationRule{{
								Rule: "oldSelf.contains('keyword')",
							}},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"oldSelf.contains('keyword')\": oldSelf cannot be used on the uncorrelatable portion of the schema within spec.variables[cpu].schema.openAPIV3Schema.items",
					"spec.variables[cpu].schema.openAPIV3Schema.items.x-kubernetes-validations[0].rule"),
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid rule: oldSef cannot be used in arrays (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type:     "array",
						MaxItems: ptr.To[int64](10),
						Items: &clusterv1.JSONSchemaProps{
							Type:     "array",
							MaxItems: ptr.To[int64](10),
							Items: &clusterv1.JSONSchemaProps{
								Type:      "string",
								MaxLength: ptr.To[int64](1024),
								XValidations: []clusterv1.ValidationRule{{
									Rule: "oldSelf.contains('keyword')",
								}},
							},
						},
					},
				},
			},
			// Note: This test verifies that the path in the error message references the first array, not the nested one.
			wantErrs: []validationMatch{
				invalid("Invalid value: \"oldSelf.contains('keyword')\": oldSelf cannot be used on the uncorrelatable portion of the schema within spec.variables[cpu].schema.openAPIV3Schema.items",
					"spec.variables[cpu].schema.openAPIV3Schema.items.items.x-kubernetes-validations[0].rule"),
			},
		},
		// CEL: cost
		{
			name: "fail if x-kubernetes-validations has invalid rules: cost total exceeds total limit",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"array": {
								// We are not setting MaxItems, so a worst case cardinality will be assumed and we exceed the per-expression limit.
								Type: "array",
								Items: &clusterv1.JSONSchemaProps{
									Type:         "string",
									MaxLength:    ptr.To[int64](500000),
									XValidations: []clusterv1.ValidationRule{{Rule: "self.contains('keyword')"}},
								},
							},
							"map": {
								// Setting MaxProperties and MaxLength allows us to stay below the per-expression limit.
								Type:          "object",
								MaxProperties: ptr.To[int64](10000),
								AdditionalProperties: &clusterv1.JSONSchemaProps{
									Type:         "string",
									MaxLength:    ptr.To[int64](2048),
									XValidations: []clusterv1.ValidationRule{{Rule: "self.contains('keyword')"}},
								},
							},
							"field": {
								// Include a validation rule that does not contribute to total limit being exceeded (i.e. it is less than 1% of the limit)
								Type:         "integer",
								XValidations: []clusterv1.ValidationRule{{Rule: "self > 50 && self < 100"}},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				forbidden("estimated rule cost exceeds budget by factor of more than 100x",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[array].items.x-kubernetes-validations[0].rule"),
				forbidden("contributed to estimated rule & messageExpression cost total exceeding cost limit",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[array].items.x-kubernetes-validations[0].rule"),
				forbidden("contributed to estimated rule & messageExpression cost total exceeding cost limit",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[map].additionalProperties.x-kubernetes-validations[0].rule"),
				forbidden("x-kubernetes-validations estimated rule & messageExpression cost total for entire OpenAPIv3 schema exceeds budget by factor of more than 100x",
					"spec.variables[cpu].schema.openAPIV3Schema"),
			},
		},
		// CEL: rule
		{
			name: "fail if x-kubernetes-validations has invalid rule: rule is not specified",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{
							{
								Rule: " ",
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"a": {
								Type: "number",
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				required("rule is not specified",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[0].rule"),
			},
		},
		// CEL: validation of defaults
		{
			name: "pass if x-kubernetes-validations has valid rule: valid default value",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`1`),
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self >= 1",
						}},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has valid rule: invalid default value",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`0`),
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule:              "self >= 1",
							MessageExpression: "'Expected integer greater or equal to 1, got %d'.format([self])",
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"integer\": Expected integer greater or equal to 1, got 0",
					"spec.variables[cpu].schema.openAPIV3Schema.default"),
			},
		},
		{
			name: "pass if x-kubernetes-validations has valid rule: valid default value (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"nestedField": {
								Type: "integer",
								Default: &apiextensionsv1.JSON{
									Raw: []byte(`1`),
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule: "self >= 1",
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has valid rule: invalid default value (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"nestedField": {
								Type: "integer",
								Default: &apiextensionsv1.JSON{
									Raw: []byte(`0`),
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule: "self >= 1",
								}},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"integer\": failed rule: self >= 1",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[nestedField].default"),
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid rule: invalid default value transition rule",
			// For CEL validation of default values, the default value is used as self & oldSelf.
			// This is because if a field is defaulted it would usually stay the same on updates,
			// so it has to be valid according to transition rules.
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`0`),
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self > oldSelf",
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"integer\": failed rule: self > oldSelf",
					"spec.variables[cpu].schema.openAPIV3Schema.default"),
			},
		},
		{
			name: "pass if x-kubernetes-validations has valid rule: valid default value transition rule",
			// For CEL validation of default values, the default value is used as self & oldSelf.
			// This is because if a field is defaulted it would usually stay the same on updates,
			// so it has to be valid according to transition rules.
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Default: &apiextensionsv1.JSON{
							Raw: []byte(`0`),
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self == oldSelf",
						}},
					},
				},
			},
		},
		// CEL: validation of examples
		{
			name: "pass if x-kubernetes-validations has valid rule: valid example value",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Example: &apiextensionsv1.JSON{
							Raw: []byte(`1`),
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self >= 1",
						}},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has valid rule: invalid example value",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Example: &apiextensionsv1.JSON{
							Raw: []byte(`0`),
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self >= 1",
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"integer\": failed rule: self >= 1",
					"spec.variables[cpu].schema.openAPIV3Schema.example"),
			},
		},
		{
			name: "pass if x-kubernetes-validations has valid rule: valid example value (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"nestedField": {
								Type: "integer",
								Example: &apiextensionsv1.JSON{
									Raw: []byte(`1`),
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule: "self >= 1",
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has valid rule: invalid example value (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"nestedField": {
								Type: "integer",
								Example: &apiextensionsv1.JSON{
									Raw: []byte(`0`),
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule: "self >= 1",
								}},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"integer\": failed rule: self >= 1",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[nestedField].example"),
			},
		},
		// CEL: validation of enums
		{
			name: "pass if x-kubernetes-validations has valid rule: valid enum value",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`1`)},
							{Raw: []byte(`2`)},
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self >= 1",
						}},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has valid rule: invalid enum value",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						Enum: []apiextensionsv1.JSON{
							{Raw: []byte(`0`)},
							{Raw: []byte(`1`)},
						},
						XValidations: []clusterv1.ValidationRule{{
							Rule: "self >= 1",
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"integer\": failed rule: self >= 1",
					"spec.variables[cpu].schema.openAPIV3Schema.enum[0]"),
			},
		},
		{
			name: "pass if x-kubernetes-validations has valid rule: valid enum value (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"nestedField": {
								Type: "integer",
								Enum: []apiextensionsv1.JSON{
									{Raw: []byte(`1`)},
									{Raw: []byte(`2`)},
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule: "self >= 1",
								}},
							},
						},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has valid rule: invalid enum value (nested)",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"nestedField": {
								Type: "integer",
								Enum: []apiextensionsv1.JSON{
									{Raw: []byte(`1`)},
									{Raw: []byte(`0`)},
								},
								XValidations: []clusterv1.ValidationRule{{
									Rule: "self >= 1",
								}},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \"integer\": failed rule: self >= 1",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[nestedField].enum[1]"),
			},
		},
		// CEL: reason
		{
			name: "fail if x-kubernetes-validations has invalid reason",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{
							{
								Rule:      "self.a > 0",
								Reason:    clusterv1.FieldValueErrorReason("InternalError"),
								FieldPath: ".a",
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"a": {
								Type: "number",
								XValidations: []clusterv1.ValidationRule{
									{
										Rule:   "true",
										Reason: clusterv1.FieldValueRequired,
									},
									{
										Rule:   "true",
										Reason: clusterv1.FieldValueInvalid,
									},
									{
										Rule:   "true",
										Reason: clusterv1.FieldValueDuplicate,
									},
									{
										Rule:   "true",
										Reason: clusterv1.FieldValueForbidden,
									},
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				unsupported("Unsupported value: \"InternalError\"",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[0].reason"),
			},
		},
		// CEL: field path
		{
			name: "fail if x-kubernetes-validations has invalid fieldPath for array",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{
							// Valid
							{
								Rule:      "true",
								FieldPath: ".foo['b.c']['c\\a']",
							},
							{
								Rule:      "true",
								FieldPath: "['a.c']",
							},
							{
								Rule:      "true",
								FieldPath: "['special.character.34$']",
							},
							{
								Rule:      "true",
								FieldPath: ".list", // referring to an entire list is supported
							},
							// Invalid
							{
								Rule:      "true",
								FieldPath: ".a.c",
							},
							{
								Rule:      "true",
								FieldPath: ".list[0]", // referring to an item of a list with a numeric index is not supported
							},
							{
								Rule:      "true",
								FieldPath: "   ",
							},
							{
								Rule:      "true",
								FieldPath: ".",
							},
							{
								Rule:      "true",
								FieldPath: "..",
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"a.c": {
								Type: "number",
							},
							"special.character.34$": {
								Type: "number",
							},
							"foo": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"b.c": {
										Type: "object",
										Properties: map[string]clusterv1.JSONSchemaProps{
											"c\a": {
												Type: "number",
											},
										},
									},
								},
							},
							"list": {
								Type: "array",
								Items: &clusterv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]clusterv1.JSONSchemaProps{
										"a": {
											Type: "number",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".a.c\": fieldPath must be a valid path: does not refer to a valid field",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[4].fieldPath"),
				invalid("Invalid value: \".list[0]\": fieldPath must be a valid path: expected single quoted string but got 0",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[5].fieldPath"),
				invalid("Invalid value: \"   \": fieldPath must be non-empty if specified",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[6].fieldPath"),
				invalid("Invalid value: \"   \": fieldPath must be a valid path: expected [ or . but got:    ",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[6].fieldPath"),
				invalid("Invalid value: \".\": fieldPath must be a valid path: unexpected end of JSON path",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[7].fieldPath"),
				invalid("Invalid value: \"..\": fieldPath must be a valid path: does not refer to a valid field",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[8].fieldPath"),
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid fieldPath",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{
							{
								Rule:      "self.a.b.c > 0.0",
								FieldPath: ".list[0].b",
							},
							{
								Rule:      "self.a.b.c > 0.0",
								FieldPath: ".list[0.b",
							},
							{
								Rule:      "self.a.b.c > 0.0",
								FieldPath: ".list0].b",
							},
							{
								Rule:      "self.a.b.c > 0.0",
								FieldPath: ".a.c",
							},
							{
								Rule:      "self.a.b.c > 0.0",
								FieldPath: ".a.b.d",
							},
							{
								Rule:      "self.a.b.c > 0.0",
								FieldPath: ".a\n",
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"a": {
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"b": {
										Type: "object",
										Properties: map[string]clusterv1.JSONSchemaProps{
											"c": {
												Type: "number",
											},
										},
									},
								},
							},
							"list": {
								Type: "array",
								Items: &clusterv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]clusterv1.JSONSchemaProps{
										"a": {
											Type: "number",
										},
									},
								},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \".list[0].b\": fieldPath must be a valid path: expected single quoted string but got 0",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[0].fieldPath"),
				invalid("Invalid value: \".list[0.b\": fieldPath must be a valid path: expected single quoted string but got 0",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[1].fieldPath"),
				invalid("Invalid value: \".list0].b\": fieldPath must be a valid path: does not refer to a valid field",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[2].fieldPath"),
				invalid("Invalid value: \".a.c\": fieldPath must be a valid path: does not refer to a valid field",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[3].fieldPath"),
				invalid("Invalid value: \".a.b.d\": fieldPath must be a valid path: does not refer to a valid field",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[4].fieldPath"),
				invalid("Invalid value: \".a\\n\": fieldPath must not contain line breaks",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[5].fieldPath"),
				invalid("Invalid value: \".a\\n\": fieldPath must be a valid path: does not refer to a valid field",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[5].fieldPath"),
			},
		},
		// CEL: message
		{
			name: "fail if x-kubernetes-validations has invalid message",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{
							// Valid
							{
								Rule:    "self.a > 0",
								Message: "valid message",
							},
							// Invalid
							{
								Rule:    "self.a > 0",
								Message: " ",
							},
							{
								Rule:    "self.a > 0",
								Message: strings.Repeat("a", 2049),
							},
							{
								Rule:    "self.a > 0",
								Message: "message with \n line break",
							},
							{
								Rule:    "self.a \n > 0",
								Message: "",
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"a": {
								Type: "number",
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("Invalid value: \" \": message must be non-empty if specified",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[1].message"),
				invalid("Invalid value: \"aaaaaaaaaa...\": message must have a maximum length of 2048 characters",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[2].message"),
				invalid("Invalid value: \"message with \\n line break\": message must not contain line breaks",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[3].message"),
				required("Required value: message must be specified if rule contains line breaks",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[4].message"),
			},
		},
		// CEL: messageExpression
		{
			name: "pass if x-kubernetes-validations has valid messageExpression",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{{
							Rule:              "self.replicas >= 1",
							MessageExpression: "'Expected integer greater or equal to 1, got %d'.format([self.replicas])",
						}},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"replicas": {
								Type: "integer",
							},
						},
					},
				},
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid messageExpression: must be non-empty if specified",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "var",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						XValidations: []clusterv1.ValidationRule{
							{
								Rule:              "self.a > 0",
								MessageExpression: " ",
							},
						},
						Properties: map[string]clusterv1.JSONSchemaProps{
							"a": {
								Type: "number",
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				required("Required value: messageExpression must be non-empty if specified",
					"spec.variables[var].schema.openAPIV3Schema.x-kubernetes-validations[0].messageExpression"),
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid messageExpression: invalid CEL expression",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "integer",
						XValidations: []clusterv1.ValidationRule{{
							Rule:              "self >= 1",
							MessageExpression: "'Expected integer greater or equal to 1, got ' + this does not compile",
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				invalid("messageExpression compilation failed: ERROR: <input>:1:55: Syntax error: mismatched input 'does' expecting <EOF>",
					"spec.variables[cpu].schema.openAPIV3Schema.x-kubernetes-validations[0].messageExpression"),
			},
		},
		{
			name: "fail if x-kubernetes-validations has invalid messageExpression: cost total exceeds total limit",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "cpu",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"array": {
								Type: "array",
								Items: &clusterv1.JSONSchemaProps{
									Type:      "string",
									MaxLength: ptr.To[int64](500000),
									XValidations: []clusterv1.ValidationRule{{
										Rule: "true",
										// Using + and string() exceeds the cost limit
										MessageExpression: "'contains: ' + string(self.contains('keyword'))",
									}},
								},
							},
							"field": {
								// Include a messageExpression that does not contribute to total limit being exceeded
								Type: "integer",
								XValidations: []clusterv1.ValidationRule{{
									Rule:              "self > 50 && self < 100",
									MessageExpression: "'Expected integer to be between 50 and 100, got %d'.format([self])",
								}},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				forbidden("estimated messageExpression cost exceeds budget by factor of more than 100x",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[array].items.x-kubernetes-validations[0].messageExpression"),
				forbidden("contributed to estimated rule & messageExpression cost total exceeding cost limit",
					"spec.variables[cpu].schema.openAPIV3Schema.properties[array].items.x-kubernetes-validations[0].messageExpression"),
				forbidden("x-kubernetes-validations estimated rule & messageExpression cost total for entire OpenAPIv3 schema exceeds budget by factor of more than 100x",
					"spec.variables[cpu].schema.openAPIV3Schema"),
			},
		},
		{
			name: "fail if x-kubernetes-validations are under oneOf/anyOf/allOf/not",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variableA",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "number",
						Not: &clusterv1.JSONSchemaProps{
							XValidations: []clusterv1.ValidationRule{{
								Rule: "should be forbidden",
							}},
						},
						AnyOf: []clusterv1.JSONSchemaProps{{
							XValidations: []clusterv1.ValidationRule{{
								Rule: "should be forbidden",
							}},
						}},
						AllOf: []clusterv1.JSONSchemaProps{{
							XValidations: []clusterv1.ValidationRule{{
								Rule: "should be forbidden",
							}},
						}},
						OneOf: []clusterv1.JSONSchemaProps{{
							XValidations: []clusterv1.ValidationRule{{
								Rule: "should be forbidden",
							}},
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.allOf[0].x-kubernetes-validations"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.anyOf[0].x-kubernetes-validations"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.not.x-kubernetes-validations"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.oneOf[0].x-kubernetes-validations"),
			},
		},
		{
			name: "fail if defaults are under oneOf/anyOf/allOf/not",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variableA",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "number",
						Not: &clusterv1.JSONSchemaProps{
							Default: &apiextensionsv1.JSON{
								Raw: []byte(`42.0`),
							},
						},
						AnyOf: []clusterv1.JSONSchemaProps{{
							Default: &apiextensionsv1.JSON{
								Raw: []byte(`42.0`),
							},
						}},
						AllOf: []clusterv1.JSONSchemaProps{{
							Default: &apiextensionsv1.JSON{
								Raw: []byte(`42.0`),
							},
						}},
						OneOf: []clusterv1.JSONSchemaProps{{
							Default: &apiextensionsv1.JSON{
								Raw: []byte(`42.0`),
							},
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				forbidden("must be undefined to be structural", "spec.variables[variableA].schema.openAPIV3Schema.allOf[0].default"),
				forbidden("must be undefined to be structural", "spec.variables[variableA].schema.openAPIV3Schema.anyOf[0].default"),
				forbidden("must be undefined to be structural", "spec.variables[variableA].schema.openAPIV3Schema.not.default"),
				forbidden("must be undefined to be structural", "spec.variables[variableA].schema.openAPIV3Schema.oneOf[0].default"),
			},
		}, {
			name: "fail if there are types set under oneOf/anyOf/allOf/not",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variableA",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "number",
						Not: &clusterv1.JSONSchemaProps{
							Properties: map[string]clusterv1.JSONSchemaProps{
								"testField": {
									Type: "number",
								},
							},
						},
						AnyOf: []clusterv1.JSONSchemaProps{{
							Properties: map[string]clusterv1.JSONSchemaProps{
								"testField": {
									Type: "number",
								},
							},
						}},
						AllOf: []clusterv1.JSONSchemaProps{{
							Properties: map[string]clusterv1.JSONSchemaProps{
								"testField": {
									Type: "number",
								},
							},
						}},
						OneOf: []clusterv1.JSONSchemaProps{{
							Properties: map[string]clusterv1.JSONSchemaProps{
								"testField": {
									Type: "number",
								},
							},
						}},
					},
				},
			},
			wantErrs: []validationMatch{
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.allOf[0].properties[testField].type"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.anyOf[0].properties[testField].type"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.not.properties[testField].type"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.oneOf[0].properties[testField].type"),
			},
		}, {
			name: "pass if oneOf/anyOf/allOf/not schemas are valid",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variableA",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "string",
						Not: &clusterv1.JSONSchemaProps{
							Format: "email",
						},
						AnyOf: []clusterv1.JSONSchemaProps{{
							Format: "ipv4",
						}, {
							Format: "ipv6",
						}},
						AllOf: []clusterv1.JSONSchemaProps{{
							Format: "ipv4",
						}, {
							Format: "ipv6",
						}},
						OneOf: []clusterv1.JSONSchemaProps{{
							Format: "ipv4",
						}, {
							Format: "ipv6",
						}},
					},
				},
			},
		}, {
			name: "pass & fail correctly for int-or-string in oneOf/anyOf/allOf schemas",
			clusterClassVariable: &clusterv1.ClusterClassVariable{
				Name: "variableA",
				Schema: clusterv1.VariableSchema{
					OpenAPIV3Schema: clusterv1.JSONSchemaProps{
						Type: "object",
						Properties: map[string]clusterv1.JSONSchemaProps{
							"anyOfExampleField": { // Valid variant
								XIntOrString: true,
								AnyOf: []clusterv1.JSONSchemaProps{{
									Type: "integer",
								}, {
									Type: "string",
								}},
							},
							"allOfExampleFieldWithAnyOf": { // Valid variant
								XIntOrString: true,
								AllOf: []clusterv1.JSONSchemaProps{{
									AnyOf: []clusterv1.JSONSchemaProps{{
										Type: "integer",
									}, {
										Type: "string",
									}},
								}},
							},
							"allOfExampleField": {
								AllOf: []clusterv1.JSONSchemaProps{{
									Type: "integer",
								}, {
									Type: "string",
								}},
							},
							"oneOfExampleField": {
								OneOf: []clusterv1.JSONSchemaProps{{
									Type: "integer",
								}, {
									Type: "string",
								}},
							},
						},
					},
				},
			},
			wantErrs: []validationMatch{
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.properties[allOfExampleField].allOf[0].type"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.properties[allOfExampleField].allOf[1].type"),
				required("must not be empty for specified object fields", "spec.variables[variableA].schema.openAPIV3Schema.properties[allOfExampleField].type"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.properties[oneOfExampleField].oneOf[0].type"),
				forbidden("must be empty to be structural", "spec.variables[variableA].schema.openAPIV3Schema.properties[oneOfExampleField].oneOf[1].type"),
				required("must not be empty for specified object fields", "spec.variables[variableA].schema.openAPIV3Schema.properties[oneOfExampleField].type"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErrs := validateClusterClassVariable(ctx,
				nil,
				tt.clusterClassVariable,
				field.NewPath("spec", "variables").Key(tt.clusterClassVariable.Name))

			checkErrors(t, tt.wantErrs, gotErrs)
		})
	}
}

func checkErrors(t *testing.T, wantErrs []validationMatch, gotErrs field.ErrorList) {
	t.Helper()

	var foundMismatch bool
	if len(gotErrs) != len(wantErrs) {
		foundMismatch = true
	} else {
		for i := range wantErrs {
			if !wantErrs[i].matches(gotErrs[i]) {
				foundMismatch = true
				break
			}
		}
	}

	if foundMismatch {
		var wantErrsStr []string
		for i := range wantErrs {
			wantErrsStr = append(wantErrsStr, fmt.Sprintf("type: %s\ntext: %s\npath: %s", string(wantErrs[i].Type), wantErrs[i].containsString, wantErrs[i].Field))
		}
		var gotErrsStr []string
		for i := range gotErrs {
			gotErrsStr = append(gotErrsStr, fmt.Sprintf("type: %s\ntext: %s\npath: %s", string(gotErrs[i].Type), gotErrs[i].Error(), gotErrs[i].Field))
		}
		t.Errorf("expected %d errors, got %d\ngot errors:\n\n%s\n\nwant errors:\n\n%s\n",
			len(wantErrs), len(gotErrs), strings.Join(gotErrsStr, "\n\n"), strings.Join(wantErrsStr, "\n\n"))
	}
}

type validationMatch struct {
	Field          *field.Path
	Type           field.ErrorType
	containsString string
}

func (v validationMatch) matches(err *field.Error) bool {
	return err.Type == v.Type && err.Field == v.Field.String() && strings.Contains(err.Error(), v.containsString)
}

func toolong(containsString string, path string) validationMatch {
	return validationMatch{containsString: containsString, Field: field.NewPath(strings.Split(path, ".")[0], strings.Split(path, ".")[1:]...), Type: field.ErrorTypeTooLong}
}

func toomany(containsString string, path string) validationMatch {
	return validationMatch{containsString: containsString, Field: field.NewPath(strings.Split(path, ".")[0], strings.Split(path, ".")[1:]...), Type: field.ErrorTypeTooMany}
}

func required(containsString string, path string) validationMatch {
	return validationMatch{containsString: containsString, Field: field.NewPath(strings.Split(path, ".")[0], strings.Split(path, ".")[1:]...), Type: field.ErrorTypeRequired}
}

func invalidType(containsString string, path string) validationMatch {
	return validationMatch{containsString: containsString, Field: field.NewPath(strings.Split(path, ".")[0], strings.Split(path, ".")[1:]...), Type: field.ErrorTypeTypeInvalid}
}

func invalid(containsString string, path string) validationMatch {
	return validationMatch{containsString: containsString, Field: field.NewPath(strings.Split(path, ".")[0], strings.Split(path, ".")[1:]...), Type: field.ErrorTypeInvalid}
}

func unsupported(containsString string, path string) validationMatch {
	return validationMatch{containsString: containsString, Field: field.NewPath(strings.Split(path, ".")[0], strings.Split(path, ".")[1:]...), Type: field.ErrorTypeNotSupported}
}

func forbidden(containsString string, path string) validationMatch {
	return validationMatch{containsString: containsString, Field: field.NewPath(strings.Split(path, ".")[0], strings.Split(path, ".")[1:]...), Type: field.ErrorTypeForbidden}
}
