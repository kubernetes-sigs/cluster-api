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
	"errors"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/types/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/test/framework"
	infrav1 "sigs.k8s.io/cluster-api/test/infrastructure/docker/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Docker Upgrade", func() {
	var (
		replicas     = 3
		namespace    = "default"
		clusterGen   = newClusterGenerator("upgrade")
		mgmtClient   ctrlclient.Client
		cluster      *clusterv1.Cluster
		controlPlane *controlplanev1.KubeadmControlPlane
	)
	SetDefaultEventuallyTimeout(15 * time.Minute)
	SetDefaultEventuallyPollingInterval(10 * time.Second)

	BeforeEach(func() {
		// Ensure multi-controlplane workload cluster is up and running
		var (
			infraCluster *infrav1.DockerCluster
			template     *infrav1.DockerMachineTemplate
			err          error
		)
		cluster, infraCluster, controlPlane, template = clusterGen.GenerateCluster(namespace, int32(replicas))
		md, infraTemplate, bootstrapTemplate := GenerateMachineDeployment(cluster, 1)

		// Set up the client to the management cluster
		mgmtClient, err = mgmt.GetClient()
		Expect(err).NotTo(HaveOccurred())

		// Set up the cluster object
		createClusterInput := framework.CreateClusterInput{
			Creator:      mgmtClient,
			Cluster:      cluster,
			InfraCluster: infraCluster,
		}
		framework.CreateCluster(ctx, createClusterInput)

		// Set up the KubeadmControlPlane
		createKubeadmControlPlaneInput := framework.CreateKubeadmControlPlaneInput{
			Creator:         mgmtClient,
			ControlPlane:    controlPlane,
			MachineTemplate: template,
		}
		framework.CreateKubeadmControlPlane(ctx, createKubeadmControlPlaneInput)

		// Wait for the cluster to provision.
		assertClusterProvisionsInput := framework.WaitForClusterToProvisionInput{
			Getter:  mgmtClient,
			Cluster: cluster,
		}
		framework.WaitForClusterToProvision(ctx, assertClusterProvisionsInput)

		// Wait for at least one control plane node to be ready
		waitForOneKubeadmControlPlaneMachineToExistInput := framework.WaitForOneKubeadmControlPlaneMachineToExistInput{
			Lister:       mgmtClient,
			Cluster:      cluster,
			ControlPlane: controlPlane,
		}
		framework.WaitForOneKubeadmControlPlaneMachineToExist(ctx, waitForOneKubeadmControlPlaneMachineToExistInput, "15m")

		// Insatll a networking solution on the workload cluster
		workloadClient, err := mgmt.GetWorkloadClient(ctx, cluster.Namespace, cluster.Name)
		Expect(err).ToNot(HaveOccurred())
		applyYAMLURLInput := framework.ApplyYAMLURLInput{
			Client:        workloadClient,
			HTTPGetter:    http.DefaultClient,
			NetworkingURL: "https://docs.projectcalico.org/manifests/calico.yaml",
			Scheme:        mgmt.Scheme,
		}
		framework.ApplyYAMLURL(ctx, applyYAMLURLInput)

		// Wait for the controlplane nodes to exist
		assertKubeadmControlPlaneNodesExistInput := framework.WaitForKubeadmControlPlaneMachinesToExistInput{
			Lister:       mgmtClient,
			Cluster:      cluster,
			ControlPlane: controlPlane,
		}
		framework.WaitForKubeadmControlPlaneMachinesToExist(ctx, assertKubeadmControlPlaneNodesExistInput, "15m", "10s")

		// Create the workload nodes
		createMachineDeploymentinput := framework.CreateMachineDeploymentInput{
			Creator:                 mgmtClient,
			MachineDeployment:       md,
			BootstrapConfigTemplate: bootstrapTemplate,
			InfraMachineTemplate:    infraTemplate,
		}
		framework.CreateMachineDeployment(ctx, createMachineDeploymentinput)

		// Wait for the workload nodes to exist
		waitForMachineDeploymentNodesToExistInput := framework.WaitForMachineDeploymentNodesToExistInput{
			Lister:            mgmtClient,
			Cluster:           cluster,
			MachineDeployment: md,
		}
		framework.WaitForMachineDeploymentNodesToExist(ctx, waitForMachineDeploymentNodesToExistInput)

		// Wait for the control plane to be ready
		waitForControlPlaneToBeReadyInput := framework.WaitForControlPlaneToBeReadyInput{
			Getter:       mgmtClient,
			ControlPlane: controlPlane,
		}
		framework.WaitForControlPlaneToBeReady(ctx, waitForControlPlaneToBeReadyInput)
	})

	AfterEach(func() {
		// Delete the workload cluster
		deleteClusterInput := framework.DeleteClusterInput{
			Deleter: mgmtClient,
			Cluster: cluster,
		}
		framework.DeleteCluster(ctx, deleteClusterInput)

		waitForClusterDeletedInput := framework.WaitForClusterDeletedInput{
			Getter:  mgmtClient,
			Cluster: cluster,
		}
		framework.WaitForClusterDeleted(ctx, waitForClusterDeletedInput)

		assertAllClusterAPIResourcesAreGoneInput := framework.AssertAllClusterAPIResourcesAreGoneInput{
			Lister:  mgmtClient,
			Cluster: cluster,
		}
		framework.AssertAllClusterAPIResourcesAreGone(ctx, assertAllClusterAPIResourcesAreGoneInput)

		ensureDockerDeletedInput := ensureDockerArtifactsDeletedInput{
			Lister:  mgmtClient,
			Cluster: cluster,
		}
		ensureDockerArtifactsDeleted(ensureDockerDeletedInput)

		// Dump cluster API and docker related resources to artifacts before deleting them.
		Expect(framework.DumpResources(mgmt, resourcesPath, GinkgoWriter)).To(Succeed())
		resources := map[string]runtime.Object{
			"DockerCluster":         &infrav1.DockerClusterList{},
			"DockerMachine":         &infrav1.DockerMachineList{},
			"DockerMachineTemplate": &infrav1.DockerMachineTemplateList{},
		}
		Expect(framework.DumpProviderResources(mgmt, resources, resourcesPath, GinkgoWriter)).To(Succeed())
	})

	It("upgrades kubernetes, kube-proxy, etcd and CoreDNS", func() {
		By("upgrading kubernetes version, etcd and CoreDNS image tags")
		patchHelper, err := patch.NewHelper(controlPlane, mgmtClient)
		Expect(err).ToNot(HaveOccurred())
		controlPlane.Spec.Version = "v1.17.2"
		controlPlane.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd = v1beta1.Etcd{
			Local: &v1beta1.LocalEtcd{
				ImageMeta: v1beta1.ImageMeta{
					// TODO: Ensure that the current version of etcd
					// is not 3.4.3-0 or that k8s version is 1.16. For now it
					// is.
					// 3.4.3-0 is the etcd version meant for k8s 1.17.x
					// k8s 1.16.x clusters ususally get deployed with etcd 3.3.x
					ImageTag: "3.4.3-0",
				},
			},
		}
		controlPlane.Spec.KubeadmConfigSpec.ClusterConfiguration.DNS = v1beta1.DNS{
			ImageMeta: v1beta1.ImageMeta{
				//Valid image when upgrading from v1.16 to v1.17.2
				ImageTag: "1.6.6",
			},
		}
		Expect(patchHelper.Patch(ctx, controlPlane)).To(Succeed())

		inClustersNamespaceListOption := ctrlclient.InNamespace(cluster.Namespace)
		// ControlPlane labels
		matchClusterListOption := ctrlclient.MatchingLabels{
			clusterv1.MachineControlPlaneLabelName: "",
			clusterv1.ClusterLabelName:             cluster.Name,
		}

		By("ensuring all machines have upgraded kubernetes")
		Eventually(func() (int, error) {
			machineList := &clusterv1.MachineList{}
			if err := mgmtClient.List(ctx, machineList, inClustersNamespaceListOption, matchClusterListOption); err != nil {
				fmt.Println(err)
				return 0, err
			}
			upgraded := 0
			for _, machine := range machineList.Items {
				if *machine.Spec.Version == controlPlane.Spec.Version {
					upgraded++
				}
			}
			if len(machineList.Items) > upgraded {
				return 0, errors.New("old nodes remain")
			}
			return upgraded, nil
		}, "15m", "30s").Should(Equal(int(*controlPlane.Spec.Replicas)))

		workloadClient, err := mgmt.GetWorkloadClient(ctx, cluster.Namespace, cluster.Name)
		Expect(err).ToNot(HaveOccurred())

		By("ensuring kube-proxy has the correct image")
		Eventually(func() (bool, error) {
			ds := &appsv1.DaemonSet{}

			if err := workloadClient.Get(ctx, ctrlclient.ObjectKey{Name: "kube-proxy", Namespace: metav1.NamespaceSystem}, ds); err != nil {
				return false, err
			}
			if ds.Spec.Template.Spec.Containers[0].Image == "k8s.gcr.io/kube-proxy:v1.17.2" {
				return true, nil
			}

			return false, nil
		}, "15m", "30s").Should(BeTrue())

		By("ensuring CoreDNS has the correct image")
		Eventually(func() (bool, error) {
			d := &appsv1.Deployment{}

			if err := workloadClient.Get(ctx, ctrlclient.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, d); err != nil {
				return false, err
			}
			if d.Spec.Template.Spec.Containers[0].Image == "k8s.gcr.io/coredns:1.6.6" {
				return true, nil
			}

			return false, nil
		}, "15m", "30s").Should(BeTrue())

		// Before patching ensure all pods are ready in workload cluster
		// Might not need this step any more.
		By("waiting for workload cluster pods to be Running")
		waitForPodListConditionInput := framework.WaitForPodListConditionInput{
			Lister:      workloadClient,
			ListOptions: &client.ListOptions{Namespace: metav1.NamespaceSystem},
			Condition:   framework.PhasePodCondition(corev1.PodRunning),
		}
		framework.WaitForPodListCondition(ctx, waitForPodListConditionInput)

		By("ensuring etcd pods have the correct image tag")
		lblSelector, err := labels.Parse("component=etcd")
		Expect(err).ToNot(HaveOccurred())
		opt := &client.ListOptions{LabelSelector: lblSelector}
		waitForPodListConditionInput = framework.WaitForPodListConditionInput{
			Lister:      workloadClient,
			ListOptions: opt,
			Condition:   framework.EtcdImageTagCondition("3.4.3-0", replicas),
		}
		framework.WaitForPodListCondition(ctx, waitForPodListConditionInput)
	})
})
