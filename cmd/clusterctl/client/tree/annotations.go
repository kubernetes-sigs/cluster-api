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

package tree

import (
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ShowObjectConditionsAnnotation documents that the presentation layer should show all the conditions for the object.
	ShowObjectConditionsAnnotation = "tree.cluster.x-k8s.io.io/show-conditions"

	// ObjectMetaNameAnnotation contains the meta name that should be used for the object in the presentation layer,
	// e.g. control plane for KCP.
	ObjectMetaNameAnnotation = "tree.cluster.x-k8s.io.io/meta-name"

	// VirtualObjectAnnotation documents that the object does not correspond to any real object, but instead is
	// a virtual object introduced to provide a better representation of the cluster status, e.g. workers.
	VirtualObjectAnnotation = "tree.cluster.x-k8s.io.io/virtual-object"

	// GroupingObjectAnnotation is an annotation that should be applied to a node in order to trigger the grouping action
	// when adding the node's children. e.g. if you have a control-plane node, and you apply this annotation, then
	// the control-plane machines added as a children of this node will be grouped in case the ready condition
	// has the same Status, Severity and Reason.
	GroupingObjectAnnotation = "tree.cluster.x-k8s.io.io/grouping-object"

	// GroupObjectAnnotation is an annotation that documents that a node is the result of a grouping operation, and
	// thus the node is representing group of sibling nodes, e.g. a group of machines.
	GroupObjectAnnotation = "tree.cluster.x-k8s.io.io/group-object"

	// GroupItemsAnnotation contains the list of names for the objects included in a group object.
	GroupItemsAnnotation = "tree.cluster.x-k8s.io.io/group-items"

	// GroupItemsSeparator is the separator used in the GroupItemsAnnotation.
	GroupItemsSeparator = ", "
)

// GetMetaName returns the object meta name that should be used for the object in the presentation layer, if defined.
func GetMetaName(obj client.Object) string {
	if val, ok := getAnnotation(obj, ObjectMetaNameAnnotation); ok {
		return val
	}
	return ""
}

// IsGroupingObject returns true in case the object is responsible to trigger the grouping action
// when adding the object's children. e.g. A control-plane object, could be responsible of grouping
// the control-plane machines while added as a children objects.
func IsGroupingObject(obj client.Object) bool {
	if val, ok := getBoolAnnotation(obj, GroupingObjectAnnotation); ok {
		return val
	}
	return false
}

// IsGroupObject return true if the object is the result of a grouping operation, and
// thus the object is representing group of sibling object, e.g. a group of machines.
func IsGroupObject(obj client.Object) bool {
	if val, ok := getBoolAnnotation(obj, GroupObjectAnnotation); ok {
		return val
	}
	return false
}

// GetGroupItems return the list of names for the objects included in a group object.
func GetGroupItems(obj client.Object) string {
	if val, ok := getAnnotation(obj, GroupItemsAnnotation); ok {
		return val
	}
	return ""
}

// IsVirtualObject return true if the object does not correspond to any real object, but instead it is
// a virtual object introduced to provide a better representation of the cluster status.
func IsVirtualObject(obj client.Object) bool {
	if val, ok := getBoolAnnotation(obj, VirtualObjectAnnotation); ok {
		return val
	}
	return false
}

// IsShowConditionsObject returns true if the presentation layer should show all the conditions for the object.
func IsShowConditionsObject(obj client.Object) bool {
	if val, ok := getBoolAnnotation(obj, ShowObjectConditionsAnnotation); ok {
		return val
	}
	return false
}

func getAnnotation(obj client.Object, annotation string) (string, bool) {
	if obj == nil {
		return "", false
	}
	val, ok := obj.GetAnnotations()[annotation]
	return val, ok
}

func getBoolAnnotation(obj client.Object, annotation string) (bool, bool) {
	val, ok := getAnnotation(obj, annotation)
	if ok {
		if boolVal, err := strconv.ParseBool(val); err == nil {
			return boolVal, true
		}
	}
	return false, false
}

func addAnnotation(obj client.Object, annotation, value string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotation] = value
	obj.SetAnnotations(annotations)
}
