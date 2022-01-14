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
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// ValidateClusterVariables validates ClusterVariables.
func ValidateClusterVariables(clusterVariables []clusterv1.ClusterVariable, clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	return validateClusterVariables(clusterVariables, clusterClassVariables, true, fldPath)
}

// ValidateMachineDeploymentVariables validates ValidateMachineDeploymentVariables.
func ValidateMachineDeploymentVariables(clusterVariables []clusterv1.ClusterVariable, clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	return validateClusterVariables(clusterVariables, clusterClassVariables, false, fldPath)
}

// validateClusterVariables validates variables via the schemas in the corresponding clusterClassVariable.
func validateClusterVariables(clusterVariables []clusterv1.ClusterVariable, clusterClassVariables []clusterv1.ClusterClassVariable, validateRequired bool, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Build maps for easier and faster access.
	clusterVariablesMap := getClusterVariablesMap(clusterVariables)
	clusterClassVariablesMap := getClusterClassVariablesMap(clusterClassVariables)

	// Validate:
	// * required variables from the ClusterClass exist on the Cluster.
	// * all variables in the Cluster are defined in the ClusterClass.
	if validateRequired {
		allErrs = append(allErrs, validateRequiredClusterVariables(clusterVariablesMap, clusterClassVariablesMap, fldPath)...)
	}
	allErrs = append(allErrs, validateClusterVariablesDefined(clusterVariables, clusterClassVariablesMap, fldPath)...)
	if len(allErrs) > 0 {
		return allErrs
	}

	// Validate all variables from the Cluster.
	for i := range clusterVariables {
		clusterVariable := clusterVariables[i]

		// Get schema.
		// Nb. We already validated above in validateClusterVariablesDefined that there is a
		// corresponding ClusterClass variable, so we don't have to check it again.
		clusterClassVariable := clusterClassVariablesMap[clusterVariable.Name]

		allErrs = append(allErrs, ValidateClusterVariable(&clusterVariable, clusterClassVariable, fldPath.Index(i))...)
	}
	return allErrs
}

// validateRequiredClusterVariables validates all required variables from the ClusterClass exist in the Cluster.
func validateRequiredClusterVariables(clusterVariables map[string]*clusterv1.ClusterVariable, clusterClassVariables map[string]*clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for variableName, clusterClassVariable := range clusterClassVariables {
		if !clusterClassVariable.Required {
			continue
		}

		if _, ok := clusterVariables[variableName]; !ok {
			allErrs = append(allErrs, field.Required(fldPath,
				fmt.Sprintf("required variable with name %q must be defined", variableName))) // TODO: consider if to use "Clusters with ClusterClass %q must have a variable with name %q"
		}
	}

	return allErrs
}

// validateClusterVariablesDefined validates all variables from the Cluster are defined in the ClusterClass.
func validateClusterVariablesDefined(clusterVariables []clusterv1.ClusterVariable, clusterClassVariables map[string]*clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for i, clusterVariable := range clusterVariables {
		if _, ok := clusterClassVariables[clusterVariable.Name]; !ok {
			return field.ErrorList{field.Invalid(fldPath.Index(i).Child("name"), clusterVariable.Name,
				fmt.Sprintf("variable with name %q is not defined in the ClusterClass", clusterVariable.Name))} // TODO: consider if to add ClusterClass name
		}
	}

	return allErrs
}

// ValidateTopLevelClusterVariablesExist validates that all overrides have a corresponding top-level variable.
func ValidateTopLevelClusterVariablesExist(clusterVariablesOverrides []clusterv1.ClusterVariable, clusterVariables []clusterv1.ClusterVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Build map for easier and faster access.
	clusterVariablesMap := getClusterVariablesMap(clusterVariables)

	for i, clusterVariableOverride := range clusterVariablesOverrides {
		if _, ok := clusterVariablesMap[clusterVariableOverride.Name]; !ok {
			return field.ErrorList{field.Invalid(fldPath.Index(i).Child("name"), clusterVariableOverride.Name,
				fmt.Sprintf("variable override with name %q is missing a corresponding top-level variable", clusterVariableOverride.Name))}
		}
	}

	return allErrs
}

// ValidateClusterVariable validates a clusterVariable.
func ValidateClusterVariable(clusterVariable *clusterv1.ClusterVariable, clusterClassVariable *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	// Parse JSON value.
	var variableValue interface{}
	// Only try to unmarshal the clusterVariable if it is not nil, otherwise the variableValue is nil.
	// Note: A clusterVariable with a nil value is the result of setting the variable value to "null" via YAML.
	if clusterVariable.Value.Raw != nil {
		if err := json.Unmarshal(clusterVariable.Value.Raw, &variableValue); err != nil {
			return field.ErrorList{field.Invalid(fldPath.Child("value"), string(clusterVariable.Value.Raw),
				fmt.Sprintf("variable %q could not be parsed: %v", clusterVariable.Name, err))}
		}
	}

	// Convert schema to Kubernetes APIExtensions Schema.
	apiExtensionsSchema, allErrs := convertToAPIExtensionsJSONSchemaProps(&clusterClassVariable.Schema.OpenAPIV3Schema, field.NewPath("schema"))
	if len(allErrs) > 0 {
		return field.ErrorList{field.InternalError(fldPath,
			fmt.Errorf("failed to convert schema definition for variable %q; ClusterClass should be checked: %v", clusterClassVariable.Name, allErrs))} // TODO: consider if to add ClusterClass name
	}

	// Create validator for schema.
	validator, _, err := validation.NewSchemaValidator(&apiextensions.CustomResourceValidation{
		OpenAPIV3Schema: apiExtensionsSchema,
	})
	if err != nil {
		return field.ErrorList{field.InternalError(fldPath,
			fmt.Errorf("failed to create schema validator for variable %q; ClusterClass should be checked: %v", clusterVariable.Name, err))} // TODO: consider if to add ClusterClass name
	}

	// Validate variable against the schema.
	// NOTE: We're reusing a library func used in CRD validation.
	return validation.ValidateCustomResource(fldPath, variableValue, validator)
}

func getClusterVariablesMap(clusterVariables []clusterv1.ClusterVariable) map[string]*clusterv1.ClusterVariable {
	variablesMap := map[string]*clusterv1.ClusterVariable{}
	for i := range clusterVariables {
		variablesMap[clusterVariables[i].Name] = &clusterVariables[i]
	}
	return variablesMap
}

func getClusterClassVariablesMap(clusterClassVariables []clusterv1.ClusterClassVariable) map[string]*clusterv1.ClusterClassVariable {
	variablesMap := map[string]*clusterv1.ClusterClassVariable{}
	for i := range clusterClassVariables {
		variablesMap[clusterClassVariables[i].Name] = &clusterClassVariables[i]
	}
	return variablesMap
}
