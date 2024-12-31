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

package v1beta2

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func TestSortMessages(t *testing.T) {
	g := NewWithT(t)
	messageMustGoFirst := map[string]bool{"m1": true}
	messagePriorityMap := map[string]MergePriority{"m1": InfoMergePriority, "m2": IssueMergePriority, "m3": UnknownMergePriority, "m4": InfoMergePriority, "m5": UnknownMergePriority, "m6": UnknownMergePriority, "m7": UnknownMergePriority}
	messageObjMapForKind := map[string][]string{"m1": {}, "m2": {"foo"}, "m3": {"foo", "bar"}, "m4": {"foo", "bar", "baz"}, "m5": {"a", "b"}, "m6": {"a"}, "m7": {"b"}}

	// Control plane goes before not control planes
	got := sortMessage("m1", "m2", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeTrue())
	got = sortMessage("m1", "m5", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeTrue())
	// Not control plane goes after control planes
	got = sortMessage("m2", "m1", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeFalse())
	got = sortMessage("m5", "m1", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeFalse())

	// issues goes before unknown
	got = sortMessage("m2", "m3", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeTrue())
	// unknown goes after issues
	got = sortMessage("m3", "m2", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeFalse())

	// unknown goes before info
	got = sortMessage("m3", "m4", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeTrue())
	// info goes after unknown
	got = sortMessage("m4", "m3", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeFalse())

	// 2 objects goes before 1 object
	got = sortMessage("m5", "m6", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeTrue())
	// 1 object goes after 2 objects
	got = sortMessage("m6", "m5", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeFalse())

	// 1 object "a" goes before 1 object "b"
	got = sortMessage("m6", "m7", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeTrue())
	// 1 object "b" goes after 1 object "a"
	got = sortMessage("m7", "m6", messageMustGoFirst, messagePriorityMap, messageObjMapForKind)
	g.Expect(got).To(BeFalse())
}

func TestSortObj(t *testing.T) {
	g := NewWithT(t)
	cpMachines := sets.Set[string]{}
	cpMachines.Insert("m3")

	// Control plane goes before not control planes
	got := sortObj("m3", "m1", cpMachines)
	g.Expect(got).To(BeTrue())
	got = sortObj("m1", "m3", cpMachines)
	g.Expect(got).To(BeFalse())

	// machines must be sorted alphabetically
	got = sortObj("m1", "m2", cpMachines)
	g.Expect(got).To(BeTrue())
	got = sortObj("m2", "m1", cpMachines)
	g.Expect(got).To(BeFalse())
}

func TestSummaryMessages(t *testing.T) {
	d := &defaultMergeStrategy{
		getPriorityFunc: GetDefaultMergePriorityFunc(),
	}
	t.Run("When status is not true, drop info messages", func(t *testing.T) {
		g := NewWithT(t)

		conditions := []ConditionWithOwnerInfo{
			// NOTE: objects are intentionally not in order so we can validate they are sorted by name
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "A", Reason: "Reason-A", Message: "Message-A", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "B", Reason: "Reason-B", Message: "Message-B", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "C", Reason: "Reason-C", Message: "Message-C", Status: metav1.ConditionTrue}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "D", Reason: "Reason-D", Status: metav1.ConditionFalse}},
		}

		message := summaryMessage(conditions, d, metav1.ConditionFalse)

		g.Expect(message).To(Equal("* A: Message-A\n" +
			"* B: Message-B\n" +
			// Info message of true condition C was dropped
			"* D: Reason-D")) // False conditions without messages must show the reason
	})
	t.Run("When status is true, surface only not empty messages", func(t *testing.T) {
		g := NewWithT(t)

		conditions := []ConditionWithOwnerInfo{
			// NOTE: objects are intentionally not in order so we can validate they are sorted by name
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "A", Reason: "Reason-A", Message: "Message-A", Status: metav1.ConditionTrue}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "B", Reason: "Reason-B", Message: "Message-B", Status: metav1.ConditionTrue}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "C", Reason: "Reason-C", Status: metav1.ConditionTrue}},
		}

		message := summaryMessage(conditions, d, metav1.ConditionTrue)

		g.Expect(message).To(Equal("* A: Message-A\n" +
			"* B: Message-B"))
	})
	t.Run("Handles multiline messages", func(t *testing.T) {
		g := NewWithT(t)

		conditions := []ConditionWithOwnerInfo{
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "A", Reason: "Reason-A", Message: "Message-A", Status: metav1.ConditionTrue}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "B", Reason: "Reason-B", Message: "* Message-B", Status: metav1.ConditionTrue}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "C", Reason: "Reason-C", Message: "* Message-C1\n* Message-C2", Status: metav1.ConditionTrue}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "D", Reason: "Reason-D", Message: "Message-D\n* More Message-D", Status: metav1.ConditionTrue}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "E", Reason: "Reason-E", Message: "* Message-E1\n  * More Message-E1\n* Message-E2", Status: metav1.ConditionTrue}},
		}

		message := summaryMessage(conditions, d, metav1.ConditionTrue)

		expected :=
			// not multiline messages stay on a single line.
			"* A: Message-A\n" +
				// single line messages but starting with a bullet gets nested.
				"* B:\n" +
				"  * Message-B\n" +
				// multiline messages gets nested.
				"* C:\n" +
				"  * Message-C1\n" +
				"  * Message-C2\n" +
				// multiline messages with some lines without bullets gets a bulled and nested.
				"* D:\n" +
				"  * Message-D\n" +
				"    * More Message-D\n" +
				// nesting in multiline messages is preserved.
				"* E:\n" +
				"  * Message-E1\n" +
				"    * More Message-E1\n" +
				"  * Message-E2"
		g.Expect(message).To(Equal(expected))
	})
}

