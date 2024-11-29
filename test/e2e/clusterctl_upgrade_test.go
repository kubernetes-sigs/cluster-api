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
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework/kubernetesversions"
)

var (
	clusterctlDownloadURL = "https://github.com/kubernetes-sigs/cluster-api/releases/download/v%s/clusterctl-{OS}-{ARCH}"
	providerCAPIPrefix    = "cluster-api:v%s"
	providerKubeadmPrefix = "kubeadm:v%s"
	providerDockerPrefix  = "docker:v%s"
)

// Note: This test should not be changed during "prepare main branch".
var _ = Describe("When testing clusterctl upgrades (v0.3=>v1.5=>current)", func() {
	// We are testing v0.3=>v1.5=>current to ensure that old entries with v1alpha3 in managed files do not cause issues
	// as described in https://github.com/kubernetes-sigs/cluster-api/issues/10051.
	// NOTE: The combination of v0.3=>v1.5=>current allows us to verify this without being forced to upgrade
	// the management cluster in the middle of the test as all 3 versions are ~ compatible with the same mgmt and workload Kubernetes versions.
	// Additionally, clusterctl v1.5 still allows the upgrade of management clusters from v1alpha3 (v1.6 doesn't).
	// https://github.com/kubernetes-sigs/cluster-api/blob/release-1.5/cmd/clusterctl/client/upgrade.go#L151-L159
	// https://github.com/kubernetes-sigs/cluster-api/blob/release-1.6/cmd/clusterctl/client/upgrade.go#L149-L155

	// Get v0.3 latest stable release
	version03 := "0.3"
	stableRelease03, err := GetStableReleaseOfMinor(ctx, version03)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version03)
	clusterctlDownloadURL03 := clusterctlDownloadURL
	if runtime.GOOS == "darwin" {
		// There is no arm64 binary for v0.3.x, so we'll use the amd64 one.
		clusterctlDownloadURL03 = "https://github.com/kubernetes-sigs/cluster-api/releases/download/v%s/clusterctl-darwin-amd64"
	}

	// Get v1.5 latest stable release
	version15 := "1.5"
	stableRelease15, err := GetStableReleaseOfMinor(ctx, version15)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version15)

	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			// Configuration for the initial provider deployment.
			InitWithBinary: fmt.Sprintf(clusterctlDownloadURL03, stableRelease03),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract.
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease03),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease03)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease03)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease03)},
			// We have to set this to an empty array as clusterctl v0.3 doesn't support
			// runtime extension providers. If we don't do this the test will automatically
			// try to deploy the latest version of our test-extension from docker.yaml.
			InitWithRuntimeExtensionProviders: []string{},
			// Configuration for the provider upgrades.
			Upgrades: []ClusterctlUpgradeSpecInputUpgrade{
				{
					// Upgrade to v1.5.
					// Note: v1.5 is the highest version we can use as it's the last one
					// that is able to upgrade from a v1alpha3 management cluster.
					WithBinary:              fmt.Sprintf(clusterctlDownloadURL, stableRelease15),
					CoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease15),
					BootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease15)},
					ControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease15)},
					InfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease15)},
				},
				{ // Upgrade to latest v1beta1.
					Contract: clusterv1.GroupVersion.Version,
				},
			},
			// CAPI v0.3.x does not work on Kubernetes >= v1.22.
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v0.3/bases.
			//  Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion: "v1.21.14",
			WorkloadKubernetesVersion: "v1.22.17",
			// CAPI does not work with Kubernetes < v1.22 if ClusterClass is enabled, so we have to disable it.
			UpgradeClusterctlVariables: map[string]string{
				"CLUSTER_TOPOLOGY": "false",
			},
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should not be changed during "prepare main branch".
var _ = Describe("When testing clusterctl upgrades (v0.4=>v1.6=>current)", func() {
	// We are testing v0.4=>v1.6=>current to ensure that old entries with v1alpha4 in managed files do not cause issues
	// as described in https://github.com/kubernetes-sigs/cluster-api/issues/10051.
	// NOTE: The combination of v0.4=>v1.6=>current allows us to verify this without being forced to upgrade
	// the management cluster in the middle of the test as all 3 versions are ~ compatible with the same mgmt and workload Kubernetes versions.
	// Additionally, clusterctl v1.6 still allows the upgrade of management clusters from v1alpha4 (v1.7 doesn't).
	// https://github.com/kubernetes-sigs/cluster-api/blob/release-1.6/cmd/clusterctl/client/upgrade.go#L149-L155
	// https://github.com/kubernetes-sigs/cluster-api/blob/release-1.7/cmd/clusterctl/client/upgrade.go#L145-L148

	// Get v0.4 latest stable release
	version04 := "0.4"
	stableRelease04, err := GetStableReleaseOfMinor(ctx, version04)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version04)

	// Get v1.6 latest stable release
	version16 := "1.6"
	stableRelease16, err := GetStableReleaseOfMinor(ctx, version16)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version16)

	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			// Configuration for the initial provider deployment.
			InitWithBinary: fmt.Sprintf(clusterctlDownloadURL, stableRelease04),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract.
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease04),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease04)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease04)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease04)},
			// We have to set this to an empty array as clusterctl v0.4 doesn't support
			// runtime extension providers. If we don't do this the test will automatically
			// try to deploy the latest version of our test-extension from docker.yaml.
			InitWithRuntimeExtensionProviders: []string{},
			// Configuration for the provider upgrades.
			Upgrades: []ClusterctlUpgradeSpecInputUpgrade{
				{
					// Upgrade to v1.6.
					// Note: v1.6 is the highest version we can use as it's the last one
					// that is able to upgrade from a v1alpha4 management cluster.
					WithBinary:              fmt.Sprintf(clusterctlDownloadURL, stableRelease16),
					CoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease16),
					BootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease16)},
					ControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease16)},
					InfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease16)},
				},
				{ // Upgrade to latest v1beta1.
					Contract: clusterv1.GroupVersion.Version,
				},
			},
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v0.4/bases.
			//  Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion:   "v1.23.17",
			WorkloadKubernetesVersion:   "v1.23.17",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should not be changed during "prepare main branch".
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
			InitWithKubernetesVersion:   "v1.23.17",
			WorkloadKubernetesVersion:   "v1.23.17",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should be changed during "prepare main branch", it should test n-2 => current.
