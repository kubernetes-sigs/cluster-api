/*
Copyright The Kubernetes Authors.

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

package api

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
)

func TestConditionsUnmarshalJSON(t *testing.T) {
	lastTransitionTime := metav1.NewTime(time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC))

	tests := []struct {
		name      string
		input     []byte
		expectErr bool
		expect    Conditions
	}{
		{
			name:   "empty array unmarshals to no conditions",
			input:  []byte(`[]`),
			expect: nil,
		},
		{
			name:      "invalid JSON returns an error",
			input:     []byte(`not-json`),
			expectErr: true,
		},
		{
			name: "keeps only the Ready condition, discards others",
			input: []byte(`[
				{"type":"Available","status":"True"},
				{"type":"Ready","status":"True","reason":"AllGood","message":"all good"},
				{"type":"Degraded","status":"False"}
			]`),
			expect: Conditions{
				{Type: "Ready", Status: "True", Reason: "AllGood", Message: "all good"},
			},
		},
		{
			name:   "no Ready condition present unmarshals to no conditions",
			input:  []byte(`[{"type":"Available","status":"True"}]`),
			expect: nil,
		},
		{
			name: "unmarshals a Ready condition coming from a clusterv1beta1.Conditions payload",
			input: mustMarshal(t, clusterv1beta1.Conditions{
				{
					Type:               "Available",
					Status:             corev1.ConditionTrue,
					LastTransitionTime: lastTransitionTime,
				},
				{
					Type:               "Ready",
					Status:             corev1.ConditionFalse,
					Severity:           clusterv1beta1.ConditionSeverityWarning,
					LastTransitionTime: lastTransitionTime,
					Reason:             "WaitingForFoo",
					Message:            "waiting for foo to complete",
				},
			}),
			expect: Conditions{
				{
					Type:               "Ready",
					Status:             "False",
					Severity:           "Warning",
					LastTransitionTime: mustRoundTripTime(t, lastTransitionTime),
					Reason:             "WaitingForFoo",
					Message:            "waiting for foo to complete",
				},
			},
		},
		{
			name: "unmarshals a Ready condition coming from a []metav1.Condition payload",
			input: mustMarshal(t, []metav1.Condition{
				{
					Type:               "Available",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: lastTransitionTime,
					Reason:             "Available",
					Message:            "is available",
				},
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 3,
					LastTransitionTime: lastTransitionTime,
					Reason:             "AllGood",
					Message:            "all good",
				},
			}),
			expect: Conditions{
				{
					Type:               "Ready",
					Status:             "True",
					ObservedGeneration: 3,
					LastTransitionTime: mustRoundTripTime(t, lastTransitionTime),
					Reason:             "AllGood",
					Message:            "all good",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var got Conditions
			err := got.UnmarshalJSON(tt.input)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.expect))
		})
	}
}

func TestConditionsReadyConditionAsAnyArray(t *testing.T) {
	readyTransitionTime := metav1.NewTime(time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC))

	tests := []struct {
		name   string
		in     Conditions
		expect []any
	}{
		{
			name:   "no conditions returns nil",
			in:     Conditions{},
			expect: nil,
		},
		{
			name: "a non-Ready condition is filtered out",
			in: Conditions{
				{Type: "Available", Status: "True"},
			},
			expect: nil,
		},
		{
			name: "zero-value fields are omitted from the resulting map",
			in: Conditions{
				{Type: "Ready", Status: "True"},
			},
			expect: []any{
				map[string]any{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
		{
			name: "a fully populated Ready condition includes all fields",
			in: Conditions{
				{
					Type:               "Ready",
					Status:             "False",
					ObservedGeneration: 2,
					LastTransitionTime: readyTransitionTime,
					Reason:             "WaitingForFoo",
					Message:            "waiting for foo to complete",
					Severity:           "Warning",
				},
			},
			expect: []any{
				map[string]any{
					"type":               "Ready",
					"status":             "False",
					"observedGeneration": int64(2),
					"lastTransitionTime": readyTransitionTime.Format(time.RFC3339),
					"reason":             "WaitingForFoo",
					"message":            "waiting for foo to complete",
					"severity":           "Warning",
				},
			},
		},
		{
			name: "a Ready condition round-tripped from a clusterv1beta1.Conditions payload has no observedGeneration",
			in: mustUnmarshalConditions(t, clusterv1beta1.Conditions{
				{
					Type:               "Ready",
					Status:             corev1.ConditionFalse,
					Severity:           clusterv1beta1.ConditionSeverityWarning,
					LastTransitionTime: readyTransitionTime,
					Reason:             "WaitingForFoo",
					Message:            "waiting for foo to complete",
				},
			}),
			expect: []any{
				map[string]any{
					"type":               "Ready",
					"status":             "False",
					"severity":           "Warning",
					"lastTransitionTime": mustRoundTripTime(t, readyTransitionTime).Format(time.RFC3339),
					"reason":             "WaitingForFoo",
					"message":            "waiting for foo to complete",
				},
			},
		},
		{
			name: "a Ready condition round-tripped from a []metav1.Condition payload has no severity",
			in: mustUnmarshalConditions(t, []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 5,
					LastTransitionTime: readyTransitionTime,
					Reason:             "AllGood",
					Message:            "all good",
				},
			}),
			expect: []any{
				map[string]any{
					"type":               "Ready",
					"status":             "True",
					"observedGeneration": int64(5),
					"lastTransitionTime": mustRoundTripTime(t, readyTransitionTime).Format(time.RFC3339),
					"reason":             "AllGood",
					"message":            "all good",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := tt.in.ReadyConditionAsAnyArray()

			g.Expect(got).To(Equal(tt.expect))
		})
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal %v: %v", v, err)
	}
	return b
}

// mustRoundTripTime marshals and unmarshals a metav1.Time, mirroring what happens to
// LastTransitionTime when a Conditions payload goes through Conditions.UnmarshalJSON.
// metav1.Time.UnmarshalJSON converts the parsed time to Local, so a UTC input built by
// the test does not compare equal to the round-tripped value unless both go through this.
func mustRoundTripTime(t *testing.T, in metav1.Time) metav1.Time {
	t.Helper()
	var out metav1.Time
	if err := json.Unmarshal(mustMarshal(t, in), &out); err != nil {
		t.Fatalf("failed to round-trip time %v: %v", in, err)
	}
	return out
}

// mustUnmarshalConditions marshals v (e.g. a clusterv1beta1.Conditions or []metav1.Condition)
// and unmarshals the result into a Conditions, exercising the real Conditions.UnmarshalJSON path.
func mustUnmarshalConditions(t *testing.T, v any) Conditions {
	t.Helper()
	var out Conditions
	if err := out.UnmarshalJSON(mustMarshal(t, v)); err != nil {
		t.Fatalf("failed to unmarshal conditions: %v", err)
	}
	return out
}
