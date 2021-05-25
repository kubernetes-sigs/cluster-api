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

package v1alpha4

import (
	"testing"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestMachineDeploymentDefault(t *testing.T) {
	g := NewWithT(t)
	md := &MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-md",
		},
		Spec: MachineDeploymentSpec{
			Template: MachineTemplateSpec{
				Spec: MachineSpec{
					Version: pointer.String("1.19.10"),
				},
			},
		},
	}
	t.Run("for MachineDeployment", utildefaulting.DefaultValidateTest(md))
	md.Default()

	g.Expect(md.Labels[ClusterLabelName]).To(Equal(md.Spec.ClusterName))
	g.Expect(md.Spec.MinReadySeconds).To(Equal(pointer.Int32Ptr(0)))
	g.Expect(md.Spec.RevisionHistoryLimit).To(Equal(pointer.Int32Ptr(1)))
	g.Expect(md.Spec.ProgressDeadlineSeconds).To(Equal(pointer.Int32Ptr(600)))
	g.Expect(md.Spec.Strategy).ToNot(BeNil())
	g.Expect(md.Spec.Selector.MatchLabels).To(HaveKeyWithValue(MachineDeploymentLabelName, "test-md"))
	g.Expect(md.Spec.Template.Labels).To(HaveKeyWithValue(MachineDeploymentLabelName, "test-md"))
	g.Expect(md.Spec.Strategy.Type).To(Equal(RollingUpdateMachineDeploymentStrategyType))
	g.Expect(md.Spec.Strategy.RollingUpdate).ToNot(BeNil())
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()).To(Equal(1))
	g.Expect(md.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(0))
	g.Expect(*md.Spec.Template.Spec.Version).To(Equal("v1.19.10"))
}

func TestMachineDeploymentValidation(t *testing.T) {
	badMaxSurge := intstr.FromString("1")
	badMaxUnavailable := intstr.FromString("0")

	goodMaxSurgePercentage := intstr.FromString("1%")
	goodMaxUnavailablePercentage := intstr.FromString("0%")

	goodMaxSurgeInt := intstr.FromInt(1)
	goodMaxUnavailableInt := intstr.FromInt(0)

	tests := []struct {
		name      string
		selectors map[string]string
		labels    map[string]string
		strategy  MachineDeploymentStrategy
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
		{
			name:      "should return error for invalid maxSurge",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
					MaxUnavailable: &goodMaxUnavailableInt,
					MaxSurge:       &badMaxSurge,
				},
			},
			expectErr: true,
		},
		{
			name:      "should return error for invalid maxUnavailable",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
					MaxUnavailable: &badMaxUnavailable,
					MaxSurge:       &goodMaxSurgeInt,
				},
			},
			expectErr: true,
		},
		{
			name:      "should not return error for valid int maxSurge and maxUnavailable",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
					MaxUnavailable: &goodMaxUnavailableInt,
					MaxSurge:       &goodMaxSurgeInt,
				},
			},
			expectErr: false,
		},
		{
			name:      "should not return error for valid percentage string maxSurge and maxUnavailable",
			selectors: map[string]string{"foo": "bar"},
			labels:    map[string]string{"foo": "bar"},
			strategy: MachineDeploymentStrategy{
				Type: RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &MachineRollingUpdateDeployment{
					MaxUnavailable: &goodMaxUnavailablePercentage,
					MaxSurge:       &goodMaxSurgePercentage,
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			md := &MachineDeployment{
				Spec: MachineDeploymentSpec{
					Strategy: &tt.strategy,
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
				g.Expect(md.ValidateCreate()).NotTo(Succeed())
				g.Expect(md.ValidateUpdate(md)).NotTo(Succeed())
			} else {
				g.Expect(md.ValidateCreate()).To(Succeed())
				g.Expect(md.ValidateUpdate(md)).To(Succeed())
			}
		})
	}
}

func TestMachineDeploymentVersionValidation(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		expectErr bool
	}{
		{
			name:      "should succeed when given a valid semantic version with prepended 'v'",
			version:   "v1.17.2",
			expectErr: false,
		},
		{
			name:      "should return error when given a valid semantic version without 'v'",
			version:   "1.17.2",
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

			md := &MachineDeployment{
				Spec: MachineDeploymentSpec{

					Template: MachineTemplateSpec{
						Spec: MachineSpec{
							Version: pointer.String(tt.version),
						},
					},
				},
			}

			if tt.expectErr {
				g.Expect(md.ValidateCreate()).NotTo(Succeed())
				g.Expect(md.ValidateUpdate(md)).NotTo(Succeed())
			} else {
				g.Expect(md.ValidateCreate()).To(Succeed())
				g.Expect(md.ValidateUpdate(md)).To(Succeed())
			}
		})
	}
}

func TestMachineDeploymentWithSpec(t *testing.T) {
	g := NewWithT(t)
	md := MachineDeployment{
		Spec: MachineDeploymentSpec{
			ClusterName: "test-cluster",
			Template: MachineTemplateSpec{
				Spec: MachineSpec{
					ClusterName: "test-cluster",
				},
			},
		},
	}

	md.Default()
	g.Expect(md.Spec.Selector.MatchLabels).To(HaveKeyWithValue(ClusterLabelName, "test-cluster"))
	g.Expect(md.Spec.Template.Labels).To(HaveKeyWithValue(ClusterLabelName, "test-cluster"))
}

func TestMachineDeploymentClusterNameImmutable(t *testing.T) {
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

			newMD := &MachineDeployment{
				Spec: MachineDeploymentSpec{
					ClusterName: tt.newClusterName,
				},
			}

			oldMD := &MachineDeployment{
				Spec: MachineDeploymentSpec{
					ClusterName: tt.oldClusterName,
				},
			}

			if tt.expectErr {
				g.Expect(newMD.ValidateUpdate(oldMD)).NotTo(Succeed())
			} else {
				g.Expect(newMD.ValidateUpdate(oldMD)).To(Succeed())
			}
		})
	}
}
