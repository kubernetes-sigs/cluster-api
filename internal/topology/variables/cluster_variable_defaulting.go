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
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	structuraldefaulting "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// DefaultClusterVariables defaults ClusterVariables.
func DefaultClusterVariables(values []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, fldPath *field.Path) ([]clusterv1.ClusterVariable, field.ErrorList) {
	return defaultClusterVariables(values, definitions, true, fldPath)
}

// DefaultMachineDeploymentVariables defaults MachineDeploymentVariables.
func DefaultMachineDeploymentVariables(values []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, fldPath *field.Path) ([]clusterv1.ClusterVariable, field.ErrorList) {
	return defaultClusterVariables(values, definitions, false, fldPath)
}

// defaultClusterVariables defaults variables.
// If they do not exist yet, they are created if createVariables is set.
func defaultClusterVariables(values []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, createVariables bool, fldPath *field.Path) ([]clusterv1.ClusterVariable, field.ErrorList) {
	var allErrs field.ErrorList

	// Get a map of ClusterVariable values. This function validates that:
	// - variables are not defined more than once in Cluster spec.
	// - variables with the same name do not have a mix of empty and non-empty DefinitionFrom.
	valuesIndex, err := newValuesIndex(values)
	if err != nil {
		return nil, append(allErrs, field.Invalid(fldPath, values,
			fmt.Sprintf("cluster variables not valid: %s", err)))
	}

	// Get an index for each variable name and definition.
	defIndex := newDefinitionsIndex(definitions)

	// Get a deterministically ordered list of all variables defined in both the Cluster and the ClusterClass.
	// Note: If the order is not deterministic variables would be continuously rewritten to the Cluster.
	allVariables := getAllVariables(values, valuesIndex, definitions)

	// Default all variables.
	defaultedValues := []clusterv1.ClusterVariable{}
	for _, variable := range allVariables {
		// Get the variable definition from the ClusterClass. If the variable is not defined add an error.
		definition, err := defIndex.get(variable.Name, variable.DefinitionFrom)
		if err != nil {
			allErrs = append(allErrs, field.Required(fldPath, err.Error()))
			continue
		}

		// Get the current value of the variable if it is defined in the Cluster spec.
		currentValue := getCurrentValue(variable, valuesIndex)

		// Default the variable.
		defaultedValue, errs := defaultValue(currentValue, definition, fldPath, createVariables)
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
			continue
		}

		// Continue if there is no defaulted variable.
		// NOTE: This happens when the variable doesn't exist on the CLuster before and
		// there is no top-level default value.
		if defaultedValue == nil {
			continue
		}
		defaultedValues = append(defaultedValues, *defaultedValue)
	}

	if len(allErrs) > 0 {
		return nil, allErrs
	}
	return defaultedValues, nil
}

// getCurrentValue returns the value of a variable for its definitionFrom, or for an empty definitionFrom if it exists.
func getCurrentValue(variable clusterv1.ClusterVariable, valuesMap map[string]map[string]clusterv1.ClusterVariable) *clusterv1.ClusterVariable {
	// If the value is set in the Cluster spec get the value.
	if valuesForName, ok := valuesMap[variable.Name]; ok {
		if value, ok := valuesForName[variable.DefinitionFrom]; ok {
			return &value
		}
	}
	return nil
}

