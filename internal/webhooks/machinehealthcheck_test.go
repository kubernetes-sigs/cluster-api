/*
Copyright 2021 The Kubernetes Authors.

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
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
)

func TestMachineHealthCheckDefault(t *testing.T) {
	g := NewWithT(t)
	mhc := &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			RemediationTemplate: &corev1.ObjectReference{},
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	webhook := &MachineHealthCheck{}

	t.Run("for MachineHealthCheck", util.CustomDefaultValidateTest(ctx, mhc, webhook))
	g.Expect(webhook.Default(ctx, mhc)).To(Succeed())

	g.Expect(mhc.Labels[clusterv1.ClusterNameLabel]).To(Equal(mhc.Spec.ClusterName))
	g.Expect(mhc.Spec.MaxUnhealthy.String()).To(Equal("100%"))
	g.Expect(mhc.Spec.NodeStartupTimeout).ToNot(BeNil())
	g.Expect(*mhc.Spec.NodeStartupTimeout).To(BeComparableTo(metav1.Duration{Duration: 10 * time.Minute}))
	g.Expect(mhc.Spec.RemediationTemplate.Namespace).To(Equal(mhc.Namespace))
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
			mhc := &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: tt.selectors,
					},
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}
			webhook := &MachineHealthCheck{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, mhc)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, mhc)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
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

			newMHC := &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: tt.newClusterName,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}
			oldMHC := &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					ClusterName: tt.oldClusterName,
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}

			warnings, err := (&MachineHealthCheck{}).ValidateUpdate(ctx, oldMHC, newMHC)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

func TestMachineHealthCheckUnhealthyConditions(t *testing.T) {
	tests := []struct {
		name                string
		unhealthyConditions []clusterv1.UnhealthyCondition
		expectErr           bool
	}{
		{
			name: "pass with correctly defined unhealthyConditions",
			unhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
			expectErr: false,
		},
		{
			name:                "do not fail if the UnhealthyCondition array is nil",
			unhealthyConditions: nil,
			expectErr:           false,
		},
		{
			name:                "do not fail if the UnhealthyCondition array is nil",
			unhealthyConditions: []clusterv1.UnhealthyCondition{},
			expectErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			mhc := &clusterv1.MachineHealthCheck{
				Spec: clusterv1.MachineHealthCheckSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"test": "test",
						},
					},
					UnhealthyConditions: tt.unhealthyConditions,
				},
			}
			webhook := &MachineHealthCheck{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, mhc)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, mhc)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
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
			name:      "when the nodeStartupTimeout is 0 (disabled)",
			timeout:   &zero,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		g := NewWithT(t)

		mhc := &clusterv1.MachineHealthCheck{
			Spec: clusterv1.MachineHealthCheckSpec{
				NodeStartupTimeout: tt.timeout,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
				UnhealthyConditions: []clusterv1.UnhealthyCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}
		webhook := &MachineHealthCheck{}

		if tt.expectErr {
			warnings, err := webhook.ValidateCreate(ctx, mhc)
			g.Expect(err).To(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
			warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
			g.Expect(err).To(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		} else {
			warnings, err := webhook.ValidateCreate(ctx, mhc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
			warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
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
		mhc := &clusterv1.MachineHealthCheck{
			Spec: clusterv1.MachineHealthCheckSpec{
				MaxUnhealthy: &maxUnhealthy,
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
				UnhealthyConditions: []clusterv1.UnhealthyCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}
		webhook := &MachineHealthCheck{}

		if tt.expectErr {
			warnings, err := webhook.ValidateCreate(ctx, mhc)
			g.Expect(err).To(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
			warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
			g.Expect(err).To(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		} else {
			warnings, err := webhook.ValidateCreate(ctx, mhc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
			warnings, err = webhook.ValidateUpdate(ctx, mhc, mhc)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(warnings).To(BeEmpty())
		}
	}
}

func TestMachineHealthCheckSelectorValidation(t *testing.T) {
	g := NewWithT(t)
	mhc := &clusterv1.MachineHealthCheck{
		Spec: clusterv1.MachineHealthCheckSpec{
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	webhook := &MachineHealthCheck{}

	err := webhook.validate(nil, mhc)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("selector must not be empty"))
}

func TestMachineHealthCheckClusterNameSelectorValidation(t *testing.T) {
	g := NewWithT(t)
	mhc := &clusterv1.MachineHealthCheck{
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName: "foo",
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					clusterv1.ClusterNameLabel: "bar",
					"baz":                      "qux",
				},
			},
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	webhook := &MachineHealthCheck{}

	err := webhook.validate(nil, mhc)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cannot specify a cluster selector other than the one specified by ClusterName"))

	mhc.Spec.Selector.MatchLabels[clusterv1.ClusterNameLabel] = "foo"
	g.Expect(webhook.validate(nil, mhc)).To(Succeed())
	delete(mhc.Spec.Selector.MatchLabels, clusterv1.ClusterNameLabel)
	g.Expect(webhook.validate(nil, mhc)).To(Succeed())
}

func TestMachineHealthCheckRemediationTemplateNamespaceValidation(t *testing.T) {
	valid := &clusterv1.MachineHealthCheck{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			Selector:            metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
			RemediationTemplate: &corev1.ObjectReference{Namespace: "foo"},
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	invalid := valid.DeepCopy()
	invalid.Spec.RemediationTemplate.Namespace = "bar"

	tests := []struct {
		name      string
		expectErr bool
		c         *clusterv1.MachineHealthCheck
	}{
		{
			name:      "should return error when MachineHealthCheck namespace and RemediationTemplate ref namespace mismatch",
			expectErr: true,
			c:         invalid,
		},
		{
			name:      "should succeed when namespaces match",
			expectErr: false,
			c:         valid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook := &MachineHealthCheck{}

			if tt.expectErr {
				g.Expect(webhook.validate(nil, tt.c)).NotTo(Succeed())
			} else {
				g.Expect(webhook.validate(nil, tt.c)).To(Succeed())
			}
		})
	}
}
