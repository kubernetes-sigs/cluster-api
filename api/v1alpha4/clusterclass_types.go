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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterclasses,shortName=cc,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion

// ClusterClass is a template which can be used to create managed topologies.
type ClusterClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterClassSpec `json:"spec,omitempty"`
}

// ClusterClassSpec describes the desired state of the ClusterClass.
type ClusterClassSpec struct {
	// Infrastructure is a reference to a provider-specific template that holds
	// the details for provisioning infrastructure specific cluster
	// for the underlying provider.
	// The underlying provider is responsible for the implementation
	// of the template to an infrastructure cluster.
	Infrastructure LocalObjectTemplate `json:"infrastructure,omitempty"`

	// ControlPlane is a reference to a local struct that holds the details
	// for provisioning the Control Plane for the Cluster.
	ControlPlane LocalObjectTemplate `json:"controlPlane,omitempty"`

	// Workers describes the worker nodes for the cluster.
	// It is a collection of node types which can be used to create
	// the worker nodes of the cluster.
	// +optional
	Workers WorkersClass `json:"workers,omitempty"`
}

// WorkersClass is a collection of deployment classes.
type WorkersClass struct {
	// MachineDeployments is a list of machine deployment classes that can be used to create
	// a set of worker nodes.
	MachineDeployments []MachineDeploymentClass `json:"machineDeployments,omitempty"`
}

// MachineDeploymentClass serves as a template to define a set of worker nodes of the cluster
// provisioned using the `ClusterClass`.
type MachineDeploymentClass struct {
	// Class denotes a type of worker node present in the cluster,
	// this name MUST be unique within a ClusterClass and can be referenced
	// in the Cluster to create a managed MachineDeployment.
	Class string `json:"class"`

	// Template is a local struct containing a collection of templates for creation of
	// MachineDeployment objects representing a set of worker nodes.
	Template MachineDeploymentClassTemplate `json:"template"`
}

// MachineDeploymentClassTemplate defines how a MachineDeployment generated from a MachineDeploymentClass
// should look like.
type MachineDeploymentClassTemplate struct {
	Metadata ObjectMeta `json:"metadata,omitempty"`

	// Bootstrap contains the bootstrap template reference to be used
	// for the creation of worker Machines.
	Bootstrap LocalObjectTemplate `json:"bootstrap"`

	// Infrastructure contains the infrastructure template reference to be used
	// for the creation of worker Machines.
	Infrastructure LocalObjectTemplate `json:"infrastructure"`
}

// LocalObjectTemplate defines a template for a topology Class.
type LocalObjectTemplate struct {
	// Ref is a required reference to a custom resource
	// offered by a provider.
	Ref *LocalObjectReference `json:"ref"`
}

// +kubebuilder:object:root=true

// ClusterClassList contains a list of Cluster.
type ClusterClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterClass{}, &ClusterClassList{})
}
