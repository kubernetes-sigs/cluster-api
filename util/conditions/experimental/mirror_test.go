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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMirrorStatusCondition(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name          string
		conditions    []metav1.Condition
		conditionType string
		options       []MirrorOption
		want          metav1.Condition
	}{
		{
			name: "Mirror a condition",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "We are good!", ObservedGeneration: 10, LastTransitionTime: now},
			},
			conditionType: "Ready",
			options:       []MirrorOption{},
			want:          metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "We are good! (from V1Beta2ResourceWithConditions default/SourceObject)", ObservedGeneration: 10, LastTransitionTime: now},
		},
		{
			name: "Mirror a condition with override type",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "We are good!", ObservedGeneration: 10, LastTransitionTime: now},
			},
			conditionType: "Ready",
			options:       []MirrorOption{OverrideType("SomethingReady")},
			want:          metav1.Condition{Type: "SomethingReady", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "We are good! (from V1Beta2ResourceWithConditions default/SourceObject)", ObservedGeneration: 10, LastTransitionTime: now},
		},
		{
			name: "Mirror a condition with empty message",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "", ObservedGeneration: 10, LastTransitionTime: now},
			},
			conditionType: "Ready",
			options:       []MirrorOption{},
			want:          metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "(from V1Beta2ResourceWithConditions default/SourceObject)", ObservedGeneration: 10, LastTransitionTime: now},
		},
		{
			name:          "Mirror a condition not yet reported",
			conditions:    []metav1.Condition{},
			conditionType: "Ready",
			options:       []MirrorOption{},
			want:          metav1.Condition{Type: "Ready", Status: metav1.ConditionUnknown, Reason: NotYetReportedReason, Message: "Condition Ready not yet reported from V1Beta2ResourceWithConditions default/SourceObject", LastTransitionTime: now},
		},
		{
			name:          "Mirror a condition not yet reported with override type",
			conditions:    []metav1.Condition{},
			conditionType: "Ready",
			options:       []MirrorOption{OverrideType("SomethingReady")},
			want:          metav1.Condition{Type: "SomethingReady", Status: metav1.ConditionUnknown, Reason: NotYetReportedReason, Message: "Condition Ready not yet reported from V1Beta2ResourceWithConditions default/SourceObject", LastTransitionTime: now},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obj := &V1Beta2ResourceWithConditions{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      "SourceObject",
				},
				Status: struct{ Conditions []metav1.Condition }{
					Conditions: tt.conditions,
				},
			}

			got, err := NewMirrorCondition(obj, tt.conditionType, tt.options...)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).ToNot(BeNil())

			// TODO: think about how to improve this; the issue happens when the condition is not yet reported and one gets generated (using time.now) - Option is to make the time overridable
			got.LastTransitionTime = tt.want.LastTransitionTime

			g.Expect(*got).To(Equal(tt.want))
		})
	}
}
