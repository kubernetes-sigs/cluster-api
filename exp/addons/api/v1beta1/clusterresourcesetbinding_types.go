/*
Copyright 2021 The Kubernetes Authors.

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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ANCHOR: ResourceBinding

// ResourceBinding shows the status of a resource that belongs to a ClusterResourceSet matched by the owner cluster of the ClusterResourceSetBinding object.
type ResourceBinding struct {
	// ResourceRef specifies a resource.
	ResourceRef `json:",inline"`

	// Hash is the hash of a resource's data. This can be used to decide if a resource is changed.
	// For "ApplyOnce" ClusterResourceSet.spec.strategy, this is no-op as that strategy does not act on change.
	// +optional
	Hash string `json:"hash,omitempty"`

	// LastAppliedTime identifies when this resource was last applied to the cluster.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`

	// Applied is to track if a resource is applied to the cluster or not.
	Applied bool `json:"applied"`
}

// ANCHOR_END: ResourceBinding

// ResourceSetBinding keeps info on all of the resources in a ClusterResourceSet.
type ResourceSetBinding struct {
	// ClusterResourceSetName is the name of the ClusterResourceSet that is applied to the owner cluster of the binding.
	ClusterResourceSetName string `json:"clusterResourceSetName"`

	// Resources is a list of resources that the ClusterResourceSet has.
	// +optional
	Resources []ResourceBinding `json:"resources,omitempty"`
}

// IsApplied returns true if the resource is applied to the cluster by checking the cluster's binding.
func (r *ResourceSetBinding) IsApplied(resourceRef ResourceRef) bool {
	for _, resource := range r.Resources {
		if reflect.DeepEqual(resource.ResourceRef, resourceRef) {
			if resource.Applied {
				return true
			}
		}
	}
	return false
}

// SetBinding sets resourceBinding for a resource in resourceSetbinding either by updating the existing one or
// creating a new one.
func (r *ResourceSetBinding) SetBinding(resourceBinding ResourceBinding) {
	for i := range r.Resources {
		if reflect.DeepEqual(r.Resources[i].ResourceRef, resourceBinding.ResourceRef) {
			r.Resources[i] = resourceBinding
			return
		}
	}
	r.Resources = append(r.Resources, resourceBinding)
}

// GetOrCreateBinding returns the ResourceSetBinding for a given ClusterResourceSet if exists,
// otherwise creates one and updates ClusterResourceSet with it.
func (c *ClusterResourceSetBinding) GetOrCreateBinding(clusterResourceSet *ClusterResourceSet) *ResourceSetBinding {
	for _, binding := range c.Spec.Bindings {
		if binding.ClusterResourceSetName == clusterResourceSet.Name {
			return binding
		}
	}
	binding := &ResourceSetBinding{ClusterResourceSetName: clusterResourceSet.Name, Resources: []ResourceBinding{}}
	c.Spec.Bindings = append(c.Spec.Bindings, binding)
	return binding
}

// DeleteBinding removes the ClusterResourceSet from the ClusterResourceSetBinding Bindings list.
func (c *ClusterResourceSetBinding) DeleteBinding(clusterResourceSet *ClusterResourceSet) {
	for i, binding := range c.Spec.Bindings {
		if binding.ClusterResourceSetName == clusterResourceSet.Name {
			copy(c.Spec.Bindings[i:], c.Spec.Bindings[i+1:])
			c.Spec.Bindings = c.Spec.Bindings[:len(c.Spec.Bindings)-1]
			break
		}
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterresourcesetbindings,scope=Namespaced,categories=cluster-api
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ClusterResourceSetBinding"

// ClusterResourceSetBinding lists all matching ClusterResourceSets with the cluster it belongs to.
type ClusterResourceSetBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterResourceSetBindingSpec `json:"spec,omitempty"`
}

// ANCHOR: ClusterResourceSetBindingSpec

// ClusterResourceSetBindingSpec defines the desired state of ClusterResourceSetBinding.
type ClusterResourceSetBindingSpec struct {
	// Bindings is a list of ClusterResourceSets and their resources.
	// +optional
	Bindings []*ResourceSetBinding `json:"bindings,omitempty"`
}

// ANCHOR_END: ClusterResourceSetBindingSpec

// +kubebuilder:object:root=true

// ClusterResourceSetBindingList contains a list of ClusterResourceSetBinding.
type ClusterResourceSetBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterResourceSetBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterResourceSetBinding{}, &ClusterResourceSetBindingList{})
}
