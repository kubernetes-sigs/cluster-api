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

package check

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

type referencedObjectsCompatibilityTestCase struct {
	name    string
	current *unstructured.Unstructured
	desired *unstructured.Unstructured
	wantErr bool
}

var referencedObjectsCompatibilityTestCases = []referencedObjectsCompatibilityTestCase{
	{
		name: "Fails if group changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "foo/v1beta1",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "bar/v1beta1",
			},
		},
		wantErr: true,
	},
	{
		name: "Fails if kind changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "bar",
			},
		},
		wantErr: true,
	},
	{
		name: "Pass if gvk remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Pass if version changes but group and kind remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/bar",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Fails if namespace changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "foo",
				},
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "bar",
				},
			},
		},
		wantErr: true,
	},
}

func TestObjectsAreCompatible(t *testing.T) {
	for _, tt := range referencedObjectsCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			allErrs := ObjectsAreCompatible(tt.current, tt.desired).ToAggregate()
			if tt.wantErr {
				g.Expect(allErrs).To(HaveOccurred())
				return
			}
			g.Expect(allErrs).ToNot(HaveOccurred())
		})
	}
}

func TestObjectsAreStrictlyCompatible(t *testing.T) {
	referencedObjectsStrictCompatibilityTestCases := append(referencedObjectsCompatibilityTestCases, []referencedObjectsCompatibilityTestCase{
		{
			name: "Fails if name changes",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "bar",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass if name remains the same",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			wantErr: false,
		},
	}...)

	for _, tt := range referencedObjectsStrictCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			allErrs := ObjectsAreStrictlyCompatible(tt.current, tt.desired).ToAggregate()
			if tt.wantErr {
				g.Expect(allErrs).To(HaveOccurred())
				return
			}
			g.Expect(allErrs).ToNot(HaveOccurred())
		})
	}
}

func TestLocalObjectTemplatesAreCompatible(t *testing.T) {
	template := clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "foo",
			Kind:       "bar",
			APIVersion: "test.group.io/versionone",
		},
	}
	compatibleNameChangeTemplate := clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "newFoo",
			Kind:       "bar",
			APIVersion: "test.group.io/versionone",
		},
	}
	compatibleAPIVersionChangeTemplate := clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "foo",
			Kind:       "bar",
			APIVersion: "test.group.io/versiontwo",
		},
	}
	incompatibleNamespaceChangeTemplate := clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "different",
			Name:       "foo",
			Kind:       "bar",
			APIVersion: "test.group.io/versionone",
		},
	}
	incompatibleAPIGroupChangeTemplate := clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "foo",
			Kind:       "bar",
			APIVersion: "production.group.io/versionone",
		},
	}
	incompatibleAPIKindChangeTemplate := clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "foo",
			Kind:       "notabar",
			APIVersion: "test.group.io/versionone",
		},
	}
	tests := []struct {
		name    string
		current clusterv1.LocalObjectTemplate
		desired clusterv1.LocalObjectTemplate
		wantErr bool
	}{
		{
			name:    "Allow change to template name",
			current: template,
			desired: compatibleNameChangeTemplate,
			wantErr: false,
		},
		{
			name:    "Allow change to template APIVersion",
			current: template,
			desired: compatibleAPIVersionChangeTemplate,
			wantErr: false,
		},
		{
			name:    "Block change to template API Group",
			current: template,
			desired: incompatibleAPIGroupChangeTemplate,
			wantErr: true,
		},
		{
			name:    "Block change to template namespace",
			current: template,
			desired: incompatibleNamespaceChangeTemplate,
			wantErr: true,
		},
		{
			name:    "Block change to template API Kind",
			current: template,
			desired: incompatibleAPIKindChangeTemplate,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := LocalObjectTemplatesAreCompatible(tt.current, tt.desired); (err != nil) != tt.wantErr {
				t.Errorf("LocalObjectTemplatesAreCompatible() error = %v, wantErr %v", err.ToAggregate(), tt.wantErr)
			}
		})
	}
}

func TestLocalObjectTemplateIsValid(t *testing.T) {
	namespace := metav1.NamespaceDefault
	pathPrefix := field.NewPath("this", "is", "a", "prefix")

	validTemplate := &clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "valid",
			Kind:       "barTemplate",
			APIVersion: "test.group.io/versionone",
		},
	}

	nilTemplate := &clusterv1.LocalObjectTemplate{
		Ref: nil,
	}
	emptyNameTemplate := &clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "",
			Kind:       "barTemplate",
			APIVersion: "test.group.io/versionone",
		},
	}
	wrongNamespaceTemplate := &clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "wrongNamespace",
			Name:       "foo",
			Kind:       "barTemplate",
			APIVersion: "test.group.io/versiontwo",
		},
	}
	notTemplateKindTemplate := &clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "foo",
			Kind:       "bar",
			APIVersion: "test.group.io/versionone",
		},
	}
	invalidAPIVersionTemplate := &clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "foo",
			Kind:       "barTemplate",
			APIVersion: "this/has/too/many/slashes",
		},
	}
	emptyAPIVersionTemplate := &clusterv1.LocalObjectTemplate{
		Ref: &corev1.ObjectReference{
			Namespace:  "default",
			Name:       "foo",
			Kind:       "barTemplate",
			APIVersion: "",
		},
	}

	tests := []struct {
		template *clusterv1.LocalObjectTemplate
		name     string
		wantErr  bool
	}{
		{
			name:     "No error with valid Template",
			template: validTemplate,
			wantErr:  false,
		},

		{
			name:     "Invalid if ref is nil",
			template: nilTemplate,
			wantErr:  true,
		},
		{
			name:     "Invalid if name is empty",
			template: emptyNameTemplate,
			wantErr:  true,
		},
		{
			name:     "Invalid if namespace doesn't match",
			template: wrongNamespaceTemplate,
			wantErr:  true,
		},
		{
			name:     "Invalid if Kind doesn't have Template suffix",
			template: notTemplateKindTemplate,
			wantErr:  true,
		},
		{
			name:     "Invalid if apiVersion is not valid",
			template: invalidAPIVersionTemplate,
			wantErr:  true,
		},
		{
			name:     "Empty apiVersion is not valid",
			template: emptyAPIVersionTemplate,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			allErrs := LocalObjectTemplateIsValid(tt.template, namespace, pathPrefix).ToAggregate()
			if tt.wantErr {
				g.Expect(allErrs).To(HaveOccurred())
				return
			}
			g.Expect(allErrs).ToNot(HaveOccurred())
		})
	}
}
