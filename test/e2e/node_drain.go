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
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// NodeDrainTimeoutSpecInput is the input for NodeDrainTimeoutSpec.
type NodeDrainTimeoutSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers (ex: CAPD + in-memory) are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

	// Flavor, if specified, must refer to a template that uses a Cluster with ClusterClass.
	// The cluster must use a KubeadmControlPlane and a MachineDeployment.
	// If not specified, "topology" is used.
	Flavor *string

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)

	// Enables additional verification for volumes blocking machine deletion.
	// Requires to add appropriate resources via CreateAdditionalResources.
	VerifyNodeVolumeDetach bool

	// Allows to overwrite the default function used for unblocking volume detachments.
	UnblockNodeVolumeDetachment func(ctx context.Context, bootstrapClusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster)

	// Allows to create additional resources.
	CreateAdditionalResources func(ctx context.Context, clusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster)
}

// NodeDrainTimeoutSpec goes through the following steps:
// * Create cluster with 3 CP & 1 worker Machine
// * Ensure Node label is set & NodeDrainTimeoutSeconds is set to 0 (wait forever)
// * Deploy MachineDrainRules
// * Deploy Deployment with unevictable Pods on CP & MD Nodes
// * Deploy Deployment with unevictable Pods with `wait-completed` label on CP & MD Nodes
// * Deploy Deployment with evictable Pods with finalizer on CP & MD Nodes
// * Deploy additional resources if defined in input
// * Trigger Node drain by scaling down the control plane to 1 and MachineDeployments to 0
// * Get draining control plane and MachineDeployment Machines
// * Verify drain of Deployments with order -5
// * Verify drain of Deployments with order -1
// * Verify skipped Pods are still there and don't have a deletionTimestamp
// * Verify wait-completed Pods are still there and don't have a deletionTimestamp
// * Verify Node drains for control plane and MachineDeployment Machines are blocked by WaitCompleted Pods
// * Force deleting the WaitCompleted Pods
// * Verify Node drains for control plane and MachineDeployment Machines are blocked by PDBs
// * Delete the unevictable pod PDBs
// * Verify machine deletion is blocked by waiting for volume detachment (only if VerifyNodeVolumeDetach is enabled)
// * Set NodeDrainTimeoutSeconds to 1s to unblock Node drain
// * Unblocks waiting for volume detachment (only if VerifyNodeVolumeDetach is enabled)
// * Verify scale down succeeded because Node drains were unblocked.
func NodeDrainTimeoutSpec(ctx context.Context, inputGetter func() NodeDrainTimeoutSpecInput) {
	var (
		specName         = "node-drain"
		input            NodeDrainTimeoutSpecInput
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

		Expect(input.E2EConfig.GetIntervals(specName, "wait-deployment-available")).ToNot(BeNil())
		Expect(input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")).ToNot(BeNil())

		if input.VerifyNodeVolumeDetach && input.UnblockNodeVolumeDetachment == nil {
			input.UnblockNodeVolumeDetachment = unblockNodeVolumeDetachmentFunc(input.E2EConfig.GetIntervals(specName, "wait-control-plane"), input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"))
		}

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("A node should be drained correctly", func() {
		By("Creating a workload cluster")
		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		controlPlaneReplicas := 3
		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   ptr.Deref(input.Flavor, "topology"),
				Namespace:                namespace.Name,
				ClusterName:              clusterName,
				KubernetesVersion:        input.E2EConfig.MustGetVariable(KubernetesVersion),
				ControlPlaneMachineCount: ptr.To[int64](int64(controlPlaneReplicas)),
				WorkerMachineCount:       ptr.To[int64](1),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)
		cluster := clusterResources.Cluster
		controlplane := clusterResources.ControlPlane
		machineDeployments := clusterResources.MachineDeployments
		Expect(machineDeployments[0].Spec.Replicas).To(Equal(ptr.To[int32](1)))

		// This label will be added to all Machines so we can later create Pods on the right Nodes.
		nodeOwnerLabelKey := "owner.node.cluster.x-k8s.io"

		By("Ensure Node label is set & NodeDrainTimeoutSeconds is set to 0 (wait forever) on ControlPlane and MachineDeployment topologies")
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				topology.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(0))
				if input.VerifyNodeVolumeDetach {
					topology.Deletion.NodeVolumeDetachTimeoutSeconds = ptr.To(int32(0))
				}
				if topology.Metadata.Labels == nil {
					topology.Metadata.Labels = map[string]string{}
				}
				topology.Metadata.Labels[nodeOwnerLabelKey] = "KubeadmControlPlane-" + controlplane.Name
			},
			WaitForControlPlane: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})
		modifyMachineDeploymentViaClusterAndWait(ctx, modifyMachineDeploymentViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyMachineDeploymentTopology: func(topology *clusterv1.MachineDeploymentTopology) {
				topology.Deletion.NodeDrainTimeoutSeconds = ptr.To(int32(0))
				if input.VerifyNodeVolumeDetach {
					topology.Deletion.NodeVolumeDetachTimeoutSeconds = ptr.To(int32(0))
				}
				if topology.Metadata.Labels == nil {
					topology.Metadata.Labels = map[string]string{}
				}
				for _, md := range machineDeployments {
					if md.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel] == topology.Name {
						topology.Metadata.Labels[nodeOwnerLabelKey] = "MachineDeployment-" + md.Name
					}
				}
			},
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		workloadClusterProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name)

		By("Deploy MachineDrainRules")
		machineDrainRules := []*clusterv1.MachineDrainRule{
			generateMachineDrainRule(namespace.Name, clusterName, "drain-order-first", -5),
			generateMachineDrainRule(namespace.Name, clusterName, "drain-order-second", -1),
			generateMachineDrainRule(namespace.Name, clusterName, "drain-order-last", 10),
		}
		for _, rule := range machineDrainRules {
			Expect(input.BootstrapClusterProxy.GetClient().Create(ctx, rule)).To(Succeed())
		}

		By("Deploy Deployment with unevictable Pods on control plane and MachineDeployment Nodes")
		framework.DeployUnevictablePod(ctx, framework.DeployPodAndWaitInput{
			WorkloadClusterProxy: workloadClusterProxy,
			ControlPlane:         controlplane,
			DeploymentName:       cpDeploymentWithPDBName(),
			Namespace:            "unevictable-workload",
			NodeSelector:         map[string]string{nodeOwnerLabelKey: "KubeadmControlPlane-" + controlplane.Name},
			ModifyDeployment: func(deployment *appsv1.Deployment) {
				// Ensure we try to drain unevictable Pods last, otherwise they block drain of evictable Pods.
				deployment.Spec.Template.Labels["mdr"] = "drain-order-last"
			},
			WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
		})
		for _, md := range machineDeployments {
			framework.DeployUnevictablePod(ctx, framework.DeployPodAndWaitInput{
				WorkloadClusterProxy: workloadClusterProxy,
				MachineDeployment:    md,
				DeploymentName:       mdDeploymentWithPDBName(md.Name),
				Namespace:            "unevictable-workload",
				NodeSelector:         map[string]string{nodeOwnerLabelKey: "MachineDeployment-" + md.Name},
				ModifyDeployment: func(deployment *appsv1.Deployment) {
					// Ensure we try to drain unevictable Pods last, otherwise they block drain of evictable Pods.
					deployment.Spec.Template.Labels["mdr"] = "drain-order-last"
				},
				WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
			})
		}
		By("Deploy an unevictable Deployment with `wait-completed` label")
		framework.DeployPodAndWait(ctx, framework.DeployPodAndWaitInput{
			WorkloadClusterProxy: workloadClusterProxy,
			ControlPlane:         controlplane,
			DeploymentName:       cpDeploymentName("wait-completed"),
			Namespace:            "unevictable-workload",
			NodeSelector:         map[string]string{nodeOwnerLabelKey: "KubeadmControlPlane-" + controlplane.Name},
			ModifyDeployment: func(deployment *appsv1.Deployment) {
				deployment.Spec.Template.Labels["cluster.x-k8s.io/drain"] = string(clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted)
			},
			WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
		})
		for _, md := range machineDeployments {
			framework.DeployPodAndWait(ctx, framework.DeployPodAndWaitInput{
				WorkloadClusterProxy: workloadClusterProxy,
				MachineDeployment:    md,
				DeploymentName:       mdDeploymentName("wait-completed", md.Name),
				Namespace:            "unevictable-workload",
				NodeSelector:         map[string]string{nodeOwnerLabelKey: "MachineDeployment-" + md.Name},
				ModifyDeployment: func(deployment *appsv1.Deployment) {
					deployment.Spec.Template.Labels["cluster.x-k8s.io/drain"] = string(clusterv1.MachineDrainRuleDrainBehaviorWaitCompleted)
				},
				WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
			})
		}

		By("Deploy Deployments with evictable Pods with finalizer on control plane and MachineDeployment Nodes")
		evictablePodDeployments := map[string]map[string]string{
			"drain-order-first":  {"mdr": "drain-order-first"},
			"drain-order-second": {"mdr": "drain-order-second"},
			"skip":               {"cluster.x-k8s.io/drain": string(clusterv1.MachineDrainRuleDrainBehaviorSkip)},
		}
		for deploymentNamePrefix, deploymentLabels := range evictablePodDeployments {
			framework.DeployEvictablePod(ctx, framework.DeployEvictablePodInput{
				WorkloadClusterProxy: workloadClusterProxy,
				ControlPlane:         controlplane,
				DeploymentName:       cpDeploymentName(deploymentNamePrefix),
				Namespace:            "evictable-workload",
				NodeSelector:         map[string]string{nodeOwnerLabelKey: "KubeadmControlPlane-" + controlplane.Name},
				ModifyDeployment: func(deployment *appsv1.Deployment) {
					deployment.Spec.Template.Finalizers = []string{"test.cluster.x-k8s.io/block"}
					for k, v := range deploymentLabels {
						deployment.Spec.Template.Labels[k] = v
					}
				},
				WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
			})
			for _, md := range machineDeployments {
				framework.DeployEvictablePod(ctx, framework.DeployEvictablePodInput{
					WorkloadClusterProxy: workloadClusterProxy,
					MachineDeployment:    md,
					DeploymentName:       mdDeploymentName(deploymentNamePrefix, md.Name),
					Namespace:            "evictable-workload",
					NodeSelector:         map[string]string{nodeOwnerLabelKey: "MachineDeployment-" + md.Name},
					ModifyDeployment: func(deployment *appsv1.Deployment) {
						deployment.Spec.Template.Finalizers = []string{"test.cluster.x-k8s.io/block"}
						for k, v := range deploymentLabels {
							deployment.Spec.Template.Labels[k] = v
						}
					},
					WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
				})
			}
		}

		if input.CreateAdditionalResources != nil {
			input.CreateAdditionalResources(ctx, input.BootstrapClusterProxy, cluster)
		}

		By("Trigger Node drain by scaling down the control plane to 1 and MachineDeployments to 0")
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				topology.Replicas = ptr.To[int32](1)
			},
			WaitForControlPlane: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})
		modifyMachineDeploymentViaClusterAndWait(ctx, modifyMachineDeploymentViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyMachineDeploymentTopology: func(topology *clusterv1.MachineDeploymentTopology) {
				topology.Replicas = ptr.To[int32](0)
			},
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Get draining control plane and MachineDeployment Machines")
		var drainingCPMachineKey client.ObjectKey
		var drainingCPNodeName string
		drainingMDMachineKeys := map[string]client.ObjectKey{}
		drainingMDNodeNames := map[string]string{}
		Eventually(func(g Gomega) {
			controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx, framework.GetControlPlaneMachinesByClusterInput{
				Lister:      input.BootstrapClusterProxy.GetClient(),
				ClusterName: cluster.Name,
				Namespace:   cluster.Namespace,
			})
			var condition *metav1.Condition
			for _, machine := range controlPlaneMachines {
				condition = conditions.Get(&machine, clusterv1.MachineDeletingCondition)
				if condition != nil && condition.Status == metav1.ConditionTrue && condition.Reason == clusterv1.MachineDeletingDrainingNodeReason {
					// We only expect to find the condition on one Machine (as KCP will only try to drain one Machine at a time)
					drainingCPMachineKey = client.ObjectKeyFromObject(&machine)
					drainingCPNodeName = machine.Status.NodeRef.Name
					return
				}
			}
			g.Expect(drainingCPNodeName).ToNot(BeEmpty())
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		for _, md := range machineDeployments {
			Eventually(func(g Gomega) {
				machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
					Lister:            input.BootstrapClusterProxy.GetClient(),
					ClusterName:       cluster.Name,
					Namespace:         cluster.Namespace,
					MachineDeployment: *md,
				})
				g.Expect(machines).To(HaveLen(1))
				drainingMDMachineKeys[md.Name] = client.ObjectKeyFromObject(&machines[0])
				drainingMDNodeNames[md.Name] = machines[0].Status.NodeRef.Name
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		}

		By("Verify drain of Deployments with order -5")
		verifyNodeDrainsBlockedAndUnblock(ctx, verifyNodeDrainsBlockedAndUnblockInput{
			BootstrapClusterProxy: input.BootstrapClusterProxy,
			WorkloadClusterProxy:  workloadClusterProxy,
			Cluster:               cluster,
			MachineDeployments:    machineDeployments,
			DrainedCPMachineKey:   drainingCPMachineKey,
			DrainedMDMachineKeys:  drainingMDMachineKeys,
			DeploymentNamePrefix:  "drain-order-first",
			CPConditionMessageSubstrings: []string{
				// The evictable Pod with order -5 was evicted. It still blocks the drain because of the finalizer, otherwise the Pod would be gone already.
				fmt.Sprintf(`(?m)\* Pod evictable-workload\/%s[^:]+: deletionTimestamp set, but still not removed from the Node`, cpDeploymentName("drain-order-first")),
				// After the Pod with order -5 is gone, the drain continues with the Pod with order -1.
				fmt.Sprintf(`(?m)After above Pods have been removed from the Node, the following Pods will be evicted:.*evictable-workload\/%s.*`, cpDeploymentName("drain-order-second")),
			},
			MDConditionMessageSubstrings: func() map[string][]string {
				messageSubStrings := map[string][]string{}
				for _, md := range machineDeployments {
					messageSubStrings[md.Name] = []string{
						// The evictable Pod with order -5 was evicted. It still blocks the drain because of the finalizer, otherwise the Pod would be gone already.
						fmt.Sprintf(`(?m)\* Pod evictable-workload\/%s[^:]+: deletionTimestamp set, but still not removed from the Node`, mdDeploymentName("drain-order-first", md.Name)),
						// After the Pod with order -5 is gone, the drain continues with the Pod with order -1.
						fmt.Sprintf(`(?m)After above Pods have been removed from the Node, the following Pods will be evicted:.*evictable-workload\/%s.*`, mdDeploymentName("drain-order-second", md.Name)),
					}
				}
				return messageSubStrings
			}(),
			WaitForMachineDelete: input.E2EConfig.GetIntervals(specName, "wait-machine-deleted"),
		})

		By("Verify drain of Deployments with order -1")
		verifyNodeDrainsBlockedAndUnblock(ctx, verifyNodeDrainsBlockedAndUnblockInput{
			BootstrapClusterProxy: input.BootstrapClusterProxy,
			WorkloadClusterProxy:  workloadClusterProxy,
			Cluster:               cluster,
			MachineDeployments:    machineDeployments,
			DrainedCPMachineKey:   drainingCPMachineKey,
			DrainedMDMachineKeys:  drainingMDMachineKeys,
			DeploymentNamePrefix:  "drain-order-second",
			CPConditionMessageSubstrings: []string{
				// The evictable Pod with order -1 was evicted. It still blocks the drain because of the finalizer, otherwise the Pod would be gone already.
				fmt.Sprintf(`(?m)\* Pod evictable-workload\/%s[^:]+: deletionTimestamp set, but still not removed from the Node`, cpDeploymentName("drain-order-second")),
				// After the Pod with order -1 is gone, the drain continues with the unevictable Pod.
				fmt.Sprintf(`(?m)After above Pods have been removed from the Node, the following Pods will be evicted:.*unevictable-workload\/%s.*`, cpDeploymentWithPDBName()),
			},
			MDConditionMessageSubstrings: func() map[string][]string {
				messageSubStrings := map[string][]string{}
				for _, md := range machineDeployments {
					messageSubStrings[md.Name] = []string{
						// The evictable Pod with order -1 was evicted. It still blocks the drain because of the finalizer, otherwise the Pod would be gone already.
						fmt.Sprintf(`(?m)\* Pod evictable-workload\/%s[^:]+: deletionTimestamp set, but still not removed from the Node`, mdDeploymentName("drain-order-second", md.Name)),
						// After the Pod with order -1 is gone, the drain continues with the unevictable Pod.
						fmt.Sprintf(`(?m)After above Pods have been removed from the Node, the following Pods will be evicted:.*unevictable-workload\/%s.*`, mdDeploymentWithPDBName(md.Name)),
					}
				}
				return messageSubStrings
			}(),
			WaitForMachineDelete: input.E2EConfig.GetIntervals(specName, "wait-machine-deleted"),
		})

		By("Verify skipped Pods are still there and don't have a deletionTimestamp")
		skippedCPPods := &corev1.PodList{}
		Expect(workloadClusterProxy.GetClient().List(ctx, skippedCPPods,
			client.InNamespace("evictable-workload"),
			client.MatchingLabels{"deployment": cpDeploymentName("skip")},
			client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", drainingCPNodeName)},
		)).To(Succeed())
		Expect(skippedCPPods.Items).To(HaveLen(1))
		Expect(skippedCPPods.Items[0].DeletionTimestamp.IsZero()).To(BeTrue())
		for _, md := range machineDeployments {
			skippedMDPods := &corev1.PodList{}
			Expect(workloadClusterProxy.GetClient().List(ctx, skippedMDPods,
				client.InNamespace("evictable-workload"),
				client.MatchingLabels{"deployment": mdDeploymentName("skip", md.Name)},
				client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", drainingMDNodeNames[md.Name])},
			)).To(Succeed())
			Expect(skippedMDPods.Items).To(HaveLen(1))
			Expect(skippedMDPods.Items[0].DeletionTimestamp.IsZero()).To(BeTrue())
		}

		By("Verify wait-completed Pods are still there and don't have a deletionTimestamp")
		waitCPPods := &corev1.PodList{}
		Expect(workloadClusterProxy.GetClient().List(ctx, waitCPPods,
			client.InNamespace("unevictable-workload"),
			client.MatchingLabels{"deployment": cpDeploymentName("wait-completed")},
			client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", drainingCPNodeName)},
		)).To(Succeed())
		Expect(skippedCPPods.Items).To(HaveLen(1))
		Expect(skippedCPPods.Items[0].DeletionTimestamp.IsZero()).To(BeTrue())
		for _, md := range machineDeployments {
			skippedMDPods := &corev1.PodList{}
			Expect(workloadClusterProxy.GetClient().List(ctx, skippedMDPods,
				client.InNamespace("unevictable-workload"),
				client.MatchingLabels{"deployment": mdDeploymentName("wait-completed", md.Name)},
				client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", drainingMDNodeNames[md.Name])},
			)).To(Succeed())
			Expect(skippedMDPods.Items).To(HaveLen(1))
			Expect(skippedMDPods.Items[0].DeletionTimestamp.IsZero()).To(BeTrue())
		}

		By("Verify Node drains for control plane and MachineDeployment Machines are blocked by WaitCompleted Pods")
		Eventually(func(g Gomega) {
			drainedCPMachine := &clusterv1.Machine{}
			g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, drainingCPMachineKey, drainedCPMachine)).To(Succeed())

			condition := conditions.Get(drainedCPMachine, clusterv1.MachineDeletingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingDrainingNodeReason))
			// The evictable Pod should be gone now.
			g.Expect(condition.Message).ToNot(ContainSubstring("deletionTimestamp set, but still not removed from the Node"))
			// The unevictable Pod should still not be evicted because of the wait-completed label.
			g.Expect(condition.Message).To(MatchRegexp(fmt.Sprintf(".*Pod unevictable-workload/%s.*: waiting for completion", cpDeploymentName("wait-completed"))))
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		for _, md := range machineDeployments {
			Eventually(func(g Gomega) {
				drainedMDMachine := &clusterv1.Machine{}
				g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, drainingMDMachineKeys[md.Name], drainedMDMachine)).To(Succeed())

				condition := conditions.Get(drainedMDMachine, clusterv1.MachineDeletingCondition)
				g.Expect(condition).ToNot(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingDrainingNodeReason))
				// The evictable Pod should be gone now.
				g.Expect(condition.Message).ToNot(ContainSubstring("deletionTimestamp set, but still not removed from the Node"))
				// The unevictable Pod should still not be evicted because of the wait-completed label.
				g.Expect(condition.Message).To(MatchRegexp(fmt.Sprintf(".*Pod unevictable-workload/%s.*: waiting for completion", mdDeploymentName("wait-completed", md.Name))))
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		}

		By("Force deleting the WaitCompleted Pods")
		forceDeleteOpts := &client.DeleteOptions{GracePeriodSeconds: ptr.To[int64](0)}
		waitDeploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "unevictable-workload", Name: cpDeploymentName("wait-completed")}}
		Expect(workloadClusterProxy.GetClient().Delete(ctx, waitDeploy, forceDeleteOpts)).To(Succeed())
		for _, md := range machineDeployments {
			waitDeploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "unevictable-workload", Name: mdDeploymentName("wait-completed", md.Name)}}
			Expect(workloadClusterProxy.GetClient().Delete(ctx, waitDeploy, forceDeleteOpts)).To(Succeed())
		}

		By("Verify Node drains for control plane and MachineDeployment Machines are blocked by PDBs")
		Eventually(func(g Gomega) {
			drainedCPMachine := &clusterv1.Machine{}
			g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, drainingCPMachineKey, drainedCPMachine)).To(Succeed())

			condition := conditions.Get(drainedCPMachine, clusterv1.MachineDeletingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingDrainingNodeReason))
			// The evictable Pod should be gone now.
			g.Expect(condition.Message).ToNot(ContainSubstring("deletionTimestamp set, but still not removed from the Node"))
			// The unevictable Pod should still not be evicted because of the PDB.
			g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("cannot evict pod as it would violate the pod's disruption budget. The disruption budget %s needs", cpDeploymentWithPDBName())))
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		for _, md := range machineDeployments {
			Eventually(func(g Gomega) {
				drainedMDMachine := &clusterv1.Machine{}
				g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, drainingMDMachineKeys[md.Name], drainedMDMachine)).To(Succeed())

				condition := conditions.Get(drainedMDMachine, clusterv1.MachineDeletingCondition)
				g.Expect(condition).ToNot(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingDrainingNodeReason))
				// The evictable Pod should be gone now.
				g.Expect(condition.Message).ToNot(ContainSubstring("deletionTimestamp set, but still not removed from the Node"))
				// The unevictable Pod should still not be evicted because of the PDB.
				g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("cannot evict pod as it would violate the pod's disruption budget. The disruption budget %s needs", mdDeploymentWithPDBName(md.Name))))
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		}

		By("Delete PDB for all unevictable pods to let drain succeed")
		framework.DeletePodDisruptionBudget(ctx, framework.DeletePodDisruptionBudgetInput{
			ClientSet: workloadClusterProxy.GetClientSet(),
			Budget:    cpDeploymentWithPDBName(),
			Namespace: "unevictable-workload",
		})
		for _, md := range machineDeployments {
			framework.DeletePodDisruptionBudget(ctx, framework.DeletePodDisruptionBudgetInput{
				ClientSet: workloadClusterProxy.GetClientSet(),
				Budget:    mdDeploymentWithPDBName(md.Name),
				Namespace: "unevictable-workload",
			})
		}

		if input.VerifyNodeVolumeDetach {
			By("Verify Node removal for control plane and MachineDeployment Machines are blocked (only by volume detachments)")
			Eventually(func(g Gomega) {
				waitingCPMachine := &clusterv1.Machine{}
				g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, drainingCPMachineKey, waitingCPMachine)).To(Succeed())

				condition := conditions.Get(waitingCPMachine, clusterv1.MachineDeletingCondition)
				g.Expect(condition).ToNot(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingWaitingForVolumeDetachReason))
				// Deletion still not be blocked because of the volume.
				g.Expect(condition.Message).To(ContainSubstring("Waiting for Node volumes to be detached"))
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
			for _, machineKey := range drainingMDMachineKeys {
				Eventually(func(g Gomega) {
					drainedMDMachine := &clusterv1.Machine{}
					g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, machineKey, drainedMDMachine)).To(Succeed())

					condition := conditions.Get(drainedMDMachine, clusterv1.MachineDeletingCondition)
					g.Expect(condition).ToNot(BeNil())
					g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
					g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingWaitingForVolumeDetachReason)) // Deletion still not be blocked because of the volume.
					g.Expect(condition.Message).To(ContainSubstring("Waiting for Node volumes to be detached"))
				}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
			}

			By("Executing input.UnblockNodeVolumeDetachment to unblock waiting for volume detachments")
			input.UnblockNodeVolumeDetachment(ctx, input.BootstrapClusterProxy, cluster)
		}

		// Set NodeDrainTimeoutSeconds and NodeVolumeDetachTimeoutSeconds to let the second ControlPlane Node get deleted without requiring manual intervention.
		By("Set NodeDrainTimeoutSeconds and NodeVolumeDetachTimeoutSeconds for ControlPlanes to 1s to unblock Node drain")
		// Note: This also verifies that KCP & MachineDeployments are still propagating changes to NodeDrainTimeoutSeconds down to
		// Machines that already have a deletionTimestamp.
		drainTimeout := ptr.To(int32(1))
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				topology.Deletion.NodeDrainTimeoutSeconds = drainTimeout
				topology.Deletion.NodeVolumeDetachTimeoutSeconds = drainTimeout
			},
			WaitForControlPlane: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})

		By("Verify scale down succeeded because Node drains and Volume detachments were unblocked")
		// When we scale down the KCP, controlplane machines are deleted one by one, so it requires more time
		// MD Machine deletion is done in parallel and will be faster.
		nodeDrainTimeoutKCPInterval := getDrainAndDeleteInterval(input.E2EConfig.GetIntervals(specName, "wait-machine-deleted"), drainTimeout, controlPlaneReplicas)
		Eventually(func(g Gomega) {
			// When all drains complete we only have 1 control plane & 0 MD replicas left.
			controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx, framework.GetControlPlaneMachinesByClusterInput{
				Lister:      input.BootstrapClusterProxy.GetClient(),
				ClusterName: cluster.Name,
				Namespace:   cluster.Namespace,
			})
			g.Expect(controlPlaneMachines).To(HaveLen(1))

			for _, md := range machineDeployments {
				machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
					Lister:            input.BootstrapClusterProxy.GetClient(),
					ClusterName:       cluster.Name,
					Namespace:         cluster.Namespace,
					MachineDeployment: *md,
				})
				g.Expect(machines).To(BeEmpty())
			}
		}, nodeDrainTimeoutKCPInterval...).Should(Succeed())

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

