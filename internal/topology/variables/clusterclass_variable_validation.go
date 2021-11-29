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

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	structuraldefaulting "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ValidateClusterClassVariables validates clusterClassVariable.
func ValidateClusterClassVariables(clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	allErrs = append(allErrs, validateClusterClassVariableNamesUnique(clusterClassVariables, fldPath)...)

	for i := range clusterClassVariables {
		allErrs = append(allErrs, validateClusterClassVariable(&clusterClassVariables[i], fldPath.Index(i))...)
	}

	return allErrs
}

// validateClusterClassVariableNamesUnique validates that ClusterClass variable names are unique.
func validateClusterClassVariableNamesUnique(clusterClassVariables []clusterv1.ClusterClassVariable, pathPrefix *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	variableNames := sets.NewString()
	for i, clusterClassVariable := range clusterClassVariables {
		if variableNames.Has(clusterClassVariable.Name) {
			allErrs = append(allErrs,
				field.Invalid(
					pathPrefix.Index(i).Child("name"),
					clusterClassVariable.Name,
					fmt.Sprintf("variable names must be unique. %v is repeated.", clusterClassVariable.Name),
				),
			)
		}
		variableNames.Insert(clusterClassVariable.Name)
	}

	return allErrs
}

// validateClusterClassVariable validates a ClusterClassVariable.
func validateClusterClassVariable(variable *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate variable name.
	allErrs = append(allErrs, validateClusterClassVariableName(variable.Name, fldPath.Child("name"))...)

	// Validate schema.
	allErrs = append(allErrs, validateSchema(&variable.Schema.OpenAPIV3Schema, variable.Name, fldPath.Child("schema", "openAPIV3Schema"))...)

	return allErrs
}

// validateClusterClassVariableName validates a variable name.
func validateClusterClassVariableName(variableName string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if variableName == "" {
		allErrs = append(allErrs, field.Invalid(fldPath, variableName, "name cannot be empty"))
	}

	// TODO(sbueringer): This should share a constant with controllers/topology/internal/extensions/patches/variables.BuiltinsName.
	if variableName == "builtin" {
		allErrs = append(allErrs, field.Invalid(fldPath, variableName, "name cannot be \"builtin\""))
	}
	if strings.Contains(variableName, ".") {
		allErrs = append(allErrs, field.Invalid(fldPath, variableName, "name cannot contain \".\""))
	}

	return allErrs
}

var validVariableTypes = sets.NewString("object", "string", "number", "integer", "boolean")

// validateSchema validates the schema.
func validateSchema(schema *clusterv1.JSONSchemaProps, parentName string, fldPath *field.Path) field.ErrorList {
	apiExtensionsSchema, err := convertToAPIExtensionsJSONSchemaProps(schema)
	if err != nil {
		return field.ErrorList{field.Invalid(fldPath, schema,
			fmt.Sprintf("invalid schema in ClusterClass for variable %q: error to convert schema %v", parentName, err))}
	}

	// Validate that type is one of the validVariableTypes.
	switch {
	case len(apiExtensionsSchema.Type) == 0:
		return field.ErrorList{field.Required(fldPath.Child("type"), "type cannot be empty")}
	case apiExtensionsSchema.Type == "null":
		return field.ErrorList{field.Forbidden(fldPath.Child("type"), "type cannot be set to null, use nullable as an alternative")}
	case !validVariableTypes.Has(apiExtensionsSchema.Type):
		return field.ErrorList{field.NotSupported(fldPath.Child("type"), apiExtensionsSchema.Type, validVariableTypes.List())}
	}

	allErrs := field.ErrorList{}
	if len(schema.Properties) != 0 {
		for propertyName, propertySchema := range schema.Properties {
			p := propertySchema
			allErrs = append(allErrs, validateSchema(&p, propertyName, fldPath.Child("properties").Key(propertyName))...)
		}
	}

	// Validate structural schema.
	// Note: structural schema only allows `type: object` on the root level, so we wrap the schema with:
	// type: object
	// properties:
	//   variableSchema: <variable-schema>
	wrappedSchema := &apiextensions.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensions.JSONSchemaProps{
			"variableSchema": *apiExtensionsSchema,
		},
	}

	// Get the structural schema for the variable.
	ss, err := structuralschema.NewStructural(wrappedSchema)
	if err != nil {
		return append(allErrs, field.Invalid(fldPath.Child("schema"), "", err.Error()))
	}

	// Validate the schema.
	if validationErrors := structuralschema.ValidateStructural(fldPath.Child("schema"), ss); len(validationErrors) > 0 {
		return append(allErrs, validationErrors...)
	}

	// Validate defaults in the structural schema.
	validationErrors, err := structuraldefaulting.ValidateDefaults(fldPath.Child("schema"), ss, true, true)
	if err != nil {
		return append(allErrs, field.Invalid(fldPath.Child("schema"), "", err.Error()))
	}
	if len(validationErrors) > 0 {
		return append(allErrs, validationErrors...)
	}

	// If the structural schema is valid, ensure a schema validator can be constructed.
	if _, _, err := validation.NewSchemaValidator(&apiextensions.CustomResourceValidation{
		OpenAPIV3Schema: apiExtensionsSchema,
	}); err != nil {
		return append(allErrs, field.Invalid(fldPath, "", fmt.Sprintf("failed to build validator: %v", err)))
	}

	return allErrs
}
