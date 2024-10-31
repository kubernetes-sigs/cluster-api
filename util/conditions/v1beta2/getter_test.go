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

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestGet(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()

	t.Run("handles nil", func(t *testing.T) {
		g := NewWithT(t)

		got := Get(nil, "bar")
		g.Expect(got).To(BeNil())
	})

	t.Run("handles pointer to nil object", func(t *testing.T) {
		g := NewWithT(t)
		var foo *builder.Phase1Obj

		got := Get(foo, "bar")
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase1 object with both legacy and v1beta2 conditions (v1beta2 nil)", func(t *testing.T) {
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
				V1Beta2: nil,
			},
		}

		got := Get(foo, "barCondition")
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase1 object with both legacy and v1beta2 conditions", func(t *testing.T) {
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
							ObservedGeneration: 10,
							LastTransitionTime: now,
							Reason:             "FooReason",
							Message:            "FooMessage",
						},
					},
				},
			},
		}

		expect := metav1.Condition{
			Type:               foo.Status.V1Beta2.Conditions[0].Type,
			Status:             foo.Status.V1Beta2.Conditions[0].Status,
			LastTransitionTime: foo.Status.V1Beta2.Conditions[0].LastTransitionTime,
			ObservedGeneration: foo.Status.V1Beta2.Conditions[0].ObservedGeneration,
			Reason:             foo.Status.V1Beta2.Conditions[0].Reason,
			Message:            foo.Status.V1Beta2.Conditions[0].Message,
		}

		got := Get(foo, "barCondition")
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(MatchCondition(expect), cmp.Diff(*got, expect))
	})

	t.Run("Phase2 object with conditions (nil) and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase2Obj{
			Status: builder.Phase2ObjStatus{
				Conditions: nil,
				Deprecated: &builder.Phase2ObjStatusDeprecated{
					V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{
						Conditions: clusterv1.Conditions{
							{
								Type:               "bazCondition",
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
							},
						},
					},
				},
			},
		}

		got := Get(foo, "barCondition")
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase2 object with conditions (empty) and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase2Obj{
			Status: builder.Phase2ObjStatus{
				Conditions: []metav1.Condition{},
				Deprecated: &builder.Phase2ObjStatusDeprecated{
					V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{
						Conditions: clusterv1.Conditions{
							{
								Type:               "bazCondition",
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
							},
						},
					},
				},
			},
		}

		got := Get(foo, "barCondition")
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase2 object with conditions and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase2Obj{
			Status: builder.Phase2ObjStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "barCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "fooReason",
					},
				},
				Deprecated: &builder.Phase2ObjStatusDeprecated{
					V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{
						Conditions: clusterv1.Conditions{
							{
								Type:               "bazCondition",
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
							},
						},
					},
				},
			},
		}

		expect := metav1.Condition{
			Type:               foo.Status.Conditions[0].Type,
			Status:             foo.Status.Conditions[0].Status,
			LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			Reason:             foo.Status.Conditions[0].Reason,
		}

		got := Get(foo, "barCondition")
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(MatchCondition(expect), cmp.Diff(*got, expect))
	})

	t.Run("Phase3 object with conditions (nil)", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			Status: builder.Phase3ObjStatus{
				Conditions: nil,
			},
		}
		got := Get(foo, "barCondition")
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase3 object with conditions (empty)", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			Status: builder.Phase3ObjStatus{
				Conditions: []metav1.Condition{},
			},
		}

		got := Get(foo, "barCondition")
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase3 object with conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			Status: builder.Phase3ObjStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "barCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "fooReason",
					},
				},
			},
		}

		expect := metav1.Condition{
			Type:               foo.Status.Conditions[0].Type,
			Status:             foo.Status.Conditions[0].Status,
			LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			Reason:             foo.Status.Conditions[0].Reason,
		}

		got := Get(foo, "barCondition")
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(MatchCondition(expect), cmp.Diff(*got, expect))
	})

	t.Run("handles objects with value getter", func(t *testing.T) {
		g := NewWithT(t)
		foo := &objectWithValueGetter{
			Status: objectWithValueGetterStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "barCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "fooReason",
					},
				},
			},
		}

		expect := metav1.Condition{
			Type:               foo.Status.Conditions[0].Type,
			Status:             foo.Status.Conditions[0].Status,
			LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			Reason:             foo.Status.Conditions[0].Reason,
		}

		got := Get(foo, "barCondition")
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(MatchCondition(expect), cmp.Diff(*got, expect))
	})
}

