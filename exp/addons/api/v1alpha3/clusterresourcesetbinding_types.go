/*
Copyright 2020 The Kubernetes Authors.

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

// ANCHOR: ResourceBinding

// ResourceBinding shows the status of a resource that belongs to a ClusterResourceSet matched by the owner cluster of the ClusterResourceSetBinding object.
type ResourceBinding struct {
	// Hash is the hash of a resource's data. This can be used to decide if a resource is changed.
	// For "ApplyOnce" ClusterResourceSet.spec.strategy, this is no-op as that strategy does not act on change.
	Hash string `json:"hash,omitempty"`

	// LastAppliedTime identifies when this resource was last applied to the cluster.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`

	// Applied is to track if a resource is applied to the cluster or not.
	Applied bool `json:"applied"`
}

// ANCHOR_END: ResourceBinding

// ResourcesSetBinding keeps info on all of the resources in a ClusterResourceSet.
type ResourcesSetBinding struct {
	// Resources is a map of Secrets/ConfigMaps and their ResourceBinding.
	// The map's key is a concatenated string of form: <resource-type>/<resource-name>.
	Resources map[string]ResourceBinding `json:"resources,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterresourcesetbindings,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// ClusterResourceSetBinding lists all matching ClusterResourceSets with the cluster it belongs to.
type ClusterResourceSetBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterResourceSetBindingSpec `json:"spec,omitempty"`
}

// ANCHOR: ClusterResourceSetBindingSpec

// ClusterResourceSetBindingSpec defines the desired state of ClusterResourceSetBinding
type ClusterResourceSetBindingSpec struct {
	// Bindings is a map of ClusterResourceSet name and its resources which is also a map.
	Bindings map[string]ResourcesSetBinding `json:"bindings,omitempty"`
}

// ANCHOR_END: ClusterResourceSetBindingSpec

// +kubebuilder:object:root=true

// ClusterResourceSetBindingList contains a list of ClusterResourceSetBinding
type ClusterResourceSetBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterResourceSetBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterResourceSetBinding{}, &ClusterResourceSetBindingList{})
}
