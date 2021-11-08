//go:build e2e
// +build e2e

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

package e2e

import (
	. "github.com/onsi/ginkgo"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
)

var _ = Describe("When testing KCP upgrade in a single control plane cluster", func() {
	KCPUpgradeSpec(ctx, func() KCPUpgradeSpecInput {
		return KCPUpgradeSpecInput{
			E2EConfig:                e2eConfig,
			ClusterctlConfigPath:     clusterctlConfigPath,
			BootstrapClusterProxy:    bootstrapClusterProxy,
			ArtifactFolder:           artifactFolder,
			SkipCleanup:              skipCleanup,
			ControlPlaneMachineCount: 1,
			Flavor:                   clusterctl.DefaultFlavor,
		}
	})
})

var _ = Describe("When testing KCP upgrade in a HA cluster", func() {
	KCPUpgradeSpec(ctx, func() KCPUpgradeSpecInput {
		return KCPUpgradeSpecInput{
			E2EConfig:                e2eConfig,
			ClusterctlConfigPath:     clusterctlConfigPath,
			BootstrapClusterProxy:    bootstrapClusterProxy,
			ArtifactFolder:           artifactFolder,
			SkipCleanup:              skipCleanup,
			ControlPlaneMachineCount: 3,
			Flavor:                   clusterctl.DefaultFlavor,
		}
	})
})

var _ = Describe("When testing KCP upgrade in a HA cluster using scale in rollout", func() {
	KCPUpgradeSpec(ctx, func() KCPUpgradeSpecInput {
		return KCPUpgradeSpecInput{
			E2EConfig:                e2eConfig,
			ClusterctlConfigPath:     clusterctlConfigPath,
			BootstrapClusterProxy:    bootstrapClusterProxy,
			ArtifactFolder:           artifactFolder,
			SkipCleanup:              skipCleanup,
			ControlPlaneMachineCount: 3,
			Flavor:                   "kcp-scale-in",
		}
	})
})