func TestUnstructuredGet(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()

	t.Run("handles nil", func(t *testing.T) {
		g := NewWithT(t)

		_, err := UnstructuredGet(nil, "bar")
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("handles pointer to nil object", func(t *testing.T) {
		g := NewWithT(t)
		var foo runtime.Unstructured

		_, err := UnstructuredGet(foo, "bar")
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("Phase1 object with both legacy and v1beta2 conditions (v1beta2 nil)", func(t *testing.T) {
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
				V1Beta2: nil,
			},
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase1 object with both legacy and v1beta2 conditions", func(t *testing.T) {
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
							ObservedGeneration: 10,
							LastTransitionTime: now,
							Reason:             "FooReason",
							Message:            "FooMessage",
						},
					},
				},
			},
		}

		expect := metav1.Condition{
			Type:               foo.Status.V1Beta2.Conditions[0].Type,
			Status:             foo.Status.V1Beta2.Conditions[0].Status,
			LastTransitionTime: foo.Status.V1Beta2.Conditions[0].LastTransitionTime,
			ObservedGeneration: foo.Status.V1Beta2.Conditions[0].ObservedGeneration,
			Reason:             foo.Status.V1Beta2.Conditions[0].Reason,
			Message:            foo.Status.V1Beta2.Conditions[0].Message,
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(MatchCondition(expect), cmp.Diff(*got, expect))
	})

	t.Run("Phase2 object with conditions (nil) and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase2Obj{
			Status: builder.Phase2ObjStatus{
				Conditions: nil,
				Deprecated: &builder.Phase2ObjStatusDeprecated{
					V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{
						Conditions: clusterv1.Conditions{
							{
								Type:               "bazCondition",
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
							},
						},
					},
				},
			},
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase2 object with conditions (empty) and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase2Obj{
			Status: builder.Phase2ObjStatus{
				Conditions: []metav1.Condition{},
				Deprecated: &builder.Phase2ObjStatusDeprecated{
					V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{
						Conditions: clusterv1.Conditions{
							{
								Type:               "bazCondition",
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
							},
						},
					},
				},
			},
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase2 object with conditions and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase2Obj{
			Status: builder.Phase2ObjStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "barCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "fooReason",
					},
				},
				Deprecated: &builder.Phase2ObjStatusDeprecated{
					V1Beta1: &builder.Phase2ObjStatusDeprecatedV1Beta1{
						Conditions: clusterv1.Conditions{
							{
								Type:               "bazCondition",
								Status:             corev1.ConditionFalse,
								LastTransitionTime: now,
							},
						},
					},
				},
			},
		}

		expect := metav1.Condition{
			Type:               foo.Status.Conditions[0].Type,
			Status:             foo.Status.Conditions[0].Status,
			LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			Reason:             foo.Status.Conditions[0].Reason,
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(MatchCondition(expect), cmp.Diff(*got, expect))
	})

	t.Run("Phase3 object with conditions (nil)", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			Status: builder.Phase3ObjStatus{
				Conditions: nil,
			},
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase3 object with conditions (empty)", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			Status: builder.Phase3ObjStatus{
				Conditions: []metav1.Condition{},
			},
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("Phase3 object with conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &builder.Phase3Obj{
			Status: builder.Phase3ObjStatus{
				Conditions: []metav1.Condition{
					{
						Type:               "barCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "fooReason",
					},
				},
			},
		}

		expect := metav1.Condition{
			Type:               foo.Status.Conditions[0].Type,
			Status:             foo.Status.Conditions[0].Status,
			LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			Reason:             foo.Status.Conditions[0].Reason,
		}

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := UnstructuredGet(&unstructured.Unstructured{Object: fooUnstructured}, "barCondition")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(*got).To(MatchCondition(expect), cmp.Diff(*got, expect))
	})
}

func TestConvertFromUnstructuredConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions []clusterv1.Condition
		want       []metav1.Condition
		wantError  bool
	}{
		{
			name: "Fails if Type is missing",
			conditions: clusterv1.Conditions{
				clusterv1.Condition{Status: corev1.ConditionTrue},
			},
			wantError: true,
		},
		{
			name: "Fails if Status is missing",
			conditions: clusterv1.Conditions{
				clusterv1.Condition{Type: clusterv1.ConditionType("foo")},
			},
			wantError: true,
		},
		{
			name: "Fails if Status is a wrong value",
			conditions: clusterv1.Conditions{
				clusterv1.Condition{Type: clusterv1.ConditionType("foo"), Status: "foo"},
			},
			wantError: true,
		},
		{
			name: "Defaults reason for positive polarity",
			conditions: clusterv1.Conditions{
				clusterv1.Condition{Type: clusterv1.ConditionType("foo"), Status: corev1.ConditionTrue},
			},
			wantError: false,
			want: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionTrue,
					Reason: NoReasonReported,
				},
			},
		},
		{
			name: "Defaults reason for negative polarity",
			conditions: clusterv1.Conditions{
				clusterv1.Condition{Type: clusterv1.ConditionType("foo"), Status: corev1.ConditionFalse},
			},
			wantError: false,
			want: []metav1.Condition{
				{
					Type:   "foo",
					Status: metav1.ConditionFalse,
					Reason: NoReasonReported,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&builder.Phase0Obj{Status: builder.Phase0ObjStatus{Conditions: tt.conditions}})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(unstructuredObj).To(HaveKey("status"))
			unstructuredStatusObj := unstructuredObj["status"].(map[string]interface{})
			g.Expect(unstructuredStatusObj).To(HaveKey("conditions"))

			got, err := convertFromUnstructuredConditions(unstructuredStatusObj["conditions"].([]interface{}))
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(got).To(Equal(tt.want), cmp.Diff(tt.want, got))
		})
	}
}

func TestIsMethods(t *testing.T) {
	g := NewWithT(t)

	obj := objectWithValueGetter{
		Status: objectWithValueGetterStatus{
			Conditions: []metav1.Condition{
				{Type: "trueCondition", Status: metav1.ConditionTrue},
				{Type: "falseCondition", Status: metav1.ConditionFalse},
				{Type: "unknownCondition", Status: metav1.ConditionUnknown},
			},
		},
	}

	// test isTrue
	g.Expect(IsTrue(obj, "trueCondition")).To(BeTrue())
	g.Expect(IsTrue(obj, "falseCondition")).To(BeFalse())
	g.Expect(IsTrue(obj, "unknownCondition")).To(BeFalse())
	// test isFalse
	g.Expect(IsFalse(obj, "trueCondition")).To(BeFalse())
	g.Expect(IsFalse(obj, "falseCondition")).To(BeTrue())
	g.Expect(IsFalse(obj, "unknownCondition")).To(BeFalse())

	// test isUnknown
	g.Expect(IsUnknown(obj, "trueCondition")).To(BeFalse())
	g.Expect(IsUnknown(obj, "falseCondition")).To(BeFalse())
	g.Expect(IsUnknown(obj, "unknownCondition")).To(BeTrue())
}

type objectWithValueGetter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              objectWithValueGetterSpec   `json:"spec,omitempty"`
	Status            objectWithValueGetterStatus `json:"status,omitempty"`
}

type objectWithValueGetterSpec struct {
	Foo string `json:"foo,omitempty"`
}

type objectWithValueGetterStatus struct {
	Bar        string             `json:"bar,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (o objectWithValueGetter) GetV1Beta2Conditions() []metav1.Condition {
	return o.Status.Conditions
}

func (o *objectWithValueGetter) SetV1Beta2Conditions(conditions []metav1.Condition) {
	o.Status.Conditions = conditions
}
