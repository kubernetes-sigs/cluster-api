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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

// SelfHostedSpecInput is the input for SelfHostedSpec.
type SelfHostedSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters
	Flavor                string

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

	// SkipUpgrade skip the upgrade of the self-hosted clusters kubernetes version.
	// If true, the variable KUBERNETES_VERSION is expected to be set.
	// If false, the variables KUBERNETES_VERSION_UPGRADE_FROM, KUBERNETES_VERSION_UPGRADE_TO,
	// ETCD_VERSION_UPGRADE_TO and COREDNS_VERSION_UPGRADE_TO are expected to be set.
	// There are also (optional) variables CONTROL_PLANE_MACHINE_TEMPLATE_UPGRADE_TO and
	// WORKERS_MACHINE_TEMPLATE_UPGRADE_TO to change the infrastructure machine template
	// during the upgrade. Note that these templates need to have the clusterctl.cluster.x-k8s.io/move
	// label in order to be moved to the self hosted cluster (since they are not part of the owner chain).
	SkipUpgrade bool

	// ControlPlaneMachineCount is used in `config cluster` to configure the count of the control plane machines used in the test.
	// Default is 1.
	ControlPlaneMachineCount *int64

	// WorkerMachineCount is used in `config cluster` to configure the count of the worker machines used in the test.
	// NOTE: If the WORKER_MACHINE_COUNT var is used multiple times in the cluster template, the absolute count of
	// worker machines is a multiple of WorkerMachineCount.
	// Default is 1.
	WorkerMachineCount *int64

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
}

