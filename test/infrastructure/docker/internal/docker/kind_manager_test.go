/*
Copyright 2021 The Kubernetes Authors.

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

package docker

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster/constants"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/infrastructure/container"
)

func TestCreateNode(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)
	containerRuntime.ResetRunContainerCallLogs()

	portMappingsWithAPIServer := []v1alpha4.PortMapping{
		{
			ListenAddress: "0.0.0.0",
			HostPort:      32765,
			ContainerPort: KubeadmContainerPort,
			Protocol:      v1alpha4.PortMappingProtocolTCP,
		},
	}
	createOpts := &nodeCreateOpts{
		Name:         "TestName",
		Image:        "TestImage",
		ClusterName:  "TestClusterName",
		Role:         constants.ControlPlaneNodeRoleValue,
		PortMappings: portMappingsWithAPIServer,
		Mounts:       []v1alpha4.Mount{},
		IPFamily:     clusterv1.IPv4IPFamily,
	}
	_, err := createNode(ctx, createOpts)

	g.Expect(err).ShouldNot(HaveOccurred())

	callLog := containerRuntime.RunContainerCalls()
	g.Expect(callLog).To(HaveLen(1))
	g.Expect(callLog[0].Output).To(BeNil())

	runConfig := callLog[0].RunConfig
	g.Expect(runConfig).ToNot(BeNil())
	g.Expect(runConfig.Name).To(Equal("TestName"))
	g.Expect(runConfig.Image).To(Equal("TestImage"))
	g.Expect(runConfig.Network).To(Equal(DefaultNetwork))
	g.Expect(runConfig.Labels).To(HaveLen(2))
	g.Expect(runConfig.Labels["io.x-k8s.kind.cluster"]).To(Equal("TestClusterName"))
}

func TestCreateControlPlaneNode(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)
	containerRuntime.ResetRunContainerCallLogs()

	containerRuntime.ResetRunContainerCallLogs()
	m := Manager{}
	node, err := m.CreateControlPlaneNode(ctx, "TestName", "TestImage", "TestCluster", "100.100.100.100", 80, []v1alpha4.Mount{}, []v1alpha4.PortMapping{}, make(map[string]string), clusterv1.IPv4IPFamily)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(node.Role()).Should(Equal(constants.ControlPlaneNodeRoleValue))

	callLog := containerRuntime.RunContainerCalls()
	g.Expect(callLog).To(HaveLen(1))
	g.Expect(callLog[0].Output).To(BeNil())

	runConfig := callLog[0].RunConfig
	g.Expect(runConfig).ToNot(BeNil())
	g.Expect(runConfig.Labels).To(HaveLen(2))
	g.Expect(runConfig.Labels["io.x-k8s.kind.role"]).To(Equal(constants.ControlPlaneNodeRoleValue))
}

func TestCreateWorkerNode(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)
	containerRuntime.ResetRunContainerCallLogs()

	containerRuntime.ResetRunContainerCallLogs()
	m := Manager{}
	node, err := m.CreateWorkerNode(ctx, "TestName", "TestImage", "TestCluster", []v1alpha4.Mount{}, []v1alpha4.PortMapping{}, make(map[string]string), clusterv1.IPv4IPFamily)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(node.Role()).Should(Equal(constants.WorkerNodeRoleValue))

	callLog := containerRuntime.RunContainerCalls()
	g.Expect(callLog).To(HaveLen(1))
	g.Expect(callLog[0].Output).To(BeNil())

	runConfig := callLog[0].RunConfig
	g.Expect(runConfig).ToNot(BeNil())
	g.Expect(runConfig.Labels).To(HaveLen(2))
	g.Expect(runConfig.Labels["io.x-k8s.kind.role"]).To(Equal(constants.WorkerNodeRoleValue))
}

func TestCreateExternalLoadBalancerNode(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)
	containerRuntime.ResetRunContainerCallLogs()

	containerRuntime.ResetRunContainerCallLogs()
	m := Manager{}
	node, err := m.CreateExternalLoadBalancerNode(ctx, "TestName", "TestImage", "TestCluster", "100.100.100.100", 0, clusterv1.IPv4IPFamily)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(node.Role()).Should(Equal(constants.ExternalLoadBalancerNodeRoleValue))

	callLog := containerRuntime.RunContainerCalls()
	g.Expect(callLog).To(HaveLen(1))
	g.Expect(callLog[0].Output).To(BeNil())

	runConfig := callLog[0].RunConfig
	g.Expect(runConfig).ToNot(BeNil())
	g.Expect(runConfig.Labels).To(HaveLen(2))
	g.Expect(runConfig.Labels["io.x-k8s.kind.role"]).To(Equal(constants.ExternalLoadBalancerNodeRoleValue))
}
