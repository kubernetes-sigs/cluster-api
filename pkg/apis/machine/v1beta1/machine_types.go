/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openshift/cluster-api/pkg/apis/machine/common"
)

const (
	// MachineFinalizer is set on PrepareForCreate callback.
	MachineFinalizer = "machine.machine.openshift.io"

	// MachineClusterLabelName is the label set on machines linked to a cluster.
	MachineClusterLabelName = "cluster.k8s.io/cluster-name"

	// MachineClusterIDLabel is the label that a machine must have to identify the
	// cluster to which it belongs.
	MachineClusterIDLabel = "machine.openshift.io/cluster-api-cluster"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

/// [Machine]
// Machine is the Schema for the machines API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec,omitempty"`
	Status MachineStatus `json:"status,omitempty"`
}

/// [Machine]

/// [MachineSpec]
// MachineSpec defines the desired state of Machine
type MachineSpec struct {
	// ObjectMeta will autopopulate the Node created. Use this to
	// indicate what labels, annotations, name prefix, etc., should be used
	// when creating the Node.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The list of the taints to be applied to the corresponding Node in additive
	// manner. This list will not overwrite any other taints added to the Node on
	// an ongoing basis by other entities. These taints should be actively reconciled
	// e.g. if you ask the machine controller to apply a taint and then manually remove
	// the taint the machine controller will put it back) but not have the machine controller
	// remove any taints
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// ProviderSpec details Provider-specific configuration to use during node creation.
	// +optional
	ProviderSpec ProviderSpec `json:"providerSpec"`

	// ProviderID is the identification ID of the machine provided by the provider.
	// This field must match the provider ID as seen on the node object corresponding to this machine.
	// This field is required by higher level consumers of cluster-api. Example use case is cluster autoscaler
	// with cluster-api as provider. Clean-up logic in the autoscaler compares machines to nodes to find out
	// machines at provider which could not get registered as Kubernetes nodes. With cluster-api as a
	// generic out-of-tree provider for autoscaler, this field is required by autoscaler to be
	// able to have a provider view of the list of machines. Another list of nodes is queried from the k8s apiserver
	// and then a comparison is done to find out unregistered machines and are marked for delete.
	// This field will be set by the actuators and consumed by higher level entities like autoscaler that will
	// be interfacing with cluster-api as generic provider.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`
}

/// [MachineSpec]

/// [MachineStatus]
// MachineStatus defines the observed state of Machine
type MachineStatus struct {
	// NodeRef will point to the corresponding Node if it exists.
	// +optional
	NodeRef *corev1.ObjectReference `json:"nodeRef,omitempty"`

	// LastUpdated identifies when this status was last observed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// ErrorReason will be set in the event that there is a terminal problem
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
	// +optional
	ErrorReason *common.MachineStatusError `json:"errorReason,omitempty"`

	// ErrorMessage will be set in the event that there is a terminal problem
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
	// +optional
	ErrorMessage *string `json:"errorMessage,omitempty"`

	// ProviderStatus details a Provider-specific status.
	// It is recommended that providers maintain their
	// own versioned API types that should be
	// serialized/deserialized from this field.
	// +optional
	ProviderStatus *runtime.RawExtension `json:"providerStatus,omitempty"`

	// Addresses is a list of addresses assigned to the machine. Queried from cloud provider, if available.
	// +optional
	Addresses []corev1.NodeAddress `json:"addresses,omitempty"`

	// LastOperation describes the last-operation performed by the machine-controller.
	// This API should be useful as a history in terms of the latest operation performed on the
	// specific machine. It should also convey the state of the latest-operation for example if
	// it is still on-going, failed or completed successfully.
	// +optional
	LastOperation *LastOperation `json:"lastOperation,omitempty"`

	// Phase represents the current phase of machine actuation.
	// E.g. Pending, Running, Terminating, Failed etc.
	// +optional
	Phase *string `json:"phase,omitempty"`
}

// LastOperation represents the detail of the last performed operation on the MachineObject.
type LastOperation struct {
	// Description is the human-readable description of the last operation.
	Description *string `json:"description,omitempty"`

	// LastUpdated is the timestamp at which LastOperation API was last-updated.
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// State is the current status of the last performed operation.
	// E.g. Processing, Failed, Successful etc
	State *string `json:"state,omitempty"`

	// Type is the type of operation which was last performed.
	// E.g. Create, Delete, Update etc
	Type *string `json:"type,omitempty"`
}

/// [MachineVersionInfo]

func (m *Machine) Validate() field.ErrorList {
	errors := field.ErrorList{}

	// validate spec.labels
	fldPath := field.NewPath("spec")
	if m.Labels[MachineClusterIDLabel] == "" {
		errors = append(errors, field.Invalid(fldPath.Child("labels"), m.Labels, fmt.Sprintf("missing %v label.", MachineClusterIDLabel)))
	}

	// validate provider config is set
	if m.Spec.ProviderSpec.Value == nil {
		errors = append(errors, field.Invalid(fldPath.Child("spec").Child("providerspec"), m.Spec.ProviderSpec, "value field must be set"))
	}

	return errors
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineList contains a list of Machine
type MachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Machine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Machine{}, &MachineList{})
}
