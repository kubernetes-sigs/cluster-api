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
	network               *clusterv1.ClusterNetwork
}

// Cluster returns a ClusterBuilder with the given name and namespace.
func Cluster(namespace, name string) *ClusterBuilder {
	return &ClusterBuilder{
		namespace: namespace,
		name:      name,
	}
}

// WithClusterNetwork sets the ClusterNetwork for the ClusterBuilder.
func (c *ClusterBuilder) WithClusterNetwork(clusterNetwork *clusterv1.ClusterNetwork) *ClusterBuilder {
	c.network = clusterNetwork
	return c
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
			Topology:       c.topology,
			ClusterNetwork: c.network,
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
	class, classNamespace string
	workers               *clusterv1.WorkersTopology
	version               string
	controlPlaneReplicas  int32
	controlPlaneMHC       *clusterv1.MachineHealthCheckTopology
	variables             []clusterv1.ClusterVariable
	controlPlaneVariables []clusterv1.ClusterVariable
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

// WithClassNamespace adds the passed classNamespace to the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithClassNamespace(ns string) *ClusterTopologyBuilder {
	c.classNamespace = ns
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

// WithControlPlaneVariables adds the passed variable values to the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithControlPlaneVariables(variables ...clusterv1.ClusterVariable) *ClusterTopologyBuilder {
	c.controlPlaneVariables = variables
	return c
}

// WithControlPlaneMachineHealthCheck adds MachineHealthCheckTopology used as the MachineHealthCheck value.
func (c *ClusterTopologyBuilder) WithControlPlaneMachineHealthCheck(mhc *clusterv1.MachineHealthCheckTopology) *ClusterTopologyBuilder {
	c.controlPlaneMHC = mhc
	return c
}

// WithMachineDeployment passes the full MachineDeploymentTopology and adds it to an existing list in the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithMachineDeployment(mdc clusterv1.MachineDeploymentTopology) *ClusterTopologyBuilder {
	c.workers.MachineDeployments = append(c.workers.MachineDeployments, mdc)
	return c
}

// WithMachinePool passes the full MachinePoolTopology and adds it to an existing list in the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithMachinePool(mpc clusterv1.MachinePoolTopology) *ClusterTopologyBuilder {
	c.workers.MachinePools = append(c.workers.MachinePools, mpc)
	return c
}

// WithVariables adds the passed variables to the ClusterTopologyBuilder.
func (c *ClusterTopologyBuilder) WithVariables(vars ...clusterv1.ClusterVariable) *ClusterTopologyBuilder {
	c.variables = vars
	return c
}

// Build returns a testable cluster Topology object with any values passed to the builder.
func (c *ClusterTopologyBuilder) Build() *clusterv1.Topology {
	t := &clusterv1.Topology{
		Class:          c.class,
		ClassNamespace: c.classNamespace,
		Workers:        c.workers,
		Version:        c.version,
		ControlPlane: clusterv1.ControlPlaneTopology{
			Replicas:           &c.controlPlaneReplicas,
			MachineHealthCheck: c.controlPlaneMHC,
		},
		Variables: c.variables,
	}

	if len(c.controlPlaneVariables) > 0 {
		t.ControlPlane.Variables = &clusterv1.ControlPlaneVariables{
			Overrides: c.controlPlaneVariables,
		}
	}

	return t
}

// MachineDeploymentTopologyBuilder holds the values needed to create a testable MachineDeploymentTopology.
type MachineDeploymentTopologyBuilder struct {
	annotations map[string]string
	class       string
	name        string
	replicas    *int32
	mhc         *clusterv1.MachineHealthCheckTopology
	variables   []clusterv1.ClusterVariable
}

// MachineDeploymentTopology returns a builder used to create a testable MachineDeploymentTopology.
func MachineDeploymentTopology(name string) *MachineDeploymentTopologyBuilder {
	return &MachineDeploymentTopologyBuilder{
		name: name,
	}
}

