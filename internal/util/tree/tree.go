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

package tree

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gobuffalo/flect"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/tree"
	"sigs.k8s.io/cluster-api/internal/contract"
)

const (
	firstElemPrefix = `├─`
	lastElemPrefix  = `└─`
	indent          = "  "
	pipe            = `│ `

	// lastObjectAnnotation defines the last object in the ObjectTree.
	// This is necessary to built the prefix for multiline condition messages.
	lastObjectAnnotation = "tree.cluster.x-k8s.io.io/last-object"
)

var (
	gray   = color.New(color.FgHiBlack)
	red    = color.New(color.FgRed)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	white  = color.New(color.FgWhite)
	cyan   = color.New(color.FgCyan)
)

// PrintObjectTreeV1Beta2 prints the cluster status to stdout.
// Note: this function is exposed only for usage in clusterctl and Cluster API E2E tests.
func PrintObjectTreeV1Beta2(tree *tree.ObjectTree, w io.Writer) {
	// Creates the output table
	tbl := tablewriter.NewWriter(w)
	tbl.SetHeader([]string{"NAME", "REPLICAS", "AVAILABLE", "READY", "UP TO DATE", "STATUS", "REASON", "SINCE", "MESSAGE"})

	formatTableTreeV1Beta2(tbl)
	// Add row for the root object, the cluster, and recursively for all the nodes representing the cluster status.
	addObjectRowV1Beta2("", tbl, tree, tree.GetRoot())

	// Prints the output table
	tbl.Render()
}

// PrintObjectTree prints the cluster status to stdout.
// Note: this function is exposed only for usage in clusterctl and Cluster API E2E tests.
func PrintObjectTree(tree *tree.ObjectTree) {
	// Creates the output table
	tbl := tablewriter.NewWriter(os.Stdout)
	tbl.SetHeader([]string{"NAME", "READY", "SEVERITY", "REASON", "SINCE", "MESSAGE"})

	formatTableTree(tbl)
	// Add row for the root object, the cluster, and recursively for all the nodes representing the cluster status.
	addObjectRow("", tbl, tree, tree.GetRoot())

	// Prints the output table
	tbl.Render()
}

// formats the table with required attributes.
func formatTableTreeV1Beta2(tbl *tablewriter.Table) {
	tbl.SetAutoWrapText(false)
	tbl.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tbl.SetAlignment(tablewriter.ALIGN_LEFT)

	tbl.SetCenterSeparator("")
	tbl.SetRowSeparator("")

	tbl.SetHeaderLine(false)
	tbl.SetTablePadding("  ")
	tbl.SetNoWhiteSpace(true)
}

// formats the table with required attributes.
func formatTableTree(tbl *tablewriter.Table) {
	tbl.SetAutoWrapText(false)
	tbl.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tbl.SetAlignment(tablewriter.ALIGN_LEFT)

	tbl.SetCenterSeparator("")
	tbl.SetColumnSeparator("")
	tbl.SetRowSeparator("")

	tbl.SetHeaderLine(false)
	tbl.SetBorder(false)
	tbl.SetTablePadding("  ")
	tbl.SetNoWhiteSpace(true)
}

