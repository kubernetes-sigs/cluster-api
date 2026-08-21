/*
Copyright The Kubernetes Authors.

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
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	contractapi "sigs.k8s.io/cluster-api/internal/contract/api"
)

// InfraMachineSpec defines the desired state of a v1beta2 InfraMachine.
type InfraMachineSpec struct {
	// providerID must match the provider ID as seen on the node object corresponding to this machine.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	ProviderID string `json:"providerID,omitempty"`

	// failureDomain is the unique identifier of the failure domain where this Machine should be placed in.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`
}

// InfraMachineStatus defines the observed state of InfraMachine.
type InfraMachineStatus struct {
	// conditions represents the observations of a InfraMachine's current state.
	Conditions contractapi.Conditions `json:"conditions,omitempty"`

	// initialization provides observations of the InfraMachine initialization process.
	// NOTE: Fields in this struct are part of the Cluster API contract and are used to orchestrate initial Machine provisioning.
	// +optional
	Initialization InfraMachineInitializationStatus `json:"initialization,omitempty,omitzero"`

	// Interruptible reports that this machine can be interrupted by CAPI.
	// +optional
	Interruptible *bool `json:"interruptible,omitempty"`

	// addresses contains the associated addresses for the InfraMachine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// failureDomain is the unique identifier of the failure domain where this Machine has been placed in.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Deprecated *InfraMachineDeprecatedStatus `json:"deprecated,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in InfraMachine's status with the V1Beta2 version.
	//
	// Deprecated: This field has just been added to handle the same cases as before it will be removed when support
	// for v1beta1 will be dropped.
	//
	// +optional
	V1Beta2 *InfraMachineV1Beta2Status `json:"v1beta2,omitempty"`
}

// InfraMachineInitializationStatus provides observations of the InfraMachine initialization process.
// +kubebuilder:validation:MinProperties=1
type InfraMachineInitializationStatus struct {
	// provisioned is true when the infrastructure provider reports that the Machine's infrastructure is fully provisioned.
	// NOTE: this field is part of the Cluster API contract, and it is used to orchestrate initial Machine provisioning.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// InfraMachineDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type InfraMachineDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *InfraMachineV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// InfraMachineV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type InfraMachineV1Beta1DeprecatedStatus struct {
	// failureReason will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a succinct value suitable
	// for machine interpretation.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureReason string `json:"failureReason,omitempty"`

	// failureMessage will be set in the event that there is a terminal problem
	// reconciling the Machine and will contain a more verbose string suitable
	// for logging and human consumption.
	//
	// This field should not be set for transitive errors that a controller
	// faces that are expected to be fixed automatically over
	// time (like service outages), but instead indicate that something is
	// fundamentally wrong with the Machine's spec or the configuration of
	// the controller, and that manual intervention is required. Examples
	// of terminal errors would be invalid combinations of settings in the
	// spec, values that are unsupported by the controller, or the
	// responsible controller itself being critically misconfigured.
	//
	// Any transient errors that occur during the reconciliation of Machines
	// can be added as events to the Machine object and/or logged in the
	// controller's output.
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage string `json:"failureMessage,omitempty"`
}

// InfraMachineV1Beta2Status groups all the fields that will be added or modified in InfraMachine with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type InfraMachineV1Beta2Status struct {
	// conditions represents the observations of a InfraMachine's current state.
	Conditions contractapi.Conditions `json:"conditions,omitempty"`
}

var _ contractapi.InfraMachine = &InfraMachine{}

// +kubebuilder:resource:path=inframachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// InfraMachine is the Schema for the InfraMachines API.
type InfraMachine struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of InfraMachine.
	// +optional
	Spec InfraMachineSpec `json:"spec,omitempty"`
	// status is the observed state of InfraMachine.
	// +optional
	Status InfraMachineStatus `json:"status,omitempty"`
}

// GetStatusWithReadyConditions returns the status of the InfraMachine with its Ready conditions.
func (c *InfraMachine) GetStatusWithReadyConditions() any {
	status := map[string]any{}
	if len(c.Status.Conditions) > 0 {
		status["conditions"] = c.Status.Conditions.ReadyConditionAsAnyArray()
	}
	if c.Status.V1Beta2 != nil && len(c.Status.V1Beta2.Conditions) > 0 {
		status["v1beta2"] = map[string]any{
			"conditions": c.Status.V1Beta2.Conditions.ReadyConditionAsAnyArray(),
		}
	}
	return status
}

// GetProviderID returns the provider ID.
func (c *InfraMachine) GetProviderID() string {
	return c.Spec.ProviderID
}

// GetProvisioned returns whether the infrastructure is provisioned.
func (c *InfraMachine) GetProvisioned() bool {
	return ptr.Deref(c.Status.Initialization.Provisioned, false)
}

// GetInterruptible returns whether the infrastructure is interruptible.
func (c *InfraMachine) GetInterruptible() bool {
	return ptr.Deref(c.Status.Interruptible, false)
}

// GetAddresses returns the machine addresses.
func (c *InfraMachine) GetAddresses() []clusterv1.MachineAddress {
	return c.Status.Addresses
}

// GetSpecFailureDomain returns the failure domain requested in spec.
func (c *InfraMachine) GetSpecFailureDomain() string {
	return c.Spec.FailureDomain
}

// GetFailureDomain returns the actual failure domain from status.
func (c *InfraMachine) GetFailureDomain() string {
	return c.Status.FailureDomain
}

// GetFailureReason returns the failure reason.
func (c *InfraMachine) GetFailureReason() string {
	if c.Status.Deprecated != nil && c.Status.Deprecated.V1Beta1 != nil {
		return c.Status.Deprecated.V1Beta1.FailureReason
	}
	return ""
}

// GetFailureMessage returns the failure message.
func (c *InfraMachine) GetFailureMessage() string {
	if c.Status.Deprecated != nil && c.Status.Deprecated.V1Beta1 != nil {
		return c.Status.Deprecated.V1Beta1.FailureMessage
	}
	return ""
}

// +kubebuilder:object:root=true

// InfraMachineList contains a list of InfraMachine.
type InfraMachineList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of InfraMachines.
	Items []InfraMachine `json:"items"`
}
