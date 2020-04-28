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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

const (
	PreviousKubernetesVersion = "KUBERNETES_VERSION_UPGRADE_FROM"
	EtcdCurrentVersion        = "ETCD_VERSION_CURRENT"
	CoreDNSCurrentVersion     = "COREDNS_VERSION_CURRENT"
)

// KCPUpgradeSpecInput is the input for KCPUpgradeSpec.
type KCPUpgradeSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
}

// KCPUpgradeSpec implements a test that verifies KCP to properly upgrade a control plane with 3 machines.
func KCPUpgradeSpec(ctx context.Context, inputGetter func() KCPUpgradeSpecInput) {
	var (
		specName      = "kcp-upgrade"
		input         KCPUpgradeSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		cluster       *clusterv1.Cluster
		controlPlane  *controlplanev1.KubeadmControlPlane
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)
		Expect(input.E2EConfig.Variables).To(HaveKey(PreviousKubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveKey(EtcdCurrentVersion))
		Expect(input.E2EConfig.Variables).To(HaveKey(CoreDNSCurrentVersion))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
	})

	It("Should successfully upgrade Kubernetes, DNS, kube-proxy, and etcd in a single control plane cluster", func() {

		By("Creating a workload cluster")
		Expect(input.E2EConfig.Variables).To(HaveKey(clusterctl.KubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveKey(clusterctl.CNIPath))

		cluster, controlPlane, _ = clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   clusterctl.DefaultFlavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("cluster-%s", util.RandomString(6)),
				KubernetesVersion:        input.GetPreviousKubernetesVersion(),
				ControlPlaneMachineCount: pointer.Int64Ptr(1),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			CNIManifestPath:              input.E2EConfig.GetCNIPath(),
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Upgrading Kubernetes, DNS, kube-proxy, and etcd versions")
		framework.UpgradeControlPlaneAndWaitForUpgrade(ctx, framework.UpgradeControlPlaneAndWaitForUpgradeInput{
			ClusterProxy: input.BootstrapClusterProxy,
			Cluster:      cluster,
			ControlPlane: controlPlane,
			//Valid image tags for v1.17.2
			EtcdImageTag:                "3.4.3-0",
			DNSImageTag:                 "1.6.7",
			KubernetesUpgradeVersion:    input.E2EConfig.GetKubernetesVersion(),
			WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForDNSUpgrade:           input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForEtcdUpgrade:          input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
		})

		By("PASSED!")
	})

	It("Should successfully upgrade Kubernetes, DNS, kube-proxy, and etcd in a HA cluster", func() {

		By("Creating a workload cluster")
		Expect(input.E2EConfig.Variables).To(HaveKey(clusterctl.KubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveKey(clusterctl.CNIPath))

		cluster, controlPlane, _ = clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   clusterctl.DefaultFlavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("cluster-%s", util.RandomString(6)),
				KubernetesVersion:        input.GetPreviousKubernetesVersion(),
				ControlPlaneMachineCount: pointer.Int64Ptr(3),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			CNIManifestPath:              input.E2EConfig.GetCNIPath(),
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Upgrading Kubernetes")
		framework.UpgradeControlPlaneAndWaitForUpgrade(ctx, framework.UpgradeControlPlaneAndWaitForUpgradeInput{
			ClusterProxy:                input.BootstrapClusterProxy,
			Cluster:                     cluster,
			ControlPlane:                controlPlane,
			EtcdImageTag:                input.GetEtcdCurrentVersion(),
			DNSImageTag:                 input.GetCoreDNSCurrentVersion(),
			KubernetesUpgradeVersion:    input.E2EConfig.GetKubernetesVersion(),
			WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForDNSUpgrade:           input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			WaitForEtcdUpgrade:          input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
		})

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

// GetPreviousKubernetesVersion returns the previous kubernetes version to test an upgrade from
func (k KCPUpgradeSpecInput) GetPreviousKubernetesVersion() string {
	return k.E2EConfig.Variables[PreviousKubernetesVersion]
}

// GetEtcdCurrentVersion returns the version of etcd to upgrade to
func (k KCPUpgradeSpecInput) GetEtcdCurrentVersion() string {
	return k.E2EConfig.Variables[EtcdCurrentVersion]
}

// GetCoreDNSUpgradeVersion returns the version of etcd to upgrade to
func (k KCPUpgradeSpecInput) GetCoreDNSCurrentVersion() string {
	return k.E2EConfig.Variables[CoreDNSCurrentVersion]
}
