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

package v1beta2

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	capierrors "sigs.k8s.io/cluster-api/api/deprecated/errors"
)

const (
	// MachinePoolFinalizer is used to ensure deletion of dependencies (nodes, infra).
	MachinePoolFinalizer = "machinepool.cluster.x-k8s.io"
)

// MachinePool's Available condition and corresponding reasons.
const (
	// MachinePoolAvailableCondition is true when InfrastructureReady and available replicas >= desired replicas.
	MachinePoolAvailableCondition = AvailableCondition

	// MachinePoolAvailableWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachinePool is not set.
	MachinePoolAvailableWaitingForReplicasSetReason = WaitingForReplicasSetReason

	// MachinePoolAvailableWaitingForAvailableReplicasSetReason surfaces when the .status.availableReplicas
	// field of the MachinePool is not set.
	MachinePoolAvailableWaitingForAvailableReplicasSetReason = "WaitingForAvailableReplicasSet"

	// MachinePoolAvailableReason surfaces when a MachinePool is available.
	MachinePoolAvailableReason = AvailableReason

	// MachinePoolNotAvailableReason surfaces when a MachinePool is not available.
	MachinePoolNotAvailableReason = NotAvailableReason

	// MachinePoolAvailableReplicaCountersNotObservedReason surfaces when the replica counters required to compute
	// the Available condition were not observed during the current reconcile.
	MachinePoolAvailableReplicaCountersNotObservedReason = "ReplicaCountersNotObserved"

	// MachinePoolAvailableInternalErrorReason surfaces unexpected failures when computing the Available condition.
	MachinePoolAvailableInternalErrorReason = InternalErrorReason
)

// MachinePool's BootstrapConfigReady condition and corresponding reasons.
const (
	// MachinePoolBootstrapConfigReadyCondition mirrors the corresponding Ready condition from the MachinePool's BootstrapConfig resource.
	MachinePoolBootstrapConfigReadyCondition = BootstrapConfigReadyCondition

	// MachinePoolBootstrapConfigReadyReason surfaces when the MachinePool bootstrap config is ready.
	MachinePoolBootstrapConfigReadyReason = ReadyReason

	// MachinePoolBootstrapDataSecretProvidedReason surfaces when a bootstrap data secret is provided (not originated
	// from a BootstrapConfig object referenced from the MachinePool).
	MachinePoolBootstrapDataSecretProvidedReason = "DataSecretProvided"

	// MachinePoolBootstrapConfigNotReadyReason surfaces when the MachinePool bootstrap config is not ready.
	MachinePoolBootstrapConfigNotReadyReason = NotReadyReason

	// MachinePoolBootstrapConfigInvalidConditionReportedReason surfaces a BootstrapConfig Ready condition (read from a bootstrap config object) which is invalid.
	MachinePoolBootstrapConfigInvalidConditionReportedReason = InvalidConditionReportedReason

	// MachinePoolBootstrapConfigInternalErrorReason surfaces unexpected failures when reading a BootstrapConfig object.
	MachinePoolBootstrapConfigInternalErrorReason = InternalErrorReason

	// MachinePoolBootstrapConfigDoesNotExistReason surfaces when a referenced bootstrap config object does not exist.
	MachinePoolBootstrapConfigDoesNotExistReason = ObjectDoesNotExistReason

	// MachinePoolBootstrapConfigDeletedReason surfaces when a referenced bootstrap config object has been deleted.
	MachinePoolBootstrapConfigDeletedReason = ObjectDeletedReason
)

// MachinePool's InfrastructureReady condition and corresponding reasons.
const (
	// MachinePoolInfrastructureReadyCondition mirrors the corresponding Ready condition from the MachinePool's Infrastructure resource.
	MachinePoolInfrastructureReadyCondition = InfrastructureReadyCondition

	// MachinePoolInfrastructureReadyReason surfaces when the MachinePool infrastructure is ready.
	MachinePoolInfrastructureReadyReason = ReadyReason

	// MachinePoolInfrastructureNotReadyReason surfaces when the MachinePool infrastructure is not ready.
	MachinePoolInfrastructureNotReadyReason = NotReadyReason

	// MachinePoolInfrastructureInvalidConditionReportedReason surfaces an infrastructure Ready condition (read from an infra machine pool object) which is invalid.
	MachinePoolInfrastructureInvalidConditionReportedReason = InvalidConditionReportedReason

	// MachinePoolInfrastructureInternalErrorReason surfaces unexpected failures when reading an infra machine pool object.
	MachinePoolInfrastructureInternalErrorReason = InternalErrorReason

	// MachinePoolInfrastructureDoesNotExistReason surfaces when a referenced infrastructure object does not exist.
	MachinePoolInfrastructureDoesNotExistReason = ObjectDoesNotExistReason

	// MachinePoolInfrastructureDeletedReason surfaces when a referenced infrastructure object has been deleted.
	MachinePoolInfrastructureDeletedReason = ObjectDeletedReason
)

