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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// IPAddressClaim's Ready condition and corresponding reasons.
const (
	// IPAddressClaimReadyCondition is true if the IPAddressClaim allocation succeeded.
	IPAddressClaimReadyCondition = clusterv1.ReadyCondition

	// IPAddressClaimReadyAllocationFailedReason is the reason used when allocating an IP address for a claim fails.
	// More details should be provided in the condition's message.
	// When the IP pool is full, [PoolExhaustedReason] should be used for better visibility instead.
	IPAddressClaimReadyAllocationFailedReason = "AllocationFailed"

	// IPAddressClaimReadyPoolNotReadyReason is the reason used when the referenced IP pool is not ready.
	IPAddressClaimReadyPoolNotReadyReason = "PoolNotReady"

	// IPAddressClaimReadyPoolExhaustedReason is the reason used when an IP pool referenced by an [IPAddressClaim] is full and no address
	// can be allocated for the claim.
	IPAddressClaimReadyPoolExhaustedReason = "PoolExhausted"
)

// IPAddressClaimSpec is the desired state of an IPAddressClaim.
type IPAddressClaimSpec struct {
	// clusterName is the name of the Cluster this object belongs to.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`

	// poolRef is a reference to the pool from which an IP address should be created.
	// +required
	PoolRef IPPoolReference `json:"poolRef,omitempty,omitzero"`
}

// IPAddressClaimStatus is the observed status of a IPAddressClaim.
// +kubebuilder:validation:MinProperties=1
type IPAddressClaimStatus struct {
	// conditions represents the observations of a IPAddressClaim's current state.
	// Known condition types are Ready.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// addressRef is a reference to the address that was created for this claim.
	// +optional
	AddressRef IPAddressReference `json:"addressRef,omitempty,omitzero"`

	// deprecated groups all the status fields that are deprecated and will be removed when all the nested field are removed.
	// +optional
	Deprecated *IPAddressClaimDeprecatedStatus `json:"deprecated,omitempty"`
}

// IPAddressReference is a reference to an IPAddress.
type IPAddressReference struct {
	// name of the IPAddress.
	// name must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name,omitempty"`
}

// IPAddressClaimDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type IPAddressClaimDeprecatedStatus struct {
	// v1beta1 groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
	// +optional
	V1Beta1 *IPAddressClaimV1Beta1DeprecatedStatus `json:"v1beta1,omitempty"`
}

// IPAddressClaimV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type IPAddressClaimV1Beta1DeprecatedStatus struct {
	// conditions summarises the current state of the IPAddressClaim
	//
	// Deprecated: This field is deprecated and is going to be removed when support for v1beta1 will be dropped. Please see https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more details.
	//
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ipaddressclaims,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Pool Name",type="string",JSONPath=".spec.poolRef.name",description="Name of the pool to allocate an address from"
// +kubebuilder:printcolumn:name="Pool Kind",type="string",JSONPath=".spec.poolRef.kind",description="Kind of the pool to allocate an address from"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of IPAdressClaim"

// IPAddressClaim is the Schema for the ipaddressclaim API.
type IPAddressClaim struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the desired state of IPAddressClaim.
	// +required
	Spec IPAddressClaimSpec `json:"spec,omitempty,omitzero"`
	// status is the observed state of IPAddressClaim.
	// +optional
	Status IPAddressClaimStatus `json:"status,omitempty,omitzero"`
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (m *IPAddressClaim) GetV1Beta1Conditions() clusterv1.Conditions {
	if m.Status.Deprecated == nil || m.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return m.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (m *IPAddressClaim) SetV1Beta1Conditions(conditions clusterv1.Conditions) {
	if m.Status.Deprecated == nil {
		m.Status.Deprecated = &IPAddressClaimDeprecatedStatus{}
	}
	if m.Status.Deprecated.V1Beta1 == nil {
		m.Status.Deprecated.V1Beta1 = &IPAddressClaimV1Beta1DeprecatedStatus{}
	}
	m.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (m *IPAddressClaim) GetConditions() []metav1.Condition {
	return m.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (m *IPAddressClaim) SetConditions(conditions []metav1.Condition) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// IPAddressClaimList is a list of IPAddressClaims.
type IPAddressClaimList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of IPAddressClaims.
	Items []IPAddressClaim `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &IPAddressClaim{}, &IPAddressClaimList{})
}
