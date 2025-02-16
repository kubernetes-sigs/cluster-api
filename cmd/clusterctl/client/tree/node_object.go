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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	Conditions clusterv1.Conditions
	V1Beta2    *NodeObjectV1Beta2Status
}

// NodeObjectV1Beta2Status is the v1Beta2 status of a node object.
type NodeObjectV1Beta2Status struct {
	Conditions []metav1.Condition
}

// GetConditions returns the set of conditions for this object.
func (o *NodeObject) GetConditions() clusterv1.Conditions {
	return o.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (o *NodeObject) SetConditions(conditions clusterv1.Conditions) {
	o.Status.Conditions = conditions
}

// GetV1Beta2Conditions returns the set of conditions for this object.
func (o *NodeObject) GetV1Beta2Conditions() []metav1.Condition {
	if o.Status.V1Beta2 == nil {
		return nil
	}
	return o.Status.V1Beta2.Conditions
}

// SetV1Beta2Conditions sets conditions for an API object.
func (o *NodeObject) SetV1Beta2Conditions(conditions []metav1.Condition) {
	if o.Status.V1Beta2 == nil && conditions != nil {
		o.Status.V1Beta2 = &NodeObjectV1Beta2Status{}
	}
	o.Status.V1Beta2.Conditions = conditions
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
