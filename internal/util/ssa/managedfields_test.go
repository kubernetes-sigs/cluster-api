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
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/cluster-api/internal/contract"
)

func TestDropManagedFields(t *testing.T) {
	ctx := context.Background()

	ssaManager := "ssa-manager"

	tests := []struct {
		name                string
		updateManager       string
		obj                 client.Object
		wantOwnershipToDrop bool
	}{
		{
			name:                "should drop ownership of fields if there is no entry for ssaManager",
			wantOwnershipToDrop: true,
		},
		{
			name:                "should not drop ownership of fields if there is an entry for ssaManager",
			updateManager:       ssaManager,
			wantOwnershipToDrop: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			createCM := &corev1.ConfigMap{
				// Have to set TypeMeta explicitly when using SSA with typed objects.
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cm-1",
					Namespace: "default",
					Labels: map[string]string{
						"label-1": "value-1",
					},
					Annotations: map[string]string{
						"annotation-1": "value-1",
					},
					Finalizers: []string{"test.com/finalizer"},
				},
				Data: map[string]string{
					"test-key": "test-value",
				},
			}
			updateCM := createCM.DeepCopy()
			updateCM.Data["test-key-update"] = "test-value-update"

			g.Expect(env.Client.Create(ctx, createCM, client.FieldOwner(classicManager))).To(Succeed())
			t.Cleanup(func() {
				createCMWithoutFinalizer := createCM.DeepCopy()
				createCMWithoutFinalizer.ObjectMeta.Finalizers = []string{}
				g.Expect(env.Client.Patch(ctx, createCMWithoutFinalizer, client.MergeFrom(createCM))).To(Succeed())
				g.Expect(env.CleanupAndWait(ctx, createCM)).To(Succeed())
			})

			if tt.updateManager != "" {
				// If updateManager is set, update the object with SSA
				g.Expect(env.Client.Patch(ctx, updateCM, client.Apply, client.FieldOwner(tt.updateManager))).To(Succeed())
			}

			gotObj := createCM.DeepCopyObject().(client.Object)
			g.Expect(env.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(createCM), gotObj)).To(Succeed())
			labelsAndAnnotationsManagedFieldPaths := []contract.Path{
				{"f:metadata", "f:annotations"},
				{"f:metadata", "f:labels"},
			}
			g.Expect(DropManagedFields(ctx, env.Client, gotObj, ssaManager, labelsAndAnnotationsManagedFieldPaths)).To(Succeed())

			if tt.wantOwnershipToDrop {
				g.Expect(gotObj.GetManagedFields()).ShouldNot(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:labels"},
				))
				g.Expect(gotObj.GetManagedFields()).ShouldNot(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:annotations"},
				))
			} else {
				g.Expect(gotObj.GetManagedFields()).Should(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:labels"},
				))
				g.Expect(gotObj.GetManagedFields()).Should(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:annotations"},
				))
			}
			// Verify ownership of other fields is not affected.
			g.Expect(gotObj.GetManagedFields()).Should(MatchFieldOwnership(
				classicManager,
				metav1.ManagedFieldsOperationUpdate,
				contract.Path{"f:metadata", "f:finalizers"},
			))
		})
	}
}

func TestDropManagedFieldsWithFakeClient(t *testing.T) {
	ctx := context.Background()

	ssaManager := "ssa-manager"

	fieldV1Map := map[string]interface{}{
		"f:metadata": map[string]interface{}{
			"f:name":        map[string]interface{}{},
			"f:labels":      map[string]interface{}{},
			"f:annotations": map[string]interface{}{},
			"f:finalizers":  map[string]interface{}{},
		},
	}
	fieldV1, err := json.Marshal(fieldV1Map)
	if err != nil {
		panic(err)
	}

	objWithoutSSAManager := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-1",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:    classicManager,
				Operation:  metav1.ManagedFieldsOperationUpdate,
				FieldsType: "FieldsV1",
				FieldsV1:   &metav1.FieldsV1{Raw: fieldV1},
				APIVersion: "v1",
			}},
			Labels: map[string]string{
				"label-1": "value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
			Finalizers: []string{"test-finalizer"},
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}

	objectWithSSAManager := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cm-2",
			Namespace: "default",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:    classicManager,
					Operation:  metav1.ManagedFieldsOperationUpdate,
					FieldsType: "FieldsV1",
					FieldsV1:   &metav1.FieldsV1{Raw: fieldV1},
					APIVersion: "v1",
				},
				{
					Manager:    ssaManager,
					Operation:  metav1.ManagedFieldsOperationApply,
					FieldsType: "FieldsV1",
					APIVersion: "v1",
				},
			},
			Labels: map[string]string{
				"label-1": "value-1",
			},
			Annotations: map[string]string{
				"annotation-1": "value-1",
			},
			Finalizers: []string{"test-finalizer"},
		},
		Data: map[string]string{
			"test-key": "test-value",
		},
	}

	tests := []struct {
		name                string
		obj                 client.Object
		wantOwnershipToDrop bool
	}{
		{
			name:                "should drop ownership of fields if there is no entry for ssaManager",
			obj:                 objWithoutSSAManager,
			wantOwnershipToDrop: true,
		},
		{
			name:                "should not drop ownership of fields if there is an entry for ssaManager",
			obj:                 objectWithSSAManager,
			wantOwnershipToDrop: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeClient := fake.NewClientBuilder().WithObjects(tt.obj).WithReturnManagedFields().Build()
			labelsAndAnnotationsManagedFieldPaths := []contract.Path{
				{"f:metadata", "f:annotations"},
				{"f:metadata", "f:labels"},
			}
			g.Expect(DropManagedFields(ctx, fakeClient, tt.obj, ssaManager, labelsAndAnnotationsManagedFieldPaths)).To(Succeed())
			if tt.wantOwnershipToDrop {
				g.Expect(tt.obj.GetManagedFields()).ShouldNot(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:labels"},
				))
				g.Expect(tt.obj.GetManagedFields()).ShouldNot(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:annotations"},
				))
			} else {
				g.Expect(tt.obj.GetManagedFields()).Should(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:labels"},
				))
				g.Expect(tt.obj.GetManagedFields()).Should(MatchFieldOwnership(
					classicManager,
					metav1.ManagedFieldsOperationUpdate,
					contract.Path{"f:metadata", "f:annotations"},
				))
			}
			// Verify ownership of other fields is not affected.
			g.Expect(tt.obj.GetManagedFields()).Should(MatchFieldOwnership(
				classicManager,
				metav1.ManagedFieldsOperationUpdate,
				contract.Path{"f:metadata", "f:finalizers"},
			))
		})
	}
}