func generateMachineDrainRule(clusterNamespace, clusterName, mdrName string, order int32) *clusterv1.MachineDrainRule {
	return &clusterv1.MachineDrainRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mdrName,
			Namespace: clusterNamespace,
		},
		Spec: clusterv1.MachineDrainRuleSpec{
			Drain: clusterv1.MachineDrainRuleDrainConfig{
				Behavior: clusterv1.MachineDrainRuleDrainBehaviorDrain,
				Order:    ptr.To[int32](order),
			},
			Machines: []clusterv1.MachineDrainRuleMachineSelector{
				// Select all Machines with the ClusterNameLabel belonging to Clusters with the ClusterNameLabel.
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							clusterv1.ClusterNameLabel: clusterName,
						},
					},
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							clusterv1.ClusterNameLabel: clusterName,
						},
					},
				},
			},
			Pods: []clusterv1.MachineDrainRulePodSelector{
				// Select all Pods with label "mdr": mdrName in all Namespaces except "kube-system".
				{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"mdr": mdrName,
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "kubernetes.io/metadata.name",
								Operator: metav1.LabelSelectorOpNotIn,
								Values: []string{
									metav1.NamespaceSystem,
								},
							},
						},
					},
				},
			},
		},
	}
}

func cpDeploymentName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, "cp")
}