// MachinePoolMachineSupportUnknownReason surfaces when the controller cannot determine if a MachinePool uses per-instance Machines.
const MachinePoolMachineSupportUnknownReason = "MachineSupportUnknown"

// MachinePool's MachinesReady condition and corresponding reasons.
const (
	// MachinePoolMachinesReadyCondition surfaces detail of issues on the controlled machines, if any.
	MachinePoolMachinesReadyCondition = MachinesReadyCondition

	// MachinePoolMachinesReadyReason surfaces when all the controlled machine's Ready conditions are true.
	MachinePoolMachinesReadyReason = ReadyReason

	// MachinePoolMachinesNotReadyReason surfaces when at least one of the controlled machine's Ready conditions is false.
	MachinePoolMachinesNotReadyReason = NotReadyReason

	// MachinePoolMachinesReadyUnknownReason surfaces when at least one of the controlled machine's Ready conditions is unknown
	// and none of the controlled machine's Ready conditions is false.
	MachinePoolMachinesReadyUnknownReason = ReadyUnknownReason

	// MachinePoolMachinesReadyNoReplicasReason surfaces when no machines exist for the MachinePool.
	MachinePoolMachinesReadyNoReplicasReason = NoReplicasReason

	// MachinePoolMachinesReadyInternalErrorReason surfaces unexpected failures when listing machines
	// or aggregating machine's conditions.
	MachinePoolMachinesReadyInternalErrorReason = InternalErrorReason
)

// MachinePool's MachinesUpToDate condition and corresponding reasons.
const (
	// MachinePoolMachinesUpToDateCondition surfaces details of controlled machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	MachinePoolMachinesUpToDateCondition = MachinesUpToDateCondition

	// MachinePoolMachinesUpToDateReason surfaces when all the controlled machine's UpToDate conditions are true.
	MachinePoolMachinesUpToDateReason = UpToDateReason

	// MachinePoolMachinesNotUpToDateReason surfaces when at least one of the controlled machine's UpToDate conditions is false.
	MachinePoolMachinesNotUpToDateReason = NotUpToDateReason

	// MachinePoolMachinesUpToDateUnknownReason surfaces when at least one of the controlled machine's UpToDate conditions is unknown
	// and none of the controlled machine's UpToDate conditions is false.
	MachinePoolMachinesUpToDateUnknownReason = UpToDateUnknownReason

	// MachinePoolMachinesUpToDateNoReplicasReason surfaces when no machines exist for the MachinePool.
	MachinePoolMachinesUpToDateNoReplicasReason = NoReplicasReason

	// MachinePoolMachinesUpToDateInternalErrorReason surfaces unexpected failures when listing machines
	// or aggregating status.
	MachinePoolMachinesUpToDateInternalErrorReason = InternalErrorReason
)

// MachinePool's RollingOut condition and corresponding reasons.
const (
	// MachinePoolRollingOutCondition is true if there is at least one machine not up-to-date.
	MachinePoolRollingOutCondition = RollingOutCondition

	// MachinePoolRollingOutReason surfaces when there is at least one machine not up-to-date.
	MachinePoolRollingOutReason = RollingOutReason

	// MachinePoolNotRollingOutReason surfaces when all the machines are up-to-date.
	MachinePoolNotRollingOutReason = NotRollingOutReason

	// MachinePoolRollingOutInternalErrorReason surfaces unexpected failures when listing machines.
	MachinePoolRollingOutInternalErrorReason = InternalErrorReason
)

