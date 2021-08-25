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

package testtypes

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterBuilder holds the variables and objects required to build a clusterv1.Cluster.
type ClusterBuilder struct {
	namespace             string
	name                  string
	class                 clusterv1.ClusterClass
	infrastructureCluster *unstructured.Unstructured
	controlPlane          *unstructured.Unstructured
}

// NewClusterBuilder returns a ClusterBuilder with the given name and namespace.
func NewClusterBuilder(namespace, name string) *ClusterBuilder {
	return &ClusterBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithInfrastructureCluster adds the passed InfrastructureCluster to the ClusterBuilder.
func (c *ClusterBuilder) WithInfrastructureCluster(t *unstructured.Unstructured) *ClusterBuilder {
	c.infrastructureCluster = t
	return c
}

// WithControlPlane adds the passed ControlPlane to the ClusterBuilder.
func (c *ClusterBuilder) WithControlPlane(t *unstructured.Unstructured) *ClusterBuilder {
	c.controlPlane = t
	return c
}

// WithClusterClass adds the passed ClusterClass Name to the ClusterBuilder.
func (c *ClusterBuilder) WithClusterClass(class clusterv1.ClusterClass) *ClusterBuilder {
	c.class = class
	return c
}

// Build returns a Cluster with the attributes added to the ClusterBuilder.
func (c *ClusterBuilder) Build() *clusterv1.Cluster {
	obj := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.name,
			Namespace: c.namespace,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{Class: c.class.Name},
		},
	}
	if c.infrastructureCluster != nil {
		obj.Spec.InfrastructureRef = objToRef(c.infrastructureCluster)
	}
	if c.controlPlane != nil {
		obj.Spec.ControlPlaneRef = objToRef(c.controlPlane)
	}
	return obj
}

// ClusterClassBuilder holds the variables and objects required to build a clusterv1.ClusterClass.
type ClusterClassBuilder struct {
	namespace                                 string
	name                                      string
	infrastructureClusterTemplate             *unstructured.Unstructured
	controlPlaneMetadata                      *clusterv1.ObjectMeta
	controlPlaneTemplate                      *unstructured.Unstructured
	controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
	machineDeploymentClasses                  []clusterv1.MachineDeploymentClass
}

// NewClusterClassBuilder returns a ClusterClassBuilder with the given name and namespace.
func NewClusterClassBuilder(namespace, name string) *ClusterClassBuilder {
	return &ClusterClassBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithInfrastructureClusterTemplate adds the passed InfrastructureClusterTemplate to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithInfrastructureClusterTemplate(t *unstructured.Unstructured) *ClusterClassBuilder {
	c.infrastructureClusterTemplate = t
	return c
}

// WithControlPlaneTemplate adds the passed ControlPlaneTemplate to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneTemplate(t *unstructured.Unstructured) *ClusterClassBuilder {
	c.controlPlaneTemplate = t
	return c
}

// WithControlPlaneMetadata adds the given labels and annotations for use with the ControlPlane to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneMetadata(labels, annotations map[string]string) *ClusterClassBuilder {
	c.controlPlaneMetadata = &clusterv1.ObjectMeta{
		Labels:      labels,
		Annotations: annotations,
	}
	return c
}

// WithControlPlaneInfrastructureMachineTemplate adds the ControlPlane's InfrastructureMachineTemplate to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneInfrastructureMachineTemplate(t *unstructured.Unstructured) *ClusterClassBuilder {
	c.controlPlaneInfrastructureMachineTemplate = t
	return c
}

// WithWorkerMachineDeploymentClasses adds the variables and objects needed to create MachineDeploymentTemplates for a ClusterClassBuilder.
func (c *ClusterClassBuilder) WithWorkerMachineDeploymentClasses(mdcs []clusterv1.MachineDeploymentClass) *ClusterClassBuilder {
	if c.machineDeploymentClasses == nil {
		c.machineDeploymentClasses = make([]clusterv1.MachineDeploymentClass, 0)
	}
	c.machineDeploymentClasses = append(c.machineDeploymentClasses, mdcs...)
	return c
}

