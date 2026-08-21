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

package v1beta1

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
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`
}

// InfraMachineStatus defines the observed state of InfraMachine.
type InfraMachineStatus struct {
	// conditions represents the observations of a InfraMachine's current state.
	Conditions contractapi.Conditions `json:"conditions,omitempty"`

	// ready denotes that the machine is ready
	// +optional
	Ready bool `json:"ready"`

	// Interruptible reports that this machine can be interrupted by CAPI.
	// +optional
	Interruptible *bool `json:"interruptible,omitempty"`

	// addresses contains the associated addresses for the dev machine.
	// Note: Usually this field would be of type []clusterv1beta1.MachineAddress, but as the MachineAddress types are
	// identical we use clusterv1.MachineAddress to make it easier to implement the InfraMachine interface.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// failureDomain is the unique identifier of the failure domain where this Machine has been placed in.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	FailureDomain string `json:"failureDomain,omitempty"`

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
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=10240
	FailureMessage string `json:"failureMessage,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in InfraMachine's status with the V1Beta2 version.
	// +optional
	V1Beta2 *InfraMachineV1Beta2Status `json:"v1beta2,omitempty"`
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

// GetProvisioned returns whether the infrastructure is ready.
func (c *InfraMachine) GetProvisioned() bool {
	return c.Status.Ready
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
	return c.Status.FailureReason
}

// GetFailureMessage returns the failure message.
func (c *InfraMachine) GetFailureMessage() string {
	return c.Status.FailureMessage
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
