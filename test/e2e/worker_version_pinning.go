/*
Copyright 2026 The Kubernetes Authors.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

// WorkerVersionPinningSpecInput is the input for WorkerVersionPinningSpec.
type WorkerVersionPinningSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// Flavor to use when creating the cluster for testing.
	Flavor string

	// InfrastructureProvider specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	InfrastructureProvider *string

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// ExtensionConfigName is the name of the ExtensionConfig.
	ExtensionConfigName string

	// ExtensionServiceNamespace is the namespace where the service for the Runtime SDK is located
	// and is used to configure in the test-namespace scoped ExtensionConfig.
	ExtensionServiceNamespace string

	// ExtensionServiceName is the name of the service to configure in the test-namespace scoped ExtensionConfig.
	ExtensionServiceName string
}

// WorkerVersionPinningSpec verifies that a MachineDeployment pinning its own Kubernetes version is
// upgraded independently of Cluster.spec.topology.version:
//   - pinning a MachineDeployment to the version it already runs does not roll it,
//   - a Cluster upgrade rolls only the MachineDeployments that do not pin a version,
//   - a pinned MachineDeployment still scales while it holds back, and new Machines join the
//     upgraded control plane at the pinned version,
//   - raising the pin rolls only that MachineDeployment,
//   - unpinning is allowed once the pin is equal to Cluster.spec.topology.version.
//
// It also verifies that the validating webhook rejects invalid version changes.
func WorkerVersionPinningSpec(ctx context.Context, inputGetter func() WorkerVersionPinningSpecInput) {
	const specName = "worker-version-pinning"
	const pinnedMDTopologyName = "md-0"
	const clusterManagedMDTopologyName = "md-1"

	var (
		input            WorkerVersionPinningSpecInput
		namespace        *corev1.Namespace
		cancelWatches    context.CancelFunc
		clusterResources *clusterctl.ApplyClusterTemplateAndWaitResult
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

		if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" && input.ExtensionConfigName == "" {
			input.ExtensionConfigName = specName
		}

		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should upgrade a pinned MachineDeployment independently of the Cluster", func() {
		if input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
			By("Deploy Test Extension ExtensionConfig")
			extensionConfig := extensionConfig(input.ExtensionConfigName, input.ExtensionServiceNamespace, input.ExtensionServiceName, true, false, namespace.Name)
			Expect(input.BootstrapClusterProxy.GetClient().Create(ctx, extensionConfig)).To(Succeed(), "Failed to create the ExtensionConfig")
		}

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		kubernetesVersionUpgradeFrom := input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeFrom)
		kubernetesVersionUpgradeTo := input.E2EConfig.MustGetVariable(KubernetesVersionUpgradeTo)
		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))

		By("Creating a workload cluster")
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   input.Flavor,
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        kubernetesVersionUpgradeFrom,
				ControlPlaneMachineCount: ptr.To[int64](1),
				WorkerMachineCount:       ptr.To[int64](1),
				ClusterctlVariables: map[string]string{
					"EXTENSION_CONFIG_NAME": input.ExtensionConfigName,
				},
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		cluster := clusterResources.Cluster
		Expect(cluster.Spec.Topology.IsDefined()).To(BeTrue(), "Cluster must use a managed topology")
		mgmtClient := input.BootstrapClusterProxy.GetClient()

		// patchCluster applies mutate to the Cluster and returns the resulting error, if any.
		patchCluster := func(mutate func(cluster *clusterv1.Cluster)) error {
			Expect(mgmtClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
			patchHelper, err := patch.NewHelper(cluster, mgmtClient)
			Expect(err).ToNot(HaveOccurred())
			mutate(cluster)
			return patchHelper.Patch(ctx, cluster)
		}

		// patchMDTopology applies mutate to the MachineDeployment topology with the given name.
		patchMDTopology := func(mdTopologyName string, mutate func(mdTopology *clusterv1.MachineDeploymentTopology)) error {
			return patchCluster(func(cluster *clusterv1.Cluster) {
				for i, mdTopology := range cluster.Spec.Topology.Workers.MachineDeployments {
					if mdTopology.Name == mdTopologyName {
						mutate(&cluster.Spec.Topology.Workers.MachineDeployments[i])
						return
					}
				}
				Fail(fmt.Sprintf("MachineDeployment topology %s not found", mdTopologyName))
			})
		}

		// machineDeployment returns the MachineDeployment for the given MachineDeployment topology name.
		machineDeployment := func(mdTopologyName string) *clusterv1.MachineDeployment {
			mds := &clusterv1.MachineDeploymentList{}
			Expect(mgmtClient.List(ctx, mds, client.InNamespace(cluster.Namespace), client.MatchingLabels{
				clusterv1.ClusterNameLabel:                          cluster.Name,
				clusterv1.ClusterTopologyMachineDeploymentNameLabel: mdTopologyName,
			})).To(Succeed())
			Expect(mds.Items).To(HaveLen(1), "expected exactly one MachineDeployment for topology %s", mdTopologyName)
			return &mds.Items[0]
		}

		// machineNames returns the names of the Machines of the given MachineDeployment.
		machineNames := func(md *clusterv1.MachineDeployment) []string {
			machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
				Lister:            mgmtClient,
				ClusterName:       cluster.Name,
				Namespace:         cluster.Namespace,
				MachineDeployment: *md,
			})
			names := make([]string, 0, len(machines))
			for _, m := range machines {
				names = append(names, m.Name)
			}
			return names
		}

		By("Pinning the MachineDeployment to the version it is already running")
		pinnedMachinesBeforePin := machineNames(machineDeployment(pinnedMDTopologyName))
		Expect(pinnedMachinesBeforePin).To(HaveLen(1))
		Expect(patchMDTopology(pinnedMDTopologyName, func(mdTopology *clusterv1.MachineDeploymentTopology) {
			mdTopology.Version = kubernetesVersionUpgradeFrom
		})).To(Succeed(), "Pinning a MachineDeployment to its current version should be allowed")

		By("Verifying pinning did not roll the MachineDeployment")
		// Give the topology controller time to reconcile, then assert nothing rolled.
		Consistently(func(g Gomega) {
			md := machineDeployment(pinnedMDTopologyName)
			g.Expect(md.Spec.Template.Spec.Version).To(Equal(kubernetesVersionUpgradeFrom))
			g.Expect(machineNames(md)).To(ConsistOf(pinnedMachinesBeforePin))
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		By("Verifying the webhook rejects a pin above the control plane version")
		Expect(patchMDTopology(pinnedMDTopologyName, func(mdTopology *clusterv1.MachineDeploymentTopology) {
			mdTopology.Version = kubernetesVersionUpgradeTo
		})).To(MatchError(ContainSubstring("version cannot be greater than the control plane version")))

		By("Upgrading the Cluster")
		Expect(patchCluster(func(cluster *clusterv1.Cluster) {
			cluster.Spec.Topology.Version = kubernetesVersionUpgradeTo
		})).To(Succeed())

		By("Waiting for the control plane to be upgraded")
		framework.WaitForControlPlaneMachinesToBeUpgraded(ctx, framework.WaitForControlPlaneMachinesToBeUpgradedInput{
			Lister:                   mgmtClient,
			Cluster:                  cluster,
			MachineCount:             1,
			KubernetesUpgradeVersion: kubernetesVersionUpgradeTo,
		}, input.E2EConfig.GetIntervals(specName, "wait-control-plane-upgrade")...)

		By("Waiting for the cluster-managed MachineDeployment to be upgraded")
		framework.WaitForMachineDeploymentMachinesToBeUpgraded(ctx, framework.WaitForMachineDeploymentMachinesToBeUpgradedInput{
			Lister:                   mgmtClient,
			Cluster:                  cluster,
			MachineCount:             1,
			KubernetesUpgradeVersion: kubernetesVersionUpgradeTo,
			MachineDeployment:        *machineDeployment(clusterManagedMDTopologyName),
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...)

		By("Verifying the pinned MachineDeployment held back")
		pinnedMD := machineDeployment(pinnedMDTopologyName)
		Expect(pinnedMD.Spec.Template.Spec.Version).To(Equal(kubernetesVersionUpgradeFrom))
		Expect(machineNames(pinnedMD)).To(ConsistOf(pinnedMachinesBeforePin), "the pinned MachineDeployment should not have rolled")

		By("Scaling the pinned MachineDeployment while it is behind the control plane")
		// This only works if the pinned MachineDeployment keeps reconciling and if the new Machine is
		// allowed to join the upgraded control plane at the older version.
		Expect(patchMDTopology(pinnedMDTopologyName, func(mdTopology *clusterv1.MachineDeploymentTopology) {
			mdTopology.Replicas = ptr.To[int32](2)
		})).To(Succeed())

		By("Verifying the new Machine joined at the pinned version")
		// Note: the MachineDeployment is re-read on every attempt, because the topology controller
		// has to propagate the new replica count to it first.
		Eventually(func(g Gomega) {
			md := machineDeployment(pinnedMDTopologyName)
			g.Expect(md.Spec.Template.Spec.Version).To(Equal(kubernetesVersionUpgradeFrom))
			g.Expect(md.Spec.Replicas).To(Equal(ptr.To[int32](2)))

			machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
				Lister:            mgmtClient,
				ClusterName:       cluster.Name,
				Namespace:         cluster.Namespace,
				MachineDeployment: *md,
			})
			g.Expect(machines).To(HaveLen(2))
			for _, m := range machines {
				g.Expect(m.Spec.Version).To(Equal(kubernetesVersionUpgradeFrom))
				g.Expect(m.Status.NodeRef.Name).ToNot(BeEmpty(), "Machine %s should have joined the cluster", m.Name)
			}
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...).Should(Succeed())
		log.Logf("Pinned MachineDeployment Machines after scaling: %v", machineNames(machineDeployment(pinnedMDTopologyName)))

		By("Verifying the webhook rejects unpinning while the MachineDeployment is behind the Cluster")
		Expect(patchMDTopology(pinnedMDTopologyName, func(mdTopology *clusterv1.MachineDeploymentTopology) {
			mdTopology.Version = ""
		})).To(MatchError(ContainSubstring("version can be unset only if it is equal to Cluster.spec.topology.version")))

		By("Verifying the webhook rejects decreasing the pin")
		Expect(patchMDTopology(pinnedMDTopologyName, func(mdTopology *clusterv1.MachineDeploymentTopology) {
			mdTopology.Version = "v1.0.0"
		})).To(MatchError(ContainSubstring("version cannot be decreased")))

		By("Raising the pin to the Cluster version")
		Expect(patchMDTopology(pinnedMDTopologyName, func(mdTopology *clusterv1.MachineDeploymentTopology) {
			mdTopology.Version = kubernetesVersionUpgradeTo
		})).To(Succeed())

		framework.WaitForMachineDeploymentMachinesToBeUpgraded(ctx, framework.WaitForMachineDeploymentMachinesToBeUpgradedInput{
			Lister:                   mgmtClient,
			Cluster:                  cluster,
			MachineCount:             2,
			KubernetesUpgradeVersion: kubernetesVersionUpgradeTo,
			MachineDeployment:        *machineDeployment(pinnedMDTopologyName),
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...)

		By("Unpinning the MachineDeployment")
		Expect(patchMDTopology(pinnedMDTopologyName, func(mdTopology *clusterv1.MachineDeploymentTopology) {
			mdTopology.Version = ""
		})).To(Succeed(), "Unpinning should be allowed once the pin is equal to Cluster.spec.topology.version")

		By("Verifying unpinning did not roll the MachineDeployment")
		Consistently(func(g Gomega) {
			md := machineDeployment(pinnedMDTopologyName)
			g.Expect(md.Spec.Template.Spec.Version).To(Equal(kubernetesVersionUpgradeTo))
			g.Expect(machineNames(md)).To(HaveLen(2))
		}, 30*time.Second, 5*time.Second).Should(Succeed())

		By("Verifying the Cluster is available")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    mgmtClient,
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		})

		By("PASSED!")
	})

	AfterEach(func() {
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
		if !input.SkipCleanup && input.ExtensionServiceNamespace != "" && input.ExtensionServiceName != "" {
			Eventually(func() error {
				return input.BootstrapClusterProxy.GetClient().Delete(ctx, &runtimev1.ExtensionConfig{ObjectMeta: metav1.ObjectMeta{Name: input.ExtensionConfigName}})
			}, 10*time.Second, 1*time.Second).Should(Succeed(), "Deleting ExtensionConfig failed")
		}
	})
}
