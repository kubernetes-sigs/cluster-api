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
	"context"
	"fmt"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	structuralpruning "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ValidateClusterVariables validates ClusterVariables based on the definitions in ClusterClass `.status.variables`.
func ValidateClusterVariables(ctx context.Context, values, oldValues []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, fldPath *field.Path) field.ErrorList {
	return validateClusterVariables(ctx, values, oldValues, definitions, true, fldPath)
}

// ValidateControlPlaneVariables validates ControlPLane variables.
func ValidateControlPlaneVariables(ctx context.Context, values, oldValues []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, fldPath *field.Path) field.ErrorList {
	return validateClusterVariables(ctx, values, oldValues, definitions, false, fldPath)
}

// ValidateMachineVariables validates MachineDeployment and MachinePool variables.
func ValidateMachineVariables(ctx context.Context, values, oldValues []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, fldPath *field.Path) field.ErrorList {
	return validateClusterVariables(ctx, values, oldValues, definitions, false, fldPath)
}

// validateClusterVariables validates variable values according to the corresponding definition.
func validateClusterVariables(ctx context.Context, values, oldValues []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, validateRequired bool, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Get a map of ClusterVariable values. This function validates that:
	// - variables are not defined more than once in Cluster spec.
	// - variables with the same name do not have a mix of empty and non-empty DefinitionFrom.
	valuesMap, err := newValuesIndex(values)
	if err != nil {
		var valueStrings []string
		for _, v := range values {
			valueStrings = append(valueStrings, fmt.Sprintf("Name: %s DefinitionFrom: %s", v.Name, v.DefinitionFrom))
		}
		return append(allErrs, field.Invalid(fldPath, "["+strings.Join(valueStrings, ",")+"]", fmt.Sprintf("cluster variables not valid: %s", err)))
	}

	// Get a map of old ClusterVariable values. We know they are all valid and not duplicate names, etc. as previous
	// validation has already asserted that.
	oldValuesMap, err := newValuesIndex(oldValues)
	if err != nil {
		var valueStrings []string
		for _, v := range values {
			valueStrings = append(valueStrings, fmt.Sprintf("Name: %s DefinitionFrom: %s", v.Name, v.DefinitionFrom))
		}
		return append(allErrs, field.Invalid(fldPath, "["+strings.Join(valueStrings, ",")+"]", fmt.Sprintf("old cluster variables not valid: %s", err)))
	}

	// Get an index of definitions for each variable name and definition from the ClusterClass variable.
	defIndex := newDefinitionsIndex(definitions)

	// Required variables definitions must exist as values on the Cluster.
	if validateRequired {
		allErrs = append(allErrs, validateRequiredVariables(valuesMap, defIndex, fldPath)...)
	}

	for _, value := range values {
		// Values must have an associated definition and must have a non-empty definitionFrom if there are conflicting definitions.
		definition, err := defIndex.get(value.Name, value.DefinitionFrom)
		if err != nil {
			allErrs = append(allErrs, field.Required(fldPath, err.Error())) // TODO: consider if to add ClusterClass name
			continue
		}

		// If there is an old variable matching this name and definition from defined in the old Cluster then pass this in
		// to cluster variable validation.
		var oldValue clusterv1.ClusterVariable
		oldValuesForName, found := oldValuesMap[value.Name]
		if found {
			oldValue = oldValuesForName[value.DefinitionFrom]
		}

		// Values must be valid according to the schema in their definition.
		allErrs = append(allErrs, ValidateClusterVariable(
			ctx,
			value.DeepCopy(),
			&oldValue,
			&clusterv1.ClusterClassVariable{
				Name:     value.Name,
				Required: definition.Required,
				Schema:   definition.Schema,
			}, fldPath)...)
	}

	return allErrs
}

// validateRequiredVariables validates all required variables from the ClusterClass exist in the Cluster.
func validateRequiredVariables(values map[string]map[string]clusterv1.ClusterVariable, definitions definitionsIndex, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for name, definitionsForName := range definitions {
		for _, def := range definitionsForName {
			// Check the required value for the specific variable definition. If the variable is not required continue.
			if !def.Required {
				continue
			}

			// If there is no variable with this name defined in the Cluster add an error and continue.
			valuesForName, found := values[name]
			if !found {
				allErrs = append(allErrs, field.Required(fldPath,
					fmt.Sprintf("required variable with name %q must be defined", name))) // TODO: consider if to use "Clusters with ClusterClass %q must have a variable with name %q"
				continue
			}

			// If there are no definition conflicts and the variable is set with an empty "DefinitionFrom" field return here.
			// This is a valid way for users to define a required value for variables across all variable definitions.
			if _, ok := valuesForName[emptyDefinitionFrom]; ok && !def.Conflicts {
				continue
			}

			// If the variable is not set for the specific definitionFrom add an error.
			if _, ok := valuesForName[def.From]; !ok {
				allErrs = append(allErrs, field.Required(fldPath,
					fmt.Sprintf("required variable with name %q from %q must be defined", name, def.From))) // TODO: consider if to use "Clusters with ClusterClass %q must have a variable with name %q"
			}
		}
	}
	return allErrs
}