// addObjectRowV1Beta2 add a row for a given object, and recursively for all the object's children.
// NOTE: each row name gets a prefix, that generates a tree view like representation.
func addObjectRowV1Beta2(prefix string, tbl *tablewriter.Table, objectTree *tree.ObjectTree, obj ctrlclient.Object) {
	// Get a row descriptor for a given object.
	// With v1beta2, the return value of this func adapt to the object represented in the line.
	rowDescriptor := newV1beta2RowDescriptor(obj)

	// If the object is a group object, override the condition message with the list of objects in the group. e.g machine-1, machine-2, ...
	if tree.IsGroupObject(obj) {
		items := strings.Split(tree.GetGroupItems(obj), tree.GroupItemsSeparator)
		if len(items) <= 2 {
			rowDescriptor.message = gray.Sprintf("See %s", strings.Join(items, tree.GroupItemsSeparator))
		} else {
			rowDescriptor.message = gray.Sprintf("See %s, ...", strings.Join(items[:2], tree.GroupItemsSeparator))
		}
	}

	// Gets the row name for the object.
	// NOTE: The object name gets manipulated in order to improve readability.
	name := getRowName(obj)

	// If we are going to show all conditions from this object, let's drop the condition picked in the rowDescriptor.
	if tree.IsShowConditionsObject(obj) {
		rowDescriptor.status = ""
		rowDescriptor.reason = ""
		rowDescriptor.age = ""
		rowDescriptor.message = ""
	}

	// Add the row representing the object that includes
	// - The row name with the tree view prefix.
	// - Replica counters
	// - The object's Available, Ready, UpToDate conditions
	// - The condition picked in the rowDescriptor.
	// Note: if the condition has a multiline message, also add additional rows for each line.
	msg := strings.Split(rowDescriptor.message, "\n")
	msg0 := ""
	if len(msg) >= 1 {
		msg0 = msg[0]
	}
	tbl.Append([]string{
		fmt.Sprintf("%s%s", gray.Sprint(prefix), name),
		rowDescriptor.replicas,
		rowDescriptor.availableCounters,
		rowDescriptor.readyCounters,
		rowDescriptor.upToDateCounters,
		rowDescriptor.status,
		rowDescriptor.reason,
		rowDescriptor.age,
		msg0})

	multilinePrefix := getRootMultiLineObjectPrefix(obj, objectTree)
	for _, m := range msg[1:] {
		tbl.Append([]string{
			gray.Sprint(multilinePrefix),
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			m})
	}

	// If it is required to show all the conditions for the object, add a row for each object's conditions.
	if tree.IsShowConditionsObject(obj) {
		addOtherConditionsV1Beta2(prefix, tbl, objectTree, obj)
	}

	// Add a row for each object's children, taking care of updating the tree view prefix.
	childrenObj := objectTree.GetObjectsByParent(obj.GetUID())
	childrenObj = orderChildrenObjects(childrenObj)

	for i, child := range childrenObj {
		addObjectRowV1Beta2(getChildPrefix(prefix, i, len(childrenObj)), tbl, objectTree, child)
	}
}

func orderChildrenObjects(childrenObj []ctrlclient.Object) []ctrlclient.Object {
	// printBefore returns true if children[i] should be printed before children[j]. Objects are sorted by z-order and
	// row name such that objects with higher z-order are printed first, and objects with the same z-order are
	// printed in alphabetical order.
	printBefore := func(i, j int) bool {
		if tree.GetZOrder(childrenObj[i]) == tree.GetZOrder(childrenObj[j]) {
			return getRowName(childrenObj[i]) < getRowName(childrenObj[j])
		}

		return tree.GetZOrder(childrenObj[i]) > tree.GetZOrder(childrenObj[j])
	}
	sort.Slice(childrenObj, printBefore)
	return childrenObj
}

// addObjectRow add a row for a given object, and recursively for all the object's children.
// NOTE: each row name gets a prefix, that generates a tree view like representation.
func addObjectRow(prefix string, tbl *tablewriter.Table, objectTree *tree.ObjectTree, obj ctrlclient.Object) {
	// Gets the descriptor for the object's ready condition, if any.
	readyDescriptor := conditionDescriptor{readyColor: gray}
	if ready := tree.GetReadyCondition(obj); ready != nil {
		readyDescriptor = newConditionDescriptor(ready)
	}

	// If the object is a group object, override the condition message with the list of objects in the group. e.g machine-1, machine-2, ...
	if tree.IsGroupObject(obj) {
		items := strings.Split(tree.GetGroupItems(obj), tree.GroupItemsSeparator)
		if len(items) <= 2 {
			readyDescriptor.message = gray.Sprintf("See %s", strings.Join(items, tree.GroupItemsSeparator))
		} else {
			readyDescriptor.message = gray.Sprintf("See %s, ...", strings.Join(items[:2], tree.GroupItemsSeparator))
		}
	}

	// Gets the row name for the object.
	// NOTE: The object name gets manipulated in order to improve readability.
	name := getRowName(obj)

	// Add the row representing the object that includes
	// - The row name with the tree view prefix.
	// - The object's ready condition.
	tbl.Append([]string{
		fmt.Sprintf("%s%s", gray.Sprint(prefix), name),
		readyDescriptor.readyColor.Sprint(readyDescriptor.status),
		readyDescriptor.readyColor.Sprint(readyDescriptor.severity),
		readyDescriptor.readyColor.Sprint(readyDescriptor.reason),
		readyDescriptor.age,
		readyDescriptor.message})

	// If it is required to show all the conditions for the object, add a row for each object's conditions.
	if tree.IsShowConditionsObject(obj) {
		addOtherConditions(prefix, tbl, objectTree, obj)
	}

	// Add a row for each object's children, taking care of updating the tree view prefix.
	childrenObj := objectTree.GetObjectsByParent(obj.GetUID())

	// printBefore returns true if children[i] should be printed before children[j]. Objects are sorted by z-order and
	// row name such that objects with higher z-order are printed first, and objects with the same z-order are
	// printed in alphabetical order.
	printBefore := func(i, j int) bool {
		if tree.GetZOrder(childrenObj[i]) == tree.GetZOrder(childrenObj[j]) {
			return getRowName(childrenObj[i]) < getRowName(childrenObj[j])
		}

		return tree.GetZOrder(childrenObj[i]) > tree.GetZOrder(childrenObj[j])
	}
	sort.Slice(childrenObj, printBefore)

	for i, child := range childrenObj {
		addObjectRow(getChildPrefix(prefix, i, len(childrenObj)), tbl, objectTree, child)
	}
}

