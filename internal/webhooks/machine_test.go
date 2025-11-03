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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachineDefault(t *testing.T) {
	g := NewWithT(t)

	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foobar",
		},
		Spec: clusterv1.MachineSpec{
			Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
				Name: "bootstrap1",
			}},
			Version: "1.17.5",
		},
	}

	webhook := &Machine{}

	t.Run("for Machine", util.CustomDefaultValidateTest(ctx, m, webhook))
	g.Expect(webhook.Default(ctx, m)).To(Succeed())

	g.Expect(m.Labels[clusterv1.ClusterNameLabel]).To(Equal(m.Spec.ClusterName))
	g.Expect(m.Spec.Version).To(Equal("v1.17.5"))
	g.Expect(*m.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(defaultNodeDeletionTimeoutSeconds))
}

func TestMachineBootstrapValidation(t *testing.T) {
	tests := []struct {
		name      string
		bootstrap clusterv1.Bootstrap
		expectErr bool
	}{
		{
			name:      "should return error if configref and data are nil",
			bootstrap: clusterv1.Bootstrap{DataSecretName: nil},
			expectErr: true,
		},
		{
			name:      "should not return error if dataSecretName is set",
			bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("test")},
			expectErr: false,
		},
		{
			name:      "should not return error if dataSecretName is set",
			bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("")},
			expectErr: false,
		},
		{
			name:      "should not return error if config ref is set",
			bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{Name: "bootstrap1"}, DataSecretName: nil},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			m := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{Bootstrap: tt.bootstrap},
			}
			webhook := &Machine{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestMachineClusterNameImmutable(t *testing.T) {
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

			newMachine := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					ClusterName: tt.newClusterName,
					Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
						Name: "bootstrap1",
					}},
				},
			}
			oldMachine := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					ClusterName: tt.oldClusterName,
					Bootstrap: clusterv1.Bootstrap{ConfigRef: clusterv1.ContractVersionedObjectReference{
						Name: "bootstrap1",
					}},
				},
			}

			warnings, err := (&Machine{}).ValidateUpdate(ctx, oldMachine, newMachine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestMachineVersionValidation(t *testing.T) {
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

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			m := &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					Version:   tt.version,
					Bootstrap: clusterv1.Bootstrap{DataSecretName: ptr.To("test")},
				},
			}
			webhook := &Machine{}

			if tt.expectErr {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).To(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			} else {
				warnings, err := webhook.ValidateCreate(ctx, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
				warnings, err = webhook.ValidateUpdate(ctx, m, m)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func TestMachineTaintValidation(t *testing.T) {
	m := builder.Machine("default", "machine1").
		WithBootstrapTemplate(builder.BootstrapTemplate("default", "bootstrap-template").Build())
	webhook := &Machine{}

	tests := []struct {
		name           string
		machine        *clusterv1.Machine
		featureEnabled bool
		expectErr      bool
	}{
		{
			name:           "should allow empty taints with feature gate disabled",
			featureEnabled: false,
			machine:        m.DeepCopy().Build(),
			expectErr:      false,
		},
		{
			name:           "should allow empty taints with feature gate enabled",
			featureEnabled: true,
			machine:        m.DeepCopy().Build(),
			expectErr:      false,
		},
		{
			name:           "should block taint key node.cluster.x-k8s.io/uninitialized",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node.cluster.x-k8s.io/uninitialized", Effect: corev1.TaintEffectNoSchedule,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should block taint key node.cluster.x-k8s.io/outdated-revision",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node.cluster.x-k8s.io/outdated-revision", Effect: corev1.TaintEffectNoSchedule,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should block taint with key prefix node.kubernetes.io/, which is not `out-of-service`",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node.kubernetes.io/some-taint", Effect: corev1.TaintEffectNoSchedule,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should allow taint node.kubernetes.io/out-of-service",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node.kubernetes.io/out-of-service", Effect: corev1.TaintEffectNoSchedule,
			}).Build(),
			expectErr: false,
		},
		{
			name:           "should block taint with keyprefix node.cloudprovider.kubernetes.io/",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node.cloudprovider.kubernetes.io/some-taint", Effect: corev1.TaintEffectNoSchedule,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should block taint key node-role.kubernetes.io/master",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node-role.kubernetes.io/master", Effect: corev1.TaintEffectNoSchedule,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should allow taint key node-role.kubernetes.io/control-plane for control-plane nodes",
			featureEnabled: true,
			machine: m.DeepCopy().
				WithLabels(map[string]string{clusterv1.MachineControlPlaneLabel: "true"}).
				WithTaints(clusterv1.MachineTaint{
					Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule,
				}).Build(),
			expectErr: false,
		},
		{
			name:           "should block taint key node-role.kubernetes.io/control-plane for worker nodes",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule,
			}).Build(),
			expectErr: true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachineTaintPropagation, tt.featureEnabled)

			warnings, err := webhook.ValidateCreate(ctx, tt.machine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())

			warnings, err = webhook.ValidateUpdate(ctx, tt.machine, tt.machine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}
