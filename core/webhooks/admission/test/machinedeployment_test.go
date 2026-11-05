/*
Copyright 2026 The Kubernetes Authors.

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
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/intstr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

const rollingUpdateCELErrMsg = "rollingUpdate can only be set if type is RollingUpdate"

func TestMachineDeploymentRolloutStrategyValidation(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-machinedeployment-rollout-strategy")
	g.Expect(err).ToNot(HaveOccurred())
	t.Cleanup(func() {
		g.Expect(env.Cleanup(ctx, ns)).To(Succeed())
	})

	maxSurge := intstr.FromInt32(1)
	newMachineDeployment := func(name string, strategy clusterv1.MachineDeploymentRolloutStrategy) *clusterv1.MachineDeployment {
		md := builder.MachineDeployment(ns.Name, name).
			WithClusterName("cluster1").
			WithBootstrapTemplate(builder.BootstrapTemplate(ns.Name, "bootstrap-template").Build()).
			WithInfrastructureTemplate(builder.InfrastructureMachineTemplate(ns.Name, "infrastructure-machine-template").Build()).
			Build()
		md.Spec.Rollout.Strategy = strategy
		return md
	}

	t.Run("Should reject rollingUpdate on create if type is OnDelete", func(t *testing.T) {
		g := NewWithT(t)

		md := newMachineDeployment("md-ondelete-rollingupdate", clusterv1.MachineDeploymentRolloutStrategy{
			Type:          clusterv1.OnDeleteMachineDeploymentStrategyType,
			RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{MaxSurge: &maxSurge},
		})
		err := env.Create(ctx, md)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(rollingUpdateCELErrMsg))
	})

	t.Run("Should reject switching to OnDelete while keeping rollingUpdate", func(t *testing.T) {
		g := NewWithT(t)

		md := newMachineDeployment("md-rollingupdate", clusterv1.MachineDeploymentRolloutStrategy{
			Type:          clusterv1.RollingUpdateMachineDeploymentStrategyType,
			RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{MaxSurge: &maxSurge},
		})
		g.Expect(env.CreateAndWait(ctx, md)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(env.CleanupAndWait(ctx, md)).To(Succeed())
		})

		invalid := md.DeepCopy()
		invalid.Spec.Rollout.Strategy.Type = clusterv1.OnDeleteMachineDeploymentStrategyType
		err := env.Update(ctx, invalid)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(rollingUpdateCELErrMsg))

		valid := md.DeepCopy()
		valid.Spec.Rollout.Strategy = clusterv1.MachineDeploymentRolloutStrategy{
			Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
		}
		g.Expect(env.Update(ctx, valid)).To(Succeed())
	})
}
