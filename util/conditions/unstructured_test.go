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

package conditions

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestUnstructuredGetConditions(t *testing.T) {
	g := NewWithT(t)

	// GetConditions should return conditions from an unstructured object
	c := &clusterv1.Cluster{}
	c.SetConditions(conditionList(true1))
	u := &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(c, u, nil)).To(Succeed())

	g.Expect(UnstructuredGetter(u).GetConditions()).To(haveSameConditionsOf(conditionList(true1)))

	// GetConditions should return nil for an unstructured object with empty conditions
	c = &clusterv1.Cluster{}
	u = &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(c, u, nil)).To(Succeed())

	g.Expect(UnstructuredGetter(u).GetConditions()).To(BeNil())

	// GetConditions should return nil for an unstructured object without conditions
	e := &corev1.Endpoints{}
	u = &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(e, u, nil)).To(Succeed())

	g.Expect(UnstructuredGetter(u).GetConditions()).To(BeNil())

	// GetConditions should return conditions from an unstructured object with a different type of conditions.
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
	u = &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(p, u, nil)).To(Succeed())

	g.Expect(UnstructuredGetter(u).GetConditions()).To(HaveLen(1))
}

func TestUnstructuredSetConditions(t *testing.T) {
	g := NewWithT(t)

	c := &clusterv1.Cluster{}
	u := &unstructured.Unstructured{}
	g.Expect(scheme.Scheme.Convert(c, u, nil)).To(Succeed())

	// set conditions
	conditions := conditionList(true1, falseInfo1)

	s := UnstructuredSetter(u)
	s.SetConditions(conditions)
	g.Expect(s.GetConditions()).To(Equal(conditions))
}
