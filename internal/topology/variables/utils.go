/*
Copyright 2023 The Kubernetes Authors.

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
	"strings"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// newValuesIndex returns a map of ClusterVariable per name.
// This function validates that:
// - DefinitionFrom is not set
// - variables are not defined more than once.
func newValuesIndex(fldPath *field.Path, values []clusterv1.ClusterVariable) (map[string]*clusterv1.ClusterVariable, field.ErrorList) {
	valuesMap := map[string]*clusterv1.ClusterVariable{}
	errs := field.ErrorList{}
	for _, value := range values {
		// Check that the variable has DefinitionFrom not set.
		if value.DefinitionFrom != "" { //nolint:staticcheck // Intentionally using the deprecated field here to check that it is not set.
			errs = append(errs, field.Invalid(fldPath.Key(value.Name), string(value.Value.Raw), fmt.Sprintf("variable %q has DefinitionFrom set. DefinitionFrom is deprecated, must not be set anymore and is going to be removed in the next apiVersion", value.Name)))
		}

		// Check that the variable has not been defined more than once.
		if _, ok := valuesMap[value.Name]; ok {
			errs = append(errs, field.Invalid(fldPath.Key(value.Name), string(value.Value.Raw), fmt.Sprintf("variable %q is set more than once", value.Name)))
		}

		// Add the variable.
		valuesMap[value.Name] = &value
	}
	if len(errs) > 0 {
		return nil, errs
	}

	return valuesMap, nil
}

// newDefinitionIndex returns a map of ClusterClassStatusVariable per name.
// This function validates that:
// - variables have definitions
// - variables don't have conflicting definitions.
func newDefinitionsIndex(fldPath *field.Path, definitions []clusterv1.ClusterClassStatusVariable) (map[string]*clusterv1.ClusterClassStatusVariable, field.ErrorList) {
	definitionsMap := map[string]*clusterv1.ClusterClassStatusVariable{}
	errs := []error{}
	for _, definition := range definitions {
		// Check that the definition has definitions.
		if len(definition.Definitions) == 0 {
			errs = append(errs, errors.Errorf("variable %q has no definitions", definition.Name))
		}

		// Check that the definitions have no conflict.
		if definition.DefinitionsConflict {
			errs = append(errs, errors.Errorf("variable %q has conflicting definitions", definition.Name))
		}

		// Add the definition.
		definitionsMap[definition.Name] = &definition
	}
	if len(errs) > 0 {
		var definitionStrings []string
		for _, d := range definitions {
			definitionStrings = append(definitionStrings, fmt.Sprintf("Name: %s", d.Name))
		}
		return nil, field.ErrorList{field.TypeInvalid(fldPath, "["+strings.Join(definitionStrings, ",")+"]", fmt.Sprintf("variable definitions in the ClusterClass not valid: %s", kerrors.NewAggregate(errs)))}
	}

	return definitionsMap, nil
}
