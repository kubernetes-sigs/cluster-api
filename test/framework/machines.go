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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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

	Expect(input.GetLister.List(ctx, machines, byClusterOptions(input.Cluster.Name, input.Cluster.Namespace)...)).To(Succeed(), "Failed to get Cluster machines %s/%s", input.Cluster.Namespace, input.Cluster.Name)
	Eventually(func() (count int, err error) {
		for _, m := range machines.Items {
			machine := &clusterv1.Machine{}
			err = input.GetLister.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, machine)
			if err != nil {
				return
			}
			if machine.Status.NodeRef != nil {
				count++
			}
		}
		return
	}, intervals...).Should(Equal(len(machines.Items)))
}

type WaitForClusterMachinesReadyInput struct {
	GetLister  GetLister
	NodeGetter Getter
	Cluster    *clusterv1.Cluster
}

func WaitForClusterMachinesReady(ctx context.Context, input WaitForClusterMachinesReadyInput, intervals ...interface{}) {
	By("Waiting for the machines' nodes to be ready")
	machines := &clusterv1.MachineList{}

	Expect(input.GetLister.List(ctx, machines, byClusterOptions(input.Cluster.Name, input.Cluster.Namespace)...)).To(Succeed(), "Failed to get Cluster machines %s/%s", input.Cluster.Namespace, input.Cluster.Name)
	Eventually(func() (count int, err error) {
		for _, m := range machines.Items {
			machine := &clusterv1.Machine{}
			err = input.GetLister.Get(ctx, client.ObjectKey{Namespace: m.Namespace, Name: m.Name}, machine)
			if err != nil {
				return
			}
			if machine.Status.NodeRef == nil {
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
	}, intervals...).Should(Equal(len(machines.Items)))
}
