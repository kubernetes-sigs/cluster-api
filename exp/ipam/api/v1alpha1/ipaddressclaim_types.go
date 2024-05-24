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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// IPAddressClaimSpec is the desired state of an IPAddressClaim.
type IPAddressClaimSpec struct {
	// PoolRef is a reference to the pool from which an IP address should be created.
	PoolRef corev1.TypedLocalObjectReference `json:"poolRef"`
}

// IPAddressClaimStatus is the observed status of a IPAddressClaim.
type IPAddressClaimStatus struct {
	// AddressRef is a reference to the address that was created for this claim.
	// +optional
	AddressRef corev1.LocalObjectReference `json:"addressRef,omitempty"`

	// Conditions summarises the current state of the IPAddressClaim
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ipaddressclaims,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pool Name",type="string",JSONPath=".spec.poolRef.name",description="Name of the pool to allocate an address from"
// +kubebuilder:printcolumn:name="Pool Kind",type="string",JSONPath=".spec.poolRef.kind",description="Kind of the pool to allocate an address from"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of IPAdressClaim"

// IPAddressClaim is the Schema for the ipaddressclaim API.
type IPAddressClaim struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IPAddressClaimSpec   `json:"spec,omitempty"`
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

// +kubebuilder:object:root=true

// IPAddressClaimList is a list of IPAddressClaims.
type IPAddressClaimList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IPAddressClaim `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &IPAddressClaim{}, &IPAddressClaimList{})
}
