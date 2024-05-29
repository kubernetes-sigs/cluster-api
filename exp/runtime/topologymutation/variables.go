/*
Copyright 2022 The Kubernetes Authors.

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

package topologymutation

import (
	"encoding/json"
	"strconv"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	patchvariables "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
)

// TODO: Add func for validating received variables are of the expected types.

// GetVariable get the variable value.
func GetVariable(templateVariables map[string]apiextensionsv1.JSON, variableName string) (*apiextensionsv1.JSON, error) {
	value, err := patchvariables.GetVariableValue(templateVariables, variableName)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// GetStringVariable get the variable value as a string.
func GetStringVariable(templateVariables map[string]apiextensionsv1.JSON, variableName string) (string, error) {
	value, err := GetVariable(templateVariables, variableName)
	if err != nil {
		return "", err
	}

	// Unquote the JSON string.
	stringValue, err := strconv.Unquote(string(value.Raw))
	if err != nil {
		return "", err
	}
	return stringValue, nil
}

// GetBoolVariable get the value as a bool.
func GetBoolVariable(templateVariables map[string]apiextensionsv1.JSON, variableName string) (bool, error) {
	value, err := GetVariable(templateVariables, variableName)
	if err != nil {
		return false, err
	}

	// Parse the JSON bool.
	boolValue, err := strconv.ParseBool(string(value.Raw))
	if err != nil {
		return false, err
	}
	return boolValue, nil
}

// GetObjectVariableInto gets variable's string value then unmarshal it into
// object passed from 'into'.
func GetObjectVariableInto(templateVariables map[string]apiextensionsv1.JSON, variableName string, into interface{}) error {
	value, err := GetVariable(templateVariables, variableName)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(value.Raw, into); err != nil {
		return errors.Wrapf(err, "failed to unmarshal variable json %q into %q", string(value.Raw), into)
	}

	return nil
}

// ToMap converts a list of Variables to a map of apiextensionsv1.JSON (name is the map key).
// This is usually used to convert the Variables in a GeneratePatchesRequestItem into a format
// that is used by MergeVariableMaps.
func ToMap(variables []runtimehooksv1.Variable) map[string]apiextensionsv1.JSON {
	variablesMap := map[string]apiextensionsv1.JSON{}
	for i := range variables {
		variablesMap[variables[i].Name] = variables[i].Value
	}
	return variablesMap
}

// MergeVariableMaps merges variables.
// This func is useful when merging global and template-specific variables.
// NOTE: In case a variable exists in multiple maps, the variable from the latter map is preserved.
// NOTE: The builtin variable object is merged instead of simply overwritten.
func MergeVariableMaps(variableMaps ...map[string]apiextensionsv1.JSON) (map[string]apiextensionsv1.JSON, error) {
	res := make(map[string]apiextensionsv1.JSON)

	for _, variableMap := range variableMaps {
		for variableName, variableValue := range variableMap {
			// If the variable already exists and is the builtin variable, merge it.
			if _, ok := res[variableName]; ok && variableName == runtimehooksv1.BuiltinsName {
				mergedV, err := mergeBuiltinVariables(res[variableName], variableValue)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to merge builtin variables")
				}
				res[variableName] = *mergedV
				continue
			}
			res[variableName] = variableValue
		}
	}

	return res, nil
}

// mergeBuiltinVariables merges builtin variable objects.
// NOTE: In case a variable exists in multiple builtin variables, the variable from the latter map is preserved.
func mergeBuiltinVariables(variableList ...apiextensionsv1.JSON) (*apiextensionsv1.JSON, error) {
	builtins := &runtimehooksv1.Builtins{}

	// Unmarshal all variables into builtins.
	// NOTE: This accumulates the fields on the builtins.
	// Fields will be overwritten by later Unmarshals if fields are
	// set on multiple variables.
	for _, variable := range variableList {
		if err := json.Unmarshal(variable.Raw, builtins); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal builtin variable")
		}
	}

	// Marshal builtins to JSON.
	builtinVariableJSON, err := json.Marshal(builtins)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal builtin variable")
	}

	return &apiextensionsv1.JSON{
		Raw: builtinVariableJSON,
	}, nil
}
