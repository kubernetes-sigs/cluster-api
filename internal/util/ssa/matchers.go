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
	"fmt"

	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
