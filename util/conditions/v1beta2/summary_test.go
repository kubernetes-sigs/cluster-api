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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestSummary(t *testing.T) {
	toLowerMsg := func(in []string) (out []string) {
		out = make([]string, len(in))
		for i := range in {
			out[i] = strings.ToLower(in[i])
		}
		return
	}
	tests := []struct {
		name          string
		conditions    []metav1.Condition
		conditionType string
		options       []SummaryOption
		want          *metav1.Condition
		wantErr       bool
	}{
		{
			name: "One issue",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},    // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},    // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C", Message: "Message-!C"}, // issue
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse, // False because there is one issue
				Reason:  issuesReportedReason,  // Using a generic reason
				Message: "* !C: Message-!C",    // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "One issue without message",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B"},   // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A"},   // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C"}, // issue
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse, // False because there is one issue
				Reason:  issuesReportedReason,  // Using a generic reason
				Message: "* !C: Reason-!C",     // messages from all the issues & unknown conditions (info dropped); since message is empty, a default one is added
			},
		},
		{
			name: "More than one issue",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionFalse, Reason: "Reason-B", Message: "Message-B"},   // issue
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},    // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C", Message: "Message-!C"}, // issue
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there are many issues
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* B: Message-B\n" +
					"* !C: Message-!C", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "More than one issue, some with multiline messages",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionFalse, Reason: "Reason-B", Message: "Message-B"},                     // issue
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},                      // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C", Message: "* Message-!C1\n* Message-!C2"}, // issue
				{Type: "D", Status: metav1.ConditionFalse, Reason: "Reason-D", Message: "Message-D\n* More message-D"},   // issue
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C", "D"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there are many issues
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* B: Message-B\n" +
					"* !C:\n" +
					"  * Message-!C1\n" +
					"  * Message-!C2\n" +
					"* D:\n" +
					"  * Message-D\n" +
					"    * More message-D", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "More than one issue and one unknown condition",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionFalse, Reason: "Reason-B", Message: "Message-B"},   // issue
				{Type: "A", Status: metav1.ConditionUnknown, Reason: "Reason-A", Message: "Message-A"}, // unknown
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-!C", Message: "Message-!C"}, // issue
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there are many issues
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* A: Message-A\n" +
					"* B: Message-B\n" +
					"* !C: Message-!C", // messages from all the issues & unknown conditions (info dropped); also, the order defined in ForConditionTypes must be preserved.
			},
		},
		{
			name: "One unknown (no issues)",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},       // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},       // info
				{Type: "!C", Status: metav1.ConditionUnknown, Reason: "Reason-!C", Message: "Message-!C"}, // unknown
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown, // Unknown because there is one unknown
				Reason:  unknownReportedReason,   // Using a generic reason
				Message: "* !C: Message-!C",      // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "More than one unknown (no issues)",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionUnknown, Reason: "Reason-B", Message: "Message-B"},    // unknown
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},       // info
				{Type: "!C", Status: metav1.ConditionUnknown, Reason: "Reason-!C", Message: "Message-!C"}, // unknown
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionUnknown, // Unknown because there are many unknown
				Reason: unknownReportedReason,   // Using a generic reason
				Message: "* B: Message-B\n" +
					"* !C: Message-!C", // messages from all the issues & unknown conditions (info dropped)
			},
		},

		{
			name: "More than one info (no issues, no unknown)",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},     // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: ""},              // info
				{Type: "!C", Status: metav1.ConditionFalse, Reason: "Reason-!C", Message: "Message-!C"}, // info
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options: []SummaryOption{
				ForConditionTypes{"A", "B", "!C"},
				NegativePolarityConditionTypes{"!C"},
				CustomMergeStrategy{
					DefaultMergeStrategy(
						TargetConditionHasPositivePolarity(true),
						GetPriorityFunc(GetDefaultMergePriorityFunc("!C")),
						SummaryMessageTransformFunc(toLowerMsg),
					),
				},
			},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionTrue, // True because there are many info
				Reason: infoReportedReason,   // Using a generic reason
				Message: "* b: message-b\n" +
					"* !c: message-!c", // messages from all the info conditions (empty messages are dropped), all lower case due to the summary message transform fun
			},
		},
		{
			name: "Default missing conditions to unknown",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// B and !C missing
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}}, // B and !C are required!
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionUnknown, // Unknown because there more than one unknown
				Reason: unknownReportedReason,   // Using a generic reason
				Message: "* B: Condition not yet reported\n" +
					"* !C: Condition not yet reported", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "Default missing conditions to unknown consider IgnoreTypesIfMissing",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// B and !C missing
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}, IgnoreTypesIfMissing{"B"}}, // B and !C are required!
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,            // Unknown because there more than one unknown
				Reason:  unknownReportedReason,              // Using a generic reason
				Message: "* !C: Condition not yet reported", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "No issue considering IgnoreTypesIfMissing",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// B and !C missing
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B", "!C"}, NegativePolarityConditionTypes{"!C"}, IgnoreTypesIfMissing{"B", "!C"}}, // A is required!
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionTrue, // True because B and !C are ignored
				Reason:  infoReportedReason,   // Using a generic reason
				Message: "* A: Message-A",     // messages from A, the only existing info
			},
		},
		{
			name: "Ignore conditions not in scope",
			conditions: []metav1.Condition{
				{Type: "B", Status: metav1.ConditionTrue, Reason: "Reason-B", Message: "Message-B"},       // info
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: ""},                // info
				{Type: "!C", Status: metav1.ConditionUnknown, Reason: "Reason-!C", Message: "Message-!C"}, // unknown
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{ForConditionTypes{"A", "B"}}, // C not in scope
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionTrue, // True because there are many info
				Reason:  infoReportedReason,   // Using a generic reason
				Message: "* B: Message-B",     // messages from all the info conditions (empty messages are dropped)
			},
		},
		{
			name: "Override condition",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},  // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-C", Message: "Message-C"}, // issue
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options: []SummaryOption{
				ForConditionTypes{"A", "!C"},
				NegativePolarityConditionTypes{"!C"},
				IgnoreTypesIfMissing{"!C"},
				OverrideConditions{
					{
						OwnerResource: ConditionOwnerInfo{
							Kind: "Phase3Obj",
							Name: "SourceObject",
						},
						Condition: metav1.Condition{
							Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-C-additional", Message: "Message-C-additional", // issue
						},
					},
				},
			}, // OverrideCondition replaces the same condition from the SourceObject
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,        // False because !C is an issue
				Reason:  issuesReportedReason,         // Using a generic reason
				Message: "* !C: Message-C-additional", // Picking the message from the additional condition (info dropped)
			},
		},
		{
			name:          "Error if ForConditionTypes is not set",
			conditions:    []metav1.Condition{},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []SummaryOption{},
			wantErr:       true,
		},
		{
			name:          "Error if ForConditionTypes includes target condition",
			conditions:    []metav1.Condition{},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options: []SummaryOption{
				ForConditionTypes{clusterv1.AvailableV1Beta2Condition},
			},
			wantErr: true,
		},
		{
			name: "Error if the same override condition is specified multiple times",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"},  // info
				{Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-C", Message: "Message-C"}, // issue
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options: []SummaryOption{
				ForConditionTypes{"A", "!C"},
				NegativePolarityConditionTypes{"!C"},
				IgnoreTypesIfMissing{"!C"},
				OverrideConditions{
					{
						OwnerResource: ConditionOwnerInfo{
							Kind: "Phase3Obj",
							Name: "SourceObject",
						},
						Condition: metav1.Condition{
							Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-C-additional", Message: "Message-C-additional", // issue
						},
					},
					{
						OwnerResource: ConditionOwnerInfo{
							Kind: "Phase3Obj",
							Name: "SourceObject",
						},
						Condition: metav1.Condition{
							Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-C-additional", Message: "Message-C-additional", // issue
						},
					},
				},
			}, // OverrideCondition is specified multiple times
			wantErr: true,
		},
		{
			name: "Error if override condition does not exist in source object",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: "Reason-A", Message: "Message-A"}, // info
				// !C is missing in source object
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options: []SummaryOption{
				ForConditionTypes{"A", "!C"},
				NegativePolarityConditionTypes{"!C"},
				IgnoreTypesIfMissing{"!C"},
				OverrideConditions{
					{
						OwnerResource: ConditionOwnerInfo{
							Kind: "Phase3Obj",
							Name: "SourceObject",
						},
						Condition: metav1.Condition{
							Type: "!C", Status: metav1.ConditionTrue, Reason: "Reason-C-additional", Message: "Message-C-additional", // issue
						},
					},
				},
			},
			wantErr: true,
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
			g.Expect(err != nil).To(Equal(tt.wantErr))

			g.Expect(got).To(Equal(tt.want))
		})
	}

	t.Run("Fails if conditions type is not provided", func(t *testing.T) {
		g := NewWithT(t)
		obj := &builder.Phase3Obj{}
		_, err := NewSummaryCondition(obj, clusterv1.AvailableV1Beta2Condition) // no ForConditionTypes --> Condition in scope will be empty
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("Fails if conditions in scope are empty", func(t *testing.T) {
		g := NewWithT(t)
		obj := &builder.Phase3Obj{}
		_, err := NewSummaryCondition(obj, clusterv1.AvailableV1Beta2Condition, ForConditionTypes{"A"}, IgnoreTypesIfMissing{"A"}) // no condition for the object, missing condition ignored --> Condition in scope will be empty
		g.Expect(err).To(HaveOccurred())
	})
}
