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

	contractapi "sigs.k8s.io/cluster-api/internal/contract/api"
)

// BootstrapConfigSpec defines the desired state of BootstrapConfig.
type BootstrapConfigSpec struct {
}

// BootstrapConfigStatus defines the observed state of BootstrapConfig.
type BootstrapConfigStatus struct {
	// conditions represents the observations of a BootstrapConfig's current state.
	Conditions contractapi.Conditions `json:"conditions,omitempty"`

	// ready indicates the BootstrapData field is ready to be consumed
	// +optional
	Ready bool `json:"ready"`

	// dataSecretName is the name of the secret that stores the bootstrap data script.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	DataSecretName *string `json:"dataSecretName,omitempty"`

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

	// v1beta2 groups all the fields that will be added or modified in BootstrapConfig's status with the V1Beta2 version.
	// +optional
	V1Beta2 *BootstrapConfigV1Beta2Status `json:"v1beta2,omitempty"`
}

// BootstrapConfigV1Beta2Status groups all the fields that will be added or modified in BootstrapConfig with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type BootstrapConfigV1Beta2Status struct {
	// conditions represents the observations of a BootstrapConfig's current state.
	Conditions contractapi.Conditions `json:"conditions,omitempty"`
}

var _ contractapi.BootstrapConfig = &BootstrapConfig{}

// +kubebuilder:resource:path=bootstrapconfigs,scope=Namespaced,categories=cluster-api
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// BootstrapConfig is the Schema for the BootstrapConfigs API.
type BootstrapConfig struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of BootstrapConfig.
	// +optional
	Spec BootstrapConfigSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of BootstrapConfig.
	// +optional
	Status BootstrapConfigStatus `json:"status,omitempty,omitzero"`
}

// GetStatusWithReadyConditions returns the status of the BootstrapConfig with its Ready conditions.
func (c *BootstrapConfig) GetStatusWithReadyConditions() any {
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

// GetDataSecretCreated returns whether the bootstrap data secret has been created.
func (c *BootstrapConfig) GetDataSecretCreated() bool {
	return c.Status.Ready
}

// GetDataSecretName returns the name of the bootstrap data secret.
func (c *BootstrapConfig) GetDataSecretName() string {
	return ptr.Deref(c.Status.DataSecretName, "")
}

// GetFailureReason returns the failure reason.
func (c *BootstrapConfig) GetFailureReason() string {
	return c.Status.FailureReason
}

// GetFailureMessage returns the failure message.
func (c *BootstrapConfig) GetFailureMessage() string {
	return c.Status.FailureMessage
}

// +kubebuilder:object:root=true

// BootstrapConfigList contains a list of BootstrapConfig.
type BootstrapConfigList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of BootstrapConfigs.
	Items []BootstrapConfig `json:"items"`
}
