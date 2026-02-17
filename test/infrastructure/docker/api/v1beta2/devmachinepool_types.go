/*
Copyright 2026 The Kubernetes Authors.

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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// DevMachinePoolFinalizer allows ReconcileDevMachinePool to clean up resources.
	DevMachinePoolFinalizer = "devmachinepool.infrastructure.cluster.x-k8s.io"
)

const (
	// ReplicasReadyCondition reports an aggregate of current status of the replicas controlled by the MachinePool.
	ReplicasReadyCondition string = "ReplicasReady"

	// ReplicasReadyReason surfaces when the DevMachinePool ReplicasReadyConditio is met.
	ReplicasReadyReason string = clusterv1.ReadyReason
)

// DevMachinePool's conditions that apply to all the supported backends.

// DevMachinePool's Ready condition and corresponding reasons.
const (
	// DevMachinePoolReadyCondition is true if
	// - The DevMachinePool's is using a docker backend and ReplicasReadyCondition is true.
	DevMachinePoolReadyCondition = clusterv1.ReadyCondition

	// DevMachinePoolReadyReason surfaces when the DevMachinePool readiness criteria is met.
	DevMachinePoolReadyReason = clusterv1.ReadyReason

	// DevMachinePoolNotReadyReason surfaces when the DevMachinePool readiness criteria is not met.
	DevMachinePoolNotReadyReason = clusterv1.NotReadyReason

	// DevMachinePoolReadyUnknownReason surfaces when at least one DevMachinePool readiness criteria is unknown
	// and no DevMachinePool readiness criteria is not met.
	DevMachinePoolReadyUnknownReason = clusterv1.ReadyUnknownReason
)

// +kubebuilder:resource:path=devmachinepools,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of DevMachinePool"

// DevMachinePool is the Schema for the devmachinepools API.
type DevMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DevMachinePoolSpec   `json:"spec,omitempty"`
	Status DevMachinePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DevMachinePoolList contains a list of DevMachinePool.
type DevMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DevMachinePool `json:"items"`
}

// DevMachinePoolSpec defines the desired state of DevMachinePool.
type DevMachinePoolSpec struct {
	// ProviderID is the identification ID of the Machine Pool
	// +optional
	ProviderID string `json:"providerID,omitempty"`

	// ProviderIDList is the list of identification IDs of machine instances managed by this Machine Pool
	// +optional
	ProviderIDList []string `json:"providerIDList,omitempty"`

	// Template contains the details used to build a replica machine within the Machine Pool
	// +optional
	Template DevMachinePoolBackendTemplate `json:"template"`
}

// DevMachinePoolBackendTemplate defines backends for a DevMachinePool.
type DevMachinePoolBackendTemplate struct {
	// docker defines a backend for a DevMachine using docker containers.
	// +optional
	Docker *DockerMachinePoolMachineTemplate `json:"docker,omitempty"`
}

// DevMachinePoolStatus defines the observed state of DevMachinePool.
type DevMachinePoolStatus struct {
	// conditions represents the observations of a DevMachinePool's current state.
	// Known condition types are Ready, ReplicasReady, Resized, ReplicasReady.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready denotes that the machine pool is ready
	// +optional
	Ready bool `json:"ready"`

	// Replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas"`

	// The generation observed by the deployment controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// InfrastructureMachineKind is the kind of the infrastructure resources behind MachinePool Machines.
	// +optional
	InfrastructureMachineKind string `json:"infrastructureMachineKind,omitempty"`

	// Instances contains the status for each instance in the pool
	// +optional
	Instances []DevMachinePoolBackendInstanceStatus `json:"instances,omitempty"`
}

// DevMachinePoolBackendInstanceStatus contains status information about a DevMachinePool instances.
type DevMachinePoolBackendInstanceStatus struct {
	// docker define backend status for a DevMachine for a machine using docker containers.
	// +optional
	Docker *DockerMachinePoolInstanceStatus `json:"docker,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (d *DevMachinePool) GetConditions() []metav1.Condition {
	return d.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (d *DevMachinePool) SetConditions(conditions []metav1.Condition) {
	d.Status.Conditions = conditions
}

func init() {
	objectTypes = append(objectTypes, &DevMachinePool{}, &DevMachinePoolList{})
}
