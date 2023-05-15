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

package labels

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestHasWatchLabel(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
		name     string
		obj      metav1.Object
		input    string
		expected bool
	}{
		{
			name: "should handle empty input",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input:    "",
			expected: false,
		},
		{
			name: "should return false if no input string is give",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.WatchLabel: "bar",
					},
				},
			},
			input:    "",
			expected: false,
		},
		{
			name: "should return true if label matches",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.WatchLabel: "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input:    "bar",
			expected: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			res := HasWatchLabel(tc.obj, tc.input)
			g.Expect(res).To(Equal(tc.expected))
		})
	}
}

func TestSetObjectLabel(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
		name string
		obj  metav1.Object
	}{
		{
			name: "labels are nil map, set label",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{},
			},
		},
		{
			name: "labels are not nil, set label",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
		},
		{
			name: "label exists in map, overwrite label",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.ClusterTopologyOwnedLabel: "random-value",
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			SetObjectLabel(tc.obj, clusterv1.ClusterTopologyOwnedLabel)
			labels := tc.obj.GetLabels()

			val, ok := labels[clusterv1.ClusterTopologyOwnedLabel]
			g.Expect(ok).To(BeTrue())
			g.Expect(val).To(Equal("true"))
		})
	}
}
