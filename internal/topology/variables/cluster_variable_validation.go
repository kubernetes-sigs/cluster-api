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

	// Get an index for variable values.
	valuesMap, err := newValuesIndex(fldPath, values)
	if len(err) > 0 {
		return append(allErrs, err...)
	}

	// Get an index for old variable values.
	// Note: We know they are all valid and not duplicate names, etc. as previous
	// validation has already asserted that.
	oldValuesMap, err := newValuesIndex(fldPath, oldValues)
	if len(err) > 0 {
		return append(allErrs, err...)
	}

	// Get an index for definitions.
	defIndex, err := newDefinitionsIndex(fldPath, definitions)
	if len(err) > 0 {
		return append(allErrs, err...)
	}

	// Required variables definitions must exist as values on the Cluster.
	if validateRequired {
		allErrs = append(allErrs, validateRequiredVariables(valuesMap, defIndex, fldPath)...)
	}

	for _, value := range values {
		// Add variable name as key, this makes it easier to read the field path.
		fldPath := fldPath.Key(value.Name)

		// Get the variable definition from the ClusterClass. If the variable is not defined add an error.
		definition, ok := defIndex[value.Name]
		if !ok {
			allErrs = append(allErrs, field.Invalid(fldPath, string(value.Value.Raw), "variable is not defined"))
			continue
		}

		// Note: We already validated in newDefinitionsIndex that Definitions is not empty
		// and we don't have conflicts, so we can just pick the first one
		def := definition.Definitions[0]

		// If there is an old variable with the same name defined in the old Cluster then pass this in
		// to cluster variable validation.
		oldValue := oldValuesMap[value.Name]

		// Values must be valid according to the schema in their definition.
		allErrs = append(allErrs, ValidateClusterVariable(
			ctx,
			value.DeepCopy(),
			oldValue,
			&clusterv1.ClusterClassVariable{
				Name:     value.Name,
				Required: def.Required,
				Schema:   def.Schema,
			}, fldPath)...)
	}

	return allErrs
}

// validateRequiredVariables validates all required variables from the ClusterClass exist in the Cluster.
func validateRequiredVariables(values map[string]*clusterv1.ClusterVariable, definitions map[string]*clusterv1.ClusterClassStatusVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for name, def := range definitions {
		// Check the required value for the specific variable definition. If the variable is not required continue.
		if !def.Definitions[0].Required {
			continue
		}

		// If there is no variable with this name defined in the Cluster add an error and continue.
		if _, found := values[name]; !found {
			allErrs = append(allErrs, field.Required(fldPath,
				fmt.Sprintf("required variable %q must be set", name))) // TODO: consider if to use "Clusters with ClusterClass %q must have a variable with name %q"
			continue
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
	if validationErrors := validation.ValidateCustomResource(fldPath.Child("value"), variableValue, validator); len(validationErrors) > 0 {
		var allErrs field.ErrorList
		for _, validationError := range validationErrors {
			// Set correct value in the field error. ValidateCustomResource sets the type instead of the value.
			validationError.BadValue = string(value.Value.Raw)
			// Fixup detail message.
			validationError.Detail = strings.TrimPrefix(validationError.Detail, " in body ")
			allErrs = append(allErrs, validationError)
		}
		return allErrs
	}

	// Structural schema pruning does not work with scalar values,
	// so we wrap the schema and the variable in objects.
	// <variable-name>: <variable-value>
	wrappedVariable := map[string]interface{}{
		"variableValue": variableValue,
	}
	// type: object
	// properties:
	//   <variable-name>: <variable-schema>
	wrappedSchema := &apiextensions.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensions.JSONSchemaProps{
			"variableValue": *apiExtensionsSchema,
		},
	}
	ss, err := structuralschema.NewStructural(wrappedSchema)
	if err != nil {
		return field.ErrorList{
			field.InternalError(fldPath,
				fmt.Errorf("failed to create structural schema for variable %q; ClusterClass should be checked: %v", value.Name, err))} // TODO: consider if to add ClusterClass name
	}

	if err := validateUnknownFields(fldPath, value, wrappedVariable, ss); err != nil {
		return err
	}

	// Note: k/k CR validation also uses celconfig.PerCallLimit when creating the validator for a custom resource.
	// The current PerCallLimit gives roughly 0.1 second for each expression validation call.
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
			"variableValue": oldVariableValue,
		}
	}

	// Note: k/k CRD validation also uses celconfig.RuntimeCELCostBudget for the Validate call.
	// The current RuntimeCELCostBudget gives roughly 1 second for the validation of a variable value.
	if validationErrors, _ := celValidator.Validate(ctx, fldPath.Child("value"), ss, wrappedVariable, oldWrappedVariable, celconfig.RuntimeCELCostBudget); len(validationErrors) > 0 {
		var allErrs field.ErrorList
		for _, validationError := range validationErrors {
			// Set correct value in the field error. ValidateCustomResource sets the type instead of the value.
			validationError.BadValue = string(value.Value.Raw)
			// Drop "variableValue" from the path.
			validationError.Field = strings.Replace(validationError.Field, "value.variableValue", "value", 1)
			allErrs = append(allErrs, validationError)
		}
		return allErrs
	}

	return nil
}

// validateUnknownFields validates the given variableValue for unknown fields.
// This func returns an error if there are variable fields in variableValue that are not defined in
// variableSchema and if x-kubernetes-preserve-unknown-fields is not set.
func validateUnknownFields(fldPath *field.Path, clusterVariable *clusterv1.ClusterVariable, wrappedVariable map[string]interface{}, ss *structuralschema.Structural) field.ErrorList {
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
		for i := range prunedUnknownFields {
			// Drop "variableValue" from the path.
			prunedUnknownFields[i] = strings.TrimPrefix(prunedUnknownFields[i], "variableValue.")
		}

		return field.ErrorList{
			field.Invalid(fldPath, string(clusterVariable.Value.Raw),
				fmt.Sprintf("failed validation: %q field(s) are not specified in the variable schema of variable %q", strings.Join(prunedUnknownFields, ","), clusterVariable.Name)),
		}
	}

	return nil
}