// MachinePool's ScalingUp condition and corresponding reasons.
const (
	// MachinePoolScalingUpCondition is true if actual replicas < desired replicas.
	MachinePoolScalingUpCondition = ScalingUpCondition

	// MachinePoolScalingUpReason surfaces when actual replicas < desired replicas.
	MachinePoolScalingUpReason = ScalingUpReason

	// MachinePoolNotScalingUpReason surfaces when actual replicas >= desired replicas.
	MachinePoolNotScalingUpReason = NotScalingUpReason

	// MachinePoolScalingUpReplicasNotObservedReason surfaces when the provider replica count was not observed
	// during the current reconcile.
	MachinePoolScalingUpReplicasNotObservedReason = "ReplicasNotObserved"

	// MachinePoolScalingUpInternalErrorReason surfaces unexpected failures when listing machines.
	MachinePoolScalingUpInternalErrorReason = InternalErrorReason

	// MachinePoolScalingUpWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachinePool is not set.
	MachinePoolScalingUpWaitingForReplicasSetReason = WaitingForReplicasSetReason
)

// MachinePool's ScalingDown condition and corresponding reasons.
const (
	// MachinePoolScalingDownCondition is true if actual replicas > desired replicas.
	MachinePoolScalingDownCondition = ScalingDownCondition

	// MachinePoolScalingDownReason surfaces when actual replicas > desired replicas.
	MachinePoolScalingDownReason = ScalingDownReason

	// MachinePoolNotScalingDownReason surfaces when actual replicas <= desired replicas.
	MachinePoolNotScalingDownReason = NotScalingDownReason

	// MachinePoolScalingDownReplicasNotObservedReason surfaces when the provider replica count was not observed
	// during the current reconcile.
	MachinePoolScalingDownReplicasNotObservedReason = "ReplicasNotObserved"

	// MachinePoolScalingDownInternalErrorReason surfaces unexpected failures when listing machines.
	MachinePoolScalingDownInternalErrorReason = InternalErrorReason

	// MachinePoolScalingDownWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachinePool is not set.
	MachinePoolScalingDownWaitingForReplicasSetReason = WaitingForReplicasSetReason
)

// MachinePool's Remediating condition and corresponding reasons.
const (
	// MachinePoolRemediatingCondition surfaces details about ongoing remediation of the controlled machines, if any.
	MachinePoolRemediatingCondition = RemediatingCondition

	// MachinePoolRemediatingReason surfaces when the MachinePool has at least one machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachinePoolRemediatingReason = RemediatingReason

	// MachinePoolNotRemediatingReason surfaces when the MachinePool does not have any machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachinePoolNotRemediatingReason = NotRemediatingReason

	// MachinePoolRemediatingInternalErrorReason surfaces unexpected failures when computing the Remediating condition.
	MachinePoolRemediatingInternalErrorReason = InternalErrorReason
)

// MachinePool's Deleting condition and corresponding reasons.
const (
	// MachinePoolDeletingCondition surfaces details about ongoing deletion of the controlled machines.
	MachinePoolDeletingCondition = DeletingCondition

	// MachinePoolNotDeletingReason surfaces when the MachinePool is not deleting because the
	// DeletionTimestamp is not set.
	MachinePoolNotDeletingReason = NotDeletingReason

	// MachinePoolDeletingReason surfaces when the MachinePool is deleting because the
	// DeletionTimestamp is set.
	MachinePoolDeletingReason = DeletingReason

	// MachinePoolDeletingInternalErrorReason surfaces unexpected failures when deleting a MachinePool.
	MachinePoolDeletingInternalErrorReason = InternalErrorReason
)

