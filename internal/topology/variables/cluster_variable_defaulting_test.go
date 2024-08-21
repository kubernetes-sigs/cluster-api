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
		wantErrs        []validationMatch
	}{
		{
			name: "Return error if variable is not defined in ClusterClass",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"1\": variable is not defined",
					"spec.topology.variables[cpu]"),
			},
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
		},
		{
			name: "Return error if variable has no definitions in ClusterClass",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"[Name: cpu]\": variable definitions in the ClusterClass not valid: variable \"cpu\" has no definitions",
					"spec.topology.variables"),
			},
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:        "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{},
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
			createVariables: true,
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
			name: "Don't default if a value is set",
			definitions: []clusterv1.ClusterClassStatusVariable{
				{
					Name:                "cpu",
					DefinitionsConflict: false,
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
					Name:                "cpu",
					DefinitionsConflict: false,
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
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`1`),
					},
				},
			},
		},
		// Variables with conflicting definitions.
		{
			name: "Error if there are any variable conflicts",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"[Name: cpu]\": variable definitions in the ClusterClass not valid: variable \"cpu\" has conflicting definitions",
					"spec.topology.variables"),
			},
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
		},
		{
			name: "Error if there are any variable conflicts when the value is set",
			wantErrs: []validationMatch{
				invalidType("Invalid value: \"[Name: cpu]\": variable definitions in the ClusterClass not valid: variable \"cpu\" has conflicting definitions",
					"spec.topology.variables"),
			},
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
				},
			},
			createVariables: true,
		},
		{
			name: "Error if a value is set with non-empty definitionFrom.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"2\": variable \"cpu\" has DefinitionFrom set. DefinitionFrom is deprecated, must not be set anymore and is going to be removed in the next apiVersion",
					"spec.topology.variables[cpu]"),
				invalid("Invalid value: \"2\": variable \"cpu\" is set more than once",
					"spec.topology.variables[cpu]"),
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
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
					// Non-empty definitionFrom is not valid.
					DefinitionFrom: "somepatch",
				},
			},
		},
		{
			name: "Error if a value is set twice.",
			wantErrs: []validationMatch{
				invalid("Invalid value: \"2\": variable \"cpu\" is set more than once",
					"spec.topology.variables[cpu]"),
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
				},
				{
					Name: "cpu",
					Value: apiextensionsv1.JSON{
						Raw: []byte(`2`),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			vars, gotErrs := defaultClusterVariables(tt.values, tt.definitions, tt.createVariables,
				field.NewPath("spec", "topology", "variables"))

			checkErrors(t, tt.wantErrs, gotErrs)
			g.Expect(vars).To(BeComparableTo(tt.want))
		})
	}
}

func Test_DefaultClusterVariable(t *testing.T) {
	tests := []struct {
		name                 string
		clusterVariable      *clusterv1.ClusterVariable
		clusterClassVariable *clusterv1.ClusterClassStatusVariable
		createVariable       bool
		want                 *clusterv1.ClusterVariable
		wantErrs             []validationMatch
	}{
		{
			name: "Default new integer variable",
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "cpu",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "cpu",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
							},
						},
					},
				},
			},
			createVariable: false,
			want:           nil,
		},
		{
			name: "Don't default existing integer variable",
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "cpu",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Default: &apiextensionsv1.JSON{Raw: []byte(`1`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "location",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "string",
								Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "location",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "string",
								Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "cpu",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "number",
								Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "cpu",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "number",
								Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "correct",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "boolean",
								Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "correct",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "boolean",
								Default: &apiextensionsv1.JSON{Raw: []byte(`true`)},
							},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
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
			},
			createVariable: true,
			want:           nil,
		},
		{
			name: "Default new object variable if there is a top-level default",
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "object",
								Default: &apiextensionsv1.JSON{Raw: []byte(`{"url":"test-url"}`)},
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
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					// Defaulting first adds the top-level object and then the enabled field.
					Raw: []byte(`{"enabled":false,"url":"test-url"}`),
				},
			},
		},
		{
			name: "Default new object variable if there is a top-level default (which is an empty object)",
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "object",
								Default: &apiextensionsv1.JSON{Raw: []byte(`{}`)},
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
			createVariable: true,
			want: &clusterv1.ClusterVariable{
				Name: "httpProxy",
				Value: apiextensionsv1.JSON{
					// Defaulting first adds the top-level empty object and then the enabled field.
					Raw: []byte(`{"enabled":false}`),
				},
			},
		},
		{
			name: "Don't default existing object variable",
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "object",
								Default: &apiextensionsv1.JSON{Raw: []byte(`{"enabled": true}`)},
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{

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
					// url is added by defaulting.
					Raw: []byte(`{"enabled":false,"url":"https://example.com"}`),
				},
			},
		},
		{
			name: "Default new map variable",
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "httpProxy",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
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
					// url is added by defaulting.
					Raw: []byte(`{"proxy1":{"enabled":false,"url":"https://example.com"},"proxy2":{"enabled":false,"url":"https://example.com"}}`),
				},
			},
		},
		{
			name: "Default new array variable",
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "testVariable",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{
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
			clusterClassVariable: &clusterv1.ClusterClassStatusVariable{
				Name: "testVariable",
				Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
					{

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

			defaultedVariable, gotErrs := defaultValue(tt.clusterVariable, tt.clusterClassVariable,
				field.NewPath("spec", "topology", "variables").Key(tt.clusterClassVariable.Name), tt.createVariable)

			checkErrors(t, tt.wantErrs, gotErrs)
			g.Expect(defaultedVariable).To(BeComparableTo(tt.want))
		})
	}
}

func Test_getAllVariables(t *testing.T) {
	g := NewWithT(t)
	t.Run("Expect values to be correctly consolidated in allVariables", func(*testing.T) {
		expectedValues := []clusterv1.ClusterVariable{
			// var1 is set and has only one definition.
			// It should be retained as is.
			{
				Name: "var1",
			},

			// var2 is set and has two definitions.
			// The value should be retained and no additional value should be added.
			{
				Name: "var2",
			},

			// var3 is not set and has only one definition.
			// One additional value should be added.
			{
				Name: "var3",
			},

			// var4 is not set and has two definitions.
			// One additional value should be added.
			{
				Name: "var4",
			},
		}

		values := []clusterv1.ClusterVariable{
			{
				Name: "var1",
			},
			{
				Name: "var2",
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
				Name: "var3",
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

		valuesIndex, err := newValuesIndex(field.NewPath("spec", "topology", "variables"), values)
		g.Expect(err).To(BeEmpty())
		got := getAllVariables(values, valuesIndex, definitions)
		g.Expect(got).To(BeComparableTo(expectedValues))
	})
}
