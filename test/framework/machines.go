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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
)

// WaitForClusterMachineNodeRefsInput is the input for WaitForClusterMachineNodesRefs.
type WaitForClusterMachineNodeRefsInput struct {
	GetLister GetLister
	Cluster   *clusterv1.Cluster
}

// WaitForClusterMachineNodeRefs waits until all nodes associated with a machine deployment exist.
func WaitForClusterMachineNodeRefs(ctx context.Context, input WaitForClusterMachineNodeRefsInput, intervals ...interface{}) {
	By("Waiting for the machines' nodes to exist")
	machines := &clusterv1.MachineList{}

	Eventually(func() error {
		return input.GetLister.List(ctx, machines, byClusterOptions(input.Cluster.Name, input.Cluster.Namespace)...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get Cluster machines %s", klog.KObj(input.Cluster))
	Eventually(func() (count int, err error) {
		for _, m := range machines.Items {
			machine := &clusterv1.Machine{}
			err = input.GetLister.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, machine)
			if err != nil {
				return
			}
			if machine.Status.NodeRef.IsDefined() {
				count++
			}
		}
		return
	}, intervals...).Should(Equal(len(machines.Items)), "Timed out waiting for %d nodes to exist", len(machines.Items))
}

type WaitForClusterMachinesReadyInput struct {
	GetLister  GetLister
	NodeGetter Getter
	Cluster    *clusterv1.Cluster
}

func WaitForClusterMachinesReady(ctx context.Context, input WaitForClusterMachinesReadyInput, intervals ...interface{}) {
	By("Waiting for the machines' nodes to be ready")
	machines := &clusterv1.MachineList{}

	Eventually(func() error {
		return input.GetLister.List(ctx, machines, byClusterOptions(input.Cluster.Name, input.Cluster.Namespace)...)
	}, retryableOperationTimeout, retryableOperationInterval).Should(Succeed(), "Failed to get Cluster Machines %s", klog.KObj(input.Cluster))
	Eventually(func() (count int, err error) {
		for _, m := range machines.Items {
			machine := &clusterv1.Machine{}
			err = input.GetLister.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, machine)
			if err != nil {
				return
			}
			if !machine.Status.NodeRef.IsDefined() {
				continue
			}
			node := &corev1.Node{}
			err = input.NodeGetter.Get(ctx, client.ObjectKey{Name: machine.Status.NodeRef.Name}, node)
			if err != nil {
				return
			}
			if util.IsNodeReady(node) {
				count++
			}
		}
		return
	}, intervals...).Should(Equal(len(machines.Items)), "Timed out waiting for %d nodes to be ready", len(machines.Items))
}

type WaitForMachinesReadyInput struct {
	Lister    Lister
	Name      string
	Namespace string
	Intervals []interface{}
}

func WaitForMachinesReady(ctx context.Context, input WaitForMachinesReadyInput) {
	By("Waiting for the machines' Ready condition to be true")
	machineList := &clusterv1.MachineList{}

	// Wait for all machines to have Ready condition set to true.
	Eventually(func(g Gomega) {
		g.Expect(input.Lister.List(ctx, machineList, client.InNamespace(input.Namespace),
			client.MatchingLabels{
				clusterv1.ClusterNameLabel: input.Name,
			})).To(Succeed())

		g.Expect(machineList.Items).ToNot(BeEmpty(), "No machines found for cluster %s", input.Name)

		for _, machine := range machineList.Items {
			readyConditionFound := false
			for _, condition := range machine.Status.Conditions {
				if condition.Type == clusterv1.ReadyCondition {
					readyConditionFound = true
					g.Expect(condition.Status).To(Equal(metav1.ConditionTrue), "The Ready condition on Machine %q should be set to true; message: %s", machine.Name, condition.Message)
					g.Expect(condition.Message).To(BeEmpty(), "The Ready condition on Machine %q should have an empty message", machine.Name)
					break
				}
			}
			g.Expect(readyConditionFound).To(BeTrue(), "Machine %q should have a Ready condition", machine.Name)
		}
	}, input.Intervals...).Should(Succeed(), "Failed to wait for Machines Ready condition for Cluster %s", klog.KRef(input.Namespace, input.Name))
}