// addAllConditionsV1Beta2 adds a row for each object condition.
func addOtherConditionsV1Beta2(prefix string, tbl *tablewriter.Table, objectTree *tree.ObjectTree, obj ctrlclient.Object) {
	// Add a row for each other condition, taking care of updating the tree view prefix.
	// In this case the tree prefix get a filler, to indent conditions from objects, and eventually a
	// and additional pipe if the object has children that should be presented after the conditions.
	filler := strings.Repeat(" ", 10)
	childrenPipe := indent
	if objectTree.IsObjectWithChild(obj.GetUID()) {
		childrenPipe = pipe
	}

	negativePolarityConditions := sets.New(
		clusterv1.PausedV1Beta2Condition,
		clusterv1.DeletingV1Beta2Condition,
		clusterv1.RollingOutV1Beta2Condition,
		clusterv1.ScalingUpV1Beta2Condition,
		clusterv1.ScalingDownV1Beta2Condition,
		clusterv1.RemediatingV1Beta2Condition,
	)

	conditions := tree.GetAllV1Beta2Conditions(obj)
	for i := range conditions {
		condition := conditions[i]
		positivePolarity := true
		if negativePolarityConditions.Has(condition.Type) {
			positivePolarity = false
		}

		childPrefix := getChildPrefix(prefix+childrenPipe+filler, i, len(conditions))
		c, status, age, reason, message := v1Beta2ConditionInfo(condition, positivePolarity)

		// Add the row representing each condition.
		// Note: if the condition has a multiline message, also add additional rows for each line.
		msg := strings.Split(message, "\n")
		msg0 := ""
		if len(msg) >= 1 {
			msg0 = msg[0]
		}
		tbl.Append([]string{
			fmt.Sprintf("%s%s", gray.Sprint(childPrefix), cyan.Sprint(condition.Type)),
			"",
			"",
			"",
			"",
			c.Sprint(status),
			reason,
			age,
			msg0})

		for _, m := range msg[1:] {
			tbl.Append([]string{
				gray.Sprint(getMultilineConditionPrefix(childPrefix)),
				"",
				"",
				"",
				"",
				"",
				"",
				"",
				m})
		}
	}
}

// addOtherConditions adds a row for each object condition except the ready condition,
// which is already represented on the object's main row.
func addOtherConditions(prefix string, tbl *tablewriter.Table, objectTree *tree.ObjectTree, obj ctrlclient.Object) {
	// Add a row for each other condition, taking care of updating the tree view prefix.
	// In this case the tree prefix get a filler, to indent conditions from objects, and eventually a
	// and additional pipe if the object has children that should be presented after the conditions.
	filler := strings.Repeat(" ", 10)
	childrenPipe := indent
	if objectTree.IsObjectWithChild(obj.GetUID()) {
		childrenPipe = pipe
	}

	otherConditions := tree.GetOtherConditions(obj)
	for i := range otherConditions {
		otherCondition := otherConditions[i]
		otherDescriptor := newConditionDescriptor(otherCondition)
		otherConditionPrefix := getChildPrefix(prefix+childrenPipe+filler, i, len(otherConditions))
		tbl.Append([]string{
			fmt.Sprintf("%s%s", gray.Sprint(otherConditionPrefix), cyan.Sprint(otherCondition.Type)),
			otherDescriptor.readyColor.Sprint(otherDescriptor.status),
			otherDescriptor.readyColor.Sprint(otherDescriptor.severity),
			otherDescriptor.readyColor.Sprint(otherDescriptor.reason),
			otherDescriptor.age,
			otherDescriptor.message})
	}
}

