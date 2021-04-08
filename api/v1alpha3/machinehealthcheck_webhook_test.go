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
	"time"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utildefaulting "sigs.k8s.io/cluster-api/util/defaulting"
)

func TestMachineHealthCheckDefault(t *testing.T) {
	g := NewWithT(t)
	mhc := &MachineHealthCheck{
		Spec: MachineHealthCheckSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
		},
	}
	t.Run("for MachineHealthCheck", utildefaulting.DefaultValidateTest(mhc))
	mhc.Default()

	g.Expect(mhc.Labels[ClusterLabelName]).To(Equal(mhc.Spec.ClusterName))
	g.Expect(mhc.Spec.MaxUnhealthy.String()).To(Equal("100%"))
	g.Expect(mhc.Spec.NodeStartupTimeout).ToNot(BeNil())
	g.Expect(*mhc.Spec.NodeStartupTimeout).To(Equal(metav1.Duration{Duration: 10 * time.Minute}))
}

func TestMachineHealthCheckLabelSelectorAsSelectorValidation(t *testing.T) {
	tests := []struct {
		name      string
		selectors map[string]string
		expectErr bool
	}{
		{
			name:      "should not return error for valid selector",
			selectors: map[string]string{"foo": "bar"},
			expectErr: false,
		},
		{
			name:      "should return error for invalid selector",
			selectors: map[string]string{"-123-foo": "bar"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mhc := &MachineHealthCheck{
				Spec: MachineHealthCheckSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: tt.selectors,
					},
				},
			}
			if tt.expectErr {
				g.Expect(mhc.ValidateCreate()).NotTo(Succeed())
				g.Expect(mhc.ValidateUpdate(mhc)).NotTo(Succeed())
			} else {
				g.Expect(mhc.ValidateCreate()).To(Succeed())
				g.Expect(mhc.ValidateUpdate(mhc)).To(Succeed())
			}
		})
	}
}

func TestMachineHealthCheckClusterNameImmutable(t *testing.T) {
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

			newMHC := &MachineHealthCheck{
				Spec: MachineHealthCheckSpec{
					ClusterName: tt.newClusterName,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
				},
			}
			oldMHC := &MachineHealthCheck{
				Spec: MachineHealthCheckSpec{
					ClusterName: tt.oldClusterName,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
				},
			}

			if tt.expectErr {
				g.Expect(newMHC.ValidateUpdate(oldMHC)).NotTo(Succeed())
			} else {
				g.Expect(newMHC.ValidateUpdate(oldMHC)).To(Succeed())
			}
		})
	}
}

func TestMachineHealthCheckNodeStartupTimeout(t *testing.T) {
	zero := metav1.Duration{Duration: 0}
	twentyNineSeconds := metav1.Duration{Duration: 29 * time.Second}
	thirtySeconds := metav1.Duration{Duration: 30 * time.Second}
	oneMinute := metav1.Duration{Duration: 1 * time.Minute}
	minusOneMinute := metav1.Duration{Duration: -1 * time.Minute}

	tests := []struct {
		name      string
		timeout   *metav1.Duration
		expectErr bool
	}{
		{
			name:      "when the nodeStartupTimeout is not given",
			timeout:   nil,
			expectErr: false,
		},
		{
			name:      "when the nodeStartupTimeout is greater than 30s",
			timeout:   &oneMinute,
			expectErr: false,
		},
		{
			name:      "when the nodeStartupTimeout is 30s",
			timeout:   &thirtySeconds,
			expectErr: false,
		},
		{
			name:      "when the nodeStartupTimeout is 29s",
			timeout:   &twentyNineSeconds,
			expectErr: true,
		},
		{
			name:      "when the nodeStartupTimeout is less than 0",
			timeout:   &minusOneMinute,
			expectErr: true,
		},
		{
			name:      "when the nodeStartupTimeout is 0",
			timeout:   &zero,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		g := NewWithT(t)

		mhc := &MachineHealthCheck{
			Spec: MachineHealthCheckSpec{
				NodeStartupTimeout: tt.timeout,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
			},
		}

		if tt.expectErr {
			g.Expect(mhc.ValidateCreate()).NotTo(Succeed())
			g.Expect(mhc.ValidateUpdate(mhc)).NotTo(Succeed())
		} else {
			g.Expect(mhc.ValidateCreate()).To(Succeed())
			g.Expect(mhc.ValidateUpdate(mhc)).To(Succeed())
		}
	}
}

func TestMachineHealthCheckMaxUnhealthy(t *testing.T) {
	tests := []struct {
		name      string
		value     intstr.IntOrString
		expectErr bool
	}{
		{
			name:      "when the value is an integer",
			value:     intstr.Parse("10"),
			expectErr: false,
		},
		{
			name:      "when the value is a percentage",
			value:     intstr.Parse("10%"),
			expectErr: false,
		},
		{
			name:      "when the value is a random string",
			value:     intstr.Parse("abcdef"),
			expectErr: true,
		},
		{
			name:      "when the value stringified integer",
			value:     intstr.FromString("10"),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		g := NewWithT(t)

		maxUnhealthy := tt.value
		mhc := &MachineHealthCheck{
			Spec: MachineHealthCheckSpec{
				MaxUnhealthy: &maxUnhealthy,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
			},
		}

		if tt.expectErr {
			g.Expect(mhc.ValidateCreate()).NotTo(Succeed())
			g.Expect(mhc.ValidateUpdate(mhc)).NotTo(Succeed())
		} else {
			g.Expect(mhc.ValidateCreate()).To(Succeed())
			g.Expect(mhc.ValidateUpdate(mhc)).To(Succeed())
		}
	}
}

func TestMachineHealthCheckSelectorValidation(t *testing.T) {
	g := NewWithT(t)
	mhc := &MachineHealthCheck{}
	err := mhc.validate(nil)
	g.Expect(err).ToNot(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("selector must not be empty"))
}

func TestMachineHealthCheckClusterNameSelectorValidation(t *testing.T) {
	g := NewWithT(t)
	mhc := &MachineHealthCheck{
		Spec: MachineHealthCheckSpec{
			ClusterName: "foo",
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					ClusterLabelName: "bar",
					"baz":            "qux",
				},
			},
		},
	}
	err := mhc.validate(nil)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cannot specify a cluster selector other than the one specified by ClusterName"))

	mhc.Spec.Selector.MatchLabels[ClusterLabelName] = "foo"
	g.Expect(mhc.validate(nil)).To(Succeed())
	delete(mhc.Spec.Selector.MatchLabels, ClusterLabelName)
	g.Expect(mhc.validate(nil)).To(Succeed())
}
