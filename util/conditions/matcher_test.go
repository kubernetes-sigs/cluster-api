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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestMatchConditions(t *testing.T) {
	testCases := []struct {
		name        string
		actual      interface{}
		expected    clusterv1.Conditions
		expectMatch bool
	}{
		{
			name:        "with an empty conditions",
			actual:      clusterv1.Conditions{},
			expected:    clusterv1.Conditions{},
			expectMatch: true,
		},
		{
			name: "with matching conditions",
			actual: clusterv1.Conditions{
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
			},
			expected: clusterv1.Conditions{
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
			},
			expectMatch: true,
		},
		{
			name: "with non-matching conditions",
			actual: clusterv1.Conditions{
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
			},
			expected: clusterv1.Conditions{
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
				{
					Type:               clusterv1.ConditionType("different"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "different",
					Message:            "different",
				},
			},
			expectMatch: false,
		},
		{
			name: "with a different number of conditions",
			actual: clusterv1.Conditions{
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
			},
			expected: clusterv1.Conditions{
				{
					Type:               clusterv1.ConditionType("type"),
					Status:             corev1.ConditionTrue,
					Severity:           clusterv1.ConditionSeverityNone,
					LastTransitionTime: metav1.Now(),
					Reason:             "reason",
					Message:            "message",
				},
			},
			expectMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			if tc.expectMatch {
				g.Expect(tc.actual).To(MatchConditions(tc.expected))
			} else {
				g.Expect(tc.actual).ToNot(MatchConditions(tc.expected))
			}
		})
	}
}

func TestMatchCondition(t *testing.T) {
	testCases := []struct {
		name        string
		actual      interface{}
		expected    clusterv1.Condition
		expectMatch bool
	}{
		{
			name:        "with an empty condition",
			actual:      clusterv1.Condition{},
			expected:    clusterv1.Condition{},
			expectMatch: true,
		},
		{
			name: "with a matching condition",
			actual: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expected: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expectMatch: true,
		},
		{
			name: "with a different time",
			actual: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expected: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Time{},
				Reason:             "reason",
				Message:            "message",
			},
			expectMatch: true,
		},
		{
			name: "with a different type",
			actual: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expected: clusterv1.Condition{
				Type:               clusterv1.ConditionType("different"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expectMatch: false,
		},
		{
			name: "with a different status",
			actual: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expected: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionFalse,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expectMatch: false,
		},
		{
			name: "with a different severity",
			actual: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expected: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityInfo,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expectMatch: false,
		},
		{
			name: "with a different reason",
			actual: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expected: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "different",
				Message:            "message",
			},
			expectMatch: false,
		},
		{
			name: "with a different message",
			actual: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "message",
			},
			expected: clusterv1.Condition{
				Type:               clusterv1.ConditionType("type"),
				Status:             corev1.ConditionTrue,
				Severity:           clusterv1.ConditionSeverityNone,
				LastTransitionTime: metav1.Now(),
				Reason:             "reason",
				Message:            "different",
			},
			expectMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			if tc.expectMatch {
				g.Expect(tc.actual).To(MatchCondition(tc.expected))
			} else {
				g.Expect(tc.actual).ToNot(MatchCondition(tc.expected))
			}
		})
	}
}