func TestAggregateMessages(t *testing.T) {
	t.Run("Groups by kind, return max 3 messages, aggregate objects, count others", func(t *testing.T) {
		g := NewWithT(t)

		conditions := []ConditionWithOwnerInfo{
			// NOTE: objects are intentionally not in order so we can validate they are sorted by name
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj02"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj04"}, Condition: metav1.Condition{Type: "A", Message: "* Message-2", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj03"}, Condition: metav1.Condition{Type: "A", Message: "* Message-2", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj06"}, Condition: metav1.Condition{Type: "A", Message: "* Message-3A\n* Message-3B", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj05"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj08"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj07"}, Condition: metav1.Condition{Type: "A", Message: "Message-4", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj09"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj10"}, Condition: metav1.Condition{Type: "A", Message: "Message-5", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineSet", Name: "obj11"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineSet", Name: "obj12"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionFalse}},
		}

		n := 3
		messages := aggregateMessages(conditions, &n, false, GetDefaultMergePriorityFunc(), map[MergePriority]string{IssueMergePriority: "with other issues"})

		g.Expect(n).To(Equal(0))
		g.Expect(messages).To(Equal([]string{
			"* MachineDeployments obj01, obj02, obj05, ... (2 more): Message-1", // MachineDeployments obj08, obj09
			"* MachineDeployments obj03, obj04:\n" +
				"  * Message-2",
			"* MachineDeployment obj06:\n" +
				"  * Message-3A\n" +
				"  * Message-3B",
			"And 2 MachineDeployments with other issues", // MachineDeployments  obj07 (Message-4), obj10 (Message-5)
			"And 2 MachineSets with other issues",        // MachineSet obj11, obj12 (Message-1)
		}))
	})
	t.Run("Issue messages goes before unknown", func(t *testing.T) {
		g := NewWithT(t)

		conditions := []ConditionWithOwnerInfo{
			// NOTE: objects are intentionally not in order so we can validate they are sorted by name
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj02"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionUnknown}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj01"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionUnknown}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj04"}, Condition: metav1.Condition{Type: "A", Message: "* Message-2", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj03"}, Condition: metav1.Condition{Type: "A", Message: "* Message-2", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj06"}, Condition: metav1.Condition{Type: "A", Message: "* Message-3A\n* Message-3B", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj05"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionUnknown}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj08"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionUnknown}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj07"}, Condition: metav1.Condition{Type: "A", Message: "Message-4\n* More Message-4", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj09"}, Condition: metav1.Condition{Type: "A", Message: "Message-1", Status: metav1.ConditionUnknown}},
			{OwnerResource: ConditionOwnerInfo{Kind: "MachineDeployment", Name: "obj10"}, Condition: metav1.Condition{Type: "A", Message: "Message-5", Status: metav1.ConditionFalse}},
		}

		n := 3
		messages := aggregateMessages(conditions, &n, false, GetDefaultMergePriorityFunc(), map[MergePriority]string{IssueMergePriority: "with other issues", UnknownMergePriority: "with status unknown"})

		g.Expect(n).To(Equal(0))
		g.Expect(messages).To(Equal([]string{
			"* MachineDeployments obj03, obj04:\n" +
				"  * Message-2",
			"* MachineDeployment obj06:\n" +
				"  * Message-3A\n" +
				"  * Message-3B",
			"* MachineDeployment obj07:\n" +
				"  * Message-4\n" +
				"    * More Message-4",
			"And 1 MachineDeployment with other issues",    // MachineDeployments obj10 (Message-5)
			"And 5 MachineDeployments with status unknown", // MachineDeployments obj01, obj02, obj05, obj08, obj09 (Message 1) << This doesn't show up because even if it applies to 5 machines because it has merge priority unknown
		}))
	})
	t.Run("Control plane machines always goes before other machines", func(t *testing.T) {
		g := NewWithT(t)

		conditions := []ConditionWithOwnerInfo{
			// NOTE: objects are intentionally not in order so we can validate they are sorted by name
			{OwnerResource: ConditionOwnerInfo{Kind: "Machine", Name: "obj02", IsControlPlaneMachine: true}, Condition: metav1.Condition{Type: "A", Message: "* Message-1", Status: metav1.ConditionUnknown}},
			{OwnerResource: ConditionOwnerInfo{Kind: "Machine", Name: "obj01"}, Condition: metav1.Condition{Type: "A", Message: "* Message-1", Status: metav1.ConditionUnknown}},
			{OwnerResource: ConditionOwnerInfo{Kind: "Machine", Name: "obj04"}, Condition: metav1.Condition{Type: "A", Message: "* Message-2", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "Machine", Name: "obj03"}, Condition: metav1.Condition{Type: "A", Message: "* Message-2", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "Machine", Name: "obj06"}, Condition: metav1.Condition{Type: "A", Message: "* Message-3A\n* Message-3B", Status: metav1.ConditionFalse}},
			{OwnerResource: ConditionOwnerInfo{Kind: "Machine", Name: "obj05"}, Condition: metav1.Condition{Type: "A", Message: "* Message-2", Status: metav1.ConditionFalse}},
		}

		n := 3
		messages := aggregateMessages(conditions, &n, false, GetDefaultMergePriorityFunc(), map[MergePriority]string{IssueMergePriority: "with other issues", UnknownMergePriority: "with status unknown"})

		g.Expect(n).To(Equal(0))
		g.Expect(messages).To(Equal([]string{
			"* Machines obj02, obj01:\n" + // control plane machines always go first, no matter if priority or number of objects (note, cp machine also go first in the machine list)
				"  * Message-1",
			"* Machines obj03, obj04, obj05:\n" +
				"  * Message-2",
			"* Machine obj06:\n" +
				"  * Message-3A\n" +
				"  * Message-3B",
		}))
	})
}

