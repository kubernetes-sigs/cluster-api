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
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
)

// ObjectTreeOptions defines the options for an ObjectTree.
type ObjectTreeOptions struct {
	// ShowOtherConditions is a list of comma separated kind or kind/name for which we should add   the ShowObjectConditionsAnnotation
	// to signal to the presentation layer to show all the conditions for the objects.
	ShowOtherConditions string

	// ShowMachineSets instructs the discovery process to include machine sets in the ObjectTree.
	ShowMachineSets bool

	// Echo displays objects if the object's ready condition has the
	// same Status, Severity and Reason of the parent's object ready condition (it is an echo)
	Echo bool

	// Grouping groups sibling object in case the ready conditions
	// have the same Status, Severity and Reason
	Grouping bool
}

// ObjectTree defines an object tree representing the status of a Cluster API cluster.
type ObjectTree struct {
	root      client.Object
	options   ObjectTreeOptions
	items     map[types.UID]client.Object
	ownership map[types.UID]map[types.UID]bool
}

// NewObjectTree creates a new object tree with the given root and options.
func NewObjectTree(root client.Object, options ObjectTreeOptions) *ObjectTree {
	// If it is requested to show all the conditions for the root, add
	// the ShowObjectConditionsAnnotation to signal this to the presentation layer.
	if isObjDebug(root, options.ShowOtherConditions) {
		addAnnotation(root, ShowObjectConditionsAnnotation, "True")
	}

	return &ObjectTree{
		root:      root,
		options:   options,
		items:     make(map[types.UID]client.Object),
		ownership: make(map[types.UID]map[types.UID]bool),
	}
}

// Add a object to the object tree.
func (od ObjectTree) Add(parent, obj client.Object, opts ...AddObjectOption) (added bool, visible bool) {
	if parent == nil || obj == nil {
		return false, false
	}
	addOpts := &addObjectOptions{}
	addOpts.ApplyOptions(opts)

	objReady := GetReadyCondition(obj)
	parentReady := GetReadyCondition(parent)

	// If it is requested to show all the conditions for the object, add
	// the ShowObjectConditionsAnnotation to signal this to the presentation layer.
	if isObjDebug(obj, od.options.ShowOtherConditions) {
		addAnnotation(obj, ShowObjectConditionsAnnotation, "True")
	}

	// If the object should be hidden if the object's ready condition is true ot it has the
	// same Status, Severity and Reason of the parent's object ready condition (it is an echo),
	// return early.
	if addOpts.NoEcho && !od.options.Echo {
		if (objReady != nil && objReady.Status == corev1.ConditionTrue) || hasSameReadyStatusSeverityAndReason(parentReady, objReady) {
			return false, false
		}
	}

	// If it is requested to use a meta name for the object in the presentation layer, add
	// the ObjectMetaNameAnnotation to signal this to the presentation layer.
	if addOpts.MetaName != "" {
		addAnnotation(obj, ObjectMetaNameAnnotation, addOpts.MetaName)
	}

	// If it is requested that this object and its sibling should be grouped in case the ready condition
	// has the same Status, Severity and Reason, process all the sibling nodes.
	if IsGroupingObject(parent) {
		siblings := od.GetObjectsByParent(parent.GetUID())

		for i := range siblings {
			s := siblings[i]
			sReady := GetReadyCondition(s)

			// If the object's ready condition has a different Status, Severity and Reason than the sibling object,
			// move on (they should not be grouped).
			if !hasSameReadyStatusSeverityAndReason(objReady, sReady) {
				continue
			}

			// If the sibling node is already a group object, upgrade it with the current object.
			if IsGroupObject(s) {
				updateGroupNode(s, sReady, obj, objReady)
				return true, false
			}

			// Otherwise the object and the current sibling should be merged in a group.

			// Create virtual object for the group and add it to the object tree.
			groupNode := createGroupNode(s, sReady, obj, objReady)
			od.addInner(parent, groupNode)

			// Remove the current sibling (now merged in the group).
			od.remove(parent, s)
			return true, false
		}
	}

	// If it is requested that the child of this node should be grouped in case the ready condition
	// has the same Status, Severity and Reason, add the GroupingObjectAnnotation to signal
	// this to the presentation layer.
	if addOpts.GroupingObject && od.options.Grouping {
		addAnnotation(obj, GroupingObjectAnnotation, "True")
	}

	// Add the object to the object tree.
	od.addInner(parent, obj)

	return true, true
}

func (od ObjectTree) remove(parent client.Object, s client.Object) {
	for _, child := range od.GetObjectsByParent(s.GetUID()) {
		od.remove(s, child)
	}
	delete(od.items, s.GetUID())
	delete(od.ownership[parent.GetUID()], s.GetUID())
}

