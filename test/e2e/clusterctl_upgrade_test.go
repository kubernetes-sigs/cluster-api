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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

var (
	clusterctlDownloadURL = "https://github.com/kubernetes-sigs/cluster-api/releases/download/v%s/clusterctl-{OS}-{ARCH}"
	providerCAPIPrefix    = "cluster-api:v%s"
	providerKubeadmPrefix = "kubeadm:v%s"
	providerDockerPrefix  = "docker:v%s"
)

var _ = Describe("When testing clusterctl upgrades (v1.0=>current)", func() {
	// Get v1.0 latest stable release
	version := "1.0"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			InitWithBinary:         fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.3.X for v1beta1).
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease)},
			// We have to set this to an empty array as clusterctl v1.0 doesn't support
			// runtime extension providers. If we don't do this the test will automatically
			// try to deploy the latest version of our test-extension from docker.yaml.
			InitWithRuntimeExtensionProviders: []string{},
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v1.0/bases.
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion: "v1.23.17",
			WorkloadKubernetesVersion: "v1.23.17",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v1.5=>current)", func() {
	// Get v1.5 latest stable release
	version := "1.5"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			InitWithBinary:         fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.6.X for v1beta1).
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease)},
			InitWithProvidersContract:       "v1beta1",
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion: "v1.28.0",
			WorkloadKubernetesVersion: "v1.28.0",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.5=>current) [ClusterClass]", func() {
	// Get v1.5 latest stable release
	version := "1.5"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			InitWithBinary:         fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.6.X for v1beta1).
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease)},
			InitWithProvidersContract:       "v1beta1",
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion: "v1.28.0",
			WorkloadKubernetesVersion: "v1.28.0",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "topology",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v1.6=>current)", func() {
	// Get v1.6 latest stable release
	version := "1.6"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InfrastructureProvider:    ptr.To("docker"),
			InitWithBinary:            fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			InitWithProvidersContract: "v1beta1",
			//  Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion: "v1.29.0",
			WorkloadKubernetesVersion: "v1.29.0",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.6=>current) [ClusterClass]", func() {
	// Get v1.6 latest stable release
	version := "1.6"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InfrastructureProvider:    ptr.To("docker"),
			InitWithBinary:            fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			InitWithProvidersContract: "v1beta1",
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion: "v1.29.0",
			WorkloadKubernetesVersion: "v1.29.0",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "topology",
		}
	})
})
