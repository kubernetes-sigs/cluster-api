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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MachineHealthCheck's RemediationAllowed condition and corresponding reasons.
const (
	// MachineHealthCheckRemediationAllowedCondition surfaces whether the MachineHealthCheck is
	// allowed to remediate any Machines or whether it is blocked from remediating any further.
	MachineHealthCheckRemediationAllowedCondition = "RemediationAllowed"

	// MachineHealthCheckTooManyUnhealthyReason is the reason used when too many Machines are unhealthy and
	// the MachineHealthCheck is blocked from making any further remediation.
	MachineHealthCheckTooManyUnhealthyReason = "TooManyUnhealthy"

	// MachineHealthCheckRemediationAllowedReason is the reason used when the number of unhealthy machine
	// is within the limits defined by the MachineHealthCheck, and thus remediation is allowed.
	MachineHealthCheckRemediationAllowedReason = "RemediationAllowed"
)

var (
	// DefaultNodeStartupTimeoutSeconds is the time allowed for a node to start up.
	// Can be made longer as part of spec if required for particular provider.
	// 10 minutes should allow the instance to start and the node to join the
	// cluster on most providers.
	DefaultNodeStartupTimeoutSeconds = int32(600)
)

// MachineHealthCheckSpec defines the desired state of MachineHealthCheck.
type MachineHealthCheckSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`

	// selector is a label selector to match machines whose health will be exercised
	// +required
	Selector metav1.LabelSelector `json:"selector,omitempty,omitzero"`

	// checks are the checks that are used to evaluate if a Machine is healthy.
	//
	// Independent of this configuration the MachineHealthCheck controller will always
	// flag Machines with `cluster.x-k8s.io/remediate-machine` annotation and
	// Machines with deleted Nodes as unhealthy.
	//
	// Furthermore, if checks.nodeStartupTimeoutSeconds is not set it
	// is defaulted to 10 minutes and evaluated accordingly.
	//
	// +optional
	Checks MachineHealthCheckChecks `json:"checks,omitempty,omitzero"`

	// remediation configures if and how remediations are triggered if a Machine is unhealthy.
	//
	// If remediation or remediation.triggerIf is not set,
	// remediation will always be triggered for unhealthy Machines.
	//
	// If remediation or remediation.templateRef is not set,
	// the OwnerRemediated condition will be set on unhealthy Machines to trigger remediation via
	// the owner of the Machines, for example a MachineSet or a KubeadmControlPlane.
	//
	// +optional
	Remediation MachineHealthCheckRemediation `json:"remediation,omitempty,omitzero"`
}

// MachineHealthCheckChecks are the checks that are used to evaluate if a Machine is healthy.
// +kubebuilder:validation:MinProperties=1
type MachineHealthCheckChecks struct {
	// nodeStartupTimeoutSeconds allows to set the maximum time for MachineHealthCheck
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
	// +kubebuilder:validation:Minimum=0
	NodeStartupTimeoutSeconds *int32 `json:"nodeStartupTimeoutSeconds,omitempty"`

	// unhealthyNodeConditions contains a list of conditions that determine
	// whether a node is considered unhealthy. The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the node is unhealthy.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	UnhealthyNodeConditions []UnhealthyNodeCondition `json:"unhealthyNodeConditions,omitempty"`

	// unhealthyMachineConditions contains a list of the machine conditions that determine
	// whether a machine is considered unhealthy.  The conditions are combined in a
	// logical OR, i.e. if any of the conditions is met, the machine is unhealthy.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	UnhealthyMachineConditions []UnhealthyMachineCondition `json:"unhealthyMachineConditions,omitempty"`
}

// MachineHealthCheckRemediation configures if and how remediations are triggered if a Machine is unhealthy.
// +kubebuilder:validation:MinProperties=1
type MachineHealthCheckRemediation struct {
	// triggerIf configures if remediations are triggered.
	// If this field is not set, remediations are always triggered.
	// +optional
	TriggerIf MachineHealthCheckRemediationTriggerIf `json:"triggerIf,omitempty,omitzero"`

	// templateRef is a reference to a remediation template
	// provided by an infrastructure provider.
	//
	// This field is completely optional, when filled, the MachineHealthCheck controller
	// creates a new object from the template referenced and hands off remediation of the machine to
	// a controller that lives outside of Cluster API.
	// +optional
	TemplateRef MachineHealthCheckRemediationTemplateReference `json:"templateRef,omitempty,omitzero"`
}

// MachineHealthCheckRemediationTriggerIf configures if remediations are triggered.
// +kubebuilder:validation:MinProperties=1
type MachineHealthCheckRemediationTriggerIf struct {
	// unhealthyLessThanOrEqualTo specifies that remediations are only triggered if the number of
	// unhealthy Machines is less than or equal to the configured value.
	// unhealthyInRange takes precedence if set.
	//
	// +optional
	UnhealthyLessThanOrEqualTo *intstr.IntOrString `json:"unhealthyLessThanOrEqualTo,omitempty"`

	// unhealthyInRange specifies that remediations are only triggered if the number of
	// unhealthy Machines is in the configured range.
	// Takes precedence over unhealthyLessThanOrEqualTo.
	// Eg. "[3-5]" - This means that remediation will be allowed only when:
	// (a) there are at least 3 unhealthy Machines (and)
	// (b) there are at most 5 unhealthy Machines
	//
	// +optional
	// +kubebuilder:validation:Pattern=^\[[0-9]+-[0-9]+\]$
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=32
	UnhealthyInRange string `json:"unhealthyInRange,omitempty"`
}

// MachineHealthCheckRemediationTemplateReference is a reference to a remediation template.
type MachineHealthCheckRemediationTemplateReference struct {
	// kind of the remediation template.
	// kind must consist of alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
	Kind string `json:"kind,omitempty"`

	// name of the remediation template.
	// name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`

	// apiVersion of the remediation template.
	// apiVersion must be fully qualified domain name followed by / and a version.
	// NOTE: This field must be kept in sync with the APIVersion of the remediation template.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=317
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/[a-z]([-a-z0-9]*[a-z0-9])?$`
	APIVersion string `json:"apiVersion,omitempty"`
}

// IsDefined returns true if the MachineHealthCheckRemediationTemplateReference is set.
func (r *MachineHealthCheckRemediationTemplateReference) IsDefined() bool {
	if r == nil {
		return false
	}
	return r.Kind != "" || r.Name != "" || r.APIVersion != ""
}

// ToObjectReference returns an object reference for the MachineHealthCheckRemediationTemplateReference in a given namespace.
func (r *MachineHealthCheckRemediationTemplateReference) ToObjectReference(namespace string) *corev1.ObjectReference {
	if r == nil {
		return nil
	}
	return &corev1.ObjectReference{
		APIVersion: r.APIVersion,
		Kind:       r.Kind,
		Namespace:  namespace,
		Name:       r.Name,
	}
}

// GroupVersionKind gets the GroupVersionKind for a MachineHealthCheckRemediationTemplateReference.
func (r *MachineHealthCheckRemediationTemplateReference) GroupVersionKind() schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(r.APIVersion, r.Kind)
}

// UnhealthyNodeCondition represents a Node condition type and value with a timeout
// specified as a duration.  When the named condition has been in the given
// status for at least the timeout value, a node is considered unhealthy.
type UnhealthyNodeCondition struct {
	// type of Node condition
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +required
	Type corev1.NodeConditionType `json:"type,omitempty"`

	// status of the condition, one of True, False, Unknown.
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:MinLength=1
	// +required
	Status corev1.ConditionStatus `json:"status,omitempty"`

	// timeoutSeconds is the duration that a node must be in a given status for,
	// after which the node is considered unhealthy.
	// For example, with a value of "3600", the node must match the status
	// for at least 1 hour before being considered unhealthy.
	// +required
	// +kubebuilder:validation:Minimum=0
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// UnhealthyMachineCondition represents a Machine condition type and value with a timeout
// specified as a duration.  When the named condition has been in the given
// status for at least the timeout value, a machine is considered unhealthy.
type UnhealthyMachineCondition struct {
	// type of Machine condition
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=316
	// +kubebuilder:validation:XValidation:rule="!(self in ['Ready','Available','HealthCheckSucceeded','OwnerRemediated','ExternallyRemediated'])",message="type must not be one of: Ready, Available, HealthCheckSucceeded, OwnerRemediated, ExternallyRemediated"
	// +required
	Type string `json:"type,omitempty"`

	// status of the condition, one of True, False, Unknown.
	// +required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status metav1.ConditionStatus `json:"status,omitempty"`

	// timeoutSeconds is the duration that a machine must be in a given status for,
	// after which the machine is considered unhealthy.
	// For example, with a value of "3600", the machine must match the status
	// for at least 1 hour before being considered unhealthy.
	// +required
	// +kubebuilder:validation:Minimum=0
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// MachineHealthCheckStatus defines the observed state of MachineHealthCheck.
// +kubebuilder:validation:MinProperties=1
type MachineHealthCheckStatus struct {
	// conditions represents the observations of a MachineHealthCheck's current state.
	// Known condition types are RemediationAllowed, Paused.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// expectedMachines is the total number of machines counted by this machine health check
	// +kubebuilder:validation:Minimum=0
	// +optional
	ExpectedMachines *int32 `json:"expectedMachines,omitempty"`

	// currentHealthy is the total number of healthy machines counted by this machine health check
	// +kubebuilder:validation:Minimum=0
	// +optional
	CurrentHealthy *int32 `json:"currentHealthy,omitempty"`

	// remediationsAllowed is the number of further remediations allowed by this machine health check before
	// maxUnhealthy short circuiting will be applied
	// +kubebuilder:validation:Minimum=0
	// +optional
	RemediationsAllowed *int32 `json:"remediationsAllowed,omitempty"`

	// observedGeneration is the latest generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// targets shows the current list of machines the machine health check is watching
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=10000
	// +kubebuilder:validation:items:MinLength=1
	// +kubebuilder:validation:items:MaxLength=253
	Targets []string `json:"targets,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *MachineHealthCheckDeprecatedStatus `json:"deprecated,omitempty"`
}

// MachineHealthCheckDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineHealthCheckDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *MachineHealthCheckV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// MachineHealthCheckV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type MachineHealthCheckV1Beta1DeprecatedStatus struct {
	// conditions defines current service state of the MachineHealthCheck.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=machinehealthchecks,shortName=mhc;mhcs,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".status.expectedMachines",description="Number of machines currently monitored"
// +kubebuilder:printcolumn:name="Healthy",type="integer",JSONPath=".status.currentHealthy",description="Current observed healthy machines"
// +kubebuilder:printcolumn:name="Paused",type="string",JSONPath=`.status.conditions[?(@.type=="Paused")].status`,description="Reconciliation paused",priority=10
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of MachineHealthCheck"

// MachineHealthCheck is the Schema for the machinehealthchecks API.
type MachineHealthCheck struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the specification of machine health check policy
	// +required
	Spec MachineHealthCheckSpec `json:"spec,omitempty,omitzero"`

	// status is the most recently observed status of MachineHealthCheck resource
	// +optional
	Status MachineHealthCheckStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (m *MachineHealthCheck) GetV1Beta1Conditions() Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (m *MachineHealthCheck) SetV1Beta1Conditions(conditions Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &MachineHealthCheckDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &MachineHealthCheckV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *MachineHealthCheck) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *MachineHealthCheck) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
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
