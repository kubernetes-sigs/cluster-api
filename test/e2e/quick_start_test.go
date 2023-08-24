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
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/kubetest"
)

var _ = Describe("When following the Cluster API quick-start", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: pointer.String("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				// This check ensures that owner references are resilient - i.e. correctly re-reconciled - when removed.
				framework.ValidateOwnerReferencesResilience(ctx, proxy, namespace, clusterName,
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
				// This check ensures that owner references are correctly updated to the correct apiVersion.
				framework.ValidateOwnerReferencesOnUpdate(ctx, proxy, namespace, clusterName,
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
			},
		}
	})
})

var _ = Describe("When following the Cluster API quick-start with ClusterClass [PR-Blocking] [ClusterClass]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 pointer.String("topology"),
			InfrastructureProvider: pointer.String("docker"),
			// This check ensures that owner references are resilient - i.e. correctly re-reconciled - when removed.
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				framework.ValidateOwnerReferencesResilience(ctx, proxy, namespace, clusterName,
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
				// This check ensures that owner references are correctly updated to the correct apiVersion.
				framework.ValidateOwnerReferencesOnUpdate(ctx, proxy, namespace, clusterName,
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
			},
		}
	})
})

// NOTE: This test requires an IPv6 management cluster (can be configured via IP_FAMILY=IPv6).
var _ = Describe("When following the Cluster API quick-start with IPv6 [IPv6]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 pointer.String("ipv6"),
			InfrastructureProvider: pointer.String("docker"),
		}
	})
})

var _ = Describe("When following the Cluster API quick-start with Ignition", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 pointer.String("ignition"),
			InfrastructureProvider: pointer.String("docker"),
		}
	})
})

var _ = Describe("When following the Cluster API quick-start with dualstack and ipv4 primary [IPv6]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 pointer.String("topology-dualstack-ipv4-primary"),
			InfrastructureProvider: pointer.String("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				By("Running kubetest dualstack tests")
				// Start running the dualstack test suite from kubetest.
				Expect(kubetest.Run(
					ctx,
					kubetest.RunInput{
						ClusterProxy:       proxy.GetWorkloadCluster(ctx, namespace, clusterName),
						ArtifactsDirectory: artifactFolder,
						ConfigFilePath:     "./data/kubetest/dualstack.yaml",
						// Pin the conformance image to workaround https://github.com/kubernetes-sigs/cluster-api/issues/9240 .
						// This should get dropped again when bumping to a version post v1.28.0 in `test/e2e/config/docker.yaml`.
						ConformanceImage: "gcr.io/k8s-staging-ci-images/conformance:v1.29.0-alpha.0.190_18290bfdc8fbe1",
					},
				)).To(Succeed())
			},
		}
	})
})

var _ = Describe("When following the Cluster API quick-start with dualstack and ipv6 primary [IPv6]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 pointer.String("topology-dualstack-ipv6-primary"),
			InfrastructureProvider: pointer.String("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				By("Running kubetest dualstack tests")
				// Start running the dualstack test suite from kubetest.
				Expect(kubetest.Run(
					ctx,
					kubetest.RunInput{
						ClusterProxy:       proxy.GetWorkloadCluster(ctx, namespace, clusterName),
						ArtifactsDirectory: artifactFolder,
						ConfigFilePath:     "./data/kubetest/dualstack.yaml",
						// Pin the conformance image to workaround https://github.com/kubernetes-sigs/cluster-api/issues/9240 .
						// This should get dropped again when bumping to a version post v1.28.0 in `test/e2e/config/docker.yaml`.
						ConformanceImage: "gcr.io/k8s-staging-ci-images/conformance:v1.29.0-alpha.0.190_18290bfdc8fbe1",
					},
				)).To(Succeed())
			},
		}
	})
})
