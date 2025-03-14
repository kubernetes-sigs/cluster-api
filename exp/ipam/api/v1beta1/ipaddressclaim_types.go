/*
Copyright 2022 The Kubernetes Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	PoolRef corev1.TypedLocalObjectReference `json:"poolRef"`
}

// IPAddressClaimStatus is the observed status of a IPAddressClaim.
type IPAddressClaimStatus struct {
	// addressRef is a reference to the address that was created for this claim.
	// +optional
	AddressRef corev1.LocalObjectReference `json:"addressRef,omitempty"`

	// conditions summarises the current state of the IPAddressClaim
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// v1beta2 groups all the fields that will be added or modified in IPAddressClaim's status with the V1Beta2 version.
	// +optional
	V1Beta2 *IPAddressClaimV1Beta2Status `json:"v1beta2,omitempty"`
}

// IPAddressClaimV1Beta2Status groups all the fields that will be added or modified in IPAddressClaimStatus with the V1Beta2 version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type IPAddressClaimV1Beta2Status struct {
	// conditions represents the observations of a IPAddressClaim's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=32
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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
	// +optional
	Spec IPAddressClaimSpec `json:"spec,omitempty"`
	// status is the observed state of IPAddressClaim.
	// +optional
	Status IPAddressClaimStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (m *IPAddressClaim) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *IPAddressClaim) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (m *IPAddressClaim) GetV1Beta2Conditions() []metav1.Condition {
	if m.Status.V1Beta2 == nil {
		return nil
	}
	return m.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (m *IPAddressClaim) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if m.Status.V1Beta2 == nil {
		m.Status.V1Beta2 = &IPAddressClaimV1Beta2Status{}
	}
	m.Status.V1Beta2.Conditions = conditions
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
