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
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatchConditions(t *testing.T) {
	t0 := metav1.Now()

	testCases := []struct {
		name        string
		actual      interface{}
		expected    []metav1.Condition
		expectMatch bool
	}{
		{
			name:        "with an empty conditions",
			actual:      []metav1.Condition{},
			expected:    []metav1.Condition{},
			expectMatch: true,
		},
		{
			name: "with matching conditions",
			actual: []metav1.Condition{
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "reason",
					Message:            "message",
				},
			},
			expected: []metav1.Condition{
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "reason",
					Message:            "message",
				},
			},
			expectMatch: true,
		},
		{
			name: "with non-matching conditions",
			actual: []metav1.Condition{
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "reason",
					Message:            "message",
				},
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "reason",
					Message:            "message",
				},
			},
			expected: []metav1.Condition{
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "reason",
					Message:            "message",
				},
				{
					Type:               "different",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "different",
					Message:            "different",
				},
			},
			expectMatch: false,
		},
		{
			name: "with a different number of conditions",
			actual: []metav1.Condition{
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "reason",
					Message:            "message",
				},
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
					Reason:             "reason",
					Message:            "message",
				},
			},
			expected: []metav1.Condition{
				{
					Type:               "type",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: t0,
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
	t0 := metav1.Now()
	t1 := metav1.NewTime(t0.Add(1 * -time.Minute))

	testCases := []struct {
		name        string
		actual      interface{}
		expected    metav1.Condition
		options     []MatchOption
		expectMatch bool
	}{
		{
			name:        "with an empty condition",
			actual:      metav1.Condition{},
			expected:    metav1.Condition{},
			options:     []MatchOption{},
			expectMatch: true,
		},
		{
			name: "with a matching condition",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			expected: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			options:     []MatchOption{},
			expectMatch: true,
		},
		{
			name: "with a different LastTransitionTime",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			expected: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t1,
				Reason:             "reason",
				Message:            "message",
			},
			options:     []MatchOption{},
			expectMatch: false,
		},
		{
			name: "with a different LastTransitionTime but with IgnoreLastTransitionTime",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			expected: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t1,
				Reason:             "reason",
				Message:            "message",
			},
			options:     []MatchOption{IgnoreLastTransitionTime(true)},
			expectMatch: true,
		},
		{
			name: "with a different type",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			expected: metav1.Condition{
				Type:               "different",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			options:     []MatchOption{},
			expectMatch: false,
		},
		{
			name: "with a different status",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			expected: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionFalse,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			options:     []MatchOption{},
			expectMatch: false,
		},
		{
			name: "with a different ObservedGeneration",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
				ObservedGeneration: 1,
			},
			expected: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
				ObservedGeneration: 2,
			},
			options:     []MatchOption{},
			expectMatch: false,
		},
		{
			name: "with a different reason",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			expected: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "different",
				Message:            "message",
			},
			options:     []MatchOption{},
			expectMatch: false,
		},
		{
			name: "with a different message",
			actual: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "message",
			},
			expected: metav1.Condition{
				Type:               "type",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: t0,
				Reason:             "reason",
				Message:            "different",
			},
			options:     []MatchOption{},
			expectMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			if tc.expectMatch {
				g.Expect(tc.actual).To(MatchCondition(tc.expected, tc.options...))
			} else {
				g.Expect(tc.actual).ToNot(MatchCondition(tc.expected, tc.options...))
			}
		})
	}
}
