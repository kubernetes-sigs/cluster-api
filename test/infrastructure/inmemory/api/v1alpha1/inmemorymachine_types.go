/*
Copyright 2023 The Kubernetes Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// MachineFinalizer allows ReconcileInMemoryMachine to clean up resources associated with InMemoryMachine before
	// removing it from the API server.
	MachineFinalizer = "inmemorymachine.infrastructure.cluster.x-k8s.io"
)

// InMemoryMachineSpec defines the desired state of InMemoryMachine.
type InMemoryMachineSpec struct {
	// ProviderID will be the container name in ProviderID format (in-memory:////<name>)
	// +optional
	ProviderID *string `json:"providerID,omitempty"`
}

// InMemoryMachineStatus defines the observed state of InMemoryMachine.
type InMemoryMachineStatus struct {
	// Ready denotes that the machine is ready
	// +optional
	Ready bool `json:"ready"`

	// Conditions defines current service state of the InMemoryMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:resource:path=inmemorymachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Machine",type="string",JSONPath=".metadata.ownerReferences[?(@.kind==\"Machine\")].name",description="Machine object which owns with this InMemoryMachine"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine ready status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of InMemoryMachine"

// InMemoryMachine is the schema for the in-memory machine API.
type InMemoryMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InMemoryMachineSpec   `json:"spec,omitempty"`
	Status InMemoryMachineStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *InMemoryMachine) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *InMemoryMachine) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// InMemoryMachineList contains a list of InMemoryMachine.
type InMemoryMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InMemoryMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InMemoryMachine{}, &InMemoryMachineList{})
}
