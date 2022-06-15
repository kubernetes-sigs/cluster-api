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

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// MergeVariableMaps merges variables.
// This func is useful when merging global and template-specific variables.
// NOTE: In case a variable exists in multiple maps, the variable from the latter map is preserved.
// NOTE: The builtin variable object is merged instead of simply overwritten.
func MergeVariableMaps(variableMaps ...map[string]apiextensionsv1.JSON) (map[string]apiextensionsv1.JSON, error) {
	res := make(map[string]apiextensionsv1.JSON)

	for _, variableMap := range variableMaps {
		for variableName, variableValue := range variableMap {
			// If the variable already exits and is the builtin variable, merge it.
			if _, ok := res[variableName]; ok && variableName == BuiltinsName {
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
	builtins := &Builtins{}

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
