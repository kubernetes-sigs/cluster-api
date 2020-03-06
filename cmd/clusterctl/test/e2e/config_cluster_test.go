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
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/kind/pkg/cluster"
)

var _ = Describe("clusterctl create cluster", func() {
	var (
		mgmtInfo     testMgmtClusterInfo
		workloadInfo testWorkloadClusterInfo
	)
	BeforeEach(func() {
		var err error
		// Mgmt cluster info object
		mgmtInfo = testMgmtClusterInfo{
			clusterctlConfigFile:    clusterctlConfigFile,
			coreProvider:            "cluster-api:v0.3.0",
			bootstrapProviders:      []string{"kubeadm-bootstrap:v0.3.0"},
			controlPlaneProviders:   []string{"kubeadm-control-plane:v0.3.0"},
			infrastructureProviders: []string{"docker:v0.3.0"},
		}
		// Create the mgmt cluster and client
		mgmtInfo.mgmtCluster, err = CreateKindCluster(kindConfigFile)
		Expect(err).ToNot(HaveOccurred())
		mgmtInfo.mgmtClient, err = mgmtInfo.mgmtCluster.GetClient()
		Expect(err).NotTo(HaveOccurred())

		initTestMgmtCluster(ctx, mgmtInfo)

		// Workload cluster info object
		workloadInfo = testWorkloadClusterInfo{
			workloadClusterName:      "e2e-workload-cluster",
			kubernetesVersion:        "1.14.2",
			controlPlaneMachineCount: 1,
			workerMachineCount:       0,
		}
		// Let's setup some varibles for the workload cluster template
		Expect(os.Setenv("DOCKER_SERVICE_CIDRS", "\"10.96.0.0/12\"")).To(Succeed())
		Expect(os.Setenv("DOCKER_POD_CIDRS", "\"192.168.0.0/16\"")).To(Succeed())
		createTestWorkloadCluster(ctx, mgmtInfo, workloadInfo)
	})

	AfterEach(func() {
		fmt.Fprintf(GinkgoWriter, "Tearing down kind mgmt cluster\n")
		mgmtInfo.mgmtCluster.Teardown(ctx)
		fmt.Fprintf(GinkgoWriter, "Tearing down kind workload cluster\n")
		if err := cluster.NewProvider().Delete(workloadInfo.workloadClusterName, ""); err != nil {
			// Treat this as a non critical error
			fmt.Fprintf(GinkgoWriter, "Deleting the kind cluster %q failed. You may need to remove this by hand.\n", workloadInfo.workloadClusterName)
		}
	})

	Context("using specific core, control-plane, bootstrap, and capd provider version", func() {
		It("should create a workload cluster", func() {
			Eventually(func() ([]v1.Node, error) {
				workloadClient, err := mgmtInfo.mgmtCluster.GetWorkloadClient(ctx, "default", workloadInfo.workloadClusterName)
				if err != nil {
					return nil, errors.Wrap(err, "failed to get workload client")
				}
				nodeList := v1.NodeList{}
				if err := workloadClient.List(ctx, &nodeList); err != nil {
					return nil, err
				}
				return nodeList.Items, nil
			}, 5*time.Minute, 10*time.Second).Should(HaveLen(2))

			// TODO: Check config file and env variables defined above.
		})
	})
})
