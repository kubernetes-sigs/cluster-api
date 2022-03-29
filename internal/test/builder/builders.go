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

package builder

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

// ClusterBuilder holds the variables and objects required to build a clusterv1.Cluster.
type ClusterBuilder struct {
	namespace             string
	name                  string
	labels                map[string]string
	annotations           map[string]string
	topology              *clusterv1.Topology
	infrastructureCluster *unstructured.Unstructured
	controlPlane          *unstructured.Unstructured
}

// Cluster returns a ClusterBuilder with the given name and namespace.
func Cluster(namespace, name string) *ClusterBuilder {
	return &ClusterBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithLabels sets the labels for the ClusterBuilder.
func (c *ClusterBuilder) WithLabels(labels map[string]string) *ClusterBuilder {
	c.labels = labels
	return c
}

// WithAnnotations sets the annotations for the ClusterBuilder.
func (c *ClusterBuilder) WithAnnotations(annotations map[string]string) *ClusterBuilder {
	c.annotations = annotations
	return c
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

// WithTopology adds the passed Topology object to the ClusterBuilder.
func (c *ClusterBuilder) WithTopology(topology *clusterv1.Topology) *ClusterBuilder {
	c.topology = topology
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
			Name:        c.name,
			Namespace:   c.namespace,
			Labels:      c.labels,
			Annotations: c.annotations,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: c.topology,
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

// ClusterTopologyBuilder contains the fields needed to build a testable ClusterTopology.
type ClusterTopologyBuilder struct {
	class                string
	workers              *clusterv1.WorkersTopology
	version              string
	controlPlaneReplicas int32
	variables            []clusterv1.ClusterVariable
}

// ClusterTopology returns a ClusterTopologyBuilder.
func ClusterTopology() *ClusterTopologyBuilder {
	return &ClusterTopologyBuilder{
		workers: &clusterv1.WorkersTopology{},
	}
}

// WithClass adds the passed ClusterClass name to the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithClass(class string) *ClusterTopologyBuilder {
	c.class = class
	return c
}

// WithVersion adds the passed version to the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithVersion(version string) *ClusterTopologyBuilder {
	c.version = version
	return c
}

// WithControlPlaneReplicas adds the passed replicas value to the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithControlPlaneReplicas(replicas int32) *ClusterTopologyBuilder {
	c.controlPlaneReplicas = replicas
	return c
}

// WithMachineDeployment passes the full MachineDeploymentTopology and adds it to an existing list in the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithMachineDeployment(mdc clusterv1.MachineDeploymentTopology) *ClusterTopologyBuilder {
	c.workers.MachineDeployments = append(c.workers.MachineDeployments, mdc)
	return c
}

// WithVariables adds the passed variables to the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithVariables(vars ...clusterv1.ClusterVariable) *ClusterTopologyBuilder {
	c.variables = vars
	return c
}

// Build returns a testable cluster Topology object with any values passed to the builder.
func (c *ClusterTopologyBuilder) Build() *clusterv1.Topology {
	return &clusterv1.Topology{
		Class:   c.class,
		Workers: c.workers,
		Version: c.version,
		ControlPlane: clusterv1.ControlPlaneTopology{
			Replicas: &c.controlPlaneReplicas,
		},
		Variables: c.variables,
	}
}

// MachineDeploymentTopologyBuilder holds the values needed to create a testable MachineDeploymentTopology.
type MachineDeploymentTopologyBuilder struct {
	class     string
	name      string
	replicas  *int32
	variables []clusterv1.ClusterVariable
}

// MachineDeploymentTopology returns a builder used to create a testable MachineDeploymentTopology.
func MachineDeploymentTopology(name string) *MachineDeploymentTopologyBuilder {
	return &MachineDeploymentTopologyBuilder{
		name: name,
	}
}

// WithClass adds a class string used as the MachineDeploymentTopology class.
func (m *MachineDeploymentTopologyBuilder) WithClass(class string) *MachineDeploymentTopologyBuilder {
	m.class = class
	return m
}

