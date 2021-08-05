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

package controllers

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetClass(t *testing.T) {
	crds := []client.Object{
		fakeInfrastructureClusterTemplateCRD,
		fakeControlPlaneTemplateCRD,
		fakeInfrastructureMachineTemplateCRD,
		fakeBootstrapTemplateCRD,
	}

	infraClusterTemplate := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "infraclustertemplate1").Obj()
	controlPlaneTemplate := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetemplate1").Obj()

	controlPlaneInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "controlplaneinframachinetemplate1").Obj()
	controlPlaneTemplateWithInfrastructureMachine := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetempaltewithinfrastructuremachine1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Obj()

	workerInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "workerinframachinetemplate1").Obj()
	workerBootstrapTemplate := newFakeBootstrapTemplate(metav1.NamespaceDefault, "workerbootstraptemplate1").Obj()

	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		objects      []client.Object
		want         *clusterTopologyClass
		wantErr      bool
	}{
		{
			name:    "ClusterClass does not exist",
			wantErr: true,
		},
		{
			name:         "ClusterClass exists without references",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "clusterclass1").Obj(),
			wantErr:      true,
		},
		{
			name: "Ref to missing InfraClusterTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				Obj(),
			wantErr: true,
		},
		{
			name: "Valid ref to InfraClusterTemplate, Ref to missing ControlPlaneTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate and ControlPlaneTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
			},
			want: &clusterTopologyClass{
				clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					Obj(),
				infrastructureClusterTemplate: infraClusterTemplate,
				controlPlane: controlPlaneTopologyClass{
					template: controlPlaneTemplate,
				},
				machineDeployments: map[string]machineDeploymentTopologyClass{},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate and ControlPlaneInfrastructureMachineTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplateWithInfrastructureMachine,
				controlPlaneInfrastructureMachineTemplate,
			},
			want: &clusterTopologyClass{
				clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
					WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
					Obj(),
				infrastructureClusterTemplate: infraClusterTemplate,
				controlPlane: controlPlaneTopologyClass{
					template:                      controlPlaneTemplateWithInfrastructureMachine,
					infrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				machineDeployments: map[string]machineDeploymentTopologyClass{},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, Ref to missing ControlPlaneInfrastructureMachineTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, worker InfrastructureMachineTemplate and BootstrapTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
				workerBootstrapTemplate,
			},
			want: &clusterTopologyClass{
				clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
					Obj(),
				infrastructureClusterTemplate: infraClusterTemplate,
				controlPlane: controlPlaneTopologyClass{
					template: controlPlaneTemplate,
				},
				machineDeployments: map[string]machineDeploymentTopologyClass{
					"workerclass1": {
						infrastructureMachineTemplate: workerInfrastructureMachineTemplate,
						bootstrapTemplate:             workerBootstrapTemplate,
					},
				},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, InfrastructureMachineTemplate, Ref to missing BootstrapTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, worker BootstrapTemplate, Ref to missing InfrastructureMachineTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerBootstrapTemplate,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)

			cluster := newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj()

			if tt.clusterClass != nil {
				cluster.Spec.Topology.Class = tt.clusterClass.Name
				objs = append(objs, tt.clusterClass)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			r := &ClusterTopologyReconciler{
				Client:                    fakeClient,
				UnstructuredCachingClient: fakeClient,
			}
			got, err := r.getClass(context.Background(), cluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}

			g.Expect(got.clusterClass).To(Equal(tt.want.clusterClass), cmp.Diff(tt.want.clusterClass, got.clusterClass))
			g.Expect(got.infrastructureClusterTemplate).To(Equal(tt.want.infrastructureClusterTemplate), cmp.Diff(tt.want.infrastructureClusterTemplate, got.infrastructureClusterTemplate))
			g.Expect(got.controlPlane).To(Equal(tt.want.controlPlane), cmp.Diff(tt.want.controlPlane, got.controlPlane, cmp.AllowUnexported(unstructured.Unstructured{}, controlPlaneTopologyClass{})))
			g.Expect(got.machineDeployments).To(Equal(tt.want.machineDeployments), cmp.Diff(tt.want.machineDeployments, got.machineDeployments, cmp.AllowUnexported(machineDeploymentTopologyClass{})))
		})
	}
}

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

			r := &ClusterTopologyReconciler{
				Client:                    fakeClient,
				UnstructuredCachingClient: fakeClient,
			}
			got, err := r.getTemplate(context.Background(), tt.ref)
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