// getChildPrefix return the tree view prefix for a row representing a child object.
func getChildPrefix(currentPrefix string, childIndex, childCount int) string {
	nextPrefix := currentPrefix

	// Alter the current prefix for hosting the next child object:

	// All ├─ should be replaced by |, so all the existing hierarchic dependencies are carried on
	nextPrefix = strings.ReplaceAll(nextPrefix, firstElemPrefix, pipe)
	// All └─ should be replaced by " " because we are under the last element of the tree (nothing to carry on)
	nextPrefix = strings.ReplaceAll(nextPrefix, lastElemPrefix, strings.Repeat(" ", len([]rune(lastElemPrefix))))

	// Add the prefix for the new child object (├─ for the firsts children, └─ for the last children).
	if childIndex < childCount-1 {
		return nextPrefix + firstElemPrefix
	}
	return nextPrefix + lastElemPrefix
}

// getMultilineConditionPrefix return the tree view prefix for a multiline condition.
func getMultilineConditionPrefix(currentPrefix string) string {
	// All ├─ should be replaced by |, so all the existing hierarchic dependencies are carried on
	if strings.HasSuffix(currentPrefix, firstElemPrefix) {
		return strings.TrimSuffix(currentPrefix, firstElemPrefix) + pipe
	}
	// All └─ should be replaced by " " because we are under the last element of the tree (nothing to carry on)
	if strings.HasSuffix(currentPrefix, lastElemPrefix) {
		return strings.TrimSuffix(currentPrefix, lastElemPrefix)
	}

	return "?"
}

// getRootMultiLineObjectPrefix generates the multiline prefix for an object.
func getRootMultiLineObjectPrefix(obj ctrlclient.Object, objectTree *tree.ObjectTree) string {
	// If the object is the last one in the tree, no prefix is needed.
	if ensureLastObjectInTree(objectTree) == string(obj.GetUID()) {
		return ""
	}

	// Determine the prefix for the current object.
	// If it is a leaf we don't have to add any prefix.
	var prefix string
	if len(objectTree.GetObjectsByParent(obj.GetUID())) > 0 {
		prefix = pipe
	}

	// Traverse upward through the tree, calculating each parent's prefix.
	// The parent of the root object is nil, so we stop when we reach that point.
	previousUID := obj.GetUID()
	parent := objectTree.GetParent(obj.GetUID())
	for parent != nil {
		// We have to break the loop if the previous ID is the same as the current ID.
		// This should never happen as the root object doesn't have set the parentship.
		if previousUID == parent.GetUID() {
			break
		}

		// Use pipe if the parent has children and the current node is not the last child.
		parentChildren := orderChildrenObjects(objectTree.GetObjectsByParent(parent.GetUID()))
		isLastChild := len(parentChildren) > 0 && parentChildren[len(parentChildren)-1].GetUID() == previousUID
		if objectTree.IsObjectWithChild(parent.GetUID()) && !isLastChild {
			prefix = pipe + prefix
		} else {
			prefix = indent + prefix
		}

		// Set prefix and move up the tree.
		previousUID = parent.GetUID()
		parent = objectTree.GetParent(parent.GetUID())
	}
	return prefix
}

func ensureLastObjectInTree(objectTree *tree.ObjectTree) string {
	// Compute last object in the tree and set it in the annotations.
	annotations := objectTree.GetRoot().GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	// Return if last object is already set.
	if val, ok := annotations[lastObjectAnnotation]; ok {
		return val
	}

	lastObjectInTree := string(getLastObjectInTree(objectTree).GetUID())
	annotations[lastObjectAnnotation] = lastObjectInTree
	objectTree.GetRoot().SetAnnotations(annotations)
	return lastObjectInTree
}

func getLastObjectInTree(objectTree *tree.ObjectTree) ctrlclient.Object {
	var objs []ctrlclient.Object

	var traverse func(obj ctrlclient.Object)
	traverse = func(obj ctrlclient.Object) {
		objs = append(objs, obj)
		children := orderChildrenObjects(objectTree.GetObjectsByParent(obj.GetUID()))
		for _, child := range children {
			traverse(child)
		}
	}

	traverse(objectTree.GetRoot())
	return objs[len(objs)-1]
}

