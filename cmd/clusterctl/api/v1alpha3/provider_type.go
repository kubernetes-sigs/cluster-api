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
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:resource:path=providers,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of Provider"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".type"
// +kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".providerName"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".version"

// Provider defines an entry in the provider inventory.
type Provider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// ProviderName indicates the name of the provider.
	// +optional
	ProviderName string `json:"providerName,omitempty"`

	// Type indicates the type of the provider.
	// See ProviderType for a list of supported values
	// +optional
	Type string `json:"type,omitempty"`

	// Version indicates the component version.
	// +optional
	Version string `json:"version,omitempty"`

	// WatchedNamespace indicates the namespace where the provider controller is is watching.
	// if empty the provider controller is watching for objects in all namespaces.
	// Deprecated: in clusterctl v1alpha4 all the providers watch all the namespaces; this field will be removed in a future version of this API
	// +optional
	WatchedNamespace string `json:"watchedNamespace,omitempty"`
}

// ManifestLabel returns the cluster.x-k8s.io/provider label value for an entry in the provider inventory.
// Please note that this label uniquely identifies the provider, e.g. bootstrap-kubeadm, but not the instances of
// the provider, e.g. namespace-1/bootstrap-kubeadm and namespace-2/bootstrap-kubeadm.
func (p *Provider) ManifestLabel() string {
	return ManifestLabel(p.ProviderName, p.GetProviderType())
}

// InstanceName return the a name that uniquely identifies an entry in the provider inventory.
// The instanceName is composed by the ManifestLabel and by the namespace where the provider is installed;
// the resulting value uniquely identify a provider instance because clusterctl does not support multiple
// instances of the same provider to be installed in the same namespace.
func (p *Provider) InstanceName() string {
	return types.NamespacedName{Namespace: p.Namespace, Name: p.ManifestLabel()}.String()
}

// SameAs returns true if two providers have the same ProviderName and Type.
// Please note that there could be many instances of the same provider.
func (p *Provider) SameAs(other Provider) bool {
	return p.ProviderName == other.ProviderName && p.Type == other.Type
}

// Equals returns true if two providers are identical (same name, provider name, type, version etc.).
func (p *Provider) Equals(other Provider) bool {
	return p.Name == other.Name &&
		p.Namespace == other.Namespace &&
		p.ProviderName == other.ProviderName &&
		p.Type == other.Type &&
		p.WatchedNamespace == other.WatchedNamespace &&
		p.Version == other.Version
}

// GetProviderType parse the Provider.Type string field and return the typed representation.
func (p *Provider) GetProviderType() ProviderType {
	switch t := ProviderType(p.Type); t {
	case
		CoreProviderType,
		BootstrapProviderType,
		InfrastructureProviderType,
		ControlPlaneProviderType:
		return t
	default:
		return ProviderTypeUnknown
	}
}

// ProviderType is a string representation of a Provider type.
type ProviderType string

const (
	// CoreProviderType is a type reserved for Cluster API core repository.
	CoreProviderType = ProviderType("CoreProvider")

	// BootstrapProviderType is the type associated with codebases that provide
	// bootstrapping capabilities.
	BootstrapProviderType = ProviderType("BootstrapProvider")

	// InfrastructureProviderType is the type associated with codebases that provide
	// infrastructure capabilities.
	InfrastructureProviderType = ProviderType("InfrastructureProvider")

	// ControlPlaneProviderType is the type associated with codebases that provide
	// control-plane capabilities.
	ControlPlaneProviderType = ProviderType("ControlPlaneProvider")

	// ProviderTypeUnknown is used when the type is unknown.
	ProviderTypeUnknown = ProviderType("")
)

// Order return an integer that can be used to sort ProviderType values.
func (p ProviderType) Order() int {
	switch p {
	case CoreProviderType:
		return 0
	case BootstrapProviderType:
		return 1
	case ControlPlaneProviderType:
		return 2
	case InfrastructureProviderType:
		return 3
	default:
		return 4
	}
}

// +kubebuilder:object:root=true

// ProviderList contains a list of Provider.
type ProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Provider `json:"items"`
}

// FilterByNamespace returns a new list of providers that reside in the namespace provided.
func (l *ProviderList) FilterByNamespace(namespace string) []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.Namespace == namespace
	})
}

// FilterByProviderNameAndType returns a new list of provider that match the name and type.
func (l *ProviderList) FilterByProviderNameAndType(provider string, providerType ProviderType) []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.ProviderName == provider && p.Type == string(providerType)
	})
}

// FilterByType returns a new list of providers that match the given type.
func (l *ProviderList) FilterByType(providerType ProviderType) []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.GetProviderType() == providerType
	})
}

// FilterCore returns a new list of providers that are in the core.
func (l *ProviderList) FilterCore() []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.GetProviderType() == CoreProviderType
	})
}

// FilterNonCore returns a new list of providers that are not in the core.
func (l *ProviderList) FilterNonCore() []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.GetProviderType() != CoreProviderType
	})
}

func (l *ProviderList) filterBy(predicate func(p Provider) bool) []Provider {
	ret := []Provider{}
	for _, i := range l.Items {
		if predicate(i) {
			ret = append(ret, i)
		}
	}
	return ret
}

func init() {
	SchemeBuilder.Register(&Provider{}, &ProviderList{})
}
