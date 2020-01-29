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
	. "github.com/onsi/gomega"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Docker", func() {
	Describe("Cluster Creation", func() {
		var (
			namespace  string
			input      *framework.ControlplaneClusterInput
			clusterGen = &ClusterGenerator{}
		)

		BeforeEach(func() {
			namespace = "default"
		})

		AfterEach(func() {
			ensureDockerArtifactsDeleted(input)
		})

		Context("Multi-node controlplane cluster", func() {
			It("should create a multi-node controlplane cluster", func() {
				replicas := 3
				cluster, infraCluster, controlPlane, template := clusterGen.GenerateCluster(namespace, int32(replicas))
				// Set failure domains here
				infraCluster.Spec.FailureDomains = clusterv1.FailureDomains{
					"domain-one":   {ControlPlane: true},
					"domain-two":   {ControlPlane: true},
					"domain-three": {ControlPlane: true},
					"domain-four":  {ControlPlane: false},
				}

				md, infraTemplate, bootstrapTemplate := GenerateMachineDeployment(cluster, 1)
				input = &framework.ControlplaneClusterInput{
					Management:    mgmt,
					Cluster:       cluster,
					InfraCluster:  infraCluster,
					CreateTimeout: 5 * time.Minute,
					MachineDeployment: framework.MachineDeployment{
						MachineDeployment:       md,
						InfraMachineTemplate:    infraTemplate,
						BootstrapConfigTemplate: bootstrapTemplate,
					},
					ControlPlane:    controlPlane,
					MachineTemplate: template,
				}
				input.ControlPlaneCluster()

				// Custom expectations around Failure Domains
				By("waiting for all machines to be running")
				inClustersNamespaceListOption := client.InNamespace(cluster.GetNamespace())
				matchClusterListOption := client.MatchingLabels{
					clusterv1.ClusterLabelName:             cluster.GetName(),
					clusterv1.MachineControlPlaneLabelName: "",
				}

				machineList := &clusterv1.MachineList{}
				mclient, err := mgmt.GetClient()
				Expect(err).NotTo(HaveOccurred())
				Expect(mclient.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption)).To(Succeed())
				failureDomainCounts := map[string]int{}
				// Set all known failure domains
				for fd := range infraCluster.Spec.FailureDomains {
					failureDomainCounts[fd] = 0
				}
				for _, machine := range machineList.Items {
					if machine.Spec.FailureDomain == nil {
						continue
					}
					failureDomain := *machine.Spec.FailureDomain
					_, ok := failureDomainCounts[failureDomain]
					// Fail if a machine is placed in a failure domain not defined on the InfraCluster
					Expect(ok).To(BeTrue(), "failure domain assigned to machine is unknown to the cluster: %q\n %v", failureDomain, machineList.Items)
					failureDomainCounts[failureDomain]++
				}
				for id, spec := range infraCluster.Spec.FailureDomains {
					if spec.ControlPlane == false {
						continue
					}
					// This is a custom expectation bound to the fact that there are exactly 3 control planes
					Expect(failureDomainCounts[id]).To(Equal(1), "each failure domain should have exactly one control plane: %v", failureDomainCounts)
				}

				input.CleanUpCoreArtifacts()
			})
		})
	})
})

func GenerateMachineDeployment(cluster *clusterv1.Cluster, replicas int32) (*clusterv1.MachineDeployment, *infrav1.DockerMachineTemplate, *bootstrapv1.KubeadmConfigTemplate) {
	namespace := cluster.GetNamespace()
	generatedName := fmt.Sprintf("%s-md", cluster.GetName())
	version := "1.16.3"

	infraTemplate := &infrav1.DockerMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: infrav1.DockerMachineTemplateSpec{},
	}

	bootstrap := &bootstrapv1.KubeadmConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
	}

	template := clusterv1.MachineTemplateSpec{
		ObjectMeta: clusterv1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: cluster.GetName(),
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       framework.TypeToKind(bootstrap),
					Namespace:  bootstrap.GetNamespace(),
					Name:       bootstrap.GetName(),
				},
			},
			InfrastructureRef: corev1.ObjectReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       framework.TypeToKind(infraTemplate),
				Namespace:  infraTemplate.GetNamespace(),
				Name:       infraTemplate.GetName(),
			},
			Version: &version,
		},
	}

	machineDeployment := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ClusterName:             cluster.GetName(),
			Replicas:                &replicas,
			Template:                template,
			Strategy:                nil,
			MinReadySeconds:         nil,
			RevisionHistoryLimit:    nil,
			Paused:                  false,
			ProgressDeadlineSeconds: nil,
		},
	}
	return machineDeployment, infraTemplate, bootstrap
}

type ClusterGenerator struct {
	counter int
}

func (c *ClusterGenerator) GenerateCluster(namespace string, replicas int32) (*clusterv1.Cluster, *infrav1.DockerCluster, *controlplanev1.KubeadmControlPlane, *infrav1.DockerMachineTemplate) {
	generatedName := fmt.Sprintf("test-%d", c.counter)
	c.counter++
	version := "1.16.3"

	infraCluster := &infrav1.DockerCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
	}

	template := &infrav1.DockerMachineTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: infrav1.DockerMachineTemplateSpec{},
	}

	kcp := &controlplanev1.KubeadmControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: controlplanev1.KubeadmControlPlaneSpec{
			Replicas: &replicas,
			Version:  version,
			InfrastructureTemplate: corev1.ObjectReference{
				Kind:       framework.TypeToKind(template),
				Namespace:  template.GetNamespace(),
				Name:       template.GetName(),
				APIVersion: infrav1.GroupVersion.String(),
			},
			KubeadmConfigSpec: bootstrapv1.KubeadmConfigSpec{
				ClusterConfiguration: &v1beta1.ClusterConfiguration{
					APIServer: v1beta1.APIServer{
						// Darwin support
						CertSANs: []string{"127.0.0.1"},
					},
				},
				InitConfiguration: &v1beta1.InitConfiguration{},
				JoinConfiguration: &v1beta1.JoinConfiguration{},
			},
		},
	}

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      generatedName,
		},
		Spec: clusterv1.ClusterSpec{
			ClusterNetwork: &clusterv1.ClusterNetwork{
				Services: &clusterv1.NetworkRanges{CIDRBlocks: []string{}},
				Pods:     &clusterv1.NetworkRanges{CIDRBlocks: []string{"192.168.0.0/16"}},
			},
			InfrastructureRef: &corev1.ObjectReference{
				APIVersion: infrav1.GroupVersion.String(),
				Kind:       framework.TypeToKind(infraCluster),
				Namespace:  infraCluster.GetNamespace(),
				Name:       infraCluster.GetName(),
			},
			ControlPlaneRef: &corev1.ObjectReference{
				APIVersion: controlplanev1.GroupVersion.String(),
				Kind:       framework.TypeToKind(kcp),
				Namespace:  kcp.GetNamespace(),
				Name:       kcp.GetName(),
			},
		},
	}
	return cluster, infraCluster, kcp, template
}
