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
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	cloudv1 "sigs.k8s.io/cluster-api/test/infrastructure/inmemory/pkg/cloud/api/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
)

// EtcdDefragSpecInput is the input for EtcdDefragSpec.
type EtcdDefragSpecInput struct {
	// This spec requires the following intervals to be defined:
	//   - wait-control-plane: time budget for the KubeadmControlPlane to become ready.
	//   - wait-defrag:        polling budget for all etcd pods to show a non-zero defrag-count annotation.
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// InfrastructureProvider specifies the infrastructure to use for clusterctl operations.
	// If not set, clusterctl will use the single provider installed in the management cluster.
	InfrastructureProvider *string

	// Allows to inject a function to be run after the test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
}

// EtcdDefragSpec validates that KubeadmControlPlane's automatic etcd defragmentation feature
// works end-to-end, including the MinDefragIntervalSeconds rate-limiting:
//
//  1. A 3-node control-plane cluster is created using the "etcd-defrag" template, which
//     uses the in-memory backend, sets defragRule: "dbSize >= 0.0" (always-true), and
//     minDefragIntervalSeconds: 5.
//
//  2. Once the cluster is ready the KCP reconcile loop calls DefragEtcdMember on each
//     member.  The in-memory fake Defragment handler increments the
//     etcd.inmemory.../defrag-count annotation on each etcd pod.
//
//  3. The test asserts that every non-learner etcd pod has defrag-count >= 1 (first cycle),
//     then checks that KubeadmControlPlane.Status.EtcdMemberDefragTimes is populated for
//     every member.
//
//  4. Because minDefragIntervalSeconds is 5, the test then waits for defrag-count >= 2 (second
//     cycle), proving that the throttle allows defrag to run again after the interval.
func EtcdDefragSpec(ctx context.Context, inputGetter func() EtcdDefragSpecInput) {
	const specName = "etcd-defrag"

	var (
		input            EtcdDefragSpecInput
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

		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should defragment every etcd member once the KubeadmControlPlane is ready", func() {
		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		By("Creating a 3-node control-plane cluster with etcdMaintenance.defragRule set")
		clusterName := fmt.Sprintf("%s-%s", specName, util.RandomString(6))
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:              filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:   input.ClusterctlConfigPath,
				KubeconfigPath:         input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider: infrastructureProvider,
				Flavor:                 specName,
				Namespace:              namespace.Name,
				ClusterName:            clusterName,
				KubernetesVersion:      input.E2EConfig.MustGetVariable(KubernetesVersion),
				// Three control-plane nodes ensure followers-before-leader ordering is exercised.
				ControlPlaneMachineCount: ptr.To[int64](3),
				// No worker nodes needed — we only test KCP etcd maintenance.
				WorkerMachineCount: ptr.To[int64](0),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
		}, clusterResources)

		By("Waiting for all etcd pods to show defrag-count >= 1")
		// The KCP reconcile loop calls reconcileEtcdDefragmentation after the cluster is Ready.
		// With defragRule "dbSize >= 0.0" (always true) and 3 members it will defragment one
		// member per reconcile, requeue after 5 s, and repeat until all are done.
		// The in-memory Defragment handler increments the defrag-count annotation on each pod.
		wlProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx,
			clusterResources.Cluster.Namespace, clusterResources.Cluster.Name)
		wlClient := wlProxy.GetClient()

		Eventually(func() (bool, error) {
			etcdPods := &corev1.PodList{}
			if err := wlClient.List(ctx, etcdPods,
				client.InNamespace("kube-system"),
				client.MatchingLabels{
					"component": "etcd",
					"tier":      "control-plane",
				},
			); err != nil {
				return false, err
			}

			if len(etcdPods.Items) == 0 {
				return false, nil
			}

			for i := range etcdPods.Items {
				pod := &etcdPods.Items[i]
				countStr, ok := pod.Annotations[cloudv1.EtcdDefragCountAnnotationName]
				if !ok {
					return false, nil
				}
				count, err := strconv.Atoi(countStr)
				if err != nil {
					return false, err
				}
				if count < 1 {
					return false, nil
				}
			}
			return true, nil
		}, input.E2EConfig.GetIntervals(specName, "wait-defrag")...).Should(
			BeTrue(),
			"Expected all etcd members to be defragmented at least once, but some pods are missing the %s annotation",
			cloudv1.EtcdDefragCountAnnotationName,
		)

		By("Verifying KubeadmControlPlane.Status.EtcdMemberDefragTimes is populated for every etcd member after the first cycle")
		// The controller stamps a per-member timestamp in EtcdMemberDefragTimes immediately after
		// each successful defragmentation. Reading the live KCP object confirms the status was persisted.
		mgmtClient := input.BootstrapClusterProxy.GetClient()
		liveKCP := &controlplanev1.KubeadmControlPlane{}
		Expect(mgmtClient.Get(ctx, client.ObjectKeyFromObject(clusterResources.ControlPlane), liveKCP)).To(Succeed())
		Expect(liveKCP.Status.EtcdMemberDefragTimes).ToNot(BeEmpty(),
			"KubeadmControlPlane.Status.EtcdMemberDefragTimes must be populated after the first defrag cycle")

		By("Waiting for all etcd pods to show defrag-count >= 2 (verifies MinDefragIntervalSeconds allows a second cycle)")
		// With minDefragIntervalSeconds: 5 the controller requeues after the interval and
		// runs a second defrag cycle shortly after the first one completes.
		Eventually(func() (bool, error) {
			etcdPods := &corev1.PodList{}
			if err := wlClient.List(ctx, etcdPods,
				client.InNamespace("kube-system"),
				client.MatchingLabels{
					"component": "etcd",
					"tier":      "control-plane",
				},
			); err != nil {
				return false, err
			}
			for i := range etcdPods.Items {
				pod := &etcdPods.Items[i]
				countStr, ok := pod.Annotations[cloudv1.EtcdDefragCountAnnotationName]
				if !ok {
					return false, nil
				}
				count, err := strconv.Atoi(countStr)
				if err != nil {
					return false, err
				}
				if count < 2 {
					return false, nil
				}
			}
			return true, nil
		}, input.E2EConfig.GetIntervals(specName, "wait-second-defrag")...).Should(
			BeTrue(),
			"Expected all etcd members to be defragmented at least twice (MinDefragInterval respected), "+
				"but some pods have defrag-count < 2 for annotation %s",
			cloudv1.EtcdDefragCountAnnotationName,
		)

		By("Verifying the KubeadmControlPlane is still fully Ready after defragmentation")
		framework.WaitForControlPlaneToBeReady(ctx, framework.WaitForControlPlaneToBeReadyInput{
			Getter:       input.BootstrapClusterProxy.GetClient(),
			ControlPlane: clusterResources.ControlPlane,
		}, input.E2EConfig.GetIntervals(specName, "wait-control-plane")...)

		By("PASSED!")
	})

	AfterEach(func() {
		framework.DumpSpecResourcesAndCleanup(ctx, specName,
			input.BootstrapClusterProxy, input.ClusterctlConfigPath,
			input.ArtifactFolder, namespace, cancelWatches,
			clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
