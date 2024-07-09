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

// DefaultMachineVariables defaults MachineDeploymentVariables and MachinePoolVariables.
func DefaultMachineVariables(values []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, fldPath *field.Path) ([]clusterv1.ClusterVariable, field.ErrorList) {
	return defaultClusterVariables(values, definitions, false, fldPath)
}

// defaultClusterVariables defaults variables.
// If they do not exist yet, they are created if createVariables is set.
func defaultClusterVariables(values []clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable, createVariables bool, fldPath *field.Path) ([]clusterv1.ClusterVariable, field.ErrorList) {
	var allErrs field.ErrorList

	// Get an index for variable values.
	valuesIndex, err := newValuesIndex(fldPath, values)
	if len(err) > 0 {
		return nil, append(allErrs, err...)
	}

	// Get an index for definitions.
	defIndex, err := newDefinitionsIndex(fldPath, definitions)
	if len(err) > 0 {
		return nil, append(allErrs, err...)
	}

	// Get a deterministically ordered list of all variables set in the Cluster and defined the ClusterClass.
	// Note: If the order is not deterministic variables would be continuously rewritten to the Cluster.
	allVariables := getAllVariables(values, valuesIndex, definitions)

	// Default all variables.
	defaultedValues := []clusterv1.ClusterVariable{}
	for _, variable := range allVariables {
		// Add variable name as key, this makes it easier to read the field path.
		fldPath := fldPath.Key(variable.Name)

		// Get the variable definition from the ClusterClass. If the variable is not defined add an error.
		definition, ok := defIndex[variable.Name]
		if !ok {
			allErrs = append(allErrs, field.Invalid(fldPath, string(variable.Value.Raw), "variable is not defined"))
			continue
		}

		// Get the current value of the variable if it is defined in the Cluster spec (nil otherwise).
		currentValue := valuesIndex[variable.Name]

		// Default the variable.
		defaultedValue, errs := defaultValue(currentValue, definition, fldPath, createVariables)
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
			continue
		}

		// Continue if there is no defaulted variable.
		// NOTE: This happens when the variable doesn't exist on the Cluster before and
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

// defaultValue defaults a clusterVariable based on the default value in the clusterClassVariable.
func defaultValue(currentValue *clusterv1.ClusterVariable, definition *clusterv1.ClusterClassStatusVariable, fldPath *field.Path, createVariable bool) (*clusterv1.ClusterVariable, field.ErrorList) {
	// Note: We already validated in newDefinitionsIndex that Definitions is not empty
	// and we don't have conflicts, so we can just pick the first one
	def := definition.Definitions[0]

	if currentValue == nil {
		// Return if the variable does not exist yet and createVariable is false.
		if !createVariable {
			return nil, nil
		}
		// Return if the variable does not exist yet and there is no top-level default value.

		if def.Schema.OpenAPIV3Schema.Default == nil {
			return nil, nil
		}
	}

	// Convert schema to Kubernetes APIExtensions schema.
	apiExtensionsSchema, errs := convertToAPIExtensionsJSONSchemaProps(&def.Schema.OpenAPIV3Schema, field.NewPath("schema"))
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
	}

	return v, nil
}

// getAllVariables returns a deterministically ordered list of all variables set in the Cluster and defined the ClusterClass.
// Ordered means that the list will first contain the existing variable values from the Cluster and
// then we will add variables from the ClusterClass (only if they don't exist already on the Cluster).
// This way if we run repeatedly through variable defaulting the order of the variables in Cluster.spec.topology doesn't change.
// If the order would change, we would be continuously writing the Cluster object (this code is also executed in the Cluster topology controller).
func getAllVariables(values []clusterv1.ClusterVariable, valuesIndex map[string]*clusterv1.ClusterVariable, definitions []clusterv1.ClusterClassStatusVariable) []clusterv1.ClusterVariable {
	// allVariables is used to get a full correctly ordered list of variables.
	allVariables := []clusterv1.ClusterVariable{}

	// Add any values that already exist.
	allVariables = append(allVariables, values...)

	// Add variables from the ClusterClass, which currently don't exist on the Cluster.
	for _, variable := range definitions {
		// If there is a value in the Cluster already this variable does not need to be defaulted.
		if _, ok := valuesIndex[variable.Name]; ok {
			continue
		}

		// Add the variable to the list.
		allVariables = append(allVariables, clusterv1.ClusterVariable{
			Name: variable.Name,
		})
	}
	return allVariables
}