var (
	fakeInfrastructureProviderGroupVersion = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1alpha4"}
	fakeControlPlaneProviderGroupVersion   = schema.GroupVersion{Group: "controlplane.cluster.x-k8s.io", Version: "v1alpha4"}
	fakeBootstrapProviderGroupVersion      = schema.GroupVersion{Group: "bootstrap.cluster.x-k8s.io", Version: "v1alpha4"}

	fakeInfrastructureClusterTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "fakeinfrastructureclustertemplates.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: fakeInfrastructureProviderGroupVersion.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "FakeInfrastructureClusterTemplate",
			},
		},
	}
	fakeControlPlaneTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "fakecontrolplanetemplates.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: fakeControlPlaneProviderGroupVersion.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "FakeControlPlaneTemplate",
			},
		},
	}
	fakeInfrastructureMachineTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "fakeinfrastructuremachinetemplates.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: fakeInfrastructureProviderGroupVersion.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "FakeInfrastructureMachineTemplate",
			},
		},
	}
	fakeBootstrapTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "fakebootstraptemplates.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: fakeBootstrapProviderGroupVersion.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "FakeBoostrapTemplate",
			},
		},
	}
)

type fakeCluster struct {
	namespace string
	name      string
}

func newFakeCluster(namespace, name string) *fakeCluster {
	return &fakeCluster{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeCluster) Obj() *clusterv1.Cluster {
	obj := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
			// Nb. This is set to the same resourceVersion the fakeClient uses internally to make comparison between objects
			// added to the fakeClient and expected objects easier.
			ResourceVersion: "999",
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{},
		},
	}
	return obj
}

type fakeClusterClass struct {
	namespace                                 string
	name                                      string
	infrastructureClusterTemplate             *unstructured.Unstructured
	controlPlaneTemplate                      *unstructured.Unstructured
	controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
	workerMachineDeploymentTemplates          map[string]fakeClusterClassMachineDeploymentTemplates
}

type fakeClusterClassMachineDeploymentTemplates struct {
	infrastructureMachineTemplate *unstructured.Unstructured
	bootstrapTemplate             *unstructured.Unstructured
}

func newFakeClusterClass(namespace, name string) *fakeClusterClass {
	return &fakeClusterClass{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeClusterClass) WithInfrastructureClusterTemplate(t *unstructured.Unstructured) *fakeClusterClass {
	f.infrastructureClusterTemplate = t
	return f
}

func (f *fakeClusterClass) WithControlPlaneTemplate(t *unstructured.Unstructured) *fakeClusterClass {
	f.controlPlaneTemplate = t
	return f
}

func (f *fakeClusterClass) WithControlPlaneInfrastructureMachineTemplate(t *unstructured.Unstructured) *fakeClusterClass {
	f.controlPlaneInfrastructureMachineTemplate = t
	return f
}

func (f *fakeClusterClass) WithWorkerMachineDeploymentTemplates(class string, infrastructureMachineTemplate, bootstrapTemplate *unstructured.Unstructured) *fakeClusterClass {
	if f.workerMachineDeploymentTemplates == nil {
		f.workerMachineDeploymentTemplates = map[string]fakeClusterClassMachineDeploymentTemplates{}
	}
	f.workerMachineDeploymentTemplates[class] = fakeClusterClassMachineDeploymentTemplates{
		infrastructureMachineTemplate: infrastructureMachineTemplate,
		bootstrapTemplate:             bootstrapTemplate,
	}
	return f
}

func (f *fakeClusterClass) Obj() *clusterv1.ClusterClass {
	obj := &clusterv1.ClusterClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterClass",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
			// Nb. This is set to the same resourceVersion the fakeClient uses internally to make comparison between objects
			// added to the fakeClient and expected objects easier.
			ResourceVersion: "999",
		},
		Spec: clusterv1.ClusterClassSpec{},
	}
	if f.infrastructureClusterTemplate != nil {
		obj.Spec.Infrastructure = clusterv1.LocalObjectTemplate{
			Ref: objToRef(f.infrastructureClusterTemplate),
		}
	}
	if f.controlPlaneTemplate != nil {
		obj.Spec.ControlPlane = clusterv1.ControlPlaneClass{
			LocalObjectTemplate: clusterv1.LocalObjectTemplate{
				Ref: objToRef(f.controlPlaneTemplate),
			},
		}
		if f.controlPlaneInfrastructureMachineTemplate != nil {
			obj.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
				Ref: objToRef(f.controlPlaneInfrastructureMachineTemplate),
			}
		}
	}
	if len(f.workerMachineDeploymentTemplates) > 0 {
		for class, mdt := range f.workerMachineDeploymentTemplates {
			obj.Spec.Workers.MachineDeployments = append(obj.Spec.Workers.MachineDeployments, clusterv1.MachineDeploymentClass{
				Class: class,
				Template: clusterv1.MachineDeploymentClassTemplate{
					Metadata: clusterv1.ObjectMeta{},
					Bootstrap: clusterv1.LocalObjectTemplate{
						Ref: objToRef(mdt.bootstrapTemplate),
					},
					Infrastructure: clusterv1.LocalObjectTemplate{
						Ref: objToRef(mdt.infrastructureMachineTemplate),
					},
				},
			})
		}
	}
	return obj
}

type fakeInfrastructureClusterTemplate struct {
	namespace string
	name      string
}

