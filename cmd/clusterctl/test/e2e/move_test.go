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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kind/pkg/cluster"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
)

var _ = Describe("clusterctl move", func() {
	var (
		fromMgmtInfo testMgmtClusterInfo
		toMgmtInfo   testMgmtClusterInfo
		workloadInfo testWorkloadClusterInfo
	)

	BeforeEach(func() {
		var err error
		// "from" mgmt cluster info object
		fromMgmtInfo = testMgmtClusterInfo{
			clusterctlConfigFile:    clusterctlConfigFile,
			coreProvider:            "cluster-api:v0.3.0",
			bootstrapProviders:      []string{"kubeadm-bootstrap:v0.3.0"},
			controlPlaneProviders:   []string{"kubeadm-control-plane:v0.3.0"},
			infrastructureProviders: []string{"docker:v0.3.0"},
		}
		// Create the "from" mgmt cluster and client
		fromMgmtInfo.mgmtCluster, err = CreateKindCluster(kindConfigFile)
		Expect(err).ToNot(HaveOccurred())
		fromMgmtInfo.mgmtClient, err = fromMgmtInfo.mgmtCluster.GetClient()
		Expect(err).NotTo(HaveOccurred())
		initTestMgmtCluster(ctx, fromMgmtInfo)

		// Create workload cluster on "from" mgmt cluster
		workloadInfo = testWorkloadClusterInfo{
			workloadClusterName:      "e2e-workload-cluster",
			kubernetesVersion:        "1.14.2",
			controlPlaneMachineCount: 1,
			workerMachineCount:       0,
		}
		createTestWorkloadCluster(ctx, fromMgmtInfo, workloadInfo)

		// "to" mgmt cluster info object
		toMgmtInfo = testMgmtClusterInfo{
			clusterctlConfigFile:    clusterctlConfigFile,
			coreProvider:            "cluster-api:v0.3.0",
			bootstrapProviders:      []string{"kubeadm-bootstrap:v0.3.0"},
			controlPlaneProviders:   []string{"kubeadm-control-plane:v0.3.0"},
			infrastructureProviders: []string{"docker:v0.3.0"},
		}
		// Create the "to" mgmt cluster and client
		toMgmtInfo.mgmtCluster, err = CreateKindCluster(kindConfigFile)
		Expect(err).ToNot(HaveOccurred())
		toMgmtInfo.mgmtClient, err = toMgmtInfo.mgmtCluster.GetClient()
		Expect(err).NotTo(HaveOccurred())
		initTestMgmtCluster(ctx, toMgmtInfo)

		// Do the move
		c, err := clusterctlclient.New(fromMgmtInfo.clusterctlConfigFile)
		Expect(err).ToNot(HaveOccurred())
		err = c.Move(clusterctlclient.MoveOptions{
			FromKubeconfig: fromMgmtInfo.mgmtCluster.KubeconfigPath,
			ToKubeconfig:   toMgmtInfo.mgmtCluster.KubeconfigPath,
		})
		Expect(err).ToNot(HaveOccurred())

	}, setupTimeout)

	AfterEach(func() {
		fmt.Fprintf(GinkgoWriter, "Tearing down kind clusters\n")
		fromMgmtInfo.mgmtCluster.Teardown(ctx)
		toMgmtInfo.mgmtCluster.Teardown(ctx)
		fmt.Fprintf(GinkgoWriter, "Tearing down kind workload cluster\n")
		if err := cluster.NewProvider().Delete(workloadInfo.workloadClusterName, ""); err != nil {
			// Treat this as a non critical error
			fmt.Fprintf(GinkgoWriter, "Deleting the kind cluster %q failed. You may need to remove this by hand.\n", workloadInfo.workloadClusterName)
		}
	})

	Context("single node workerload cluster", func() {
		It("should move all Cluster API objects to the new mgmt cluster, unpause the Cluster and delete all objects from previous mgmt cluster", func() {
			Eventually(
				func() error {
					if err := toMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: workloadInfo.workloadClusterName}, &clusterv1.Cluster{}); err != nil {
						return err
					}
					if err := toMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "controlplane-0"}, &clusterv1.Machine{}); err != nil {
						return err
					}
					if err := toMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "controlplane-0-config"}, &bootstrapv1.KubeadmConfig{}); err != nil {
						return err
					}
					if err := toMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: workloadInfo.workloadClusterName}, &infrav1.DockerCluster{}); err != nil {
						return err
					}
					if err := toMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "controlplane-0"}, &infrav1.DockerMachine{}); err != nil {
						return err
					}
					return nil
				}, 3*time.Minute, 5*time.Second,
			).ShouldNot(HaveOccurred())
			// Should unpause Cluster object in the new mgmt cluster.
			Eventually(
				func() (bool, error) {
					testCluster := &clusterv1.Cluster{}
					if err := toMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: workloadInfo.workloadClusterName}, testCluster); err != nil {
						return false, err
					}
					if testCluster.Spec.Paused {
						return false, nil
					}
					return true, nil
				}, 3*time.Minute, 5*time.Second,
			).Should(BeTrue())
			// Should delete all Cluster API objects from the previous management cluster.
			Eventually(
				func() bool {
					if !apierrors.IsNotFound(fromMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: workloadInfo.workloadClusterName}, &clusterv1.Cluster{})) {
						return false
					}
					if !apierrors.IsNotFound(fromMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "controlplane-0"}, &clusterv1.Machine{})) {
						return false
					}
					if !apierrors.IsNotFound(fromMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "controlplane-0-config"}, &bootstrapv1.KubeadmConfig{})) {
						return false
					}
					if !apierrors.IsNotFound(fromMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: workloadInfo.workloadClusterName}, &infrav1.DockerCluster{})) {
						return false
					}
					if !apierrors.IsNotFound(fromMgmtInfo.mgmtClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "controlplane-0"}, &infrav1.DockerMachine{})) {
						return false
					}
					return true
				}, 3*time.Minute, 5*time.Second,
			).Should(BeTrue())
		})
	})
})