// getRowName returns the object name in the tree view according to following rules:
// - group objects are represented as #of objects kind, e.g. 3 Machines...
// - other virtual objects are represented using the object name, e.g. Workers, or meta name if provided.
// - objects with a meta name are represented as meta name - (kind/name), e.g. ClusterInfrastructure - DockerCluster/test1
// - other objects are represented as kind/name, e.g.Machine/test1-md-0-779b87ff56-642vs
// - if the object is being deleted, a prefix will be added.
func getRowName(obj ctrlclient.Object) string {
	if tree.IsGroupObject(obj) {
		items := strings.Split(tree.GetGroupItems(obj), tree.GroupItemsSeparator)
		kind := flect.Pluralize(strings.TrimSuffix(obj.GetObjectKind().GroupVersionKind().Kind, "Group"))
		return white.Add(color.Bold).Sprintf("%d %s...", len(items), kind)
	}

	if tree.IsVirtualObject(obj) {
		if metaName := tree.GetMetaName(obj); metaName != "" {
			return metaName
		}
		return obj.GetName()
	}

	objName := fmt.Sprintf("%s/%s",
		obj.GetObjectKind().GroupVersionKind().Kind,
		color.New(color.Bold).Sprint(obj.GetName()))

	name := objName
	if objectPrefix := tree.GetMetaName(obj); objectPrefix != "" {
		name = fmt.Sprintf("%s - %s", objectPrefix, gray.Sprintf("%s", name))
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		name = fmt.Sprintf("%s %s", red.Sprintf("!! DELETED !!"), name)
	}

	return name
}

// v1beta2RowDescriptor contains all the info for representing a condition.
type v1beta2RowDescriptor struct {
	age               string
	replicas          string
	availableCounters string
	readyCounters     string
	upToDateCounters  string
	status            string
	reason            string
	message           string
}