// WithAnnotations adds annotations map used as the MachineDeploymentTopology annotations.
func (m *MachineDeploymentTopologyBuilder) WithAnnotations(annotations map[string]string) *MachineDeploymentTopologyBuilder {
	m.annotations = annotations
	return m
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

// WithMachineHealthCheck adds MachineHealthCheckTopology used as the MachineHealthCheck value.
func (m *MachineDeploymentTopologyBuilder) WithMachineHealthCheck(mhc *clusterv1.MachineHealthCheckTopology) *MachineDeploymentTopologyBuilder {
	m.mhc = mhc
	return m
}

// Build returns a testable MachineDeploymentTopology with any values passed to the builder.
func (m *MachineDeploymentTopologyBuilder) Build() clusterv1.MachineDeploymentTopology {
	md := clusterv1.MachineDeploymentTopology{
		Metadata: clusterv1.ObjectMeta{
			Annotations: m.annotations,
		},
		Class:              m.class,
		Name:               m.name,
		Replicas:           m.replicas,
		MachineHealthCheck: m.mhc,
	}

	if len(m.variables) > 0 {
		md.Variables = &clusterv1.MachineDeploymentVariables{
			Overrides: m.variables,
		}
	}

	return md
}

// MachinePoolTopologyBuilder holds the values needed to create a testable MachinePoolTopology.
type MachinePoolTopologyBuilder struct {
	class          string
	name           string
	replicas       *int32
	failureDomains []string
	variables      []clusterv1.ClusterVariable
}

// MachinePoolTopology returns a builder used to create a testable MachinePoolTopology.
func MachinePoolTopology(name string) *MachinePoolTopologyBuilder {
	return &MachinePoolTopologyBuilder{
		name: name,
	}
}

// WithClass adds a class string used as the MachinePoolTopology class.
func (m *MachinePoolTopologyBuilder) WithClass(class string) *MachinePoolTopologyBuilder {
	m.class = class
	return m
}

// WithReplicas adds a replicas value used as the MachinePoolTopology replicas value.
func (m *MachinePoolTopologyBuilder) WithReplicas(replicas int32) *MachinePoolTopologyBuilder {
	m.replicas = &replicas
	return m
}

// WithFailureDomains adds a failureDomains value used as the MachinePoolTopology failureDomains value.
func (m *MachinePoolTopologyBuilder) WithFailureDomains(failureDomains ...string) *MachinePoolTopologyBuilder {
	m.failureDomains = failureDomains
	return m
}

// WithVariables adds variables used as the MachinePoolTopology variables value.
func (m *MachinePoolTopologyBuilder) WithVariables(variables ...clusterv1.ClusterVariable) *MachinePoolTopologyBuilder {
	m.variables = variables
	return m
}

// Build returns a testable MachinePoolTopology with any values passed to the builder.
func (m *MachinePoolTopologyBuilder) Build() clusterv1.MachinePoolTopology {
	mp := clusterv1.MachinePoolTopology{
		Class:          m.class,
		Name:           m.name,
		Replicas:       m.replicas,
		FailureDomains: m.failureDomains,
	}

	if len(m.variables) > 0 {
		mp.Variables = &clusterv1.MachinePoolVariables{
			Overrides: m.variables,
		}
	}

	return mp
}

// ClusterClassBuilder holds the variables and objects required to build a clusterv1.ClusterClass.
type ClusterClassBuilder struct {
	namespace                                 string
	name                                      string
	infrastructureClusterTemplate             *unstructured.Unstructured
	controlPlaneMetadata                      *clusterv1.ObjectMeta
	controlPlaneReadinessGates                []clusterv1.MachineReadinessGate
	controlPlaneTemplate                      *unstructured.Unstructured
	controlPlaneInfrastructureMachineTemplate *unstructured.Unstructured
	controlPlaneMHC                           *clusterv1.MachineHealthCheckClass
	controlPlaneNodeDrainTimeout              *metav1.Duration
	controlPlaneNodeVolumeDetachTimeout       *metav1.Duration
	controlPlaneNodeDeletionTimeout           *metav1.Duration
	controlPlaneNamingStrategy                *clusterv1.ControlPlaneClassNamingStrategy
	infraClusterNamingStrategy                *clusterv1.InfrastructureNamingStrategy
	machineDeploymentClasses                  []clusterv1.MachineDeploymentClass
	machinePoolClasses                        []clusterv1.MachinePoolClass
	variables                                 []clusterv1.ClusterClassVariable
	statusVariables                           []clusterv1.ClusterClassStatusVariable
	patches                                   []clusterv1.ClusterClassPatch
	conditions                                clusterv1.Conditions
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

// WithControlPlaneReadinessGates adds the given readinessGates for use with the ControlPlane to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneReadinessGates(readinessGates []clusterv1.MachineReadinessGate) *ClusterClassBuilder {
	c.controlPlaneReadinessGates = readinessGates
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

// WithControlPlaneNodeDrainTimeout adds a NodeDrainTimeout for the ControlPlane to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneNodeDrainTimeout(t *metav1.Duration) *ClusterClassBuilder {
	c.controlPlaneNodeDrainTimeout = t
	return c
}

// WithControlPlaneNodeVolumeDetachTimeout adds a NodeVolumeDetachTimeout for the ControlPlane to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneNodeVolumeDetachTimeout(t *metav1.Duration) *ClusterClassBuilder {
	c.controlPlaneNodeVolumeDetachTimeout = t
	return c
}

// WithControlPlaneNodeDeletionTimeout adds a NodeDeletionTimeout for the ControlPlane to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneNodeDeletionTimeout(t *metav1.Duration) *ClusterClassBuilder {
	c.controlPlaneNodeDeletionTimeout = t
	return c
}

// WithControlPlaneNamingStrategy sets the NamingStrategy for the ControlPlane to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithControlPlaneNamingStrategy(n *clusterv1.ControlPlaneClassNamingStrategy) *ClusterClassBuilder {
	c.controlPlaneNamingStrategy = n
	return c
}

// WithInfraClusterStrategy sets the NamingStrategy for the infra cluster to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithInfraClusterStrategy(n *clusterv1.InfrastructureNamingStrategy) *ClusterClassBuilder {
	c.infraClusterNamingStrategy = n
	return c
}

// WithVariables adds the Variables to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithVariables(vars ...clusterv1.ClusterClassVariable) *ClusterClassBuilder {
	c.variables = vars
	return c
}

// WithStatusVariables adds the ClusterClassStatusVariables to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithStatusVariables(vars ...clusterv1.ClusterClassStatusVariable) *ClusterClassBuilder {
	c.statusVariables = vars
	return c
}

// WithConditions adds the conditions to the ClusterClassBuilder.
func (c *ClusterClassBuilder) WithConditions(conditions ...clusterv1.Condition) *ClusterClassBuilder {
	c.conditions = conditions
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

// WithWorkerMachinePoolClasses adds the variables and objects needed to create MachinePoolTemplates for a ClusterClassBuilder.
func (c *ClusterClassBuilder) WithWorkerMachinePoolClasses(mpcs ...clusterv1.MachinePoolClass) *ClusterClassBuilder {
	if c.machinePoolClasses == nil {
		c.machinePoolClasses = make([]clusterv1.MachinePoolClass, 0)
	}
	c.machinePoolClasses = append(c.machinePoolClasses, mpcs...)
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
		Status: clusterv1.ClusterClassStatus{
			Conditions: c.conditions,
			Variables:  c.statusVariables,
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
	if c.controlPlaneReadinessGates != nil {
		obj.Spec.ControlPlane.ReadinessGates = c.controlPlaneReadinessGates
	}
	if c.controlPlaneTemplate != nil {
		obj.Spec.ControlPlane.LocalObjectTemplate = clusterv1.LocalObjectTemplate{
			Ref: objToRef(c.controlPlaneTemplate),
		}
	}
	if c.controlPlaneMHC != nil {
		obj.Spec.ControlPlane.MachineHealthCheck = c.controlPlaneMHC
	}
	if c.controlPlaneNodeDrainTimeout != nil {
		obj.Spec.ControlPlane.NodeDrainTimeout = c.controlPlaneNodeDrainTimeout
	}
	if c.controlPlaneNodeVolumeDetachTimeout != nil {
		obj.Spec.ControlPlane.NodeVolumeDetachTimeout = c.controlPlaneNodeVolumeDetachTimeout
	}
	if c.controlPlaneNodeDeletionTimeout != nil {
		obj.Spec.ControlPlane.NodeDeletionTimeout = c.controlPlaneNodeDeletionTimeout
	}
	if c.controlPlaneInfrastructureMachineTemplate != nil {
		obj.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
			Ref: objToRef(c.controlPlaneInfrastructureMachineTemplate),
		}
	}
	if c.controlPlaneNamingStrategy != nil {
		obj.Spec.ControlPlane.NamingStrategy = c.controlPlaneNamingStrategy
	}
	if c.infraClusterNamingStrategy != nil {
		obj.Spec.InfrastructureNamingStrategy = c.infraClusterNamingStrategy
	}

	obj.Spec.Workers.MachineDeployments = c.machineDeploymentClasses
	obj.Spec.Workers.MachinePools = c.machinePoolClasses
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
	readinessGates                []clusterv1.MachineReadinessGate
	failureDomain                 *string
	nodeDrainTimeout              *metav1.Duration
	nodeVolumeDetachTimeout       *metav1.Duration
	nodeDeletionTimeout           *metav1.Duration
	minReadySeconds               *int32
	strategy                      *clusterv1.MachineDeploymentStrategy
	namingStrategy                *clusterv1.MachineDeploymentClassNamingStrategy
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

// WithReadinessGates sets the readinessGates for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithReadinessGates(readinessGates []clusterv1.MachineReadinessGate) *MachineDeploymentClassBuilder {
	m.readinessGates = readinessGates
	return m
}

// WithFailureDomain sets the FailureDomain for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithFailureDomain(f *string) *MachineDeploymentClassBuilder {
	m.failureDomain = f
	return m
}

