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

package experimental

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,   // False because there is one issue
				Reason:  "Reason-!C",             // Picking the reason from the only existing issue
				Message: "!C (True): Message-!C", // messages from all the issues & unknown conditions (info dropped)
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
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                         // False because there are many issues
				Reason:  ManyIssuesReason,                              // Using a generic reason
				Message: "B (False): Message-B; !C (True): Message-!C", // messages from all the issues & unknown conditions (info dropped)
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
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                                                 // False because there are many issues
				Reason:  ManyIssuesReason,                                                      // Using a generic reason
				Message: "B (False): Message-B; !C (True): Message-!C; A (Unknown): Message-A", // messages from all the issues & unknown conditions (info dropped)
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
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,    // Unknown because there is one unknown
				Reason:  "Reason-!C",                // Picking the reason from the only existing unknown
				Message: "!C (Unknown): Message-!C", // messages from all the issues & unknown conditions (info dropped)
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
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,                            // Unknown because there are many unknown
				Reason:  ManyUnknownsReason,                                 // Using a generic reason
				Message: "B (Unknown): Message-B; !C (Unknown): Message-!C", // messages from all the issues & unknown conditions (info dropped)
			},
		},

		// TODO: Think about how to test One info (no issues, no unknown): If I use only one ConditionTypes is considered Aggregate, If I use more, it is not One info (no issues, no unknown)

		{
			name: "More than one info (no issues, no unknown)",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},     // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: ""},              // info
				{Type: "!C", Status: metav1.ConditionFalse, Reason: "Reason-!C", Message: "Message-!C"}, // info
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}, WithMergeStrategy{newDefaultMergeStrategy()}},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionTrue,                          // True because there are many info
				Reason:  ManyInfoReason,                                // Using a generic reason
				Message: "B (True): Message-B; !C (False): Message-!C", // messages from all the info conditions (empty messages are dropped)
			},
		},
		{
			name: "Default missing conditions to unknown",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// B and !C missing
			},
			conditionType: AvailableCondition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}}, // B and !C are required!
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,                                                                  // Unknown because there more than one unknown
				Reason:  ManyUnknownsReason,                                                                       // Picking the reason from the only existing issue
				Message: "B (Unknown): Condition B not yet reported; !C (Unknown): Condition !C not yet reported", // messages from all the issues & unknown conditions (info dropped)
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
				Status:  metav1.ConditionTrue,  // True because there are many info
				Reason:  ManyInfoReason,        // Using a generic reason
				Message: "B (True): Message-B", // messages from all the info conditions (empty messages are dropped)
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
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, WithNegativeConditionTypes{"!C"}, WithStepCounter(true)},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,                      // Unknown because there is one unknown
				Reason:  "Reason-!C",                                  // Picking the reason from the only existing unknown
				Message: "2 of 3 completed; !C (Unknown): Message-!C", // step counter + messages from all the issues & unknown conditions (info dropped)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obj := &V1Beta2ResourceWithConditions{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      "SourceObject",
				},
				Status: struct{ Conditions []metav1.Condition }{
					Conditions: tt.conditions,
				},
			}

			got, err := NewSummaryCondition(obj, tt.conditionType, tt.options...)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
