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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

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

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// Allows to inject a function to be run before checking control-plane machines to be upgraded.
	// If not specified, this is a no-op.
	PreWaitForControlPlaneToBeUpgraded func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace, workloadClusterName string)

	// ClusterctlVariables allows injecting variables to the cluster template.
	// If not specified, this is a no-op.
	ClusterctlVariables map[string]string
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

		etcdVersionUpgradeTo    string
		coreDNSVersionUpgradeTo string

		clusterResources       *clusterctl.ApplyClusterTemplateAndWaitResult
		kubetestConfigFilePath string

		clusterName string
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

		if !input.SkipConformanceTests {
			Expect(input.E2EConfig.Variables).To(HaveKey(kubetestConfigurationVariable), "% spec requires a %s variable to be defined in the config file", specName, kubetestConfigurationVariable)
			kubetestConfigFilePath = input.E2EConfig.MustGetVariable(kubetestConfigurationVariable)
			Expect(kubetestConfigFilePath).To(BeAnExistingFile(), "%s should be a valid kubetest config file")
		}

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

		etcdVersionUpgradeTo = input.E2EConfig.GetVariableOrEmpty(EtcdVersionUpgradeTo)
		coreDNSVersionUpgradeTo = input.E2EConfig.GetVariableOrEmpty(CoreDNSVersionUpgradeTo)

		// Setup a Namespace where to host objects for this spec and create a watcher for the Namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create and upgrade a workload cluster and eventually run kubetest", func() {
		By("Creating a workload cluster")

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		clusterName = fmt.Sprintf("%s-%s", specName, util.RandomString(6))

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				ClusterctlVariables:      input.ClusterctlVariables,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   ptr.Deref(input.Flavor, "upgrades"),
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeFrom),
				ControlPlaneMachineCount: ptr.To[int64](controlPlaneMachineCount),
				WorkerMachineCount:       ptr.To[int64](workerMachineCount),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
			WaitForMachinePools:          input.E2EConfig.GetIntervals(specName, "wait-machine-pool-nodes"),
		}, clusterResources)

		By("Recording Machines before the upgrade")
		preUpgradeMachines := framework.GetMachinesByCluster(ctx, framework.GetMachinesByClusterInput{
			Lister:      input.BootstrapClusterProxy.GetClient(),
			ClusterName: clusterResources.Cluster.Name,
			Namespace:   namespace.Name,
		})
		preUpgradeMachineNames := sets.New[string]()
		for _, m := range preUpgradeMachines {
			// Excluding MachinePool Machines as they are harder to reason about, e.g.
			// we don't know if MachinePool Machines are supported by the infra provider.
			if isMachinePoolMachine(m) {
				continue
			}
			preUpgradeMachineNames.Insert(m.Name)
		}

		// Continuously track Machines for the duration of the upgrade so that we can check
		// if there have been any unexpected Machine creations.
		stopTrackingMachines := trackMachineNames(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster.Name, namespace.Name)

		if clusterResources.Cluster.Spec.Topology.IsDefined() {
			// Cluster is using ClusterClass, upgrade via topology.
			By("Upgrading the Cluster topology")
			framework.UpgradeClusterTopologyAndWaitForUpgrade(ctx, framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
				ClusterProxy:                         input.BootstrapClusterProxy,
				Cluster:                              clusterResources.Cluster,
				ControlPlane:                         clusterResources.ControlPlane,
				EtcdImageTag:                         etcdVersionUpgradeTo,
				DNSImageTag:                          coreDNSVersionUpgradeTo,
				MachineDeployments:                   clusterResources.MachineDeployments,
				MachinePools:                         clusterResources.MachinePools,
				KubernetesUpgradeVersion:             input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
				WaitForControlPlaneToBeUpgraded:      input.E2EConfig.GetIntervals(specName, "wait-control-plane-upgrade"),
				WaitForMachineDeploymentToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-deployment-upgrade"),
				WaitForMachinePoolToBeUpgraded:       input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
				WaitForKubeProxyUpgrade:              input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForDNSUpgrade:                    input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForEtcdUpgrade:                   input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				PreWaitForControlPlaneToBeUpgraded: func() {
					if input.PreWaitForControlPlaneToBeUpgraded != nil {
						input.PreWaitForControlPlaneToBeUpgraded(input.BootstrapClusterProxy, namespace.Name, clusterName)
					}
				},
			})
		} else {
			// Cluster is not using ClusterClass, upgrade via individual resources.
			By("Upgrading the Kubernetes control-plane")
			var (
				upgradeCPMachineTemplateTo      *string
				upgradeWorkersMachineTemplateTo *string
			)

			if input.E2EConfig.HasVariable(CPMachineTemplateUpgradeTo) {
				upgradeCPMachineTemplateTo = ptr.To(input.E2EConfig.MustGetVariable(CPMachineTemplateUpgradeTo))
			}

			if input.E2EConfig.HasVariable(WorkersMachineTemplateUpgradeTo) {
				upgradeWorkersMachineTemplateTo = ptr.To(input.E2EConfig.MustGetVariable(WorkersMachineTemplateUpgradeTo))
			}

			framework.UpgradeControlPlaneAndWaitForUpgrade(ctx, framework.UpgradeControlPlaneAndWaitForUpgradeInput{
				ClusterProxy:                input.BootstrapClusterProxy,
				Cluster:                     clusterResources.Cluster,
				ControlPlane:                clusterResources.ControlPlane,
				EtcdImageTag:                etcdVersionUpgradeTo,
				DNSImageTag:                 coreDNSVersionUpgradeTo,
				KubernetesUpgradeVersion:    input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
				UpgradeMachineTemplate:      upgradeCPMachineTemplateTo,
				WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForKubeProxyUpgrade:     input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForDNSUpgrade:           input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForEtcdUpgrade:          input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				PreWaitForControlPlaneToBeUpgraded: func() {
					if input.PreWaitForControlPlaneToBeUpgraded != nil {
						input.PreWaitForControlPlaneToBeUpgraded(input.BootstrapClusterProxy, namespace.Name, clusterName)
					}
				},
			})

			if workerMachineCount > 0 {
				By("Upgrading the machine deployment")
				framework.UpgradeMachineDeploymentsAndWait(ctx, framework.UpgradeMachineDeploymentsAndWaitInput{
					ClusterProxy:                input.BootstrapClusterProxy,
					Cluster:                     clusterResources.Cluster,
					UpgradeVersion:              input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
					UpgradeMachineTemplate:      upgradeWorkersMachineTemplateTo,
					MachineDeployments:          clusterResources.MachineDeployments,
					WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
				})

				if len(clusterResources.MachinePools) > 0 {
					By("Upgrading the machinepool instances")
					framework.UpgradeMachinePoolAndWait(ctx, framework.UpgradeMachinePoolAndWaitInput{
						ClusterProxy:                   input.BootstrapClusterProxy,
						Cluster:                        clusterResources.Cluster,
						UpgradeVersion:                 input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
						WaitForMachinePoolToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
						MachinePools:                   clusterResources.MachinePools,
					})
				}
			}
		}

		By("Waiting until nodes are ready")
		workloadProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)
		workloadClient := workloadProxy.GetClient()
		framework.WaitForNodesReady(ctx, framework.WaitForNodesReadyInput{
			Lister:            workloadClient,
			KubernetesVersion: input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
			Count:             int(clusterResources.ExpectedTotalNodes()),
			WaitForNodesReady: input.E2EConfig.GetIntervals(specName, "wait-nodes-ready"),
		})

		Byf("Verify the expected number of Machines were created by the upgrade")
		observedMachineNames := stopTrackingMachines()
		newMachineNames := observedMachineNames.Difference(preUpgradeMachineNames)
		Expect(newMachineNames).To(HaveLen(len(preUpgradeMachineNames)),
			"Expected %d new Machines to be created by the upgrade, got %d: pre-upgrade Machines: %v, post-upgrade Machines: %v",
			len(preUpgradeMachineNames), len(newMachineNames), sets.List(preUpgradeMachineNames), sets.List(newMachineNames))

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    input.BootstrapClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		Byf("Verify Machines Ready condition is true")
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    input.BootstrapClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
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
					ClusterName:        clusterResources.Cluster.GetName(),
				},
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to run Kubernetes conformance")
		}

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec Namespace, then cleanups the cluster object and the spec Namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