// WithNodeDrainTimeout sets the NodeDrainTimeout for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithNodeDrainTimeout(t *metav1.Duration) *MachineDeploymentClassBuilder {
	m.nodeDrainTimeout = t
	return m
}

// WithNodeVolumeDetachTimeout sets the NodeVolumeDetachTimeout for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithNodeVolumeDetachTimeout(t *metav1.Duration) *MachineDeploymentClassBuilder {
	m.nodeVolumeDetachTimeout = t
	return m
}

// WithNodeDeletionTimeout sets the NodeDeletionTimeout for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithNodeDeletionTimeout(t *metav1.Duration) *MachineDeploymentClassBuilder {
	m.nodeDeletionTimeout = t
	return m
}

// WithMinReadySeconds sets the MinReadySeconds for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithMinReadySeconds(t *int32) *MachineDeploymentClassBuilder {
	m.minReadySeconds = t
	return m
}

// WithStrategy sets the Strategy for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithStrategy(s *clusterv1.MachineDeploymentStrategy) *MachineDeploymentClassBuilder {
	m.strategy = s
	return m
}

// WithNamingStrategy sets the NamingStrategy for the MachineDeploymentClassBuilder.
func (m *MachineDeploymentClassBuilder) WithNamingStrategy(n *clusterv1.MachineDeploymentClassNamingStrategy) *MachineDeploymentClassBuilder {
	m.namingStrategy = n
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
	if m.readinessGates != nil {
		obj.ReadinessGates = m.readinessGates
	}
	if m.failureDomain != nil {
		obj.FailureDomain = m.failureDomain
	}
	if m.nodeDrainTimeout != nil {
		obj.NodeDrainTimeout = m.nodeDrainTimeout
	}
	if m.nodeVolumeDetachTimeout != nil {
		obj.NodeVolumeDetachTimeout = m.nodeVolumeDetachTimeout
	}
	if m.nodeDeletionTimeout != nil {
		obj.NodeDeletionTimeout = m.nodeDeletionTimeout
	}
	if m.minReadySeconds != nil {
		obj.MinReadySeconds = m.minReadySeconds
	}
	if m.strategy != nil {
		obj.Strategy = m.strategy
	}
	if m.namingStrategy != nil {
		obj.NamingStrategy = m.namingStrategy
	}
	return obj
}

// MachinePoolClassBuilder holds the variables and objects required to build a clusterv1.MachinePoolClass.
type MachinePoolClassBuilder struct {
	class                             string
	infrastructureMachinePoolTemplate *unstructured.Unstructured
	bootstrapTemplate                 *unstructured.Unstructured
	labels                            map[string]string
	annotations                       map[string]string
	failureDomains                    []string
	nodeDrainTimeout                  *metav1.Duration
	nodeVolumeDetachTimeout           *metav1.Duration
	nodeDeletionTimeout               *metav1.Duration
	minReadySeconds                   *int32
	namingStrategy                    *clusterv1.MachinePoolClassNamingStrategy
}

// MachinePoolClass returns a MachinePoolClassBuilder with the given name and namespace.
func MachinePoolClass(class string) *MachinePoolClassBuilder {
	return &MachinePoolClassBuilder{
		class: class,
	}
}

// WithInfrastructureTemplate registers the passed Unstructured object as the InfrastructureMachinePoolTemplate for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithInfrastructureTemplate(t *unstructured.Unstructured) *MachinePoolClassBuilder {
	m.infrastructureMachinePoolTemplate = t
	return m
}

// WithBootstrapTemplate registers the passed Unstructured object as the BootstrapTemplate for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithBootstrapTemplate(t *unstructured.Unstructured) *MachinePoolClassBuilder {
	m.bootstrapTemplate = t
	return m
}

// WithLabels sets the labels for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithLabels(labels map[string]string) *MachinePoolClassBuilder {
	m.labels = labels
	return m
}

// WithAnnotations sets the annotations for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithAnnotations(annotations map[string]string) *MachinePoolClassBuilder {
	m.annotations = annotations
	return m
}

// WithFailureDomains sets the FailureDomains for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithFailureDomains(failureDomains ...string) *MachinePoolClassBuilder {
	m.failureDomains = failureDomains
	return m
}

// WithNodeDrainTimeout sets the NodeDrainTimeout for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithNodeDrainTimeout(t *metav1.Duration) *MachinePoolClassBuilder {
	m.nodeDrainTimeout = t
	return m
}

// WithNodeVolumeDetachTimeout sets the NodeVolumeDetachTimeout for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithNodeVolumeDetachTimeout(t *metav1.Duration) *MachinePoolClassBuilder {
	m.nodeVolumeDetachTimeout = t
	return m
}

// WithNodeDeletionTimeout sets the NodeDeletionTimeout for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithNodeDeletionTimeout(t *metav1.Duration) *MachinePoolClassBuilder {
	m.nodeDeletionTimeout = t
	return m
}

