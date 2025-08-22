/*
Copyright 2024 The Kubernetes Authors.

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

package tree

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// NodeObject represent a node in the tree which doesn't correspond to Cluster, MachineDeployment, Machine etc.
// An example of NodeObject are GroupNodes, which are used e.g. to represent a set of Machines.
// Note: NodeObject implements condition getter and setter interfaces as well as the minimal set of methods
// usually existing on Kubernetes objects.
type NodeObject struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status NodeStatus
}

// NodeStatus is the status of a node object.
type NodeStatus struct {
	Deprecated *NodeDeprecatedStatus
	Conditions []metav1.Condition
}

// NodeDeprecatedStatus groups all the status fields that are deprecated and will be removed in a future version.
// See https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20240916-improve-status-in-CAPI-resources.md for more context.
type NodeDeprecatedStatus struct {
	V1Beta1 *NodeV1Beta1DeprecatedStatus
}

// NodeV1Beta1DeprecatedStatus groups all the status fields that are deprecated and will be removed when support for v1beta1 will be dropped.
type NodeV1Beta1DeprecatedStatus struct {
	Conditions clusterv1.Conditions
}

// GetV1Beta1Conditions returns the set of conditions for this object.
func (o *NodeObject) GetV1Beta1Conditions() clusterv1.Conditions {
	if o.Status.Deprecated == nil || o.Status.Deprecated.V1Beta1 == nil {
		return nil
	}
	return o.Status.Deprecated.V1Beta1.Conditions
}

// SetV1Beta1Conditions sets the conditions on this object.
func (o *NodeObject) SetV1Beta1Conditions(conditions clusterv1.Conditions) {
	if o.Status.Deprecated == nil {
		o.Status.Deprecated = &NodeDeprecatedStatus{}
	}
	if o.Status.Deprecated.V1Beta1 == nil {
		o.Status.Deprecated.V1Beta1 = &NodeV1Beta1DeprecatedStatus{}
	}
	o.Status.Deprecated.V1Beta1.Conditions = conditions
}

// GetConditions returns the set of conditions for this object.
func (o *NodeObject) GetConditions() []metav1.Condition {
	return o.Status.Conditions
}

// SetConditions sets conditions for an API object.
func (o *NodeObject) SetConditions(conditions []metav1.Condition) {
	o.Status.Conditions = conditions
}

// GetUID returns object's UID.
func (o *NodeObject) GetUID() types.UID {
	return o.UID
}

// SetUID sets object's UID.
func (o *NodeObject) SetUID(uid types.UID) {
	o.UID = uid
}

// GetCreationTimestamp returns object's CreationTimestamp.
func (o *NodeObject) GetCreationTimestamp() metav1.Time {
	return o.CreationTimestamp
}

// SetCreationTimestamp sets object's CreationTimestamp.
func (o *NodeObject) SetCreationTimestamp(timestamp metav1.Time) {
	o.CreationTimestamp = timestamp
}

// GetDeletionTimestamp returns object's DeletionTimestamp.
func (o *NodeObject) GetDeletionTimestamp() *metav1.Time {
	return o.DeletionTimestamp
}

// SetDeletionTimestamp sets object's DeletionTimestamp.
func (o *NodeObject) SetDeletionTimestamp(timestamp *metav1.Time) {
	o.DeletionTimestamp = timestamp
}

// GetOwnerReferences returns object's OwnerReferences.
func (o *NodeObject) GetOwnerReferences() []metav1.OwnerReference {
	return o.OwnerReferences
}

// SetOwnerReferences sets object's OwnerReferences.
func (o *NodeObject) SetOwnerReferences(references []metav1.OwnerReference) {
	o.OwnerReferences = references
}

// GetManagedFields returns object's ManagedFields.
func (o *NodeObject) GetManagedFields() []metav1.ManagedFieldsEntry {
	return o.ManagedFields
}

// SetManagedFields sets object's ManagedFields.
func (o *NodeObject) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {
	o.ManagedFields = managedFields
}

// DeepCopyObject returns a deep copy of the object.
func (o *NodeObject) DeepCopyObject() runtime.Object {
	panic("implement me")
}
