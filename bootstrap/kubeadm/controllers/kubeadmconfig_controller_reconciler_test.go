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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha3"
)

var _ = Describe("KubeadmConfigReconciler", func() {
	BeforeEach(func() {})
	AfterEach(func() {})

	Context("Reconcile a KubeadmConfig", func() {
		It("should wait until infrastructure is ready", func() {
			cluster := newCluster("cluster1")
			Expect(testEnv.Create(context.Background(), cluster)).To(Succeed())

			machine := newMachine(cluster, "my-machine")
			Expect(testEnv.Create(context.Background(), machine)).To(Succeed())

			config := newKubeadmConfig(machine, "my-machine-config")
			Expect(testEnv.Create(context.Background(), config)).To(Succeed())

			reconciler := KubeadmConfigReconciler{
				Log:    log.Log,
				Client: testEnv,
			}
			By("Calling reconcile should requeue")
			result, err := reconciler.Reconcile(ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: "default",
					Name:      "my-machine-config",
				},
			})
			Expect(err).To(Succeed())
			Expect(result.Requeue).To(BeFalse())
		})
	})
})

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
