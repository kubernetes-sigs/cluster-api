/*
Copyright 2023 The Kubernetes Authors.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

// AutoscalerSpecInput is the input for AutoscalerSpec.
type AutoscalerSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// Flavor, if specified is the template flavor to be used for this test.
	// Note:
	//  - the file creating the service account to be used by the autoscaler when connecting to the management cluster
	//    - must be named "autoscaler-to-workload-management.yaml"
	//    - must deploy objects in the $CLUSTER_NAMESPACE
	//    - must create a service account with name "cluster-$CLUSTER_NAME" and the RBAC rules required to work.
	//    - must create a secret with name "cluster-$CLUSTER_NAME-token" and type "kubernetes.io/service-account-token".
	//  - the file creating the autoscaler deployment in the workload cluster
	//    - must be named "autoscaler-to-workload-workload.yaml"
	//    - must deploy objects in the cluster-autoscaler-system namespace
	//    - must create a deployment named "cluster-autoscaler"
	//    - must run the autoscaler with --cloud-provider=clusterapi,
	//      --node-group-auto-discovery=clusterapi:namespace=${CLUSTER_NAMESPACE},clusterName=${CLUSTER_NAME}
	//      and --cloud-config pointing to a kubeconfig to connect to the management cluster
	//      using the token above.
	//    - could use following vars to build the management cluster kubeconfig:
	//      $MANAGEMENT_CLUSTER_TOKEN, $MANAGEMENT_CLUSTER_CA, $MANAGEMENT_CLUSTER_ADDRESS
	Flavor                 *string
	InfrastructureProvider string
	AutoscalerVersion      string
}

// AutoscalerSpec implements a test for the autoscaler, and more specifically for the autoscaler
// being deployed in the workload cluster.
func AutoscalerSpec(ctx context.Context, inputGetter func() AutoscalerSpecInput) {
	var (
		specName         = "autoscaler"
		input            AutoscalerSpecInput
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
		Expect(input.InfrastructureProvider).ToNot(BeNil(), "Invalid argument. input.InfrastructureProvider can't be empty when calling %s spec", specName)
		Expect(input.AutoscalerVersion).ToNot(BeNil(), "Invalid argument. input.AutoscalerVersion can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a workload cluster", func() {
		By("Creating a workload cluster")

		flavor := clusterctl.DefaultFlavor
		if input.Flavor != nil {
			flavor = *input.Flavor
		}

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   input.InfrastructureProvider,
				Flavor:                   flavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: pointer.Int64(1),
				WorkerMachineCount:       pointer.Int64(1),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		// Get a ClusterProxy so we can interact with the workload cluster
		workloadClusterProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, clusterResources.Cluster.Namespace, clusterResources.Cluster.Name)

		By("Installing the autoscaler in the workload cluster")
		infrastructureProviderVersions := input.E2EConfig.GetProviderVersions(input.InfrastructureProvider)
		latestProviderVersion := infrastructureProviderVersions[len(infrastructureProviderVersions)-1]

		framework.ApplyAutoscalerToWorkloadCluster(ctx, framework.ApplyAutoscalerToWorkloadClusterInput{
			ArtifactFolder:         input.ArtifactFolder,
			InfrastructureProvider: input.InfrastructureProvider,
			LatestProviderVersion:  latestProviderVersion,
			ManagementClusterProxy: input.BootstrapClusterProxy,
			WorkloadClusterProxy:   workloadClusterProxy,
			Cluster:                clusterResources.Cluster,
			AutoscalerVersion:      input.AutoscalerVersion,
		}, input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), "wait-controllers")...)

		By("Creating workload that force the system to scale up")
		framework.AddScaleUpDeploymentAndWait(ctx, framework.AddScaleUpDeploymentAndWaitInput{
			ClusterProxy: workloadClusterProxy,
		}, input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), "wait-autoscaler")...)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
