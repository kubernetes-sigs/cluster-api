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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1validation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation/field"

	capierrors "sigs.k8s.io/cluster-api/errors"
)

const (
	// MachineSetTopologyFinalizer is the finalizer used by the topology MachineDeployment controller to
	// clean up referenced template resources if necessary when a MachineSet is being deleted.
	MachineSetTopologyFinalizer = "machineset.topology.cluster.x-k8s.io"

	// MachineSetFinalizer is the finalizer used by the MachineSet controller to
	// ensure ordered cleanup of corresponding Machines when a Machineset is being deleted.
	MachineSetFinalizer = "cluster.x-k8s.io/machineset"

	// MachineSetMoveMachinesToMachineSetAnnotation is an internal annotation added by the MD controller to the oldMS
	// when it should scale down by moving machines that can be updated in-place to the newMS instead of deleting them.
	// The annotation value is the newMS name.
	// Note: This annotation is used in pair with MachineSetReceiveMachinesFromMachineSetsAnnotation to perform a two-ways check before moving a machine from oldMS to newMS:
	//
	//	"oldMS must have: move to newMS" and "newMS must have: receive replicas from oldMS"
	MachineSetMoveMachinesToMachineSetAnnotation = "in-place-updates.internal.cluster.x-k8s.io/move-machines-to-machineset"

	// MachineSetReceiveMachinesFromMachineSetsAnnotation is an internal annotation added by the MD controller to the newMS
	// when it should receive replicas from oldMSs as a first step of an in-place update operation
	// The annotation value is a comma separated list of oldMSs.
	// Note: This annotation is used in pair with MachineSetMoveMachinesToMachineSetAnnotation to perform a two-ways check before moving a machine from oldMS to newMS:
	//
	//	"oldMS must have: move to newMS" and "newMS must have: receive replicas from oldMS"
	MachineSetReceiveMachinesFromMachineSetsAnnotation = "in-place-updates.internal.cluster.x-k8s.io/receive-machines-from-machinesets"

	// AcknowledgedMoveAnnotation is an internal annotation with a list of machines added by the MD controller
	// to a MachineSet when it acknowledges a machine pending acknowledge after being moved from an oldMS.
	// The annotation value is a comma separated list of Machines already acknowledged; a machine is dropped
	// from this annotation as soon as pending-acknowledge-move is removed from the machine; the annotation is dropped when empty.
	// Note: This annotation is used in pair with PendingAcknowledgeMoveAnnotation on Machines.
	AcknowledgedMoveAnnotation = "in-place-updates.internal.cluster.x-k8s.io/acknowledged-move"
)

