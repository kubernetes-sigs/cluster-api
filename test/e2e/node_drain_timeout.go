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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
	// If not specified, "node-drain" is used.
	Flavor *string

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
}

// NodeDrainTimeoutSpec goes through the following steps:
// * Create cluster with 3 CP & 1 worker Machine
// * Ensure Node label is set & NodeDrainTimeout is set to 0 (wait forever)
// * Deploy Deployment with unevictable Pods on CP & MD Nodes
// * Deploy Deployment with evictable Pods with finalizer on CP & MD Nodes
// * Trigger Scale down to 1 CP and 0 MD Machines
// * Verify Node drains for control plane and MachineDeployment Machines are blocked (PDBs & Pods with finalizer)
//   - DrainingSucceeded conditions should:
//   - show 1 evicted Pod with deletionTimestamp (still exists because of finalizer)
//   - show 1 Pod which could not be evicted because of PDB
//   - Verify the evicted Pod has terminated (i.e. succeeded) and it was evicted
//
// * Unblock deletion of evicted Pods by removing the finalizer
// * Verify Node drains for control plane and MachineDeployment Machines are blocked (only PDBs)
//   - DrainingSucceeded conditions should:
//   - not contain any Pods with deletionTimestamp
//   - show 1 Pod which could not be evicted because of PDB
//
// * Set NodeDrainTimeout to 1s to unblock drain
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

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("A node should be forcefully removed if it cannot be drained in time", func() {
		By("Creating a workload cluster")
		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		controlPlaneReplicas := 3
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   ptr.Deref(input.Flavor, "node-drain"),
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
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

		By("Ensure Node label is set & NodeDrainTimeout is set to 0 (wait forever) on ControlPlane and MachineDeployment topologies")
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				topology.NodeDrainTimeout = &metav1.Duration{Duration: time.Duration(0)}
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
				topology.NodeDrainTimeout = &metav1.Duration{Duration: time.Duration(0)}
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

		By("Deploy Deployment with unevictable Pods on control plane Nodes.")
		cpDeploymentAndPDBName := fmt.Sprintf("%s-%s", "unevictable-pod-cp", util.RandomString(3))
		framework.DeployUnevictablePod(ctx, framework.DeployUnevictablePodInput{
			WorkloadClusterProxy:               workloadClusterProxy,
			ControlPlane:                       controlplane,
			DeploymentName:                     cpDeploymentAndPDBName,
			Namespace:                          "unevictable-workload",
			NodeSelector:                       map[string]string{nodeOwnerLabelKey: "KubeadmControlPlane-" + controlplane.Name},
			WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
		})
		By("Deploy Deployment with unevictable Pods on MachineDeployment Nodes.")
		mdDeploymentAndPDBNames := map[string]string{}
		for _, md := range machineDeployments {
			mdDeploymentAndPDBNames[md.Name] = fmt.Sprintf("%s-%s", "unevictable-pod-md", util.RandomString(3))
			framework.DeployUnevictablePod(ctx, framework.DeployUnevictablePodInput{
				WorkloadClusterProxy:               workloadClusterProxy,
				MachineDeployment:                  md,
				DeploymentName:                     mdDeploymentAndPDBNames[md.Name],
				Namespace:                          "unevictable-workload",
				NodeSelector:                       map[string]string{nodeOwnerLabelKey: "MachineDeployment-" + md.Name},
				WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
			})
		}

		By("Deploy Deployment with evictable Pods with finalizer on control plane Nodes.")
		cpDeploymentWithFinalizerName := fmt.Sprintf("%s-%s", "evictable-pod-cp", util.RandomString(3))
		framework.DeployEvictablePod(ctx, framework.DeployEvictablePodInput{
			WorkloadClusterProxy: workloadClusterProxy,
			ControlPlane:         controlplane,
			DeploymentName:       cpDeploymentWithFinalizerName,
			Namespace:            "evictable-workload",
			NodeSelector:         map[string]string{nodeOwnerLabelKey: "KubeadmControlPlane-" + controlplane.Name},
			ModifyDeployment: func(deployment *appsv1.Deployment) {
				deployment.Spec.Template.ObjectMeta.Finalizers = []string{"test.cluster.x-k8s.io/block"}
			},
			WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
		})
		By("Deploy Deployment with evictable Pods with finalizer on MachineDeployment Nodes.")
		mdDeploymentWithFinalizerName := map[string]string{}
		for _, md := range machineDeployments {
			mdDeploymentWithFinalizerName[md.Name] = fmt.Sprintf("%s-%s", "evictable-pod-md", util.RandomString(3))
			framework.DeployEvictablePod(ctx, framework.DeployEvictablePodInput{
				WorkloadClusterProxy: workloadClusterProxy,
				MachineDeployment:    md,
				DeploymentName:       mdDeploymentWithFinalizerName[md.Name],
				Namespace:            "evictable-workload",
				NodeSelector:         map[string]string{nodeOwnerLabelKey: "MachineDeployment-" + md.Name},
				ModifyDeployment: func(deployment *appsv1.Deployment) {
					deployment.Spec.Template.ObjectMeta.Finalizers = []string{"test.cluster.x-k8s.io/block"}
				},
				WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
			})
		}

		By("Trigger scale down the control plane to 1 and MachineDeployments to 0.")
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

		By("Verify Node drains for control plane and MachineDeployment Machines are blocked (PDBs & Pods with finalizer")
		var drainedCPMachine *clusterv1.Machine
		var evictedCPPod *corev1.Pod
		Eventually(func(g Gomega) {
			controlPlaneMachines := framework.GetControlPlaneMachinesByCluster(ctx, framework.GetControlPlaneMachinesByClusterInput{
				Lister:      input.BootstrapClusterProxy.GetClient(),
				ClusterName: cluster.Name,
				Namespace:   cluster.Namespace,
			})
			var condition *clusterv1.Condition
			for _, machine := range controlPlaneMachines {
				condition = conditions.Get(&machine, clusterv1.DrainingSucceededCondition)
				if condition != nil {
					// We only expect to find the condition on one Machine (as KCP will only try to drain one Machine at a time)
					drainedCPMachine = &machine
					break
				}
			}
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(corev1.ConditionFalse))
			// The evictable Pod should be evicted. It still blocks the drain because of the finalizer, otherwise the Pod would be gone already.
			g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("Pods with deletionTimestamp that still exist: evictable-workload/%s", cpDeploymentWithFinalizerName)))
			// The unevictable Pod should not be evicted because of the PDB.
			g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("Cannot evict pod as it would violate the pod's disruption budget. The disruption budget %s needs", cpDeploymentAndPDBName)))

			// Verify evictable Pod was evicted and terminated (i.e. phase is succeeded)
			evictedPods := &corev1.PodList{}
			g.Expect(workloadClusterProxy.GetClient().List(ctx, evictedPods,
				client.InNamespace("evictable-workload"),
				client.MatchingLabels{"deployment": cpDeploymentWithFinalizerName},
				client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", drainedCPMachine.Status.NodeRef.Name)},
			)).To(Succeed())
			g.Expect(evictedPods.Items).To(HaveLen(1))
			evictedCPPod = &evictedPods.Items[0]
			verifyPodEvictedAndSucceeded(g, evictedCPPod)
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		drainedMDMachines := map[string]*clusterv1.Machine{}
		evictedMDPods := map[string]*corev1.Pod{}
		for _, md := range machineDeployments {
			Eventually(func(g Gomega) {
				machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
					Lister:            input.BootstrapClusterProxy.GetClient(),
					ClusterName:       cluster.Name,
					Namespace:         cluster.Namespace,
					MachineDeployment: *md,
				})
				g.Expect(machines).To(HaveLen(1))
				drainedMDMachines[md.Name] = &machines[0]

				condition := conditions.Get(&machines[0], clusterv1.DrainingSucceededCondition)
				g.Expect(condition).ToNot(BeNil())
				g.Expect(condition.Status).To(Equal(corev1.ConditionFalse))
				// The evictable Pod should be evicted. It still blocks the drain because of the finalizer, otherwise the Pod would be gone already.
				g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("Pods with deletionTimestamp that still exist: evictable-workload/%s", mdDeploymentWithFinalizerName[md.Name])))
				// The unevictable Pod should not be evicted because of the PDB.
				g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("Cannot evict pod as it would violate the pod's disruption budget. The disruption budget %s needs", mdDeploymentAndPDBNames[md.Name])))

				// Verify evictable Pod was evicted and terminated (i.e. phase is succeeded)
				evictedPods := &corev1.PodList{}
				g.Expect(workloadClusterProxy.GetClient().List(ctx, evictedPods,
					client.InNamespace("evictable-workload"),
					client.MatchingLabels{"deployment": mdDeploymentWithFinalizerName[md.Name]},
					client.MatchingFieldsSelector{Selector: fields.OneTermEqualSelector("spec.nodeName", machines[0].Status.NodeRef.Name)},
				)).To(Succeed())
				g.Expect(evictedPods.Items).To(HaveLen(1))
				evictedMDPods[md.Name] = &evictedPods.Items[0]
				verifyPodEvictedAndSucceeded(g, evictedMDPods[md.Name])
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		}

		By("Unblock deletion of evicted Pods by removing the finalizer")
		Eventually(func(g Gomega) {
			g.Expect(workloadClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(evictedCPPod), evictedCPPod)).To(Succeed())
			originalPod := evictedCPPod.DeepCopy()
			evictedCPPod.Finalizers = []string{}
			g.Expect(workloadClusterProxy.GetClient().Patch(ctx, evictedCPPod, client.MergeFrom(originalPod))).To(Succeed())
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		for _, md := range machineDeployments {
			Eventually(func(g Gomega) {
				evictedMDPod := evictedMDPods[md.Name]
				g.Expect(workloadClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(evictedMDPod), evictedMDPod)).To(Succeed())
				originalPod := evictedMDPod.DeepCopy()
				evictedMDPod.Finalizers = []string{}
				g.Expect(workloadClusterProxy.GetClient().Patch(ctx, evictedMDPod, client.MergeFrom(originalPod))).To(Succeed())
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		}

		By("Verify Node drains for control plane and MachineDeployment Machines are blocked (only PDBs")
		Eventually(func(g Gomega) {
			g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(drainedCPMachine), drainedCPMachine)).To(Succeed())

			condition := conditions.Get(drainedCPMachine, clusterv1.DrainingSucceededCondition)
			g.Expect(condition).ToNot(BeNil())
			g.Expect(condition.Status).To(Equal(corev1.ConditionFalse))
			// The evictable Pod should be gone now.
			g.Expect(condition.Message).ToNot(ContainSubstring("Pods with deletionTimestamp that still exist"))
			// The unevictable Pod should still not be evicted because of the PDB.
			g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("Cannot evict pod as it would violate the pod's disruption budget. The disruption budget %s needs", cpDeploymentAndPDBName)))
		}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		for _, md := range machineDeployments {
			Eventually(func(g Gomega) {
				g.Expect(input.BootstrapClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(drainedMDMachines[md.Name]), drainedMDMachines[md.Name])).To(Succeed())

				condition := conditions.Get(drainedMDMachines[md.Name], clusterv1.DrainingSucceededCondition)
				g.Expect(condition).ToNot(BeNil())
				g.Expect(condition.Status).To(Equal(corev1.ConditionFalse))
				// The evictable Pod should be gone now.
				g.Expect(condition.Message).ToNot(ContainSubstring("Pods with deletionTimestamp that still exist"))
				// The unevictable Pod should still not be evicted because of the PDB.
				g.Expect(condition.Message).To(ContainSubstring(fmt.Sprintf("Cannot evict pod as it would violate the pod's disruption budget. The disruption budget %s needs", mdDeploymentAndPDBNames[md.Name])))
			}, input.E2EConfig.GetIntervals(specName, "wait-machine-deleted")...).Should(Succeed())
		}

		By("Set NodeDrainTimeout to 1s to unblock Node drain")
		// Note: This also verifies that KCP & MachineDeployments are still propagating changes to NodeDrainTimeout down to
		// Machines that already have a deletionTimestamp.
		drainTimeout := &metav1.Duration{Duration: time.Duration(1) * time.Second}
		modifyControlPlaneViaClusterAndWait(ctx, modifyControlPlaneViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyControlPlaneTopology: func(topology *clusterv1.ControlPlaneTopology) {
				topology.NodeDrainTimeout = drainTimeout
			},
			WaitForControlPlane: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		})
		modifyMachineDeploymentViaClusterAndWait(ctx, modifyMachineDeploymentViaClusterAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ModifyMachineDeploymentTopology: func(topology *clusterv1.MachineDeploymentTopology) {
				topology.NodeDrainTimeout = drainTimeout
			},
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Verify scale down succeeded because Node drains were unblocked")
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

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
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

func getDrainAndDeleteInterval(deleteInterval []interface{}, drainTimeout *metav1.Duration, replicas int) []interface{} {
	deleteTimeout, err := time.ParseDuration(deleteInterval[0].(string))
	Expect(err).ToNot(HaveOccurred())
	// We add the drain timeout to the specified delete timeout per replica.
	intervalDuration := (drainTimeout.Duration + deleteTimeout) * time.Duration(replicas)
	res := []interface{}{intervalDuration.String(), deleteInterval[1]}
	return res
}
