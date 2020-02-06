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

// ProviderType is a string representation of a TaskGroup create policy.
type ProviderType string

const (
	// CoreProviderType
	CoreProviderType = ProviderType("CoreProvider")

	// BootstrapProviderType
	BootstrapProviderType = ProviderType("BootstrapProvider")

	// InfrastructureProviderType
	InfrastructureProviderType = ProviderType("InfrastructureProvider")

	// ControlPlaneProviderType
	ControlPlaneProviderType = ProviderType("ControlPlaneProvider")

	// ProviderTypeUnknown
	ProviderTypeUnknown = ProviderType("")
)

// GetProviderType attempts to parse the ProviderType field and return
// the typed ProviderType representation.
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

// InstanceName return the instance name for the provider, that is composed by the provider name and the namespace
// where the provider is installed (nb. clusterctl does not support multiple instances of the same provider to be
// installed in the same namespace)
func (p *Provider) InstanceName() string {
	return types.NamespacedName{Namespace: p.Namespace, Name: p.Name}.String()
}

// HasWatchingOverlapWith returns true if the provider has an overlapping watching namespace with another provider.
func (p *Provider) HasWatchingOverlapWith(other Provider) bool {
	return p.WatchedNamespace == "" || p.WatchedNamespace == other.WatchedNamespace || other.WatchedNamespace == ""
}

// Equals returns true if two providers are exactly the same.
func (p *Provider) Equals(other Provider) bool {
	return p.Name == other.Name &&
		p.Namespace == other.Namespace &&
		p.Type == other.Type &&
		p.WatchedNamespace == other.WatchedNamespace &&
		p.Version == other.Version
}

// +kubebuilder:object:root=true

// ProviderList contains a list of Provider
type ProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Provider `json:"items"`
}

func (l *ProviderList) FilterByName(name string) []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.Name == name
	})
}

func (l *ProviderList) FilterByNamespace(namespace string) []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.Namespace == namespace
	})
}

func (l *ProviderList) FilterByType(providerType ProviderType) []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.GetProviderType() == providerType
	})
}

func (l *ProviderList) FilterCore() []Provider {
	return l.filterBy(func(p Provider) bool {
		return p.GetProviderType() == CoreProviderType
	})
}

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
