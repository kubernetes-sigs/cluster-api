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
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/valyala/fastjson"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
)

const (
	leftArrayDelim  = "["
	rightArrayDelim = "]"
)

// GetVariableValue returns a variable from the variables map.
func GetVariableValue(variables map[string]apiextensionsv1.JSON, variablePath string) (*apiextensionsv1.JSON, error) {
	// Split the variablePath (format: "<variableName>.<relativePath>").
	variableSplit := strings.Split(variablePath, ".")
	variableName, relativePath := variableSplit[0], variableSplit[1:]

	// Parse the path segment.
	variableNameSegment, err := parsePathSegment(variableName)
	if err != nil {
		return nil, errors.Wrapf(err, "variable %q is invalid", variablePath)
	}

	// Get the variable.
	value, ok := variables[variableNameSegment.path]
	if !ok {
		return nil, notFoundError{reason: notFoundReason, message: fmt.Sprintf("variable %q does not exist", variableName)}
	}

	// Return the value, if variablePath points to a top-level variable, i.e. hos no relativePath and no
	// array index (i.e. "<variableName>").
	if len(relativePath) == 0 && !variableNameSegment.HasIndex() {
		return &value, nil
	}

	// Parse the variable object.
	variable, err := fastjson.ParseBytes(value.Raw)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse variable %q: %s", variableName, string(value.Raw))
	}

	// If variableName contains an array index, get the array element (i.e. starts with "<variableName>[i]").
	// Then return it, if there is no relative path (i.e. "<variableName>[i]")
	if variableNameSegment.HasIndex() {
		variable, err = getVariableArrayElement(variable, variableNameSegment, variablePath)
		if err != nil {
			return nil, err
		}

		if len(relativePath) == 0 {
			return &apiextensionsv1.JSON{
				Raw: variable.MarshalTo([]byte{}),
			}, nil
		}
	}

	// If the variablePath points to a nested variable, i.e. has a relativePath, inspect the variable object.

	// Retrieve each segment of the relativePath incrementally, taking care of resolving array indexes.
	for _, p := range relativePath {
		// Parse the path segment.
		pathSegment, err := parsePathSegment(p)
		if err != nil {
			return nil, errors.Wrapf(err, "variable %q has invalid syntax", variablePath)
		}

		// Return if the variable does not exist.
		if !variable.Exists(pathSegment.path) {
			return nil, notFoundError{reason: notFoundReason, message: fmt.Sprintf("variable %q does not exist: failed to lookup segment %q", variablePath, pathSegment.path)}
		}

		// Get the variable from the variable object.
		variable = variable.Get(pathSegment.path)

		// Continue if the path doesn't contain an index.
		if !pathSegment.HasIndex() {
			continue
		}

		variable, err = getVariableArrayElement(variable, pathSegment, variablePath)
		if err != nil {
			return nil, err
		}
	}

	// Return the marshalled value of the variable.
	return &apiextensionsv1.JSON{
		Raw: variable.MarshalTo([]byte{}),
	}, nil
}

type pathSegment struct {
	path  string
	index *int
}

func (p pathSegment) HasIndex() bool {
	return p.index != nil
}

func parsePathSegment(segment string) (*pathSegment, error) {
	if (strings.Contains(segment, leftArrayDelim) && !strings.Contains(segment, rightArrayDelim)) ||
		(!strings.Contains(segment, leftArrayDelim) && strings.Contains(segment, rightArrayDelim)) {
		return nil, errors.Errorf("failed to parse path segment %q", segment)
	}

	if !strings.Contains(segment, leftArrayDelim) && !strings.Contains(segment, rightArrayDelim) {
		return &pathSegment{
			path: segment,
		}, nil
	}

	arrayIndexStr := segment[strings.Index(segment, leftArrayDelim)+1 : strings.Index(segment, rightArrayDelim)]
	index, err := strconv.Atoi(arrayIndexStr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse array index in path segment %q", segment)
	}
	if index < 0 {
		return nil, errors.Errorf("invalid array index %d in path segment %q", index, segment)
	}

	return &pathSegment{
		path:  segment[:strings.Index(segment, leftArrayDelim)], //nolint:gocritic // We already check above that segment contains leftArrayDelim,
		index: ptr.To(index),
	}, nil
}

// getVariableArrayElement gets the array element of a given array.
func getVariableArrayElement(array *fastjson.Value, arrayPathSegment *pathSegment, fullVariablePath string) (*fastjson.Value, error) {
	// Retrieve the array element, handling index out of range.
	arr, err := array.Array()
	if err != nil {
		return nil, errors.Wrapf(err, "variable %q is invalid: failed to get array %q", fullVariablePath, arrayPathSegment.path)
	}

	if len(arr) < *arrayPathSegment.index+1 {
		return nil, errors.Errorf("variable %q is invalid: array does not have index %d", fullVariablePath, arrayPathSegment.index)
	}

	return arr[*arrayPathSegment.index], nil
}
