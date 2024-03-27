/*
Copyright 2023 The Kubernetes Authors.

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

// Package ownerrefs contains ownerref utils.
package ownerrefs

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OwnerReferenceTo converts an object to an OwnerReference.
// Note: We pass in gvk explicitly as we can't rely on GVK being set on all objects
// (only on Unstructured).
func OwnerReferenceTo(obj client.Object, gvk schema.GroupVersionKind) *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: gvk.GroupVersion().String(),
		Kind:       gvk.Kind,
		Name:       obj.GetName(),
		UID:        obj.GetUID(),
	}
}

// HasOwnerReferenceFrom checks if obj has an ownerRef pointing to owner.
func HasOwnerReferenceFrom(obj, owner client.Object) bool {
	for _, o := range obj.GetOwnerReferences() {
		if o.Kind == owner.GetObjectKind().GroupVersionKind().Kind && o.Name == owner.GetName() {
			return true
		}
	}
	return false
}
