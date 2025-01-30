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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// ShowClusterResourceSets instructs the discovery process to include cluster resource sets in the ObjectTree.
	ShowClusterResourceSets bool

	// ShowTemplates instructs the discovery process to include infrastructure and bootstrap config templates in the ObjectTree.
	ShowTemplates bool

	// AddTemplateVirtualNode instructs the discovery process to group template under a virtual node.
	AddTemplateVirtualNode bool

	// Echo displays objects if the object's ready condition has the
	// same Status, Severity and Reason of the parent's object ready condition (it is an echo)
	Echo bool

	// Grouping groups sibling object in case the ready conditions
	// have the same Status, Severity and Reason
	Grouping bool

	// V1Beta2 instructs tree to use V1Beta2 conditions.
	V1Beta2 bool
}

// ObjectTree defines an object tree representing the status of a Cluster API cluster.
type ObjectTree struct {
	root       client.Object
	options    ObjectTreeOptions
	items      map[types.UID]client.Object
	ownership  map[types.UID]map[types.UID]bool
	parentship map[types.UID]types.UID
}

// NewObjectTree creates a new object tree with the given root and options.
func NewObjectTree(root client.Object, options ObjectTreeOptions) *ObjectTree {
	// If it is requested to show all the conditions for the root, add
	// the ShowObjectConditionsAnnotation to signal this to the presentation layer.
	if isObjDebug(root, options.ShowOtherConditions) {
		addAnnotation(root, ShowObjectConditionsAnnotation, "True")
	}

	return &ObjectTree{
		root:       root,
		options:    options,
		items:      make(map[types.UID]client.Object),
		ownership:  make(map[types.UID]map[types.UID]bool),
		parentship: make(map[types.UID]types.UID),
	}
}

