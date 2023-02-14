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

// Package ssa contains utils related to ssa.
package ssa

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanUpManagedFieldsForSSAAdoption(t *testing.T) {
	ctx := context.Background()

	ssaManager := "ssa-manager"
	classicManager := "manager"

	objWithoutAnyManager := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-1",
			Namespace: "default",
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}
	objWithOnlyClassicManager := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-1",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:   classicManager,
				Operation: metav1.ManagedFieldsOperationUpdate,
			}},
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}
	objWithOnlySSAManager := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-1",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:   ssaManager,
				Operation: metav1.ManagedFieldsOperationApply,
			}},
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}
	objWithClassicManagerAndSSAManager := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-1",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:   classicManager,
					Operation: metav1.ManagedFieldsOperationUpdate,
				},
				{
					Manager:   ssaManager,
					Operation: metav1.ManagedFieldsOperationApply,
				},
			},
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}

	tests := []struct {
		name                       string
		obj                        client.Object
		wantEntryForClassicManager bool
	}{
		{
			name:                       "should add an entry for ssaManager if it does not have one",
			obj:                        objWithoutAnyManager,
			wantEntryForClassicManager: false,
		},
		{
			name:                       "should add an entry for ssaManager and drop entry for classic manager if it exists",
			obj:                        objWithOnlyClassicManager,
			wantEntryForClassicManager: false,
		},
		{
			name:                       "should keep the entry for ssa-manager if it already has one (no-op)",
			obj:                        objWithOnlySSAManager,
			wantEntryForClassicManager: false,
		},
		{
			name:                       "should keep the entry with ssa-manager if it already has one - should not drop other manager entries (no-op)",
			obj:                        objWithClassicManagerAndSSAManager,
			wantEntryForClassicManager: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.obj).Build()
			g.Expect(CleanUpManagedFieldsForSSAAdoption(ctx, tt.obj, ssaManager, fakeClient)).Should(Succeed())
			g.Expect(tt.obj.GetManagedFields()).Should(
				ContainElement(MatchManagedFieldsEntry(ssaManager, metav1.ManagedFieldsOperationApply)))
			if tt.wantEntryForClassicManager {
				g.Expect(tt.obj.GetManagedFields()).Should(
					ContainElement(MatchManagedFieldsEntry(classicManager, metav1.ManagedFieldsOperationUpdate)))
			} else {
				g.Expect(tt.obj.GetManagedFields()).ShouldNot(
					ContainElement(MatchManagedFieldsEntry(classicManager, metav1.ManagedFieldsOperationUpdate)))
			}
		})
	}
}
