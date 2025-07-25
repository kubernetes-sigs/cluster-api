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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MachineHealthCheck's RemediationAllowed condition and corresponding reasons that will be used in v1Beta2 API version.
const (
	// MachineHealthCheckRemediationAllowedV1Beta2Condition surfaces whether the MachineHealthCheck is
	// allowed to remediate any Machines or whether it is blocked from remediating any further.
	MachineHealthCheckRemediationAllowedV1Beta2Condition = "RemediationAllowed"

	// MachineHealthCheckTooManyUnhealthyV1Beta2Reason is the reason used when too many Machines are unhealthy and
	// the MachineHealthCheck is blocked from making any further remediation.
	MachineHealthCheckTooManyUnhealthyV1Beta2Reason = "TooManyUnhealthy"

	// MachineHealthCheckRemediationAllowedV1Beta2Reason is the reason used when the number of unhealthy machine
	// is within the limits defined by the MachineHealthCheck, and thus remediation is allowed.
	MachineHealthCheckRemediationAllowedV1Beta2Reason = "RemediationAllowed"
)

var (
	// DefaultNodeStartupTimeout is the time allowed for a node to start up.
	// Can be made longer as part of spec if required for particular provider.
	// 10 minutes should allow the instance to start and the node to join the
	// cluster on most providers.
	DefaultNodeStartupTimeout = metav1.Duration{Duration: 10 * time.Minute}
)

// MachineHealthCheckSpec defines the desired state of MachineHealthCheck.
type MachineHealthCheckSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName"`

	// selector is a label selector to match machines whose health will be exercised
	// +required
	Selector metav1.LabelSelector `json:"selector"`

	// unhealthyConditions contains a list of the conditions that determine
	// whether a node is considered unhealthy.  The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the node is unhealthy.
	//
	// +optional
	// +kubebuilder:validation:MaxItems=100
	UnhealthyConditions []UnhealthyCondition `json:"unhealthyConditions,omitempty"`

	// maxUnhealthy specifies the maximum number of unhealthy machines allowed.
	// Any further remediation is only allowed if at most "maxUnhealthy" machines selected by
	// "selector" are not healthy.
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/issues/10722 for more details.
	//
	// +optional
	MaxUnhealthy *intstr.IntOrString `json:"maxUnhealthy,omitempty"`

	// unhealthyRange specifies the range of unhealthy machines allowed.
	// Any further remediation is only allowed if the number of machines selected by "selector" as not healthy
	// is within the range of "unhealthyRange". Takes precedence over maxUnhealthy.
	// Eg. "[3-5]" - This means that remediation will be allowed only when:
	// (a) there are at least 3 unhealthy machines (and)
	// (b) there are at most 5 unhealthy machines
	//
	// Deprecated: This field is deprecated and is going to be removed in the next apiVersion. Please see https://github.com/kubernetes-sigs/cluster-api/issues/10722 for more details.
	//
	// +optional
	// +kubebuilder:validation:Pattern=^\[[0-9]+-[0-9]+\]$
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	UnhealthyRange *string `json:"unhealthyRange,omitempty"`

	// nodeStartupTimeout allows to set the maximum time for MachineHealthCheck
	// to consider a Machine unhealthy if a corresponding Node isn't associated
	// through a `Spec.ProviderID` field.
	//
	// The duration set in this field is compared to the greatest of:
	// - Cluster's infrastructure ready condition timestamp (if and when available)
	// - Control Plane's initialized condition timestamp (if and when available)
	// - Machine's infrastructure ready condition timestamp (if and when available)
	// - Machine's metadata creation timestamp
	//
	// Defaults to 10 minutes.
	// If you wish to disable this feature, set the value explicitly to 0.
	// +optional
	NodeStartupTimeout *metav1.Duration `json:"nodeStartupTimeout,omitempty"`

	// remediationTemplate is a reference to a remediation template
	// provided by an infrastructure provider.
	//
	// This field is completely optional, when filled, the MachineHealthCheck controller
	// creates a new object from the template referenced and hands off remediation of the machine to
	// a controller that lives outside of Cluster API.
	// +optional
	RemediationTemplate *corev1.ObjectReference `json:"remediationTemplate,omitempty"`
}

