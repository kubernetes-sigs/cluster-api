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

package controllers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"

	bootstrapv1 "sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
)

var _ = Describe("KubeadmConfigReconciler", func() {
	BeforeEach(func() {})
	AfterEach(func() {})

	Context("Reconcile a KubeadmConfig", func() {
		It("should wait until infrastructure is ready", func() {
			cluster := newCluster("cluster1")
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())

			machine := newMachine(cluster, "my-machine")
			Expect(k8sClient.Create(context.Background(), machine)).To(Succeed())

			config := newKubeadmConfig(machine, "my-machine-config")
			Expect(k8sClient.Create(context.Background(), config)).To(Succeed())

			reconciler := KubeadmConfigReconciler{
				Log:    log.Log,
				Client: k8sClient,
			}
			By("Calling reconcile should requeue")
			result, err := reconciler.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "my-machine-config",
				},
			})
			Expect(err).To(Succeed())
			Expect(result.Requeue).To(BeFalse())
		})
		/*
				When apimachinery decodes into a typed struct, the decoder strips the TypeMeta from the object;
				the theory at the time being that because it was a typed object, you knew its API version, group, and kind.
				if fact this leads to errors with k8sClient, because it loses GVK, and this leads r.Status().Patch to fail
				with "the server could not find the requested resource (patch kubeadmconfigs.bootstrap.cluster.x-k8s.io control-plane-config)"

			 	There's a WIP PR to k/k to fix this.
				After this merge, we can implement more behavioral test

				It("should process only control plane machines when infrastructure is ready but control plane is not", func() {
					cluster := newCluster("cluster2")
					Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
					cluster.Status.InfrastructureReady = true
					Expect(k8sClient.Status().Update(context.Background(), cluster)).To(Succeed())

					controlplaneMachine := newMachine(cluster, "control-plane")
					controlplaneMachine.ObjectMeta.Labels[clusterv1alpha2.MachineControlPlaneLabelName] = "true"
					Expect(k8sClient.Create(context.Background(), controlplaneMachine)).To(Succeed())

					controlplaneConfig := newKubeadmConfig(controlplaneMachine, "control-plane-config")
					controlplaneConfig.Spec.ClusterConfiguration = &kubeadmv1beta1.ClusterConfiguration{}
					controlplaneConfig.Spec.InitConfiguration = &kubeadmv1beta1.InitConfiguration{}
					Expect(k8sClient.Create(context.Background(), controlplaneConfig)).To(Succeed())

					workerMachine := newMachine(cluster, "worker")
					Expect(k8sClient.Create(context.Background(), workerMachine)).To(Succeed())

					workerConfig := newKubeadmConfig(workerMachine, "worker-config")
					Expect(k8sClient.Create(context.Background(), workerConfig)).To(Succeed())

					reconciler := KubeadmConfigReconciler{
						Log:    log.Log,
						Client: k8sClient,
					}

					By("Calling reconcile on a config corresponding to worker node should requeue")
					resultWorker, err := reconciler.Reconcile(ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: "default",
							Name:      "worker-config",
						},
					})
					Expect(err).To(Succeed())
					Expect(resultWorker.Requeue).To(BeFalse())
					Expect(resultWorker.RequeueAfter).To(Equal(30 * time.Second))

					By("Calling reconcile on a config corresponding to a control plane node should create BootstrapData")
					resultControlPlane, err := reconciler.Reconcile(ctrl.Request{
						NamespacedName: types.NamespacedName{
							Namespace: "default",
							Name:      "control-plane-config",
						},
					})
					Expect(err).To(Succeed())
					Expect(resultControlPlane.Requeue).To(BeFalse())
					Expect(resultControlPlane.RequeueAfter).To(BeZero())

					controlplaneConfigAfter, err := getKubeadmConfig(k8sClient, "control-plane-config")
					Expect(err).To(Succeed())
					Expect(controlplaneConfigAfter.Status.Ready).To(BeTrue())
					Expect(controlplaneConfigAfter.Status.BootstrapData).NotTo(BeEmpty())
				})
		*/
	})
})

// test utils

// getKubeadmConfig returns a KubeadmConfig object from the cluster
func getKubeadmConfig(c client.Client, name string) (*bootstrapv1.KubeadmConfig, error) {
	ctx := context.Background()
	controlplaneConfigKey := client.ObjectKey{
		Namespace: "default",
		Name:      name,
	}
	config := &bootstrapv1.KubeadmConfig{}
	err := c.Get(ctx, controlplaneConfigKey, config)
	return config, err
}
