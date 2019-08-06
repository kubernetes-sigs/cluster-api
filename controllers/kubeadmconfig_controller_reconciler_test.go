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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api-bootstrap-provider-kubeadm/api/v1alpha2"
	clusterv1alpha2 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var _ = Describe("KubeadmConfigReconciler", func() {
	BeforeEach(func() {})
	AfterEach(func() {})

	Context("Reconcile a KubeadmConfig", func() {
		It("should successfully run through the reconcile function", func() {
			cluster := &clusterv1alpha2.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-cluster",
				},
				Status: clusterv1alpha2.ClusterStatus{
					InfrastructureReady: true,
				},
			}
			machine := &clusterv1alpha2.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-machine",
					Labels: map[string]string{
						clusterv1alpha2.MachineClusterLabelName: "my-cluster",
					},
				},
				Spec: clusterv1alpha2.MachineSpec{
					Bootstrap: clusterv1alpha2.Bootstrap{
						ConfigRef: &v1.ObjectReference{
							Kind:       "KubeadmConfig",
							APIVersion: "v1alpha2",
						},
					},
				},
			}
			config := &v1alpha2.KubeadmConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "my-config",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind:       "Machine",
							APIVersion: clusterv1alpha2.SchemeGroupVersion.String(),
							Name:       "my-machine",
							UID:        "a uid",
						},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cluster)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), machine)).To(Succeed())
			Expect(k8sClient.Create(context.Background(), config)).To(Succeed())

			reconciler := KubeadmConfigReconciler{
				Log:    log.ZapLogger(true),
				Client: k8sClient,
			}
			By("Calling reconcile")
			result, err := reconciler.Reconcile(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: "default",
					Name:      "my-config",
				},
			})
			Expect(err).To(Succeed())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())
		})
	})
})
