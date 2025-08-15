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

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachinePoolFinalizer is used to ensure deletion of dependencies (nodes, infra).
	MachinePoolFinalizer = "machinepool.cluster.x-k8s.io"
)

/*
NOTE: we are commenting const for MachinePool's V1Beta2 conditions and reasons because not yet implemented for the 1.9 CAPI release.
However, we are keeping the v1beta2 struct in the MachinePool struct because the code that will collect conditions and replica
counters at cluster level is already implemented.

// Conditions that will be used for the MachinePool object in v1Beta2 API version.
const (
	// MachinePoolAvailableV1Beta2Condition is true when InfrastructureReady and available replicas >= desired replicas.
	MachinePoolAvailableV1Beta2Condition = clusterv1beta1.AvailableV1Beta2Condition

	// MachinePoolBootstrapConfigReadyV1Beta2Condition mirrors the corresponding condition from the MachinePool's BootstrapConfig resource.
	MachinePoolBootstrapConfigReadyV1Beta2Condition = clusterv1beta1.BootstrapConfigReadyV1Beta2Condition

	// MachinePoolInfrastructureReadyV1Beta2Condition mirrors the corresponding condition from the MachinePool's Infrastructure resource.
	MachinePoolInfrastructureReadyV1Beta2Condition = clusterv1beta1.InfrastructureReadyV1Beta2Condition

	// MachinePoolMachinesReadyV1Beta2Condition surfaces detail of issues on the controlled machines, if any.
	MachinePoolMachinesReadyV1Beta2Condition = clusterv1beta1.MachinesReadyV1Beta2Condition

	// MachinePoolMachinesUpToDateV1Beta2Condition surfaces details of controlled machines not up to date, if any.
	MachinePoolMachinesUpToDateV1Beta2Condition = clusterv1beta1.MachinesUpToDateV1Beta2Condition

	// MachinePoolScalingUpV1Beta2Condition is true if available replicas < desired replicas.
	MachinePoolScalingUpV1Beta2Condition = clusterv1beta1.ScalingUpV1Beta2Condition

	// MachinePoolScalingDownV1Beta2Condition is true if replicas > desired replicas.
	MachinePoolScalingDownV1Beta2Condition = clusterv1beta1.ScalingDownV1Beta2Condition

	// MachinePoolRemediatingV1Beta2Condition surfaces details about ongoing remediation of the controlled machines, if any.
	MachinePoolRemediatingV1Beta2Condition = clusterv1beta1.RemediatingV1Beta2Condition

	// MachinePoolDeletingV1Beta2Condition surfaces details about ongoing deletion of the controlled machines.
	MachinePoolDeletingV1Beta2Condition = clusterv1beta1.DeletingV1Beta2Condition
).
*/

// MachinePoolSpec defines the desired state of MachinePool.
type MachinePoolSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName"`

	// replicas is the number of desired machines. Defaults to 1.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// template describes the machines that will be created.
	// +required
	Template MachineTemplateSpec `json:"template"`

	// minReadySeconds is the minimum number of seconds for which a newly created machine instances should
	// be ready.
	// Defaults to 0 (machine instance will be considered available as soon as it
	// is ready)
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// providerIDList are the identification IDs of machine instances provided by the provider.
	// This field must match the provider IDs as seen on the node objects corresponding to a machine pool's machine instances.
	// +optional
	// +kubebuilder:validation:MaxItems=10000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	ProviderIDList []string `json:"providerIDList,omitempty"`

	// failureDomains is the list of failure domains this MachinePool should be attached to.
	// +optional
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	FailureDomains []string `json:"failureDomains,omitempty"`
}

// MachinePoolStatus defines the observed state of MachinePool.
type MachinePoolStatus struct {
	// nodeRefs will point to the corresponding Nodes if it they exist.
	// +optional
	// +kubebuilder:validation:MaxItems=10000
	NodeRefs []corev1.ObjectReference `json:"nodeRefs,omitempty"`

	// replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas"`

	// readyReplicas is the number of ready replicas for this MachinePool. A machine is considered ready when the node has been created and is "Ready".
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas (ready for at least minReadySeconds) for this MachinePool.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// unavailableReplicas is the total number of unavailable machine instances targeted by this machine pool.
	// This is the total number of machine instances that are still required for
	// the machine pool to have 100% available capacity. They may either
	// be machine instances that are running but not yet available or machine instances
	// that still have not been created.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// failureReason indicates that there is a problem reconciling the state, and
	// will be set to a token value suitable for programmatic interpretation.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason *capierrors.MachinePoolStatusFailure `json:"failureReason,omitempty"`

	// failureMessage indicates that there is a problem reconciling the state,
	// and will be set to a descriptive error message.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage *string `json:"failureMessage,omitempty"`

	// phase represents the current phase of cluster actuation.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Provisioning;Provisioned;Running;ScalingUp;ScalingDown;Scaling;Deleting;Failed;Unknown
	Phase string `json:"phase,omitempty"`

	// bootstrapReady is the state of the bootstrap provider.
	// +optional
	BootstrapReady bool `json:"bootstrapReady"`

	// infrastructureReady is the state of the infrastructure provider.
	// +optional
	InfrastructureReady bool `json:"infrastructureReady"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions define the current service state of the MachinePool.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in MachinePool's status with the V1Beta2 version.
	// +optional
	V1Beta2 *MachinePoolV1Beta2Status `json:"v1beta2,omitempty"`
}

// MachinePoolV1Beta2Status groups all the fields that will be added or modified in MachinePoolStatus with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachinePoolV1Beta2Status struct {
	// conditions represents the observations of a MachinePool's current state.
	// Known condition types are Available, BootstrapConfigReady, InfrastructureReady, MachinesReady, MachinesUpToDate,
	// ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// readyReplicas is the number of ready replicas for this MachinePool. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas for this MachinePool. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas targeted by this MachinePool. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`
}

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

	// MachinePoolPhaseScaling is the MachinePool state when the
	// MachinePool infrastructure is scaling.
	// This phase value is appropriate to indicate an active state of scaling by an external autoscaler.
	MachinePoolPhaseScaling = MachinePoolPhase("Scaling")

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
		MachinePoolPhaseScaling,
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
// +kubebuilder:deprecatedversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="Total number of machines desired by this MachinePool",priority=10
// +kubebuilder:printcolumn:name="Replicas",type="string",JSONPath=".status.replicas",description="MachinePool replicas count"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="MachinePool status such as Terminating/Pending/Provisioning/Running/Failed etc"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachinePool"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.template.spec.version",description="Kubernetes version associated with this MachinePool"
// +k8s:conversion-gen=false

// MachinePool is the Schema for the machinepools API.
type MachinePool struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of MachinePool.
	// +optional
	Spec MachinePoolSpec `json:"spec,omitempty"`
	// status is the observed state of MachinePool.
	// +optional
	Status MachinePoolStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (m *MachinePool) GetConditions() Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *MachinePool) SetConditions(conditions Conditions) {
	m.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (m *MachinePool) GetV1Beta2Conditions() []metav1.Condition {
	if m.Status.V1Beta2 == nil {
		return nil
	}
	return m.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (m *MachinePool) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if m.Status.V1Beta2 == nil {
		m.Status.V1Beta2 = &MachinePoolV1Beta2Status{}
	}
	m.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachinePoolList contains a list of MachinePool.
type MachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of MachinePools.
	Items []MachinePool `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachinePool{}, &MachinePoolList{})
}
