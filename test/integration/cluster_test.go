// +build integration

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

package integration

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controllers"
	"sigs.k8s.io/cluster-api/test/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

func init() {
	if err := clusterv1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
}

var clusterSpec = &clusterv1.ClusterSpec{
	ClusterNetwork: &clusterv1.ClusterNetwork{
		ServiceDomain: "mydomain.com",
		Pods: &clusterv1.NetworkRanges{
			CIDRBlocks: []string{"192.168.0.0/16"},
		},
	},
}

// Timeout for waiting events in seconds
const TIMEOUT = 60

func TestCluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cluster-Controller")
}

var _ = Describe("Cluster-Controller", func() {
	var testEnv *helpers.TestEnvironment
	var ns *corev1.Namespace

	BeforeEach(func() {
		var err error
		testEnv, err = helpers.NewTestEnvironment()
		Expect(err).NotTo(HaveOccurred())

		logger := klogr.New()
		Expect((&controllers.ClusterReconciler{
			Client: testEnv,
			Log:    logger,
		}).SetupWithManager(testEnv.Manager, controller.Options{MaxConcurrentReconciles: 1})).To(Succeed())

		go func() {
			Expect(testEnv.StartManager()).To(Succeed())
		}()
		// Create namespace for test
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "clusterapi-test-"}}
		Expect(testEnv.Create(context.Background(), ns)).To(Succeed())
	})

	AfterEach(func() {
		Expect(testEnv.Delete(context.Background(), ns)).To(Succeed())
		Expect(testEnv.Stop()).To(Succeed())
	})

	Describe("Create Cluster", func() {
		It("Should reach to pending phase after creation", func(done Done) {
			ctx := context.Background()
			// Create Cluster
			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "cluster-",
					Namespace:    ns.Name,
				},
				Spec: *clusterSpec.DeepCopy(),
			}

			Expect(testEnv.Create(ctx, cluster)).To(Succeed())
			Eventually(func() bool {
				if err := testEnv.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace}, cluster); err != nil {
					return false
				}
				return cluster.Status.Phase == string(clusterv1.ClusterPhasePending)
			}).Should(BeTrue())

			close(done)
		}, TIMEOUT)
	})
})
