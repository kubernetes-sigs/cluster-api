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
	"sigs.k8s.io/cluster-api/internal/test/builder"
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
			allErrs := ObjectsAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
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

			allErrs := ObjectsAreStrictlyCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
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
			g := NewWithT(t)
			allErrs := LocalObjectTemplatesAreCompatible(tt.current, tt.desired, field.NewPath("spec"))
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
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
			allErrs := LocalObjectTemplateIsValid(tt.template, namespace, pathPrefix)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestClusterClassesAreCompatible(t *testing.T) {
	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	incompatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	compatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
		Namespace:  "default",
	}

	tests := []struct {
		name    string
		current *clusterv1.ClusterClass
		desired *clusterv1.ClusterClass
		wantErr bool
	}{
		{
			name: "pass for compatible clusterClasses",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(compatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(compatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "error if clusterClass has incompatible ControlPlane ref",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass for incompatible ref in MachineDeploymentClass bootstrapTemplate clusterClasses",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(ref)).Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "pass if machineDeploymentClass is removed from ClusterClass",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		g := NewWithT(t)
		t.Run(tt.name, func(t *testing.T) {
			allErrs := ClusterClassesAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachineDeploymentClassesAreCompatible(t *testing.T) {
	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	compatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
		Namespace:  "default",
	}
	incompatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}

	tests := []struct {
		name    string
		current *clusterv1.ClusterClass
		desired *clusterv1.ClusterClass
		wantErr bool
	}{
		{
			name: "pass if machineDeploymentClasses are compatible",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(compatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "pass if machineDeploymentClass is removed from ClusterClass",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "error if machineDeploymentClass has multiple incompatible references",
			current: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build(),
				).
				Build(),
			desired: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(incompatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(incompatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(compatibleRef)).Build()).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachineDeploymentClassesAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachineDeploymentClassesAreUnique(t *testing.T) {
	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		wantErr      bool
	}{
		{
			name: "pass if MachineDeploymentClasses are unique",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "fail if MachineDeploymentClasses are duplicated",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if multiple MachineDeploymentClasses are identical",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachineDeploymentClassesAreUnique(tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestMachineDeploymentTopologiesAreUniqueAndDefinedInClusterClass(t *testing.T) {
	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		cluster      *clusterv1.Cluster
		wantErr      bool
	}{
		{
			name: "fail if MachineDeploymentTopologies name is empty",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						// The name should not be empty.
						builder.MachineDeploymentTopology("").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if MachineDeploymentTopologies are unique and defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "fail if MachineDeploymentTopologies are unique but not defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("bb").
								Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "fail if MachineDeploymentTopologies are not unique but is defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: true,
		},
		{
			name: "pass if MachineDeploymentTopologies are unique and share a class that is defined in ClusterClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinfra1").Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						WithVersion("v1.22.2").
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers1").
								WithClass("aa").
								Build()).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("workers2").
								WithClass("aa").
								Build()).
						Build()).
				Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := MachineDeploymentTopologiesAreValidAndDefinedInClusterClass(tt.cluster, tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func TestClusterClassReferencesAreValid(t *testing.T) {
	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	invalidRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
		Namespace:  "",
	}

	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		wantErr      bool
	}{
		{
			name: "pass for clusterClass with valid references",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				Build(),
			wantErr: false,
		},
		{
			name: "error if clusterClass has multiple invalid refs",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(invalidRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("a").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build(),
					*builder.MachineDeploymentClass("b").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build(),
					*builder.MachineDeploymentClass("c").
						WithInfrastructureTemplate(
							refToUnstructured(invalidRef)).
						WithBootstrapTemplate(
							refToUnstructured(invalidRef)).
						Build()).
				Build(),
			wantErr: true,
		},

		{
			name: "error if clusterClass has invalid ControlPlane ref",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					refToUnstructured(invalidRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							refToUnstructured(ref)).
						Build()).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			allErrs := ClusterClassReferencesAreValid(tt.clusterClass)
			if tt.wantErr {
				g.Expect(allErrs).ToNot(BeEmpty())
				return
			}
			g.Expect(allErrs).To(BeEmpty())
		})
	}
}

func refToUnstructured(ref *corev1.ObjectReference) *unstructured.Unstructured {
	gvk := ref.GetObjectKind().GroupVersionKind()
	output := &unstructured.Unstructured{}
	output.SetKind(gvk.Kind)
	output.SetAPIVersion(gvk.GroupVersion().String())
	output.SetName(ref.Name)
	output.SetNamespace(ref.Namespace)
	return output
}
