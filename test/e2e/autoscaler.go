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
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

const AutoscalerWorkloadYAMLPath = "AUTOSCALER_WORKLOAD"

// AutoscalerSpecInput is the input for AutoscalerSpec.
type AutoscalerSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// Flavor, if specified must refer to a managed topology cluster template
	// which has exactly one MachineDeployment. The replicas should be nil on the MachineDeployment.
	// The MachineDeployment should have the autoscaler annotations set on it.
	// If not specified, it defaults to "topology-autoscaler".
	Flavor *string
	// InfrastructureProviders specifies the infrastructure to use for clusterctl
	// operations (Example: get cluster templates).
	// Note: In most cases this need not be specified. It only needs to be specified when
	// multiple infrastructure providers (ex: CAPD + Kubemark) are installed on the cluster as clusterctl will not be
	// able to identify the default.
	InfrastructureProvider *string
	// InfrastructureMachineTemplateKind should be the plural form of the InfraMachineTemplate kind.
	// It should be specified in lower case.
	// Example: dockermachinetemplates.
	InfrastructureMachineTemplateKind string
	AutoscalerVersion                 string
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
		Expect(input.E2EConfig.Variables).To(HaveKey(AutoscalerWorkloadYAMLPath), "%s needs to be defined when calling %s", AutoscalerWorkloadYAMLPath, specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.InfrastructureMachineTemplateKind).NotTo(Equal(""), "Invalid argument. input.InfrastructureMachineTemplateKind cannot be empty when calling %s spec", specName)
		Expect(input.AutoscalerVersion).ToNot(BeNil(), "Invalid argument. input.AutoscalerVersion can't be empty when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a workload cluster", func() {
		By("Creating a workload cluster")

		flavor := "topology-autoscaler"
		if input.Flavor != nil {
			flavor = *input.Flavor
		}

		infrastructureProvider := clusterctl.DefaultInfrastructureProvider
		if input.InfrastructureProvider != nil {
			infrastructureProvider = *input.InfrastructureProvider
		}

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   flavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: pointer.Int64(1),
				WorkerMachineCount:       pointer.Int64(0),
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		Expect(clusterResources.Cluster.Spec.Topology).NotTo(BeNil(), "Autoscaler test expected a Classy Cluster")
		// Ensure the MachineDeploymentTopology has the autoscaler annotations.
		mdTopology := clusterResources.Cluster.Spec.Topology.Workers.MachineDeployments[0]
		Expect(mdTopology.Metadata.Annotations).NotTo(BeNil(), "MachineDeployment is expected to have autoscaler annotations")
		nodeGroupMinSize, ok := mdTopology.Metadata.Annotations[clusterv1.AutoscalerMinSizeAnnotation]
		Expect(ok).To(BeTrue(), "MachineDeploymentTopology %s does not have the %q autoscaler annotation", mdTopology.Name, clusterv1.AutoscalerMinSizeAnnotation)
		nodeGroupMaxSize, ok := mdTopology.Metadata.Annotations[clusterv1.AutoscalerMaxSizeAnnotation]
		Expect(ok).To(BeTrue(), "MachineDeploymentTopology %s does not have the %q autoscaler annotation", mdTopology.Name, clusterv1.AutoscalerMaxSizeAnnotation)

		// Get a ClusterProxy so we can interact with the workload cluster
		workloadClusterProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, clusterResources.Cluster.Namespace, clusterResources.Cluster.Name)
		originalReplicas := *clusterResources.MachineDeployments[0].Spec.Replicas
		Expect(strconv.Itoa(int(originalReplicas))).To(Equal(nodeGroupMinSize), "MachineDeployment should have replicas as defined in %s", clusterv1.AutoscalerMinSizeAnnotation)

		By("Installing the autoscaler on the workload cluster")
		autoscalerWorkloadYAMLPath := input.E2EConfig.GetVariable(AutoscalerWorkloadYAMLPath)
		framework.ApplyAutoscalerToWorkloadCluster(ctx, framework.ApplyAutoscalerToWorkloadClusterInput{
			ArtifactFolder:                    input.ArtifactFolder,
			InfrastructureMachineTemplateKind: input.InfrastructureMachineTemplateKind,
			WorkloadYamlPath:                  autoscalerWorkloadYAMLPath,
			ManagementClusterProxy:            input.BootstrapClusterProxy,
			WorkloadClusterProxy:              workloadClusterProxy,
			Cluster:                           clusterResources.Cluster,
			AutoscalerVersion:                 input.AutoscalerVersion,
		}, input.E2EConfig.GetIntervals(specName, "wait-controllers")...)

		By("Creating workload that forces the system to scale up")
		framework.AddScaleUpDeploymentAndWait(ctx, framework.AddScaleUpDeploymentAndWaitInput{
			ClusterProxy: workloadClusterProxy,
		}, input.E2EConfig.GetIntervals(specName, "wait-autoscaler")...)

		By("Checking the MachineDeployment is scaled up")
		scaledUpReplicas := originalReplicas + 1
		framework.AssertMachineDeploymentReplicas(ctx, framework.AssertMachineDeploymentReplicasInput{
			Getter:                   input.BootstrapClusterProxy.GetClient(),
			MachineDeployment:        clusterResources.MachineDeployments[0],
			Replicas:                 scaledUpReplicas,
			WaitForMachineDeployment: input.E2EConfig.GetIntervals(specName, "wait-autoscaler"),
		})

		By("Disabling the autoscaler")
		framework.DisableAutoscalerForMachineDeploymentTopologyAndWait(ctx, framework.DisableAutoscalerForMachineDeploymentTopologyAndWaitInput{
			ClusterProxy:                  input.BootstrapClusterProxy,
			Cluster:                       clusterResources.Cluster,
			WaitForAnnotationsToBeDropped: input.E2EConfig.GetIntervals(specName, "wait-controllers"),
		})

		By("Checking we can manually scale up the MachineDeployment")
		// Scale up the MachineDeployment. Since autoscaler is disabled we should be able to do this.
		excessReplicas := scaledUpReplicas + 1
		framework.ScaleAndWaitMachineDeploymentTopology(ctx, framework.ScaleAndWaitMachineDeploymentTopologyInput{
			ClusterProxy:              input.BootstrapClusterProxy,
			Cluster:                   clusterResources.Cluster,
			Replicas:                  excessReplicas,
			WaitForMachineDeployments: input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		})

		By("Checking enabling autoscaler will scale down the MachineDeployment to correct size")
		// Enable autoscaler on the MachineDeployment.
		framework.EnableAutoscalerForMachineDeploymentTopologyAndWait(ctx, framework.EnableAutoscalerForMachineDeploymentTopologyAndWaitInput{
			ClusterProxy:                input.BootstrapClusterProxy,
			Cluster:                     clusterResources.Cluster,
			NodeGroupMinSize:            nodeGroupMinSize,
			NodeGroupMaxSize:            nodeGroupMaxSize,
			WaitForAnnotationsToBeAdded: input.E2EConfig.GetIntervals(specName, "wait-autoscaler"),
		})

		By("Checking the MachineDeployment is scaled down")
		// Since we scaled up the MachineDeployment manually and the workload has not changed auto scaler
		// should detect that there are unneeded nodes and scale down the MachineDeployment.
		framework.AssertMachineDeploymentReplicas(ctx, framework.AssertMachineDeploymentReplicasInput{
			Getter:                   input.BootstrapClusterProxy.GetClient(),
			MachineDeployment:        clusterResources.MachineDeployments[0],
			Replicas:                 scaledUpReplicas,
			WaitForMachineDeployment: input.E2EConfig.GetIntervals(specName, "wait-controllers"),
		})

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}
