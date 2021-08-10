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

package topology

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetTemplate(t *testing.T) {
	fakeControlPlaneTemplateCRDv99 := fakeControlPlaneTemplateCRD.DeepCopy()
	fakeControlPlaneTemplateCRDv99.Labels = map[string]string{
		"cluster.x-k8s.io/v1alpha4": "v1alpha4_v99",
	}
	crds := []client.Object{
		fakeControlPlaneTemplateCRDv99,
		fakeBootstrapTemplateCRD,
	}

	controlPlaneTemplate := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetemplate1").Obj()
	controlPlaneTemplatev99 := controlPlaneTemplate.DeepCopy()
	controlPlaneTemplatev99.SetAPIVersion(fakeControlPlaneProviderGroupVersion.Group + "/v99")

	workerBootstrapTemplate := newFakeBootstrapTemplate(metav1.NamespaceDefault, "workerbootstraptemplate1").Obj()

	tests := []struct {
		name    string
		ref     *corev1.ObjectReference
		objects []client.Object
		want    *unstructured.Unstructured
		wantRef *corev1.ObjectReference
		wantErr bool
	}{
		{
			name:    "Get object fails: ref is nil",
			ref:     nil,
			wantErr: true,
		},
		{
			name: "Get object",
			ref:  objToRef(workerBootstrapTemplate),
			objects: []client.Object{
				workerBootstrapTemplate,
			},
			want:    workerBootstrapTemplate,
			wantRef: objToRef(workerBootstrapTemplate),
		},
		{
			name:    "Get object fails: object does not exist",
			ref:     objToRef(workerBootstrapTemplate),
			objects: []client.Object{},
			wantErr: true,
		},
		{
			name: "Get object and update the ref",
			ref:  objToRef(controlPlaneTemplate),
			objects: []client.Object{
				controlPlaneTemplatev99,
			},
			want:    controlPlaneTemplatev99,
			wantRef: objToRef(controlPlaneTemplatev99),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			r := &ClusterReconciler{
				Client:                    fakeClient,
				UnstructuredCachingClient: fakeClient,
			}
			got, err := r.getTemplate(ctx, tt.ref)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(got).To(Equal(tt.want), cmp.Diff(tt.want, got))
			g.Expect(tt.ref).To(Equal(tt.wantRef), cmp.Diff(tt.wantRef, tt.ref))
		})
	}
}

func TestGetNestedRef(t *testing.T) {
	t.Run("Gets a nested ref if defined", func(t *testing.T) {
		g := NewWithT(t)

		infrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "control-plane-machine-infrastructure-template1").Obj()
		controlPlaneTemplate := newFakeControlPlane(metav1.NamespaceDefault, "control-plane-template").
			WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
			Obj()

		ref, err := getNestedRef(controlPlaneTemplate, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ref).ToNot(BeNil())
		g.Expect(ref.APIVersion).To(Equal(infrastructureMachineTemplate.GetAPIVersion()))
		g.Expect(ref.Kind).To(Equal(infrastructureMachineTemplate.GetKind()))
		g.Expect(ref.Name).To(Equal(infrastructureMachineTemplate.GetName()))
		g.Expect(ref.Namespace).To(Equal(infrastructureMachineTemplate.GetNamespace()))
	})
	t.Run("getNestedRef fails if the nested ref does not exist", func(t *testing.T) {
		g := NewWithT(t)

		controlPlaneTemplate := newFakeControlPlane(metav1.NamespaceDefault, "control-plane-template").Obj()

		ref, err := getNestedRef(controlPlaneTemplate, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(HaveOccurred())
		g.Expect(ref).To(BeNil())
	})
	t.Run("getNestedRef fails if the nested ref exist but it is incomplete", func(t *testing.T) {
		g := NewWithT(t)

		controlPlaneTemplate := newFakeControlPlane(metav1.NamespaceDefault, "control-plane-template").Obj()

		err := unstructured.SetNestedField(controlPlaneTemplate.UnstructuredContent(), "foo", "spec", "machineTemplate", "infrastructureRef", "kind")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(controlPlaneTemplate.UnstructuredContent(), "bar", "spec", "machineTemplate", "infrastructureRef", "namespace")
		g.Expect(err).ToNot(HaveOccurred())
		err = unstructured.SetNestedField(controlPlaneTemplate.UnstructuredContent(), "baz", "spec", "machineTemplate", "infrastructureRef", "apiVersion")
		g.Expect(err).ToNot(HaveOccurred())
		// Reference name missing

		ref, err := getNestedRef(controlPlaneTemplate, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(HaveOccurred())
		g.Expect(ref).To(BeNil())
	})
}

func TestSetNestedRef(t *testing.T) {
	t.Run("Sets a nested ref", func(t *testing.T) {
		g := NewWithT(t)
		infrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "control-plane-machine-infrastructure-template1").Obj()
		controlPlaneTemplate := newFakeControlPlane(metav1.NamespaceDefault, "control-plane-template").Obj()

		err := setNestedRef(controlPlaneTemplate, infrastructureMachineTemplate, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).To(BeNil())

		ref, err := getNestedRef(controlPlaneTemplate, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ref).ToNot(BeNil())
		g.Expect(ref.APIVersion).To(Equal(infrastructureMachineTemplate.GetAPIVersion()))
		g.Expect(ref.Kind).To(Equal(infrastructureMachineTemplate.GetKind()))
		g.Expect(ref.Name).To(Equal(infrastructureMachineTemplate.GetName()))
		g.Expect(ref.Namespace).To(Equal(infrastructureMachineTemplate.GetNamespace()))
	})
}

func TestObjToRef(t *testing.T) {
	t.Run("Gets a ref from an obj", func(t *testing.T) {
		g := NewWithT(t)
		infrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "control-plane-machine-infrastructure-template1").Obj()
		ref := objToRef(infrastructureMachineTemplate)

		g.Expect(ref).ToNot(BeNil())
		g.Expect(ref.APIVersion).To(Equal(infrastructureMachineTemplate.GetAPIVersion()))
		g.Expect(ref.Kind).To(Equal(infrastructureMachineTemplate.GetKind()))
		g.Expect(ref.Name).To(Equal(infrastructureMachineTemplate.GetName()))
		g.Expect(ref.Namespace).To(Equal(infrastructureMachineTemplate.GetNamespace()))
	})
}
