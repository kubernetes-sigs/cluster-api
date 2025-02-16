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

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMirrorStatusCondition(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()
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
			want:          metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "We are good!", LastTransitionTime: now},
		},
		{
			name: "Mirror a condition with target type",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "We are good!", ObservedGeneration: 10, LastTransitionTime: now},
			},
			conditionType: "Ready",
			options:       []MirrorOption{TargetConditionType("SomethingReady")},
			want:          metav1.Condition{Type: "SomethingReady", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "We are good!", LastTransitionTime: now},
		},
		{
			name: "Mirror a condition with empty message",
			conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "", ObservedGeneration: 10, LastTransitionTime: now},
			},
			conditionType: "Ready",
			options:       []MirrorOption{},
			want:          metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllGood!", Message: "", LastTransitionTime: now},
		},
		{
			name:          "Mirror a condition not yet reported",
			conditions:    []metav1.Condition{},
			conditionType: "Ready",
			options:       []MirrorOption{},
			want:          metav1.Condition{Type: "Ready", Status: metav1.ConditionUnknown, Reason: NotYetReportedReason, Message: "Condition Ready not yet reported"},
		},
		{
			name:          "Mirror a condition not yet reported with target type",
			conditions:    []metav1.Condition{},
			conditionType: "Ready",
			options:       []MirrorOption{TargetConditionType("SomethingReady")},
			want:          metav1.Condition{Type: "SomethingReady", Status: metav1.ConditionUnknown, Reason: NotYetReportedReason, Message: "Condition Ready not yet reported"},
		},
		{
			name:          "Mirror a condition not yet reported with a fallback condition",
			conditions:    []metav1.Condition{},
			conditionType: "Ready",
			options: []MirrorOption{
				FallbackCondition{
					Status:  BoolToStatus(true),
					Reason:  "SomeReason",
					Message: "Foo",
				},
			},
			want: metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "SomeReason", Message: "Foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obj := &builder.Phase3Obj{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: metav1.NamespaceDefault,
					Name:      "SourceObject",
				},
				Status: builder.Phase3ObjStatus{
					Conditions: tt.conditions,
				},
			}

			got := NewMirrorCondition(obj, tt.conditionType, tt.options...)
			g.Expect(got).ToNot(BeNil())
			g.Expect(*got).To(Equal(tt.want))
		})
	}
}
