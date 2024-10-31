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

	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestNewPatch(t *testing.T) {
	now := metav1.Now()
	fooTrue := metav1.Condition{Type: "foo", Status: metav1.ConditionTrue, LastTransitionTime: now}
	fooFalse := metav1.Condition{Type: "foo", Status: metav1.ConditionFalse, LastTransitionTime: now}

	tests := []struct {
		name    string
		before  Setter
		after   Setter
		want    Patch
		wantErr bool
	}{
		{
			name:    "nil before return error",
			before:  nil,
			after:   objectWithConditions(),
			wantErr: true,
		},
		{
			name:    "nil after return error",
			before:  objectWithConditions(),
			after:   nil,
			wantErr: true,
		},
		{
			name:    "nil Interface before return error",
			before:  nilObject(),
			after:   objectWithConditions(),
			wantErr: true,
		},
		{
			name:    "nil Interface after return error",
			before:  objectWithConditions(),
			after:   nilObject(),
			wantErr: true,
		},
		{
			name:    "No changes return empty patch",
			before:  objectWithConditions(),
			after:   objectWithConditions(),
			want:    nil,
			wantErr: false,
		},

		{
			name:   "No changes return empty patch",
			before: objectWithConditions(fooTrue),
			after:  objectWithConditions(fooTrue),
			want:   nil,
		},
		{
			name:   "Detects AddConditionPatch",
			before: objectWithConditions(),
			after:  objectWithConditions(fooTrue),
			want: Patch{
				{
					Before: nil,
					After:  &fooTrue,
					Op:     AddConditionPatch,
				},
			},
		},
		{
			name:   "Detects ChangeConditionPatch",
			before: objectWithConditions(fooTrue),
			after:  objectWithConditions(fooFalse),
			want: Patch{
				{
					Before: &fooTrue,
					After:  &fooFalse,
					Op:     ChangeConditionPatch,
				},
			},
		},
		{
			name:   "Detects RemoveConditionPatch",
			before: objectWithConditions(fooTrue),
			after:  objectWithConditions(),
			want: Patch{
				{
					Before: &fooTrue,
					After:  nil,
					Op:     RemoveConditionPatch,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := NewPatch(tt.before, tt.after)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).To(Not(HaveOccurred()))
			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}

func TestApply(t *testing.T) {
	now := metav1.Now().Rfc3339Copy()
	fooTrue := metav1.Condition{Type: "foo", Status: metav1.ConditionTrue, LastTransitionTime: now}
	fooFalse := metav1.Condition{Type: "foo", Status: metav1.ConditionFalse, LastTransitionTime: now}
	fooFalse2 := metav1.Condition{Type: "foo", Status: metav1.ConditionFalse, Reason: "Something else", LastTransitionTime: now}

	addMilliseconds := func(c metav1.Condition) metav1.Condition {
		c1 := c.DeepCopy()
		c1.LastTransitionTime.Time = c1.LastTransitionTime.Add(10 * time.Millisecond)
		return *c1
	}

	tests := []struct {
		name    string
		before  Setter
		after   Setter
		latest  Setter
		options []PatchApplyOption
		want    []metav1.Condition
		wantErr bool
	}{
		{
			name:    "error with nil interface object",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(fooFalse),
			latest:  nilObject(),
			want:    []metav1.Condition{fooTrue},
			wantErr: true,
		},
		{
			name:    "error with nil latest object",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(fooFalse),
			latest:  nil,
			want:    []metav1.Condition{fooTrue},
			wantErr: true,
		},
		{
			name:    "No patch return same list",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(fooTrue),
			latest:  objectWithConditions(fooTrue),
			want:    []metav1.Condition{fooTrue},
			wantErr: false,
		},
		{
			name:    "Add: When a condition does not exists, it should add",
			before:  objectWithConditions(),
			after:   objectWithConditions(addMilliseconds(fooTrue)), // this will force the test to fail if an AddConditionPatch operation doesn't drop milliseconds
			latest:  objectWithConditions(),
			want:    []metav1.Condition{fooTrue},
			wantErr: false,
		},
		{
			name:    "Add: When a condition already exists but without conflicts, it should add",
			before:  objectWithConditions(),
			after:   objectWithConditions(fooTrue),
			latest:  objectWithConditions(fooTrue),
			want:    []metav1.Condition{fooTrue},
			wantErr: false,
		},
		{
			name:    "Add: When a condition already exists but with conflicts, it should error",
			before:  objectWithConditions(),
			after:   objectWithConditions(fooTrue),
			latest:  objectWithConditions(fooFalse),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Add: When a condition already exists but with conflicts, it should not error if force override is set",
			before:  objectWithConditions(),
			after:   objectWithConditions(addMilliseconds(fooTrue)), // this will force the test to fail if an AddConditionPatch operation doesn't drop milliseconds
			latest:  objectWithConditions(fooFalse),
			options: []PatchApplyOption{ForceOverwrite(true)},
			want:    []metav1.Condition{fooTrue}, // after condition should be kept in case of error
			wantErr: false,
		},
		{
			name:    "Add: When a condition already exists but with conflicts, it should not error if the condition is owned",
			before:  objectWithConditions(),
			after:   objectWithConditions(addMilliseconds(fooTrue)), // this will force the test to fail if an AddConditionPatch operation doesn't drop milliseconds
			latest:  objectWithConditions(fooFalse),
			options: []PatchApplyOption{OwnedConditionTypes{"foo"}},
			want:    []metav1.Condition{fooTrue}, // after condition should be kept in case of error
			wantErr: false,
		},
		{
			name:    "Remove: When a condition was already deleted, it should pass",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(),
			latest:  objectWithConditions(),
			want:    []metav1.Condition{},
			wantErr: false,
		},
		{
			name:    "Remove: When a condition already exists but without conflicts, it should delete",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(),
			latest:  objectWithConditions(fooTrue),
			want:    []metav1.Condition{},
			wantErr: false,
		},
		{
			name:    "Remove: When a condition already exists but with conflicts, it should error",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(),
			latest:  objectWithConditions(fooFalse),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Remove: When a condition already exists but with conflicts, it should not error if force override is set",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(),
			latest:  objectWithConditions(fooFalse),
			options: []PatchApplyOption{ForceOverwrite(true)},
			want:    []metav1.Condition{},
			wantErr: false,
		},
		{
			name:    "Remove: When a condition already exists but with conflicts, it should not error if the condition is owned",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(),
			latest:  objectWithConditions(fooFalse),
			options: []PatchApplyOption{OwnedConditionTypes{"foo"}},
			want:    []metav1.Condition{},
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists without conflicts, it should change",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(addMilliseconds(fooFalse)), // this will force the test to fail if an ChangeConditionPatch operation doesn't drop milliseconds
			latest:  objectWithConditions(fooTrue),
			want:    []metav1.Condition{fooFalse},
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is agreement on the final state, it should change",
			before:  objectWithConditions(fooFalse),
			after:   objectWithConditions(fooTrue),
			latest:  objectWithConditions(fooTrue),
			want:    []metav1.Condition{fooTrue},
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is no agreement on the final state, it should error",
			before:  objectWithConditions(fooFalse),
			after:   objectWithConditions(fooFalse2),
			latest:  objectWithConditions(fooTrue),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is no agreement on the final state, it should not error if force override is set",
			before:  objectWithConditions(fooFalse),
			after:   objectWithConditions(addMilliseconds(fooFalse2)), // this will force the test to fail if an ChangeConditionPatch operation doesn't drop milliseconds
			latest:  objectWithConditions(fooTrue),
			options: []PatchApplyOption{ForceOverwrite(true)},
			want:    []metav1.Condition{fooFalse2},
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is no agreement on the final state, it should not error if the condition is owned",
			before:  objectWithConditions(fooFalse),
			after:   objectWithConditions(addMilliseconds(fooFalse2)), // this will force the test to fail if an ChangeConditionPatch operation doesn't drop milliseconds
			latest:  objectWithConditions(fooTrue),
			options: []PatchApplyOption{OwnedConditionTypes{"foo"}},
			want:    []metav1.Condition{fooFalse2},
			wantErr: false,
		},
		{
			name:    "Change: When a condition was deleted, it should error",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(fooFalse),
			latest:  objectWithConditions(),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Change: When a condition was deleted, it should not error if force override is set",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(fooFalse),
			latest:  objectWithConditions(),
			options: []PatchApplyOption{ForceOverwrite(true)},
			want:    []metav1.Condition{fooFalse},
			wantErr: false,
		},
		{
			name:    "Change: When a condition was deleted, it should not error if the condition is owned",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(fooFalse),
			latest:  objectWithConditions(),
			options: []PatchApplyOption{OwnedConditionTypes{"foo"}},
			want:    []metav1.Condition{fooFalse},
			wantErr: false,
		},
		{
			name:    "Error when nil passed as an ApplyOption",
			before:  objectWithConditions(fooTrue),
			after:   objectWithConditions(fooFalse),
			latest:  objectWithConditions(),
			options: []PatchApplyOption{nil},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Ignore the error here to allow testing of patch.Apply with a nil patch
			patch, _ := NewPatch(tt.before, tt.after)

			err := patch.Apply(tt.latest, tt.options...)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			gotConditions := tt.latest.GetV1Beta2Conditions()
			g.Expect(gotConditions).To(MatchConditions(tt.want))
		})
	}
}

func objectWithConditions(conditions ...metav1.Condition) Setter {
	obj := &builder.Phase3Obj{}
	obj.Status.Conditions = conditions
	return obj
}

func nilObject() Setter {
	var obj *builder.Phase3Obj
	return obj
}