// SelfHostedSpec implements a test that verifies Cluster API creating a cluster, pivoting to a self-hosted cluster.
// NOTE: This test works with Clusters with and without ClusterClass.
func SelfHostedSpec(ctx context.Context, inputGetter func() SelfHostedSpecInput) {
	var (
		specName         = "self-hosted"
		input            SelfHostedSpecInput
		namespace        *corev1.Namespace
		cancelWatches    context.CancelFunc
		clusterResources *clusterctl.ApplyClusterTemplateAndWaitResult

		selfHostedClusterProxy  framework.ClusterProxy
		selfHostedNamespace     *corev1.Namespace
		selfHostedCancelWatches context.CancelFunc
		selfHostedCluster       *clusterv1.Cluster

		controlPlaneMachineCount int64
		workerMachineCount       int64

		etcdVersionUpgradeTo    string
		coreDNSVersionUpgradeTo string

		kubernetesVersion string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		if input.SkipUpgrade {
			// Use KubernetesVersion if no upgrade step is defined by test input.
			Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))

			kubernetesVersion = input.E2EConfig.MustGetVariable(KubernetesVersion)
		} else {
			Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeFrom))
			Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeTo))

			kubernetesVersion = input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeFrom)

			etcdVersionUpgradeTo = input.E2EConfig.GetVariableOrEmpty(EtcdVersionUpgradeTo)
			coreDNSVersionUpgradeTo = input.E2EConfig.GetVariableOrEmpty(CoreDNSVersionUpgradeTo)
		}

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)

		if input.ControlPlaneMachineCount == nil {
			controlPlaneMachineCount = 1
		} else {
			controlPlaneMachineCount = *input.ControlPlaneMachineCount
		}

		if input.WorkerMachineCount == nil {
			workerMachineCount = 1
		} else {
			workerMachineCount = *input.WorkerMachineCount
		}
	})

	It("Should pivot the bootstrap cluster to a self-hosted cluster", func() {
		By("Creating a workload cluster")

		workloadClusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		clusterctlVariables := map[string]string{}

		// In case the infrastructure-docker provider is installed, ensure to add the preload images variable to load the
		// controller images into the nodes.
		// NOTE: we are checking the bootstrap cluster and assuming the workload cluster will be on the same infrastructure provider.
		// Also, given that we use it to set a variable, then it is up to cluster templates to use it or not.
		hasDockerInfrastructureProvider := hasProvider(ctx, input.BootstrapClusterProxy.GetClient(), "infrastructure-docker")

		// In case the infrastructure-docker provider is installed, ensure to add the preload images variable to load the
		// controller images into the nodes.
		if hasDockerInfrastructureProvider {
			images := []string{}
			if preloadList := strings.TrimSuffix(strings.TrimPrefix(input.E2EConfig.GetVariableOrEmpty("DOCKER_PRELOAD_IMAGES"), "["), "]"); preloadList != "" {
				images = strings.Split(preloadList, ",")
			}
			for _, image := range input.E2EConfig.Images {
				images = append(images, fmt.Sprintf("%q", image.Name))
			}
			clusterctlVariables["DOCKER_PRELOAD_IMAGES"] = `[` + strings.Join(images, ",") + `]`
		}

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   input.Flavor,
				Namespace:                namespace.Name,
				ClusterName:              workloadClusterName,
				KubernetesVersion:        kubernetesVersion,
				ControlPlaneMachineCount: &controlPlaneMachineCount,
				WorkerMachineCount:       &workerMachineCount,
				ClusterctlVariables:      clusterctlVariables,
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		By("Turning the workload cluster into a management cluster")

		cluster := clusterResources.Cluster
		// Get a ClusterBroker so we can interact with the workload cluster
		selfHostedClusterProxy = input.BootstrapClusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name, framework.WithMachineLogCollector(input.BootstrapClusterProxy.GetLogCollector()))

		Byf("Creating a namespace for hosting the %s test spec", specName)
		selfHostedNamespace, selfHostedCancelWatches = framework.CreateNamespaceAndWatchEvents(ctx, framework.CreateNamespaceAndWatchEventsInput{
			Creator:   selfHostedClusterProxy.GetClient(),
			ClientSet: selfHostedClusterProxy.GetClientSet(),
			Name:      namespace.Name,
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", "bootstrap"),
		})

		if input.PostNamespaceCreated != nil {
			log.Logf("Calling postNamespaceCreated for namespace %s", selfHostedNamespace.Name)
			input.PostNamespaceCreated(selfHostedClusterProxy, selfHostedNamespace.Name)
		}

		By("Initializing the workload cluster")
		// watchesCtx is used in log streaming to be able to get canceld via cancelWatches after ending the test suite.
		watchesCtx, cancelWatches := context.WithCancel(ctx)
		defer cancelWatches()
		clusterctl.InitManagementClusterAndWatchControllerLogs(watchesCtx, clusterctl.InitManagementClusterAndWatchControllerLogsInput{
			ClusterProxy:              selfHostedClusterProxy,
			ClusterctlConfigPath:      input.ClusterctlConfigPath,
			InfrastructureProviders:   input.E2EConfig.InfrastructureProviders(),
			IPAMProviders:             input.E2EConfig.IPAMProviders(),
			RuntimeExtensionProviders: input.E2EConfig.RuntimeExtensionProviders(),
			AddonProviders:            input.E2EConfig.AddonProviders(),
			LogFolder:                 filepath.Join(input.ArtifactFolder, "clusters", cluster.Name),
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		By("Ensure API servers are stable before doing move")
		// Nb. This check was introduced to prevent doing move to self-hosted in an aggressive way and thus avoid flakes.
		// More specifically, we were observing the test failing to get objects from the API server during move, so we
		// are now testing the API servers are stable before starting move.
		Consistently(func() error {
			kubeSystem := &corev1.Namespace{}
			return input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKey{Name: "kube-system"}, kubeSystem)
		}, "5s", "100ms").Should(Succeed(), "Failed to assert bootstrap API server stability")
		Consistently(func() error {
			kubeSystem := &corev1.Namespace{}
			return selfHostedClusterProxy.GetClient().Get(ctx, client.ObjectKey{Name: "kube-system"}, kubeSystem)
		}, "5s", "100ms").Should(Succeed(), "Failed to assert self-hosted API server stability")

		By("Ensure all machines have NodeRef before doing move")
		// Ensure all machines have NodeRef before attempting to move.
		// This prevents clusterctl move failures when machines are still provisioning.
		framework.WaitForClusterMachineNodeRefs(ctx, framework.WaitForClusterMachineNodeRefsInput{
			GetLister: input.BootstrapClusterProxy.GetClient(),
			Cluster:   cluster,
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade")...)

		// Get the machines of the workloadCluster before it is moved to become self-hosted to make sure that the move did not trigger
		// any unexpected rollouts.
		preMoveMachineList := &unstructured.UnstructuredList{}
		preMoveMachineList.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("MachineList"))
		err := input.BootstrapClusterProxy.GetClient().List(
			ctx,
			preMoveMachineList,
			client.InNamespace(namespace.Name),
			client.MatchingLabels{clusterv1.ClusterNameLabel: workloadClusterName},
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to list machines before move")

		By("Moving the cluster to self hosted")
		clusterctl.Move(ctx, clusterctl.MoveInput{
			LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", "bootstrap"),
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			FromKubeconfigPath:   input.BootstrapClusterProxy.GetKubeconfigPath(),
			ToKubeconfigPath:     selfHostedClusterProxy.GetKubeconfigPath(),
			Namespace:            namespace.Name,
		})

		// Note: clusterctl should restore the managedFields to the same as before the move,
		// and thus removing any managedField entries with clusterctl as a manager. This should happen
		// for all the objects processed by move, but for sake of simplicity we test only the Cluster
		// object. The Cluster object has special processing for the paused field during the move to
		// avoid having clusterctl as the manager of the field.
		log.Logf("Ensure clusterctl does not take ownership on any fields on the self-hosted cluster")
		selfHostedCluster = framework.GetClusterByName(ctx, framework.GetClusterByNameInput{
			Getter:    selfHostedClusterProxy.GetClient(),
			Name:      cluster.Name,
			Namespace: selfHostedNamespace.Name,
		})
		hasClusterctlManagedFields := false
		for _, managedField := range selfHostedCluster.GetManagedFields() {
			if managedField.Manager == "clusterctl" {
				hasClusterctlManagedFields = true
				break
			}
		}
		Expect(hasClusterctlManagedFields).To(BeFalse(), "clusterctl should not manage any fields on the Cluster after the move")

		log.Logf("Waiting for the cluster to be reconciled after moving to self hosted")
		selfHostedCluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
			Getter:    selfHostedClusterProxy.GetClient(),
			Namespace: selfHostedNamespace.Name,
			Name:      cluster.Name,
		}, input.E2EConfig.GetIntervals(specName, "wait-cluster")...)

		controlPlane := framework.GetKubeadmControlPlaneByCluster(ctx, framework.GetKubeadmControlPlaneByClusterInput{
			Lister:      selfHostedClusterProxy.GetClient(),
			ClusterName: selfHostedCluster.Name,
			Namespace:   selfHostedCluster.Namespace,
		})
		Expect(controlPlane).ToNot(BeNil())

		// After the move: wait for MachineList to be available after the upgrade.
		log.Logf("Verify there are no unexpected rollouts")
		Eventually(func() error {
			postMoveMachineList := &unstructured.UnstructuredList{}
			postMoveMachineList.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("MachineList"))
			return selfHostedClusterProxy.GetClient().List(
				ctx,
				postMoveMachineList,
				client.InNamespace(namespace.Name),
				client.MatchingLabels{clusterv1.ClusterNameLabel: workloadClusterName},
			)
		}, "3m", "30s").ShouldNot(HaveOccurred(), "MachineList should be available after move to self-hosted cluster")

		log.Logf("Waiting for three minutes before checking if an unexpected rollout happened")
		time.Sleep(time.Minute * 3)

		// After the move: check that there were no unexpected rollouts.
		postMoveMachineList := &unstructured.UnstructuredList{}
		log.Logf("Verify there are no unexpected rollouts")
		Eventually(func() error {
			postMoveMachineList.SetGroupVersionKind(clusterv1.GroupVersion.WithKind("MachineList"))
			return selfHostedClusterProxy.GetClient().List(
				ctx,
				postMoveMachineList,
				client.InNamespace(namespace.Name),
				client.MatchingLabels{clusterv1.ClusterNameLabel: workloadClusterName},
			)
		}, "3m", "30s").ShouldNot(HaveOccurred(), "MachineList should be available after move to self-hosted cluster")
		Expect(validateMachineRollout(preMoveMachineList, postMoveMachineList)).To(BeTrue(), "Machines should not roll out after move to self-hosted cluster")

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    selfHostedClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		Byf("Verify Machines Ready condition is true")
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    selfHostedClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		if input.SkipUpgrade {
			// Only do upgrade step if defined by test input.
			return
		}

		log.Logf("Waiting for control plane to be ready")
		framework.WaitForControlPlaneAndMachinesReady(ctx, framework.WaitForControlPlaneAndMachinesReadyInput{
			GetLister:    selfHostedClusterProxy.GetClient(),
			Cluster:      clusterResources.Cluster,
			ControlPlane: clusterResources.ControlPlane,
		}, input.E2EConfig.GetIntervals(specName, "wait-control-plane")...)

		By("Upgrading the self-hosted Cluster")
		if clusterResources.Cluster.Spec.Topology.IsDefined() {
			// Cluster is using ClusterClass, upgrade via topology.
			By("Upgrading the Cluster topology")
			framework.UpgradeClusterTopologyAndWaitForUpgrade(ctx, framework.UpgradeClusterTopologyAndWaitForUpgradeInput{
				ClusterProxy:                   selfHostedClusterProxy,
				Cluster:                        clusterResources.Cluster,
				ControlPlane:                   clusterResources.ControlPlane,
				EtcdImageTag:                   etcdVersionUpgradeTo,
				DNSImageTag:                    coreDNSVersionUpgradeTo,
				MachineDeployments:             clusterResources.MachineDeployments,
				MachinePools:                   clusterResources.MachinePools,
				KubernetesUpgradeVersion:       input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
				WaitForMachinesToBeUpgraded:    input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForMachinePoolToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
				WaitForKubeProxyUpgrade:        input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForDNSUpgrade:              input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
				WaitForEtcdUpgrade:             input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
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
				ClusterProxy:                selfHostedClusterProxy,
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
			})

			if workerMachineCount > 0 {
				By("Upgrading the machine deployment")
				framework.UpgradeMachineDeploymentsAndWait(ctx, framework.UpgradeMachineDeploymentsAndWaitInput{
					ClusterProxy:                selfHostedClusterProxy,
					Cluster:                     clusterResources.Cluster,
					UpgradeVersion:              input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
					UpgradeMachineTemplate:      upgradeWorkersMachineTemplateTo,
					MachineDeployments:          clusterResources.MachineDeployments,
					WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
				})

				if len(clusterResources.MachinePools) > 0 {
					By("Upgrading the machinepool instances")
					framework.UpgradeMachinePoolAndWait(ctx, framework.UpgradeMachinePoolAndWaitInput{
						ClusterProxy:                   selfHostedClusterProxy,
						Cluster:                        clusterResources.Cluster,
						UpgradeVersion:                 input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
						WaitForMachinePoolToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-pool-upgrade"),
						MachinePools:                   clusterResources.MachinePools,
					})
				}
			}
		}

		By("Waiting until nodes are ready")
		workloadProxy := selfHostedClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)
		workloadClient := workloadProxy.GetClient()
		framework.WaitForNodesReady(ctx, framework.WaitForNodesReadyInput{
			Lister:            workloadClient,
			KubernetesVersion: input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo),
			Count:             int(clusterResources.ExpectedTotalNodes()),
			WaitForNodesReady: input.E2EConfig.GetIntervals(specName, "wait-nodes-ready"),
		})

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    selfHostedClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		Byf("Verify Machines Ready condition is true")
		framework.VerifyMachinesReady(ctx, framework.VerifyMachinesReadyInput{
			Lister:    selfHostedClusterProxy.GetClient(),
			Name:      clusterResources.Cluster.Name,
			Namespace: clusterResources.Cluster.Namespace,
		})

		By("PASSED!")
	})

	AfterEach(func() {
		if selfHostedNamespace != nil {
			// Dump all Cluster API related resources to artifacts before pivoting back.
			framework.DumpAllResourcesAndLogs(ctx, selfHostedClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, clusterResources.Cluster)
		}
		if selfHostedCluster != nil {
			By("Ensure API servers are stable before doing move")
			// Nb. This check was introduced to prevent doing move back to bootstrap in an aggressive way and thus avoid flakes.
			// More specifically, we were observing the test failing to get objects from the API server during move, so we
			// are now testing the API servers are stable before starting move.
			Consistently(func() error {
				kubeSystem := &corev1.Namespace{}
				return input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKey{Name: "kube-system"}, kubeSystem)
			}, "5s", "100ms").Should(Succeed(), "Failed to assert bootstrap API server stability")
			Consistently(func() error {
				kubeSystem := &corev1.Namespace{}
				return selfHostedClusterProxy.GetClient().Get(ctx, client.ObjectKey{Name: "kube-system"}, kubeSystem)
			}, "5s", "100ms").Should(Succeed(), "Failed to assert self-hosted API server stability")

			By("Ensure all machines have NodeRef before doing move back")
			// Ensure all machines have NodeRef before attempting to move back to bootstrap.
			// This prevents clusterctl move failures when machines are still provisioning.
			framework.WaitForClusterMachineNodeRefs(ctx, framework.WaitForClusterMachineNodeRefsInput{
				GetLister: selfHostedClusterProxy.GetClient(),
				Cluster:   selfHostedCluster,
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade")...)

			By("Moving the cluster back to bootstrap")
			clusterctl.Move(ctx, clusterctl.MoveInput{
				LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", clusterResources.Cluster.Name),
				ClusterctlConfigPath: input.ClusterctlConfigPath,
				FromKubeconfigPath:   selfHostedClusterProxy.GetKubeconfigPath(),
				ToKubeconfigPath:     input.BootstrapClusterProxy.GetKubeconfigPath(),
				Namespace:            selfHostedNamespace.Name,
			})

			log.Logf("Waiting for the cluster to be reconciled after moving back to bootstrap")
			clusterResources.Cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
				Getter:    input.BootstrapClusterProxy.GetClient(),
				Namespace: namespace.Name,
				Name:      clusterResources.Cluster.Name,
			}, input.E2EConfig.GetIntervals(specName, "wait-cluster")...)
		}
		if selfHostedCancelWatches != nil {
			selfHostedCancelWatches()
		}

		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

func hasProvider(ctx context.Context, c client.Client, providerName string) bool {
	providerList := clusterctlv1.ProviderList{}
	Eventually(func() error {
		return c.List(ctx, &providerList)
	}, "1m", "5s").Should(Succeed(), "Failed to list the Providers")

	for _, provider := range providerList.Items {
		if provider.GetName() == providerName {
			return true
		}
	}
	return false
}
