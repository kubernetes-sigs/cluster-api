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
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

// MachineDeploymentUpgradesSpecInput is the input for MachineDeploymentUpgradesSpec.
type MachineDeploymentUpgradesSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
}

// MachineDeploymentUpgradesSpec implements a test that verifies that MachineDeployment upgrades are successful.
func MachineDeploymentUpgradesSpec(ctx context.Context, inputGetter func() MachineDeploymentUpgradesSpecInput) {
	var (
		specName      = "md-upgrades"
		input         MachineDeploymentUpgradesSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		cluster       *clusterv1.Cluster
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveValidVersion(input.E2EConfig.GetVariable(KubernetesVersion)))
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersionUpgradeFrom))
		Expect(input.E2EConfig.Variables).To(HaveValidVersion(input.E2EConfig.GetVariable(KubernetesVersionUpgradeFrom)))
		Expect(input.E2EConfig.Variables).To(HaveKey(CNIPath))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
	})

	It("Should successfully upgrade Machines upon changes in relevant MachineDeployment fields", func() {

		By("Creating a workload cluster")

		var mds []*clusterv1.MachineDeployment
		cluster, _, mds = clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   clusterctl.DefaultFlavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("cluster-%s", util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersionUpgradeFrom),
				ControlPlaneMachineCount: pointer.Int64Ptr(1),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			CNIManifestPath:              input.E2EConfig.GetVariable(CNIPath),
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Upgrading MachineDeployment's Kubernetes version to a valid version")
		framework.UpgradeMachineDeploymentsAndWait(context.TODO(), framework.UpgradeMachineDeploymentsAndWaitInput{
			ClusterProxy:                input.BootstrapClusterProxy,
			Cluster:                     cluster,
			UpgradeVersion:              input.E2EConfig.GetVariable(KubernetesVersion),
			WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			MachineDeployments:          mds,
		})

		By("Upgrading MachineDeployment Infrastructure ref and wait for rolling upgrade")
		framework.UpgradeMachineDeploymentInfrastructureRefAndWait(context.TODO(), framework.UpgradeMachineDeploymentInfrastructureRefAndWaitInput{
			ClusterProxy:                input.BootstrapClusterProxy,
			Cluster:                     cluster,
			WaitForMachinesToBeUpgraded: input.E2EConfig.GetIntervals(specName, "wait-machine-upgrade"),
			MachineDeployments:          mds,
		})
		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
