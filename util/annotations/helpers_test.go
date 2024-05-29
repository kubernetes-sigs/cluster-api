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

package annotations

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAddAnnotations(t *testing.T) {
	g := NewWithT(t)

	testcases := []struct {
		name     string
		obj      metav1.Object
		input    map[string]string
		expected map[string]string
		changed  bool
	}{
		{
			name: "should return false if no changes are made",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input: map[string]string{
				"foo": "bar",
			},
			expected: map[string]string{
				"foo": "bar",
			},
			changed: false,
		},
		{
			name: "should do nothing if no annotations are provided",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input: map[string]string{},
			expected: map[string]string{
				"foo": "bar",
			},
			changed: false,
		},
		{
			name: "should do nothing if no annotations are provided and have been nil before",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input:    map[string]string{},
			expected: nil,
			changed:  false,
		},
		{
			name: "should return true if annotations are added",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input: map[string]string{
				"thing1": "thing2",
				"buzz":   "blah",
			},
			expected: map[string]string{
				"foo":    "bar",
				"thing1": "thing2",
				"buzz":   "blah",
			},
			changed: true,
		},
		{
			name: "should return true if annotations are changed",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input: map[string]string{
				"foo": "buzz",
			},
			expected: map[string]string{
				"foo": "buzz",
			},
			changed: true,
		},
		{
			name: "should return true if annotations are changed and have been nil before",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input: map[string]string{
				"foo": "buzz",
			},
			expected: map[string]string{
				"foo": "buzz",
			},
			changed: true,
		},
		{
			name: "should add annotations to an empty unstructured",
			obj:  &unstructured.Unstructured{},
			input: map[string]string{
				"foo": "buzz",
			},
			expected: map[string]string{
				"foo": "buzz",
			},
			changed: true,
		},
		{
			name: "should add annotations to a non empty unstructured",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			input: map[string]string{
				"thing1": "thing2",
				"buzz":   "blah",
			},
			expected: map[string]string{
				"foo":    "bar",
				"thing1": "thing2",
				"buzz":   "blah",
			},
			changed: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(*testing.T) {
			res := AddAnnotations(tc.obj, tc.input)
			g.Expect(res).To(Equal(tc.changed))
			g.Expect(tc.obj.GetAnnotations()).To(Equal(tc.expected))
		})
	}
}

func TestHasTruthyAnnotationValue(t *testing.T) {
	tests := []struct {
		name          string
		obj           metav1.Object
		annotationKey string
		expected      bool
	}{
		{
			name: "annotation does not exist",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"cluster.x-k8s.io/some-other-annotation": "",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			annotationKey: "cluster.x-k8s.io/replicas-managed-by",
			expected:      false,
		},
		{
			name: "no val",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"cluster.x-k8s.io/replicas-managed-by": "",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			annotationKey: "cluster.x-k8s.io/replicas-managed-by",
			expected:      true,
		},
		{
			name: "annotation exists, true value",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"cluster.x-k8s.io/replicas-managed-by": "true",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			annotationKey: "cluster.x-k8s.io/replicas-managed-by",
			expected:      true,
		},
		{
			name: "annotation exists, random string value",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"cluster.x-k8s.io/replicas-managed-by": "foo",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			annotationKey: "cluster.x-k8s.io/replicas-managed-by",
			expected:      true,
		},
		{
			name: "annotation exists, false value",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"cluster.x-k8s.io/replicas-managed-by": "false",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			annotationKey: "cluster.x-k8s.io/replicas-managed-by",
			expected:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ret := hasTruthyAnnotationValue(tt.obj, tt.annotationKey)
			if tt.expected {
				g.Expect(ret).To(BeTrue())
			} else {
				g.Expect(ret).To(BeFalse())
			}
		})
	}
}
