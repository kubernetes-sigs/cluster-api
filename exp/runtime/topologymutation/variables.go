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
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	patchvariables "sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/patches/variables"
)

// TODO: Add func for validating received variables are of the expected types
// TODO: Add func for converting complex variables into go structs and/or other basic types.

// GetVariable get the variable value.
func GetVariable(templateVariables map[string]apiextensionsv1.JSON, variableName string) (*apiextensionsv1.JSON, bool, error) {
	value, err := patchvariables.GetVariableValue(templateVariables, variableName)
	if err != nil {
		if patchvariables.IsNotFoundError(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return value, true, nil
}

// GetStringVariable get the variable value as a string.
func GetStringVariable(templateVariables map[string]apiextensionsv1.JSON, variableName string) (string, bool, error) {
	value, found, err := GetVariable(templateVariables, variableName)
	if !found || err != nil {
		return "", found, err
	}

	// Unquote the JSON string.
	stringValue, err := strconv.Unquote(string(value.Raw))
	if err != nil {
		return "", true, err
	}
	return stringValue, true, nil
}
