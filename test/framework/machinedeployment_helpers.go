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

package framework

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateMachineDeploymentInput is the input for CreateMachineDeployment.
type CreateMachineDeploymentInput struct {
	Creator                 Creator
	MachineDeployment       *clusterv1.MachineDeployment
	BootstrapConfigTemplate runtime.Object
	InfraMachineTemplate    runtime.Object
}

// CreateMachineDeployment creates the machine deployment and dependencies.
func CreateMachineDeployment(ctx context.Context, input CreateMachineDeploymentInput) {
	By("creating a core MachineDeployment resource")
	Expect(input.Creator.Create(ctx, input.MachineDeployment)).To(Succeed())

	By("creating a BootstrapConfigTemplate resource")
	Expect(input.Creator.Create(ctx, input.BootstrapConfigTemplate)).To(Succeed())

	By("creating an InfrastructureMachineTemplate resource")
	Expect(input.Creator.Create(ctx, input.InfraMachineTemplate)).To(Succeed())
}

// GetMachineDeploymentsByClusterInput is the input for GetMachineDeploymentsByCluster.
type GetMachineDeploymentsByClusterInput struct {
	Lister      Lister
	ClusterName string
	Namespace   string
}

// GetMachineDeploymentsByCluster returns the MachineDeployments objects for a cluster.
// Important! this method relies on labels that are created by the CAPI controllers during the first reconciliation, so
// it is necessary to ensure this is already happened before calling it.
func GetMachineDeploymentsByCluster(ctx context.Context, input GetMachineDeploymentsByClusterInput) []*clusterv1.MachineDeployment {
	deploymentList := &clusterv1.MachineDeploymentList{}
	Expect(input.Lister.List(ctx, deploymentList, byClusterOptions(input.ClusterName, input.Namespace)...)).To(Succeed(), "Failed to list MachineDeployments object for Cluster %s/%s", input.Namespace, input.ClusterName)

	deployments := make([]*clusterv1.MachineDeployment, len(deploymentList.Items))
	for i := range deploymentList.Items {
		deployments[i] = &deploymentList.Items[i]
	}
	return deployments
}

// WaitForMachineDeploymentNodesToExistInput is the input for WaitForMachineDeploymentNodesToExist.
type WaitForMachineDeploymentNodesToExistInput struct {
	Lister            Lister
	Cluster           *clusterv1.Cluster
	MachineDeployment *clusterv1.MachineDeployment
}

// WaitForMachineDeploymentNodesToExist waits until all nodes associated with a machine deployment exist.
func WaitForMachineDeploymentNodesToExist(ctx context.Context, input WaitForMachineDeploymentNodesToExistInput, intervals ...interface{}) {
	By("waiting for the workload nodes to exist")
	Eventually(func() (int, error) {
		selectorMap, err := metav1.LabelSelectorAsMap(&input.MachineDeployment.Spec.Selector)
		if err != nil {
			return 0, err
		}
		ms := &clusterv1.MachineSetList{}
		if err := input.Lister.List(ctx, ms, client.InNamespace(input.Cluster.Namespace), client.MatchingLabels(selectorMap)); err != nil {
			return 0, err
		}
		if len(ms.Items) == 0 {
			return 0, errors.New("no machinesets were found")
		}
		machineSet := ms.Items[0]
		selectorMap, err = metav1.LabelSelectorAsMap(&machineSet.Spec.Selector)
		if err != nil {
			return 0, err
		}
		machines := &clusterv1.MachineList{}
		if err := input.Lister.List(ctx, machines, client.InNamespace(machineSet.Namespace), client.MatchingLabels(selectorMap)); err != nil {
			return 0, err
		}
		count := 0
		for _, machine := range machines.Items {
			if machine.Status.NodeRef != nil {
				count++
			}
		}
		return count, nil
	}, intervals...).Should(Equal(int(*input.MachineDeployment.Spec.Replicas)))
}
