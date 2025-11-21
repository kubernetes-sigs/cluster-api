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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

// MachineDeploymentRolloutSpecInput is the input for MachineDeploymentRolloutSpec.
type MachineDeploymentRolloutSpecInput struct {
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

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
}

// MachineDeploymentRolloutSpec implements a test that verifies that MachineDeployment rolling updates are successful.
func MachineDeploymentRolloutSpec(ctx context.Context, inputGetter func() MachineDeploymentRolloutSpecInput) {
	var (
		specName         = "md-rollout"
		input            MachineDeploymentRolloutSpecInput
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
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveValidVersion(input.E2EConfig.MustGetVariable(KubernetesVersion)))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should successfully upgrade Machines upon changes in relevant MachineDeployment fields", func() {
		By("Creating a workload cluster")
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
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersion),
				ControlPlaneMachineCount: ptr.To[int64](1),
				WorkerMachineCount:       ptr.To[int64](1),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		// Get all machines before doing in-place changes so we can check at the end that no machines were replaced.
		machinesBeforeInPlaceChanges := getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)

		By("Upgrade MachineDeployment in-place mutable fields and wait for in-place propagation")
		framework.UpgradeMachineDeploymentInPlaceMutableFieldsAndWait(ctx, framework.UpgradeMachineDeploymentInPlaceMutableFieldsAndWaitInput{
			ClusterProxy:                input.BootstrapClusterProxy,
			Cluster:                     clusterResources.Cluster,
			WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			MachineDeployments:          clusterResources.MachineDeployments,
		})

		preExistingAlwaysTaint := clusterv1.MachineTaint{
			Key:         "pre-existing-always-taint",
			Value:       "always-value",
			Effect:      corev1.TaintEffectPreferNoSchedule,
			Propagation: clusterv1.MachineTaintPropagationAlways,
		}

		preExistingOnInitializationTaint := clusterv1.MachineTaint{
			Key:         "pre-existing-on-initialization-taint",
			Value:       "on-initialization-value",
			Effect:      corev1.TaintEffectPreferNoSchedule,
			Propagation: clusterv1.MachineTaintPropagationOnInitialization,
		}
		addingAlwaysTaint := clusterv1.MachineTaint{
			Key:         "added-always-taint",
			Value:       "added-always-value",
			Effect:      corev1.TaintEffectPreferNoSchedule,
			Propagation: clusterv1.MachineTaintPropagationAlways,
		}

		addingOnInitializationTaint := clusterv1.MachineTaint{
			Key:         "added-on-initialization-taint",
			Value:       "added-on-initialization-value",
			Effect:      corev1.TaintEffectPreferNoSchedule,
			Propagation: clusterv1.MachineTaintPropagationOnInitialization,
		}

		wantMachineTaints := []clusterv1.MachineTaint{
			preExistingAlwaysTaint,
			preExistingOnInitializationTaint,
		}
		wantNodeTaints := toCoreV1Taints(
			preExistingAlwaysTaint,
			preExistingOnInitializationTaint,
		)

		Byf("Verify MachineDeployment Machines and Nodes have the correct taints")
		wlClient := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, clusterResources.Cluster.Namespace, clusterResources.Cluster.Name).GetClient()
		verifyMachineAndNodeTaints(ctx, verifyMachineAndNodeTaintsInput{
			BootstrapClusterClient: input.BootstrapClusterProxy.GetClient(),
			WorkloadClusterClient:  wlClient,
			ClusterName:            clusterResources.Cluster.Name,
			MachineDeployments:     clusterResources.MachineDeployments,
			MachineTaints:          wantMachineTaints,
			NodeTaints:             wantNodeTaints,
		})

		Byf("Verify in-place propagation by adding new taints to the MachineDeployment")
		wantMachineTaints = []clusterv1.MachineTaint{
			preExistingAlwaysTaint,
			preExistingOnInitializationTaint,
			addingAlwaysTaint,
			addingOnInitializationTaint,
		}
		wantNodeTaints = toCoreV1Taints(
			preExistingAlwaysTaint,
			preExistingOnInitializationTaint,
			addingAlwaysTaint,
		)
		for _, md := range clusterResources.MachineDeployments {
			patchHelper, err := patch.NewHelper(md, input.BootstrapClusterProxy.GetClient())
			Expect(err).ToNot(HaveOccurred())
			md.Spec.Template.Spec.Taints = wantMachineTaints
			Expect(patchHelper.Patch(ctx, md)).To(Succeed())
		}

		verifyMachineAndNodeTaints(ctx, verifyMachineAndNodeTaintsInput{
			BootstrapClusterClient: input.BootstrapClusterProxy.GetClient(),
			WorkloadClusterClient:  wlClient,
			ClusterName:            clusterResources.Cluster.Name,
			MachineDeployments:     clusterResources.MachineDeployments,
			MachineTaints:          wantMachineTaints,
			NodeTaints:             wantNodeTaints,
		})

		Byf("Verify in-place propagation when removing preExisting Always and OnInitialization taint from the nodes")
		nodes := corev1.NodeList{}
		Expect(wlClient.List(ctx, &nodes)).To(Succeed())
		// Remove the initial taints from the nodes.
		for _, node := range nodes.Items {
			patchHelper, err := patch.NewHelper(&node, wlClient)
			Expect(err).ToNot(HaveOccurred())
			newTaints := []corev1.Taint{}
			for _, taint := range node.Spec.Taints {
				if taint.Key == preExistingAlwaysTaint.Key {
					continue
				}
				if taint.Key == preExistingOnInitializationTaint.Key {
					continue
				}
				newTaints = append(newTaints, taint)
			}
			node.Spec.Taints = newTaints
			Expect(patchHelper.Patch(ctx, &node)).To(Succeed())
		}

		wantNodeTaints = toCoreV1Taints(
			preExistingAlwaysTaint,
			addingAlwaysTaint,
		)

		verifyMachineAndNodeTaints(ctx, verifyMachineAndNodeTaintsInput{
			BootstrapClusterClient: input.BootstrapClusterProxy.GetClient(),
			WorkloadClusterClient:  wlClient,
			ClusterName:            clusterResources.Cluster.Name,
			MachineDeployments:     clusterResources.MachineDeployments,
			MachineTaints:          wantMachineTaints,
			NodeTaints:             wantNodeTaints,
		})

		Byf("Verify in-place propagation by removing taints from the MachineDeployment")
		wantMachineTaints = []clusterv1.MachineTaint{
			preExistingOnInitializationTaint,
			addingOnInitializationTaint,
		}
		wantNodeTaints = toCoreV1Taints()
		for _, md := range clusterResources.MachineDeployments {
			patchHelper, err := patch.NewHelper(md, input.BootstrapClusterProxy.GetClient())
			Expect(err).ToNot(HaveOccurred())
			md.Spec.Template.Spec.Taints = wantMachineTaints
			Expect(patchHelper.Patch(ctx, md)).To(Succeed())
		}

		verifyMachineAndNodeTaints(ctx, verifyMachineAndNodeTaintsInput{
			BootstrapClusterClient: input.BootstrapClusterProxy.GetClient(),
			WorkloadClusterClient:  wlClient,
			ClusterName:            clusterResources.Cluster.Name,
			MachineDeployments:     clusterResources.MachineDeployments,
			MachineTaints:          wantMachineTaints,
			NodeTaints:             wantNodeTaints,
		})

		By("Verifying there are no unexpected rollouts through in-place changes")
		Consistently(func(g Gomega) {
			machinesAfterInPlaceChanges := getMachinesByCluster(ctx, input.BootstrapClusterProxy.GetClient(), clusterResources.Cluster)
			g.Expect(machinesAfterInPlaceChanges).To(BeComparableTo(machinesBeforeInPlaceChanges), "Machines must not be replaced through in-place rollout")
		}, 30*time.Second, 1*time.Second).Should(Succeed())

		By("Upgrading MachineDeployment Infrastructure ref and wait for rolling upgrade")
		framework.UpgradeMachineDeploymentInfrastructureRefAndWait(ctx, framework.UpgradeMachineDeploymentInfrastructureRefAndWaitInput{
			ClusterProxy:                input.BootstrapClusterProxy,
			Cluster:                     clusterResources.Cluster,
			WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			MachineDeployments:          clusterResources.MachineDeployments,
		})

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

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

