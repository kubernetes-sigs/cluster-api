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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

// IPAddressClaimSpec is the desired state of an IPAddressClaim.
type IPAddressClaimSpec struct {
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
	Conditions clusterv1beta1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ipaddressclaims,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
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
func (m *IPAddressClaim) GetConditions() clusterv1beta1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *IPAddressClaim) SetConditions(conditions clusterv1beta1.Conditions) {
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
