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
	"strings"

	"github.com/pkg/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

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

	if err := json.Unmarshal(sanitizeJSON(value.Raw), into); err != nil {
		return errors.Wrapf(err, "failed to unmarshal variable json %q into %q", string(value.Raw), into)
	}

	return nil
}

func sanitizeJSON(input []byte) (output []byte) {
	output = []byte(strings.ReplaceAll(string(input), "\\", ""))
	return output
}
