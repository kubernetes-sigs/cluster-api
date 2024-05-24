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
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/framework/kubetest"
	"sigs.k8s.io/cluster-api/util"
)

// K8SConformanceSpecInput is the input for K8SConformanceSpec.
type K8SConformanceSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers (ex: CAPD + in-memory) are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string

	Flavor              string
	ControlPlaneWaiters clusterctl.ControlPlaneWaiters

	// Allows to inject a function to be run after test namespace is created.
	// If not specified, this is a no-op.
	PostNamespaceCreated func(managementClusterProxy framework.ClusterProxy, workloadClusterNamespace string)
}

// K8SConformanceSpec implements a spec that creates a cluster and runs Kubernetes conformance suite.
func K8SConformanceSpec(ctx context.Context, inputGetter func() K8SConformanceSpecInput) {
	const (
		kubetestConfigurationVariable = "KUBETEST_CONFIGURATION"
		kubetestNumberOfNodesVariable = "KUBETEST_NUMBER_OF_NODES"
		kubetestGinkgoNodesVariable   = "KUBETEST_GINKGO_NODES"
	)
	var (
		specName               = "k8s-conformance"
		input                  K8SConformanceSpecInput
		namespace              *corev1.Namespace
		cancelWatches          context.CancelFunc
		clusterResources       *clusterctl.ApplyClusterTemplateAndWaitResult
		kubetestConfigFilePath string
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))
		Expect(input.E2EConfig.Variables).To(HaveKey(kubetestConfigurationVariable), "% spec requires a %s variable to be defined in the config file", specName, kubetestConfigurationVariable)
		kubetestConfigFilePath = input.E2EConfig.GetVariable(kubetestConfigurationVariable)
		Expect(kubetestConfigFilePath).To(BeAnExistingFile(), "%s should be a valid kubetest config file")

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = framework.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, input.PostNamespaceCreated)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a workload cluster and run kubetest", func() {
		By("Creating a workload cluster")

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		// NOTE: The number of CP nodes does not have relevance for conformance; instead, the number of workers allows
		// better parallelism of tests and thus a lower execution time.
		var workerMachineCount int64 = 5

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
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: ptr.To[int64](1),
				WorkerMachineCount:       ptr.To[int64](workerMachineCount),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		workloadProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, namespace.Name, clusterResources.Cluster.Name)

		var err error

		numberOfNodes := int(workerMachineCount)
		if s, ok := os.LookupEnv(kubetestNumberOfNodesVariable); ok && s != "" {
			numberOfNodes, err = strconv.Atoi(s)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse kubetestNumberOfNodesVariable to int")
		}

		ginkgoNodes := int(workerMachineCount)
		if s, ok := os.LookupEnv(kubetestGinkgoNodesVariable); ok && s != "" {
			ginkgoNodes, err = strconv.Atoi(s)
			Expect(err).ToNot(HaveOccurred(), "Failed to parse kubetestGinkgoNodesVariable to int")
		}

		// Start running conformance test suites.
		err = kubetest.Run(
			ctx,
			kubetest.RunInput{
				ClusterProxy:       workloadProxy,
				NumberOfNodes:      numberOfNodes,
				ArtifactsDirectory: input.ArtifactFolder,
				ConfigFilePath:     kubetestConfigFilePath,
				GinkgoNodes:        ginkgoNodes,
				ClusterName:        clusterResources.Cluster.GetName(),
			},
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to run Kubernetes conformance")

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		framework.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
