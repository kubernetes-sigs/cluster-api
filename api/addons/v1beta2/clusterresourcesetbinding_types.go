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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// ResourceBinding shows the status of a resource that belongs to a ClusterResourceSet matched by the owner cluster of the ClusterResourceSetBinding object.
type ResourceBinding struct {
	// ResourceRef specifies a resource.
	ResourceRef `json:",inline"`

	// hash is the hash of a resource's data. This can be used to decide if a resource is changed.
	// For "ApplyOnce" ClusterResourceSet.spec.strategy, this is no-op as that strategy does not act on change.
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Hash string `json:"hash,omitempty"`

	// lastAppliedTime identifies when this resource was last applied to the cluster.
	// +optional
	LastAppliedTime metav1.Time `json:"lastAppliedTime,omitempty,omitzero"`

	// applied is to track if a resource is applied to the cluster or not.
	// +required
	Applied *bool `json:"applied,omitempty"`
}

// ResourceSetBinding keeps info on all of the resources in a ClusterResourceSet.
type ResourceSetBinding struct {
	// clusterResourceSetName is the name of the ClusterResourceSet that is applied to the owner cluster of the binding.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	ClusterResourceSetName string `json:"clusterResourceSetName,omitempty"`

	// resources is a list of resources that the ClusterResourceSet has.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	Resources []ResourceBinding `json:"resources,omitempty"`
}

// IsApplied returns true if the resource is applied to the cluster by checking the cluster's binding.
func (r *ResourceSetBinding) IsApplied(resourceRef ResourceRef) bool {
	resourceBinding := r.GetResource(resourceRef)
	return resourceBinding != nil && ptr.Deref(resourceBinding.Applied, false)
}

// GetResource returns a ResourceBinding for a resource ref if present.
func (r *ResourceSetBinding) GetResource(resourceRef ResourceRef) *ResourceBinding {
	for _, resource := range r.Resources {
		if reflect.DeepEqual(resource.ResourceRef, resourceRef) {
			return &resource
		}
	}
	return nil
}

// SetBinding sets resourceBinding for a resource in ResourceSetBinding either by updating the existing one or
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
	for i := range c.Spec.Bindings {
		if c.Spec.Bindings[i].ClusterResourceSetName == clusterResourceSet.Name {
			return &c.Spec.Bindings[i]
		}
	}
	binding := ResourceSetBinding{ClusterResourceSetName: clusterResourceSet.Name, Resources: []ResourceBinding{}}
	c.Spec.Bindings = append(c.Spec.Bindings, binding)
	return &c.Spec.Bindings[len(c.Spec.Bindings)-1]
}

// RemoveBinding removes the ClusterResourceSet from the ClusterResourceSetBinding Bindings list.
func (c *ClusterResourceSetBinding) RemoveBinding(clusterResourceSet *ClusterResourceSet) {
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
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.clusterName",description="Cluster"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of ClusterResourceSetBinding"

// ClusterResourceSetBinding lists all matching ClusterResourceSets with the cluster it belongs to.
type ClusterResourceSetBinding struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec is the desired state of ClusterResourceSetBinding.
	// +required
	Spec ClusterResourceSetBindingSpec `json:"spec,omitempty,omitzero"`
}

// ClusterResourceSetBindingSpec defines the desired state of ClusterResourceSetBinding.
type ClusterResourceSetBindingSpec struct {
	// bindings is a list of ClusterResourceSets and their resources.
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=100
	Bindings []ResourceSetBinding `json:"bindings,omitempty"`

	// clusterName is the name of the Cluster this binding applies to.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	ClusterName string `json:"clusterName,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterResourceSetBindingList contains a list of ClusterResourceSetBinding.
type ClusterResourceSetBindingList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#lists-and-simple-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is the list of ClusterResourceSetBindings.
	Items []ClusterResourceSetBinding `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &ClusterResourceSetBinding{}, &ClusterResourceSetBindingList{})
}
