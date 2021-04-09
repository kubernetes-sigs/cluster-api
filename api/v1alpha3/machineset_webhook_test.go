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

package v1alpha3

import (
	"testing"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestMachineSetDefault(t *testing.T) {
	g := NewWithT(t)
	ms := &MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ms",
		},
	}
	t.Run("for MachineSet", utildefaulting.DefaultValidateTest(ms))
	ms.Default()

	g.Expect(ms.Labels[ClusterLabelName]).To(Equal(ms.Spec.ClusterName))
	g.Expect(ms.Spec.DeletePolicy).To(Equal(string(RandomMachineSetDeletePolicy)))
	g.Expect(ms.Spec.Selector.MatchLabels).To(HaveKeyWithValue(MachineSetLabelName, "test-ms"))
	g.Expect(ms.Spec.Template.Labels).To(HaveKeyWithValue(MachineSetLabelName, "test-ms"))
}

func TestMachineSetLabelSelectorMatchValidation(t *testing.T) {
	tests := []struct {
		name      string
		selectors map[string]string
		labels    map[string]string
		expectErr bool
	}{
		{
			name:      "should return error on mismatch",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "baz"},
			expectErr: true,
		},
		{
			name:      "should return error on missing labels",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"": ""},
			expectErr: true,
		},
		{
			name:      "should return error if all selectors don't match",
			selectors: map[string]string{"foo": "bar", "hello": "world"},
			labels:    map[string]string{"foo": "bar"},
			expectErr: true,
		},
		{
			name:      "should not return error on match",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			expectErr: false,
		},
		{
			name:      "should return error for invalid selector",
			selectors: map[string]string{"-123-foo": "bar"},
			labels:    map[string]string{"-123-foo": "bar"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ms := &MachineSet{
				Spec: MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: tt.selectors,
					},
					Template: MachineTemplateSpec{
						ObjectMeta: ObjectMeta{
							Labels: tt.labels,
						},
					},
				},
			}
			if tt.expectErr {
				g.Expect(ms.ValidateCreate()).NotTo(Succeed())
				g.Expect(ms.ValidateUpdate(ms)).NotTo(Succeed())
			} else {
				g.Expect(ms.ValidateCreate()).To(Succeed())
				g.Expect(ms.ValidateUpdate(ms)).To(Succeed())
			}
		})
	}

}

func TestMachineSetClusterNameImmutable(t *testing.T) {
	tests := []struct {
		name           string
		oldClusterName string
		newClusterName string
		expectErr      bool
	}{
		{
			name:           "when the cluster name has not changed",
			oldClusterName: "foo",
			newClusterName: "foo",
			expectErr:      false,
		},
		{
			name:           "when the cluster name has changed",
			oldClusterName: "foo",
			newClusterName: "bar",
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			newMS := &MachineSet{
				Spec: MachineSetSpec{
					ClusterName: tt.newClusterName,
				},
			}

			oldMS := &MachineSet{
				Spec: MachineSetSpec{
					ClusterName: tt.oldClusterName,
				},
			}

			if tt.expectErr {
				g.Expect(newMS.ValidateUpdate(oldMS)).NotTo(Succeed())
			} else {
				g.Expect(newMS.ValidateUpdate(oldMS)).To(Succeed())
			}
		})
	}
}
