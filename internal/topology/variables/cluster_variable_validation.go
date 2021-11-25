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

// ValidateClusterVariables validates clusterVariable via the schemas in the corresponding clusterClassVariable.
func ValidateClusterVariables(clusterVariables []clusterv1.ClusterVariable, clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Build maps for easier and faster access.
	clusterVariablesMap := getClusterVariablesMap(clusterVariables)
	clusterClassVariablesMap := getClusterClassVariablesMap(clusterClassVariables)

	// Validate:
	// * required variables from the ClusterClass exist on the Cluster.
	// * all variables in the Cluster are defined in the ClusterClass.
	allErrs = append(allErrs, validateRequiredClusterVariables(clusterVariablesMap, clusterClassVariablesMap, fldPath)...)
	allErrs = append(allErrs, validateClusterVariablesDefined(clusterVariablesMap, clusterClassVariablesMap, fldPath)...)
	if len(allErrs) > 0 {
		return allErrs
	}

	// Validate all variables from the Cluster.
	for variableName, clusterVariable := range clusterVariablesMap {
		// Get schema.
		// Nb. We already validated above in validateClusterVariablesDefined that there is a
		// corresponding ClusterClass variable, so we don't have to check it again.
		clusterClassVariable := clusterClassVariablesMap[variableName]

		allErrs = append(allErrs, validateClusterVariable(clusterVariable, clusterClassVariable, fldPath)...)
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
				fmt.Sprintf("required variable %q does not exist", variableName)))
		}
	}

	return allErrs
}

// validateClusterVariablesDefined validates all variables from the Cluster are defined in the ClusterClass.
func validateClusterVariablesDefined(clusterVariables map[string]*clusterv1.ClusterVariable, clusterClassVariables map[string]*clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	for variableName, clusterVariable := range clusterVariables {
		if _, ok := clusterClassVariables[variableName]; !ok {
			return field.ErrorList{field.Invalid(fldPath, string(clusterVariable.Value.Raw),
				fmt.Sprintf("variable %q is not defined in the ClusterClass", variableName))}
		}
	}

	return allErrs
}

// validateClusterVariable validates a clusterVariable.
func validateClusterVariable(clusterVariable *clusterv1.ClusterVariable, clusterClassVariable *clusterv1.ClusterClassVariable, fldPath *field.Path) field.ErrorList {
	// Parse JSON value.
	var variableValue interface{}
	if err := json.Unmarshal(clusterVariable.Value.Raw, &variableValue); err != nil {
		return field.ErrorList{field.Invalid(fldPath, string(clusterVariable.Value.Raw),
			fmt.Sprintf("variable %q could not be parsed: %v", clusterVariable.Name, err))}
	}

	// Convert schema to Kubernetes APIExtensions Schema.
	apiExtensionsSchema, err := convertToAPIExtensionsJSONSchemaProps(&clusterClassVariable.Schema.OpenAPIV3Schema)
	if err != nil {
		return field.ErrorList{field.Invalid(fldPath, string(clusterVariable.Value.Raw),
			fmt.Sprintf("invalid schema in ClusterClass for variable %q: failed to convert schema %v", clusterVariable.Name, err))}
	}

	// Create validator for schema.
	validator, _, err := validation.NewSchemaValidator(&apiextensions.CustomResourceValidation{
		OpenAPIV3Schema: apiExtensionsSchema,
	})
	if err != nil {
		return field.ErrorList{field.Invalid(fldPath, string(clusterVariable.Value.Raw),
			fmt.Sprintf("invalid schema in ClusterClass for variable %q: %v", clusterVariable.Name, err))}
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
