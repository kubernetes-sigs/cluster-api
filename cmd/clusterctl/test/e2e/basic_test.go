// +build e2e

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
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/util"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/test/framework/discovery"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("When using clusterctl", func() {

	var (
		suiteArtifactFolder string
		logPath             string
		namespace           string
		clusterName         string
		client              ctrlclient.Client

		cluster *clusterv1.Cluster
	)

	BeforeEach(func() {
		suiteArtifactFolder = filepath.Join(artifactFolder, "basic")
		logPath = filepath.Join(suiteArtifactFolder, "logs")
		namespace = "default" //TODO(fabriziopandini): read from kubeconfig
		clusterName = fmt.Sprintf("test-cluster-%s", util.RandomString(6))

		By("Getting a client for the management cluster")
		var err error
		client, err = managementCluster.GetClient()
		Expect(err).NotTo(HaveOccurred(), "Failed to get a client for the management cluster")
	})

	AfterEach(func() {
		//TODO(fabriziopandini): dump resources

		By("Deleting the Cluster object")

		framework.DeleteCluster(ctx, framework.DeleteClusterInput{
			Deleter: client,
			Cluster: cluster,
		})

		By("Waiting for the Cluster object to be deleted")

		framework.WaitForClusterDeleted(ctx, framework.WaitForClusterDeletedInput{
			Getter:  client,
			Cluster: cluster,
		}, e2eConfig.IntervalsOrDefault("basic/wait-delete-cluster", "3m", "10s")...)

		By("Check for all the cluster api resources being deleted")

		framework.AssertAllClusterAPIResourcesAreGone(ctx, framework.AssertAllClusterAPIResourcesAreGoneInput{
			Lister:  client,
			Cluster: cluster,
		})

		//TODO(fabriziopandini): ensure all provider resources are deleted
	})

	It("Should create a workload cluster", func() {
		By("Running clusterctl config cluster to get a cluster template")

		Expect(e2eConfig.Variables).To(HaveKey("KUBERNETES_VERSION"), "Invalid configuration. Please add KUBERNETES_VERSION to the e2e.config file")
		Expect(e2eConfig.Variables).To(HaveKey("CONTROL_PLANE_MACHINE_COUNT"), "Invalid configuration. Please add CONTROL_PLANE_MACHINE_COUNT to the e2e.config file")
		controlPlaneMachineCount, err := strconv.Atoi(e2eConfig.Variables["CONTROL_PLANE_MACHINE_COUNT"])
		Expect(err).ToNot(HaveOccurred(), "Invalid configuration. The CONTROL_PLANE_MACHINE_COUNT variable should be a valid integer number")
		Expect(e2eConfig.Variables).To(HaveKey("WORKER_MACHINE_COUNT"), "Invalid configuration. Please add WORKER_MACHINE_COUNT to the e2e.config file")
		workerMachineCount, err := strconv.Atoi(e2eConfig.Variables["WORKER_MACHINE_COUNT"])
		Expect(err).ToNot(HaveOccurred(), "Invalid configuration. The WORKER_MACHINE_COUNT variable should be a valid integer number")

		workloadClusterTemplate := clusterctl.ConfigCluster(ctx, clusterctl.ConfigClusterInput{
			// pass reference to the management cluster hosting this test
			KubeconfigPath: managementClusterKubeConfigPath,
			// pass the clusterctl config file that points to the local provider repository created for this test,
			ClusterctlConfigPath: clusterctlConfigPath,
			// define the cluster template source
			InfrastructureProvider: e2eConfig.InfraProvider(),
			// define template variables
			ClusterName:              clusterName,
			KubernetesVersion:        e2eConfig.Variables["KUBERNETES_VERSION"],
			ControlPlaneMachineCount: controlPlaneMachineCount,
			WorkerMachineCount:       workerMachineCount,
			// setup output path for clusterctl logs
			LogPath: logPath,
		})
		Expect(workloadClusterTemplate).ToNot(BeNil(), "Failed to get the cluster template")

		By("Applying the cluster template to the cluster")

		managementCluster.Apply(ctx, workloadClusterTemplate)

		By("Waiting for the cluster infrastructure to be provisioned")

		cluster = discovery.GetClusterByName(ctx, discovery.GetClusterByNameInput{
			Getter:    client,
			Name:      clusterName,
			Namespace: namespace,
		})
		Expect(cluster).ToNot(BeNil(), "Failed to get the Cluster object")
		framework.WaitForClusterToProvision(ctx, framework.WaitForClusterToProvisionInput{
			Getter:  client,
			Cluster: cluster,
		}, e2eConfig.IntervalsOrDefault("basic/wait-provision-cluster", "3m", "10s")...)

		By("Waiting for the first control plane machine to be provisioned")

		controlPlane := discovery.GetKubeadmControlPlaneByCluster(ctx, discovery.GetKubeadmControlPlaneByClusterInput{
			Lister:      client,
			ClusterName: clusterName,
			Namespace:   namespace,
		})
		Expect(controlPlane).ToNot(BeNil())
		framework.WaitForOneKubeadmControlPlaneMachineToExist(ctx, framework.WaitForOneKubeadmControlPlaneMachineToExistInput{
			Lister:       client,
			Cluster:      cluster,
			ControlPlane: controlPlane,
		}, e2eConfig.IntervalsOrDefault("basic/wait-provision-first-control-plane-node", "3m", "10s")...)

		By("Installing Calico on the workload cluster")

		workloadClient, err := managementCluster.GetWorkloadClient(ctx, namespace, clusterName)
		Expect(err).ToNot(HaveOccurred(), "Failed to get the client for the workload cluster with name %s", cluster.Name)

		//TODO(fabriziopandini): read calico manifest from data folder, so we are removing an external variable that might affect the test
		applyYAMLURLInput := framework.ApplyYAMLURLInput{
			Client:        workloadClient,
			HTTPGetter:    http.DefaultClient,
			NetworkingURL: "https://docs.projectcalico.org/manifests/calico.yaml",
			Scheme:        scheme,
		}
		framework.ApplyYAMLURL(ctx, applyYAMLURLInput)

		By("Waiting for the remaining control plane machines to be provisioned (if any)")
		if controlPlane.Spec.Replicas != nil && int(*controlPlane.Spec.Replicas) > 1 {
			framework.WaitForKubeadmControlPlaneMachinesToExist(ctx, framework.WaitForKubeadmControlPlaneMachinesToExistInput{
				Lister:       client,
				Cluster:      cluster,
				ControlPlane: controlPlane,
			}, e2eConfig.IntervalsOrDefault("basic/wait-provision-remaining-control-plane-nodes", "3m", "10s")...)

			//TODO(fabriziopandini): apparently the next text does not pass on docker. verify

			// Wait for the control plane to be ready
			/*
					waitForControlPlaneToBeReadyInput := coreframework.WaitForControlPlaneToBeReadyInput{
					Getter:       client,
					ControlPlane: controlPlane,
				}
				coreframework.WaitForControlPlaneToBeReady(ctx, waitForControlPlaneToBeReadyInput)
			*/
		}

		By("Waiting for the worker machines to be provisioned")
		machineDeployments := discovery.GetMachineDeploymentsByCluster(ctx, discovery.GetMachineDeploymentsByClusterInput{
			Lister:      client,
			ClusterName: clusterName,
			Namespace:   "default",
		})
		for _, deployment := range machineDeployments {
			framework.WaitForMachineDeploymentNodesToExist(ctx, framework.WaitForMachineDeploymentNodesToExistInput{
				Lister:            client,
				Cluster:           cluster,
				MachineDeployment: deployment,
			}, e2eConfig.IntervalsOrDefault("basic/wait-provision-worker-nodes", "3m", "10s")...)
		}
	})
})