type verifyMachineAndNodeTaintsInput struct {
	BootstrapClusterClient client.Client
	WorkloadClusterClient  client.Client
	ClusterName            string
	MachineDeployments     []*clusterv1.MachineDeployment
	MachineTaints          []clusterv1.MachineTaint
	NodeTaints             []corev1.Taint
}

func verifyMachineAndNodeTaints(ctx context.Context, input verifyMachineAndNodeTaintsInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for verifyMachineAndNodeTaints")
	Expect(input.BootstrapClusterClient).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterClient can't be nil when calling verifyMachineAndNodeTaints")
	Expect(input.WorkloadClusterClient).ToNot(BeNil(), "Invalid argument. input.WorkloadClusterClient can't be nil when calling verifyMachineAndNodeTaints")
	Expect(input.ClusterName).NotTo(BeEmpty(), "Invalid argument. input.ClusterName can't be empty when calling verifyMachineAndNodeTaints")
	Expect(input.MachineDeployments).NotTo(BeNil(), "Invalid argument. input.MachineDeployments can't be nil when calling verifyMachineAndNodeTaints")

	Eventually(func(g Gomega) {
		for _, md := range input.MachineDeployments {
			machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
				Lister:            input.BootstrapClusterClient,
				ClusterName:       input.ClusterName,
				Namespace:         md.Namespace,
				MachineDeployment: *md,
			})
			g.Expect(machines).To(HaveLen(int(ptr.Deref(md.Spec.Replicas, 0))))
			for _, machine := range machines {
				g.Expect(machine.Spec.Taints).To(ConsistOf(input.MachineTaints))
				g.Expect(machine.Status.NodeRef.IsDefined()).To(BeTrue())

				node := &corev1.Node{}
				g.Expect(input.WorkloadClusterClient.Get(ctx, client.ObjectKey{Name: machine.Status.NodeRef.Name}, node)).To(Succeed())
				g.Expect(node.Spec.Taints).To(ConsistOf(input.NodeTaints))
			}
		}
	}, "1m").Should(Succeed())
}

func toCoreV1Taints(machineTaints ...clusterv1.MachineTaint) []corev1.Taint {
	taints := []corev1.Taint{}
	for _, machineTaint := range machineTaints {
		taints = append(taints, corev1.Taint{
			Key:    machineTaint.Key,
			Value:  machineTaint.Value,
			Effect: machineTaint.Effect,
		})
	}
	return taints
}