// Build takes the objects and variables in the ClusterClass builder and uses them to create a ClusterClass object.
func (c *ClusterClassBuilder) Build() *clusterv1.ClusterClass {
	obj := &clusterv1.ClusterClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterClass",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.name,
			Namespace: c.namespace,
		},
		Spec: clusterv1.ClusterClassSpec{},
	}
	if c.infrastructureClusterTemplate != nil {
		obj.Spec.Infrastructure = clusterv1.LocalObjectTemplate{
			Ref: objToRef(c.infrastructureClusterTemplate),
		}
	}
	if c.controlPlaneMetadata != nil {
		obj.Spec.ControlPlane.Metadata = *c.controlPlaneMetadata
	}
	if c.controlPlaneTemplate != nil {
		obj.Spec.ControlPlane.LocalObjectTemplate = clusterv1.LocalObjectTemplate{
			Ref: objToRef(c.controlPlaneTemplate),
		}
	}
	if c.controlPlaneInfrastructureMachineTemplate != nil {
		obj.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
			Ref: objToRef(c.controlPlaneInfrastructureMachineTemplate),
		}
	}
	obj.Spec.Workers.MachineDeployments = c.machineDeploymentClasses
	return obj
}

// MachineDeploymentClassBuilder holds the variables and objects required to build a clusterv1.MachineDeploymentClass.
type MachineDeploymentClassBuilder struct {
	namespace                     string
	name                          string
	class                         string
	infrastructureMachineTemplate *unstructured.Unstructured
	bootstrapTemplate             *unstructured.Unstructured
	labels                        map[string]string
	annotations                   map[string]string
}

// NewMachineDeploymentClassBuilder returns a MachineDeploymentClassBuilder with the given name and namespace.
func NewMachineDeploymentClassBuilder(namespace, name string) *MachineDeploymentClassBuilder {
	return &MachineDeploymentClassBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithInfrastructureTemplate registers the passed Unstructured object as the InfrastructureMachineTemplate for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithInfrastructureTemplate(t *unstructured.Unstructured) *MachineDeploymentClassBuilder {
	m.infrastructureMachineTemplate = t
	return m
}

// WithBootstrapTemplate registers the passed Unstructured object as the BootstrapTemplate for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithBootstrapTemplate(t *unstructured.Unstructured) *MachineDeploymentClassBuilder {
	m.bootstrapTemplate = t
	return m
}

// WithClass sets the MachineDeploymentClass name for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithClass(class string) *MachineDeploymentClassBuilder {
	m.class = class
	return m
}

// WithLabels sets the labels for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithLabels(labels map[string]string) *MachineDeploymentClassBuilder {
	m.labels = labels
	return m
}

// WithAnnotations sets the annotations for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithAnnotations(annotations map[string]string) *MachineDeploymentClassBuilder {
	m.annotations = annotations
	return m
}

// Build creates a full MachineDeploymentClass object with the variables passed to the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) Build() *clusterv1.MachineDeploymentClass {
	return &clusterv1.MachineDeploymentClass{
		Class: m.class,
		Template: clusterv1.MachineDeploymentClassTemplate{
			Metadata: clusterv1.ObjectMeta{
				Labels:      m.labels,
				Annotations: m.annotations,
			},
			Bootstrap: clusterv1.LocalObjectTemplate{
				Ref: objToRef(m.bootstrapTemplate),
			},
			Infrastructure: clusterv1.LocalObjectTemplate{
				Ref: objToRef(m.infrastructureMachineTemplate),
			},
		},
	}
}

// InfrastructureMachineTemplateBuilder holds the variables and objects needed to build an InfrastructureMachineTemplate.
type InfrastructureMachineTemplateBuilder struct {
	namespace  string
	name       string
	specFields map[string]interface{}
}

