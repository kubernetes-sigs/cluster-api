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

// DefaultClusterVariables defaults variables which do not exist in clusterVariable, if they
// have a default value in the corresponding schema in clusterClassVariable.
func DefaultClusterVariables(clusterVariables []clusterv1.ClusterVariable, clusterClassVariables []clusterv1.ClusterClassVariable, fldPath *field.Path) ([]clusterv1.ClusterVariable, field.ErrorList) {
	var allErrs field.ErrorList

	// Build maps for easier and faster access.
	clusterVariablesMap := getClusterVariablesMap(clusterVariables)
	clusterClassVariablesMap := getClusterClassVariablesMap(clusterClassVariables)

	// allVariables is used to get a full correctly ordered list of variables.
	allVariables := []string{}
	// Add any ClusterVariables that already exist.
	for _, variable := range clusterVariables {
		allVariables = append(allVariables, variable.Name)
	}
	// Add variables from the ClusterClass, which currently don't exist on the Cluster.
	for _, variable := range clusterClassVariables {
		// Continue if the ClusterClass variable already exists.
		if _, ok := clusterVariablesMap[variable.Name]; ok {
			continue
		}

		allVariables = append(allVariables, variable.Name)
	}

	// Default all variables.
	defaultedClusterVariables := []clusterv1.ClusterVariable{}
	for i, variableName := range allVariables {
		clusterClassVariable := clusterClassVariablesMap[variableName]
		clusterVariable := clusterVariablesMap[variableName]

		defaultedClusterVariable, errs := defaultClusterVariable(clusterVariable, clusterClassVariable, fldPath.Index(i))
		if len(errs) > 0 {
			allErrs = append(allErrs, errs...)
			continue
		}

		// Continue if there is no defaulted variable.
		// NOTE: This happens when the variable doesn't exist on the CLuster before and
		// there is no top-level default value.
		if defaultedClusterVariable == nil {
			continue
		}

		defaultedClusterVariables = append(defaultedClusterVariables, *defaultedClusterVariable)
	}

	if len(allErrs) > 0 {
		return nil, allErrs
	}

	return defaultedClusterVariables, nil
}

// defaultClusterVariable defaults a clusterVariable based on the default value in the clusterClassVariable.
func defaultClusterVariable(clusterVariable *clusterv1.ClusterVariable, clusterClassVariable *clusterv1.ClusterClassVariable, fldPath *field.Path) (*clusterv1.ClusterVariable, field.ErrorList) {
	// Return if the variable does not exist yet and there is no top-level default value.
	if clusterVariable == nil && clusterClassVariable.Schema.OpenAPIV3Schema.Default == nil {
		return nil, nil
	}

	// Convert schema to Kubernetes APIExtensions schema.
	apiExtensionsSchema, err := convertToAPIExtensionsJSONSchemaProps(&clusterClassVariable.Schema.OpenAPIV3Schema)
	if err != nil {
		return nil, field.ErrorList{field.Invalid(fldPath, "",
			fmt.Sprintf("invalid schema in ClusterClass for variable %q: error to convert schema %v", clusterVariable.Name, err))}
	}

	var value interface{}
	// If the variable already exists, parse the current value.
	if clusterVariable != nil && len(clusterVariable.Value.Raw) > 0 {
		if err := json.Unmarshal(clusterVariable.Value.Raw, &value); err != nil {
			return nil, field.ErrorList{field.Invalid(fldPath, "",
				fmt.Sprintf("failed to unmarshal variable value %q: %v", string(clusterVariable.Value.Raw), err))}
		}
	}

	// Structural schema defaulting does not work with scalar values,
	// so we wrap the schema and the variable in objects.
	// <variable-name>: <variable-value>
	wrappedVariable := map[string]interface{}{
		clusterClassVariable.Name: value,
	}
	// type: object
	// properties:
	//   <variable-name>: <variable-schema>
	wrappedSchema := &apiextensions.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensions.JSONSchemaProps{
			clusterClassVariable.Name: *apiExtensionsSchema,
		},
	}

	// Default the variable via the structural schema library.
	ss, err := structuralschema.NewStructural(wrappedSchema)
	if err != nil {
		return nil, field.ErrorList{field.Invalid(fldPath, "",
			fmt.Sprintf("failed defaulting variable %q: %v", clusterVariable.Name, err))}
	}
	structuraldefaulting.Default(wrappedVariable, ss)

	// Marshal the defaulted value.
	defaultedVariableValue, err := json.Marshal(wrappedVariable[clusterClassVariable.Name])
	if err != nil {
		return nil, field.ErrorList{field.Invalid(fldPath, "",
			fmt.Sprintf("failed to marshal default value of variable %q: %v", clusterClassVariable.Name, err))}
	}
	return &clusterv1.ClusterVariable{
		Name: clusterClassVariable.Name,
		Value: apiextensionsv1.JSON{
			Raw: defaultedVariableValue,
		},
	}, nil
}
