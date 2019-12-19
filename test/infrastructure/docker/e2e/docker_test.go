// +build e2e

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

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"

	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
)

var _ = Describe("Docker", func() {
	Describe("Cluster Creation", func() {
		var (
			namespace  string
			clusterGen *ClusterGenerator
			nodeGen    *NodeGenerator
			input      *framework.ControlplaneClusterInput
		)

		BeforeEach(func() {
			namespace = "default"
			clusterGen = &ClusterGenerator{}
			nodeGen = &NodeGenerator{}
		})

		AfterEach(func() {
			ensureDockerArtifactsDeleted(input)
		})

		Context("One node cluster", func() {
			It("should create a single node cluster", func() {
				cluster, infraCluster := clusterGen.GenerateCluster(namespace)
				nodes := make([]framework.Node, 1)
				for i := range nodes {
					nodes[i] = nodeGen.GenerateNode(cluster.GetName())
				}
				input = &framework.ControlplaneClusterInput{
					Management:    mgmt,
					Cluster:       cluster,
					InfraCluster:  infraCluster,
					Nodes:         nodes,
					CreateTimeout: 3 * time.Minute,
				}
				input.ControlPlaneCluster()

				input.CleanUpCoreArtifacts()
			})
		})

		Context("Multi-node controlplane cluster", func() {
			It("should create a multi-node controlplane cluster", func() {
				cluster, infraCluster := clusterGen.GenerateCluster(namespace)
				nodes := make([]framework.Node, 3)
				for i := range nodes {
					nodes[i] = nodeGen.GenerateNode(cluster.Name)
				}

				input = &framework.ControlplaneClusterInput{
					Management:    mgmt,
					Cluster:       cluster,
					InfraCluster:  infraCluster,
					Nodes:         nodes,
					CreateTimeout: 5 * time.Minute,
				}
				input.ControlPlaneCluster()

				input.CleanUpCoreArtifacts()
			})
		})
	})
})

type ClusterGenerator struct {
	counter int
}

func (c *ClusterGenerator) GenerateCluster(namespace string) (*capiv1.Cluster, *infrav1.DockerCluster) {
	generatedName := fmt.Sprintf("test-%d", c.counter)
	c.counter++

	infraCluster := &infrav1.DockerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
	}

	cluster := &capiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: capiv1.ClusterSpec{
			ClusterNetwork: &capiv1.ClusterNetwork{
				Services: &capiv1.NetworkRanges{CIDRBlocks: []string{}},
				Pods:     &capiv1.NetworkRanges{CIDRBlocks: []string{"192.168.0.0/16"}},
			},
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       framework.TypeToKind(infraCluster),
				Namespace:  infraCluster.GetNamespace(),
				Name:       infraCluster.GetName(),
			},
		},
	}
	return cluster, infraCluster
}

type NodeGenerator struct {
	counter int
}

func (n *NodeGenerator) GenerateNode(clusterName string) framework.Node {
	namespace := "default"
	version := "v1.15.3"
	generatedName := fmt.Sprintf("controlplane-%d", n.counter)
	n.counter++
	infraMachine := &infrav1.DockerMachine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
	}

	bootstrapConfig := &bootstrapv1.KubeadmConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: bootstrapv1.KubeadmConfigSpec{
			ClusterConfiguration: &v1beta1.ClusterConfiguration{
				APIServer: v1beta1.APIServer{
					// Darwin support
					CertSANs: []string{"127.0.0.1"},
				},
			},
			InitConfiguration: &v1beta1.InitConfiguration{},
			JoinConfiguration: &v1beta1.JoinConfiguration{},
		},
	}

	machine := &capiv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
			Labels: map[string]string{
				capiv1.MachineControlPlaneLabelName: "",
				capiv1.ClusterLabelName:             clusterName,
			},
		},
		Spec: capiv1.MachineSpec{
			Bootstrap: capiv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       framework.TypeToKind(bootstrapConfig),
					Namespace:  bootstrapConfig.GetNamespace(),
					Name:       bootstrapConfig.GetName(),
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       framework.TypeToKind(infraMachine),
				Namespace:  infraMachine.GetNamespace(),
				Name:       infraMachine.GetName(),
			},
			Version:     &version,
			ClusterName: clusterName,
		},
	}
	return framework.Node{
		Machine:         machine,
		InfraMachine:    infraMachine,
		BootstrapConfig: bootstrapConfig,
	}
}
