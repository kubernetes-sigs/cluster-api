/*
Copyright 2025 The Kubernetes Authors.

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

// Package v1beta2 contains test types.
// +kubebuilder:object:generate=true
// +groupName=test.cluster.x-k8s.io
package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=testresourcetemplates,scope=Namespaced
// +kubebuilder:storageversion

// TestResourceTemplate defines a test resource template.
type TestResourceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestResourceTemplateSpec `json:"spec,omitempty"`
}

// TestResourceTemplateSpec defines the spec of a TestResourceTemplate.
type TestResourceTemplateSpec struct {
	// +required
	Template TestResourceTemplateResource `json:"template"`
}

// TestResourceTemplateResource defines the template resource of a TestResourceTemplate.
type TestResourceTemplateResource struct {
	// +required
	Spec TestResourceSpec `json:"spec"`
}

// TestResourceTemplateList is a list of TestResourceTemplate.
// +kubebuilder:object:root=true
type TestResourceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestResourceTemplate `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=testresources,scope=Namespaced
// +kubebuilder:storageversion

// TestResource defines a test resource.
type TestResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestResourceSpec `json:"spec,omitempty"`
}

// TestResourceSpec defines the resource spec.
type TestResourceSpec struct {
	// mandatory field from the Cluster API control plane contract - replicas support

	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Mandatory field from the Cluster API control plane contract - version support
	// Note: field is not required because this CRD is also used for non - control plane cases.

	// +optional
	Version string `json:"version,omitempty"`

	// Mandatory field from the Cluster API control plane contract - machine support
	// Note: field is not required because this CRD is also used for non - control plane cases.

	// +required
	MachineTemplate TestResourceMachineTemplateSpec `json:"machineTemplate"`

	// General purpose fields to be used in different test scenario.

	// +optional
	Omittable string `json:"omittable,omitempty"`
}

// TestResourceMachineTemplateSpec define the spec for machineTemplate in a resource.
// Note: infrastructureRef field is not required because this CRD is also used for non - control plane cases.
type TestResourceMachineTemplateSpec struct {
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	InfrastructureRef TestContractVersionedObjectReference `json:"infrastructureRef"`
}

// TestContractVersionedObjectReference is a reference to a resource for which the version is inferred from contract labels.
// Note: fields are not required / do not have validation for sake of simplicity (not relevant for the test).
type TestContractVersionedObjectReference struct {
	// +optional
	Kind string `json:"kind"`

	// +optional
	Name string `json:"name"`

	// +optional
	APIGroup string `json:"apiGroup"`
}

// TestResourceStatus defines the status of a TestResource.
type TestResourceStatus struct {
	// mandatory field from the Cluster API contract - replicas support

	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// Mandatory field from the Cluster API contract - version support

	// +optional
	Version *string `json:"version,omitempty"`
}

// TestResourceList is a list of TestResource.
// +kubebuilder:object:root=true
type TestResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestResource `json:"items"`
}