// WithReplicas adds a replicas value used as the MachineDeploymentTopology replicas value.
func (m *MachineDeploymentTopologyBuilder) WithReplicas(replicas int32) *MachineDeploymentTopologyBuilder {
	m.replicas = &replicas
	return m
}

// WithVariables adds variables used as the MachineDeploymentTopology variables value.
func (m *MachineDeploymentTopologyBuilder) WithVariables(variables ...clusterv1.ClusterVariable) *MachineDeploymentTopologyBuilder {
	m.variables = variables
	return m
}

// Build returns a testable MachineDeploymentTopology with any values passed to the builder.
func (m *MachineDeploymentTopologyBuilder) Build() clusterv1.MachineDeploymentTopology {
	md := clusterv1.MachineDeploymentTopology{
		Class:    m.class,
		Name:     m.name,
		Replicas: m.replicas,
	}

	if len(m.variables) > 0 {
		md.Variables = &clusterv1.MachineDeploymentVariables{
			Overrides: m.variables,
		}
	}

	return md
}

// ClusterClassBuilder holds the variables and objects required to build a clusterv1.ClusterClass.
type ClusterClassBuilder struct {
	namespace                                 string
	name                                      string
	infrastructureClusterTemplate             *unstructured.Unstructured
	controlPlaneMetadata                      *clusterv1.ObjectMeta
	controlPlaneTemplate                      *unstructured.Unstructured
	controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
	controlPlaneMHC                           *clusterv1.MachineHealthCheckClass
	machineDeploymentClasses                  []clusterv1.MachineDeploymentClass
	variables                                 []clusterv1.ClusterClassVariable
	patches                                   []clusterv1.ClusterClassPatch
}

// ClusterClass returns a ClusterClassBuilder with the given name and namespace.
func ClusterClass(namespace, name string) *ClusterClassBuilder {
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

// WithControlPlaneMachineHealthCheck adds a MachineHealthCheck for the ControlPlane to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneMachineHealthCheck(mhc *clusterv1.MachineHealthCheckClass) *ClusterClassBuilder {
	c.controlPlaneMHC = mhc
	return c
}

// WithVariables adds the Variables the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithVariables(vars ...clusterv1.ClusterClassVariable) *ClusterClassBuilder {
	c.variables = vars
	return c
}

// WithPatches adds the patches to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithPatches(patches []clusterv1.ClusterClassPatch) *ClusterClassBuilder {
	c.patches = patches
	return c
}

