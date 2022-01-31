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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectReference should be used to reference objects in a different namespace.
type ObjectReference struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind,omitempty" protobuf:"bytes,1,opt,name=kind"`
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name,omitempty" protobuf:"bytes,2,opt,name=name"`
	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,2,opt,name=namespace"`
	// API group of the referent.
	// +optional
	Group string `json:"group,omitempty" protobuf:"bytes,3,opt,name=apiVersion"`
}

func NewObjectReference(obj client.Object) *ObjectReference {
	ref := &ObjectReference{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	ref.SetGroupKind(obj.GetObjectKind().GroupVersionKind().GroupKind())
	return ref
}

func (r *ObjectReference) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: r.Group, Kind: r.Kind}
}

func (r *ObjectReference) SetGroupKind(gk schema.GroupKind) {
	r.Group = gk.Group
	r.Kind = gk.Kind
}

func (r *ObjectReference) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Name: r.Name, Namespace: r.Namespace}
}

func (r *ObjectReference) Pin(uid types.UID) *PinnedObjectReference {
	pinned := PinnedObjectReference{
		Name:      r.Name,
		Namespace: r.Namespace,
		UID:       uid,
	}
	pinned.SetGroupKind(r.GroupKind())
	return &pinned
}

func (r *ObjectReference) ToLocal() *LocalObjectReference {
	local := LocalObjectReference{
		Name: r.Name,
	}
	local.SetGroupKind(r.GroupKind())
	return &local
}

// LocalObjectReference should be used to reference to objects in the same namespace.
type LocalObjectReference struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind,omitempty" protobuf:"bytes,1,opt,name=kind"`
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name,omitempty" protobuf:"bytes,2,opt,name=name"`
	// API group of the referent.
	// +optional
	Group string `json:"group,omitempty" protobuf:"bytes,3,opt,name=apiVersion"`
}

func NewLocalObjectReference(obj client.Object) *LocalObjectReference {
	ref := &LocalObjectReference{
		Name: obj.GetName(),
	}
	ref.SetGroupKind(obj.GetObjectKind().GroupVersionKind().GroupKind())
	return ref
}

func (r *LocalObjectReference) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: r.Group, Kind: r.Kind}
}

func (r *LocalObjectReference) SetGroupKind(gk schema.GroupKind) {
	r.Group = gk.Group
	r.Kind = gk.Kind
}

func (r *LocalObjectReference) NamespacedName(namespace string) types.NamespacedName {
	return r.ToNamespaced(namespace).NamespacedName()
}

func (r *LocalObjectReference) Pin(uid types.UID) *PinnedLocalObjectReference {
	pinned := PinnedLocalObjectReference{
		Name: r.Name,
		UID:  uid,
	}
	pinned.SetGroupKind(r.GroupKind())
	return &pinned
}

func (r *LocalObjectReference) ToNamespaced(namespace string) *ObjectReference {
	namespaced := ObjectReference{
		Name:      r.Name,
		Namespace: namespace,
	}
	namespaced.SetGroupKind(r.GroupKind())
	return &namespaced
}

// PinnedObjectReference should be used to point to a specific resource in a different namespace.
// It is most useful in the status to be precise about what exact object is referenced.
type PinnedObjectReference struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind,omitempty" protobuf:"bytes,1,opt,name=kind"`
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name,omitempty" protobuf:"bytes,2,opt,name=name"`
	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,2,opt,name=namespace"`
	// API group of the referent.
	// +optional
	Group string `json:"group,omitempty" protobuf:"bytes,3,opt,name=apiVersion"`
	// UID of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids
	UID types.UID `json:"uid,omitempty" protobuf:"bytes,4,opt,name=uid"`
}

func NewPinnedObjectReference(obj client.Object) *PinnedObjectReference {
	ref := &PinnedObjectReference{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		UID:       obj.GetUID(),
	}
	ref.SetGroupKind(obj.GetObjectKind().GroupVersionKind().GroupKind())
	return ref
}

func (r *PinnedObjectReference) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: r.Group, Kind: r.Kind}
}

func (r *PinnedObjectReference) SetGroupKind(gk schema.GroupKind) {
	r.Group = gk.Group
	r.Kind = gk.Kind
}

func (r *PinnedObjectReference) NamespacedName() types.NamespacedName {
	return r.Unpin().NamespacedName()
}

func (r *PinnedObjectReference) Unpin() *ObjectReference {
	unpinned := ObjectReference{
		Name:      r.Name,
		Namespace: r.Namespace,
	}
	unpinned.SetGroupKind(r.GroupKind())
	return &unpinned
}

// PinnedObjectReference should be used to point to a specific resource in the same namespace.
// It is most useful in the status to be precise about what exact object is referenced.
type PinnedLocalObjectReference struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	Kind string `json:"kind,omitempty" protobuf:"bytes,1,opt,name=kind"`
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name,omitempty" protobuf:"bytes,2,opt,name=name"`
	// API group of the referent.
	// +optional
	Group string `json:"group,omitempty" protobuf:"bytes,3,opt,name=apiVersion"`
	// UID of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids
	UID types.UID `json:"uid,omitempty" protobuf:"bytes,4,opt,name=uid"`
}

func NewPinnedLocalObjectReference(obj client.Object) *PinnedLocalObjectReference {
	ref := &PinnedLocalObjectReference{
		Name: obj.GetName(),
		UID:  obj.GetUID(),
	}
	ref.SetGroupKind(obj.GetObjectKind().GroupVersionKind().GroupKind())
	return ref
}

func (r *PinnedLocalObjectReference) GroupKind() schema.GroupKind {
	return schema.GroupKind{Group: r.Group, Kind: r.Kind}
}

func (r *PinnedLocalObjectReference) SetGroupKind(gk schema.GroupKind) {
	r.Group = gk.Group
	r.Kind = gk.Kind
}

func (r *PinnedLocalObjectReference) NamespacedName(namespace string) types.NamespacedName {
	return r.Unpin().NamespacedName(namespace)
}

func (r *PinnedLocalObjectReference) Unpin() *LocalObjectReference {
	unpinned := LocalObjectReference{
		Name: r.Name,
	}
	unpinned.SetGroupKind(r.GroupKind())
	return &unpinned
}
