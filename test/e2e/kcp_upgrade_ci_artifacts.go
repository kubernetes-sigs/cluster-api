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
	"context"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/e2e/internal/setup"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/framework/kubernetesversions"
	"sigs.k8s.io/cluster-api/util"
)

// KCPUpgradeCIArtifactsSpecInput is the input for KCPUpgradeSpec.
type KCPUpgradeCIArtifactsSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
}

// KCPUpgradeCIArtifactsSpec implements a test that verifies KCP to properly upgrade a control plane with 3 machines.
func KCPUpgradeCIArtifactsSpec(ctx context.Context, inputGetter func() KCPUpgradeCIArtifactsSpecInput) {
	var (
		specName      = "kcp-upgrade-ci-artifacts"
		input         KCPUpgradeCIArtifactsSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		cluster       *clusterv1.Cluster
		ciVersion     string
		lastVersion   string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		os.Setenv("USE_CI_ARTIFACTS", "false")
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)
		var err error
		ciVersion, err = kubernetesversions.LatestCIRelease()
		Expect(err).NotTo(HaveOccurred())
		lastVersion, err = kubernetesversions.PreviousMinorRelease(ciVersion)
		os.Setenv("DOCKER_NODE_VERSION", lastVersion)
		Expect(err).NotTo(HaveOccurred())
		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setup.CreateSpecNamespace(
			ctx,
			setup.CreateSpecNamespaceInput{
				ArtifactsDirectory: input.ArtifactFolder,
				ClusterProxy:       input.BootstrapClusterProxy,
				SpecName:           specName,
			},
		)
	})

	It("Should successfully upgrade Kubernetes to the latest main branch version", func() {
		clusterName := fmt.Sprintf("cluster-%s", util.RandomString(6))
		By("Creating a workload cluster")
		cluster, _, _ = clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   "with-ci-artifacts",
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        lastVersion,
				ControlPlaneMachineCount: pointer.Int64Ptr(1),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		os.Setenv("USE_CI_ARTIFACTS", "true")

		By("Upgrading Kubernetes")
		cluster, _, _ = clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   "with-ci-artifacts",
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        ciVersion,
				ControlPlaneMachineCount: pointer.Int64Ptr(1),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
		})

		By("Waiting for control plane to be up to date")
		framework.WaitForControlPlaneMachinesToBeUpgraded(ctx, framework.WaitForControlPlaneMachinesToBeUpgradedInput{
			Cluster:                  cluster,
			MachineCount:             1,
			KubernetesUpgradeVersion: ciVersion,
			Lister:                   input.BootstrapClusterProxy.GetClient(),
		},
			input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade")...,
		)

		By("PASSED!")
	})

	It("Should successfully upgrade Kubernetes to the latest main branch version in a HA cluster", func() {
		clusterName := fmt.Sprintf("clusterha-upgrade-%s", util.RandomString(6))
		By("Creating a workload cluster")

		cluster, _, _ = clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   "with-ci-artifacts",
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        lastVersion,
				ControlPlaneMachineCount: pointer.Int64Ptr(3),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Upgrading Kubernetes")

		cluster, _, _ = clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   "with-ci-artifacts",
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        ciVersion,
				ControlPlaneMachineCount: pointer.Int64Ptr(3),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Waiting for control plane to be up to date")
		framework.WaitForControlPlaneMachinesToBeUpgraded(ctx, framework.WaitForControlPlaneMachinesToBeUpgradedInput{
			Cluster:                  cluster,
			MachineCount:             3,
			KubernetesUpgradeVersion: ciVersion,
			Lister:                   input.BootstrapClusterProxy.GetClient(),
		},
			"20m",
		)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		setup.DumpSpecResourcesAndCleanup(
			ctx,
			setup.DumpSpecResourcesAndCleanupInput{
				SpecName:           specName,
				ClusterProxy:       input.BootstrapClusterProxy,
				ArtifactsDirectory: input.ArtifactFolder,
				Namespace:          namespace,
				CancelWatches:      cancelWatches,
				Cluster:            cluster,
				IntervalsGetter:    input.E2EConfig.GetIntervals,
				SkipCleanup:        input.SkipCleanup,
			},
		)
	})
}
