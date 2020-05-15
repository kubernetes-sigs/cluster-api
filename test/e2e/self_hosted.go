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
	"sigs.k8s.io/cluster-api/test/framework/bootstrap"
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
}

// SelfHostedSpec implements a test that verifies Cluster API creating a cluster, pivoting to a self-hosted cluster.
func SelfHostedSpec(ctx context.Context, inputGetter func() SelfHostedSpecInput) {
	var (
		specName      = "self-hosted"
		input         SelfHostedSpecInput
		namespace     *corev1.Namespace
		cancelWatches context.CancelFunc
		cluster       *clusterv1.Cluster

		selfHostedClusterProxy  framework.ClusterProxy
		selfHostedNamespace     *corev1.Namespace
		selfHostedCancelWatches context.CancelFunc
		selfHostedCluster       *clusterv1.Cluster
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0755)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)
		Expect(input.E2EConfig.Variables).To(HaveKey(CNIPath))
		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(context.TODO(), specName, input.BootstrapClusterProxy, input.ArtifactFolder)
	})

	It("Should pivot the bootstrap cluster to a self-hosted cluster", func() {

		By("Creating a workload cluster")

		cluster, _, _ = clusterctl.ApplyClusterTemplateAndWait(context.TODO(), clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   clusterctl.DefaultFlavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("cluster-%s", util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: pointer.Int64Ptr(1),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			CNIManifestPath:              input.E2EConfig.GetVariable(CNIPath),
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Turning the workload cluster into a management cluster")
		//TODO: refactor into an helper func e.g. "UpgradeToManagementCluster"

		// In case of the cluster id a DockerCluster, we should load controller images into the nodes.
		// Nb. this can be achieved also by changing the DockerMachine spec, but for the time being we are using
		// this approach because this allows to have a single source of truth for images, the e2e config
		if cluster.Spec.InfrastructureRef.Kind == "DockerCluster" {
			bootstrap.LoadImagesToKindCluster(context.TODO(), bootstrap.LoadImagesToKindClusterInput{
				Name:   cluster.Name,
				Images: input.E2EConfig.Images,
			})
		}

		// Get a ClusterBroker so we can interact with the workload cluster
		selfHostedClusterProxy = input.BootstrapClusterProxy.GetWorkloadCluster(context.TODO(), cluster.Namespace, cluster.Name)

		Byf("Creating a namespace for hosting the %s test spec", specName)
		selfHostedNamespace, selfHostedCancelWatches = framework.CreateNamespaceAndWatchEvents(context.TODO(), framework.CreateNamespaceAndWatchEventsInput{
			Creator:   selfHostedClusterProxy.GetClient(),
			ClientSet: selfHostedClusterProxy.GetClientSet(),
			Name:      namespace.Name,
			LogFolder: filepath.Join(input.ArtifactFolder, "clusters", "bootstrap"),
		})

		By("Initializing the workload cluster")
		clusterctl.InitManagementClusterAndWatchControllerLogs(context.TODO(), clusterctl.InitManagementClusterAndWatchControllerLogsInput{
			ClusterProxy:            selfHostedClusterProxy,
			ClusterctlConfigPath:    input.ClusterctlConfigPath,
			InfrastructureProviders: input.E2EConfig.InfrastructureProviders(),
			LogFolder:               filepath.Join(input.ArtifactFolder, "clusters", "self-hosted"),
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		//TODO: refactor in to an helper func e.g. "MoveToSelfHostedAndWait"
		By("Moving the cluster to self hosted")
		clusterctl.Move(context.TODO(), clusterctl.MoveInput{
			LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", "bootstrap"),
			ClusterctlConfigPath: input.ClusterctlConfigPath,
			FromKubeconfigPath:   input.BootstrapClusterProxy.GetKubeconfigPath(),
			ToKubeconfigPath:     selfHostedClusterProxy.GetKubeconfigPath(),
			Namespace:            namespace.Name,
		})

		fmt.Fprintf(GinkgoWriter, "Waiting for the cluster infrastructure to be provisioned\n")
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

		By("PASSED!")
	})

	AfterEach(func() {
		//TODO: refactor in to an helper func e.g. "MoveToBootstrapAndWait"
		if selfHostedNamespace != nil {
			// Dump all Cluster API related resources to artifacts before pivoting back.
			framework.DumpAllResources(ctx, framework.DumpAllResourcesInput{
				Lister:    selfHostedClusterProxy.GetClient(),
				Namespace: namespace.Name,
				LogPath:   filepath.Join(input.ArtifactFolder, "clusters", "self-hosted", "resources"),
			})
		}
		if selfHostedCluster != nil {
			By("Moving the cluster back to bootstrap")
			clusterctl.Move(ctx, clusterctl.MoveInput{
				LogFolder:            filepath.Join(input.ArtifactFolder, "clusters", "self-hosted"),
				ClusterctlConfigPath: input.ClusterctlConfigPath,
				FromKubeconfigPath:   selfHostedClusterProxy.GetKubeconfigPath(),
				ToKubeconfigPath:     input.BootstrapClusterProxy.GetKubeconfigPath(),
				Namespace:            selfHostedNamespace.Name,
			})

			fmt.Fprintf(GinkgoWriter, "Waiting for the cluster infrastructure to be provisioned\n")
			cluster = framework.DiscoveryAndWaitForCluster(ctx, framework.DiscoveryAndWaitForClusterInput{
				Getter:    input.BootstrapClusterProxy.GetClient(),
				Namespace: namespace.Name,
				Name:      cluster.Name,
			}, input.E2EConfig.GetIntervals(specName, "wait-cluster")...)
		}
		if selfHostedCancelWatches != nil {
			selfHostedCancelWatches()
		}

		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