var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.7=>current) [ClusterClass]", func() {
	// Get v1.7 latest stable release
	version := "1.7"
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
			// use the latest version for the contract (which is the next minor for v1beta1).
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease)},
			InitWithProvidersContract:       "v1beta1",
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion:   "v1.30.0",
			WorkloadKubernetesVersion:   "v1.30.0",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should be changed during "prepare main branch", it should test n-1 => current.
var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.8=>current) [ClusterClass]", func() {
	// Get v1.8 latest stable release
	version := "1.8"
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
			InitWithKubernetesVersion:   "v1.31.2",
			WorkloadKubernetesVersion:   "v1.31.2",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should be changed during "prepare main branch", it should test n-1 => current.
var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.8=>current) on K8S latest ci mgmt cluster [ClusterClass]", func() {
	// Get v1.8 latest stable release
	version := "1.8"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		initKubernetesVersion, err := kubernetesversions.ResolveVersion(ctx, e2eConfig.GetVariable("KUBERNETES_VERSION_LATEST_CI"))
		Expect(err).ToNot(HaveOccurred())
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InfrastructureProvider:    ptr.To("docker"),
			InitWithBinary:            fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			InitWithProvidersContract: "v1beta1",
			// Note: InitWithKubernetesVersion should be the latest of the next supported kubernetes version by the target Cluster API version.
			// Note: WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion:   initKubernetesVersion,
			WorkloadKubernetesVersion:   "v1.31.2",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: true,
		}
	})
})
