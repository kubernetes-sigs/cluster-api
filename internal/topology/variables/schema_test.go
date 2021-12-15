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
	"reflect"
	"testing"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"

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
				MaxLength:        pointer.Int64(4),
				MinLength:        pointer.Int64(2),
				Pattern:          "abc.*",
				Maximum:          pointer.Int64(43),
				ExclusiveMaximum: true,
				Minimum:          pointer.Int64(1),
				ExclusiveMinimum: false,
			},
			want: &apiextensions.JSONSchemaProps{
				Type:             "integer",
				Format:           "uri",
				MaxLength:        pointer.Int64(4),
				MinLength:        pointer.Int64(2),
				Pattern:          "abc.*",
				Maximum:          pointer.Float64(43),
				ExclusiveMaximum: true,
				Minimum:          pointer.Float64(1),
				ExclusiveMinimum: false,
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
						Minimum: pointer.Int64(1),
					},
					"property2": {
						Type:      "string",
						Format:    "uri",
						MinLength: pointer.Int64(2),
						MaxLength: pointer.Int64(4),
					},
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Properties: map[string]apiextensions.JSONSchemaProps{
					"property1": {
						Type:    "integer",
						Minimum: pointer.Float64(1),
					},
					"property2": {
						Type:      "string",
						Format:    "uri",
						MinLength: pointer.Int64(2),
						MaxLength: pointer.Int64(4),
					},
				},
			},
		},
		{
			name: "pass for schema validation with array",
			schema: &clusterv1.JSONSchemaProps{
				Items: &clusterv1.JSONSchemaProps{
					Type:      "integer",
					Minimum:   pointer.Int64(1),
					Format:    "uri",
					MinLength: pointer.Int64(2),
					MaxLength: pointer.Int64(4),
				},
			},
			want: &apiextensions.JSONSchemaProps{
				Items: &apiextensions.JSONSchemaPropsOrArray{
					Schema: &apiextensions.JSONSchemaProps{
						Type:      "integer",
						Minimum:   pointer.Float64(1),
						Format:    "uri",
						MinLength: pointer.Int64(2),
						MaxLength: pointer.Int64(4),
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertToAPIExtensionsJSONSchemaProps() got = %v, want %v", got, tt.want)
			}
		})
	}
}