func (od ObjectTree) addInner(parent client.Object, obj client.Object) {
	od.items[obj.GetUID()] = obj
	if od.ownership[parent.GetUID()] == nil {
		od.ownership[parent.GetUID()] = make(map[types.UID]bool)
	}
	od.ownership[parent.GetUID()][obj.GetUID()] = true
}

// GetRoot returns the root of the tree.
func (od ObjectTree) GetRoot() client.Object { return od.root }

// GetObject returns the object with the given uid.
func (od ObjectTree) GetObject(id types.UID) client.Object { return od.items[id] }

// IsObjectWithChild determines if an object has dependants.
func (od ObjectTree) IsObjectWithChild(id types.UID) bool {
	return len(od.ownership[id]) > 0
}

// GetObjectsByParent returns all the dependant objects for the given uid.
func (od ObjectTree) GetObjectsByParent(id types.UID) []client.Object {
	out := make([]client.Object, 0, len(od.ownership[id]))
	for k := range od.ownership[id] {
		out = append(out, od.GetObject(k))
	}
	return out
}

func hasSameReadyStatusSeverityAndReason(a, b *clusterv1.Condition) bool {
	if a == nil && b == nil {
		return true
	}
	if (a == nil) != (b == nil) {
		return false
	}

	return a.Status == b.Status &&
		a.Severity == b.Severity &&
		a.Reason == b.Reason
}

func createGroupNode(sibling client.Object, siblingReady *clusterv1.Condition, obj client.Object, objReady *clusterv1.Condition) *unstructured.Unstructured {
	kind := fmt.Sprintf("%sGroup", obj.GetObjectKind().GroupVersionKind().Kind)

	// Create a new group node and add the GroupObjectAnnotation to signal
	// this to the presentation layer.
	// NB. The group nodes gets a unique ID to avoid conflicts.
	groupNode := VirtualObject(obj.GetNamespace(), kind, readyStatusSeverityAndReasonUID(obj))
	addAnnotation(groupNode, GroupObjectAnnotation, "True")

	// Update the list of items included in the group and store it in the GroupItemsAnnotation.
	items := []string{obj.GetName(), sibling.GetName()}
	sort.Strings(items)
	addAnnotation(groupNode, GroupItemsAnnotation, strings.Join(items, GroupItemsSeparator))

	// Update the group's ready condition.
	if objReady != nil {
		objReady.LastTransitionTime = minLastTransitionTime(objReady, siblingReady)
		objReady.Message = ""
		setReadyCondition(groupNode, objReady)
	}
	return groupNode
}

func readyStatusSeverityAndReasonUID(obj client.Object) string {
	ready := GetReadyCondition(obj)
	if ready == nil {
		return fmt.Sprintf("zzz_%s", util.RandomString(6))
	}
	return fmt.Sprintf("zz_%s_%s_%s_%s", ready.Status, ready.Severity, ready.Reason, util.RandomString(6))
}

func minLastTransitionTime(a, b *clusterv1.Condition) metav1.Time {
	if a == nil && b == nil {
		return metav1.Time{}
	}
	if (a != nil) && (b == nil) {
		return a.LastTransitionTime
	}
	if (a == nil) && (b != nil) {
		return b.LastTransitionTime
	}
	if a.LastTransitionTime.Time.After(b.LastTransitionTime.Time) {
		return b.LastTransitionTime
	}
	return a.LastTransitionTime
}

func updateGroupNode(groupObj client.Object, groupReady *clusterv1.Condition, obj client.Object, objReady *clusterv1.Condition) {
	// Update the list of items included in the group and store it in the GroupItemsAnnotation.
	items := strings.Split(GetGroupItems(groupObj), GroupItemsSeparator)
	items = append(items, obj.GetName())
	sort.Strings(items)
	addAnnotation(groupObj, GroupItemsAnnotation, strings.Join(items, GroupItemsSeparator))

	// Update the group's ready condition.
	if groupReady != nil {
		groupReady.LastTransitionTime = minLastTransitionTime(objReady, groupReady)
		groupReady.Message = ""
		setReadyCondition(groupObj, groupReady)
	}
}

func isObjDebug(obj client.Object, debugFilter string) bool {
	if debugFilter == "" {
		return false
	}
	for _, filter := range strings.Split(debugFilter, ",") {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		if strings.EqualFold(filter, "all") {
			return true
		}
		kn := strings.Split(filter, "/")
		if len(kn) == 2 {
			if obj.GetObjectKind().GroupVersionKind().Kind == kn[0] && obj.GetName() == kn[1] {
				return true
			}
			continue
		}
		if obj.GetObjectKind().GroupVersionKind().Kind == kn[0] {
			return true
		}
	}
	return false
}