func TestSortConditions(t *testing.T) {
	g := NewWithT(t)

	t0 := metav1.Now()
	t1 := metav1.Time{Time: t0.Add(10 * time.Minute)}
	t2 := metav1.Time{Time: t0.Add(20 * time.Minute)}

	conditions := []ConditionWithOwnerInfo{
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionFalse, LastTransitionTime: t0}},
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionFalse, LastTransitionTime: t1}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionTrue, LastTransitionTime: t2}},
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionUnknown, LastTransitionTime: t0}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionTrue, LastTransitionTime: t2}},
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionUnknown, LastTransitionTime: t0}},
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionUnknown, LastTransitionTime: t2}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionTrue, LastTransitionTime: t1}},
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionFalse, LastTransitionTime: t1}},
	}

	orderedConditionTypes := []string{"A", "B", "!C"}
	sortConditions(conditions, orderedConditionTypes)

	// Check conditions are sorted by orderedConditionTypes and by LastTransitionTime

	g.Expect(conditions).To(Equal([]ConditionWithOwnerInfo{
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionUnknown, LastTransitionTime: t0}},
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionFalse, LastTransitionTime: t1}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionTrue, LastTransitionTime: t2}},
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionFalse, LastTransitionTime: t0}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionTrue, LastTransitionTime: t1}},
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionUnknown, LastTransitionTime: t2}},
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionUnknown, LastTransitionTime: t0}},
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionFalse, LastTransitionTime: t1}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionTrue, LastTransitionTime: t2}},
	}))
}