// Add a object to the object tree.
func (od ObjectTree) Add(parent, obj client.Object, opts ...AddObjectOption) (added bool, visible bool) {
	if parent == nil || obj == nil {
		return false, false
	}
	addOpts := &addObjectOptions{}
	addOpts.ApplyOptions(opts)

	// Get a small set of conditions that will be used to determine e.g. when grouping or when an object is just an echo of
	// its parent.
	var objReady, parentReady *clusterv1.Condition
	var objAvailableV1Beta2, objReadyV1Beta2, objUpToDateV1Beta2, parentReadyV1Beta2 *metav1.Condition
	switch od.options.V1Beta2 {
	case true:
		objAvailableV1Beta2 = GetAvailableV1Beta2Condition(obj)
		objReadyV1Beta2 = GetReadyV1Beta2Condition(obj)
		objUpToDateV1Beta2 = GetMachineUpToDateV1Beta2Condition(obj)
		parentReadyV1Beta2 = GetReadyV1Beta2Condition(parent)
	default:
		objReady = GetReadyCondition(obj)
		parentReady = GetReadyCondition(parent)
	}

	// If it is requested to show all the conditions for the object, add
	// the ShowObjectConditionsAnnotation to signal this to the presentation layer.
	if isObjDebug(obj, od.options.ShowOtherConditions) {
		addAnnotation(obj, ShowObjectConditionsAnnotation, "True")
	}

	// If echo should be dropped from the ObjectTree, return if the object's ready condition is true, and it is the same it has of parent's object ready condition (it is an echo).
	// Note: the Echo option applies only for infrastructure machine or bootstrap config objects, and for those objects only Ready condition makes sense.
	if addOpts.NoEcho && !od.options.Echo {
		switch od.options.V1Beta2 {
		case true:
			if (objReadyV1Beta2 != nil && objReadyV1Beta2.Status == metav1.ConditionTrue) || hasSameAvailableReadyUptoDateStatusAndReason(nil, nil, parentReadyV1Beta2, objReadyV1Beta2, nil, nil) {
				return false, false
			}
		default:
			if (objReady != nil && objReady.Status == corev1.ConditionTrue) || hasSameReadyStatusSeverityAndReason(parentReady, objReady) {
				return false, false
			}
		}
	}

	// If it is requested to use a meta name for the object in the presentation layer, add
	// the ObjectMetaNameAnnotation to signal this to the presentation layer.
	if addOpts.MetaName != "" {
		addAnnotation(obj, ObjectMetaNameAnnotation, addOpts.MetaName)
	}

	// Add the ObjectZOrderAnnotation to signal this to the presentation layer.
	addAnnotation(obj, ObjectZOrderAnnotation, strconv.Itoa(addOpts.ZOrder))

	// If it is requested that this object and its sibling should be grouped in case the ready condition
	// has the same Status, Severity and Reason, process all the sibling nodes.
	if IsGroupingObject(parent) {
		siblings := od.GetObjectsByParent(parent.GetUID())

		// The loop below will process the next node and decide if it belongs in a group. Since objects in the same group
		// must have the same Kind, we sort by Kind so objects of the same Kind will be together in the list.
		sort.Slice(siblings, func(i, j int) bool {
			return siblings[i].GetObjectKind().GroupVersionKind().Kind < siblings[j].GetObjectKind().GroupVersionKind().Kind
		})

		for i := range siblings {
			s := siblings[i]

			var sReady *clusterv1.Condition
			var sAvailableV1Beta2, sReadyV1Beta2, sUpToDateV1Beta2 *metav1.Condition
			switch od.options.V1Beta2 {
			case true:
				// If the object's ready condition has a different Available/ReadyUpToDate condition than the sibling object,
				// move on (they should not be grouped).
				sAvailableV1Beta2 = GetAvailableV1Beta2Condition(s)
				sReadyV1Beta2 = GetReadyV1Beta2Condition(s)
				sUpToDateV1Beta2 = GetMachineUpToDateV1Beta2Condition(s)
				if !hasSameAvailableReadyUptoDateStatusAndReason(objAvailableV1Beta2, sAvailableV1Beta2, objReadyV1Beta2, sReadyV1Beta2, objUpToDateV1Beta2, sUpToDateV1Beta2) {
					continue
				}
			default:
				sReady = GetReadyCondition(s)

				// If the object's ready condition has a different Status, Severity and Reason than the sibling object,
				// move on (they should not be grouped).
				if !hasSameReadyStatusSeverityAndReason(objReady, sReady) {
					continue
				}
			}

			// If the sibling node is already a group object
			if IsGroupObject(s) {
				// Check to see if the group object kind matches the object, i.e. group is MachineGroup and object is Machine.
				// If so, upgrade it with the current object.
				if s.GetObjectKind().GroupVersionKind().Kind == obj.GetObjectKind().GroupVersionKind().Kind+"Group" {
					switch od.options.V1Beta2 {
					case true:
						updateV1Beta2GroupNode(s, sReadyV1Beta2, obj, objAvailableV1Beta2, objReadyV1Beta2, objUpToDateV1Beta2)
					default:
						updateGroupNode(s, sReady, obj, objReady)
					}

					return true, false
				}
			} else if s.GetObjectKind().GroupVersionKind().Kind != obj.GetObjectKind().GroupVersionKind().Kind {
				// If the sibling is not a group object, check if the sibling and the object are of the same kind. If not, move on.
				continue
			}

			// Otherwise the object and the current sibling should be merged in a group.

			// Create virtual object for the group and add it to the object tree.
			var groupNode *NodeObject
			switch od.options.V1Beta2 {
			case true:
				groupNode = createV1Beta2GroupNode(s, sReadyV1Beta2, obj, objAvailableV1Beta2, objReadyV1Beta2, objUpToDateV1Beta2)
			default:
				groupNode = createGroupNode(s, sReady, obj, objReady)
			}

			// By default, grouping objects should be sorted last.
			addAnnotation(groupNode, ObjectZOrderAnnotation, strconv.Itoa(GetZOrder(obj)))

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
	delete(od.parentship, s.GetUID())
}

func (od ObjectTree) addInner(parent client.Object, obj client.Object) {
	od.items[obj.GetUID()] = obj
	if od.ownership[parent.GetUID()] == nil {
		od.ownership[parent.GetUID()] = make(map[types.UID]bool)
	}
	od.ownership[parent.GetUID()][obj.GetUID()] = true
	od.parentship[obj.GetUID()] = parent.GetUID()
}

// GetParent returns parent of an object.
func (od ObjectTree) GetParent(id types.UID) client.Object {
	parentID, ok := od.parentship[id]
	if !ok {
		return nil
	}
	if parentID == od.root.GetUID() {
		return od.root
	}
	return od.items[parentID]
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

func hasSameAvailableReadyUptoDateStatusAndReason(availableA, availableB, readyA, readyB, upToDateA, upToDateB *metav1.Condition) bool {
	if !hasSameStatusAndReason(availableA, availableB) {
		return false
	}
	if !hasSameStatusAndReason(readyA, readyB) {
		return false
	}
	if !hasSameStatusAndReason(upToDateA, upToDateB) {
		return false
	}
	return true
}

func hasSameStatusAndReason(a, b *metav1.Condition) bool {
	if ((a == nil) != (b == nil)) || ((a != nil && b != nil) && (a.Status != b.Status || a.Reason != b.Reason)) {
		return false
	}
	return true
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

func createV1Beta2GroupNode(sibling client.Object, siblingReady *metav1.Condition, obj client.Object, objAvailable, objReady, objUpToDate *metav1.Condition) *NodeObject {
	kind := fmt.Sprintf("%sGroup", obj.GetObjectKind().GroupVersionKind().Kind)

	// Create a new group node and add the GroupObjectAnnotation to signal
	// this to the presentation layer.
	// NB. The group nodes gets a unique ID to avoid conflicts.
	groupNode := VirtualObject(obj.GetNamespace(), kind, readyStatusReasonUIDV1Beta2(obj))
	addAnnotation(groupNode, GroupObjectAnnotation, "True")

	// Update the list of items included in the group and store it in the GroupItemsAnnotation.
	items := []string{obj.GetName(), sibling.GetName()}
	sort.Strings(items)
	addAnnotation(groupNode, GroupItemsAnnotation, strings.Join(items, GroupItemsSeparator))

	// Update the group's available condition and counter.
	addAnnotation(groupNode, GroupItemsAvailableCounter, "0")
	if objAvailable != nil {
		objAvailable.LastTransitionTime = metav1.Time{}
		objAvailable.Message = ""
		setAvailableV1Beta2Condition(groupNode, objAvailable)
		if objAvailable.Status == metav1.ConditionTrue {
			// When creating a group, it is already the sum of obj and its own sibling,
			// and they all have same conditions.
			addAnnotation(groupNode, GroupItemsAvailableCounter, "2")
		}
	}

	// Update the group's ready condition and counter.
	addAnnotation(groupNode, GroupItemsReadyCounter, "0")
	if objReady != nil {
		objReady.LastTransitionTime = minLastTransitionTimeV1Beta2(objReady, siblingReady)
		objReady.Message = ""
		setReadyV1Beta2Condition(groupNode, objReady)
		if objReady.Status == metav1.ConditionTrue {
			// When creating a group, it is already the sum of obj and its own sibling,
			// and they all have same conditions.
			addAnnotation(groupNode, GroupItemsReadyCounter, "2")
		}
	}

	// Update the group's upToDate condition and counter.
	addAnnotation(groupNode, GroupItemsUpToDateCounter, "0")
	if objUpToDate != nil {
		objUpToDate.LastTransitionTime = metav1.Time{}
		objUpToDate.Message = ""
		setUpToDateV1Beta2Condition(groupNode, objUpToDate)
		if objUpToDate.Status == metav1.ConditionTrue {
			// When creating a group, it is already the sum of obj and its own sibling,
			// and they all have same conditions.
			addAnnotation(groupNode, GroupItemsUpToDateCounter, "2")
		}
	}

	return groupNode
}

func readyStatusReasonUIDV1Beta2(obj client.Object) string {
	ready := GetReadyV1Beta2Condition(obj)
	if ready == nil {
		return fmt.Sprintf("zzz_%s", util.RandomString(6))
	}
	return fmt.Sprintf("zz_%s_%s_%s", ready.Status, ready.Reason, util.RandomString(6))
}

func minLastTransitionTimeV1Beta2(a, b *metav1.Condition) metav1.Time {
	if a == nil && b == nil {
		return metav1.Time{}
	}
	if (a != nil) && (b == nil) {
		return a.LastTransitionTime
	}
	if a == nil {
		return b.LastTransitionTime
	}
	if a.LastTransitionTime.Time.After(b.LastTransitionTime.Time) {
		return b.LastTransitionTime
	}
	return a.LastTransitionTime
}

func createGroupNode(sibling client.Object, siblingReady *clusterv1.Condition, obj client.Object, objReady *clusterv1.Condition) *NodeObject {
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
	if a == nil {
		return b.LastTransitionTime
	}
	if a.LastTransitionTime.Time.After(b.LastTransitionTime.Time) {
		return b.LastTransitionTime
	}
	return a.LastTransitionTime
}

func updateV1Beta2GroupNode(groupObj client.Object, groupReady *metav1.Condition, obj client.Object, objAvailable, objReady, objUpToDate *metav1.Condition) {
	// Update the list of items included in the group and store it in the GroupItemsAnnotation.
	items := strings.Split(GetGroupItems(groupObj), GroupItemsSeparator)
	items = append(items, obj.GetName())
	sort.Strings(items)
	addAnnotation(groupObj, GroupItemsAnnotation, strings.Join(items, GroupItemsSeparator))

	// Update the group's available counter.
	if objAvailable != nil {
		if objAvailable.Status == metav1.ConditionTrue {
			availableCounter := GetGroupItemsAvailableCounter(groupObj)
			addAnnotation(groupObj, GroupItemsAvailableCounter, fmt.Sprintf("%d", availableCounter+1))
		}
	}

	// Update the group's ready condition and ready counter.
	if groupReady != nil {
		groupReady.LastTransitionTime = minLastTransitionTimeV1Beta2(objReady, groupReady)
		groupReady.Message = ""
		setReadyV1Beta2Condition(groupObj, groupReady)
	}

	if objReady != nil && objReady.Status == metav1.ConditionTrue {
		readyCounter := GetGroupItemsReadyCounter(groupObj)
		addAnnotation(groupObj, GroupItemsReadyCounter, fmt.Sprintf("%d", readyCounter+1))
	}

	// Update the group's upToDate counter.
	if objUpToDate != nil {
		if objUpToDate.Status == metav1.ConditionTrue {
			upToDateCounter := GetGroupItemsUpToDateCounter(groupObj)
			addAnnotation(groupObj, GroupItemsUpToDateCounter, fmt.Sprintf("%d", upToDateCounter+1))
		}
	}
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
	for _, filter := range strings.Split(strings.ToLower(debugFilter), ",") {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		if strings.EqualFold(filter, "all") {
			return true
		}
		kn := strings.Split(filter, "/")
		if len(kn) == 2 {
			if strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind) == kn[0] && obj.GetName() == kn[1] {
				return true
			}
			continue
		}
		if strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind) == kn[0] {
			return true
		}
	}
	return false
}
