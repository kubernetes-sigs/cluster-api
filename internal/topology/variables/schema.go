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
	"encoding/json"
	"fmt"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// convertToAPIExtensionsJSONSchemaProps converts a clusterv1.JSONSchemaProps to apiextensions.JSONSchemaProp.
// NOTE: This is used whenever we want to use one of the upstream libraries, as they use apiextensions.JSONSchemaProp.
// NOTE: If new fields are added to clusterv1.JSONSchemaProps (e.g. to support complex types), the corresponding
// schema validation must be added to validateRootSchema too.
func convertToAPIExtensionsJSONSchemaProps(schema *clusterv1.JSONSchemaProps, fldPath *field.Path) (*apiextensions.JSONSchemaProps, field.ErrorList) {
	var allErrs field.ErrorList

	props := &apiextensions.JSONSchemaProps{
		Type:             schema.Type,
		Required:         schema.Required,
		MaxProperties:    schema.MaxProperties,
		MinProperties:    schema.MinProperties,
		MaxItems:         schema.MaxItems,
		MinItems:         schema.MinItems,
		UniqueItems:      schema.UniqueItems,
		Format:           schema.Format,
		MaxLength:        schema.MaxLength,
		MinLength:        schema.MinLength,
		Pattern:          schema.Pattern,
		ExclusiveMaximum: schema.ExclusiveMaximum,
		ExclusiveMinimum: schema.ExclusiveMinimum,
		XIntOrString:     schema.XIntOrString,
	}

	// Only set XPreserveUnknownFields to true if it's true.
	// apiextensions.JSONSchemaProps only allows setting XPreserveUnknownFields
	// to true or undefined, false is forbidden.
	if schema.XPreserveUnknownFields {
		props.XPreserveUnknownFields = ptr.To(true)
	}

	if schema.Default != nil && schema.Default.Raw != nil {
		var v interface{}
		if err := json.Unmarshal(schema.Default.Raw, &v); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("default"), string(schema.Default.Raw),
				fmt.Sprintf("default is not valid JSON: %v", err)))
		} else {
			var v apiextensions.JSON
			err := apiextensionsv1.Convert_v1_JSON_To_apiextensions_JSON(schema.Default, &v, nil)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("default"), string(schema.Default.Raw),
					fmt.Sprintf("failed to convert default: %v", err)))
			} else {
				props.Default = &v
			}
		}
	}

	if len(schema.Enum) > 0 {
		for i, enum := range schema.Enum {
			if enum.Raw == nil {
				continue
			}

			var v interface{}
			if err := json.Unmarshal(enum.Raw, &v); err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("enum").Index(i), string(enum.Raw),
					fmt.Sprintf("enum value is not valid JSON: %v", err)))
			} else {
				var v apiextensions.JSON
				err := apiextensionsv1.Convert_v1_JSON_To_apiextensions_JSON(&schema.Enum[i], &v, nil)
				if err != nil {
					allErrs = append(allErrs, field.Invalid(fldPath.Child("enum").Index(i), string(enum.Raw),
						fmt.Sprintf("failed to convert enum value: %v", err)))
				} else {
					props.Enum = append(props.Enum, v)
				}
			}
		}
	}

	if schema.Example != nil && schema.Example.Raw != nil {
		var v interface{}
		if err := json.Unmarshal(schema.Example.Raw, &v); err != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("example"), string(schema.Example.Raw),
				fmt.Sprintf("example is not valid JSON: %v", err)))
		} else {
			var value apiextensions.JSON
			err := apiextensionsv1.Convert_v1_JSON_To_apiextensions_JSON(schema.Example, &value, nil)
			if err != nil {
				allErrs = append(allErrs, field.Invalid(fldPath.Child("example"), string(schema.Example.Raw),
					fmt.Sprintf("failed to convert example value: %v", err)))
			} else {
				props.Example = &value
			}
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

	if schema.AdditionalProperties != nil {
		apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(schema.AdditionalProperties, fldPath.Child("additionalProperties"))
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		} else {
			props.AdditionalProperties = &apiextensions.JSONSchemaPropsOrBool{
				// Allows must be true to allow "additional properties".
				// Otherwise only the ones from .Properties are allowed.
				Allows: true,
				Schema: apiExtensionsSchema,
			}
		}
	}

	if len(schema.Properties) > 0 {
		props.Properties = map[string]apiextensions.JSONSchemaProps{}
		for propertyName, propertySchema := range schema.Properties {
			p := propertySchema
			apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(&p, fldPath.Child("properties").Key(propertyName))
			if len(errs) > 0 {
				allErrs = append(allErrs, errs...)
			} else {
				props.Properties[propertyName] = *apiExtensionsSchema
			}
		}
	}

	if schema.Items != nil {
		apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(schema.Items, fldPath.Child("items"))
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		} else {
			props.Items = &apiextensions.JSONSchemaPropsOrArray{
				Schema: apiExtensionsSchema,
			}
		}
	}

	if len(schema.OneOf) > 0 {
		props.OneOf = make([]apiextensions.JSONSchemaProps, 0, len(schema.OneOf))
		for idx, oneOf := range schema.OneOf {
			apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(&oneOf, fldPath.Child("oneOf").Index(idx))
			if len(errs) > 0 {
				allErrs = append(allErrs, errs...)
			} else {
				props.OneOf = append(props.OneOf, *apiExtensionsSchema)
			}
		}
	}

	if len(schema.AnyOf) > 0 {
		props.AnyOf = make([]apiextensions.JSONSchemaProps, 0, len(schema.AnyOf))
		for idx, anyOf := range schema.AnyOf {
			apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(&anyOf, fldPath.Child("anyOf").Index(idx))
			if len(errs) > 0 {
				allErrs = append(allErrs, errs...)
			} else {
				props.AnyOf = append(props.AnyOf, *apiExtensionsSchema)
			}
		}
	}

	if len(schema.AllOf) > 0 {
		props.AllOf = make([]apiextensions.JSONSchemaProps, 0, len(schema.AllOf))
		for idx, allOf := range schema.AllOf {
			apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(&allOf, fldPath.Child("allOf").Index(idx))
			if len(errs) > 0 {
				allErrs = append(allErrs, errs...)
			} else {
				props.AllOf = append(props.AllOf, *apiExtensionsSchema)
			}
		}
	}

	if schema.Not != nil {
		apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(schema.Not, fldPath.Child("not"))
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
		} else {
			props.Not = apiExtensionsSchema
		}
	}

	if schema.XValidations != nil {
		props.XValidations = convertToAPIExtensionsXValidations(schema.XValidations)
	}

	return props, allErrs
}

func convertToAPIExtensionsXValidations(validationRules []clusterv1.ValidationRule) apiextensions.ValidationRules {
	apiExtValidationRules := make(apiextensions.ValidationRules, 0, len(validationRules))

	for _, validationRule := range validationRules {
		var reason *apiextensions.FieldValueErrorReason
		if validationRule.Reason != "" {
			reason = ptr.To(apiextensions.FieldValueErrorReason(validationRule.Reason))
		}

		apiExtValidationRules = append(
			apiExtValidationRules,
			apiextensions.ValidationRule{
				Rule:              validationRule.Rule,
				Message:           validationRule.Message,
				MessageExpression: validationRule.MessageExpression,
				Reason:            reason,
				FieldPath:         validationRule.FieldPath,
			},
		)
	}

	return apiExtValidationRules
}
