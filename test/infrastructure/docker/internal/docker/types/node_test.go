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

package types

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/test/infrastructure/container"
)

func TestIP(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)

	node := &Node{
		Name: "TestNode",
	}

	ipv4, ipv6, err := node.IP(ctx)

	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(ipv4).To(Equal("TestNodeIPv4"))
	g.Expect(ipv6).To(Equal("TestNodeIPv6"))
}

func TestDeleteContainer(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)

	node := &Node{
		Name: "TestNode",
	}

	containerRuntime.ResetDeleteContainerCallLogs()
	err := node.Delete(ctx)

	g.Expect(err).ShouldNot(HaveOccurred())

	callLog := containerRuntime.DeleteContainerCalls()
	g.Expect(callLog).To(HaveLen(1))
	g.Expect(callLog[0]).To(Equal("TestNode"))
}

func TestKillContainer(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)

	node := &Node{
		Name: "TestNode",
	}

	containerRuntime.ResetKillContainerCallLogs()
	err := node.Kill(ctx, "TestSignal")

	g.Expect(err).ShouldNot(HaveOccurred())

	callLog := containerRuntime.KillContainerCalls()
	g.Expect(callLog).To(HaveLen(1))
	g.Expect(callLog[0].Container).To(Equal("TestNode"))
	g.Expect(callLog[0].Signal).To(Equal("TestSignal"))
}

func TestCommandRun(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)

	containerRuntime.ResetExecContainerCallLogs()
	cmd := GetContainerCmder("TestContainer").Command("test", "one", "two")
	err := cmd.Run(ctx)

	g.Expect(err).ShouldNot(HaveOccurred())

	callLog := containerRuntime.ExecContainerCalls()
	g.Expect(callLog).To(HaveLen(1))
	g.Expect(callLog[0].ContainerName).To(Equal("TestContainer"))
	g.Expect(callLog[0].Command).To(Equal("test"))
	g.Expect(callLog[0].Args).To(ContainElements([]string{"one", "two"}))
}

func TestWriteFile(t *testing.T) {
	g := NewWithT(t)
	containerRuntime := &container.FakeRuntime{}
	ctx := container.RuntimeInto(context.Background(), containerRuntime)

	containerRuntime.ResetExecContainerCallLogs()

	node := NewNode("TestContainer", "TestImage", "testing")
	err := node.WriteFile(ctx, "/tmp/test123", "testcontent")
	g.Expect(err).ShouldNot(HaveOccurred())

	callLog := containerRuntime.ExecContainerCalls()
	g.Expect(callLog).To(HaveLen(2))

	// First call is to make sure the output directory exists
	g.Expect(callLog[0].ContainerName).To(Equal("TestContainer"))
	g.Expect(callLog[0].Command).To(Equal("mkdir"))
	g.Expect(callLog[0].Args).To(ContainElements([]string{"-p", "/tmp"}))

	// Second call is to write out the contents to the file
	g.Expect(callLog[1].ContainerName).To(Equal("TestContainer"))
	g.Expect(callLog[1].Command).To(Equal("cp"))
	g.Expect(callLog[1].Args).To(ContainElements([]string{"/dev/stdin", "/tmp/test123"}))

	content := make([]byte, 12)
	count, err := callLog[1].Config.InputBuffer.Read(content)
	data := string(content[:count])
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(data).To(Equal("testcontent"))
}
