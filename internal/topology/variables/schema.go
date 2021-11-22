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
	"github.com/pkg/errors"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// convertToAPIExtensionsJSONSchemaProps converts a clusterv1.JSONSchemaProps to apiextensions.JSONSchemaProp.
// NOTE: This is used whenever we want to use one of the upstream libraries, as they use apiextensions.JSONSchemaProp.
// NOTE: If new fields are added to clusterv1.JSONSchemaProps (e.g. to support complex types), the corresponding
// schema validation must be added to validateSchema too.
func convertToAPIExtensionsJSONSchemaProps(schema *clusterv1.JSONSchemaProps) (*apiextensions.JSONSchemaProps, error) {
	props := &apiextensions.JSONSchemaProps{
		Type:             schema.Type,
		Nullable:         schema.Nullable,
		Format:           schema.Format,
		MaxLength:        schema.MaxLength,
		MinLength:        schema.MinLength,
		Pattern:          schema.Pattern,
		ExclusiveMaximum: schema.ExclusiveMaximum,
		ExclusiveMinimum: schema.ExclusiveMinimum,
		Required:         schema.Required,
	}

	if len(schema.Properties) > 0 {
		props.Properties = map[string]apiextensions.JSONSchemaProps{}

		for propertyName, propertySchema := range schema.Properties {
			p := propertySchema
			upstreamPropertySchema, err := convertToAPIExtensionsJSONSchemaProps(&p)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to convert schema of field %q", propertyName)
			}
			props.Properties[propertyName] = *upstreamPropertySchema
		}
	}

	if schema.Items != nil {
		upstreamSchema, err := convertToAPIExtensionsJSONSchemaProps(schema.Items)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert schema of items")
		}
		props.Items = &apiextensions.JSONSchemaPropsOrArray{
			Schema: upstreamSchema,
		}
	}

	if schema.Default != nil {
		var v apiextensions.JSON
		err := apiextensionsv1.Convert_v1_JSON_To_apiextensions_JSON(schema.Default, &v, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert default value %q", *schema.Default)
		}
		props.Default = &v
	}

	if len(schema.Enum) > 0 {
		for i := range schema.Enum {
			var v apiextensions.JSON
			err := apiextensionsv1.Convert_v1_JSON_To_apiextensions_JSON(&schema.Enum[i], &v, nil)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to convert enum value %q", schema.Enum[i])
			}
			props.Enum = append(props.Enum, v)
		}
	}

	if schema.Maximum != nil {
		f := float64(*schema.Maximum)
		props.Maximum = &f
	}

	if schema.Minimum != nil {
		f := float64(*schema.Minimum)
		props.Minimum = &f
	}

	return props, nil
}
