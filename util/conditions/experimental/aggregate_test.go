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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestAggregate(t *testing.T) {
	tests := []struct {
		name          string
		conditions    [][]metav1.Condition
		conditionType string
		options       []AggregateOption
		want          *metav1.Condition
	}{
		{
			name: "One issue",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj2
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                  // False because there is one issue
				Reason:  "Reason-1",                             // Picking the reason from the only existing issue
				Message: "(False): Message-1 from default/obj0", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "Same issue from up to tree objects",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj4
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                                              // False because there is one issue
				Reason:  ManyIssuesReason,                                                   // Using a generic reason
				Message: "(False): Message-1 from default/obj0, default/obj1, default/obj2", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "Same issue from more than tree objects",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}}, // obj4
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-B", Message: "Message-1"}}, // obj5
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Message: "Message-99"}},                     // obj6
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                                                                                        // False because there is one issue
				Reason:  ManyIssuesReason,                                                                                             // Using a generic reason
				Message: "(False): Message-1 from default/obj0, default/obj1, default/obj2 and 2 other V1Beta2ResourceWithConditions", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "Up to three different issue messages",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},  // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},  // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj4
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj5
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-3", Message: "Message-3"}},  // obj6
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj7
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                                                                                                                                        // False because there is one issue
				Reason:  ManyIssuesReason,                                                                                                                                             // Using a generic reason
				Message: "(False): Message-1 from default/obj0, default/obj3, default/obj4; (False): Message-2 from default/obj1, default/obj2; (False): Message-3 from default/obj5", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "More than three different issue messages",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},  // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-4", Message: "Message-4"}},  // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-5", Message: "Message-5"}},  // obj4
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},  // obj5
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-3", Message: "Message-3"}},  // obj6
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}}, // obj7
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                                                                                                                                                               // False because there is one issue
				Reason:  ManyIssuesReason,                                                                                                                                                                    // Using a generic reason
				Message: "(False): Message-1 from default/obj0, default/obj4; (False): Message-2 from default/obj1; (False): Message-3 from default/obj5; other 2 V1Beta2ResourceWithConditions with issues", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "Less than 2 issue messages and unknown message",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},   // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},   // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-3", Message: "Message-3"}}, // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},  // obj4
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                                                                                                // False because there is one issue
				Reason:  ManyIssuesReason,                                                                                                     // Using a generic reason
				Message: "(False): Message-1 from default/obj0; (False): Message-2 from default/obj1; (Unknown): Message-3 from default/obj2", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "At least 3 issue messages and unknown message",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-1", Message: "Message-1"}},   // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-2", Message: "Message-2"}},   // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-3", Message: "Message-3"}}, // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionFalse, Reason: "Reason-4", Message: "Message-4"}},   // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},  // obj4
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionFalse,                                                                                                                                             // False because there is one issue
				Reason:  ManyIssuesReason,                                                                                                                                                  // Using a generic reason
				Message: "(False): Message-1 from default/obj0; (False): Message-2 from default/obj1; (False): Message-4 from default/obj3; other 1 V1Beta2ResourceWithConditions unknown", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "unknown messages",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-1", Message: "Message-1"}}, // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-2", Message: "Message-2"}}, // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-4", Message: "Message-4"}}, // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-5", Message: "Message-5"}}, // obj4
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-1", Message: "Message-1"}}, // obj5
				{{Type: AvailableCondition, Status: metav1.ConditionUnknown, Reason: "Reason-3", Message: "Message-3"}}, // obj6
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-99", Message: "Message-99"}},  // obj7
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionUnknown,                                                                                                                                                               // False because there is one issue
				Reason:  ManyUnknownsReason,                                                                                                                                                                    // Using a generic reason
				Message: "(Unknown): Message-1 from default/obj0, default/obj4; (Unknown): Message-2 from default/obj1; (Unknown): Message-3 from default/obj5; other 2 V1Beta2ResourceWithConditions unknown", // messages from all the issues & unknown conditions (info dropped)
			},
		},
		{
			name: "info messages",
			conditions: [][]metav1.Condition{
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}}, // obj1
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-2", Message: "Message-2"}}, // obj2
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-4", Message: "Message-4"}}, // obj3
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-5", Message: ""}},          // obj4
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-1", Message: "Message-1"}}, // obj5
				{{Type: AvailableCondition, Status: metav1.ConditionTrue, Reason: "Reason-3", Message: "Message-3"}}, // obj6
			},
			conditionType: AvailableCondition,
			options:       []AggregateOption{},
			want: &metav1.Condition{
				Type:    AvailableCondition,
				Status:  metav1.ConditionTrue,                                                                                                                                                                    // False because there is one issue
				Reason:  ManyInfoReason,                                                                                                                                                                          // Using a generic reason
				Message: "(True): Message-1 from default/obj0, default/obj4; (True): Message-2 from default/obj1; (True): Message-3 from default/obj5; other 1 V1Beta2ResourceWithConditions with info messages", // messages from all the issues & unknown conditions (info dropped)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := make([]runtime.Object, 0, len(tt.conditions))
			for i := range tt.conditions {
				objs = append(objs, &V1Beta2ResourceWithConditions{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: metav1.NamespaceDefault,
						Name:      fmt.Sprintf("obj%d", i),
					},
					Status: struct{ Conditions []metav1.Condition }{
						Conditions: tt.conditions[i],
					},
				})
			}

			got, err := NewAggregateCondition(objs, tt.conditionType, tt.options...)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
