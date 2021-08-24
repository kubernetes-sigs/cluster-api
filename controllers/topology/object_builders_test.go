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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	"sigs.k8s.io/cluster-api/internal/testtypes"
)

// TODO: move under internal/testtypes

var (
	fakeInfrastructureProviderGroupVersion = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1alpha4"}
	fakeControlPlaneProviderGroupVersion   = schema.GroupVersion{Group: "controlplane.cluster.x-k8s.io", Version: "v1alpha4"}
	fakeBootstrapProviderGroupVersion      = schema.GroupVersion{Group: "bootstrap.cluster.x-k8s.io", Version: "v1alpha4"}
	fakeInfrastuctureClusterCRD            = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "genericinfrastructurecluster.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: fakeInfrastructureProviderGroupVersion.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "GenericInfrastructureCluster",
			},
		},
	}
	fakeControlPlaneCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "fakecontrolplane.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				"cluster.x-k8s.io/v1alpha4": "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: fakeControlPlaneProviderGroupVersion.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: "FakeControlPlane",
			},
		},
	}
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
	namespace             string
	name                  string
	class                 clusterv1.ClusterClass
	infrastructureCluster *unstructured.Unstructured
	controlPlane          *unstructured.Unstructured
}

func newFakeCluster(namespace, name string) *fakeCluster {
	return &fakeCluster{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeCluster) WithInfrastructureCluster(t *unstructured.Unstructured) *fakeCluster {
	f.infrastructureCluster = t
	return f
}

func (f *fakeCluster) WithControlPlane(t *unstructured.Unstructured) *fakeCluster {
	f.controlPlane = t
	return f
}

func (f *fakeCluster) WithClass(class clusterv1.ClusterClass) *fakeCluster {
	f.class = class
	return f
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
			Topology: &clusterv1.Topology{Class: f.class.Name},
		},
	}
	if f.infrastructureCluster != nil {
		obj.Spec.InfrastructureRef = contract.ObjToRef(f.infrastructureCluster)
	}
	if f.controlPlane != nil {
		obj.Spec.ControlPlaneRef = contract.ObjToRef(f.controlPlane)
	}
	return obj
}

type fakeClusterClass struct {
	namespace                                 string
	name                                      string
	infrastructureClusterTemplate             *unstructured.Unstructured
	controlPlaneMetadata                      *clusterv1.ObjectMeta
	controlPlaneTemplate                      *unstructured.Unstructured
	controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
	workerMachineDeploymentTemplates          map[string]fakeClusterClassMachineDeploymentTemplates
}

type fakeClusterClassMachineDeploymentTemplates struct {
	clusterv1.ObjectMeta
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

func (f *fakeClusterClass) WithControlPlaneMetadata(labels, annotations map[string]string) *fakeClusterClass {
	f.controlPlaneMetadata = &clusterv1.ObjectMeta{
		Labels:      labels,
		Annotations: annotations,
	}
	return f
}

func (f *fakeClusterClass) WithControlPlaneInfrastructureMachineTemplate(t *unstructured.Unstructured) *fakeClusterClass {
	f.controlPlaneInfrastructureMachineTemplate = t
	return f
}

func (f *fakeClusterClass) WithWorkerMachineDeploymentClass(class string, labels, annotations map[string]string, infrastructureMachineTemplate, bootstrapTemplate *unstructured.Unstructured) *fakeClusterClass {
	if f.workerMachineDeploymentTemplates == nil {
		f.workerMachineDeploymentTemplates = map[string]fakeClusterClassMachineDeploymentTemplates{}
	}
	f.workerMachineDeploymentTemplates[class] = fakeClusterClassMachineDeploymentTemplates{
		ObjectMeta: clusterv1.ObjectMeta{
			Labels:      labels,
			Annotations: annotations,
		},
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
			Ref: contract.ObjToRef(f.infrastructureClusterTemplate),
		}
	}
	if f.controlPlaneMetadata != nil {
		obj.Spec.ControlPlane.Metadata = *f.controlPlaneMetadata
	}
	if f.controlPlaneTemplate != nil {
		obj.Spec.ControlPlane.LocalObjectTemplate = clusterv1.LocalObjectTemplate{
			Ref: contract.ObjToRef(f.controlPlaneTemplate),
		}
	}
	if f.controlPlaneInfrastructureMachineTemplate != nil {
		obj.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
			Ref: contract.ObjToRef(f.controlPlaneInfrastructureMachineTemplate),
		}
	}
	if len(f.workerMachineDeploymentTemplates) > 0 {
		for class, mdt := range f.workerMachineDeploymentTemplates {
			obj.Spec.Workers.MachineDeployments = append(obj.Spec.Workers.MachineDeployments, clusterv1.MachineDeploymentClass{
				Class: class,
				Template: clusterv1.MachineDeploymentClassTemplate{
					Metadata: clusterv1.ObjectMeta{
						Labels:      mdt.Labels,
						Annotations: mdt.Annotations,
					},
					Bootstrap: clusterv1.LocalObjectTemplate{
						Ref: contract.ObjToRef(mdt.bootstrapTemplate),
					},
					Infrastructure: clusterv1.LocalObjectTemplate{
						Ref: contract.ObjToRef(mdt.infrastructureMachineTemplate),
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
		if err := contract.ControlPlaneTemplate().InfrastructureMachineTemplate().Set(obj, f.infrastructureMachineTemplate); err != nil {
			panic(err)
		}
	}
	return obj
}

type fakeInfrastructureCluster struct {
	namespace string
	name      string
}

func newFakeInfrastructureCluster(namespace, name string) *fakeInfrastructureCluster {
	return &fakeInfrastructureCluster{
		namespace: namespace,
		name:      name,
	}
}

func (f *fakeInfrastructureCluster) Obj() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(testtypes.InfrastructureGroupVersion.String())
	obj.SetKind("GenericInfrastructureCluster")
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
		if err := contract.ControlPlane().InfrastructureMachineTemplate().Set(obj, f.infrastructureMachineTemplate); err != nil {
			panic(err)
		}
	}
	return obj
}

type fakeMachineDeployment struct {
	namespace              string
	name                   string
	bootstrapTemplate      *unstructured.Unstructured
	infrastructureTemplate *unstructured.Unstructured
	labels                 map[string]string
}

func newFakeMachineDeployment(namespace, name string) *fakeMachineDeployment {
	return &fakeMachineDeployment{
		name:      name,
		namespace: namespace,
	}
}

func (f *fakeMachineDeployment) WithBootstrapTemplate(ref *unstructured.Unstructured) *fakeMachineDeployment {
	f.bootstrapTemplate = ref
	return f
}

func (f *fakeMachineDeployment) WithInfrastructureTemplate(ref *unstructured.Unstructured) *fakeMachineDeployment {
	f.infrastructureTemplate = ref
	return f
}

func (f *fakeMachineDeployment) WithLabels(labels map[string]string) *fakeMachineDeployment {
	f.labels = labels
	return f
}

func (f *fakeMachineDeployment) Obj() *clusterv1.MachineDeployment {
	obj := &clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.name,
			Namespace: f.namespace,
			Labels:    f.labels,
		},
	}
	if f.bootstrapTemplate != nil {
		obj.Spec.Template.Spec.Bootstrap.ConfigRef = contract.ObjToRef(f.bootstrapTemplate)
	}
	if f.infrastructureTemplate != nil {
		obj.Spec.Template.Spec.InfrastructureRef = *contract.ObjToRef(f.infrastructureTemplate)
	}
	return obj
}