// trackMachineNames polls for Machines belonging to clusterName in the background and returns a function
// that stops the polling and returns the set of every Machine name that was observed while polling was active.
func trackMachineNames(ctx context.Context, c client.Client, clusterName, namespace string) func() sets.Set[string] {
	var mu sync.Mutex
	observed := sets.New[string]()

	poll := func() {
		machineList := &clusterv1.MachineList{}
		if err := c.List(ctx, machineList, client.InNamespace(namespace), client.MatchingLabels{clusterv1.ClusterNameLabel: clusterName}); err != nil {
			// Ignore transient list errors, the next tick (or the final poll on stop) will pick up the state.
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, m := range machineList.Items {
			if isMachinePoolMachine(m) {
				// Excluding MachinePool Machines as they are harder to reason about, e.g.
				// we don't know if MachinePool Machines are supported by the infra provider.
				continue
			}
			observed.Insert(m.Name)
		}
	}

	stopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer GinkgoRecover()
		defer close(done)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			poll()
			select {
			case <-stopCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return func() sets.Set[string] {
		cancel()
		<-done

		// Final poll to catch the state right before stopping (uses the original, non-cancelled ctx).
		poll()

		mu.Lock()
		defer mu.Unlock()
		return observed.Clone()
	}
}

// isMachinePoolMachine returns true if m is owned by a MachinePool.
func isMachinePoolMachine(m clusterv1.Machine) bool {
	_, ok := m.Labels[clusterv1.MachinePoolNameLabel]
	return ok
}
