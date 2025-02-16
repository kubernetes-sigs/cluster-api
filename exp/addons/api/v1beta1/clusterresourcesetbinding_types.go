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
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ANCHOR: ResourceBinding

// ResourceBinding shows the status of a resource that belongs to a ClusterResourceSet matched by the owner cluster of the ClusterResourceSetBinding object.
type ResourceBinding struct {
	// ResourceRef specifies a resource.
	ResourceRef `json:",inline"`

	// hash is the hash of a resource's data. This can be used to decide if a resource is changed.
	// For "ApplyOnce" ClusterResourceSet.spec.strategy, this is no-op as that strategy does not act on change.
	// +optional
	Hash string `json:"hash,omitempty"`

	// lastAppliedTime identifies when this resource was last applied to the cluster.
	// +optional
	LastAppliedTime *metav1.Time `json:"lastAppliedTime,omitempty"`

	// applied is to track if a resource is applied to the cluster or not.
	Applied bool `json:"applied"`
}

// ANCHOR_END: ResourceBinding

// ResourceSetBinding keeps info on all of the resources in a ClusterResourceSet.
type ResourceSetBinding struct {
	// clusterResourceSetName is the name of the ClusterResourceSet that is applied to the owner cluster of the binding.
	ClusterResourceSetName string `json:"clusterResourceSetName"`

	// resources is a list of resources that the ClusterResourceSet has.
	// +optional
	Resources []ResourceBinding `json:"resources,omitempty"`
}

// IsApplied returns true if the resource is applied to the cluster by checking the cluster's binding.
func (r *ResourceSetBinding) IsApplied(resourceRef ResourceRef) bool {
	resourceBinding := r.GetResource(resourceRef)
	return resourceBinding != nil && resourceBinding.Applied
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
	for _, binding := range c.Spec.Bindings {
		if binding.ClusterResourceSetName == clusterResourceSet.Name {
			return binding
		}
	}
	binding := &ResourceSetBinding{ClusterResourceSetName: clusterResourceSet.Name, Resources: []ResourceBinding{}}
	c.Spec.Bindings = append(c.Spec.Bindings, binding)
	return binding
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

// DeleteBinding removes the ClusterResourceSet from the ClusterResourceSetBinding Bindings list.
//
// Deprecated: This function is deprecated and will be removed in an upcoming release of Cluster API.
func (c *ClusterResourceSetBinding) DeleteBinding(clusterResourceSet *ClusterResourceSet) {
	for i, binding := range c.Spec.Bindings {
		if binding.ClusterResourceSetName == clusterResourceSet.Name {
			copy(c.Spec.Bindings[i:], c.Spec.Bindings[i+1:])
			c.Spec.Bindings = c.Spec.Bindings[:len(c.Spec.Bindings)-1]
			break
		}
	}
	c.OwnerReferences = removeOwnerRef(c.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: GroupVersion.String(),
		Kind:       "ClusterResourceSet",
		Name:       clusterResourceSet.Name,
	})
}

// removeOwnerRef returns the slice of owner references after removing the supplied owner ref.
// Note: removeOwnerRef ignores apiVersion and UID. It will remove the passed ownerReference where it matches Name, Group and Kind.
//
// Deprecated: This function is deprecated and will be removed in an upcoming release of Cluster API.
func removeOwnerRef(ownerReferences []metav1.OwnerReference, inputRef metav1.OwnerReference) []metav1.OwnerReference {
	if index := indexOwnerRef(ownerReferences, inputRef); index != -1 {
		return append(ownerReferences[:index], ownerReferences[index+1:]...)
	}
	return ownerReferences
}

// indexOwnerRef returns the index of the owner reference in the slice if found, or -1.
//
// Deprecated: This function is deprecated and will be removed in an upcoming release of Cluster API.
func indexOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) int {
	for index, r := range ownerReferences {
		if referSameObject(r, ref) {
			return index
		}
	}
	return -1
}

// Returns true if a and b point to the same object based on Group, Kind and Name.
//
// Deprecated: This function is deprecated and will be removed in an upcoming release of Cluster API.
func referSameObject(a, b metav1.OwnerReference) bool {
	aGV, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	bGV, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return aGV.Group == bGV.Group && a.Kind == b.Kind && a.Name == b.Name
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
	// bindings is a list of ClusterResourceSets and their resources.
	// +optional
	Bindings []*ResourceSetBinding `json:"bindings,omitempty"`

	// clusterName is the name of the Cluster this binding applies to.
	// Note: this field mandatory in v1beta2.
	// +optional
	ClusterName string `json:"clusterName,omitempty"`
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
	objectTypes = append(objectTypes, &ClusterResourceSetBinding{}, &ClusterResourceSetBindingList{})
}
