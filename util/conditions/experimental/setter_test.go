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

func TestSetAll(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()

	conditions := []metav1.Condition{
		{
			Type:               "fooCondition",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: 10,
			LastTransitionTime: now,
			Reason:             "FooReason",
			Message:            "FooMessage",
		},
	}

	cloneConditions := func() []metav1.Condition {
		ret := make([]metav1.Condition, len(conditions))
		copy(ret, conditions)
		return ret
	}

	t.Run("fails with nil", func(t *testing.T) {
		g := NewWithT(t)

		conditions := cloneConditions()
		err := SetAll(nil, conditions)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("fails for nil object", func(t *testing.T) {
		g := NewWithT(t)
		var foo *V1Beta1ResourceWithLegacyConditions

		conditions := cloneConditions()
		err := SetAll(foo, conditions)
		g.Expect(err).To(HaveOccurred())

		var fooUnstructured *unstructured.Unstructured

		err = SetAll(fooUnstructured, conditions, ConditionFields{"status", "experimentalConditions"})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("fails for object without status", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithoutStatus{}

		conditions := cloneConditions()
		err := SetAll(foo, conditions)
		g.Expect(err).To(HaveOccurred())

		// TODO: think about how unstructured can detect status without statue (if we ever need it, because for unstructured the users explicitly define the ConditionFields)
	})

	t.Run("fails for object with status without conditions or experimental conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithStatusWithoutConditions{}

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		// TODO: think about how unstructured can detect status without conditions or experimental condition type (if we ever need it, because for unstructured the users explicitly define the ConditionFields)
	})

	t.Run("fails for object with wrong condition type", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithWrongConditionType{}

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		// TODO: think about how unstructured can detect status with wrong conditions type (if we ever need it, because for unstructured the users explicitly define the ConditionFields)
	})

	t.Run("fails for object with wrong experimental condition type", func(t *testing.T) {
		g := NewWithT(t)
		foo := &ObjWithWrongExperimentalConditionType{}

		_, err := GetAll(foo)
		g.Expect(err).To(HaveOccurred())

		// TODO: think about how unstructured can detect status with wrong experimental conditions type (if we ever need it, because for unstructured the users explicitly define the ConditionFields)
	})

	t.Run("fails for Unstructured when ConditionFields are not provided", func(t *testing.T) {
		g := NewWithT(t)
		fooUnstructured := &unstructured.Unstructured{}

		conditions := cloneConditions()
		err := SetAll(fooUnstructured, conditions)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("v1beta object with legacy conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta1ResourceWithLegacyConditions{
			Status: struct{ Conditions clusterv1.Conditions }{Conditions: nil},
		}

		conditions := cloneConditions()
		err := SetAll(foo, conditions)
		g.Expect(err).To(HaveOccurred()) // Can't set legacy conditions.
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
				ExperimentalConditions: nil,
			},
		}

		conditions := cloneConditions()
		err := SetAll(foo, conditions)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(foo.Status.ExperimentalConditions).To(Equal(conditions), cmp.Diff(foo.Status.ExperimentalConditions, conditions))
	})

	t.Run("v1beta1 object with both legacy and experimental conditions / Unstructured", func(t *testing.T) {
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
				ExperimentalConditions: nil,
			},
		}

		conditions := cloneConditions()
		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		u := &unstructured.Unstructured{Object: fooUnstructured}
		err = SetAll(u, conditions, ConditionFields{"status", "experimentalConditions"})
		g.Expect(err).NotTo(HaveOccurred())

		fooFromUnstructured := &V1Beta1ResourceWithLegacyAndExperimentalConditionsV1{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), fooFromUnstructured)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := GetAll(fooFromUnstructured)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(conditions), cmp.Diff(got, conditions))
	})

	t.Run("v1beta2 object with conditions and backward compatible conditions", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta2ResourceWithConditionsAndBackwardCompatibleConditions{
			Status: struct {
				Conditions            []metav1.Condition
				BackwardCompatibility struct{ Conditions clusterv1.Conditions }
			}{
				Conditions: nil,
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

		conditions := cloneConditions()
		err := SetAll(foo, conditions)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(foo.Status.Conditions).To(Equal(conditions), cmp.Diff(foo.Status.Conditions, conditions))
	})

	t.Run("v1beta2 object with conditions and backward compatible conditions / Unstructured", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta2ResourceWithConditionsAndBackwardCompatibleConditions{
			Status: struct {
				Conditions            []metav1.Condition
				BackwardCompatibility struct{ Conditions clusterv1.Conditions }
			}{
				Conditions: nil,
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

		conditions := cloneConditions()
		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		u := &unstructured.Unstructured{Object: fooUnstructured}
		err = SetAll(u, conditions, ConditionFields{"status", "conditions"})
		g.Expect(err).NotTo(HaveOccurred())

		fooFromUnstructured := &V1Beta2ResourceWithConditionsAndBackwardCompatibleConditions{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), fooFromUnstructured)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := GetAll(fooFromUnstructured)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(conditions), cmp.Diff(got, conditions))
	})

	t.Run("v1beta2 object with conditions (end state)", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta2ResourceWithConditions{
			Status: struct {
				Conditions []metav1.Condition
			}{
				Conditions: nil,
			},
		}

		conditions := cloneConditions()
		err := SetAll(foo, conditions)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(foo.Status.Conditions).To(Equal(conditions), cmp.Diff(foo.Status.Conditions, conditions))
	})

	t.Run("v1beta2 object with conditions (end state) / Unstructured", func(t *testing.T) {
		g := NewWithT(t)
		foo := &V1Beta2ResourceWithConditions{
			Status: struct {
				Conditions []metav1.Condition
			}{
				Conditions: nil,
			},
		}

		conditions := cloneConditions()
		fooUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(foo)
		g.Expect(err).NotTo(HaveOccurred())

		u := &unstructured.Unstructured{Object: fooUnstructured}
		err = SetAll(u, conditions, ConditionFields{"status", "conditions"})
		g.Expect(err).NotTo(HaveOccurred())

		fooFromUnstructured := &V1Beta2ResourceWithConditions{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), fooFromUnstructured)
		g.Expect(err).NotTo(HaveOccurred())

		got, err := GetAll(fooFromUnstructured)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(conditions), cmp.Diff(got, conditions))
	})
}