func newFakeInfrastructureClusterTemplate(namespace, name string) *fakeInfrastructureClusterTemplate {
	return &fakeInfrastructureClusterTemplate{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeInfrastructureClusterTemplate) Obj() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(fakeInfrastructureProviderGroupVersion.String())
	obj.SetKind("FakeInfrastructureClusterTemplate")
	obj.SetNamespace(f.namespace)
	obj.SetName(f.name)

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), true, "spec", "template", "spec", "fakeSetting"); err != nil {
		panic(err)
	}

	return obj
}

type fakeInfrastructureMachineTemplate struct {
	namespace string
	name      string
}

func newFakeInfrastructureMachineTemplate(namespace, name string) *fakeInfrastructureMachineTemplate {
	return &fakeInfrastructureMachineTemplate{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeInfrastructureMachineTemplate) Obj() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(fakeInfrastructureProviderGroupVersion.String())
	obj.SetKind("FakeInfrastructureMachineTemplate")
	obj.SetNamespace(f.namespace)
	obj.SetName(f.name)

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), true, "spec", "template", "spec", "fakeSetting"); err != nil {
		panic(err)
	}

	return obj
}

type fakeBootstrapTemplate struct {
	namespace string
	name      string
}

func newFakeBootstrapTemplate(namespace, name string) *fakeBootstrapTemplate {
	return &fakeBootstrapTemplate{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeBootstrapTemplate) Obj() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(fakeBootstrapProviderGroupVersion.String())
	obj.SetKind("FakeBoostrapTemplate")
	obj.SetNamespace(f.namespace)
	obj.SetName(f.name)
	// Nb. This is set to the same resourceVersion the fakeClient uses internally to make comparison between objects
	// added to the fakeClient and expected objects easier.
	obj.SetResourceVersion("999")

	return obj
}

type fakeControlPlaneTemplate struct {
	namespace                     string
	name                          string
	infrastructureMachineTemplate *unstructured.Unstructured
}

func newFakeControlPlaneTemplate(namespace, name string) *fakeControlPlaneTemplate {
	return &fakeControlPlaneTemplate{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeControlPlaneTemplate) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *fakeControlPlaneTemplate {
	f.infrastructureMachineTemplate = t
	return f
}

func (f *fakeControlPlaneTemplate) Obj() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(fakeControlPlaneProviderGroupVersion.String())
	obj.SetKind("FakeControlPlaneTemplate")
	obj.SetNamespace(f.namespace)
	obj.SetName(f.name)
	// Nb. This is set to the same resourceVersion the fakeClient uses internally to make comparison between objects
	// added to the fakeClient and expected objects easier.
	obj.SetResourceVersion("999")

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), true, "spec", "template", "spec", "fakeSetting"); err != nil {
		panic(err)
	}

	if f.infrastructureMachineTemplate != nil {
		if err := setNestedRef(obj, f.infrastructureMachineTemplate, "spec", "template", "spec", "machineTemplate", "infrastructureRef"); err != nil {
			panic(err)
		}
	}
	return obj
}

type fakeInfrastructureCluster struct {
	namespace string
	name      string
}

func newFakeInfrastructureCluster(namespace, name string) *fakeInfrastructureCluster { //nolint:deadcode
	return &fakeInfrastructureCluster{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeInfrastructureCluster) Obj() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(fakeControlPlaneProviderGroupVersion.String())
	obj.SetKind("FakeInfrastructureCluster")
	obj.SetNamespace(f.namespace)
	obj.SetName(f.name)
	// Nb. This is set to the same resourceVersion the fakeClient uses internally to make comparison between objects
	// added to the fakeClient and expected objects easier.
	obj.SetResourceVersion("999")

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), true, "spec", "fakeSetting"); err != nil {
		panic(err)
	}

	return obj
}

type fakeControlPlane struct {
	namespace                     string
	name                          string
	infrastructureMachineTemplate *unstructured.Unstructured
}

func newFakeControlPlane(namespace, name string) *fakeControlPlane {
	return &fakeControlPlane{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeControlPlane) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *fakeControlPlane {
	f.infrastructureMachineTemplate = t
	return f
}

func (f *fakeControlPlane) Obj() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(fakeControlPlaneProviderGroupVersion.String())
	obj.SetKind("FakeControlPlane")
	obj.SetNamespace(f.namespace)
	obj.SetName(f.name)
	// Nb. This is set to the same resourceVersion the fakeClient uses internally to make comparison between objects
	// added to the fakeClient and expected objects easier.
	obj.SetResourceVersion("999")

	if err := unstructured.SetNestedField(obj.UnstructuredContent(), true, "spec", "fakeSetting"); err != nil {
		panic(err)
	}

	if f.infrastructureMachineTemplate != nil {
		if err := setNestedRef(obj, f.infrastructureMachineTemplate, "spec", "machineTemplate", "infrastructureRef"); err != nil {
			panic(err)
		}
	}
	return obj
}