// NewInfrastructureMachineTemplateBuilder creates an InfrastructureMachineTemplateBuilder with the given name and namespace.
func NewInfrastructureMachineTemplateBuilder(namespace, name string) *InfrastructureMachineTemplateBuilder {
	return &InfrastructureMachineTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
func (i *InfrastructureMachineTemplateBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureMachineTemplateBuilder {
	i.specFields = fields
	return i
}

// Build takes the objects and variables in the  InfrastructureMachineTemplateBuilder and generates an unstructured object.
func (i *InfrastructureMachineTemplateBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureMachineKind)
	obj.SetNamespace(i.namespace)
	obj.SetName(i.name)

	setSpecFields(obj, i.specFields)
	return obj
}

// BootstrapTemplateBuilder holds the variables needed to build a generic BootstrapTemplate.
type BootstrapTemplateBuilder struct {
	namespace string
	name      string
}

// NewBootstrapTemplateBuilder creates a BootstrapTemplateBuilder with the given name and namespace.
func NewBootstrapTemplateBuilder(namespace, name string) *BootstrapTemplateBuilder {
	return &BootstrapTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// Build creates a new Unstructured object with the information passed to the BootstrapTemplateBuilder.
func (b *BootstrapTemplateBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(BootstrapGroupVersion.String())
	obj.SetKind(GenericBootstrapConfigTemplateKind)
	obj.SetNamespace(b.namespace)
	obj.SetName(b.name)

	return obj
}

// InfrastructureClusterTemplateBuilder holds the variables needed to build a generic InfrastructureClusterTemplate.
type InfrastructureClusterTemplateBuilder struct {
	namespace  string
	name       string
	specFields map[string]interface{}
}

// NewInfrastructureClusterTemplateBuilder returns an InfrastructureClusterTemplateBuilder with the given name and namespace.
func NewInfrastructureClusterTemplateBuilder(namespace, name string) *InfrastructureClusterTemplateBuilder {
	return &InfrastructureClusterTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
func (i *InfrastructureClusterTemplateBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureClusterTemplateBuilder {
	i.specFields = fields
	return i
}

// Build creates a new Unstructured object with the variables passed to the InfrastructureClusterTemplateBuilder.
func (i *InfrastructureClusterTemplateBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureClusterTemplateKind)
	obj.SetNamespace(i.namespace)
	obj.SetName(i.name)

	setSpecFields(obj, i.specFields)

	return obj
}

// ControlPlaneTemplateBuilder holds the variables and objects needed to build a generic ControlPlane template.
type ControlPlaneTemplateBuilder struct {
	namespace                     string
	name                          string
	infrastructureMachineTemplate *unstructured.Unstructured
	specFields                    map[string]interface{}
}

// NewControlPlaneTemplateBuilder creates a NewControlPlaneTemplate builder with the given name and namespace.
func NewControlPlaneTemplateBuilder(namespace, name string) *ControlPlaneTemplateBuilder {
	return &ControlPlaneTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
func (c *ControlPlaneTemplateBuilder) WithSpecFields(fields map[string]interface{}) *ControlPlaneTemplateBuilder {
	c.specFields = fields
	return c
}

// WithInfrastructureMachineTemplate adds the given Unstructured object to the ControlPlaneTemplateBuilder as its InfrastructureMachineTemplate.
func (c *ControlPlaneTemplateBuilder) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *ControlPlaneTemplateBuilder {
	c.infrastructureMachineTemplate = t
	return c
}

// Build creates an Unstructured object from the variables passed to the ControlPlaneTemplateBuilder.
func (c *ControlPlaneTemplateBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ControlPlaneGroupVersion.String())
	obj.SetKind(GenericControlPlaneTemplateKind)
	obj.SetNamespace(c.namespace)
	obj.SetName(c.name)

	setSpecFields(obj, c.specFields)

	if c.infrastructureMachineTemplate != nil {
		// TODO(killianmuldoon): Update to use the internal/contract package
		if err := setNestedRef(obj, c.infrastructureMachineTemplate, "spec", "template", "spec", "machineTemplate", "infrastructureRef"); err != nil {
			panic(err)
		}
	}
	return obj
}

// InfrastructureClusterBuilder holds the variables and objects needed to build a generic InfrastructureCluster.
type InfrastructureClusterBuilder struct {
	namespace  string
	name       string
	specFields map[string]interface{}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
func (i *InfrastructureClusterBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureClusterBuilder {
	i.specFields = fields
	return i
}

// NewInfrastructureClusterBuilder returns and InfrastructureClusterBuilder with the given name and namespace.
func NewInfrastructureClusterBuilder(namespace, name string) *InfrastructureClusterBuilder {
	return &InfrastructureClusterBuilder{
		namespace: namespace,
		name:      name,
	}
}

// Build returns an Unstructured object with the information passed to the InfrastructureClusterBuilder.
func (i *InfrastructureClusterBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureClusterKind)
	obj.SetNamespace(i.namespace)
	obj.SetName(i.name)

	setSpecFields(obj, i.specFields)
	return obj
}

// ControlPlaneBuilder holds the variables and objects needed to build a generic object for cluster.spec.controlPlaneRef.
type ControlPlaneBuilder struct {
	namespace                     string
	name                          string
	infrastructureMachineTemplate *unstructured.Unstructured
	specFields                    map[string]interface{}
}

// NewControlPlaneBuilder returns a ControlPlaneBuilder with the given name and Namespace.
func NewControlPlaneBuilder(namespace, name string) *ControlPlaneBuilder {
	return &ControlPlaneBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithInfrastructureMachineTemplate adds the given unstructured object to the ControlPlaneBuilder as its InfrastructureMachineTemplate.
func (f *ControlPlaneBuilder) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *ControlPlaneBuilder {
	f.infrastructureMachineTemplate = t
	return f
}

// Build generates an Unstructured object from the information passed to the ControlPlaneBuilder.
func (f *ControlPlaneBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ControlPlaneGroupVersion.String())
	obj.SetKind(GenericControlPlaneKind)
	obj.SetNamespace(f.namespace)
	obj.SetName(f.name)

	setSpecFields(obj, f.specFields)

	if f.infrastructureMachineTemplate != nil {
		// TODO(killianmuldoon): Update to use the internal/contract package
		if err := setNestedRef(obj, f.infrastructureMachineTemplate, "spec", "machineTemplate", "infrastructureRef"); err != nil {
			panic(err)
		}
	}
	return obj
}

// MachineDeploymentBuilder holds the variables and objects needed to build a generic MachineDeployment.
type MachineDeploymentBuilder struct {
	namespace              string
	name                   string
	bootstrapTemplate      *unstructured.Unstructured
	infrastructureTemplate *unstructured.Unstructured
	replicas               *int32
	labels                 map[string]string
}

// NewMachineDeploymentBuilder creates a MachineDeploymentBuilder with the given name and namespace.
func NewMachineDeploymentBuilder(namespace, name string) *MachineDeploymentBuilder {
	return &MachineDeploymentBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithBootstrapTemplate adds the passed Unstructured object to the MachineDeploymentBuilder as a bootstrapTemplate.
func (m *MachineDeploymentBuilder) WithBootstrapTemplate(ref *unstructured.Unstructured) *MachineDeploymentBuilder {
	m.bootstrapTemplate = ref
	return m
}

// WithInfrastructureTemplate adds the passed unstructured object to the MachineDeployment builder as an infrastructureMachineTemplate.
func (m *MachineDeploymentBuilder) WithInfrastructureTemplate(ref *unstructured.Unstructured) *MachineDeploymentBuilder {
	m.infrastructureTemplate = ref
	return m
}

// WithLabels adds the given labels to the MachineDeploymentBuilder.
func (m *MachineDeploymentBuilder) WithLabels(labels map[string]string) *MachineDeploymentBuilder {
	m.labels = labels
	return m
}

// WithReplicas sets the number of replicas for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentBuilder) WithReplicas(replicas *int32) *MachineDeploymentBuilder {
	m.replicas = replicas
	return m
}

// Build creates a new MachineDeployment with the variables and objects passed to the MachineDeploymentBuilder.
func (m *MachineDeploymentBuilder) Build() *clusterv1.MachineDeployment {
	obj := &clusterv1.MachineDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineDeployment",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.name,
			Namespace: m.namespace,
			Labels:    m.labels,
		},
	}
	obj.Spec.Replicas = m.replicas
	if m.bootstrapTemplate != nil {
		obj.Spec.Template.Spec.Bootstrap.ConfigRef = objToRef(m.bootstrapTemplate)
	}
	if m.infrastructureTemplate != nil {
		obj.Spec.Template.Spec.InfrastructureRef = *objToRef(m.infrastructureTemplate)
	}
	return obj
}

// objToRef returns a reference to the given object.
func objToRef(obj client.Object) *corev1.ObjectReference {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return &corev1.ObjectReference{
		Kind:       gvk.Kind,
		APIVersion: gvk.GroupVersion().String(),
		Namespace:  obj.GetNamespace(),
		Name:       obj.GetName(),
	}
}

// setNestedRef sets the value of a nested field to a reference to the refObj provided.
func setNestedRef(obj, refObj *unstructured.Unstructured, fields ...string) error {
	ref := map[string]interface{}{
		"kind":       refObj.GetKind(),
		"namespace":  refObj.GetNamespace(),
		"name":       refObj.GetName(),
		"apiVersion": refObj.GetAPIVersion(),
	}
	return unstructured.SetNestedField(obj.UnstructuredContent(), ref, fields...)
}

// setSpecFields sets fields in an unstructured object from a map.
func setSpecFields(obj *unstructured.Unstructured, fields map[string]interface{}) {
	for k, v := range fields {
		fieldParts := strings.Split(k, ".")
		if len(fieldParts) == 0 {
			panic(fmt.Errorf("fieldParts invalid"))
		}
		if fieldParts[0] != "spec" {
			panic(fmt.Errorf("can not set fields outside spec"))
		}
		if err := unstructured.SetNestedField(obj.UnstructuredContent(), v, strings.Split(k, ".")...); err != nil {
			panic(err)
		}
	}
}
