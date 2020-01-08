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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	cabpkv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// eventuallyInterval is the polling interval used by gomega.Eventually
	eventuallyInterval = 10 * time.Second
)

// ControlplaneClusterInput defines the necessary dependencies to run a multi-node control plane cluster.
type ControlplaneClusterInput struct {
	Management        ManagementCluster
	Cluster           *clusterv1.Cluster
	InfraCluster      runtime.Object
	Nodes             []Node
	MachineDeployment MachineDeployment
	RelatedResources  []runtime.Object
	CreateTimeout     time.Duration
	DeleteTimeout     time.Duration
}

// SetDefaults defaults the struct fields if necessary.
func (input *ControlplaneClusterInput) SetDefaults() {
	if input.CreateTimeout == 0 {
		input.CreateTimeout = 10 * time.Minute
	}

	if input.DeleteTimeout == 0 {
		input.DeleteTimeout = 5 * time.Minute
	}
}

// ControlPlaneCluster creates an n node control plane cluster.
// Assertions:
//  * The number of nodes in the created cluster will equal the number
//    of control plane nodes plus the number of replicas in the machine
//    deployment.
func (input *ControlplaneClusterInput) ControlPlaneCluster() {
	ctx := context.Background()
	Expect(input.Management).ToNot(BeNil())

	mgmtClient, err := input.Management.GetClient()
	Expect(err).NotTo(HaveOccurred(), "stack: %+v", err)

	By("creating an InfrastructureCluster resource")
	Expect(mgmtClient.Create(ctx, input.InfraCluster)).To(Succeed())

	// This call happens in an eventually because of a race condition with the
	// webhook server. If the latter isn't fully online then this call will
	// fail.
	By("creating a Cluster resource linked to the InfrastructureCluster resource")
	Eventually(func() error {
		if err := mgmtClient.Create(ctx, input.Cluster); err != nil {
			fmt.Printf("%+v\n", err)
			return err
		}
		return nil
	}, input.CreateTimeout, eventuallyInterval).Should(BeNil())

	By("creating related resources")
	for _, obj := range input.RelatedResources {
		By(fmt.Sprintf("creating a/an %s resource", obj.GetObjectKind().GroupVersionKind()))
		Eventually(func() error {
			return mgmtClient.Create(ctx, obj)
		}, input.CreateTimeout, eventuallyInterval).Should(BeNil())
	}

	// expectedNumberOfNodes is the number of nodes that should be deployed to
	// the cluster. This is the control plane nodes plus the number of replicas
	// defined for a possible MachineDeployment.
	expectedNumberOfNodes := len(input.Nodes)

	// Create the additional control plane nodes.
	for i, node := range input.Nodes {
		expectedNumberOfNodes++

		By(fmt.Sprintf("creating %d control plane node's InfrastructureMachine resource", i+1))
		Expect(mgmtClient.Create(ctx, node.InfraMachine)).To(Succeed())

		By(fmt.Sprintf("creating %d control plane node's BootstrapConfig resource", i+1))
		Expect(mgmtClient.Create(ctx, node.BootstrapConfig)).To(Succeed())

		By(fmt.Sprintf("creating %d control plane node's Machine resource with a linked InfrastructureMachine and BootstrapConfig", i+1))
		Expect(mgmtClient.Create(ctx, node.Machine)).To(Succeed())
	}

	By("waiting for cluster to enter the provisioned phase")
	Eventually(func() (string, error) {
		cluster := &clusterv1.Cluster{}
		key := client.ObjectKey{
			Namespace: input.Cluster.GetNamespace(),
			Name:      input.Cluster.GetName(),
		}
		if err := mgmtClient.Get(ctx, key, cluster); err != nil {
			return "", err
		}
		return cluster.Status.Phase, nil
	}, input.CreateTimeout, eventuallyInterval).Should(Equal(string(clusterv1.ClusterPhaseProvisioned)))

	// Create the machine deployment if the replica count >0.
	if machineDeployment := input.MachineDeployment.MachineDeployment; machineDeployment != nil {
		if replicas := machineDeployment.Spec.Replicas; replicas != nil && *replicas > 0 {
			expectedNumberOfNodes += int(*replicas)

			By("creating a core MachineDeployment resource")
			Expect(mgmtClient.Create(ctx, machineDeployment)).To(Succeed())

			By("creating a BootstrapConfigTemplate resource")
			Expect(mgmtClient.Create(ctx, input.MachineDeployment.BootstrapConfigTemplate)).To(Succeed())

			By("creating an InfrastructureMachineTemplate resource")
			Expect(mgmtClient.Create(ctx, input.MachineDeployment.InfraMachineTemplate)).To(Succeed())
		}
	}

	By("waiting for the workload nodes to exist")
	Eventually(func() ([]v1.Node, error) {
		workloadClient, err := input.Management.GetWorkloadClient(ctx, input.Cluster.Namespace, input.Cluster.Name)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get workload client")
		}
		nodeList := v1.NodeList{}
		if err := workloadClient.List(ctx, &nodeList); err != nil {
			return nil, err
		}
		return nodeList.Items, nil
	}, input.CreateTimeout, 10*time.Second).Should(HaveLen(expectedNumberOfNodes))

	By("waiting for all machines to be running")
	inClustersNamespaceListOption := client.InNamespace(input.Cluster.Namespace)
	matchClusterListOption := client.MatchingLabels{clusterv1.ClusterLabelName: input.Cluster.Name}
	Eventually(func() (bool, error) {
		// Get a list of all the Machine resources that belong to the Cluster.
		machineList := &clusterv1.MachineList{}
		if err := mgmtClient.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
			return false, err
		}
		if len(machineList.Items) != expectedNumberOfNodes {
			return false, errors.Errorf("number of Machines %d != expected number of nodes %d", len(machineList.Items), expectedNumberOfNodes)
		}
		for _, machine := range machineList.Items {
			if machine.Status.Phase != string(clusterv1.MachinePhaseRunning) {
				return false, errors.Errorf("machine %s is not running, it's %s", machine.Name, machine.Status.Phase)
			}
		}
		return true, nil
	}, input.CreateTimeout, eventuallyInterval).Should(BeTrue())
}

// CleanUpCoreArtifacts deletes the cluster and waits for everything to be gone.
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
	}, input.DeleteTimeout, eventuallyInterval).Should(HaveLen(0))

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
