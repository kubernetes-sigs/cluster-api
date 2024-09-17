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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestSummary(t *testing.T) {
	tests := []struct {
		name          string
		conditions    []metav1.Condition
		conditionType string
		options       []SummaryOption
		want          *metav1.Condition
	}{
		{
			name: "One issue",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},    // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},    // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C", Message: "Message-!C"}, // issue
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse, // False because there is one issue
				Reason:  "Reason-!C",           // Picking the reason from the only existing issue
				Message: "!C: Message-!C",      // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "One issue without message",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B"},   // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A"},   // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C"}, // issue
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,             // False because there is one issue
				Reason:  "Reason-!C",                       // Picking the reason from the only existing issue
				Message: "!C: No additional info provided", // messages from all the issues & unknown conditions (info dropped); since message is empty, a default one is added
			},
		},
		{
			name: "More than one issue",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionFalse, Reason: "Reason-B", Message: "Message-B"},   // issue
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},    // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C", Message: "Message-!C"}, // issue
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,          // False because there are many issues
				Reason:  MultipleIssuesReportedReason,   // Using a generic reason
				Message: "B: Message-B; !C: Message-!C", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "More than one issue and one unknown condition",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionFalse, Reason: "Reason-B", Message: "Message-B"},   // issue
				{Type: "A", Status: metav1.ConditionUnknown, Reason: "Reason-A", Message: "Message-A"}, // unknown
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C", Message: "Message-!C"}, // issue
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                        // False because there are many issues
				Reason:  MultipleIssuesReportedReason,                 // Using a generic reason
				Message: "B: Message-B; !C: Message-!C; A: Message-A", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "One unknown (no issues)",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},       // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},       // info
				{Type: "!C", Status: metav1.ConditionUnknown, Reason: "Reason-!C", Message: "Message-!C"}, // unknown
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown, // Unknown because there is one unknown
				Reason:  "Reason-!C",             // Picking the reason from the only existing unknown
				Message: "!C: Message-!C",        // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "More than one unknown (no issues)",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionUnknown, Reason: "Reason-B", Message: "Message-B"},    // unknown
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},       // info
				{Type: "!C", Status: metav1.ConditionUnknown, Reason: "Reason-!C", Message: "Message-!C"}, // unknown
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,        // Unknown because there are many unknown
				Reason:  MultipleUnknownReportedReason,  // Using a generic reason
				Message: "B: Message-B; !C: Message-!C", // messages from all the issues & unknown conditions (info dropped)
			},
		},

		{
			name: "More than one info (no issues, no unknown)",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},     // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: ""},              // info
				{Type: "!C", Status: metav1.ConditionFalse, Reason: "Reason-!C", Message: "Message-!C"}, // info
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}, CustomMergeStrategy{newDefaultMergeStrategy()}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionTrue,           // True because there are many info
				Reason:  MultipleInfoReportedReason,     // Using a generic reason
				Message: "B: Message-B; !C: Message-!C", // messages from all the info conditions (empty messages are dropped)
			},
		},
		{
			name: "Default missing conditions to unknown",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// B and !C missing
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}}, // B and !C are required!
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,                                              // Unknown because there more than one unknown
				Reason:  MultipleUnknownReportedReason,                                        // Using a generic reason
				Message: "B: Condition B not yet reported; !C: Condition !C not yet reported", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "Default missing conditions to unknown consider IgnoreTypesIfMissing",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// B and !C missing
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}, IgnoreTypesIfMissing{"B"}}, // B and !C are required!
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,             // Unknown because there more than one unknown
				Reason:  NotYetReportedReason,                // Picking the reason from the only existing issue, which is a default missing condition added for !C
				Message: "!C: Condition !C not yet reported", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "No issue considering IgnoreTypesIfMissing",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// B and !C missing
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}, IgnoreTypesIfMissing{"B", "!C"}}, // A is required!
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionTrue, // True because B and !C are ignored
				Reason:  "Reason-A",           // Picking the reason from A, the only existing info
				Message: "A: Message-A",       // messages from A, the only existing info
			},
		},
		{
			name: "Ignore conditions not in scope",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},       // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: ""},                // info
				{Type: "!C", Status: metav1.ConditionUnknown, Reason: "Reason-!C", Message: "Message-!C"}, // unknown
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B"}}, // C not in scope
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionTrue,       // True because there are many info
				Reason:  MultipleInfoReportedReason, // Using a generic reason
				Message: "B: Message-B",             // messages from all the info conditions (empty messages are dropped)
			},
		},
		{
			name: "With stepCounter",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},       // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: ""},                // info
				{Type: "!C", Status: metav1.ConditionUnknown, Reason: "Reason-!C", Message: "Message-!C"}, // unknown
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}, StepCounter(true)},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,            // Unknown because there is one unknown
				Reason:  "Reason-!C",                        // Picking the reason from the only existing unknown
				Message: "2 of 3 completed; !C: Message-!C", // step counter + messages from all the issues & unknown conditions (info dropped)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obj := &builder.Phase3Obj{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      "SourceObject",
				},
				Status: builder.Phase3ObjStatus{
					Conditions: tt.conditions,
				},
			}

			got, err := NewSummaryCondition(obj, tt.conditionType, tt.options...)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}

	t.Run("Fails if conditions type is not provided", func(t *testing.T) {
		g := NewWithT(t)
		obj := &builder.Phase3Obj{}
		_, err := NewSummaryCondition(obj, AvailableCondition) // no ForConditionTypes --> Condition in scope will be empty
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("Fails if conditions in scope are empty", func(t *testing.T) {
		g := NewWithT(t)
		obj := &builder.Phase3Obj{}
		_, err := NewSummaryCondition(obj, AvailableCondition, ForConditionTypes{"A"}, IgnoreTypesIfMissing{"A"}) // no condition for the object, missing condition ignored --> Condition in scope will be empty
		g.Expect(err).To(HaveOccurred())
	})
}
