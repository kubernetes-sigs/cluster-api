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

package ssa

import (
	"encoding/json"
	"fmt"

	"github.com/onsi/gomega/types"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/internal/contract"
)

// MatchManagedFieldsEntry is a gomega Matcher to check if a ManagedFieldsEntry has the given name and operation.
func MatchManagedFieldsEntry(manager string, operation metav1.ManagedFieldsOperationType) types.GomegaMatcher {
	return &managedFieldMatcher{
		manager:   manager,
		operation: operation,
	}
}

type managedFieldMatcher struct {
	manager   string
	operation metav1.ManagedFieldsOperationType
}

func (mf *managedFieldMatcher) Match(actual interface{}) (bool, error) {
	managedFieldsEntry, ok := actual.(metav1.ManagedFieldsEntry)
	if !ok {
		return false, fmt.Errorf("expecting metav1.ManagedFieldsEntry got %T", actual)
	}

	return managedFieldsEntry.Manager == mf.manager && managedFieldsEntry.Operation == mf.operation, nil
}

func (mf *managedFieldMatcher) FailureMessage(actual interface{}) string {
	managedFieldsEntry := actual.(metav1.ManagedFieldsEntry)
	return fmt.Sprintf("Expected ManagedFieldsEntry to match Manager:%s and Operation:%s, got Manager:%s, Operation:%s",
		mf.manager, mf.operation, managedFieldsEntry.Manager, managedFieldsEntry.Operation)
}

func (mf *managedFieldMatcher) NegatedFailureMessage(actual interface{}) string {
	managedFieldsEntry := actual.(metav1.ManagedFieldsEntry)
	return fmt.Sprintf("Expected ManagedFieldsEntry to not match Manager:%s and Operation:%s, got Manager:%s, Operation:%s",
		mf.manager, mf.operation, managedFieldsEntry.Manager, managedFieldsEntry.Operation)
}

// MatchFieldOwnership is a gomega Matcher to check if path is owned by the given manager and operation.
// Note: The path has to be specified as is observed in managed fields. Example: to check if the labels are owned
// by the correct manager the correct way to pass the path is contract.Path{"f:metadata","f:labels"}.
func MatchFieldOwnership(manager string, operation metav1.ManagedFieldsOperationType, path contract.Path) types.GomegaMatcher {
	return &fieldOwnershipMatcher{
		path:      path,
		manager:   manager,
		operation: operation,
	}
}

type fieldOwnershipMatcher struct {
	path      contract.Path
	manager   string
	operation metav1.ManagedFieldsOperationType
}

func (fom *fieldOwnershipMatcher) Match(actual interface{}) (bool, error) {
	managedFields, ok := actual.([]metav1.ManagedFieldsEntry)
	if !ok {
		return false, fmt.Errorf("expecting []metav1.ManagedFieldsEntry got %T", actual)
	}
	for _, managedFieldsEntry := range managedFields {
		if managedFieldsEntry.Manager == fom.manager && managedFieldsEntry.Operation == fom.operation {
			fieldsV1 := map[string]interface{}{}
			if err := json.Unmarshal(managedFieldsEntry.FieldsV1.Raw, &fieldsV1); err != nil {
				return false, errors.Wrap(err, "failed to parse managedFieldsEntry.FieldsV1")
			}
			FilterIntent(&FilterIntentInput{
				Path:         contract.Path{},
				Value:        fieldsV1,
				ShouldFilter: IsNotAllowedPath([]contract.Path{fom.path}),
			})
			return len(fieldsV1) > 0, nil
		}
	}
	return false, nil
}

func (fom *fieldOwnershipMatcher) FailureMessage(actual interface{}) string {
	managedFields := actual.([]metav1.ManagedFieldsEntry)
	return fmt.Sprintf("Expected Path %s to be owned by Manager:%s and Operation:%s, did not find correct ownership: %s",
		fom.path, fom.manager, fom.operation, managedFields)
}

func (fom *fieldOwnershipMatcher) NegatedFailureMessage(actual interface{}) string {
	managedFields := actual.([]metav1.ManagedFieldsEntry)
	return fmt.Sprintf("Expected Path %s to not be owned by Manager:%s and Operation:%s, did not find correct ownership: %s",
		fom.path, fom.manager, fom.operation, managedFields)
}
