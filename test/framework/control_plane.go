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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ControlplaneClusterInput defines the necessary dependencies to run a multi-node control plane cluster.
type ControlplaneClusterInput struct {
	Management    ManagementCluster
	Cluster       *clusterv1.Cluster
	InfraCluster  runtime.Object
	Nodes         []Node
	CreateTimeout time.Duration
	DeleteTimeout time.Duration

	ControlPlane    *controlplanev1.KubeadmControlPlane
	MachineTemplate runtime.Object
}

// SetDefaults defaults the struct fields if necessary.
func (m *ControlplaneClusterInput) SetDefaults() {
	if m.CreateTimeout == 0 {
		m.CreateTimeout = 10 * time.Minute
	}

	if m.DeleteTimeout == 0 {
		m.DeleteTimeout = 5 * time.Minute
	}
}

// ControlPlaneCluster creates an n node control plane cluster.
// Assertions:
//  * The number of nodes in the created cluster will equal the number of nodes in the input data.
func (input *ControlplaneClusterInput) ControlPlaneCluster() {
	ctx := context.Background()
	Expect(input.Management).ToNot(BeNil())

	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	By("creating an InfrastructureCluster resource")
	Expect(mgmtClient.Create(ctx, input.InfraCluster)).To(Succeed())

	By("creating a Cluster resource linked to the InfrastructureCluster resource")
	Eventually(func() error {
		err := mgmtClient.Create(ctx, input.Cluster)
		if err != nil {
			fmt.Println(err)
		}
		return err
	}, input.CreateTimeout, 10*time.Second).Should(BeNil())

	By("creating the machine template")
	Expect(mgmtClient.Create(ctx, input.MachineTemplate)).To(Succeed())

	By("creating a KubeadmControlPlane")
	Eventually(func() error {
		err := mgmtClient.Create(ctx, input.ControlPlane)
		if err != nil {
			fmt.Println(err)
		}
		return err
	}, input.CreateTimeout, 10*time.Second).Should(BeNil())

	//// create all the machines at once
	//for _, node := range input.Nodes {
	//	By("creating an InfrastructureMachine resource")
	//	Expect(mgmtClient.Create(ctx, node.InfraMachine)).To(Succeed())
	//
	//	By("creating a bootstrap config")
	//	Expect(mgmtClient.Create(ctx, node.BootstrapConfig)).To(Succeed())
	//
	//	By("creating a core Machine resource with a linked InfrastructureMachine and BootstrapConfig")
	//	Expect(mgmtClient.Create(ctx, node.Machine)).To(Succeed())
	//}

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

	// wait for the controlplane to be ready
	By("waiting for the controlplane to be ready")
	Eventually(func() bool {
		controlplane := &controlplanev1.KubeadmControlPlane{}
		key := client.ObjectKey{
			Namespace: input.ControlPlane.GetNamespace(),
			Name:      input.ControlPlane.GetName(),
		}
		if err := mgmtClient.Get(ctx, key, controlplane); err != nil {
			fmt.Println(err.Error())
			return false
		}
		return controlplane.Status.Ready
	}, input.CreateTimeout, 10*time.Second).Should(BeTrue())

	//By("waiting for the workload nodes to exist")
	//Eventually(func() []v1.Node {
	//	nodes := v1.NodeList{}
	//	By("ensuring the workload client can be generated")
	//	err := wait.PollImmediate(10*time.Second, 5*time.Minute, func() (bool, error) {
	//		_, err := input.Management.GetWorkloadClient(ctx, input.Cluster.Namespace, input.Cluster.Name)
	//		switch {
	//		case apierrors.IsNotFound(err):
	//			return false, nil
	//		case err != nil:
	//			return true, err
	//		default:
	//			return true, nil
	//		}
	//	})
	//	Expect(err).NotTo(HaveOccurred())
	//	By("getting the workload client and listing the nodes")
	//	workloadClient, err := input.Management.GetWorkloadClient(ctx, input.Cluster.Namespace, input.Cluster.Name)
	//	Expect(err).NotTo(HaveOccurred(), "Stack:\n%+v\n", err)
	//	Expect(workloadClient.List(ctx, &nodes)).To(Succeed())
	//	return nodes.Items
	//}, input.CreateTimeout, 10*time.Second).Should(HaveLen(len(input.Nodes)))
}

// CleanUp deletes the cluster and waits for everything to be gone.
// Assertions:
//   * Deletes Machines
//   * Deletes MachineSets
//   * Deletes MachineDeployments
//   * Deletes KubeadmConfigs
//   * Deletes Secrets
func (input *ControlplaneClusterInput) CleanUpCoreArtifacts() {
	input.SetDefaults()
	ctx := context.Background()
	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	By(fmt.Sprintf("deleting cluster %s", input.Cluster.GetName()))
	Expect(mgmtClient.Delete(ctx, input.Cluster)).To(Succeed())

	Eventually(func() []clusterv1.Cluster {
		clusters := clusterv1.ClusterList{}
		Expect(mgmtClient.List(ctx, &clusters)).To(Succeed())
		return clusters.Items
	}, input.DeleteTimeout, 10*time.Second).Should(HaveLen(0))

	lbl, err := labels.Parse(fmt.Sprintf("%s=%s", clusterv1.ClusterLabelName, input.Cluster.GetClusterName()))
	Expect(err).ToNot(HaveOccurred())
	listOpts := &client.ListOptions{LabelSelector: lbl}

	By("ensuring all CAPI artifacts have been deleted")
	ensureArtifactsDeleted(ctx, mgmtClient, listOpts)
}

func ensureArtifactsDeleted(ctx context.Context, mgmtClient client.Client, opt *client.ListOptions) {
	// assertions
	ml := &clusterv1.MachineList{}
	Expect(mgmtClient.List(ctx, ml, opt)).To(Succeed())
	Expect(ml.Items).To(HaveLen(0))

	msl := &clusterv1.MachineSetList{}
	Expect(mgmtClient.List(ctx, msl, opt)).To(Succeed())
	Expect(msl.Items).To(HaveLen(0))

	mdl := &clusterv1.MachineDeploymentList{}
	Expect(mgmtClient.List(ctx, mdl, opt)).To(Succeed())
	Expect(mdl.Items).To(HaveLen(0))

	kcpl := &controlplanev1.KubeadmControlPlaneList{}
	Expect(mgmtClient.List(ctx, kcpl, opt)).To(Succeed())
	Expect(kcpl.Items).To(HaveLen(0))

	kcl := &cabpkv1.KubeadmConfigList{}
	Expect(mgmtClient.List(ctx, kcl, opt)).To(Succeed())
	Expect(kcl.Items).To(HaveLen(0))

	sl := &corev1.SecretList{}
	Expect(mgmtClient.List(ctx, sl, opt)).To(Succeed())
	Expect(sl.Items).To(HaveLen(0))
}