// UnhealthyCondition represents a Node condition type and value with a timeout
// specified as a duration.  When the named condition has been in the given
// status for at least the timeout value, a node is considered unhealthy.
type UnhealthyCondition struct {
	// type of Node condition
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +required
	Type corev1.NodeConditionType `json:"type"`

	// status of the condition, one of True, False, Unknown.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +required
	Status corev1.ConditionStatus `json:"status"`

	// timeout is the duration that a node must be in a given status for,
	// after which the node is considered unhealthy.
	// For example, with a value of "1h", the node must match the status
	// for at least 1 hour before being considered unhealthy.
	// +required
	Timeout metav1.Duration `json:"timeout"`
}

// MachineHealthCheckStatus defines the observed state of MachineHealthCheck.
type MachineHealthCheckStatus struct {
	// expectedMachines is the total number of machines counted by this machine health check
	// +kubebuilder:validation:Minimum=0
	// +optional
	ExpectedMachines int32 `json:"expectedMachines"`

	// currentHealthy is the total number of healthy machines counted by this machine health check
	// +kubebuilder:validation:Minimum=0
	// +optional
	CurrentHealthy int32 `json:"currentHealthy"`

	// remediationsAllowed is the number of further remediations allowed by this machine health check before
	// maxUnhealthy short circuiting will be applied
	// +kubebuilder:validation:Minimum=0
	// +optional
	RemediationsAllowed int32 `json:"remediationsAllowed"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// targets shows the current list of machines the machine health check is watching
	// +optional
	// +kubebuilder:validation:MaxItems=10000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=253
	Targets []string `json:"targets,omitempty"`

	// conditions defines current service state of the MachineHealthCheck.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in MachineHealthCheck's status with the V1Beta2 version.
	// +optional
	V1Beta2 *MachineHealthCheckV1Beta2Status `json:"v1beta2,omitempty"`
}

// MachineHealthCheckV1Beta2Status groups all the fields that will be added or modified in MachineHealthCheck with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineHealthCheckV1Beta2Status struct {
	// conditions represents the observations of a MachineHealthCheck's current state.
	// Known condition types are RemediationAllowed, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinehealthchecks,shortName=mhc;mhcs,scope=Namespaced,categories=cluster-api
// +kubebuilder:deprecatedversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="ExpectedMachines",type="integer",JSONPath=".status.expectedMachines",description="Number of machines currently monitored"
// +kubebuilder:printcolumn:name="MaxUnhealthy",type="string",JSONPath=".spec.maxUnhealthy",description="Maximum number of unhealthy machines allowed"
// +kubebuilder:printcolumn:name="CurrentHealthy",type="integer",JSONPath=".status.currentHealthy",description="Current observed healthy machines"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachineHealthCheck"

// MachineHealthCheck is the Schema for the machinehealthchecks API.
type MachineHealthCheck struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the specification of machine health check policy
	// +optional
	Spec MachineHealthCheckSpec `json:"spec,omitempty"`

	// status is the most recently observed status of MachineHealthCheck resource
	// +optional
	Status MachineHealthCheckStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (m *MachineHealthCheck) GetConditions() Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *MachineHealthCheck) SetConditions(conditions Conditions) {
	m.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (m *MachineHealthCheck) GetV1Beta2Conditions() []metav1.Condition {
	if m.Status.V1Beta2 == nil {
		return nil
	}
	return m.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (m *MachineHealthCheck) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if m.Status.V1Beta2 == nil {
		m.Status.V1Beta2 = &MachineHealthCheckV1Beta2Status{}
	}
	m.Status.V1Beta2.Conditions = conditions
}

// +kubebuilder:object:root=true

// MachineHealthCheckList contains a list of MachineHealthCheck.
type MachineHealthCheckList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of MachineHealthChecks.
	Items []MachineHealthCheck `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &MachineHealthCheck{}, &MachineHealthCheckList{})
}