// newV1beta2RowDescriptor returns a v1beta2ConditionDescriptor for the given condition.
// Note: the return value of this func adapt to the object represented in the line.
func newV1beta2RowDescriptor(obj ctrlclient.Object) v1beta2RowDescriptor {
	v := v1beta2RowDescriptor{}
	switch obj := obj.(type) {
	case *clusterv1.Cluster:
		// If the object is a cluster, returns all the replica counters (CP and worker replicas are summed for sake of simplicity);
		// also, pick the available condition as the condition to show for this object in case not all the conditions are visualized.
		if obj.Status.V1Beta2 != nil {
			cp := obj.Status.V1Beta2.ControlPlane
			if cp == nil {
				cp = &clusterv1.ClusterControlPlaneStatus{}
			}
			w := obj.Status.V1Beta2.Workers
			if w == nil {
				w = &clusterv1.WorkersStatus{}
			}
			if cp.DesiredReplicas != nil || w.DesiredReplicas != nil || cp.Replicas != nil || w.Replicas != nil {
				v.replicas = fmt.Sprintf("%d/%d", ptr.Deref(cp.Replicas, 0)+ptr.Deref(w.Replicas, 0), ptr.Deref(cp.DesiredReplicas, 0)+ptr.Deref(w.DesiredReplicas, 0))
			}
			if cp.AvailableReplicas != nil || w.AvailableReplicas != nil {
				v.availableCounters = fmt.Sprintf("%d", ptr.Deref(cp.AvailableReplicas, 0)+ptr.Deref(w.AvailableReplicas, 0))
			}
			if cp.ReadyReplicas != nil || w.ReadyReplicas != nil {
				v.readyCounters = fmt.Sprintf("%d", ptr.Deref(cp.ReadyReplicas, 0)+ptr.Deref(w.ReadyReplicas, 0))
			}
			if cp.UpToDateReplicas != nil || w.UpToDateReplicas != nil {
				v.upToDateCounters = fmt.Sprintf("%d", ptr.Deref(cp.UpToDateReplicas, 0)+ptr.Deref(w.UpToDateReplicas, 0))
			}
		}

		if available := tree.GetAvailableV1Beta2Condition(obj); available != nil {
			availableColor, availableStatus, availableAge, availableReason, availableMessage := v1Beta2ConditionInfo(*available, true)
			v.status = availableColor.Sprintf("Available: %s", availableStatus)
			v.reason = availableReason
			v.age = availableAge
			v.message = availableMessage
		}
	case *clusterv1.MachineDeployment:
		// If the object is a MachineDeployment, returns all the replica counters and pick the available condition
		// as the condition to show for this object in case not all the conditions are visualized.
		if obj.Spec.Replicas != nil {
			v.replicas = fmt.Sprintf("%d/%d", *obj.Spec.Replicas, obj.Status.Replicas)
		}
		if obj.Status.V1Beta2 != nil {
			if obj.Status.V1Beta2.ReadyReplicas != nil {
				v.availableCounters = fmt.Sprintf("%d", *obj.Status.V1Beta2.AvailableReplicas)
			}
			if obj.Status.V1Beta2.ReadyReplicas != nil {
				v.readyCounters = fmt.Sprintf("%d", *obj.Status.V1Beta2.ReadyReplicas)
			}
			if obj.Status.V1Beta2.UpToDateReplicas != nil {
				v.upToDateCounters = fmt.Sprintf("%d", *obj.Status.V1Beta2.UpToDateReplicas)
			}
		}

		if available := tree.GetAvailableV1Beta2Condition(obj); available != nil {
			availableColor, availableStatus, availableAge, availableReason, availableMessage := v1Beta2ConditionInfo(*available, true)
			v.status = availableColor.Sprintf("Available: %s", availableStatus)
			v.reason = availableReason
			v.age = availableAge
			v.message = availableMessage
		}

	case *clusterv1.MachineSet:
		// If the object is a MachineSet, returns all the replica counters and pick the available condition
		// as the condition to show for this object in case not all the conditions are visualized.
		if obj.Spec.Replicas != nil {
			v.replicas = fmt.Sprintf("%d/%d", *obj.Spec.Replicas, obj.Status.Replicas)
		}
		if obj.Status.V1Beta2 != nil {
			if obj.Status.V1Beta2.ReadyReplicas != nil {
				v.availableCounters = fmt.Sprintf("%d", *obj.Status.V1Beta2.AvailableReplicas)
			}
			if obj.Status.V1Beta2.ReadyReplicas != nil {
				v.readyCounters = fmt.Sprintf("%d", *obj.Status.V1Beta2.ReadyReplicas)
			}
			if obj.Status.V1Beta2.UpToDateReplicas != nil {
				v.upToDateCounters = fmt.Sprintf("%d", *obj.Status.V1Beta2.UpToDateReplicas)
			}
		}

	case *clusterv1.Machine:
		// If the object is a Machine, use Available, Ready and UpToDate conditions to infer replica counters;
		// additionally pick the ready condition as the condition to show for this object in case not all the conditions are visualized.
		v.replicas = "1"

		v.availableCounters = "0"
		if available := tree.GetAvailableV1Beta2Condition(obj); available != nil {
			if available.Status == metav1.ConditionTrue {
				v.availableCounters = "1"
			}
		}

		v.readyCounters = "0"
		if ready := tree.GetReadyV1Beta2Condition(obj); ready != nil {
			readyColor, readyStatus, readyAge, readyReason, readyMessage := v1Beta2ConditionInfo(*ready, true)
			v.status = readyColor.Sprintf("Ready: %s", readyStatus)
			v.reason = readyReason
			v.age = readyAge
			v.message = readyMessage
			if ready.Status == metav1.ConditionTrue {
				v.readyCounters = "1"
			}
		}

		v.upToDateCounters = "0"
		if upToDate := tree.GetMachineUpToDateV1Beta2Condition(obj); upToDate != nil {
			if upToDate.Status == metav1.ConditionTrue {
				v.upToDateCounters = "1"
			}
		}

	case *unstructured.Unstructured:
		// If the object is a Unstructured, pick the ready condition as the condition to show for this object
		// in case not all the conditions are visualized.
		// Also, if the Unstructured object implements the Cluster API control plane contract, surface
		// corresponding replica counters.
		if ready := tree.GetReadyV1Beta2Condition(obj); ready != nil {
			readyColor, readyStatus, readyAge, readyReason, readyMessage := v1Beta2ConditionInfo(*ready, true)
			v.status = readyColor.Sprintf("Ready: %s", readyStatus)
			v.reason = readyReason
			v.age = readyAge
			v.message = readyMessage
		}

		if tree.GetObjectContract(obj) == "ControlPlane" {
			if current, err := contract.ControlPlane().StatusReplicas().Get(obj); err == nil && current != nil {
				if desired, err := contract.ControlPlane().Replicas().Get(obj); err == nil && desired != nil {
					v.replicas = fmt.Sprintf("%d/%d", *current, *desired)
				}
			}

			if c, err := contract.ControlPlane().V1Beta2AvailableReplicas().Get(obj); err == nil && c != nil {
				v.availableCounters = fmt.Sprintf("%d", *c)
			}
			if c, err := contract.ControlPlane().V1Beta2ReadyReplicas().Get(obj); err == nil && c != nil {
				v.readyCounters = fmt.Sprintf("%d", *c)
			}
			if c, err := contract.ControlPlane().V1Beta2UpToDateReplicas().Get(obj); err == nil && c != nil {
				v.upToDateCounters = fmt.Sprintf("%d", *c)
			}
		}

	case *tree.NodeObject:
		// If the object represent a group of objects, surface the corresponding replica counters.
		// Also, pick the ready condition as the condition to show for this group.
		if tree.IsGroupObject(obj) {
			v.readyCounters = fmt.Sprintf("%d", tree.GetGroupItemsReadyCounter(obj))
			v.availableCounters = fmt.Sprintf("%d", tree.GetGroupItemsAvailableCounter(obj))
			v.upToDateCounters = fmt.Sprintf("%d", tree.GetGroupItemsUpToDateCounter(obj))
		}

		if ready := tree.GetReadyV1Beta2Condition(obj); ready != nil {
			readyColor, readyStatus, readyAge, readyReason, readyMessage := v1Beta2ConditionInfo(*ready, true)
			v.status = readyColor.Sprintf("Ready: %s", readyStatus)
			v.reason = readyReason
			v.age = readyAge
			v.message = readyMessage
		}
	}

	return v
}