// MachinePoolSpec defines the desired state of MachinePool.
type MachinePoolSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`

	// replicas is the number of desired machines. Defaults to 1.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// template describes the machines that will be created.
	// +required
	Template MachineTemplateSpec `json:"template,omitempty,omitzero"`

	// providerIDList are the identification IDs of machine instances provided by the provider.
	// This field must match the provider IDs as seen on the node objects corresponding to a machine pool's machine instances.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=10000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=512
	ProviderIDList []string `json:"providerIDList,omitempty"`

	// failureDomains is the list of failure domains this MachinePool should be attached to.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=256
	FailureDomains []string `json:"failureDomains,omitempty"`
}

// MachinePoolStatus defines the observed state of MachinePool.
// +kubebuilder:validation:MinProperties=1
type MachinePoolStatus struct {
	// conditions represents the observations of a MachinePool's current state.
	// Known condition types are Available, BootstrapConfigReady, InfrastructureReady, MachinesReady, MachinesUpToDate,
	// RollingOut, ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// initialization provides observations of the MachinePool initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial MachinePool provisioning.
	// +optional
	Initialization MachinePoolInitializationStatus `json:"initialization,omitempty,omitzero"`

	// nodeRefs will point to the corresponding Nodes if they exist.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=10000
	NodeRefs []corev1.ObjectReference `json:"nodeRefs,omitempty"`

	// replicas is the most recently observed number of replicas.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// readyReplicas is the number of ready replicas for this MachinePool. A machine is considered ready when Machine's Ready condition is true.
	// For MachinePools without Machines, this is the number of corresponding Nodes with the Ready condition true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas for this MachinePool. A machine is considered available when Machine's Available condition is true.
	// For MachinePools without Machines, this is the number of corresponding Nodes that are ready for at least minReadySeconds.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas targeted by this MachinePool. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// This field is not reported for MachinePools without Machines.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// versions is the aggregated Kubernetes versions in this MachinePool.
	// +optional
	// +listType=map
	// +listMapKey=version
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Versions []StatusVersion `json:"versions,omitempty"`

	// phase represents the current phase of cluster actuation.
	// +optional
	// +kubebuilder:validation:Enum=Pending;Provisioning;Provisioned;Running;ScalingUp;ScalingDown;Scaling;Deleting;Failed;Unknown
	Phase string `json:"phase,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *MachinePoolDeprecatedStatus `json:"deprecated,omitempty"`
}

// MachinePoolInitializationStatus provides observations of the MachinePool initialization process.
// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial MachinePool provisioning.
// +kubebuilder:validation:MinProperties=1
type MachinePoolInitializationStatus struct {
	// infrastructureProvisioned is true when the infrastructure provider reports that MachinePool's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed.
	// +optional
	InfrastructureProvisioned *bool `json:"infrastructureProvisioned,omitempty"`

	// bootstrapDataSecretCreated is true when the bootstrap provider reports that the MachinePool's boostrap secret is created.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate provisioning.
	// The value of this field is never updated after provisioning is completed.
	// +optional
	BootstrapDataSecretCreated *bool `json:"bootstrapDataSecretCreated,omitempty"`
}

// MachinePoolDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachinePoolDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *MachinePoolV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// MachinePoolV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachinePoolV1Beta1DeprecatedStatus struct {
	// conditions define the current service state of the MachinePool.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

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
	FailureMessage *string `json:"failureMessage,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// readyReplicas is the number of ready replicas for this MachinePool. A machine is considered ready when the node has been created and is "Ready".
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// availableReplicas is the number of available replicas (ready for at least minReadySeconds) for this MachinePool.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// unavailableReplicas is the total number of unavailable machine instances targeted by this machine pool.
	// This is the total number of machine instances that are still required for
	// the machine pool to have 100% available capacity. They may either
	// be machine instances that are running but not yet available or machine instances
	// that still have not been created.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed
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
	//
	// Deprecated: This enum value is deprecated; the Failed phase won't be set anymore by controllers, and it is preserved only
	// for conversion from v1beta1 objects; the Failed phase is going to be removed when support for v1beta1 will be dropped.
	// Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
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
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="The desired number of machines"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.replicas",description="The number of machines"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="The number of machines with Ready condition true"
// +kubebuilder:printcolumn:name="Available",type=integer,JSONPath=".status.availableReplicas",description="The number of machines with Available condition true"
// +kubebuilder:printcolumn:name="Up-to-date",type=integer,JSONPath=".status.upToDateReplicas",description="The number of machines with UpToDate condition true"
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="MachinePool status such as Terminating/Pending/Provisioning/Running/Failed etc"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachinePool"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.template.spec.version",description="Kubernetes version associated with this MachinePool"
// +k8s:conversion-gen=false

// MachinePool is the Schema for the machinepools API.
// NOTE: This CRD can only be used if the MachinePool feature gate is enabled.
type MachinePool struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of MachinePool.
	// +required
	Spec MachinePoolSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of MachinePool.
	// +optional
	Status MachinePoolStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (m *MachinePool) GetV1Beta1Conditions() Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (m *MachinePool) SetV1Beta1Conditions(conditions Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &MachinePoolDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &MachinePoolV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *MachinePool) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *MachinePool) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
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