// WithMinReadySeconds sets the MinReadySeconds for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithMinReadySeconds(t *int32) *MachinePoolClassBuilder {
	m.minReadySeconds = t
	return m
}

// WithNamingStrategy sets the NamingStrategy for the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) WithNamingStrategy(n *clusterv1.MachinePoolClassNamingStrategy) *MachinePoolClassBuilder {
	m.namingStrategy = n
	return m
}

// Build creates a full MachinePoolClass object with the variables passed to the MachinePoolClassBuilder.
func (m *MachinePoolClassBuilder) Build() *clusterv1.MachinePoolClass {
	obj := &clusterv1.MachinePoolClass{
		Class: m.class,
		Template: clusterv1.MachinePoolClassTemplate{
			Metadata: clusterv1.ObjectMeta{
				Labels:      m.labels,
				Annotations: m.annotations,
			},
		},
	}
	if m.bootstrapTemplate != nil {
		obj.Template.Bootstrap.Ref = objToRef(m.bootstrapTemplate)
	}
	if m.infrastructureMachinePoolTemplate != nil {
		obj.Template.Infrastructure.Ref = objToRef(m.infrastructureMachinePoolTemplate)
	}
	if m.failureDomains != nil {
		obj.FailureDomains = m.failureDomains
	}
	if m.nodeDrainTimeout != nil {
		obj.NodeDrainTimeout = m.nodeDrainTimeout
	}
	if m.nodeVolumeDetachTimeout != nil {
		obj.NodeVolumeDetachTimeout = m.nodeVolumeDetachTimeout
	}
	if m.nodeDeletionTimeout != nil {
		obj.NodeDeletionTimeout = m.nodeDeletionTimeout
	}
	if m.minReadySeconds != nil {
		obj.MinReadySeconds = m.minReadySeconds
	}
	if m.namingStrategy != nil {
		obj.NamingStrategy = m.namingStrategy
	}
	return obj
}

