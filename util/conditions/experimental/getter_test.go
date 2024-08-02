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

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

type ObjWithoutStatus struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (f *ObjWithoutStatus) DeepCopyObject() runtime.Object {
	panic("implement me")
}

type ObjWithStatusWithoutConditions struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status struct {
	}
}

func (f *ObjWithStatusWithoutConditions) DeepCopyObject() runtime.Object {
	panic("implement me")
}

type ObjWithWrongConditionType struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status struct {
		Conditions []string
	}
}

func (f *ObjWithWrongConditionType) DeepCopyObject() runtime.Object {
	panic("implement me")
}

type ObjWithWrongExperimentalConditionType struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status struct {
		Conditions             clusterv1.Conditions
		ExperimentalConditions []string
	}
}

func (f *ObjWithWrongExperimentalConditionType) DeepCopyObject() runtime.Object {
	panic("implement me")
}

type V1Beta1ResourceWithLegacyConditions struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status struct {
		Conditions clusterv1.Conditions
	}
}

func (f *V1Beta1ResourceWithLegacyConditions) DeepCopyObject() runtime.Object {
	panic("implement me")
}

type V1Beta1ResourceWithLegacyAndExperimentalConditionsV1 struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status struct {
		Conditions             clusterv1.Conditions
		ExperimentalConditions []metav1.Condition
	}
}

func (f *V1Beta1ResourceWithLegacyAndExperimentalConditionsV1) DeepCopyObject() runtime.Object {
	panic("implement me")
}

type V1Beta2ResourceWithConditionsAndBackwardCompatibleConditions struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status struct {
		Conditions            []metav1.Condition
		BackwardCompatibility struct {
			Conditions clusterv1.Conditions
		}
	}
}

func (f *V1Beta2ResourceWithConditionsAndBackwardCompatibleConditions) DeepCopyObject() runtime.Object {
	panic("implement me")
}

type V1Beta2ResourceWithConditions struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Status struct {
		Conditions []metav1.Condition
	}
}

func (f *V1Beta2ResourceWithConditions) DeepCopyObject() runtime.Object {
	panic("implement me")
}

func TestGetAll(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()

	t.Run("fails with nil", func(t *testing.T) {
		g := NewWithT(t)

		_, err := GetAll(nil)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("fails for nil object", func(t *testing.T) {
		g := NewWithT(t)
		var foo *V1Beta1ResourceWithLegacyConditions

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		var fooUnstructured *unstructured.Unstructured

		_, err = GetAll(fooUnstructured)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("fails for object without status", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithoutStatus{}

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		_, err = GetAll(&unstructured.Unstructured{Object: fooUnstructured})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("fails for object with status without conditions or experimental conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithStatusWithoutConditions{}

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		_, err = GetAll(&unstructured.Unstructured{Object: fooUnstructured})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("fails for object with wrong condition type", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithWrongConditionType{}

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		// TODO: think about how unstructured can detect wrong type
	})

	t.Run("fails for object with wrong experimental condition type", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithWrongExperimentalConditionType{}

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		// TODO: think about how unstructured can detect wrong type
	})

	t.Run("v1beta object with legacy conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta1ResourceWithLegacyConditions{
			Status: struct{ Conditions clusterv1.Conditions }{Conditions: clusterv1.Conditions{
				{
					Type:               "fooCondition",
					Status:             corev1.ConditionTrue,
					LastTransitionTime: now,
				},
				{
					Type:               "fooCondition",
					Status:             corev1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "FooReason",
					Message:            "FooMessage",
				},
			}},
		}

		expect := []metav1.Condition{
			{
				Type:               string(foo.Status.Conditions[0].Type),
				Status:             metav1.ConditionStatus(foo.Status.Conditions[0].Status),
				LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			},
			{
				Type:               string(foo.Status.Conditions[1].Type),
				Status:             metav1.ConditionStatus(foo.Status.Conditions[1].Status),
				LastTransitionTime: foo.Status.Conditions[1].LastTransitionTime,
				Reason:             foo.Status.Conditions[1].Reason,
				Message:            foo.Status.Conditions[1].Message,
			},
		}

		got, err := GetAll(foo)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err = GetAll(&unstructured.Unstructured{Object: fooUnstructured})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))
	})

	t.Run("v1beta1 object with both legacy and experimental conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta1ResourceWithLegacyAndExperimentalConditionsV1{
			Status: struct {
				Conditions             clusterv1.Conditions
				ExperimentalConditions []metav1.Condition
			}{
				Conditions: clusterv1.Conditions{
					{
						Type:               "barCondition",
						Status:             corev1.ConditionFalse,
						LastTransitionTime: now,
					},
				},
				ExperimentalConditions: []metav1.Condition{
					{
						Type:               "fooCondition",
						Status:             metav1.ConditionTrue,
						ObservedGeneration: 10,
						LastTransitionTime: now,
						Reason:             "FooReason",
						Message:            "FooMessage",
					},
				},
			},
		}

		expect := []metav1.Condition{
			{
				Type:               foo.Status.ExperimentalConditions[0].Type,
				Status:             foo.Status.ExperimentalConditions[0].Status,
				LastTransitionTime: foo.Status.ExperimentalConditions[0].LastTransitionTime,
				ObservedGeneration: foo.Status.ExperimentalConditions[0].ObservedGeneration,
				Reason:             foo.Status.ExperimentalConditions[0].Reason,
				Message:            foo.Status.ExperimentalConditions[0].Message,
			},
		}

		got, err := GetAll(foo)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err = GetAll(&unstructured.Unstructured{Object: fooUnstructured})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))
	})

	t.Run("v1beta2 object with conditions and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta2ResourceWithConditionsAndBackwardCompatibleConditions{
			Status: struct {
				Conditions            []metav1.Condition
				BackwardCompatibility struct{ Conditions clusterv1.Conditions }
			}{
				Conditions: []metav1.Condition{
					{
						Type:               "fooCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
					},
				},
				BackwardCompatibility: struct{ Conditions clusterv1.Conditions }{
					Conditions: clusterv1.Conditions{
						{
							Type:               "barCondition",
							Status:             corev1.ConditionFalse,
							LastTransitionTime: now,
						},
					},
				},
			},
		}

		expect := []metav1.Condition{
			{
				Type:               foo.Status.Conditions[0].Type,
				Status:             foo.Status.Conditions[0].Status,
				LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			},
		}

		got, err := GetAll(foo)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err = GetAll(&unstructured.Unstructured{Object: fooUnstructured})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))
	})

	t.Run("v1beta2 object with conditions (end state)", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta2ResourceWithConditions{
			Status: struct {
				Conditions []metav1.Condition
			}{
				Conditions: []metav1.Condition{
					{
						Type:               "fooCondition",
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
					},
				},
			},
		}

		expect := []metav1.Condition{
			{
				Type:               foo.Status.Conditions[0].Type,
				Status:             foo.Status.Conditions[0].Status,
				LastTransitionTime: foo.Status.Conditions[0].LastTransitionTime,
			},
		}

		got, err := GetAll(foo)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))

		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		got, err = GetAll(&unstructured.Unstructured{Object: fooUnstructured})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(expect), cmp.Diff(got, expect))
	})
}
