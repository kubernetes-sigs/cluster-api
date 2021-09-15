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
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestNewPatch(t *testing.T) {
	fooTrue := TrueCondition("foo")
	fooFalse := FalseCondition("foo", "reason foo", clusterv1.ConditionSeverityInfo, "message foo")

	tests := []struct {
		name   string
		before Getter
		after  Getter
		want   Patch
	}{
		{
			name:   "No changes return empty patch",
			before: getterWithConditions(),
			after:  getterWithConditions(),
			want:   nil,
		},
		{
			name:   "No changes return empty patch",
			before: getterWithConditions(fooTrue),
			after:  getterWithConditions(fooTrue),
			want:   nil,
		},
		{
			name:   "Detects AddConditionPatch",
			before: getterWithConditions(),
			after:  getterWithConditions(fooTrue),
			want: Patch{
				{
					Before: nil,
					After:  fooTrue,
					Op:     AddConditionPatch,
				},
			},
		},
		{
			name:   "Detects ChangeConditionPatch",
			before: getterWithConditions(fooTrue),
			after:  getterWithConditions(fooFalse),
			want: Patch{
				{
					Before: fooTrue,
					After:  fooFalse,
					Op:     ChangeConditionPatch,
				},
			},
		},
		{
			name:   "Detects RemoveConditionPatch",
			before: getterWithConditions(fooTrue),
			after:  getterWithConditions(),
			want: Patch{
				{
					Before: fooTrue,
					After:  nil,
					Op:     RemoveConditionPatch,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := NewPatch(tt.before, tt.after)

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestApply(t *testing.T) {
	fooTrue := TrueCondition("foo")
	fooFalse := FalseCondition("foo", "reason foo", clusterv1.ConditionSeverityInfo, "message foo")
	fooWarning := FalseCondition("foo", "reason foo", clusterv1.ConditionSeverityWarning, "message foo")

	tests := []struct {
		name    string
		before  Getter
		after   Getter
		latest  Setter
		options []ApplyOption
		want    clusterv1.Conditions
		wantErr bool
	}{
		{
			name:    "No patch return same list",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(fooTrue),
			latest:  setterWithConditions(fooTrue),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Add: When a condition does not exists, it should add",
			before:  getterWithConditions(),
			after:   getterWithConditions(fooTrue),
			latest:  setterWithConditions(),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Add: When a condition already exists but without conflicts, it should add",
			before:  getterWithConditions(),
			after:   getterWithConditions(fooTrue),
			latest:  setterWithConditions(fooTrue),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Add: When a condition already exists but with conflicts, it should error",
			before:  getterWithConditions(),
			after:   getterWithConditions(fooTrue),
			latest:  setterWithConditions(fooFalse),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Add: When a condition already exists but with conflicts, it should not error if the condition is owned",
			before:  getterWithConditions(),
			after:   getterWithConditions(fooTrue),
			latest:  setterWithConditions(fooFalse),
			options: []ApplyOption{WithOwnedConditions("foo")},
			want:    conditionList(fooTrue), // after condition should be kept in case of error
			wantErr: false,
		},
		{
			name:    "Remove: When a condition was already deleted, it should pass",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(),
			latest:  setterWithConditions(),
			want:    conditionList(),
			wantErr: false,
		},
		{
			name:    "Remove: When a condition already exists but without conflicts, it should delete",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(),
			latest:  setterWithConditions(fooTrue),
			want:    conditionList(),
			wantErr: false,
		},
		{
			name:    "Remove: When a condition already exists but with conflicts, it should error",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(),
			latest:  setterWithConditions(fooFalse),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Remove: When a condition already exists but with conflicts, it should not error if the condition is owned",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(),
			latest:  setterWithConditions(fooFalse),
			options: []ApplyOption{WithOwnedConditions("foo")},
			want:    conditionList(), // after condition should be kept in case of error
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists without conflicts, it should change",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(fooFalse),
			latest:  setterWithConditions(fooTrue),
			want:    conditionList(fooFalse),
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is agreement on the final state, it should change",
			before:  getterWithConditions(fooFalse),
			after:   getterWithConditions(fooTrue),
			latest:  setterWithConditions(fooTrue),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is no agreement on the final state, it should error",
			before:  getterWithConditions(fooWarning),
			after:   getterWithConditions(fooFalse),
			latest:  setterWithConditions(fooTrue),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is no agreement on the final state, it should not error if the condition is owned",
			before:  getterWithConditions(fooWarning),
			after:   getterWithConditions(fooFalse),
			latest:  setterWithConditions(fooTrue),
			options: []ApplyOption{WithOwnedConditions("foo")},
			want:    conditionList(fooFalse), // after condition should be kept in case of error
			wantErr: false,
		},
		{
			name:    "Change: When a condition was deleted, it should error",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(fooFalse),
			latest:  setterWithConditions(),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Change: When a condition was deleted, it should not error if the condition is owned",
			before:  getterWithConditions(fooTrue),
			after:   getterWithConditions(fooFalse),
			latest:  setterWithConditions(),
			options: []ApplyOption{WithOwnedConditions("foo")},
			want:    conditionList(fooFalse), // after condition should be kept in case of error
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			patch := NewPatch(tt.before, tt.after)

			err := patch.Apply(tt.latest, tt.options...)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(tt.latest.GetConditions()).To(haveSameConditionsOf(tt.want))
		})
	}
}

func TestApplyDoesNotAlterLastTransitionTime(t *testing.T) {
	g := NewWithT(t)

	before := &clusterv1.Cluster{}
	after := &clusterv1.Cluster{
		Status: clusterv1.ClusterStatus{
			Conditions: clusterv1.Conditions{
				clusterv1.Condition{
					Type:               "foo",
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Now().UTC().Truncate(time.Second)),
				},
			},
		},
	}
	latest := &clusterv1.Cluster{}

	// latest has no conditions, so we are actually adding the condition but in this case we should not set the LastTransition Time
	// but we should preserve the LastTransition set in after

	diff := NewPatch(before, after)
	err := diff.Apply(latest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(latest.GetConditions()).To(Equal(after.GetConditions()))
}
