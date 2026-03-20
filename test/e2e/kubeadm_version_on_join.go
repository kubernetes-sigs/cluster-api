/*
Copyright 2025 The Kubernetes Authors.

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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/test/e2e/internal/log"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
)

// KubeadmVersionOnJoinSpecInput is the input for KubeadmVersionOnJoinSpec.
type KubeadmVersionOnJoinSpecInput struct {
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
}

// KubeadmVersionOnJoinSpec verifies that when a worker Machine joins a cluster during an
// upgrade, the bootstrap controller uses the control plane's Kubernetes version for the
// kubeadm version file. A fetch-kubeadm.sh preKubeadmCommand then downloads the matching
// kubeadm binary so that kubeadm join succeeds against the upgraded control plane, even
// though the worker's spec version is still the old version.
func KubeadmVersionOnJoinSpec(ctx context.Context, inputGetter func() KubeadmVersionOnJoinSpecInput) {
	const specName = "kubeadm-version-on-join"

	var (
		input            KubeadmVersionOnJoinSpecInput
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

		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should use the control plane kubeadm version when a worker joins during an upgrade", func() {
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
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		Expect(clusterResources.Cluster).ToNot(BeNil())
		Expect(clusterResources.Cluster.Spec.Topology.IsDefined()).To(BeTrue(), "Cluster must use ClusterClass topology")
		Expect(clusterResources.MachineDeployments).To(HaveLen(1))

		mgmtClient := input.BootstrapClusterProxy.GetClient()
		cluster := clusterResources.Cluster

		By("Adding defer-upgrade and skip-preflight-checks annotations to the MachineDeployment topology")
		patchHelper, err := patch.NewHelper(cluster, mgmtClient)
		Expect(err).ToNot(HaveOccurred())

		mdTopology := cluster.Spec.Topology.Workers.MachineDeployments[0]
		if mdTopology.Metadata.Annotations == nil {
			mdTopology.Metadata.Annotations = map[string]string{}
		}
		mdTopology.Metadata.Annotations[clusterv1.ClusterTopologyDeferUpgradeAnnotation] = ""
		mdTopology.Metadata.Annotations[clusterv1.MachineSetSkipPreflightChecksAnnotation] = string(clusterv1.MachineSetPreflightCheckAll)
		cluster.Spec.Topology.Workers.MachineDeployments[0] = mdTopology
		Eventually(func() error {
			return patchHelper.Patch(ctx, cluster)
		}, 1*time.Minute, 10*time.Second).Should(Succeed(), "Failed to add annotations to MachineDeployment topology")

		By("Waiting for the skip-preflight-checks annotation to propagate to the MachineSet")
		Eventually(func() bool {
			msList := &clusterv1.MachineSetList{}
			if err := mgmtClient.List(ctx, msList,
				client.InNamespace(cluster.Namespace),
				client.MatchingLabels{
					clusterv1.ClusterNameLabel:                          cluster.Name,
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md-0",
				},
			); err != nil {
				return false
			}
			for i := range msList.Items {
				if v, ok := msList.Items[i].Annotations[clusterv1.MachineSetSkipPreflightChecksAnnotation]; ok && v != "" {
					log.Logf("MachineSet %s has skip-preflight-checks annotation: %s", msList.Items[i].Name, v)
					return true
				}
			}
			return false
		}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
			"Timed out waiting for skip-preflight-checks annotation to propagate to MachineSet")

		By("Upgrading the Cluster topology version to trigger CP upgrade")
		patchHelper, err = patch.NewHelper(cluster, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		cluster.Spec.Topology.Version = kubernetesVersionUpgradeTo
		Eventually(func() error {
			return patchHelper.Patch(ctx, cluster)
		}, 1*time.Minute, 10*time.Second).Should(Succeed(), "Failed to patch Cluster topology version")

		By("Waiting for control plane machines to be upgraded")
		framework.WaitForControlPlaneMachinesToBeUpgraded(ctx, framework.WaitForControlPlaneMachinesToBeUpgradedInput{
			Lister:                   mgmtClient,
			Cluster:                  cluster,
			MachineCount:             1,
			KubernetesUpgradeVersion: kubernetesVersionUpgradeTo,
		}, input.E2EConfig.GetIntervals(specName, "wait-control-plane-upgrade")...)

		By("Verifying worker machines have NOT been upgraded (deferred)")
		md := clusterResources.MachineDeployments[0]
		machines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
			Lister:            mgmtClient,
			ClusterName:       cluster.Name,
			Namespace:         cluster.Namespace,
			MachineDeployment: *md,
		})
		Expect(machines).To(HaveLen(1), "Expected exactly 1 worker machine")
		originalMachine := machines[0]
		Expect(originalMachine.Spec.Version).To(Equal(kubernetesVersionUpgradeFrom),
			"Worker machine %s should still be at old version %s, got %s", originalMachine.Name, kubernetesVersionUpgradeFrom, originalMachine.Spec.Version)
		log.Logf("Original worker machine: %s (version %s)", originalMachine.Name, originalMachine.Spec.Version)

		By("Scaling the MachineDeployment directly to 2 replicas")
		// The topology controller skips MachineDeployment reconciliation while the upgrade is
		// deferred, so we scale the underlying MachineDeployment object directly.
		Expect(mgmtClient.Get(ctx, client.ObjectKeyFromObject(md), md)).To(Succeed())
		framework.ScaleAndWaitMachineDeployment(ctx, framework.ScaleAndWaitMachineDeploymentInput{
			ClusterProxy:              input.BootstrapClusterProxy,
			Cluster:                   cluster,
			MachineDeployment:         md,
			Replicas:                  2,
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Identifying the new worker Machine")
		currentMachines := framework.GetMachinesByMachineDeployments(ctx, framework.GetMachinesByMachineDeploymentsInput{
			Lister:            mgmtClient,
			ClusterName:       cluster.Name,
			Namespace:         cluster.Namespace,
			MachineDeployment: *md,
		})
		var newMachine *clusterv1.Machine
		for i := range currentMachines {
			if currentMachines[i].Name != originalMachine.Name {
				newMachine = &currentMachines[i]
				break
			}
		}
		Expect(newMachine).ToNot(BeNil(), "Could not find new Machine (original: %s)", originalMachine.Name)
		log.Logf("New worker machine: %s, nodeRef: %s", newMachine.Name, newMachine.Status.NodeRef.Name)

		By("Verifying the kubeadm version file and fetch-kubeadm.sh on the new node")
		containerName := newMachine.Status.NodeRef.Name
		verifyKubeadmVersionOnNode(ctx, containerName, kubernetesVersionUpgradeTo)

		By("Removing the defer-upgrade and skip-preflight-checks annotations and syncing topology replicas")
		// Update the topology replicas to 2 to match the direct MD scaling we did above,
		// so the topology controller doesn't scale the MachineDeployment back down.
		Expect(mgmtClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
		patchHelper, err = patch.NewHelper(cluster, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		mdTopology = cluster.Spec.Topology.Workers.MachineDeployments[0]
		delete(mdTopology.Metadata.Annotations, clusterv1.ClusterTopologyDeferUpgradeAnnotation)
		delete(mdTopology.Metadata.Annotations, clusterv1.MachineSetSkipPreflightChecksAnnotation)
		mdTopology.Replicas = ptr.To[int32](2)
		cluster.Spec.Topology.Workers.MachineDeployments[0] = mdTopology
		Eventually(func() error {
			return patchHelper.Patch(ctx, cluster)
		}, 1*time.Minute, 10*time.Second).Should(Succeed(), "Failed to remove annotations and sync replicas")

		By("Waiting for all worker machines to be upgraded")
		mdList := &clusterv1.MachineDeploymentList{}
		Expect(mgmtClient.List(ctx, mdList,
			client.InNamespace(cluster.Namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel:                          cluster.Name,
				clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md-0",
			},
		)).To(Succeed())
		Expect(mdList.Items).To(HaveLen(1))
		framework.WaitForMachineDeploymentMachinesToBeUpgraded(ctx, framework.WaitForMachineDeploymentMachinesToBeUpgradedInput{
			Lister:                   mgmtClient,
			Cluster:                  cluster,
			MachineCount:             2,
			KubernetesUpgradeVersion: kubernetesVersionUpgradeTo,
			MachineDeployment:        mdList.Items[0],
		}, input.E2EConfig.GetIntervals(specName, "wait-worker-nodes")...)

		Byf("Verify Cluster Available condition is true")
		framework.VerifyClusterAvailable(ctx, framework.VerifyClusterAvailableInput{
			Getter:    mgmtClient,
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		})

		By("PASSED!")
	})

	AfterEach(func() {
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ClusterctlConfigPath, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

// verifyKubeadmVersionOnNode verifies the kubeadm version file and kubeadm binary
// version on the CAPD container match the expected version.
func verifyKubeadmVersionOnNode(ctx context.Context, containerName, expectedVersion string) {
	containerRuntime, err := container.NewDockerClient()
	Expect(err).ToNot(HaveOccurred(), "Failed to create container runtime client")

	// The version file is written by the bootstrap controller using semver.String(),
	// which strips the "v" prefix (e.g. "1.35.0" not "v1.35.0").
	expectedVersionNoPfx := strings.TrimPrefix(expectedVersion, "v")

	// Verify the version file was written with the expected content.
	log.Logf("Checking /run/cluster-api/kubeadm-version on container %s", containerName)
	out, err := execInContainer(ctx, containerRuntime, containerName, "cat", "/run/cluster-api/kubeadm-version")
	Expect(err).ToNot(HaveOccurred(), "Failed to read kubeadm version file: %s", out)
	versionFileContent := strings.TrimSpace(out)
	Expect(versionFileContent).To(Equal(expectedVersionNoPfx),
		"Version file content %q does not match expected %q", versionFileContent, expectedVersionNoPfx)
	log.Logf("Version file contains: %s", versionFileContent)

	// Verify that fetch-kubeadm.sh ran and found the version file. This proves the
	// version file was present before kubeadm join (i.e. before preKubeadmCommands ran).
	log.Logf("Checking fetch-kubeadm.log on container %s", containerName)
	out, err = execInContainer(ctx, containerRuntime, containerName, "cat", "/var/log/fetch-kubeadm.log")
	Expect(err).ToNot(HaveOccurred(), "Failed to read fetch-kubeadm.log: %s", out)
	log.Logf("fetch-kubeadm.log:\n%s", out)
	Expect(out).To(ContainSubstring("raw version from file:"),
		"fetch-kubeadm.sh must find the version file; log:\n%s", out)

	// Log the kubeadm binary version (soft check -- the curl download may fail in
	// environments without internet access, so we don't fail on a version mismatch).
	log.Logf("Checking kubeadm version on container %s", containerName)
	out, err = execInContainer(ctx, containerRuntime, containerName, "kubeadm", "version", "-o", "short")
	if err == nil {
		kubeadmVersion := strings.TrimSpace(out)
		log.Logf("kubeadm version: %s (expected %s)", kubeadmVersion, expectedVersion)
	} else {
		log.Logf("Could not get kubeadm version: %s", out)
	}
}

// execInContainer runs a command in a container and returns the combined stdout/stderr output.
func execInContainer(ctx context.Context, cr container.Runtime, containerName, command string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	err := cr.ExecContainer(ctx, containerName, &container.ExecContainerInput{
		OutputBuffer: &stdout,
		ErrorBuffer:  &stderr,
	}, command, args...)
	combined := stdout.String() + stderr.String()
	return combined, err
}
