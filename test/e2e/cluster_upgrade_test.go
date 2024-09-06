//go:build e2e
// +build e2e

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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("When upgrading a workload cluster using ClusterClass and testing K8S conformance [Conformance] [K8s-Upgrade] [ClusterClass]", func() {
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			Flavor:                 ptr.To("upgrades"),
		}
	})
})

var _ = Describe("When upgrading a workload cluster using ClusterClass [ClusterClass]", func() {
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			Flavor:                 ptr.To("topology"),
			// This test is run in CI in parallel with other tests. To keep the test duration reasonable
			// the conformance tests are skipped.
			ControlPlaneMachineCount: ptr.To[int64](1),
			WorkerMachineCount:       ptr.To[int64](2),
			SkipConformanceTests:     true,
		}
	})
})

var _ = Describe("When upgrading a workload cluster using ClusterClass with a HA control plane [ClusterClass]", Label("Foo"), func() {
	controlPlaneMachineCount := int64(3)
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			// This test is run in CI in parallel with other tests. To keep the test duration reasonable
			// the conformance tests are skipped.
			SkipConformanceTests:     true,
			ControlPlaneMachineCount: ptr.To[int64](controlPlaneMachineCount),
			WorkerMachineCount:       ptr.To[int64](1),
			Flavor:                   ptr.To("kcp-pre-drain"),
			PreWaitForControlPlaneToBeUpgraded: func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace, workloadClusterName string) {
				log.Logf("Waiting for control-plane machines to have the upgraded Kubernetes version")

				preDrainHook := "pre-drain.delete.hook.machine.cluster.x-k8s.io/kcp-ready-check"

				cluster := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: workloadClusterNamespace,
						Name:      workloadClusterName,
					},
				}
				Expect(managementClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())

				// This replaces the WaitForControlPlaneMachinesToBeUpgraded function and additionally:
				// * checks that kube-proxy is healthy
				// * removes the pre-drain hook from kcp machines if all is healthy.
				Eventually(func() (int64, error) {
					machines := framework.GetControlPlaneMachinesByCluster(ctx, framework.GetControlPlaneMachinesByClusterInput{
						Lister:      managementClusterProxy.GetClient(),
						ClusterName: cluster.Name,
						Namespace:   cluster.Namespace,
					})

					// Collect information about:
					// * how many control-plane machines already got upgraded
					// * control-plane machines which are in deletion but waiting for the pre-drain hook
					// * workload cluster nodes
					// * kube-proxy pods
					var upgraded int64
					deletingMachinesWithPreDrainHook := []clusterv1.Machine{}
					for _, m := range machines {
						if *m.Spec.Version == cluster.Spec.Topology.Version && conditions.IsTrue(&m, clusterv1.MachineNodeHealthyCondition) {
							upgraded++
						}
						if !m.DeletionTimestamp.IsZero() && m.Annotations[preDrainHook] == "true" {
							deletingMachinesWithPreDrainHook = append(deletingMachinesWithPreDrainHook, m)
						}
					}

					wlClient := managementClusterProxy.GetWorkloadCluster(ctx, workloadClusterNamespace, workloadClusterName).GetClient()
					nodes := corev1.NodeList{}
					if err := wlClient.List(ctx, &nodes); err != nil {
						return 0, errors.New("failed to list nodes in workload cluster")
					}

					kubeProxyPods := corev1.PodList{}
					if err := wlClient.List(ctx, &kubeProxyPods, client.InNamespace(metav1.NamespaceSystem), client.MatchingLabels{"k8s-app": "kube-proxy"}); err != nil {
						return 0, errors.New("failed to list kube-proxy pods in workload cluster")
					}

					errList := []error{}

					// Check all nodes to be Ready.
					for _, node := range nodes.Items {
						for _, condition := range node.Status.Conditions {
							if condition.Type != corev1.NodeReady || condition.Status == corev1.ConditionTrue {
								continue
							}
							errList = append(errList, errors.Errorf("Node's %s Ready condition is false", node.GetName()))
						}
					}

					// Check if the expected number of kube-proxy pods exist and all of them are healthy.
					if len(nodes.Items) != len(kubeProxyPods.Items) {
						errList = append(errList, errors.Errorf("exected %d kube-proxy pods to exist, got %d", len(nodes.Items), len(kubeProxyPods.Items)))
					}
					for _, pod := range kubeProxyPods.Items {
						for _, condition := range pod.Status.Conditions {
							if condition.Type != corev1.PodReady || condition.Status == corev1.ConditionTrue {
								continue
							}
							errList = append(errList, errors.Errorf("Pod's %s Ready condition is false", pod.GetName()))
						}
					}

					if err := kerrors.NewAggregate(errList); err != nil {
						return 0, errors.Wrap(err, "blocking upgrade because cluster is not stable")
					}

					// Remove pre-drain webhook from machines because all current machines are considered ok.
					if len(deletingMachinesWithPreDrainHook) > 0 {
						if len(deletingMachinesWithPreDrainHook) > 1 {
							return 0, errors.Errorf("expected a maximum of 1 control-plane machines to be in deleting and having the pre-drain hook but got %d", len(deletingMachinesWithPreDrainHook))
						}

						// Removing pre-drain hook from machine.
						m := &deletingMachinesWithPreDrainHook[0]
						patchHelper, err := patch.NewHelper(m, managementClusterProxy.GetClient())
						if err != nil {
							return 0, errors.Wrapf(err, "creating patchHelper for Machine %s", klog.KObj(m))
						}
						delete(m.Annotations, preDrainHook)

						if err := patchHelper.Patch(ctx, m); err != nil {
							return 0, errors.Wrapf(err, "failed to remove pre-drain annotation from Machine %s", klog.KObj(m))
						}

						// Return to enter the function again.
						return 0, errors.Errorf("deletion of Machine %s was blocked by pre-drain hook", klog.KObj(m))
					}

					if int64(len(machines)) > upgraded {
						return 0, errors.New("old Machines remain")
					}

					return upgraded, nil
				}, e2eConfig.GetIntervals("k8s-upgrade-and-conformance", "wait-machine-upgrade")...).Should(Equal(controlPlaneMachineCount), "Timed out waiting for all control-plane machines in Cluster %s to be upgraded to kubernetes version %s", klog.KObj(cluster), cluster.Spec.Topology.Version)
			},
		}
	})
})

var _ = Describe("When upgrading a workload cluster using ClusterClass with a HA control plane using scale-in rollout [ClusterClass]", func() {
	ClusterUpgradeConformanceSpec(ctx, func() ClusterUpgradeConformanceSpecInput {
		return ClusterUpgradeConformanceSpecInput{
			E2EConfig:              e2eConfig,
			ClusterctlConfigPath:   clusterctlConfigPath,
			BootstrapClusterProxy:  bootstrapClusterProxy,
			ArtifactFolder:         artifactFolder,
			SkipCleanup:            skipCleanup,
			InfrastructureProvider: ptr.To("docker"),
			// This test is run in CI in parallel with other tests. To keep the test duration reasonable
			// the conformance tests are skipped.
			SkipConformanceTests:     true,
			ControlPlaneMachineCount: ptr.To[int64](3),
			WorkerMachineCount:       ptr.To[int64](1),
			Flavor:                   ptr.To("kcp-scale-in"),
		}
	})
})
