//go:build e2e
// +build e2e

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

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	"k8s.io/utils/ptr"
)

var _ = Describe("When upgrading a workload cluster using ClusterClass and testing K8S conformance [Conformance] [K8s-Upgrade] [ClusterClass]", func() {
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			Flavor:                 ptr.To("upgrades"),
		}
	})
})

var _ = Describe("When upgrading a workload cluster using ClusterClass [ClusterClass]", func() {
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			Flavor:                 ptr.To("topology"),
			// This test is run in CI in parallel with other tests. To keep the test duration reasonable
			// the conformance tests are skipped.
			ControlPlaneMachineCount: ptr.To[int64](1),
			WorkerMachineCount:       ptr.To[int64](2),
			SkipConformanceTests:     true,
		}
	})
})

var _ = Describe("When upgrading a workload cluster using ClusterClass with a HA control plane [ClusterClass]", func() {
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			// This test is run in CI in parallel with other tests. To keep the test duration reasonable
			// the conformance tests are skipped.
			SkipConformanceTests:     true,
			ControlPlaneMachineCount: ptr.To[int64](3),
			WorkerMachineCount:       ptr.To[int64](1),
			Flavor:                   ptr.To("topology"),
		}
	})
})

var _ = Describe("When upgrading a workload cluster using ClusterClass with a HA control plane using scale-in rollout [ClusterClass]", func() {
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			// This test is run in CI in parallel with other tests. To keep the test duration reasonable
			// the conformance tests are skipped.
			SkipConformanceTests:     true,
			ControlPlaneMachineCount: ptr.To[int64](3),
			WorkerMachineCount:       ptr.To[int64](1),
			Flavor:                   ptr.To("kcp-scale-in"),
		}
	})
})
