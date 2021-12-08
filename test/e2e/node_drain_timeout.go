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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
)

// NodeDrainTimeoutSpecInput is the input for NodeDrainTimeoutSpec.
type NodeDrainTimeoutSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool

	// Flavor, if specified, must refer to a template that contains
	// a KubeadmControlPlane resource with spec.machineTemplate.nodeDrainTimeout
	// configured and a MachineDeployment resource that has
	// spec.template.spec.nodeDrainTimeout configured.
	// If not specified, "node-drain" is used.
	Flavor *string
}

func NodeDrainTimeoutSpec(ctx context.Context, inputGetter func() NodeDrainTimeoutSpecInput) {
	var (
		specName           = "node-drain"
		input              NodeDrainTimeoutSpecInput
		namespace          *corev1.Namespace
		cancelWatches      context.CancelFunc
		clusterResources   *clusterctl.ApplyClusterTemplateAndWaitResult
		machineDeployments []*clusterv1.MachineDeployment
		controlplane       *controlplanev1.KubeadmControlPlane
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
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("A node should be forcefully removed if it cannot be drained in time", func() {
		By("Creating a workload cluster")
		controlPlaneReplicas := 3
		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   clusterctl.DefaultInfrastructureProvider,
				Flavor:                   pointer.StringDeref(input.Flavor, "node-drain"),
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: pointer.Int64Ptr(int64(controlPlaneReplicas)),
				WorkerMachineCount:       pointer.Int64Ptr(1),
			},
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)
		cluster := clusterResources.Cluster
		controlplane = clusterResources.ControlPlane
		machineDeployments = clusterResources.MachineDeployments
		Expect(machineDeployments[0].Spec.Replicas).To(Equal(pointer.Int32Ptr(1)))

		By("Add a deployment with unevictable pods and podDisruptionBudget to the workload cluster. The deployed pods cannot be evicted in the node draining process.")
		workloadClusterProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, cluster.Namespace, cluster.Name)
		framework.DeployUnevictablePod(ctx, framework.DeployUnevictablePodInput{
			WorkloadClusterProxy:               workloadClusterProxy,
			DeploymentName:                     fmt.Sprintf("%s-%s", "unevictable-pod", util.RandomString(3)),
			Namespace:                          namespace.Name + "-unevictable-workload",
			WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
		})

		By("Scale the machinedeployment down to zero. If we didn't have the NodeDrainTimeout duration, the node drain process would block this operator.")
		// Because all the machines of a machinedeployment can be deleted at the same time, so we only prepare the interval for 1 replica.
		nodeDrainTimeoutMachineDeploymentInterval := getDrainAndDeleteInterval(input.E2EConfig.GetIntervals(specName, "wait-machine-deleted"), machineDeployments[0].Spec.Template.Spec.NodeDrainTimeout, 1)
		for _, md := range machineDeployments {
			framework.ScaleAndWaitMachineDeployment(ctx, framework.ScaleAndWaitMachineDeploymentInput{
				ClusterProxy:              input.BootstrapClusterProxy,
				Cluster:                   cluster,
				MachineDeployment:         md,
				WaitForMachineDeployments: nodeDrainTimeoutMachineDeploymentInterval,
				Replicas:                  0,
			})
		}

		By("Deploy deployment with unevictable pods on control plane nodes.")
		framework.DeployUnevictablePod(ctx, framework.DeployUnevictablePodInput{
			WorkloadClusterProxy:               workloadClusterProxy,
			ControlPlane:                       controlplane,
			DeploymentName:                     fmt.Sprintf("%s-%s", "unevictable-pod", util.RandomString(3)),
			Namespace:                          namespace.Name + "-unevictable-workload",
			WaitForDeploymentAvailableInterval: input.E2EConfig.GetIntervals(specName, "wait-deployment-available"),
		})

		By("Scale down the controlplane of the workload cluster and make sure that nodes running workload can be deleted even the draining process is blocked.")
		// When we scale down the KCP, controlplane machines are by default deleted one by one, so it requires more time.
		nodeDrainTimeoutKCPInterval := getDrainAndDeleteInterval(input.E2EConfig.GetIntervals(specName, "wait-machine-deleted"), controlplane.Spec.MachineTemplate.NodeDrainTimeout, controlPlaneReplicas)
		framework.ScaleAndWaitControlPlane(ctx, framework.ScaleAndWaitControlPlaneInput{
			ClusterProxy:        input.BootstrapClusterProxy,
			Cluster:             cluster,
			ControlPlane:        controlplane,
			Replicas:            1,
			WaitForControlPlane: nodeDrainTimeoutKCPInterval,
		})

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

func getDrainAndDeleteInterval(deleteInterval []interface{}, drainTimeout *metav1.Duration, replicas int) []interface{} {
	deleteTimeout, err := time.ParseDuration(deleteInterval[0].(string))
	Expect(err).NotTo(HaveOccurred())
	// We add the drain timeout to the specified delete timeout per replica.
	intervalDuration := (drainTimeout.Duration + deleteTimeout) * time.Duration(replicas)
	res := []interface{}{intervalDuration.String(), deleteInterval[1]}
	return res
}
