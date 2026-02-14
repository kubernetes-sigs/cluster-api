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
	"os"
	"path"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/kubernetesversions"
)

var (
	clusterctlDownloadURL = "https://github.com/kubernetes-sigs/cluster-api/releases/download/v%s/clusterctl-{OS}-{ARCH}"
	providerCAPIPrefix    = "cluster-api:v%s"
	providerKubeadmPrefix = "kubeadm:v%s"
	providerDockerPrefix  = "docker:v%s"
)

var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.10=>v1.12=>current) [PR-Blocking] [ClusterClass]", Label("PR-Blocking", "ClusterClass"), func() {
	version := "1.10"
	stableRelease10, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)

	version = "1.12"
	stableRelease12, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			// Configuration for the initial provider deployment.
			InitWithBinary: fmt.Sprintf(clusterctlDownloadURL, stableRelease10),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is the next minor for v1beta1).
			InitWithCoreProvider:              fmt.Sprintf(providerCAPIPrefix, stableRelease10),
			InitWithBootstrapProviders:        []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease10)},
			InitWithControlPlaneProviders:     []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease10)},
			InitWithInfrastructureProviders:   []string{fmt.Sprintf(providerDockerPrefix, stableRelease10)},
			InitWithRuntimeExtensionProviders: []string{},
			Upgrades: []ClusterctlUpgradeSpecInputUpgrade{
				{
					WithBinary:              fmt.Sprintf(clusterctlDownloadURL, stableRelease12),
					CoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease12),
					BootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease12)},
					ControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease12)},
					InfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease12)},
					PreMachineDeploymentScaleUp: func(proxy framework.ClusterProxy, namespace string, clusterName string) {
						// Added as PreMachineDeploymentScaleUp as it triggers a rollout and we have to avoid that the test fails because of the rollout detection
						cc, err := os.ReadFile(path.Join(artifactFolder, "repository", "infrastructure-docker", "v" + stableRelease12, "clusterclass-quick-start-only.yaml"))
						Expect(err).ToNot(HaveOccurred())
						log.Logf("Applying the new ClusterClass")
						Expect(proxy.CreateOrUpdate(ctx, cc)).To(Succeed())
					},
				},
				{ // Upgrade to latest contract version.
					Contract: clusterv1.GroupVersion.Version,
					PostUpgrade: func(proxy framework.ClusterProxy, namespace, clusterName string) {
						framework.ValidateCRDMigration(ctx, proxy, namespace, clusterName,
							crdShouldBeMigrated, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
					},
				},
			},
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			// When picking this version, please check also the list of versions known by the source Cluster API version (rif. test/infrastructure/kind/mapper.go).
			InitWithKubernetesVersion:   "v1.33.0",
			WorkloadKubernetesVersion:   "v1.33.0",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should be changed during "prepare main branch", it should test n-3 => current.
var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.10=>current) [ClusterClass]", Label("ClusterClass"), func() {
	// Get n-3 latest stable release
	version := "1.10"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			InitWithBinary:        fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is the next minor for v1beta1).
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease)},
			InitWithProvidersContract:       "v1beta1",
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			// When picking this version, please check also the list of versions known by the source Cluster API version (rif. test/infrastructure/kind/mapper.go).
			InitWithKubernetesVersion:   "v1.33.0",
			WorkloadKubernetesVersion:   "v1.33.0",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should be changed during "prepare main branch", it should test n-2 => current.
