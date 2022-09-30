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
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("When testing clusterctl upgrades (v0.3=>current)", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InitWithBinary:            "https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.3.25/clusterctl-{OS}-{ARCH}",
			InitWithProvidersContract: "v1alpha3",
			// CAPI v0.3.x does not work on Kubernetes >= v1.22.
			InitWithKubernetesVersion: "v1.21.12",
			// CAPI does not work with Kubernetes < v1.22 if ClusterClass is enabled, so we have to disable it.
			UpgradeClusterctlVariables: map[string]string{
				"CLUSTER_TOPOLOGY": "false",
			},
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v0.4=>current)", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InitWithBinary:            "https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.4.8/clusterctl-{OS}-{ARCH}",
			InitWithProvidersContract: "v1alpha4",
			InitWithKubernetesVersion: "v1.25.0",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v1.2=>current)", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InitWithBinary:            "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.2.2/clusterctl-{OS}-{ARCH}",
			InitWithProvidersContract: "v1beta1",
			InitWithKubernetesVersion: "v1.25.0",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.2=>current) [ClusterClass]", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InitWithBinary:            "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.2.2/clusterctl-{OS}-{ARCH}",
			InitWithProvidersContract: "v1beta1",
			InitWithKubernetesVersion: "v1.25.0",
			WorkloadFlavor:            "topology",
		}
	})
})
