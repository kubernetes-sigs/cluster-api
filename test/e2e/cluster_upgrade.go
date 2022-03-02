/*
Copyright 2021 The Kubernetes Authors.

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

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/framework/kubetest"
	"sigs.k8s.io/cluster-api/util"
)

// ClusterUpgradeConformanceSpecInput is the input for ClusterUpgradeConformanceSpec.
type ClusterUpgradeConformanceSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	SkipConformanceTests  bool

	// ControlPlaneMachineCount is used in `config cluster` to configure the count of the control plane machines used in the test.
	// Default is 1.
	ControlPlaneMachineCount *int64
	// WorkerMachineCount is used in `config cluster` to configure the count of the worker machines used in the test.
	// NOTE: If the WORKER_MACHINE_COUNT var is used multiple times in the cluster template, the absolute count of
	// worker machines is a multiple of WorkerMachineCount.
	// Default is 2.
	WorkerMachineCount *int64

	// Flavor to use when creating the cluster for testing, "upgrades" is used if not specified.
	Flavor *string
}

// ClusterUpgradeConformanceSpec implements a spec that upgrades a cluster and runs the Kubernetes conformance suite.
// Upgrading a cluster refers to upgrading the control-plane and worker nodes (managed by MD and machine pools).
// NOTE: This test only works with a KubeadmControlPlane.
// NOTE: This test works with Clusters with and without ClusterClass.
// When using ClusterClass the ClusterClass must have the variables "etcdImageTag" and "coreDNSImageTag" of type string.
// Those variables should have corresponding patches which set the etcd and CoreDNS tags in KCP.
func ClusterUpgradeConformanceSpec(ctx context.Context, inputGetter func() ClusterUpgradeConformanceSpecInput) {
	const (
		kubetestConfigurationVariable = "KUBETEST_CONFIGURATION"
		specName                      = "k8s-upgrade-and-conformance"
	)
	var (
		input         ClusterUpgradeConformanceSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc

		controlPlaneMachineCount int64
		workerMachineCount       int64

		clusterResources       *clusterctl.ApplyClusterTemplateAndWaitResult
		kubetestConfigFilePath string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeFrom))
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeTo))
		Expect(input.E2EConfig.Variables).To(HaveKey(EtcdVersionUpgradeTo))
		Expect(input.E2EConfig.Variables).To(HaveKey(CoreDNSVersionUpgradeTo))

		Expect(input.E2EConfig.Variables).To(HaveKey(kubetestConfigurationVariable), "% spec requires a %s variable to be defined in the config file", specName, kubetestConfigurationVariable)
		kubetestConfigFilePath = input.E2EConfig.GetVariable(kubetestConfigurationVariable)
		Expect(kubetestConfigFilePath).To(BeAnExistingFile(), "%s should be a valid kubetest config file")

		if input.ControlPlaneMachineCount == nil {
			controlPlaneMachineCount = 1
		} else {
			controlPlaneMachineCount = *input.ControlPlaneMachineCount
		}

		if input.WorkerMachineCount == nil {
			workerMachineCount = 2
		} else {
			workerMachineCount = *input.WorkerMachineCount
		}

		// Setup a Namespace where to host objects for this spec and create a watcher for the Namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create and upgrade a workload cluster and run kubetest", func() {
		By("Creating a workload cluster")

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   pointer.StringDeref(input.Flavor, "upgrades"),
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersionUpgradeFrom),
				ControlPlaneMachineCount: pointer.Int64Ptr(controlPlaneMachineCount),
				WorkerMachineCount:       pointer.Int64Ptr(workerMachineCount),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			WaitForMachinePools:          input.E2EConfig.GetIntervals(specName, "wait-machine-pool-nodes"),
		}, clusterResources)

		if clusterResources.Cluster.Spec.Topology != nil {
			// Cluster is using ClusterClass, upgrade via topology.
			By("Upgrading the Cluster topology")
			framework.UpgradeClusterTopologyAndWaitForUpgrade(ctx, framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
				ClusterProxy:                input.BootstrapClusterProxy,
				Cluster:                     clusterResources.Cluster,
				ControlPlane:                clusterResources.ControlPlane,
				EtcdImageTag:                input.E2EConfig.GetVariable(EtcdVersionUpgradeTo),
				DNSImageTag:                 input.E2EConfig.GetVariable(CoreDNSVersionUpgradeTo),
				MachineDeployments:          clusterResources.MachineDeployments,
				KubernetesUpgradeVersion:    input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
				WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForKubeProxyUpgrade:     input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForDNSUpgrade:           input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForEtcdUpgrade:          input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			})
		} else {
			// Cluster is not using ClusterClass, upgrade via individual resources.
			By("Upgrading the Kubernetes control-plane")
			var (
				upgradeCPMachineTemplateTo      *string
				upgradeWorkersMachineTemplateTo *string
			)

			if input.E2EConfig.HasVariable(CPMachineTemplateUpgradeTo) {
				upgradeCPMachineTemplateTo = pointer.StringPtr(input.E2EConfig.GetVariable(CPMachineTemplateUpgradeTo))
			}

			if input.E2EConfig.HasVariable(WorkersMachineTemplateUpgradeTo) {
				upgradeWorkersMachineTemplateTo = pointer.StringPtr(input.E2EConfig.GetVariable(WorkersMachineTemplateUpgradeTo))
			}

			framework.UpgradeControlPlaneAndWaitForUpgrade(ctx, framework.UpgradeControlPlaneAndWaitForUpgradeInput{
				ClusterProxy:                input.BootstrapClusterProxy,
				Cluster:                     clusterResources.Cluster,
				ControlPlane:                clusterResources.ControlPlane,
				EtcdImageTag:                input.E2EConfig.GetVariable(EtcdVersionUpgradeTo),
				DNSImageTag:                 input.E2EConfig.GetVariable(CoreDNSVersionUpgradeTo),
				KubernetesUpgradeVersion:    input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
				UpgradeMachineTemplate:      upgradeCPMachineTemplateTo,
				WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForKubeProxyUpgrade:     input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForDNSUpgrade:           input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForEtcdUpgrade:          input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			})

			if workerMachineCount > 0 {
				By("Upgrading the machine deployment")
				framework.UpgradeMachineDeploymentsAndWait(ctx, framework.UpgradeMachineDeploymentsAndWaitInput{
					ClusterProxy:                input.BootstrapClusterProxy,
					Cluster:                     clusterResources.Cluster,
					UpgradeVersion:              input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
					UpgradeMachineTemplate:      upgradeWorkersMachineTemplateTo,
					MachineDeployments:          clusterResources.MachineDeployments,
					WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
				})
			}
		}

		// Only attempt to upgrade MachinePools if they were provided in the template.
		if len(clusterResources.MachinePools) > 0 && workerMachineCount > 0 {
			By("Upgrading the machinepool instances")
			framework.UpgradeMachinePoolAndWait(ctx, framework.UpgradeMachinePoolAndWaitInput{
				ClusterProxy:                   input.BootstrapClusterProxy,
				Cluster:                        clusterResources.Cluster,
				UpgradeVersion:                 input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
				WaitForMachinePoolToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
				MachinePools:                   clusterResources.MachinePools,
			})
		}

		By("Waiting until nodes are ready")
		workloadProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)
		workloadClient := workloadProxy.GetClient()
		framework.WaitForNodesReady(ctx, framework.WaitForNodesReadyInput{
			Lister:            workloadClient,
			KubernetesVersion: input.E2EConfig.GetVariable(KubernetesVersionUpgradeTo),
			Count:             int(clusterResources.ExpectedTotalNodes()),
			WaitForNodesReady: input.E2EConfig.GetIntervals(specName, "wait-nodes-ready"),
		})

		if !input.SkipConformanceTests {
			By("Running conformance tests")
			// Start running the conformance test suite.
			err := kubetest.Run(
				ctx,
				kubetest.RunInput{
					ClusterProxy:       workloadProxy,
					NumberOfNodes:      int(clusterResources.ExpectedWorkerNodes()),
					ArtifactsDirectory: input.ArtifactFolder,
					ConfigFilePath:     kubetestConfigFilePath,
					GinkgoNodes:        int(clusterResources.ExpectedWorkerNodes()),
				},
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to run Kubernetes conformance")
		}

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec Namespace, then cleanups the cluster object and the spec Namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