var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.11=>current) [ClusterClass]", Label("ClusterClass"), func() {
	// Get n-2 latest stable release
	version := "1.11"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			InitWithBinary:        fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is the next minor for v1beta1).
			InitWithCoreProvider:            fmt.Sprintf(providerCAPIPrefix, stableRelease),
			InitWithBootstrapProviders:      []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithControlPlaneProviders:   []string{fmt.Sprintf(providerKubeadmPrefix, stableRelease)},
			InitWithInfrastructureProviders: []string{fmt.Sprintf(providerDockerPrefix, stableRelease)},
			InitWithProvidersContract:       "v1beta1",
			Upgrades: []ClusterctlUpgradeSpecInputUpgrade{
				{ // Upgrade to latest contract version.
					Contract: clusterv1.GroupVersion.Version,
					PostUpgrade: func(proxy framework.ClusterProxy, namespace, clusterName string) {
						framework.ValidateCRDMigration(ctx, proxy, namespace, clusterName,
							crdShouldBeMigrated, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
					},
				},
			},
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			// When picking this version, please check also the list of versions known by the source Cluster API version (rif. test/infrastructure/kind/mapper.go).
			InitWithKubernetesVersion:   "v1.34.0",
			WorkloadKubernetesVersion:   "v1.34.0",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: true,
		}
	})
})

// Note: This test should be changed during "prepare main branch", it should test n-1 => current.
var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.12=>current) [ClusterClass]", Label("ClusterClass"), func() {
	// Get n-1 latest stable release
	version := "1.12"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InitWithBinary:            fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			InitWithProvidersContract: "v1beta1",
			Upgrades: []ClusterctlUpgradeSpecInputUpgrade{
				{ // Upgrade to latest contract version.
					Contract: clusterv1.GroupVersion.Version,
					PostUpgrade: func(proxy framework.ClusterProxy, namespace, clusterName string) {
						framework.ValidateCRDMigration(ctx, proxy, namespace, clusterName,
							crdShouldBeMigrated, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
					},
				},
			},
			// Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			// When picking this version, please check also the list of versions known by the source Cluster API version (rif. test/infrastructure/kind/mapper.go).
			InitWithKubernetesVersion:   "v1.35.0",
			WorkloadKubernetesVersion:   "v1.35.0",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: false, // Using false for one test case to ensure this code path of the test keeps working.
		}
	})
})

// Note: This test should be changed during "prepare main branch", it should test n-1 => current.
var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.12=>current) on K8S latest ci mgmt cluster [ClusterClass]", Label("ClusterClass"), func() {
	// Get n-1 latest stable release
	version := "1.12"
	stableRelease, err := GetStableReleaseOfMinor(ctx, version)
	Expect(err).ToNot(HaveOccurred(), "Failed to get stable version for minor release : %s", version)
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		initKubernetesVersion, err := kubernetesversions.ResolveVersion(ctx, e2eConfig.MustGetVariable("KUBERNETES_VERSION_LATEST_CI"))
		Expect(err).ToNot(HaveOccurred())
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InitWithBinary:            fmt.Sprintf(clusterctlDownloadURL, stableRelease),
			InitWithProvidersContract: "v1beta1",
			Upgrades: []ClusterctlUpgradeSpecInputUpgrade{
				{ // Upgrade to latest contract version.
					Contract: clusterv1.GroupVersion.Version,
					PostUpgrade: func(proxy framework.ClusterProxy, namespace, clusterName string) {
						framework.ValidateCRDMigration(ctx, proxy, namespace, clusterName,
							crdShouldBeMigrated, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
					},
				},
			},
			// Note: InitWithKubernetesVersion should be the latest of the next supported kubernetes version by the target Cluster API version.
			// Note: WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			// When picking this version, please check also the list of versions known by the source Cluster API version (rif. test/infrastructure/kind/mapper.go).
			InitWithKubernetesVersion:   initKubernetesVersion,
			WorkloadKubernetesVersion:   "v1.35.0",
			MgmtFlavor:                  "topology",
			WorkloadFlavor:              "topology",
			UseKindForManagementCluster: true,
		}
	})
})

func crdShouldBeMigrated(crd apiextensionsv1.CustomResourceDefinition) bool {
	return strings.HasSuffix(crd.Name, ".cluster.x-k8s.io") &&
		crd.Name != "providers.clusterctl.cluster.x-k8s.io"
}