func TestSplitConditionsByPriority(t *testing.T) {
	g := NewWithT(t)

	conditions := []ConditionWithOwnerInfo{
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionFalse}},    // issue
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionTrue}},     // info
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionFalse}},    // issue
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionTrue}},    // issue
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionUnknown}},  // unknown
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionUnknown}}, // unknown
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionUnknown}},  // unknown
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionTrue}},     // info
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionFalse}},   // info
	}

	issueConditions, unknownConditions, infoConditions := splitConditionsByPriority(conditions, GetDefaultMergePriorityFunc("!C"))

	// Check condition are grouped as expected and order is preserved.

	g.Expect(issueConditions).To(Equal([]ConditionWithOwnerInfo{
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionFalse}},
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionFalse}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionTrue}},
	}))

	g.Expect(unknownConditions).To(Equal([]ConditionWithOwnerInfo{
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionUnknown}},
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionUnknown}},
		{OwnerResource: ConditionOwnerInfo{Name: "bar"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionUnknown}},
	}))

	g.Expect(infoConditions).To(Equal([]ConditionWithOwnerInfo{
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "A", Status: metav1.ConditionTrue}},
		{OwnerResource: ConditionOwnerInfo{Name: "foo"}, Condition: metav1.Condition{Type: "B", Status: metav1.ConditionTrue}},
		{OwnerResource: ConditionOwnerInfo{Name: "baz"}, Condition: metav1.Condition{Type: "!C", Status: metav1.ConditionFalse}},
	}))
}

func TestDefaultMergePriority(t *testing.T) {
	tests := []struct {
		name             string
		condition        metav1.Condition
		negativePolarity bool
		wantPriority     MergePriority
	}{
		{
			name:             "Issue (PositivePolarity)",
			condition:        metav1.Condition{Type: "foo", Status: metav1.ConditionFalse},
			negativePolarity: false,
			wantPriority:     IssueMergePriority,
		},
		{
			name:             "Unknown (PositivePolarity)",
			condition:        metav1.Condition{Type: "foo", Status: metav1.ConditionUnknown},
			negativePolarity: false,
			wantPriority:     UnknownMergePriority,
		},
		{
			name:             "Info (PositivePolarity)",
			condition:        metav1.Condition{Type: "foo", Status: metav1.ConditionTrue},
			negativePolarity: false,
			wantPriority:     InfoMergePriority,
		},
		{
			name:             "NoStatus (PositivePolarity)",
			condition:        metav1.Condition{Type: "foo"},
			negativePolarity: false,
			wantPriority:     UnknownMergePriority,
		},
		{
			name:             "Issue (NegativePolarity)",
			condition:        metav1.Condition{Type: "foo", Status: metav1.ConditionTrue},
			negativePolarity: true,
			wantPriority:     IssueMergePriority,
		},
		{
			name:             "Unknown (NegativePolarity)",
			condition:        metav1.Condition{Type: "foo", Status: metav1.ConditionUnknown},
			negativePolarity: true,
			wantPriority:     UnknownMergePriority,
		},
		{
			name:             "Info (NegativePolarity)",
			condition:        metav1.Condition{Type: "foo", Status: metav1.ConditionFalse},
			negativePolarity: true,
			wantPriority:     InfoMergePriority,
		},
		{
			name:             "NoStatus (NegativePolarity)",
			condition:        metav1.Condition{Type: "foo"},
			negativePolarity: true,
			wantPriority:     UnknownMergePriority,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			negativePolarityConditionTypes := []string{}
			if tt.negativePolarity {
				negativePolarityConditionTypes = append(negativePolarityConditionTypes, tt.condition.Type)
			}
			gotPriority := GetDefaultMergePriorityFunc(negativePolarityConditionTypes...)(tt.condition)

			g.Expect(gotPriority).To(Equal(tt.wantPriority))
		})
	}
}
