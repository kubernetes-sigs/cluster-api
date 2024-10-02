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
)

// ANCHOR: MachineSetSpec

// MachineSetSpec defines the desired state of MachineSet.
type MachineSetSpec struct {
	// ClusterName is the name of the Cluster this object belongs to.
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// Replicas is the number of desired replicas.
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

	// MinReadySeconds is the minimum number of seconds for which a Node for a newly created machine should be ready before considering the replica available.
	// Defaults to 0 (machine will be considered available as soon as the Node is ready)
	// +optional
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// DeletePolicy defines the policy used to identify nodes to delete when downscaling.
	// Defaults to "Random".  Valid values are "Random, "Newest", "Oldest"
	// +kubebuilder:validation:Enum=Random;Newest;Oldest
	// +optional
	DeletePolicy string `json:"deletePolicy,omitempty"`

	// Selector is a label query over machines that should match the replica count.
	// Label keys and values that must match in order to be controlled by this MachineSet.
	// It must match the machine template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	Selector metav1.LabelSelector `json:"selector"`

	// Template is the object that describes the machine that will be created if
	// insufficient replicas are detected.
	// Object references to custom resources are treated as templates.
	// +optional
	Template MachineTemplateSpec `json:"template,omitempty"`
}

// ANCHOR_END: MachineSetSpec

// ANCHOR: MachineTemplateSpec

// MachineTemplateSpec describes the data needed to create a Machine from a template.
type MachineTemplateSpec struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the machine.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec MachineSpec `json:"spec,omitempty"`
}

// ANCHOR_END: MachineTemplateSpec

// MachineSetDeletePolicy defines how priority is assigned to nodes to delete when
// downscaling a MachineSet. Defaults to "Random".
type MachineSetDeletePolicy string

const (
	// RandomMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// Finally, it picks Machines at random to delete.
	RandomMachineSetDeletePolicy MachineSetDeletePolicy = "Random"

	// NewestMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// It then prioritizes the newest Machines for deletion based on the Machine's CreationTimestamp.
	NewestMachineSetDeletePolicy MachineSetDeletePolicy = "Newest"

	// OldestMachineSetDeletePolicy prioritizes both Machines that have the annotation
	// "cluster.x-k8s.io/delete-machine=yes" and Machines that are unhealthy
	// (Status.FailureReason or Status.FailureMessage are set to a non-empty value
	// or NodeHealthy type of Status.Conditions is not true).
	// It then prioritizes the oldest Machines for deletion based on the Machine's CreationTimestamp.
	OldestMachineSetDeletePolicy MachineSetDeletePolicy = "Oldest"
)

// ANCHOR: MachineSetStatus

// MachineSetStatus defines the observed state of MachineSet.
type MachineSetStatus struct {
	// Selector is the same as the label selector but in the string format to avoid introspection
	// by clients. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	Selector string `json:"selector,omitempty"`

	// Replicas is the most recently observed number of replicas.
	// +optional
	Replicas int32 `json:"replicas"`

	// The number of replicas that have labels matching the labels of the machine template of the MachineSet.
	// +optional
	FullyLabeledReplicas int32 `json:"fullyLabeledReplicas"`

	// The number of ready replicas for this MachineSet. A machine is considered ready when the node has been created and is "Ready".
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"`

	// The number of available replicas (ready for at least minReadySeconds) for this MachineSet.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"`

	// ObservedGeneration reflects the generation of the most recently observed MachineSet.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

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
	// +optional
	FailureReason *capierrors.MachineSetStatusError `json:"failureReason,omitempty"`
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
	// Conditions defines current service state of the MachineSet.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in MachineSet's status with the V1Beta2 version.
	// +optional
	V1Beta2 *MachineSetV1Beta2Status `json:"v1beta2,omitempty"`
}

// MachineSetV1Beta2Status groups all the fields that will be added or modified in MachineSetStatus with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineSetV1Beta2Status struct {
	// conditions represents the observations of a MachineSet's current state.
	// Known condition types are MachinesReady, MachinesUpToDate, ScalingUp, ScalingDown, Remediating, Deleting, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// readyReplicas is the number of ready replicas for this MachineSet. A machine is considered ready when Machine's Ready condition is true.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// availableReplicas is the number of available replicas for this MachineSet. A machine is considered available when Machine's Available condition is true.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`

	// upToDateReplicas is the number of up-to-date replicas for this MachineSet. A machine is considered up-to-date when Machine's UpToDate condition is true.
	// +optional
	UpToDateReplicas *int32 `json:"upToDateReplicas,omitempty"`
}

// ANCHOR_END: MachineSetStatus

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
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="Total number of machines desired by this machineset",priority=10
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.replicas",description="Total number of non-terminated machines targeted by this machineset"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.readyReplicas",description="Total number of ready machines targeted by this machineset."
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableReplicas",description="Total number of available machines (ready for at least minReadySeconds)"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachineSet"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.template.spec.version",description="Kubernetes version associated with this MachineSet"

// MachineSet is the Schema for the machinesets API.
type MachineSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSetSpec   `json:"spec,omitempty"`
	Status MachineSetStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for the MachineSet.
func (m *MachineSet) GetConditions() Conditions {
	return m.Status.Conditions
}

// SetConditions updates the set of conditions on the MachineSet.
func (m *MachineSet) SetConditions(conditions Conditions) {
	m.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (m *MachineSet) GetV1Beta2Conditions() []metav1.Condition {
	if m.Status.V1Beta2 == nil {
		return nil
	}
	return m.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (m *MachineSet) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if m.Status.V1Beta2 == nil && conditions != nil {
		m.Status.V1Beta2 = &MachineSetV1Beta2Status{}
	}
	m.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachineSetList contains a list of MachineSet.
type MachineSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MachineSet `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachineSet{}, &MachineSetList{})
}
