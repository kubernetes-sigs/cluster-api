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
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var _ = Describe("When testing clusterctl upgrades (v0.3=>v1.5=>current)", func() {
	// We are testing v0.3=>v1.5=>current to ensure that old entries with v1alpha3 in managed files do not cause issues
	// as described in https://github.com/kubernetes-sigs/cluster-api/issues/10051.
	// NOTE: The combination of v0.3=>v1.5=>current allows us to verify this without being forced to upgrade
	// the management cluster in the middle of the test as all 3 versions are ~ compatible with the same mgmt and workload Kubernetes versions.
	// Additionally, clusterctl v1.5 still allows the upgrade of management clusters from v1alpha3 (v1.6 doesn't).
	clusterctlDownloadURL03 := "https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.3.25/clusterctl-{OS}-{ARCH}"
	if runtime.GOOS == "darwin" {
		// There is no arm64 binary for v0.3.x, so we'll use the amd64 one.
		clusterctlDownloadURL03 = "https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.3.25/clusterctl-darwin-amd64"
	}
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: pointer.String("docker"),
			// Configuration for the initial provider deployment.
			InitWithBinary: clusterctlDownloadURL03,
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract.
			InitWithCoreProvider:            "cluster-api:v0.3.25",
			InitWithBootstrapProviders:      []string{"kubeadm:v0.3.25"},
			InitWithControlPlaneProviders:   []string{"kubeadm:v0.3.25"},
			InitWithInfrastructureProviders: []string{"docker:v0.3.25"},
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
					WithBinary:              "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.5.0/clusterctl-{OS}-{ARCH}",
					CoreProvider:            "cluster-api:v1.5.0",
					BootstrapProviders:      []string{"kubeadm:v1.5.0"},
					ControlPlaneProviders:   []string{"kubeadm:v1.5.0"},
					InfrastructureProviders: []string{"docker:v1.5.0"},
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
			MgmtFlavor:     "topology",
			WorkloadFlavor: "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v0.4=>v1.5=>current)", func() {
	// We are testing v0.4=>v1.5=>current to ensure that old entries with v1alpha4 in managed files do not cause issues
	// as described in https://github.com/kubernetes-sigs/cluster-api/issues/10051.
	// NOTE: The combination of v0.4=>v1.5=>current allows us to verify this without being forced to upgrade
	// the management cluster in the middle of the test as all 3 versions are ~ compatible with the same mgmt and workload Kubernetes versions.
	// Additionally, clusterctl v1.5 still allows the upgrade of management clusters from v1alpha4 (v1.6 does as well, but v1.7 doesn't).
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: pointer.String("docker"),
			// Configuration for the initial provider deployment.
			InitWithBinary: "https://github.com/kubernetes-sigs/cluster-api/releases/download/v0.4.8/clusterctl-{OS}-{ARCH}",
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract.
			InitWithCoreProvider:            "cluster-api:v0.4.8",
			InitWithBootstrapProviders:      []string{"kubeadm:v0.4.8"},
			InitWithControlPlaneProviders:   []string{"kubeadm:v0.4.8"},
			InitWithInfrastructureProviders: []string{"docker:v0.4.8"},
			// We have to set this to an empty array as clusterctl v0.4 doesn't support
			// runtime extension providers. If we don't do this the test will automatically
			// try to deploy the latest version of our test-extension from docker.yaml.
			InitWithRuntimeExtensionProviders: []string{},
			// Configuration for the provider upgrades.
			Upgrades: []ClusterctlUpgradeSpecInputUpgrade{
				{
					// Upgrade to v1.5.
					// Note: v1.5 is a version we can use as it's
					// able to upgrade from a v1alpha4 management cluster (v1.6 would be able to as well)
					WithBinary:              "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.5.0/clusterctl-{OS}-{ARCH}",
					CoreProvider:            "cluster-api:v1.5.0",
					BootstrapProviders:      []string{"kubeadm:v1.5.0"},
					ControlPlaneProviders:   []string{"kubeadm:v1.5.0"},
					InfrastructureProviders: []string{"docker:v1.5.0"},
				},
				{ // Upgrade to latest v1beta1.
					Contract: clusterv1.GroupVersion.Version,
				},
			},
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v0.4/bases.
			//  Note: Both InitWithKubernetesVersion and WorkloadKubernetesVersion should be the highest mgmt cluster version supported by the source Cluster API version.
			InitWithKubernetesVersion: "v1.23.17",
			WorkloadKubernetesVersion: "v1.23.17",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v1.0=>current)", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: pointer.String("docker"),
			InitWithBinary:         "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.0.5/clusterctl-{OS}-{ARCH}",
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.3.X for v1beta1).
			InitWithCoreProvider:            "cluster-api:v1.0.5",
			InitWithBootstrapProviders:      []string{"kubeadm:v1.0.5"},
			InitWithControlPlaneProviders:   []string{"kubeadm:v1.0.5"},
			InitWithInfrastructureProviders: []string{"docker:v1.0.5"},
			// We have to set this to an empty array as clusterctl v1.0 doesn't support
			// runtime extension providers. If we don't do this the test will automatically
			// try to deploy the latest version of our test-extension from docker.yaml.
			InitWithRuntimeExtensionProviders: []string{},
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v1.0/bases.
			InitWithKubernetesVersion: "v1.23.17",
			WorkloadKubernetesVersion: "v1.23.17",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v1.4=>current)", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: pointer.String("docker"),
			InitWithBinary:         "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.4.5/clusterctl-{OS}-{ARCH}",
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.5.X for v1beta1).
			InitWithCoreProvider:            "cluster-api:v1.4.5",
			InitWithBootstrapProviders:      []string{"kubeadm:v1.4.5"},
			InitWithControlPlaneProviders:   []string{"kubeadm:v1.4.5"},
			InitWithInfrastructureProviders: []string{"docker:v1.4.5"},
			InitWithProvidersContract:       "v1beta1",
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/e2e/data/infrastructure-docker/v1.4/bases.
			InitWithKubernetesVersion: "v1.27.3",
			WorkloadKubernetesVersion: "v1.27.3",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.4=>current) [ClusterClass]", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: pointer.String("docker"),
			InitWithBinary:         "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.4.5/clusterctl-{OS}-{ARCH}",
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.5.X for v1beta1).
			InitWithCoreProvider:            "cluster-api:v1.4.5",
			InitWithBootstrapProviders:      []string{"kubeadm:v1.4.5"},
			InitWithControlPlaneProviders:   []string{"kubeadm:v1.4.5"},
			InitWithInfrastructureProviders: []string{"docker:v1.4.5"},
			InitWithProvidersContract:       "v1beta1",
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/e2e/data/infrastructure-docker/v1.4/bases.
			InitWithKubernetesVersion: "v1.27.3",
			WorkloadKubernetesVersion: "v1.27.3",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "topology",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v1.5=>current)", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InfrastructureProvider:    pointer.String("docker"),
			InitWithBinary:            "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.5.0/clusterctl-{OS}-{ARCH}",
			InitWithProvidersContract: "v1beta1",
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/e2e/data/infrastructure-docker/v1.5/bases.
			InitWithKubernetesVersion: "v1.28.0",
			WorkloadKubernetesVersion: "v1.28.0",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.5=>current) [ClusterClass]", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:                 e2eConfig,
			ClusterctlConfigPath:      clusterctlConfigPath,
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ArtifactFolder:            artifactFolder,
			SkipCleanup:               skipCleanup,
			InfrastructureProvider:    pointer.String("docker"),
			InitWithBinary:            "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.5.0/clusterctl-{OS}-{ARCH}",
			InitWithProvidersContract: "v1beta1",
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/e2e/data/infrastructure-docker/v1.5/bases.
			InitWithKubernetesVersion: "v1.28.0",
			WorkloadKubernetesVersion: "v1.28.0",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "topology",
		}
	})
})
