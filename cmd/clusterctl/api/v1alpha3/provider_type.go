/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:resource:path=providers,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".type"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".version"
// +kubebuilder:printcolumn:name="Watch Namespace",type="string",JSONPath=".watchedNamespace"

// Provider is the Schema for the providers API
type Provider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Type indicates the type of the provider.
	// See ProviderType for a list of supported values
	// +optional
	Type string `json:"type,omitempty"`

	// Version indicates the component version.
	// +optional
	Version string `json:"version,omitempty"`

	// WatchedNamespace indicates the namespace where the provider controller is is watching.
	// if empty the provider controller is watching for objects in all namespaces.
	// +optional
	WatchedNamespace string `json:"watchedNamespace,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderList contains a list of Provider
type ProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Provider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Provider{}, &ProviderList{})
}

// ProviderType is a string representation of a TaskGroup create policy.
type ProviderType string

const (
	// CoreProviderType
	CoreProviderType = ProviderType("CoreProvider")

	// BootstrapProviderType
	BootstrapProviderType = ProviderType("BootstrapProvider")

	// InfrastructureProviderType
	InfrastructureProviderType = ProviderType("InfrastructureProvider")

	// ProviderTypeUnknown
	ProviderTypeUnknown = ProviderType("")
)

// GetTypedProviderType attempts to parse the ProviderType field and return
// the typed ProviderType representation.
func (in *Provider) GetTypedProviderType() ProviderType {
	switch t := ProviderType(in.Type); t {
	case
		CoreProviderType,
		BootstrapProviderType,
		InfrastructureProviderType:
		return t
	default:
		return ProviderTypeUnknown
	}
}
