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
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v0.3/bases.
			InitWithKubernetesVersion: "v1.21.12",
			// CAPI does not work with Kubernetes < v1.22 if ClusterClass is enabled, so we have to disable it.
			WorkloadKubernetesVersion: "v1.21.12",
			UpgradeClusterctlVariables: map[string]string{
				"CLUSTER_TOPOLOGY": "false",
			},
			MgmtFlavor:     "topology",
			WorkloadFlavor: "",
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
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v0.4/bases.
			InitWithKubernetesVersion: "v1.23.13",
			WorkloadKubernetesVersion: "v1.23.13",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades (v1.2=>current)", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			InitWithBinary:        "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.2.12/clusterctl-{OS}-{ARCH}",
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.3.X for v1beta1).
			InitWithCoreProvider:            "cluster-api:v1.2.12",
			InitWithBootstrapProviders:      []string{"kubeadm:v1.2.12"},
			InitWithControlPlaneProviders:   []string{"kubeadm:v1.2.12"},
			InitWithInfrastructureProviders: []string{"docker:v1.2.12"},
			// We have to set this to an empty array as clusterctl v1.2 doesn't support
			// runtime extension providers. If we don't do this the test will automatically
			// try to deploy the latest version of our test-extension from docker.yaml.
			InitWithRuntimeExtensionProviders: []string{},
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v1.2/bases.
			InitWithKubernetesVersion: "v1.26.4",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "",
		}
	})
})

var _ = Describe("When testing clusterctl upgrades using ClusterClass (v1.2=>current) [ClusterClass]", func() {
	ClusterctlUpgradeSpec(ctx, func() ClusterctlUpgradeSpecInput {
		return ClusterctlUpgradeSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,
			InitWithBinary:        "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.2.7/clusterctl-{OS}-{ARCH}",
			// We have to pin the providers because with `InitWithProvidersContract` the test would
			// use the latest version for the contract (which is v1.3.0 for v1beta1).
			InitWithCoreProvider:            "cluster-api:v1.2.12",
			InitWithBootstrapProviders:      []string{"kubeadm:v1.2.12"},
			InitWithControlPlaneProviders:   []string{"kubeadm:v1.2.12"},
			InitWithInfrastructureProviders: []string{"docker:v1.2.12"},
			// We have to set this to an empty array as clusterctl v1.2 doesn't support
			// runtime extension providers. If we don't do this the test will automatically
			// try to deploy the latest version of our test-extension from docker.yaml.
			InitWithRuntimeExtensionProviders: []string{},
			// NOTE: If this version is changed here the image and SHA must also be updated in all DockerMachineTemplates in `test/data/infrastructure-docker/v1.2/bases.
			InitWithKubernetesVersion: "v1.26.4",
			MgmtFlavor:                "topology",
			WorkloadFlavor:            "topology",
		}
	})
})