// WithWorkerMachineDeploymentClasses adds the variables and objects needed to create MachineDeploymentTemplates for a ClusterClassBuilder.
func (c *ClusterClassBuilder) WithWorkerMachineDeploymentClasses(mdcs ...clusterv1.MachineDeploymentClass) *ClusterClassBuilder {
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
		Spec: clusterv1.ClusterClassSpec{
			Variables: c.variables,
			Patches:   c.patches,
		},
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
	if c.controlPlaneMHC != nil {
		obj.Spec.ControlPlane.MachineHealthCheck = c.controlPlaneMHC
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
	class                         string
	infrastructureMachineTemplate *unstructured.Unstructured
	bootstrapTemplate             *unstructured.Unstructured
	labels                        map[string]string
	annotations                   map[string]string
	machineHealthCheckClass       *clusterv1.MachineHealthCheckClass
}

// MachineDeploymentClass returns a MachineDeploymentClassBuilder with the given name and namespace.
func MachineDeploymentClass(class string) *MachineDeploymentClassBuilder {
	return &MachineDeploymentClassBuilder{
		class: class,
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

// WithMachineHealthCheckClass sets the MachineHealthCheckClass for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithMachineHealthCheckClass(mhc *clusterv1.MachineHealthCheckClass) *MachineDeploymentClassBuilder {
	m.machineHealthCheckClass = mhc
	return m
}

// Build creates a full MachineDeploymentClass object with the variables passed to the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) Build() *clusterv1.MachineDeploymentClass {
	obj := &clusterv1.MachineDeploymentClass{
		Class: m.class,
		Template: clusterv1.MachineDeploymentClassTemplate{
			Metadata: clusterv1.ObjectMeta{
				Labels:      m.labels,
				Annotations: m.annotations,
			},
		},
	}
	if m.bootstrapTemplate != nil {
		obj.Template.Bootstrap.Ref = objToRef(m.bootstrapTemplate)
	}
	if m.infrastructureMachineTemplate != nil {
		obj.Template.Infrastructure.Ref = objToRef(m.infrastructureMachineTemplate)
	}
	if m.machineHealthCheckClass != nil {
		obj.MachineHealthCheck = m.machineHealthCheckClass
	}
	return obj
}

// InfrastructureMachineTemplateBuilder holds the variables and objects needed to build an InfrastructureMachineTemplate.
// +kubebuilder:object:generate=false
type InfrastructureMachineTemplateBuilder struct {
	namespace  string
	name       string
	specFields map[string]interface{}
}

// InfrastructureMachineTemplate creates an InfrastructureMachineTemplateBuilder with the given name and namespace.
func InfrastructureMachineTemplate(namespace, name string) *InfrastructureMachineTemplateBuilder {
	return &InfrastructureMachineTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
// Example map: map[string]interface{}{
//     "spec.version": "v1.2.3",
// }.
func (i *InfrastructureMachineTemplateBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureMachineTemplateBuilder {
	i.specFields = fields
	return i
}

// Build takes the objects and variables in the  InfrastructureMachineTemplateBuilder and generates an unstructured object.
func (i *InfrastructureMachineTemplateBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureMachineTemplateKind)
	obj.SetNamespace(i.namespace)
	obj.SetName(i.name)

	// Initialize the spec.template.spec to make the object valid in reconciliation.
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})

	setSpecFields(obj, i.specFields)
	return obj
}

// BootstrapTemplateBuilder holds the variables needed to build a generic BootstrapTemplate.
// +kubebuilder:object:generate=false
type BootstrapTemplateBuilder struct {
	namespace  string
	name       string
	specFields map[string]interface{}
}

// BootstrapTemplate creates a BootstrapTemplateBuilder with the given name and namespace.
func BootstrapTemplate(namespace, name string) *BootstrapTemplateBuilder {
	return &BootstrapTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
func (b *BootstrapTemplateBuilder) WithSpecFields(fields map[string]interface{}) *BootstrapTemplateBuilder {
	b.specFields = fields
	return b
}

// Build creates a new Unstructured object with the information passed to the BootstrapTemplateBuilder.
func (b *BootstrapTemplateBuilder) Build() *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(BootstrapGroupVersion.String())
	obj.SetKind(GenericBootstrapConfigTemplateKind)
	obj.SetNamespace(b.namespace)
	obj.SetName(b.name)
	setSpecFields(obj, b.specFields)

	// Initialize the spec.template.spec to make the object valid in reconciliation.
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})

	return obj
}

// InfrastructureClusterTemplateBuilder holds the variables needed to build a generic InfrastructureClusterTemplate.
// +kubebuilder:object:generate=false
type InfrastructureClusterTemplateBuilder struct {
	namespace  string
	name       string
	specFields map[string]interface{}
}

// InfrastructureClusterTemplate returns an InfrastructureClusterTemplateBuilder with the given name and namespace.
func InfrastructureClusterTemplate(namespace, name string) *InfrastructureClusterTemplateBuilder {
	return &InfrastructureClusterTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
// Example map: map[string]interface{}{
//     "spec.version": "v1.2.3",
// }.
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

	// Initialize the spec.template.spec to make the object valid in reconciliation.
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})

	setSpecFields(obj, i.specFields)

	return obj
}

