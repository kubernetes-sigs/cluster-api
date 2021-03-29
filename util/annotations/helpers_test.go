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
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestAddAnnotations(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
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
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			res := AddAnnotations(tc.obj, tc.input)
			g.Expect(res).To(Equal(tc.changed))
			g.Expect(tc.obj.GetAnnotations()).To(Equal(tc.expected))
		})
	}
}
