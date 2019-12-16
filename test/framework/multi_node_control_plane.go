/*
Copyright 2019 The Kubernetes Authors.

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

package framework

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiNodeControlplaneClusterInput defines the necessary dependencies to run a multi-node control plane cluster.
type MultiNodeControlplaneClusterInput struct {
	Management        ManagementCluster
	Cluster           *clusterv1.Cluster
	InfraCluster      runtime.Object
	ControlplaneNodes []Node
	CreateTimeout     time.Duration
}

// SetDefaults defaults the struct fields if necessary.
func (m *MultiNodeControlplaneClusterInput) SetDefaults() {
	if m.CreateTimeout == 0 {
		m.CreateTimeout = 10 * time.Minute
	}
}

// MulitNodeControlPlaneCluster creates an n node control plane cluster.
// Assertions:
//  * The number of nodes in the created cluster will equal the number of nodes in the input data.
func MultiNodeControlPlaneCluster(input *MultiNodeControlplaneClusterInput) {
	ctx := context.Background()
	Expect(input.Management).ToNot(BeNil())

	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	By("creating an InfrastructureCluster resource")
	Expect(mgmtClient.Create(ctx, input.InfraCluster)).NotTo(HaveOccurred())

	By("creating a Cluster resource linked to the InfrastructureCluster resource")
	Eventually(func() error {
		err := mgmtClient.Create(ctx, input.Cluster)
		if err != nil {
			fmt.Println(err)
		}
		return err
	}, input.CreateTimeout, 10*time.Second).Should(BeNil())

	// create all the machines at once
	for _, node := range input.ControlplaneNodes {
		By("creating an InfrastructureMachine resource")
		Expect(mgmtClient.Create(ctx, node.InfraMachine)).NotTo(HaveOccurred())

		By("creating a bootstrap config")
		Expect(mgmtClient.Create(ctx, node.BootstrapConfig)).NotTo(HaveOccurred())

		By("creating a core Machine resource with a linked InfrastructureMachine and BootstrapConfig")
		Expect(mgmtClient.Create(ctx, node.Machine)).NotTo(HaveOccurred())
	}

	// Wait for the cluster infrastructure
	Eventually(func() string {
		cluster := &clusterv1.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		if err := mgmtClient.Get(ctx, key, cluster); err != nil {
			return err.Error()
		}
		return cluster.Status.Phase
	}, input.CreateTimeout, 10*time.Second).Should(Equal(string(clusterv1.ClusterPhaseProvisioned)))

	// wait for all the machines to be running
	By("waiting for all machines to be running")
	for _, node := range input.ControlplaneNodes {
		Eventually(func() string {
			machine := &clusterv1.Machine{}
			key := client.ObjectKey{
				Namespace: node.Machine.GetNamespace(),
				Name:      node.Machine.GetName(),
			}
			if err := mgmtClient.Get(ctx, key, machine); err != nil {
				return err.Error()
			}
			return machine.Status.Phase
		}, input.CreateTimeout, 10*time.Second).Should(Equal(string(clusterv1.MachinePhaseRunning)))
	}

	By("waiting for the workload nodes to exist")
	Eventually(func() []v1.Node {
		nodes := v1.NodeList{}
		By("ensuring the workload client can be generated")
		err := wait.PollImmediate(10*time.Second, 5*time.Minute, func() (bool, error) {
			_, err := input.Management.GetWorkloadClient(ctx, input.Cluster.Namespace, input.Cluster.Name)
			switch {
			case apierrors.IsNotFound(err):
				return false, nil
			case err != nil:
				return true, err
			default:
				return true, nil
			}
		})
		Expect(err).NotTo(HaveOccurred())
		By("getting the workload client and listing the nodes")
		workloadClient, err := input.Management.GetWorkloadClient(ctx, input.Cluster.Namespace, input.Cluster.Name)
		Expect(err).NotTo(HaveOccurred(), "Stack:\n%+v\n", err)
		Expect(workloadClient.List(ctx, &nodes)).NotTo(HaveOccurred())
		return nodes.Items
	}, input.CreateTimeout, 10*time.Second).Should(HaveLen(len(input.ControlplaneNodes)))
}
