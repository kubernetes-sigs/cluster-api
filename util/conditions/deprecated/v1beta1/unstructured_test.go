/*
Copyright 2020 The Kubernetes Authors.

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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestUnstructuredGetConditions(t *testing.T) {
	g := NewWithT(t)

	// GetV1Beta1Conditions should return conditions from an unstructured object (conditions in status.deprecated.v1beta1.conditions)
	u1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"deprecated": map[string]interface{}{
					"v1beta1": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "true1",
								"status": "True",
							},
						},
					},
				},
			},
		},
	}

	g.Expect(UnstructuredGetter(u1).GetV1Beta1Conditions()).To(haveSameConditionsOf(conditionList(true1)))

	// GetV1Beta1Conditions should return conditions from an unstructured object
	u2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "true1",
						"status": "True",
					},
				},
			},
		},
	}

	g.Expect(UnstructuredGetter(u2).GetV1Beta1Conditions()).To(haveSameConditionsOf(conditionList(true1)))

	// GetV1Beta1Conditions should return nil for an unstructured object with empty conditions
	u3 := &unstructured.Unstructured{}

	g.Expect(UnstructuredGetter(u3).GetV1Beta1Conditions()).To(BeNil())

	// GetV1Beta1Conditions should return nil for an unstructured object without conditions
	e := &corev1.Pod{}
	u4 := &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(e, u4, nil)).To(Succeed())

	g.Expect(UnstructuredGetter(u4).GetV1Beta1Conditions()).To(BeNil())

	// GetV1Beta1Conditions should return conditions from an unstructured object with a different type of conditions.
	p := &corev1.Pod{Status: corev1.PodStatus{
		Conditions: []corev1.PodCondition{
			{
				Type:               "foo",
				Status:             "foo",
				LastProbeTime:      metav1.Time{},
				LastTransitionTime: metav1.Time{},
				Reason:             "foo",
				Message:            "foo",
			},
		},
	}}
	u5 := &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(p, u5, nil)).To(Succeed())

	g.Expect(UnstructuredGetter(u5).GetV1Beta1Conditions()).To(HaveLen(1))
}

func TestUnstructuredSetConditions(t *testing.T) {
	g := NewWithT(t)

	c := &clusterv1.Cluster{}
	u := &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(c, u, nil)).To(Succeed())

	// set conditions
	conditions := conditionList(true1, falseInfo1)

	s := UnstructuredSetter(u)
	s.SetV1Beta1Conditions(conditions)
	g.Expect(u.Object).To(HaveKey("status"))
	status := u.Object["status"].(map[string]any)
	g.Expect(status).To(HaveKey("deprecated"))
	deprecated := status["deprecated"].(map[string]any)
	g.Expect(deprecated).To(HaveKey("v1beta1"))
	v1beta1 := deprecated["v1beta1"].(map[string]any)
	g.Expect(v1beta1).To(HaveKey("conditions"))

	g.Expect(s.GetV1Beta1Conditions()).To(BeComparableTo(conditions))
}
