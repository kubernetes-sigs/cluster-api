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

package v1alpha4

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:unservedversion
// +kubebuilder:deprecatedversion
// +kubebuilder:resource:path=clusterclasses,shortName=cc,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ClusterClass"

// ClusterClass is a template which can be used to create managed topologies.
//
// Deprecated: This type will be removed in one of the next releases.
type ClusterClass struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of ClusterClass.
	Spec ClusterClassSpec `json:"spec,omitempty"`
}

// ClusterClassSpec describes the desired state of the ClusterClass.
type ClusterClassSpec struct {
	// infrastructure is a reference to a provider-specific template that holds
	// the details for provisioning infrastructure specific cluster
	// for the underlying provider.
	// The underlying provider is responsible for the implementation
	// of the template to an infrastructure cluster.
	Infrastructure LocalObjectTemplate `json:"infrastructure,omitempty"`

	// controlPlane is a reference to a local struct that holds the details
	// for provisioning the Control Plane for the Cluster.
	ControlPlane ControlPlaneClass `json:"controlPlane,omitempty"`

	// workers describes the worker nodes for the cluster.
	// It is a collection of node types which can be used to create
	// the worker nodes of the cluster.
	// +optional
	Workers WorkersClass `json:"workers,omitempty"`
}

// ControlPlaneClass defines the class for the control plane.
type ControlPlaneClass struct {
	// metadata is the metadata applied to the machines of the ControlPlane.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	//
	// This field is supported if and only if the control plane provider template
	// referenced is Machine based.
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// LocalObjectTemplate contains the reference to the control plane provider.
	LocalObjectTemplate `json:",inline"`

	// machineInfrastructure defines the metadata and infrastructure information
	// for control plane machines.
	//
	// This field is supported if and only if the control plane provider template
	// referenced above is Machine based and supports setting replicas.
	//
	// +optional
	MachineInfrastructure *LocalObjectTemplate `json:"machineInfrastructure,omitempty"`
}

// WorkersClass is a collection of deployment classes.
type WorkersClass struct {
	// machineDeployments is a list of machine deployment classes that can be used to create
	// a set of worker nodes.
	MachineDeployments []MachineDeploymentClass `json:"machineDeployments,omitempty"`
}

// MachineDeploymentClass serves as a template to define a set of worker nodes of the cluster
// provisioned using the `ClusterClass`.
type MachineDeploymentClass struct {
	// class denotes a type of worker node present in the cluster,
	// this name MUST be unique within a ClusterClass and can be referenced
	// in the Cluster to create a managed MachineDeployment.
	Class string `json:"class"`

	// template is a local struct containing a collection of templates for creation of
	// MachineDeployment objects representing a set of worker nodes.
	Template MachineDeploymentClassTemplate `json:"template"`
}

// MachineDeploymentClassTemplate defines how a MachineDeployment generated from a MachineDeploymentClass
// should look like.
type MachineDeploymentClassTemplate struct {
	// metadata is the metadata applied to the machines of the MachineDeployment.
	// At runtime this metadata is merged with the corresponding metadata from the topology.
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// bootstrap contains the bootstrap template reference to be used
	// for the creation of worker Machines.
	Bootstrap LocalObjectTemplate `json:"bootstrap"`

	// infrastructure contains the infrastructure template reference to be used
	// for the creation of worker Machines.
	Infrastructure LocalObjectTemplate `json:"infrastructure"`
}

// LocalObjectTemplate defines a template for a topology Class.
type LocalObjectTemplate struct {
	// ref is a required reference to a custom resource
	// offered by a provider.
	Ref *corev1.ObjectReference `json:"ref"`
}

// +kubebuilder:object:root=true

// ClusterClassList contains a list of Cluster.
//
// Deprecated: This type will be removed in one of the next releases.
type ClusterClassList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of ClusterClasses.
	Items []ClusterClass `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ClusterClass{}, &ClusterClassList{})
}
