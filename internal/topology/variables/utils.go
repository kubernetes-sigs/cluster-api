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
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// emptyDefinitionFrom is the definitionFrom value used when none is supplied by the user.
	emptyDefinitionFrom = ""
)

// newValuesIndex return a map of ClusterVariable values per name and definitionsFrom.
// This function validates that:
// - variables are not defined more than once in Cluster spec/
// - variables with the same name do not have a mix of empty and non-empty DefinitionFrom.
func newValuesIndex(values []clusterv1.ClusterVariable) (map[string]map[string]clusterv1.ClusterVariable, error) {
	valuesMap := map[string]map[string]clusterv1.ClusterVariable{}
	errs := []error{}
	for _, value := range values {
		c := value
		_, ok := valuesMap[c.Name]
		if !ok {
			valuesMap[c.Name] = map[string]clusterv1.ClusterVariable{}
		}
		// Check that the variable has not been defined more than once with the same definitionFrom.
		if _, ok := valuesMap[c.Name][c.DefinitionFrom]; ok {
			errs = append(errs, errors.Errorf("variable %q from %q is defined more than once", c.Name, c.DefinitionFrom))
			continue
		}
		// Add the variable.
		valuesMap[c.Name][c.DefinitionFrom] = c
	}
	if len(errs) > 0 {
		return nil, kerrors.NewAggregate(errs)
	}
	// Validate that the variables do not have incompatible values in their `definitionFrom` fields.
	if err := validateValuesDefinitionFrom(valuesMap); err != nil {
		return nil, err
	}

	return valuesMap, nil
}

// validateValuesDefinitionFrom validates that variables are not defined with both an empty DefinitionFrom and a
// non-empty DefinitionFrom.
func validateValuesDefinitionFrom(values map[string]map[string]clusterv1.ClusterVariable) error {
	var errs []error
	for name, valuesForName := range values {
		for _, value := range valuesForName {
			// Append an error if the value has a non-empty definitionFrom but there's also a value with
			// an empty DefinitionFrom.
			if _, ok := valuesForName[emptyDefinitionFrom]; ok && value.DefinitionFrom != emptyDefinitionFrom {
				errs = append(errs, errors.Errorf("variable %q is defined with a mix of empty and non-empty values for definitionFrom", name))
				break // No need to check other values for this variable.
			}
		}
	}
	if len(errs) > 0 {
		return kerrors.NewAggregate(errs)
	}
	return nil
}

type statusVariableDefinition struct {
	*clusterv1.ClusterClassStatusVariableDefinition
	Name      string
	Conflicts bool
}

type definitionsIndex map[string]map[string]*statusVariableDefinition

// newDefinitionsIndex returns a definitionsIndex with ClusterClassStatusVariable definitions by name and definition.From.
// This index has special handling for variables with no definition conflicts in the `get` method.
func newDefinitionsIndex(definitions []clusterv1.ClusterClassStatusVariable) definitionsIndex {
	i := definitionsIndex{}
	for _, def := range definitions {
		i.store(def)
	}
	return i
}
func (i definitionsIndex) store(definition clusterv1.ClusterClassStatusVariable) {
	for _, d := range definition.Definitions {
		if _, ok := i[definition.Name]; !ok {
			i[definition.Name] = map[string]*statusVariableDefinition{}
		}
		i[definition.Name][d.From] = &statusVariableDefinition{
			Name:                                 definition.Name,
			Conflicts:                            definition.DefinitionsConflict,
			ClusterClassStatusVariableDefinition: d.DeepCopy(),
		}
	}
}

// get returns a statusVariableDefinition for a given name and definitionFrom. If the definition has no conflicts it can
// be retrieved using an emptyDefinitionFrom.
// Return an error if the definition is not found or if the definitionFrom is empty and there are conflicts in the definitions.
func (i definitionsIndex) get(name, definitionFrom string) (*statusVariableDefinition, error) {
	// If no variable with this name exists return an error.
	if _, ok := i[name]; !ok {
		return nil, errors.Errorf("no definitions found for variable %q", name)
	}

	// If the definition exists for the specific definitionFrom, return it.
	if def, ok := i[name][definitionFrom]; ok {
		return def, nil
	}

	// If definitionFrom is empty and there are no conflicts return a definition with an emptyDefinitionFrom.
	if definitionFrom == emptyDefinitionFrom {
		for _, def := range i[name] {
			if !def.Conflicts {
				return &statusVariableDefinition{
					Name:      def.Name,
					Conflicts: def.Conflicts,
					ClusterClassStatusVariableDefinition: &clusterv1.ClusterClassStatusVariableDefinition{
						// Return the definition with an empty definitionFrom. This ensures when a user gets
						// a definition with an emptyDefinitionFrom, the return value also has emptyDefinitionFrom.
						// This is used in variable defaulting to ensure variables that only need one value for multiple
						// definitions have an emptyDefinitionFrom.
						From:     emptyDefinitionFrom,
						Required: def.Required,
						Schema:   def.Schema,
					},
				}, nil
			}
			return nil, errors.Errorf("variable %q has conflicting definitions. It requires a non-empty `definitionFrom`", name)
		}
	}
	return nil, errors.Errorf("no definitions found for variable %q from %q", name, definitionFrom)
}