// MachineSetSpec defines the desired state of MachineSet.
type MachineSetSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`

	// replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	//
	// Defaults to:
	// * if the Kubernetes autoscaler min size and max size annotations are set:
	//   - if it's a new MachineSet, use min size
	//   - if the replicas field of the old MachineSet is < min size, use min size
	//   - if the replicas field of the old MachineSet is > max size, use max size
	//   - if the replicas field of the old MachineSet is in the (min size, max size) range, keep the value from the oldMS
	// * otherwise use 1
	// Note: Defaulting will be run whenever the replicas field is not set:
	// * A new MachineSet is created with replicas not set.
	// * On an existing MachineSet the replicas field was first set and is now unset.
	// Those cases are especially relevant for the following Kubernetes autoscaler use cases:
	// * A new MachineSet is created and replicas should be managed by the autoscaler
	// * An existing MachineSet which initially wasn't controlled by the autoscaler
	//   should be later controlled by the autoscaler
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// selector is a label query over machines that should match the replica count.
	// Label keys and values that must match in order to be controlled by this MachineSet.
	// It must match the machine template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	// +required
	Selector metav1.LabelSelector `json:"selector,omitempty,omitzero"`

	// template is the object that describes the machine that will be created if
	// insufficient replicas are detected.
	// Object references to custom resources are treated as templates.
	// +required
	Template MachineTemplateSpec `json:"template,omitempty,omitzero"`

	// machineNaming allows changing the naming pattern used when creating Machines.
	// Note: InfraMachines & BootstrapConfigs will use the same name as the corresponding Machines.
	// +optional
	MachineNaming MachineNamingSpec `json:"machineNaming,omitempty,omitzero"`

	// deletion contains configuration options for MachineSet deletion.
	// +optional
	Deletion MachineSetDeletionSpec `json:"deletion,omitempty,omitzero"`
}

// MachineSetDeletionSpec contains configuration options for MachineSet deletion.
// +kubebuilder:validation:MinProperties=1
type MachineSetDeletionSpec struct {
	// order defines the order in which Machines are deleted when downscaling.
	// Defaults to "Random".  Valid values are "Random, "Newest", "Oldest"
	// +optional
	Order MachineSetDeletionOrder `json:"order,omitempty"`
}

// MachineSet's ScalingUp condition and corresponding reasons.
const (
	// MachineSetScalingUpCondition is true if actual replicas < desired replicas.
	// Note: In case a MachineSet preflight check is preventing scale up, this will surface in the condition message.
	MachineSetScalingUpCondition = ScalingUpCondition

	// MachineSetScalingUpReason surfaces when actual replicas < desired replicas.
	MachineSetScalingUpReason = ScalingUpReason

	// MachineSetNotScalingUpReason surfaces when actual replicas >= desired replicas.
	MachineSetNotScalingUpReason = NotScalingUpReason

	// MachineSetScalingUpInternalErrorReason surfaces unexpected failures when listing machines.
	MachineSetScalingUpInternalErrorReason = InternalErrorReason

	// MachineSetScalingUpWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachineSet is not set.
	MachineSetScalingUpWaitingForReplicasSetReason = WaitingForReplicasSetReason
)

// MachineSet's ScalingDown condition and corresponding reasons.
const (
	// MachineSetScalingDownCondition is true if actual replicas > desired replicas.
	MachineSetScalingDownCondition = ScalingDownCondition

	// MachineSetScalingDownReason surfaces when actual replicas > desired replicas.
	MachineSetScalingDownReason = ScalingDownReason

	// MachineSetNotScalingDownReason surfaces when actual replicas <= desired replicas.
	MachineSetNotScalingDownReason = NotScalingDownReason

	// MachineSetScalingDownInternalErrorReason surfaces unexpected failures when listing machines.
	MachineSetScalingDownInternalErrorReason = InternalErrorReason

	// MachineSetScalingDownWaitingForReplicasSetReason surfaces when the .spec.replicas
	// field of the MachineSet is not set.
	MachineSetScalingDownWaitingForReplicasSetReason = WaitingForReplicasSetReason
)

// MachineSet's MachinesReady condition and corresponding reasons.
// Note: Reason's could also be derived from the aggregation of machine's Ready conditions.
const (
	// MachineSetMachinesReadyCondition surfaces detail of issues on the controlled machines, if any.
	MachineSetMachinesReadyCondition = MachinesReadyCondition

	// MachineSetMachinesReadyReason surfaces when all the controlled machine's Ready conditions are true.
	MachineSetMachinesReadyReason = ReadyReason

	// MachineSetMachinesNotReadyReason surfaces when at least one of the controlled machine's Ready conditions is false.
	MachineSetMachinesNotReadyReason = NotReadyReason

	// MachineSetMachinesReadyUnknownReason surfaces when at least one of the controlled machine's Ready conditions is unknown
	// and none of the controlled machine's Ready conditions is false.
	MachineSetMachinesReadyUnknownReason = ReadyUnknownReason

	// MachineSetMachinesReadyNoReplicasReason surfaces when no machines exist for the MachineSet.
	MachineSetMachinesReadyNoReplicasReason = NoReplicasReason

	// MachineSetMachinesReadyInternalErrorReason surfaces unexpected failures when listing machines
	// or aggregating machine's conditions.
	MachineSetMachinesReadyInternalErrorReason = InternalErrorReason
)

// MachineSet's MachinesUpToDate condition and corresponding reasons.
// Note: Reason's could also be derived from the aggregation of machine's MachinesUpToDate conditions.
const (
	// MachineSetMachinesUpToDateCondition surfaces details of controlled machines not up to date, if any.
	// Note: New machines are considered 10s after machine creation. This gives time to the machine's owner controller to recognize the new machine and add the UpToDate condition.
	MachineSetMachinesUpToDateCondition = MachinesUpToDateCondition

	// MachineSetMachinesUpToDateReason surfaces when all the controlled machine's UpToDate conditions are true.
	MachineSetMachinesUpToDateReason = UpToDateReason

	// MachineSetMachinesNotUpToDateReason surfaces when at least one of the controlled machine's UpToDate conditions is false.
	MachineSetMachinesNotUpToDateReason = NotUpToDateReason

	// MachineSetMachinesUpToDateUnknownReason surfaces when at least one of the controlled machine's UpToDate conditions is unknown
	// and none of the controlled machine's UpToDate conditions is false.
	MachineSetMachinesUpToDateUnknownReason = UpToDateUnknownReason

	// MachineSetMachinesUpToDateNoReplicasReason surfaces when no machines exist for the MachineSet.
	MachineSetMachinesUpToDateNoReplicasReason = NoReplicasReason

	// MachineSetMachinesUpToDateInternalErrorReason surfaces unexpected failures when listing machines
	// or aggregating status.
	MachineSetMachinesUpToDateInternalErrorReason = InternalErrorReason
)

// MachineSet's Remediating condition and corresponding reasons.
const (
	// MachineSetRemediatingCondition surfaces details about ongoing remediation of the controlled machines, if any.
	MachineSetRemediatingCondition = RemediatingCondition

	// MachineSetRemediatingReason surfaces when the MachineSet has at least one machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachineSetRemediatingReason = RemediatingReason

	// MachineSetNotRemediatingReason surfaces when the MachineSet does not have any machine with HealthCheckSucceeded set to false
	// and with the OwnerRemediated condition set to false.
	MachineSetNotRemediatingReason = NotRemediatingReason

	// MachineSetRemediatingInternalErrorReason surfaces unexpected failures when computing the Remediating condition.
	MachineSetRemediatingInternalErrorReason = InternalErrorReason
)

// Reasons that will be used for the OwnerRemediated condition set by MachineHealthCheck on MachineSet controlled machines
// being remediated in v1Beta2 API version.
const (
	// MachineSetMachineCannotBeRemediatedReason surfaces when remediation of a MachineSet machine can't be started.
	MachineSetMachineCannotBeRemediatedReason = "CannotBeRemediated"

	// MachineSetMachineRemediationDeferredReason surfaces when remediation of a MachineSet machine must be deferred.
	MachineSetMachineRemediationDeferredReason = "RemediationDeferred"

	// MachineSetMachineRemediationMachineDeletingReason surfaces when remediation of a MachineSet machine
	// has been completed by deleting the unhealthy machine.
	// Note: After an unhealthy machine is deleted, a new one is created by the MachineSet as part of the
	// regular reconcile loop that ensures the correct number of replicas exist.
	MachineSetMachineRemediationMachineDeletingReason = "MachineDeleting"
)

// MachineSet's Deleting condition and corresponding reasons.
const (
	// MachineSetDeletingCondition surfaces details about ongoing deletion of the controlled machines.
	MachineSetDeletingCondition = DeletingCondition

	// MachineSetNotDeletingReason surfaces when the MachineSet is not deleting because the
	// DeletionTimestamp is not set.
	MachineSetNotDeletingReason = NotDeletingReason

	// MachineSetDeletingReason surfaces when the MachineSet is deleting because the
	// DeletionTimestamp is set.
	MachineSetDeletingReason = DeletingReason

	// MachineSetDeletingInternalErrorReason surfaces unexpected failures when deleting a MachineSet.
	MachineSetDeletingInternalErrorReason = InternalErrorReason
)

// MachineTemplateSpec describes the data needed to create a Machine from a template.
type MachineTemplateSpec struct {
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec is the specification of the desired behavior of the machine.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +required
	Spec MachineSpec `json:"spec,omitempty,omitzero"`
}

// MachineSetDeletionOrder defines how priority is assigned to nodes to delete when
// downscaling a MachineSet. Defaults to "Random".
// +kubebuilder:validation:Enum=Random;Newest;Oldest
type MachineSetDeletionOrder string

const (
	// RandomMachineSetDeletionOrder prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// Finally, it picks Machines at random to delete.
	RandomMachineSetDeletionOrder MachineSetDeletionOrder = "Random"

	// NewestMachineSetDeletionOrder prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// It then prioritizes the newest Machines for deletion based on the Machine's CreationTimestamp.
	NewestMachineSetDeletionOrder MachineSetDeletionOrder = "Newest"

	// OldestMachineSetDeletionOrder prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// It then prioritizes the oldest Machines for deletion based on the Machine's CreationTimestamp.
	OldestMachineSetDeletionOrder MachineSetDeletionOrder = "Oldest"
)

// MachineSetStatus defines the observed state of MachineSet.
// +kubebuilder:validation:MinProperties=1
type MachineSetStatus struct {
	// conditions represents the observations of a MachineSet's current state.
	// Known condition types are MachinesReady, MachinesUpToDate, ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// selector is the same as the label selector but in the string format to avoid introspection
	// by clients. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=4096
	Selector string `json:"selector,omitempty"`

	// replicas is the most recently observed number of replicas.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// readyReplicas is the number of ready replicas for this MachineSet. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas for this MachineSet. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas for this MachineSet. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`

	// observedGeneration reflects the generation of the most recently observed MachineSet.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *MachineSetDeprecatedStatus `json:"deprecated,omitempty"`
}

// MachineSetDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineSetDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *MachineSetV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// MachineSetV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineSetV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the MachineSet.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// failureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	//
	// In the event that there is a terminal problem reconciling the
	// replicas, both FailureReason and FailureMessage will be set. FailureReason
	// will be populated with a succinct value suitable for machine
	// interpretation, while FailureMessage will contain a more verbose
	// string suitable for logging and human consumption.
	//
	// These fields should not be set for transitive errors that a
	// controller faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the MachineTemplate's spec or the configuration of
	// the machine controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the machine controller, or the
	// responsible machine controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the MachineSet object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FailureReason *capierrors.MachineSetStatusError `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage *string `json:"failureMessage,omitempty"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// fullyLabeledReplicas is the number of replicas that have labels matching the labels of the machine template of the MachineSet.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	FullyLabeledReplicas int32 `json:"fullyLabeledReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// readyReplicas is the number of ready replicas for this MachineSet. A machine is considered ready when the node has been created and is "Ready".
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed

	// availableReplicas is the number of available replicas (ready for at least minReadySeconds) for this MachineSet.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"` //nolint:kubeapilinter // field will be removed when v1beta1 is removed
}

// Validate validates the MachineSet fields.
func (m *MachineSet) Validate() field.ErrorList {
	errors := field.ErrorList{}

	// validate spec.selector and spec.template.labels
	fldPath := field.NewPath("spec")
	errors = append(errors, metav1validation.ValidateLabelSelector(&m.Spec.Selector, metav1validation.LabelSelectorValidationOptions{}, fldPath.Child("selector"))...)
	if len(m.Spec.Selector.MatchLabels)+len(m.Spec.Selector.MatchExpressions) == 0 {
		errors = append(errors, field.Invalid(fldPath.Child("selector"), m.Spec.Selector, "empty selector is not valid for MachineSet."))
	}
	selector, err := metav1.LabelSelectorAsSelector(&m.Spec.Selector)
	if err != nil {
		errors = append(errors, field.Invalid(fldPath.Child("selector"), m.Spec.Selector, "invalid label selector."))
	} else {
		labels := labels.Set(m.Spec.Template.Labels)
		if !selector.Matches(labels) {
			errors = append(errors, field.Invalid(fldPath.Child("template", "metadata", "labels"), m.Spec.Template.Labels, "`selector` does not match template `labels`"))
		}
	}

	return errors
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinesets,shortName=ms,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="The desired number of machines"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.replicas",description="The number of machines"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="The number of machines with Ready condition true"
// +kubebuilder:printcolumn:name="Available",type=integer,JSONPath=".status.availableReplicas",description="The number of machines with Available condition true"
// +kubebuilder:printcolumn:name="Up-to-date",type=integer,JSONPath=".status.upToDateReplicas",description="The number of machines with UpToDate condition true"
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachineSet"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.template.spec.version",description="Kubernetes version associated with this MachineSet"

// MachineSet is the Schema for the machinesets API.
type MachineSet struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of MachineSet.
	// +required
	Spec MachineSetSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of MachineSet.
	// +optional
	Status MachineSetStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for the MachineSet.
func (m *MachineSet) GetV1Beta1Conditions() Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions updates the set of conditions on the MachineSet.
func (m *MachineSet) SetV1Beta1Conditions(conditions Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &MachineSetDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &MachineSetV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *MachineSet) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *MachineSet) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachineSetList contains a list of MachineSet.
type MachineSetList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of MachineSets.
	Items []MachineSet `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachineSet{}, &MachineSetList{})
}
