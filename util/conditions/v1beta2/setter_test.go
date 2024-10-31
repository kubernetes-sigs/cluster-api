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

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestSet(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()

	condition := metav1.Condition{
		Type:               "fooCondition",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 0, // NOTE: this is a dedicated tests about inferring ObservedGeneration.
		LastTransitionTime: now,
		Reason:             "FooReason",
		Message:            "FooMessage",
	}

	cloneCondition := func() metav1.Condition {
		return *condition.DeepCopy()
	}

	t.Run("no-op with nil", func(_ *testing.T) {
		condition := cloneCondition()
		Set(nil, condition)
	})

	t.Run("handles pointer to nil object", func(_ *testing.T) {
		var foo *builder.Phase1Obj
		condition := cloneCondition()
		Set(foo, condition)
	})

	t.Run("Phase1Obj object with both legacy and v1beta2 conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase1Obj{
			Status: builder.Phase1ObjStatus{
				Conditions: clusterv1.Conditions{
					{
						Type:               "bazCondition",
						Status:             corev1.ConditionFalse,
						LastTransitionTime: now,
					},
				},
				V1Beta2: &builder.Phase1ObjStatusV1Beta2{
					Conditions: []metav1.Condition{
						{
							Type:               "barCondition",
							Status:             metav1.ConditionTrue,
							LastTransitionTime: now,
						},
					},
				},
			},
		}

		condition := cloneCondition()
		expected := []metav1.Condition{
			foo.Status.V1Beta2.Conditions[0],
			condition,
		}

		Set(foo, condition)
		g.Expect(foo.Status.V1Beta2.Conditions).To(Equal(expected), cmp.Diff(foo.Status.V1Beta2.Conditions, expected))
	})

	t.Run("Phase2Obj object with conditions and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase2Obj{
			Status: builder.Phase2ObjStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "barCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
					},
				},
				Deprecated: &builder.Phase2ObjStatusDeprecated{
					V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{
						Conditions: clusterv1.Conditions{
							{
								Type:               "barCondition",
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
							},
						},
					},
				},
			},
		}

		condition := cloneCondition()
		expected := []metav1.Condition{
			foo.Status.Conditions[0],
			condition,
		}

		Set(foo, condition)
		g.Expect(foo.Status.Conditions).To(Equal(expected), cmp.Diff(foo.Status.Conditions, expected))
	})

	t.Run("Phase3Obj object with conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			Status: builder.Phase3ObjStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "barCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
					},
					{
						Type:               "zzzCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
					},
				},
			},
		}

		condition := cloneCondition()
		expected := []metav1.Condition{
			foo.Status.Conditions[0],
			condition,
			foo.Status.Conditions[1],
		}

		Set(foo, condition)
		g.Expect(foo.Status.Conditions).To(Equal(expected), cmp.Diff(foo.Status.Conditions, expected))
	})

	t.Run("Set infers ObservedGeneration", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			ObjectMeta: metav1.ObjectMeta{Generation: 123},
			Status: builder.Phase3ObjStatus{
				Conditions: nil,
			},
		}

		condition := metav1.Condition{
			Type:               "fooCondition",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: now,
			Reason:             "FooReason",
			Message:            "FooMessage",
		}

		Set(foo, condition)

		condition.ObservedGeneration = foo.Generation
		expected := []metav1.Condition{condition}
		g.Expect(foo.Status.Conditions).To(Equal(expected), cmp.Diff(foo.Status.Conditions, expected))
	})

	t.Run("Set drops milliseconds", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			ObjectMeta: metav1.ObjectMeta{Generation: 123},
			Status: builder.Phase3ObjStatus{
				Conditions: nil,
			},
		}

		condition := metav1.Condition{
			Type:    "fooCondition",
			Status:  metav1.ConditionTrue,
			Reason:  "FooReason",
			Message: "FooMessage",
		}

		// Check LastTransitionTime after setting a condition for the first time
		Set(foo, condition)
		ltt1 := foo.Status.Conditions[0].LastTransitionTime.Time
		g.Expect(ltt1).To(Equal(ltt1.Truncate(1*time.Second)), cmp.Diff(ltt1, ltt1.Truncate(1*time.Second)))

		// Check LastTransitionTime after changing an existing condition
		condition.Status = metav1.ConditionFalse     // this will force set to change the LastTransitionTime
		condition.LastTransitionTime = metav1.Time{} // this will force set to compute a new LastTransitionTime
		Set(foo, condition)
		ltt2 := foo.Status.Conditions[0].LastTransitionTime.Time
		g.Expect(ltt2).To(Equal(ltt2.Truncate(1*time.Second)), cmp.Diff(ltt2, ltt2.Truncate(1*time.Second)))

		// Check LastTransitionTime after setting a Time with milliseconds
		condition.Status = metav1.ConditionTrue     // this will force set to change the LastTransitionTime
		condition.LastTransitionTime = metav1.Now() // this will force set to not default LastTransitionTime
		Set(foo, condition)
		ltt3 := foo.Status.Conditions[0].LastTransitionTime.Time
		g.Expect(ltt3).To(Equal(ltt3.Truncate(1*time.Second)), cmp.Diff(ltt3, ltt3.Truncate(1*time.Second)))
	})
}

func TestDelete(t *testing.T) {
	g := NewWithT(t)

	obj := &builder.Phase2Obj{
		Status: builder.Phase2ObjStatus{
			Conditions: []metav1.Condition{
				{Type: "trueCondition", Status: metav1.ConditionTrue},
				{Type: "falseCondition", Status: metav1.ConditionFalse},
			},
		},
	}

	Delete(nil, "foo") // no-op
	Delete(obj, "trueCondition")
	Delete(obj, "trueCondition") // no-op

	g.Expect(obj.GetV1Beta2Conditions()).To(MatchConditions([]metav1.Condition{{Type: "falseCondition", Status: metav1.ConditionFalse}}, IgnoreLastTransitionTime(true)))
}
