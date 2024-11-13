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

func TestAggregateMessages(t *testing.T) {
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
	messages := aggregateMessages(conditions, &n, false, "with other issues")

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

	issueConditions, unknownConditions, infoConditions := splitConditionsByPriority(conditions, GetDefaultMergePriorityFunc(sets.New[string]("!C")))

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

			negativePolarityConditionTypes := sets.New[string]()
			if tt.negativePolarity {
				negativePolarityConditionTypes.Insert(tt.condition.Type)
			}
			gotPriority := GetDefaultMergePriorityFunc(negativePolarityConditionTypes)(tt.condition)

			g.Expect(gotPriority).To(Equal(tt.wantPriority))
		})
	}
}