// ControlPlaneTemplateBuilder holds the variables and objects needed to build a generic ControlPlane template.
// +kubebuilder:object:generate=false
type ControlPlaneTemplateBuilder struct {
	namespace                     string
	name                          string
	infrastructureMachineTemplate *unstructured.Unstructured
	specFields                    map[string]interface{}
}

// ControlPlaneTemplate creates a NewControlPlaneTemplate builder with the given name and namespace.
func ControlPlaneTemplate(namespace, name string) *ControlPlaneTemplateBuilder {
	return &ControlPlaneTemplateBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
// Example map: map[string]interface{}{
//     "spec.version": "v1.2.3",
// }.
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

	// Initialize the spec.template.spec to make the object valid in reconciliation.
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})

	setSpecFields(obj, c.specFields)

	if c.infrastructureMachineTemplate != nil {
		if err := setNestedRef(obj, c.infrastructureMachineTemplate, "spec", "template", "spec", "machineTemplate", "infrastructureRef"); err != nil {
			panic(err)
		}
	}
	return obj
}

// InfrastructureClusterBuilder holds the variables and objects needed to build a generic InfrastructureCluster.
// +kubebuilder:object:generate=false
type InfrastructureClusterBuilder struct {
	namespace  string
	name       string
	specFields map[string]interface{}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
// Example map: map[string]interface{}{
//     "spec.version": "v1.2.3",
// }.
func (i *InfrastructureClusterBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureClusterBuilder {
	i.specFields = fields
	return i
}

// InfrastructureCluster returns and InfrastructureClusterBuilder with the given name and namespace.
func InfrastructureCluster(namespace, name string) *InfrastructureClusterBuilder {
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
// +kubebuilder:object:generate=false
type ControlPlaneBuilder struct {
	namespace                     string
	name                          string
	infrastructureMachineTemplate *unstructured.Unstructured
	replicas                      *int64
	version                       *string
	specFields                    map[string]interface{}
	statusFields                  map[string]interface{}
}

// ControlPlane returns a ControlPlaneBuilder with the given name and Namespace.
func ControlPlane(namespace, name string) *ControlPlaneBuilder {
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

// WithReplicas sets the number of replicas for the ControlPlaneBuilder.
func (f *ControlPlaneBuilder) WithReplicas(replicas int64) *ControlPlaneBuilder {
	f.replicas = &replicas
	return f
}

// WithVersion adds the passed version to the ControlPlaneBuilder.
func (f *ControlPlaneBuilder) WithVersion(version string) *ControlPlaneBuilder {
	f.version = &version
	return f
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
// Example map: map[string]interface{}{
//     "spec.version": "v1.2.3",
// }.
func (f *ControlPlaneBuilder) WithSpecFields(m map[string]interface{}) *ControlPlaneBuilder {
	f.specFields = m
	return f
}

// WithStatusFields sets a map of status fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the status field.
//
// Note: all the paths should start with "status."
//
// Example map: map[string]interface{}{
//     "status.version": "v1.2.3",
// }.
func (f *ControlPlaneBuilder) WithStatusFields(m map[string]interface{}) *ControlPlaneBuilder {
	f.statusFields = m
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
	setStatusFields(obj, f.statusFields)

	// TODO(killianmuldoon): Update to use the internal/contract package, when it is importable from here
	if f.infrastructureMachineTemplate != nil {
		if err := setNestedRef(obj, f.infrastructureMachineTemplate, "spec", "machineTemplate", "infrastructureRef"); err != nil {
			panic(err)
		}
	}
	if f.replicas != nil {
		if err := unstructured.SetNestedField(obj.UnstructuredContent(), *f.replicas, "spec", "replicas"); err != nil {
			panic(err)
		}
	}
	if f.version != nil {
		if err := unstructured.SetNestedField(obj.UnstructuredContent(), *f.version, "spec", "version"); err != nil {
			panic(err)
		}
	}

	return obj
}

// MachinePoolBuilder holds the variables and objects needed to build a generic MachinePool.
type MachinePoolBuilder struct {
	namespace              string
	name                   string
	bootstrapTemplate      *unstructured.Unstructured
	infrastructureTemplate *unstructured.Unstructured
	version                *string
	clusterName            string
	replicas               *int32
	labels                 map[string]string
	status                 *expv1.MachinePoolStatus
}

// MachinePool creates a MachinePoolBuilder with the given name and namespace.
func MachinePool(namespace, name string) *MachinePoolBuilder {
	return &MachinePoolBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithBootstrapTemplate adds the passed Unstructured object to the MachinePoolBuilder as a bootstrapTemplate.
func (m *MachinePoolBuilder) WithBootstrapTemplate(ref *unstructured.Unstructured) *MachinePoolBuilder {
	m.bootstrapTemplate = ref
	return m
}

// WithInfrastructureTemplate adds the passed unstructured object to the MachinePool builder as an infrastructureMachineTemplate.
func (m *MachinePoolBuilder) WithInfrastructureTemplate(ref *unstructured.Unstructured) *MachinePoolBuilder {
	m.infrastructureTemplate = ref
	return m
}

// WithLabels adds the given labels to the MachinePoolBuilder.
func (m *MachinePoolBuilder) WithLabels(labels map[string]string) *MachinePoolBuilder {
	m.labels = labels
	return m
}

// WithVersion sets the passed version on the MachinePool spec.
func (m *MachinePoolBuilder) WithVersion(version string) *MachinePoolBuilder {
	m.version = &version
	return m
}

// WithClusterName sets the passed clusterName on the MachinePool spec.
func (m *MachinePoolBuilder) WithClusterName(clusterName string) *MachinePoolBuilder {
	m.clusterName = clusterName
	return m
}

// WithReplicas sets the number of replicas for the MachinePoolBuilder.
func (m *MachinePoolBuilder) WithReplicas(replicas int32) *MachinePoolBuilder {
	m.replicas = &replicas
	return m
}

// WithStatus sets the passed status object as the status of the MachinePool object.
func (m *MachinePoolBuilder) WithStatus(status expv1.MachinePoolStatus) *MachinePoolBuilder {
	m.status = &status
	return m
}

// Build creates a new MachinePool with the variables and objects passed to the MachinePoolBuilder.
func (m *MachinePoolBuilder) Build() *expv1.MachinePool {
	obj := &expv1.MachinePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachinePool",
			APIVersion: expv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.name,
			Namespace: m.namespace,
			Labels:    m.labels,
		},
		Spec: expv1.MachinePoolSpec{
			ClusterName: m.clusterName,
			Replicas:    m.replicas,
		},
	}
	if m.bootstrapTemplate != nil {
		obj.Spec.Template.Spec.Bootstrap.ConfigRef = objToRef(m.bootstrapTemplate)
	}
	if m.infrastructureTemplate != nil {
		obj.Spec.Template.Spec.InfrastructureRef = *objToRef(m.infrastructureTemplate)
	}
	if m.version != nil {
		obj.Spec.Template.Spec.Version = m.version
	}
	if m.status != nil {
		obj.Status = *m.status
	}
	return obj
}

// MachineDeploymentBuilder holds the variables and objects needed to build a generic MachineDeployment.
type MachineDeploymentBuilder struct {
	namespace              string
	name                   string
	bootstrapTemplate      *unstructured.Unstructured
	infrastructureTemplate *unstructured.Unstructured
	version                *string
	replicas               *int32
	generation             *int64
	labels                 map[string]string
	status                 *clusterv1.MachineDeploymentStatus
}

// MachineDeployment creates a MachineDeploymentBuilder with the given name and namespace.
func MachineDeployment(namespace, name string) *MachineDeploymentBuilder {
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

// WithVersion sets the passed version on the machine deployment spec.
func (m *MachineDeploymentBuilder) WithVersion(version string) *MachineDeploymentBuilder {
	m.version = &version
	return m
}

// WithReplicas sets the number of replicas for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentBuilder) WithReplicas(replicas int32) *MachineDeploymentBuilder {
	m.replicas = &replicas
	return m
}

// WithGeneration sets the passed value on the machine deployments object metadata.
func (m *MachineDeploymentBuilder) WithGeneration(generation int64) *MachineDeploymentBuilder {
	m.generation = &generation
	return m
}

// WithStatus sets the passed status object as the status of the machine deployment object.
func (m *MachineDeploymentBuilder) WithStatus(status clusterv1.MachineDeploymentStatus) *MachineDeploymentBuilder {
	m.status = &status
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
	if m.generation != nil {
		obj.Generation = *m.generation
	}
	if m.version != nil {
		obj.Spec.Template.Spec.Version = m.version
	}
	obj.Spec.Replicas = m.replicas
	if m.bootstrapTemplate != nil {
		obj.Spec.Template.Spec.Bootstrap.ConfigRef = objToRef(m.bootstrapTemplate)
	}
	if m.infrastructureTemplate != nil {
		obj.Spec.Template.Spec.InfrastructureRef = *objToRef(m.infrastructureTemplate)
	}
	if m.status != nil {
		obj.Status = *m.status
	}
	return obj
}

// MachineSetBuilder holds the variables and objects needed to build a generic MachineSet.
type MachineSetBuilder struct {
	namespace              string
	name                   string
	bootstrapTemplate      *unstructured.Unstructured
	infrastructureTemplate *unstructured.Unstructured
	replicas               *int32
	labels                 map[string]string
}

// MachineSet creates a MachineSetBuilder with the given name and namespace.
func MachineSet(namespace, name string) *MachineSetBuilder {
	return &MachineSetBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithBootstrapTemplate adds the passed Unstructured object to the MachineSetBuilder as a bootstrapTemplate.
func (m *MachineSetBuilder) WithBootstrapTemplate(ref *unstructured.Unstructured) *MachineSetBuilder {
	m.bootstrapTemplate = ref
	return m
}

// WithInfrastructureTemplate adds the passed unstructured object to the MachineSet builder as an infrastructureMachineTemplate.
func (m *MachineSetBuilder) WithInfrastructureTemplate(ref *unstructured.Unstructured) *MachineSetBuilder {
	m.infrastructureTemplate = ref
	return m
}

// WithLabels adds the given labels to the MachineSetBuilder.
func (m *MachineSetBuilder) WithLabels(labels map[string]string) *MachineSetBuilder {
	m.labels = labels
	return m
}

// WithReplicas sets the number of replicas for the MachineSetClassBuilder.
func (m *MachineSetBuilder) WithReplicas(replicas *int32) *MachineSetBuilder {
	m.replicas = replicas
	return m
}

// Build creates a new MachineSet with the variables and objects passed to the MachineSetBuilder.
func (m *MachineSetBuilder) Build() *clusterv1.MachineSet {
	obj := &clusterv1.MachineSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineSet",
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

// MachineBuilder holds the variables required to build a Machine.
type MachineBuilder struct {
	name        string
	namespace   string
	version     *string
	clusterName string
	bootstrap   *unstructured.Unstructured
	labels      map[string]string
}

// Machine returns a MachineBuilder.
func Machine(namespace, name string) *MachineBuilder {
	return &MachineBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithVersion adds a version to the MachineBuilder.
func (m *MachineBuilder) WithVersion(version string) *MachineBuilder {
	m.version = &version
	return m
}

// WithBootstrapTemplate adds a bootstrap template to the MachineBuilder.
func (m *MachineBuilder) WithBootstrapTemplate(bootstrap *unstructured.Unstructured) *MachineBuilder {
	m.bootstrap = bootstrap
	return m
}

// WithClusterName adds a clusterName to the MachineBuilder.
func (m *MachineBuilder) WithClusterName(clusterName string) *MachineBuilder {
	m.clusterName = clusterName
	return m
}

// WithLabels adds the given labels to the MachineSetBuilder.
func (m *MachineBuilder) WithLabels(labels map[string]string) *MachineBuilder {
	m.labels = labels
	return m
}

// Build produces a Machine object from the information passed to the MachineBuilder.
func (m *MachineBuilder) Build() *clusterv1.Machine {
	machine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: m.namespace,
			Name:      m.name,
			Labels:    m.labels,
		},
		Spec: clusterv1.MachineSpec{
			Version:     m.version,
			ClusterName: m.clusterName,
		},
	}
	if m.bootstrap != nil {
		machine.Spec.Bootstrap.ConfigRef = objToRef(m.bootstrap)
	}
	if m.clusterName != "" {
		if len(m.labels) == 0 {
			machine.Labels = map[string]string{}
		}
		machine.ObjectMeta.Labels[clusterv1.ClusterLabelName] = m.clusterName
	}
	return machine
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

// setStatusFields sets fields in an unstructured object from a map.
func setStatusFields(obj *unstructured.Unstructured, fields map[string]interface{}) {
	for k, v := range fields {
		fieldParts := strings.Split(k, ".")
		if len(fieldParts) == 0 {
			panic(fmt.Errorf("fieldParts invalid"))
		}
		if fieldParts[0] != "status" {
			panic(fmt.Errorf("can not set fields outside status"))
		}
		if err := unstructured.SetNestedField(obj.UnstructuredContent(), v, strings.Split(k, ".")...); err != nil {
			panic(err)
		}
	}
}

// MachineHealthCheckBuilder holds fields for creating a MachineHealthCheck.
type MachineHealthCheckBuilder struct {
	name         string
	namespace    string
	ownerRefs    []metav1.OwnerReference
	selector     metav1.LabelSelector
	clusterName  string
	conditions   []clusterv1.UnhealthyCondition
	maxUnhealthy *intstr.IntOrString
}

// MachineHealthCheck returns a MachineHealthCheckBuilder with the given name and namespace.
func MachineHealthCheck(namespace, name string) *MachineHealthCheckBuilder {
	return &MachineHealthCheckBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithSelector adds the selector used to target machines for the MachineHealthCheck.
func (m *MachineHealthCheckBuilder) WithSelector(selector metav1.LabelSelector) *MachineHealthCheckBuilder {
	m.selector = selector
	return m
}

// WithClusterName adds a cluster name for the MachineHealthCheck.
func (m *MachineHealthCheckBuilder) WithClusterName(clusterName string) *MachineHealthCheckBuilder {
	m.clusterName = clusterName
	return m
}

// WithUnhealthyConditions adds the spec used to build the parameters of the MachineHealthCheck.
func (m *MachineHealthCheckBuilder) WithUnhealthyConditions(conditions []clusterv1.UnhealthyCondition) *MachineHealthCheckBuilder {
	m.conditions = conditions
	return m
}

// WithOwnerReferences adds ownerreferences for the MachineHealthCheck.
func (m *MachineHealthCheckBuilder) WithOwnerReferences(ownerRefs []metav1.OwnerReference) *MachineHealthCheckBuilder {
	m.ownerRefs = ownerRefs
	return m
}

// WithMaxUnhealthy adds a MaxUnhealthyValue for the MachineHealthCheck.
func (m *MachineHealthCheckBuilder) WithMaxUnhealthy(maxUnhealthy *intstr.IntOrString) *MachineHealthCheckBuilder {
	m.maxUnhealthy = maxUnhealthy
	return m
}

// Build returns a MachineHealthCheck with the supplied details.
func (m *MachineHealthCheckBuilder) Build() *clusterv1.MachineHealthCheck {
	// create a MachineHealthCheck with the spec given in the ClusterClass
	return &clusterv1.MachineHealthCheck{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachineHealthCheck",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            m.name,
			Namespace:       m.namespace,
			OwnerReferences: m.ownerRefs,
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName:         m.clusterName,
			Selector:            m.selector,
			UnhealthyConditions: m.conditions,
			MaxUnhealthy:        m.maxUnhealthy,
		},
	}
}