// defaultValue defaults a clusterVariable based on the default value in the clusterClassVariable.
func defaultValue(currentValue *clusterv1.ClusterVariable, definition *statusVariableDefinition, fldPath *field.Path, createVariable bool) (*clusterv1.ClusterVariable, field.ErrorList) {
	if currentValue == nil {
		// Return if the variable does not exist yet and createVariable is false.
		if !createVariable {
			return nil, nil
		}
		// Return if the variable does not exist yet and there is no top-level default value.
		if definition.Schema.OpenAPIV3Schema.Default == nil {
			return nil, nil
		}
	}

	// Convert schema to Kubernetes APIExtensions schema.
	apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(&definition.Schema.OpenAPIV3Schema, field.NewPath("schema"))
	if len(errs) > 0 {
		return nil, field.ErrorList{field.Invalid(fldPath, "",
			fmt.Sprintf("invalid schema in ClusterClass for variable %q: error to convert schema %v", definition.Name, errs))}
	}

	var value interface{}
	// If the variable already exists, parse the current value.
	if currentValue != nil && len(currentValue.Value.Raw) > 0 {
		if err := json.Unmarshal(currentValue.Value.Raw, &value); err != nil {
			return nil, field.ErrorList{field.Invalid(fldPath, "",
				fmt.Sprintf("failed to unmarshal variable %q value %q: %v", currentValue.Name, string(currentValue.Value.Raw), err))}
		}
	}

	// Structural schema defaulting does not work with scalar values,
	// so we wrap the schema and the variable in objects.
	// <variable-name>: <variable-value>
	wrappedVariable := map[string]interface{}{
		definition.Name: value,
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

	// Default the variable via the structural schema library.
	ss, err := structuralschema.NewStructural(wrappedSchema)
	if err != nil {
		return nil, field.ErrorList{field.Invalid(fldPath, "",
			fmt.Sprintf("failed defaulting variable %q: %v", currentValue.Name, err))}
	}
	structuraldefaulting.Default(wrappedVariable, ss)

	// Marshal the defaulted value.
	defaultedVariableValue, err := json.Marshal(wrappedVariable[definition.Name])
	if err != nil {
		return nil, field.ErrorList{field.Invalid(fldPath, "",
			fmt.Sprintf("failed to marshal default value of variable %q: %v", definition.Name, err))}
	}
	v := &clusterv1.ClusterVariable{
		Name: definition.Name,
		Value: apiextensionsv1.JSON{
			Raw: defaultedVariableValue,
		},
		DefinitionFrom: definition.From,
	}

	return v, nil
}

// getAllVariables returns a correctly ordered list of all variables defined in the ClusterClass and the Cluster.
func getAllVariables(values []clusterv1.ClusterVariable, valuesIndex map[string]map[string]clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable) []clusterv1.ClusterVariable {
	// allVariables is used to get a full correctly ordered list of variables.
	allVariables := []clusterv1.ClusterVariable{}
	uniqueVariableDefinitions := map[string]bool{}

	// Add any values that already exist.
	allVariables = append(allVariables, values...)

	// Add variables from the ClusterClass, which currently don't exist on the Cluster.
	for _, variable := range definitions {
		for _, definition := range variable.Definitions {
			definitionFrom := definition.From

			// 1) If there is a value in the Cluster with this definitionFrom or with an empty definitionFrom this variable does not need to be defaulted.
			if _, ok := valuesIndex[variable.Name]; ok {
				if _, ok := valuesIndex[variable.Name][definitionFrom]; ok {
					continue
				}
				if _, ok := valuesIndex[variable.Name][emptyDefinitionFrom]; ok {
					continue
				}
			}

			// 2) If the definition has no conflicts and no variable of the same name is defined in the Cluster set the definitionFrom to emptyDefinitionFrom.
			if !variable.DefinitionsConflict && len(valuesIndex[variable.Name]) == 0 {
				definitionFrom = emptyDefinitionFrom
			}

			// 3) If a variable with this name and definition has been added already, continue.
			// This prevents adding the same variable multiple times where the variable is defaulted with an emptyDefinitionFrom.
			if _, ok := uniqueVariableDefinitions[definitionFrom+variable.Name]; ok {
				continue
			}

			// Otherwise add the variable to the list.
			allVariables = append(allVariables, clusterv1.ClusterVariable{
				Name:           variable.Name,
				DefinitionFrom: definitionFrom,
			})
			uniqueVariableDefinitions[definitionFrom+variable.Name] = true
		}
	}
	return allVariables
}
