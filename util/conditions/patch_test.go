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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestNewPatch(t *testing.T) {
	fooTrue := TrueCondition("foo")
	fooFalse := FalseCondition("foo", "reason foo", clusterv1.ConditionSeverityInfo, "message foo")

	tests := []struct {
		name   string
		base   Getter
		target Getter
		want   Patch
	}{
		{
			name:   "No changes return empty patch",
			base:   getterWithConditions(),
			target: getterWithConditions(),
			want:   nil,
		},
		{
			name:   "No changes return empty patch",
			base:   getterWithConditions(fooTrue),
			target: getterWithConditions(fooTrue),
			want:   nil,
		},
		{
			name:   "Detects AddConditionPatch",
			base:   getterWithConditions(),
			target: getterWithConditions(fooTrue),
			want: Patch{
				{
					Base:   nil,
					Target: fooTrue,
					Op:     AddConditionPatch,
				},
			},
		},
		{
			name:   "Detects ChangeConditionPatch",
			base:   getterWithConditions(fooTrue),
			target: getterWithConditions(fooFalse),
			want: Patch{
				{
					Base:   fooTrue,
					Target: fooFalse,
					Op:     ChangeConditionPatch,
				},
			},
		},
		{
			name:   "Detects RemoveConditionPatch",
			base:   getterWithConditions(fooTrue),
			target: getterWithConditions(),
			want: Patch{
				{
					Base:   fooTrue,
					Target: nil,
					Op:     RemoveConditionPatch,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := NewPatch(tt.base, tt.target)

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
		base    Getter
		target  Getter
		source  Setter
		want    clusterv1.Conditions
		wantErr bool
	}{
		{
			name:    "No patch return same list",
			base:    getterWithConditions(fooTrue),
			target:  getterWithConditions(fooTrue),
			source:  setterWithConditions(fooTrue),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Add: When a condition does not exists, it should add",
			base:    getterWithConditions(),
			target:  getterWithConditions(fooTrue),
			source:  setterWithConditions(),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Add: When a condition already exists but without conflicts, it should add",
			base:    getterWithConditions(),
			target:  getterWithConditions(fooTrue),
			source:  setterWithConditions(fooTrue),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Add: When a condition already exists but with conflicts, it should error",
			base:    getterWithConditions(),
			target:  getterWithConditions(fooTrue),
			source:  setterWithConditions(fooFalse),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Remove: When a condition was already deleted, it should pass",
			base:    getterWithConditions(fooTrue),
			target:  getterWithConditions(),
			source:  setterWithConditions(),
			want:    conditionList(),
			wantErr: false,
		},
		{
			name:    "Remove: When a condition already exists but without conflicts, it should delete",
			base:    getterWithConditions(fooTrue),
			target:  getterWithConditions(),
			source:  setterWithConditions(fooTrue),
			want:    conditionList(),
			wantErr: false,
		},
		{
			name:    "Remove: When a condition already exists but with conflicts, it should error",
			base:    getterWithConditions(fooTrue),
			target:  getterWithConditions(),
			source:  setterWithConditions(fooFalse),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Change: When a condition exists without conflicts, it should change",
			base:    getterWithConditions(fooTrue),
			target:  getterWithConditions(fooFalse),
			source:  setterWithConditions(fooTrue),
			want:    conditionList(fooFalse),
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is agreement on the final state, it should change",
			base:    getterWithConditions(fooFalse),
			target:  getterWithConditions(fooTrue),
			source:  setterWithConditions(fooTrue),
			want:    conditionList(fooTrue),
			wantErr: false,
		},
		{
			name:    "Change: When a condition exists with conflicts but there is no agreement on the final state, it should error",
			base:    getterWithConditions(fooWarning),
			target:  getterWithConditions(fooFalse),
			source:  setterWithConditions(fooTrue),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Change: When a condition was deleted, it should error",
			base:    getterWithConditions(fooTrue),
			target:  getterWithConditions(fooFalse),
			source:  setterWithConditions(),
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			patch := NewPatch(tt.base, tt.target)

			err := patch.Apply(tt.source)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(tt.source.GetConditions()).To(haveSameConditionsOf(tt.want))
		})
	}
}