func v1Beta2ConditionInfo(c metav1.Condition, positivePolarity bool) (color *color.Color, status, age, reason, message string) {
	switch c.Status {
	case metav1.ConditionFalse:
		if positivePolarity {
			color = red
		} else {
			color = green
		}
	case metav1.ConditionUnknown:
		color = yellow
	case metav1.ConditionTrue:
		if positivePolarity {
			color = green
		} else {
			color = red
		}
	default:
		color = gray
	}

	status = string(c.Status)
	reason = c.Reason
	age = duration.HumanDuration(time.Since(c.LastTransitionTime.Time))
	message = formatParagraph(c.Message, 100)

	return
}

var re = regexp.MustCompile(`[\s]+`)

// formatParagraph takes a strings and splits it into n lines of maxWidth length.
// If the string contains line breaks, those are preserved.
func formatParagraph(text string, maxWidth int) string {
	lines := []string{}
	for _, l := range strings.Split(text, "\n") {
		tmp := ""
		for _, c := range l {
			if c == ' ' {
				tmp += " "
				continue
			}
			break
		}
		indent := tmp
		if strings.HasPrefix(strings.TrimSpace(l), "* ") {
			indent += "  "
		}
		for _, w := range re.Split(l, -1) {
			if len(tmp)+len(w) < maxWidth {
				if strings.TrimSpace(tmp) != "" {
					tmp += " "
				}
				tmp += w
				continue
			}
			lines = append(lines, tmp)
			tmp = indent + w
		}
		lines = append(lines, tmp)
	}
	return strings.Join(lines, "\n")
}

// conditionDescriptor contains all the info for representing a condition.
type conditionDescriptor struct {
	readyColor *color.Color
	age        string
	status     string
	severity   string
	reason     string
	message    string
}

// newConditionDescriptor returns a conditionDescriptor for the given condition.
func newConditionDescriptor(c *clusterv1.Condition) conditionDescriptor {
	v := conditionDescriptor{}

	v.status = string(c.Status)
	v.severity = string(c.Severity)
	v.reason = c.Reason
	v.message = c.Message

	// Eventually cut the message to keep the table dimension under control.
	if len(v.message) > 100 {
		v.message = fmt.Sprintf("%s ...", v.message[:100])
	}

	// Compute the condition age.
	v.age = duration.HumanDuration(time.Since(c.LastTransitionTime.Time))

	// Determine the color to be used for showing the conditions according to Status and Severity in case Status is false.
	switch c.Status {
	case corev1.ConditionTrue:
		v.readyColor = green
	case corev1.ConditionFalse, corev1.ConditionUnknown:
		switch c.Severity {
		case clusterv1.ConditionSeverityError:
			v.readyColor = red
		case clusterv1.ConditionSeverityWarning:
			v.readyColor = yellow
		default:
			v.readyColor = white
		}
	default:
		v.readyColor = gray
	}

	return v
}
