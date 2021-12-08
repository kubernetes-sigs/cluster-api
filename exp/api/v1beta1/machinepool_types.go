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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachinePoolFinalizer is used to ensure deletion of dependencies (nodes, infra).
	MachinePoolFinalizer = "machinepool.cluster.x-k8s.io"
)

// ANCHOR: MachinePoolSpec

// MachinePoolSpec defines the desired state of MachinePool.
type MachinePoolSpec struct {
	// ClusterName is the name of the Cluster this object belongs to.
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// Number of desired machines. Defaults to 1.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Template describes the machines that will be created.
	Template clusterv1.MachineTemplateSpec `json:"template"`

	// Minimum number of seconds for which a newly created machine instances should
	// be ready.
	// Defaults to 0 (machine instance will be considered available as soon as it
	// is ready)
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// ProviderIDList are the identification IDs of machine instances provided by the provider.
	// This field must match the provider IDs as seen on the node objects corresponding to a machine pool's machine instances.
	// +optional
	ProviderIDList []string `json:"providerIDList,omitempty"`

	// FailureDomains is the list of failure domains this MachinePool should be attached to.
	// +optional
	FailureDomains []string `json:"failureDomains,omitempty"`
}

// ANCHOR_END: MachinePoolSpec

// ANCHOR: MachinePoolStatus

// MachinePoolStatus defines the observed state of MachinePool.
type MachinePoolStatus struct {
	// NodeRefs will point to the corresponding Nodes if it they exist.
	// +optional
	NodeRefs []corev1.ObjectReference `json:"nodeRefs,omitempty"`

	// Replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas"`

	// The number of ready replicas for this MachinePool. A machine is considered ready when the node has been created and is "Ready".
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// The number of available replicas (ready for at least minReadySeconds) for this MachinePool.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Total number of unavailable machine instances targeted by this machine pool.
	// This is the total number of machine instances that are still required for
	// the machine pool to have 100% available capacity. They may either
	// be machine instances that are running but not yet available or machine instances
	// that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// FailureReason indicates that there is a problem reconciling the state, and
	// will be set to a token value suitable for programmatic interpretation.
	// +optional
	FailureReason *capierrors.MachinePoolStatusFailure `json:"failureReason,omitempty"`

	// FailureMessage indicates that there is a problem reconciling the state,
	// and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// Phase represents the current phase of cluster actuation.
	// E.g. Pending, Running, Terminating, Failed etc.
	// +optional
	Phase string `json:"phase,omitempty"`

	// BootstrapReady is the state of the bootstrap provider.
	// +optional
	BootstrapReady bool `json:"bootstrapReady"`

	// InfrastructureReady is the state of the infrastructure provider.
	// +optional
	InfrastructureReady bool `json:"infrastructureReady"`

	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions define the current service state of the MachinePool.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// ANCHOR_END: MachinePoolStatus

// MachinePoolPhase is a string representation of a MachinePool Phase.
//
// This type is a high-level indicator of the status of the MachinePool as it is provisioned,
// from the API user’s perspective.
//
// The value should not be interpreted by any software components as a reliable indication
// of the actual state of the MachinePool, and controllers should not use the MachinePool Phase field
// value when making decisions about what action to take.
//
// Controllers should always look at the actual state of the MachinePool’s fields to make those decisions.
type MachinePoolPhase string

const (
	// MachinePoolPhasePending is the first state a MachinePool is assigned by
	// Cluster API MachinePool controller after being created.
	MachinePoolPhasePending = MachinePoolPhase("Pending")

	// MachinePoolPhaseProvisioning is the state when the
	// MachinePool infrastructure is being created or updated.
	MachinePoolPhaseProvisioning = MachinePoolPhase("Provisioning")

	// MachinePoolPhaseProvisioned is the state when its
	// infrastructure has been created and configured.
	MachinePoolPhaseProvisioned = MachinePoolPhase("Provisioned")

	// MachinePoolPhaseRunning is the MachinePool state when its instances
	// have become Kubernetes Nodes in the Ready state.
	MachinePoolPhaseRunning = MachinePoolPhase("Running")

	// MachinePoolPhaseScalingUp is the MachinePool state when the
	// MachinePool infrastructure is scaling up.
	MachinePoolPhaseScalingUp = MachinePoolPhase("ScalingUp")

	// MachinePoolPhaseScalingDown is the MachinePool state when the
	// MachinePool infrastructure is scaling down.
	MachinePoolPhaseScalingDown = MachinePoolPhase("ScalingDown")

	// MachinePoolPhaseDeleting is the MachinePool state when a delete
	// request has been sent to the API Server,
	// but its infrastructure has not yet been fully deleted.
	MachinePoolPhaseDeleting = MachinePoolPhase("Deleting")

	// MachinePoolPhaseFailed is the MachinePool state when the system
	// might require user intervention.
	MachinePoolPhaseFailed = MachinePoolPhase("Failed")

	// MachinePoolPhaseUnknown is returned if the MachinePool state cannot be determined.
	MachinePoolPhaseUnknown = MachinePoolPhase("Unknown")
)

// SetTypedPhase sets the Phase field to the string representation of MachinePoolPhase.
func (m *MachinePoolStatus) SetTypedPhase(p MachinePoolPhase) {
	m.Phase = string(p)
}

// GetTypedPhase attempts to parse the Phase field and return
// the typed MachinePoolPhase representation as described in `machinepool_phase_types.go`.
func (m *MachinePoolStatus) GetTypedPhase() MachinePoolPhase {
	switch phase := MachinePoolPhase(m.Phase); phase {
	case
		MachinePoolPhasePending,
		MachinePoolPhaseProvisioning,
		MachinePoolPhaseProvisioned,
		MachinePoolPhaseRunning,
		MachinePoolPhaseScalingUp,
		MachinePoolPhaseScalingDown,
		MachinePoolPhaseDeleting,
		MachinePoolPhaseFailed:
		return phase
	default:
		return MachinePoolPhaseUnknown
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinepools,shortName=mp,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Replicas",type="string",JSONPath=".status.replicas",description="MachinePool replicas count"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="MachinePool status such as Terminating/Pending/Provisioning/Running/Failed etc"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachinePool"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.template.spec.version",description="Kubernetes version associated with this MachinePool"
// +k8s:conversion-gen=false

// MachinePool is the Schema for the machinepools API.
type MachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachinePoolSpec   `json:"spec,omitempty"`
	Status MachinePoolStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (m *MachinePool) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *MachinePool) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachinePoolList contains a list of MachinePool.
type MachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MachinePool{}, &MachinePoolList{})
}