// InfrastructureMachineTemplateBuilder holds the variables and objects needed to build an InfrastructureMachineTemplate.
type InfrastructureMachineTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// InfrastructureMachineTemplate creates an InfrastructureMachineTemplateBuilder with the given name and namespace.
func InfrastructureMachineTemplate(namespace, name string) *InfrastructureMachineTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureMachineTemplateKind)
	// Set the mandatory spec fields for the object.
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})
	return &InfrastructureMachineTemplateBuilder{
		obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *InfrastructureMachineTemplateBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureMachineTemplateBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build takes the objects and variables in the  InfrastructureMachineTemplateBuilder and generates an unstructured object.
func (i *InfrastructureMachineTemplateBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// TestInfrastructureMachineTemplateBuilder holds the variables and objects needed to build an TestInfrastructureMachineTemplate.
type TestInfrastructureMachineTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// TestInfrastructureMachineTemplate creates an TestInfrastructureMachineTemplateBuilder with the given name and namespace.
func TestInfrastructureMachineTemplate(namespace, name string) *TestInfrastructureMachineTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(TestInfrastructureMachineTemplateKind)
	// Set the mandatory spec fields for the object.
	if err := unstructured.SetNestedField(obj.Object, map[string]interface{}{}, "spec", "template", "spec"); err != nil {
		panic(err)
	}
	return &TestInfrastructureMachineTemplateBuilder{
		obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."; the path should correspond to a field defined in the CRD.
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *TestInfrastructureMachineTemplateBuilder) WithSpecFields(fields map[string]interface{}) *TestInfrastructureMachineTemplateBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build takes the objects and variables in the  InfrastructureMachineTemplateBuilder and generates an unstructured object.
func (i *TestInfrastructureMachineTemplateBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// InfrastructureMachinePoolTemplateBuilder holds the variables and objects needed to build an InfrastructureMachinePoolTemplate.
type InfrastructureMachinePoolTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// InfrastructureMachinePoolTemplate creates an InfrastructureMachinePoolTemplateBuilder with the given name and namespace.
func InfrastructureMachinePoolTemplate(namespace, name string) *InfrastructureMachinePoolTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureMachinePoolTemplateKind)
	// Set the mandatory spec fields for the object.
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})
	return &InfrastructureMachinePoolTemplateBuilder{
		obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *InfrastructureMachinePoolTemplateBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureMachinePoolTemplateBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build takes the objects and variables in the  InfrastructureMachineTemplateBuilder and generates an unstructured object.
func (i *InfrastructureMachinePoolTemplateBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// TestInfrastructureMachinePoolTemplateBuilder holds the variables and objects needed to build an TestInfrastructureMachinePoolTemplate.
type TestInfrastructureMachinePoolTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// TestInfrastructureMachinePoolTemplate creates an TestInfrastructureMachinePoolTemplateBuilder with the given name and namespace.
func TestInfrastructureMachinePoolTemplate(namespace, name string) *TestInfrastructureMachinePoolTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(TestInfrastructureMachinePoolTemplateKind)
	// Set the mandatory spec fields for the object.
	if err := unstructured.SetNestedField(obj.Object, map[string]interface{}{}, "spec", "template", "spec"); err != nil {
		panic(err)
	}
	return &TestInfrastructureMachinePoolTemplateBuilder{
		obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."; the path should correspond to a field defined in the CRD.
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *TestInfrastructureMachinePoolTemplateBuilder) WithSpecFields(fields map[string]interface{}) *TestInfrastructureMachinePoolTemplateBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build takes the objects and variables in the TestInfrastructureMachineTemplateBuilder and generates an unstructured object.
func (i *TestInfrastructureMachinePoolTemplateBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// InfrastructureMachinePoolBuilder holds the variables and objects needed to build an InfrastructureMachinePool.
type InfrastructureMachinePoolBuilder struct {
	obj *unstructured.Unstructured
}

// InfrastructureMachinePool creates an InfrastructureMachinePoolBuilder with the given name and namespace.
func InfrastructureMachinePool(namespace, name string) *InfrastructureMachinePoolBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureMachinePoolKind)
	// Set the mandatory spec fields for the object.
	setSpecFields(obj, map[string]interface{}{"spec": map[string]interface{}{}})
	return &InfrastructureMachinePoolBuilder{
		obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *InfrastructureMachinePoolBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureMachinePoolBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build takes the objects and variables in the InfrastructureMachinePoolBuilder and generates an unstructured object.
func (i *InfrastructureMachinePoolBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// TestInfrastructureMachinePoolBuilder holds the variables and objects needed to build an TestInfrastructureMachinePool.
type TestInfrastructureMachinePoolBuilder struct {
	obj *unstructured.Unstructured
}

// TestInfrastructureMachinePool creates an TestInfrastructureMachinePoolBuilder with the given name and namespace.
func TestInfrastructureMachinePool(namespace, name string) *TestInfrastructureMachinePoolBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(TestInfrastructureMachinePoolKind)
	// Set the mandatory spec fields for the object.
	if err := unstructured.SetNestedField(obj.Object, map[string]interface{}{}, "spec"); err != nil {
		panic(err)
	}
	return &TestInfrastructureMachinePoolBuilder{
		obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."; the path should correspond to a field defined in the CRD.
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *TestInfrastructureMachinePoolBuilder) WithSpecFields(fields map[string]interface{}) *TestInfrastructureMachinePoolBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build takes the objects and variables in the TestInfrastructureMachinePoolBuilder and generates an unstructured object.
func (i *TestInfrastructureMachinePoolBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// BootstrapTemplateBuilder holds the variables needed to build a generic BootstrapTemplate.
type BootstrapTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// BootstrapTemplate creates a BootstrapTemplateBuilder with the given name and namespace.
func BootstrapTemplate(namespace, name string) *BootstrapTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(BootstrapGroupVersion.String())
	obj.SetKind(GenericBootstrapConfigTemplateKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})

	return &BootstrapTemplateBuilder{obj: obj}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
func (b *BootstrapTemplateBuilder) WithSpecFields(fields map[string]interface{}) *BootstrapTemplateBuilder {
	setSpecFields(b.obj, fields)
	return b
}

// Build creates a new Unstructured object with the information passed to the BootstrapTemplateBuilder.
func (b *BootstrapTemplateBuilder) Build() *unstructured.Unstructured {
	return b.obj
}

// TestBootstrapTemplateBuilder holds the variables needed to build a generic TestBootstrapTemplate.
type TestBootstrapTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// TestBootstrapTemplate creates a TestBootstrapTemplateBuilder with the given name and namespace.
func TestBootstrapTemplate(namespace, name string) *TestBootstrapTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(BootstrapGroupVersion.String())
	obj.SetKind(TestBootstrapConfigTemplateKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})

	return &TestBootstrapTemplateBuilder{
		obj: obj,
	}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
// NOTE: The path should correspond to a field defined in the CRD.
func (b *TestBootstrapTemplateBuilder) WithSpecFields(fields map[string]interface{}) *TestBootstrapTemplateBuilder {
	setSpecFields(b.obj, fields)
	return b
}

// Build creates a new Unstructured object with the information passed to the BootstrapTemplateBuilder.
func (b *TestBootstrapTemplateBuilder) Build() *unstructured.Unstructured {
	return b.obj
}

// BootstrapConfigBuilder holds the variables needed to build a generic BootstrapConfig.
type BootstrapConfigBuilder struct {
	obj *unstructured.Unstructured
}

// BootstrapConfig creates a BootstrapConfigBuilder with the given name and namespace.
func BootstrapConfig(namespace, name string) *BootstrapConfigBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(BootstrapGroupVersion.String())
	obj.SetKind(GenericBootstrapConfigKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	setSpecFields(obj, map[string]interface{}{"spec": map[string]interface{}{}})

	return &BootstrapConfigBuilder{obj: obj}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
func (b *BootstrapConfigBuilder) WithSpecFields(fields map[string]interface{}) *BootstrapConfigBuilder {
	setSpecFields(b.obj, fields)
	return b
}

// Build creates a new Unstructured object with the information passed to the BootstrapConfigBuilder.
func (b *BootstrapConfigBuilder) Build() *unstructured.Unstructured {
	return b.obj
}

// TestBootstrapConfigBuilder holds the variables needed to build a generic TestBootstrapConfig.
type TestBootstrapConfigBuilder struct {
	obj *unstructured.Unstructured
}

// TestBootstrapConfig creates a TestBootstrapConfigBuilder with the given name and namespace.
func TestBootstrapConfig(namespace, name string) *TestBootstrapConfigBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(BootstrapGroupVersion.String())
	obj.SetKind(TestBootstrapConfigKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	setSpecFields(obj, map[string]interface{}{"spec": map[string]interface{}{}})

	return &TestBootstrapConfigBuilder{
		obj: obj,
	}
}

// WithSpecFields will add fields of any type to the object spec. It takes an argument, fields, which is of the form path: object.
// NOTE: The path should correspond to a field defined in the CRD.
func (b *TestBootstrapConfigBuilder) WithSpecFields(fields map[string]interface{}) *TestBootstrapConfigBuilder {
	setSpecFields(b.obj, fields)
	return b
}

// Build creates a new Unstructured object with the information passed to the BootstrapConfigBuilder.
func (b *TestBootstrapConfigBuilder) Build() *unstructured.Unstructured {
	return b.obj
}

// InfrastructureClusterTemplateBuilder holds the variables needed to build a generic InfrastructureClusterTemplate.
type InfrastructureClusterTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// InfrastructureClusterTemplate returns an InfrastructureClusterTemplateBuilder with the given name and namespace.
func InfrastructureClusterTemplate(namespace, name string) *InfrastructureClusterTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureClusterTemplateKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	if err := unstructured.SetNestedField(obj.Object, map[string]interface{}{}, "spec", "template", "spec"); err != nil {
		panic(err)
	}
	return &InfrastructureClusterTemplateBuilder{
		obj: obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *InfrastructureClusterTemplateBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureClusterTemplateBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build creates a new Unstructured object with the variables passed to the InfrastructureClusterTemplateBuilder.
func (i *InfrastructureClusterTemplateBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// TestInfrastructureClusterTemplateBuilder holds the variables needed to build a generic TestInfrastructureClusterTemplate.
type TestInfrastructureClusterTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// TestInfrastructureClusterTemplate returns an TestInfrastructureClusterTemplateBuilder with the given name and namespace.
func TestInfrastructureClusterTemplate(namespace, name string) *TestInfrastructureClusterTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(TestInfrastructureClusterTemplateKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	if err := unstructured.SetNestedField(obj.Object, map[string]interface{}{}, "spec", "template", "spec"); err != nil {
		panic(err)
	}
	return &TestInfrastructureClusterTemplateBuilder{
		obj: obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."; the path should correspond to a field defined in the CRD.
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *TestInfrastructureClusterTemplateBuilder) WithSpecFields(fields map[string]interface{}) *TestInfrastructureClusterTemplateBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build creates a new Unstructured object with the variables passed to the InfrastructureClusterTemplateBuilder.
func (i *TestInfrastructureClusterTemplateBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// ControlPlaneTemplateBuilder holds the variables and objects needed to build a generic ControlPlane template.
type ControlPlaneTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// ControlPlaneTemplate creates a NewControlPlaneTemplate builder with the given name and namespace.
func ControlPlaneTemplate(namespace, name string) *ControlPlaneTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ControlPlaneGroupVersion.String())
	obj.SetKind(GenericControlPlaneTemplateKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)

	// Initialize the spec.template.spec to make the object valid in reconciliation.
	setSpecFields(obj, map[string]interface{}{"spec.template.spec": map[string]interface{}{}})
	return &ControlPlaneTemplateBuilder{obj: obj}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (c *ControlPlaneTemplateBuilder) WithSpecFields(fields map[string]interface{}) *ControlPlaneTemplateBuilder {
	setSpecFields(c.obj, fields)
	return c
}

// WithInfrastructureMachineTemplate adds the given Unstructured object to the ControlPlaneTemplateBuilder as its InfrastructureMachineTemplate.
func (c *ControlPlaneTemplateBuilder) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *ControlPlaneTemplateBuilder {
	if err := setNestedRef(c.obj, t, "spec", "template", "spec", "machineTemplate", "infrastructureRef"); err != nil {
		panic(err)
	}
	return c
}

// Build creates an Unstructured object from the variables passed to the ControlPlaneTemplateBuilder.
func (c *ControlPlaneTemplateBuilder) Build() *unstructured.Unstructured {
	return c.obj
}

// TestControlPlaneTemplateBuilder holds the variables and objects needed to build a generic ControlPlane template.
type TestControlPlaneTemplateBuilder struct {
	obj *unstructured.Unstructured
}

// TestControlPlaneTemplate creates a NewControlPlaneTemplate builder with the given name and namespace.
func TestControlPlaneTemplate(namespace, name string) *TestControlPlaneTemplateBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion(ControlPlaneGroupVersion.String())
	obj.SetKind(TestControlPlaneTemplateKind)
	// Set the mandatory spec field for the object.
	if err := unstructured.SetNestedField(obj.Object, map[string]interface{}{}, "spec", "template", "spec"); err != nil {
		panic(err)
	}
	return &TestControlPlaneTemplateBuilder{
		obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."; the path should correspond to a field defined in the CRD.
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (c *TestControlPlaneTemplateBuilder) WithSpecFields(fields map[string]interface{}) *TestControlPlaneTemplateBuilder {
	setSpecFields(c.obj, fields)
	return c
}

// WithInfrastructureMachineTemplate adds the given Unstructured object to the ControlPlaneTemplateBuilder as its InfrastructureMachineTemplate.
func (c *TestControlPlaneTemplateBuilder) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *TestControlPlaneTemplateBuilder {
	if err := setNestedRef(c.obj, t, "spec", "template", "spec", "machineTemplate", "infrastructureRef"); err != nil {
		panic(err)
	}
	return c
}

// Build creates an Unstructured object from the variables passed to the ControlPlaneTemplateBuilder.
func (c *TestControlPlaneTemplateBuilder) Build() *unstructured.Unstructured {
	return c.obj
}

// InfrastructureClusterBuilder holds the variables and objects needed to build a generic InfrastructureCluster.
type InfrastructureClusterBuilder struct {
	obj *unstructured.Unstructured
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *InfrastructureClusterBuilder) WithSpecFields(fields map[string]interface{}) *InfrastructureClusterBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// InfrastructureCluster returns and InfrastructureClusterBuilder with the given name and namespace.
func InfrastructureCluster(namespace, name string) *InfrastructureClusterBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(GenericInfrastructureClusterKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return &InfrastructureClusterBuilder{obj: obj}
}

// Build returns an Unstructured object with the information passed to the InfrastructureClusterBuilder.
func (i *InfrastructureClusterBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// TestInfrastructureClusterBuilder holds the variables and objects needed to build a generic TestInfrastructureCluster.
type TestInfrastructureClusterBuilder struct {
	obj *unstructured.Unstructured
}

// TestInfrastructureCluster returns and TestInfrastructureClusterBuilder with the given name and namespace.
func TestInfrastructureCluster(namespace, name string) *TestInfrastructureClusterBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(InfrastructureGroupVersion.String())
	obj.SetKind(TestInfrastructureClusterKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return &TestInfrastructureClusterBuilder{
		obj: obj,
	}
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."; the path should correspond to a field defined in the CRD.
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (i *TestInfrastructureClusterBuilder) WithSpecFields(fields map[string]interface{}) *TestInfrastructureClusterBuilder {
	setSpecFields(i.obj, fields)
	return i
}

// Build returns an Unstructured object with the information passed to the InfrastructureClusterBuilder.
func (i *TestInfrastructureClusterBuilder) Build() *unstructured.Unstructured {
	return i.obj
}

// ControlPlaneBuilder holds the variables and objects needed to build a generic object for cluster.spec.controlPlaneRef.
type ControlPlaneBuilder struct {
	obj *unstructured.Unstructured
}

// ControlPlane returns a ControlPlaneBuilder with the given name and Namespace.
func ControlPlane(namespace, name string) *ControlPlaneBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ControlPlaneGroupVersion.String())
	obj.SetKind(GenericControlPlaneKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return &ControlPlaneBuilder{
		obj: obj,
	}
}

// WithInfrastructureMachineTemplate adds the given unstructured object to the ControlPlaneBuilder as its InfrastructureMachineTemplate.
func (c *ControlPlaneBuilder) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *ControlPlaneBuilder {
	// TODO(killianmuldoon): Update to use the internal/contract package, when it is importable from here
	if err := setNestedRef(c.obj, t, "spec", "machineTemplate", "infrastructureRef"); err != nil {
		panic(err)
	}
	return c
}

// WithReplicas sets the number of replicas for the ControlPlaneBuilder.
func (c *ControlPlaneBuilder) WithReplicas(replicas int64) *ControlPlaneBuilder {
	if err := unstructured.SetNestedField(c.obj.Object, replicas, "spec", "replicas"); err != nil {
		panic(err)
	}
	return c
}

// WithVersion adds the passed version to the ControlPlaneBuilder.
func (c *ControlPlaneBuilder) WithVersion(version string) *ControlPlaneBuilder {
	if err := unstructured.SetNestedField(c.obj.Object, version, "spec", "version"); err != nil {
		panic(err)
	}
	return c
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (c *ControlPlaneBuilder) WithSpecFields(fields map[string]interface{}) *ControlPlaneBuilder {
	setSpecFields(c.obj, fields)
	return c
}

// WithStatusFields sets a map of status fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the status field.
//
// Note: all the paths should start with "status."
//
//	Example map: map[string]interface{}{
//	    "status.version": "v1.2.3",
//	}.
func (c *ControlPlaneBuilder) WithStatusFields(fields map[string]interface{}) *ControlPlaneBuilder {
	setStatusFields(c.obj, fields)
	return c
}

// Build generates an Unstructured object from the information passed to the ControlPlaneBuilder.
func (c *ControlPlaneBuilder) Build() *unstructured.Unstructured {
	return c.obj
}

// TestControlPlaneBuilder holds the variables and objects needed to build a generic object for cluster.spec.controlPlaneRef.
type TestControlPlaneBuilder struct {
	obj *unstructured.Unstructured
}

// TestControlPlane returns a TestControlPlaneBuilder with the given name and Namespace.
func TestControlPlane(namespace, name string) *ControlPlaneBuilder {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ControlPlaneGroupVersion.String())
	obj.SetKind(TestControlPlaneKind)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return &ControlPlaneBuilder{
		obj: obj,
	}
}

// WithInfrastructureMachineTemplate adds the given unstructured object to the TestControlPlaneBuilder as its InfrastructureMachineTemplate.
func (c *TestControlPlaneBuilder) WithInfrastructureMachineTemplate(t *unstructured.Unstructured) *TestControlPlaneBuilder {
	// TODO(killianmuldoon): Update to use the internal/contract package, when it is importable from here
	if err := setNestedRef(c.obj, t, "spec", "machineTemplate", "infrastructureRef"); err != nil {
		panic(err)
	}
	return c
}

// WithReplicas sets the number of replicas for the TestControlPlaneBuilder.
func (c *TestControlPlaneBuilder) WithReplicas(replicas int64) *TestControlPlaneBuilder {
	if err := unstructured.SetNestedField(c.obj.Object, replicas, "spec", "replicas"); err != nil {
		panic(err)
	}
	return c
}

// WithVersion adds the passed version to the TestControlPlaneBuilder.
func (c *TestControlPlaneBuilder) WithVersion(version string) *TestControlPlaneBuilder {
	if err := unstructured.SetNestedField(c.obj.Object, version, "spec", "version"); err != nil {
		panic(err)
	}
	return c
}

// WithLabels adds the passed labels to the ControlPlaneBuilder.
func (c *ControlPlaneBuilder) WithLabels(labels map[string]string) *ControlPlaneBuilder {
	c.obj.SetLabels(labels)
	return c
}

// WithAnnotations adds the passed annotations to the ControlPlaneBuilder.
func (c *ControlPlaneBuilder) WithAnnotations(annotations map[string]string) *ControlPlaneBuilder {
	c.obj.SetAnnotations(annotations)
	return c
}

// WithSpecFields sets a map of spec fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the spec field.
//
// Note: all the paths should start with "spec."
//
//	Example map: map[string]interface{}{
//	    "spec.version": "v1.2.3",
//	}.
func (c *TestControlPlaneBuilder) WithSpecFields(fields map[string]interface{}) *TestControlPlaneBuilder {
	setSpecFields(c.obj, fields)
	return c
}

// WithStatusFields sets a map of status fields on the unstructured object. The keys in the map represent the path and the value corresponds
// to the value of the status field.
//
// Note: all the paths should start with "status."
//
//	Example map: map[string]interface{}{
//	    "status.version": "v1.2.3",
//	}.
func (c *TestControlPlaneBuilder) WithStatusFields(fields map[string]interface{}) *TestControlPlaneBuilder {
	setStatusFields(c.obj, fields)
	return c
}

// Build generates an Unstructured object from the information passed to the TestControlPlaneBuilder.
func (c *TestControlPlaneBuilder) Build() *unstructured.Unstructured {
	return c.obj
}

// NodeBuilder holds the variables required to build a Node.
type NodeBuilder struct {
	name   string
	status corev1.NodeStatus
}

// Node returns a NodeBuilder.
func Node(name string) *NodeBuilder {
	return &NodeBuilder{
		name: name,
	}
}

// WithStatus adds Status to the NodeBuilder.
func (n *NodeBuilder) WithStatus(status corev1.NodeStatus) *NodeBuilder {
	n.status = status
	return n
}

// Build produces a new Node from the information passed to the NodeBuilder.
func (n *NodeBuilder) Build() *corev1.Node {
	obj := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: n.name,
		},
		Status: n.status,
	}
	return obj
}

// MachinePoolBuilder holds the variables and objects needed to build a generic MachinePool.
type MachinePoolBuilder struct {
	namespace       string
	name            string
	bootstrap       *unstructured.Unstructured
	infrastructure  *unstructured.Unstructured
	version         *string
	clusterName     string
	replicas        *int32
	labels          map[string]string
	annotations     map[string]string
	status          *expv1.MachinePoolStatus
	minReadySeconds *int32
}

// MachinePool creates a MachinePoolBuilder with the given name and namespace.
func MachinePool(namespace, name string) *MachinePoolBuilder {
	return &MachinePoolBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithBootstrap adds the passed Unstructured object to the MachinePoolBuilder as a bootstrap.
func (m *MachinePoolBuilder) WithBootstrap(ref *unstructured.Unstructured) *MachinePoolBuilder {
	m.bootstrap = ref
	return m
}

// WithInfrastructure adds the passed Unstructured object to the MachinePool builder as an InfrastructureMachinePool.
func (m *MachinePoolBuilder) WithInfrastructure(ref *unstructured.Unstructured) *MachinePoolBuilder {
	m.infrastructure = ref
	return m
}

// WithLabels adds the given labels to the MachinePoolBuilder.
func (m *MachinePoolBuilder) WithLabels(labels map[string]string) *MachinePoolBuilder {
	m.labels = labels
	return m
}

// WithAnnotations adds the given annotations to the MachinePoolBuilder.
func (m *MachinePoolBuilder) WithAnnotations(annotations map[string]string) *MachinePoolBuilder {
	m.annotations = annotations
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

// WithMinReadySeconds sets the passed value on the machine pool spec.
func (m *MachinePoolBuilder) WithMinReadySeconds(minReadySeconds int32) *MachinePoolBuilder {
	m.minReadySeconds = &minReadySeconds
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
			Name:        m.name,
			Namespace:   m.namespace,
			Labels:      m.labels,
			Annotations: m.annotations,
		},
		Spec: expv1.MachinePoolSpec{
			ClusterName:     m.clusterName,
			Replicas:        m.replicas,
			MinReadySeconds: m.minReadySeconds,
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Version:     m.version,
					ClusterName: m.clusterName,
				},
			},
		},
	}
	if m.bootstrap != nil {
		obj.Spec.Template.Spec.Bootstrap.ConfigRef = objToRef(m.bootstrap)
	}
	if m.infrastructure != nil {
		obj.Spec.Template.Spec.InfrastructureRef = *objToRef(m.infrastructure)
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
	clusterName            string
	bootstrapTemplate      *unstructured.Unstructured
	infrastructureTemplate *unstructured.Unstructured
	selector               *metav1.LabelSelector
	version                *string
	replicas               *int32
	generation             *int64
	labels                 map[string]string
	annotations            map[string]string
	status                 *clusterv1.MachineDeploymentStatus
	minReadySeconds        *int32
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

// WithSelector adds the passed selector to the MachineDeployment as the selector.
func (m *MachineDeploymentBuilder) WithSelector(selector metav1.LabelSelector) *MachineDeploymentBuilder {
	m.selector = &selector
	return m
}

// WithClusterName adds the clusterName to the MachineDeploymentBuilder.
func (m *MachineDeploymentBuilder) WithClusterName(name string) *MachineDeploymentBuilder {
	m.clusterName = name
	return m
}

// WithLabels adds the given labels to the MachineDeploymentBuilder.
func (m *MachineDeploymentBuilder) WithLabels(labels map[string]string) *MachineDeploymentBuilder {
	m.labels = labels
	return m
}

// WithAnnotations adds the given annotations to the MachineDeploymentBuilder.
func (m *MachineDeploymentBuilder) WithAnnotations(annotations map[string]string) *MachineDeploymentBuilder {
	m.annotations = annotations
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

// WithMinReadySeconds sets the passed value on the machine deployment spec.
func (m *MachineDeploymentBuilder) WithMinReadySeconds(minReadySeconds int32) *MachineDeploymentBuilder {
	m.minReadySeconds = &minReadySeconds
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
			Name:        m.name,
			Namespace:   m.namespace,
			Labels:      m.labels,
			Annotations: m.annotations,
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
	if m.selector != nil {
		obj.Spec.Selector = *m.selector
	}
	if m.status != nil {
		obj.Status = *m.status
	}
	if m.clusterName != "" {
		obj.Spec.Template.Spec.ClusterName = m.clusterName
		obj.Spec.ClusterName = m.clusterName
		if obj.Spec.Selector.MatchLabels == nil {
			obj.Spec.Selector.MatchLabels = map[string]string{}
		}
		obj.Spec.Selector.MatchLabels[clusterv1.ClusterNameLabel] = m.clusterName
		obj.Spec.Template.Labels = map[string]string{
			clusterv1.ClusterNameLabel: m.clusterName,
		}
	}
	obj.Spec.MinReadySeconds = m.minReadySeconds

	return obj
}

// MachineSetBuilder holds the variables and objects needed to build a MachineSet.
type MachineSetBuilder struct {
	namespace              string
	name                   string
	bootstrapTemplate      *unstructured.Unstructured
	infrastructureTemplate *unstructured.Unstructured
	replicas               *int32
	labels                 map[string]string
	clusterName            string
	ownerRefs              []metav1.OwnerReference
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

// WithInfrastructureTemplate adds the passed unstructured object to the MachineSetBuilder as an infrastructureMachineTemplate.
func (m *MachineSetBuilder) WithInfrastructureTemplate(ref *unstructured.Unstructured) *MachineSetBuilder {
	m.infrastructureTemplate = ref
	return m
}

// WithLabels adds the given labels to the MachineSetBuilder.
func (m *MachineSetBuilder) WithLabels(labels map[string]string) *MachineSetBuilder {
	m.labels = labels
	return m
}

// WithReplicas sets the number of replicas for the MachineSetBuilder.
func (m *MachineSetBuilder) WithReplicas(replicas *int32) *MachineSetBuilder {
	m.replicas = replicas
	return m
}

// WithClusterName sets the number of replicas for the MachineSetBuilder.
func (m *MachineSetBuilder) WithClusterName(name string) *MachineSetBuilder {
	m.clusterName = name
	return m
}

// WithOwnerReferences adds ownerReferences for the MachineSetBuilder.
func (m *MachineSetBuilder) WithOwnerReferences(ownerRefs []metav1.OwnerReference) *MachineSetBuilder {
	m.ownerRefs = ownerRefs
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
			Name:            m.name,
			Namespace:       m.namespace,
			Labels:          m.labels,
			OwnerReferences: m.ownerRefs,
		},
	}
	obj.Spec.ClusterName = m.clusterName
	obj.Spec.Template.Spec.ClusterName = m.clusterName
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
		machine.ObjectMeta.Labels[clusterv1.ClusterNameLabel] = m.clusterName
	}
	return machine
}

// objToRef returns a reference to the given object.
// Note: This function only operates on Unstructured instead of client.Object
// because it is only safe to assume for Unstructured that the GVK is set.
func objToRef(obj *unstructured.Unstructured) *corev1.ObjectReference {
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
	mhc := &clusterv1.MachineHealthCheck{
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
	if m.clusterName != "" {
		mhc.Labels = map[string]string{clusterv1.ClusterNameLabel: m.clusterName}
	}

	return mhc
}
