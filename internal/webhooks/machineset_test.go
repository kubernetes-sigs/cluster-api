/*
Copyright 2022 The Kubernetes Authors.

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

package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

func TestMachineSetDefault(t *testing.T) {
	g := NewWithT(t)
	ms := &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ms",
		},
		Spec: clusterv1.MachineSetSpec{
			Template: clusterv1.MachineTemplateSpec{
				Spec: clusterv1.MachineSpec{
					Version: pointer.String("1.19.10"),
				},
			},
		},
	}
	webhook := &MachineSet{}

	t.Run("for MachineSet", util.CustomDefaultValidateTest(ctx, ms, webhook))
	g.Expect(webhook.Default(ctx, ms)).To(Succeed())
	g.Expect(ms.Labels[clusterv1.ClusterLabelName]).To(Equal(ms.Spec.ClusterName))
	g.Expect(ms.Spec.DeletePolicy).To(Equal(string(clusterv1.RandomMachineSetDeletePolicy)))
	g.Expect(ms.Spec.Selector.MatchLabels).To(HaveKeyWithValue(clusterv1.MachineSetLabelName, "test-ms"))
	g.Expect(ms.Spec.Template.Labels).To(HaveKeyWithValue(clusterv1.MachineSetLabelName, "test-ms"))
	g.Expect(*ms.Spec.Template.Spec.Version).To(Equal("v1.19.10"))
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
			ms := &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: tt.selectors,
					},
					Template: clusterv1.MachineTemplateSpec{
						ObjectMeta: clusterv1.ObjectMeta{
							Labels: tt.labels,
						},
					},
				},
			}
			secondMS := ms.DeepCopy()
			webhook := &MachineSet{}

			if tt.expectErr {
				g.Expect(webhook.ValidateCreate(ctx, ms)).NotTo(Succeed())
				g.Expect(webhook.ValidateUpdate(ctx, secondMS, ms)).NotTo(Succeed())
			} else {
				g.Expect(webhook.ValidateCreate(ctx, ms)).To(Succeed())
				g.Expect(webhook.ValidateUpdate(ctx, secondMS, ms)).To(Succeed())
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
			ms := &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					ClusterName: tt.newClusterName,
				},
			}
			secondMS := &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					ClusterName: tt.oldClusterName,
				},
			}
			webhook := &MachineSet{}

			if tt.expectErr {
				g.Expect(webhook.ValidateUpdate(ctx, ms, secondMS)).NotTo(Succeed())
			} else {
				g.Expect(webhook.ValidateUpdate(ctx, ms, secondMS)).To(Succeed())
			}
		})
	}
}

func TestMachineSetVersionValidation(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		expectErr bool
	}{
		{
			name:      "should succeed when given a valid semantic version with prepended 'v'",
			version:   "v1.19.2",
			expectErr: false,
		},
		{
			name:      "should return error when given a valid semantic version without 'v'",
			version:   "1.19.2",
			expectErr: true,
		},
		{
			name:      "should return error when given an invalid semantic version",
			version:   "1",
			expectErr: true,
		},
		{
			name:      "should return error when given an invalid semantic version",
			version:   "v1",
			expectErr: true,
		},
		{
			name:      "should return error when given an invalid semantic version",
			version:   "wrong_version",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ms := &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Template: clusterv1.MachineTemplateSpec{
						Spec: clusterv1.MachineSpec{
							Version: pointer.String(tt.version),
						},
					},
				},
			}
			secondMS := ms.DeepCopy()
			webhook := &MachineSet{}

			if tt.expectErr {
				g.Expect(webhook.ValidateCreate(ctx, ms)).NotTo(Succeed())
				g.Expect(webhook.ValidateUpdate(ctx, ms, secondMS)).NotTo(Succeed())
			} else {
				g.Expect(webhook.ValidateCreate(ctx, ms)).To(Succeed())
				g.Expect(webhook.ValidateUpdate(ctx, ms, secondMS)).To(Succeed())
			}
		})
	}
}
