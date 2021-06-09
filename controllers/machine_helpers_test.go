/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"testing"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHasMatchingLabels(t *testing.T) {
	testCases := []struct {
		name     string
		selector metav1.LabelSelector
		labels   map[string]string
		expected bool
	}{
		{
			name: "selector matches labels",
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			labels: map[string]string{
				"foo":  "bar",
				"more": "labels",
			},
			expected: true,
		},
		{
			name: "selector does not match labels",
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			labels: map[string]string{
				"no": "match",
			},
			expected: false,
		},
		{
			name:     "selector is empty",
			selector: metav1.LabelSelector{},
			labels:   map[string]string{},
			expected: false,
		},
		{
			name: "selector is invalid",
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Operator: "bad-operator",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			got := hasMatchingLabels(tc.selector, tc.labels)
			g.Expect(got).To(Equal(tc.expected))
		})
	}
}
