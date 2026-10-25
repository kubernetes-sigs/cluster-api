/*
Copyright 2025 The Kubernetes Authors.

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

package test

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestMachineTaintValidation(t *testing.T) {
	infraMachine := builder.InfrastructureMachine("default", "infrastructure-machine1").Build()
	m := builder.Machine("default", "m").
		WithClusterName("cluster1").
		WithBootstrapTemplate(builder.BootstrapTemplate("default", "bootstrap-template").Build()).
		WithInfrastructureMachine(infraMachine)

	chars254 := "moreThen253Charactersaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaadddz"
	chars253 := "253charactersaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaddd"
	chars63 := "63charactersaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbddd"
	chars64 := "moreThene63Charactersaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbbbbbbbbdddz"

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
			name:           "should forbid taints with feature gate disabled",
			featureEnabled: false,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "some/taint", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
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
				Key: "node.cluster.x-k8s.io/uninitialized", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
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
				Key: "node.kubernetes.io/some-taint", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should allow taint node.kubernetes.io/out-of-service",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node.kubernetes.io/out-of-service", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: false,
		},
		{
			name:           "should block taint with key prefix node.cloudprovider.kubernetes.io/",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node.cloudprovider.kubernetes.io/some-taint", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should block taint key node-role.kubernetes.io/master",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node-role.kubernetes.io/master", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should allow taint key node-role.kubernetes.io/control-plane for control-plane nodes",
			featureEnabled: true,
			machine: m.DeepCopy().
				WithLabels(map[string]string{clusterv1.MachineControlPlaneLabel: "true"}).
				WithTaints(clusterv1.MachineTaint{
					Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
				}).Build(),
			expectErr: false,
		},
		{
			name:           "should block taint key node-role.kubernetes.io/control-plane for worker nodes",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should allow taint key without prefix and 63 characters",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key:    chars63,
				Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: false,
		},
		{
			name:           "should block taint key without prefix and more than 63 characters",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key:    chars64,
				Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should allow taint key with prefix of 253 characters",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key:    chars253 + "/" + chars63,
				Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: false,
		},
		{
			name:           "should block taint key with prefix of more than 253 characters",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key:    chars254 + "/" + chars63,
				Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should block taint key with empty prefix",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "/with-empty-prefix", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
		{
			name:           "should block taint key with multiple slashes",
			featureEnabled: true,
			machine: m.DeepCopy().WithTaints(clusterv1.MachineTaint{
				Key: "one/two/with-prefix", Effect: corev1.TaintEffectNoSchedule, Propagation: clusterv1.MachineTaintPropagationAlways,
			}).Build(),
			expectErr: true,
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.MachineTaintPropagation, tt.featureEnabled)

			tt.machine.Name = fmt.Sprintf("machine-%02d", i)

			err := env.CreateAndWait(ctx, tt.machine)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