func cpDeploymentWithPDBName() string {
	return "unevictable-cp"
}

func mdDeploymentName(prefix, name string) string {
	return fmt.Sprintf("%s-%s", prefix, name)
}

func mdDeploymentWithPDBName(name string) string {
	return fmt.Sprintf("unevictable-%s", name)
}

type verifyNodeDrainsBlockedAndUnblockInput struct {
	BootstrapClusterProxy        framework.ClusterProxy
	WorkloadClusterProxy         framework.ClusterProxy
	Cluster                      *clusterv1.Cluster
	MachineDeployments           []*clusterv1.MachineDeployment
	DrainedCPMachineKey          client.ObjectKey
	DrainedMDMachineKeys         map[string]client.ObjectKey
	DeploymentNamePrefix         string
	CPConditionMessageSubstrings []string
	MDConditionMessageSubstrings map[string][]string
	WaitForMachineDelete         []interface{}
}

func verifyNodeDrainsBlockedAndUnblock(ctx context.Context, input verifyNodeDrainsBlockedAndUnblockInput) {
	By(fmt.Sprintf("Verify Node drains for control plane and MachineDeployment Machines are blocked (%s)", input.DeploymentNamePrefix))
	var evictedCPPod *corev1.Pod
	Eventually(func(g Gomega) {
		drainedCPMachine := &clusterv1.Machine{}
		g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, input.DrainedCPMachineKey, drainedCPMachine)).To(Succeed())

		// Verify condition on drained CP Machine.
		condition := conditions.Get(drainedCPMachine, clusterv1.MachineDeletingCondition)
		g.Expect(condition).ToNot(BeNil())
		g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingDrainingNodeReason))
		for _, messageSubstring := range input.CPConditionMessageSubstrings {
			var re = regexp.MustCompile(messageSubstring)
			match := re.MatchString(condition.Message)
			g.Expect(match).To(BeTrue(), fmt.Sprintf("message '%s' does not match regexp %s", condition.Message, messageSubstring))
		}

		// Verify evictable Pod was evicted and terminated (i.e. phase is succeeded)
		evictedPods := &corev1.PodList{}
		g.Expect(input.WorkloadClusterProxy.GetClient().List(ctx, evictedPods,
			client.InNamespace("evictable-workload"),
			client.MatchingLabels{"deployment": cpDeploymentName(input.DeploymentNamePrefix)},
			client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", drainedCPMachine.Status.NodeRef.Name)},
		)).To(Succeed())
		g.Expect(evictedPods.Items).To(HaveLen(1))
		evictedCPPod = &evictedPods.Items[0]
		verifyPodEvictedAndSucceeded(g, evictedCPPod)
	}, input.WaitForMachineDelete...).Should(Succeed())

	evictedMDPods := map[string]*corev1.Pod{}
	for _, md := range input.MachineDeployments {
		Eventually(func(g Gomega) {
			drainedMDMachine := &clusterv1.Machine{}
			g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, input.DrainedMDMachineKeys[md.Name], drainedMDMachine)).To(Succeed())

			// Verify condition on drained MD Machine.
			condition := conditions.Get(drainedMDMachine, clusterv1.MachineDeletingCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(condition.Reason).To(Equal(clusterv1.MachineDeletingDrainingNodeReason))
			for _, messageSubstring := range input.MDConditionMessageSubstrings[md.Name] {
				var re = regexp.MustCompile(messageSubstring)
				match := re.MatchString(condition.Message)
				g.Expect(match).To(BeTrue(), fmt.Sprintf("message '%s' does not match regexp %s", condition.Message, messageSubstring))
			}

			// Verify evictable Pod was evicted and terminated (i.e. phase is succeeded)
			evictedPods := &corev1.PodList{}
			g.Expect(input.WorkloadClusterProxy.GetClient().List(ctx, evictedPods,
				client.InNamespace("evictable-workload"),
				client.MatchingLabels{"deployment": mdDeploymentName(input.DeploymentNamePrefix, md.Name)},
				client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", drainedMDMachine.Status.NodeRef.Name)},
			)).To(Succeed())
			g.Expect(evictedPods.Items).To(HaveLen(1))
			evictedMDPods[md.Name] = &evictedPods.Items[0]
			verifyPodEvictedAndSucceeded(g, evictedMDPods[md.Name])
		}, input.WaitForMachineDelete...).Should(Succeed())
	}

	By(fmt.Sprintf("Unblock deletion of evicted Pods by removing the finalizer (%s)", input.DeploymentNamePrefix))
	Eventually(func(g Gomega) {
		g.Expect(input.WorkloadClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(evictedCPPod), evictedCPPod)).To(Succeed())
		originalPod := evictedCPPod.DeepCopy()
		evictedCPPod.Finalizers = []string{}
		g.Expect(input.WorkloadClusterProxy.GetClient().Patch(ctx, evictedCPPod, client.MergeFrom(originalPod))).To(Succeed())
	}, input.WaitForMachineDelete...).Should(Succeed())
	for _, md := range input.MachineDeployments {
		Eventually(func(g Gomega) {
			evictedMDPod := evictedMDPods[md.Name]
			g.Expect(input.WorkloadClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(evictedMDPod), evictedMDPod)).To(Succeed())
			originalPod := evictedMDPod.DeepCopy()
			evictedMDPod.Finalizers = []string{}
			g.Expect(input.WorkloadClusterProxy.GetClient().Patch(ctx, evictedMDPod, client.MergeFrom(originalPod))).To(Succeed())
		}, input.WaitForMachineDelete...).Should(Succeed())
	}
}