// ValidateClusterVariable validates a clusterVariable.
func ValidateClusterVariable(ctx context.Context, value, oldValue *clusterv1.ClusterVariable, definition *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	// Parse JSON value.
	var variableValue interface{}

	// Only try to unmarshal the clusterVariable if it is not nil, otherwise the variableValue is nil.
	// Note: A clusterVariable with a nil value is the result of setting the variable value to "null" via YAML.
	if value.Value.Raw != nil {
		if err := json.Unmarshal(value.Value.Raw, &variableValue); err != nil {
			return field.ErrorList{field.Invalid(fldPath.Child("value"), string(value.Value.Raw),
				fmt.Sprintf("variable %q could not be parsed: %v", value.Name, err))}
		}
	}

	// Convert schema to Kubernetes APIExtensions Schema.
	apiExtensionsSchema, allErrs := convertToAPIExtensionsJSONSchemaProps(&definition.Schema.OpenAPIV3Schema, field.NewPath("schema"))
	if len(allErrs) > 0 {
		return field.ErrorList{field.InternalError(fldPath,
			fmt.Errorf("failed to convert schema definition for variable %q; ClusterClass should be checked: %v", definition.Name, allErrs))} // TODO: consider if to add ClusterClass name
	}

	// Create validator for schema.
	validator, _, err := validation.NewSchemaValidator(apiExtensionsSchema)
	if err != nil {
		return field.ErrorList{field.InternalError(fldPath,
			fmt.Errorf("failed to create schema validator for variable %q; ClusterClass should be checked: %v", value.Name, err))} // TODO: consider if to add ClusterClass name
	}

	// Validate variable against the schema.
	// NOTE: We're reusing a library func used in CRD validation.
	if err := validation.ValidateCustomResource(fldPath, variableValue, validator); err != nil {
		return err
	}

	// Structural schema pruning does not work with scalar values,
	// so we wrap the schema and the variable in objects.
	// <variable-name>: <variable-value>
	wrappedVariable := map[string]interface{}{
		value.Name: variableValue,
	}
	// type: object
	// properties:
	//   <variable-name>: <variable-schema>
	wrappedSchema := &apiextensions.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensions.JSONSchemaProps{
			definition.Name: *apiExtensionsSchema,
		},
	}
	ss, err := structuralschema.NewStructural(wrappedSchema)
	if err != nil {
		return field.ErrorList{
			field.InternalError(fldPath,
				fmt.Errorf("failed to create structural schema for variable %q; ClusterClass should be checked: %v", value.Name, err))} // TODO: consider if to add ClusterClass name
	}

	if err := validateUnknownFields(fldPath, definition.Name, wrappedVariable, ss); err != nil {
		return err
	}

	celValidator := cel.NewValidator(ss, false, celconfig.PerCallLimit)
	// celValidation will be nil if there are no CEL validations specified in the schema
	// under `x-kubernetes-validations`.
	if celValidator == nil {
		return nil
	}

	// Only extract old variable value if there are CEL validations and if the old variable is not nil.
	var oldWrappedVariable map[string]interface{}

	if oldValue != nil && oldValue.Value.Raw != nil {
		var oldVariableValue interface{}

		if err := json.Unmarshal(oldValue.Value.Raw, &oldVariableValue); err != nil {
			return field.ErrorList{field.Invalid(fldPath.Child("value"), string(oldValue.Value.Raw),
				fmt.Sprintf("old value of variable %q could not be parsed: %v", value.Name, err))}
		}

		oldWrappedVariable = map[string]interface{}{
			definition.Name: oldVariableValue,
		}
	}

	if err, _ := celValidator.Validate(ctx, fldPath, ss, wrappedVariable, oldWrappedVariable, celconfig.RuntimeCELCostBudget); err != nil {
		return err
	}

	return nil
}

// validateUnknownFields validates the given variableValue for unknown fields.
// This func returns an error if there are variable fields in variableValue that are not defined in
// variableSchema and if x-kubernetes-preserve-unknown-fields is not set.
func validateUnknownFields(fldPath *field.Path, variableName string, wrappedVariable map[string]interface{}, ss *structuralschema.Structural) field.ErrorList {
	// Run Prune to check if it would drop any unknown fields.
	opts := structuralschema.UnknownFieldPathOptions{
		// TrackUnknownFieldPaths has to be true so PruneWithOptions returns the unknown fields.
		TrackUnknownFieldPaths: true,
	}
	prunedUnknownFields := structuralpruning.PruneWithOptions(wrappedVariable, ss, false, opts)
	if len(prunedUnknownFields) > 0 {
		// If prune dropped any unknown fields, return an error.
		// This means that not all variable fields have been defined in the variable schema and
		// x-kubernetes-preserve-unknown-fields was not set.
		return field.ErrorList{
			field.Invalid(fldPath, "",
				fmt.Sprintf("failed validation: %q fields are not specified in the variable schema of variable %q", strings.Join(prunedUnknownFields, ","), variableName)),
		}
	}

	return nil
}
