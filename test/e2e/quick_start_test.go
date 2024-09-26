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
	"k8s.io/utils/ptr"

	clusterctlcluster "sigs.k8s.io/cluster-api/cmd/clusterctl/client/cluster"
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
			InfrastructureProvider: ptr.To("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				// This check ensures that owner references are resilient - i.e. correctly re-reconciled - when removed.
				By("Checking that owner references are resilient")
				framework.ValidateOwnerReferencesResilience(ctx, proxy, namespace, clusterName, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName),
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
				// This check ensures that owner references are correctly updated to the correct apiVersion.
				By("Checking that owner references are updated to the correct API version")
				framework.ValidateOwnerReferencesOnUpdate(ctx, proxy, namespace, clusterName, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName),
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
				// This check ensures that finalizers are resilient - i.e. correctly re-reconciled - when removed.
				By("Checking that finalizers are resilient")
				framework.ValidateFinalizersResilience(ctx, proxy, namespace, clusterName, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName),
					framework.CoreFinalizersAssertionWithLegacyClusters,
					framework.KubeadmControlPlaneFinalizersAssertion,
					framework.ExpFinalizersAssertion,
					framework.DockerInfraFinalizersAssertion,
				)
				// This check ensures that the resourceVersions are stable, i.e. it verifies there are no
				// continuous reconciles when everything should be stable.
				By("Checking that resourceVersions are stable")
				framework.ValidateResourceVersionStable(ctx, proxy, namespace, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
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
			Flavor:                 ptr.To("topology"),
			InfrastructureProvider: ptr.To("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				// This check ensures that owner references are resilient - i.e. correctly re-reconciled - when removed.
				By("Checking that owner references are resilient")
				framework.ValidateOwnerReferencesResilience(ctx, proxy, namespace, clusterName, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName),
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
				// This check ensures that owner references are correctly updated to the correct apiVersion.
				By("Checking that owner references are updated to the correct API version")
				framework.ValidateOwnerReferencesOnUpdate(ctx, proxy, namespace, clusterName, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName),
					framework.CoreOwnerReferenceAssertion,
					framework.ExpOwnerReferenceAssertions,
					framework.DockerInfraOwnerReferenceAssertions,
					framework.KubeadmBootstrapOwnerReferenceAssertions,
					framework.KubeadmControlPlaneOwnerReferenceAssertions,
					framework.KubernetesReferenceAssertions,
				)
				// This check ensures that finalizers are resilient - i.e. correctly re-reconciled - when removed.
				By("Checking that finalizers are resilient")
				framework.ValidateFinalizersResilience(ctx, proxy, namespace, clusterName, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName),
					framework.CoreFinalizersAssertionWithClassyClusters,
					framework.KubeadmControlPlaneFinalizersAssertion,
					framework.ExpFinalizersAssertion,
					framework.DockerInfraFinalizersAssertion,
				)
				// This check ensures that the resourceVersions are stable, i.e. it verifies there are no
				// continuous reconciles when everything should be stable.
				By("Checking that resourceVersions are stable")
				framework.ValidateResourceVersionStable(ctx, proxy, namespace, clusterctlcluster.FilterClusterObjectsWithNameFilter(clusterName))
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
			Flavor:                 ptr.To("ipv6"),
			InfrastructureProvider: ptr.To("docker"),
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
			Flavor:                 ptr.To("ignition"),
			InfrastructureProvider: ptr.To("docker"),
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
			Flavor:                 ptr.To("topology-dualstack-ipv4-primary"),
			InfrastructureProvider: ptr.To("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				By("Running kubetest dualstack tests")
				// Start running the dualstack test suite from kubetest.
				Expect(kubetest.Run(
					ctx,
					kubetest.RunInput{
						ClusterProxy:       proxy.GetWorkloadCluster(ctx, namespace, clusterName),
						ArtifactsDirectory: artifactFolder,
						ConfigFilePath:     "./data/kubetest/dualstack.yaml",
						ClusterName:        clusterName,
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
			Flavor:                 ptr.To("topology-dualstack-ipv6-primary"),
			InfrastructureProvider: ptr.To("docker"),
			PostMachinesProvisioned: func(proxy framework.ClusterProxy, namespace, clusterName string) {
				By("Running kubetest dualstack tests")
				// Start running the dualstack test suite from kubetest.
				Expect(kubetest.Run(
					ctx,
					kubetest.RunInput{
						ClusterProxy:       proxy.GetWorkloadCluster(ctx, namespace, clusterName),
						ArtifactsDirectory: artifactFolder,
						ConfigFilePath:     "./data/kubetest/dualstack.yaml",
						ClusterName:        clusterName,
					},
				)).To(Succeed())
			},
		}
	})
})

var _ = Describe("When following the Cluster API quick-start with ClusterClass without any worker definitions [ClusterClass]", func() {
	QuickStartSpec(ctx, func() QuickStartSpecInput {
		return QuickStartSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			Flavor:                 ptr.To("topology-kcp-only"),
			InfrastructureProvider: ptr.To("docker"),
			// Note: the used template is not using the corresponding variable
			WorkerMachineCount: ptr.To[int64](0),
		}
	})
})
