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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestAggregate(t *testing.T) {
	tests := []struct {
		name          string
		conditions    [][]metav1.Condition
		conditionType string
		options       []AggregateOption
		want          *metav1.Condition
		wantErr       bool
	}{
		{
			name: "One issue",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj1
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,         // False because there is one issue
				Reason:  issuesReportedReason,          // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "One issue with negative polarity",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}},    // obj0
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-99", Message: "Message-99"}}, // obj1
			},
			conditionType: clusterv1.ScalingUpV1Beta2Condition,
			options:       []AggregateOption{NegativePolarityConditionTypes{clusterv1.ScalingUpV1Beta2Condition}},
			want: &metav1.Condition{
				Type:    clusterv1.ScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,          // True because there is one issue, and the target condition has negative polarity
				Reason:  issuesReportedReason,          // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name:          "Error if negative polarity conditions are misconfigured",
			conditions:    [][]metav1.Condition{},
			conditionType: clusterv1.ScalingUpV1Beta2Condition,
			options:       []AggregateOption{NegativePolarityConditionTypes{"foo"}}, // NegativePolarityConditionTypes if set must equal source condition
			want:          nil,
			wantErr:       true,
		},
		{
			name: "One issue with custom merge strategy",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}},    // obj0
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-99", Message: "Message-99"}}, // obj1
			},
			conditionType: clusterv1.ScalingUpV1Beta2Condition,
			options: []AggregateOption{NegativePolarityConditionTypes{clusterv1.ScalingUpV1Beta2Condition}, CustomMergeStrategy{
				MergeStrategy: DefaultMergeStrategy(
					TargetConditionHasPositivePolarity(true),
					GetPriorityFunc(GetDefaultMergePriorityFunc(clusterv1.ScalingUpV1Beta2Condition)),
					ComputeReasonFunc(GetDefaultComputeMergeReasonFunc(
						"bad",
						"unknown",
						"good",
					)),
				),
			}},
			want: &metav1.Condition{
				Type:    clusterv1.ScalingUpV1Beta2Condition,
				Status:  metav1.ConditionFalse,         // False because there is one issue, and the custom merge strategy doesn't set the flag that defines that the target condition has negative polarity
				Reason:  "bad",                         // Using reason from the ComputeReasonFunc
				Message: "* Phase3Obj obj0: Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "One issue with custom merge strategy (negative polarity)",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}},    // obj0
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-99", Message: "Message-99"}}, // obj1
			},
			conditionType: clusterv1.ScalingUpV1Beta2Condition,
			options: []AggregateOption{NegativePolarityConditionTypes{clusterv1.ScalingUpV1Beta2Condition}, CustomMergeStrategy{
				MergeStrategy: DefaultMergeStrategy(
					TargetConditionHasPositivePolarity(false),
					GetPriorityFunc(GetDefaultMergePriorityFunc(clusterv1.ScalingUpV1Beta2Condition)),
					ComputeReasonFunc(GetDefaultComputeMergeReasonFunc(
						"good", // Note: with negative polarity, false is good
						"unknown",
						"bad",
					)),
				),
			}},
			want: &metav1.Condition{
				Type:    clusterv1.ScalingUpV1Beta2Condition,
				Status:  metav1.ConditionTrue,          // True because there is one issue, and the custom merge strategy sets the flag that defines that the target condition has negative polarity
				Reason:  "good",                        // Using reason from the ComputeReasonFunc
				Message: "* Phase3Obj obj0: Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "One issue with target type",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj1
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{TargetConditionType("SomethingAvailable")},
			want: &metav1.Condition{
				Type:    "SomethingAvailable",
				Status:  metav1.ConditionFalse,         // False because there is one issue
				Reason:  issuesReportedReason,          // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "One issue with target type and negative polarity",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}},    // obj0
				{{Type: clusterv1.ScalingUpV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-99", Message: "Message-99"}}, // obj1
			},
			conditionType: clusterv1.ScalingUpV1Beta2Condition,
			options:       []AggregateOption{TargetConditionType("SomethingAvailable"), NegativePolarityConditionTypes{clusterv1.ScalingUpV1Beta2Condition}},
			want: &metav1.Condition{
				Type:    "SomethingAvailable",
				Status:  metav1.ConditionTrue,          // True because there is one issue, and the target condition has negative polarity
				Reason:  issuesReportedReason,          // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "Same issue from up to three objects",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj3
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,                      // False because there is one issue
				Reason:  issuesReportedReason,                       // Using a generic reason
				Message: "* Phase3Objs obj0, obj1, obj2: Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "Same issue from more than three objects",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-B", Message: "Message-1"}}, // obj4
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Message: "Message-99"}},                     // obj5
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    clusterv1.AvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,                                    // False because there is one issue
				Reason:  issuesReportedReason,                                     // Using a generic reason
				Message: "* Phase3Objs obj0, obj1, obj2, ... (2 more): Message-1", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "Up to three different issue messages",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},  // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},  // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj4
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-3", Message: "Message-3"}},  // obj5
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj6
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Objs obj0, obj3, obj4: Message-1\n" +
					"* Phase3Objs obj1, obj2: Message-2\n" +
					"* Phase3Obj obj5: Message-3", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "Up to three different issue messages; if message is a list, it should be indented",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "* Message-1"}},                // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "* Message-2A\n* Message-2B"}}, // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "* Message-2A\n* Message-2B"}}, // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "* Message-1"}},                // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "* Message-1"}},                // obj4
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-3", Message: "* Message-3"}},                // obj5
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},                 // obj6
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Objs obj0, obj3, obj4:\n" +
					"  * Message-1\n" +
					"* Phase3Objs obj1, obj2:\n" +
					"  * Message-2A\n" +
					"  * Message-2B\n" +
					"* Phase3Obj obj5:\n" +
					"  * Message-3", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "More than three different issue messages",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},  // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-4", Message: "Message-4"}},  // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-5", Message: "Message-5"}},  // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj4
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-3", Message: "Message-3"}},  // obj5
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj6
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Objs obj0, obj4: Message-1\n" +
					"* Phase3Obj obj1: Message-2\n" +
					"* Phase3Obj obj2: Message-4\n" +
					"And 2 Phase3Objs with other issues", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "Less than 2 issue messages and unknown message",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},   // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},   // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-3", Message: "Message-3"}}, // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},  // obj3
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1\n" +
					"* Phase3Obj obj1: Message-2\n" +
					"* Phase3Obj obj2: Message-3", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "At least 3 issue messages and unknown message",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},   // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},   // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-3", Message: "Message-3"}}, // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-4", Message: "Message-4"}},   // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},  // obj4
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1\n" +
					"* Phase3Obj obj1: Message-2\n" +
					"* Phase3Obj obj3: Message-4\n" +
					"And 1 Phase3Obj with status unknown", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "More than 3 issue messages and unknown message",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},   // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},   // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-3", Message: "Message-3"}}, // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-4", Message: "Message-4"}},   // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-5", Message: "Message-5"}},   // obj4
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-6", Message: "Message-6"}},   // obj5
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},  // obj6
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1\n" +
					"* Phase3Obj obj1: Message-2\n" +
					"* Phase3Obj obj3: Message-4\n" +
					"And 2 Phase3Objs with other issues\n" +
					"And 1 Phase3Obj with status unknown", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "unknown messages",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-1", Message: "Message-1"}}, // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-2", Message: "Message-2"}}, // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-4", Message: "Message-4"}}, // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-5", Message: "Message-5"}}, // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-1", Message: "Message-1"}}, // obj4
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionUnknown, Reason: "Reason-3", Message: "Message-3"}}, // obj5
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},  // obj6
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionUnknown, // Unknown because there is at least an unknown and no issue
				Reason: unknownReportedReason,   // Using a generic reason
				Message: "* Phase3Objs obj0, obj4: Message-1\n" +
					"* Phase3Obj obj1: Message-2\n" +
					"* Phase3Obj obj2: Message-4\n" +
					"And 2 Phase3Objs with status unknown", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "info messages",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}}, // obj0
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-2", Message: "Message-2"}}, // obj1
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-4", Message: "Message-4"}}, // obj2
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-5", Message: ""}},          // obj3
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}}, // obj4
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionTrue, Reason: "Reason-3", Message: "Message-3"}}, // obj5
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionTrue, // True because there are no issue and unknown
				Reason: infoReportedReason,   // Using a generic reason
				Message: "* Phase3Objs obj0, obj4: Message-1\n" +
					"* Phase3Obj obj1: Message-2\n" +
					"* Phase3Obj obj2: Message-4\n" +
					"And 1 Phase3Obj with additional info", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "Missing conditions are defaulted",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj0
				{}, // obj2 without available condition
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:   clusterv1.AvailableV1Beta2Condition,
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1\n" +
					"* Phase3Obj obj1: Condition Available not yet reported", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
		{
			name: "Missing conditions are defaulted why a custom target condition type",
			conditions: [][]metav1.Condition{
				{{Type: clusterv1.AvailableV1Beta2Condition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj0
				{}, // obj2 without available condition
			},
			conditionType: clusterv1.AvailableV1Beta2Condition,
			options:       []AggregateOption{TargetConditionType("SomethingAvailable")},
			want: &metav1.Condition{
				Type:   "SomethingAvailable",
				Status: metav1.ConditionFalse, // False because there is one issue
				Reason: issuesReportedReason,  // Using a generic reason
				Message: "* Phase3Obj obj0: Message-1\n" +
					"* Phase3Obj obj1: Condition Available not yet reported", // messages from all the issues & unknown conditions (info dropped)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := make([]Getter, 0, len(tt.conditions))
			for i := range tt.conditions {
				objs = append(objs, &builder.Phase3Obj{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: metav1.NamespaceDefault,
						Name:      fmt.Sprintf("obj%d", i),
					},
					Status: builder.Phase3ObjStatus{
						Conditions: tt.conditions[i],
					},
				})
			}

			got, err := NewAggregateCondition(objs, tt.conditionType, tt.options...)
			g.Expect(err != nil).To(Equal(tt.wantErr))

			g.Expect(got).To(Equal(tt.want))
		})
	}

	t.Run("Fails if source objects are empty", func(t *testing.T) {
		var objs []*builder.Phase3Obj
		g := NewWithT(t)
		_, err := NewAggregateCondition(objs, clusterv1.AvailableV1Beta2Condition)
		g.Expect(err).To(HaveOccurred())
	})
}