func verifyPodEvictedAndSucceeded(g Gomega, pod *corev1.Pod) {
	g.Expect(pod.Status.Phase).To(Equal(corev1.PodSucceeded))
	podEvicted := false
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.DisruptionTarget && c.Reason == "EvictionByEvictionAPI" && c.Status == corev1.ConditionTrue {
			podEvicted = true
			break
		}
	}
	g.Expect(podEvicted).To(BeTrue(), "Expected Pod to be evicted")
}

func getDrainAndDeleteInterval(deleteInterval []interface{}, drainTimeout *int32, replicas int) []interface{} {
	deleteTimeout, err := time.ParseDuration(deleteInterval[0].(string))
	Expect(err).ToNot(HaveOccurred())
	// We add the drain timeout to the specified delete timeout per replica.
	intervalDuration := (time.Duration(*drainTimeout)*time.Second + deleteTimeout) * time.Duration(replicas)
	res := []interface{}{intervalDuration.String(), deleteInterval[1]}
	return res
}

func unblockNodeVolumeDetachmentFunc(waitControlPlaneIntervals, waitWorkerNodeIntervals []interface{}) func(ctx context.Context, bootstrapClusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster) {
	return func(ctx context.Context, bootstrapClusterProxy framework.ClusterProxy, cluster *clusterv1.Cluster) {
		By("Set NodeVolumeDetachTimeoutSeconds to 1s to unblock waiting for volume detachments")
		// Note: This also verifies that KCP & MachineDeployments are still propagating changes to NodeVolumeDetachTimeoutSeconds down to
		// Machines that already have a deletionTimestamp.
		nodeVolumeDetachTimeout := ptr.To(int32(1))
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: bootstrapClusterProxy,
			Cluster:      cluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				topology.Deletion.NodeVolumeDetachTimeoutSeconds = nodeVolumeDetachTimeout
			},
			WaitForControlPlane: waitControlPlaneIntervals,
		})
		modifyMachineDeploymentViaClusterAndWait(ctx, modifyMachineDeploymentViaClusterAndWaitInput{
			ClusterProxy: bootstrapClusterProxy,
			Cluster:      cluster,
			ModifyMachineDeploymentTopology: func(topology *clusterv1.MachineDeploymentTopology) {
				topology.Deletion.NodeVolumeDetachTimeoutSeconds = nodeVolumeDetachTimeout
			},
			WaitForMachineDeployments: waitWorkerNodeIntervals,
		})
	}
}
