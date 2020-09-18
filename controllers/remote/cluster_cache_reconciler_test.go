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

package remote

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("ClusterCache Reconciler suite", func() {
	Context("When running the ClusterCacheReconciler", func() {
		var (
			mgr           manager.Manager
			doneMgr       chan struct{}
			cct           *ClusterCacheTracker
			k8sClient     client.Client
			testNamespace *corev1.Namespace
		)

		// createAndWatchCluster creates a new cluster and ensures the clusterCacheTracker has a clusterAccessor for it
		createAndWatchCluster := func(clusterName string) {
			By(fmt.Sprintf("Creating a cluster %q", clusterName))
			testCluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: testNamespace.GetName(),
				},
			}
			Expect(k8sClient.Create(ctx, testCluster)).To(Succeed())

			// Check the cluster can be fetched from the API server
			testClusterKey := util.ObjectKey(testCluster)
			Eventually(func() error {
				return k8sClient.Get(ctx, testClusterKey, &clusterv1.Cluster{})
			}, timeout).Should(Succeed())

			By("Creating a test cluster kubeconfig")
			Expect(testEnv.CreateKubeconfigSecret(testCluster)).To(Succeed())

			// Check the secret can be fetched from the API server
			secretKey := client.ObjectKey{Namespace: testNamespace.GetName(), Name: fmt.Sprintf("%s-kubeconfig", testCluster.GetName())}
			Eventually(func() error {
				return k8sClient.Get(ctx, secretKey, &corev1.Secret{})
			}, timeout).Should(Succeed())

			By("Creating a clusterAccessor for the cluster")
			_, err := cct.GetClient(ctx, testClusterKey)
			Expect(err).To(BeNil())
		}

		BeforeEach(func() {
			By("Setting up a new manager")
			var err error
			mgr, err = manager.New(testEnv.Config, manager.Options{
				Scheme:             scheme.Scheme,
				MetricsBindAddress: "0",
			})
			Expect(err).NotTo(HaveOccurred())

			By("Setting up a ClusterCacheTracker")
			cct, err = NewClusterCacheTracker(log.NullLogger{}, mgr)
			Expect(err).NotTo(HaveOccurred())

			By("Creating the ClusterCacheReconciler")
			r := &ClusterCacheReconciler{
				Log:     log.NullLogger{},
				Client:  mgr.GetClient(),
				Tracker: cct,
			}
			Expect(r.SetupWithManager(mgr, controller.Options{})).To(Succeed())

			By("Starting the manager")
			doneMgr = make(chan struct{})
			go func() {
				Expect(mgr.Start(doneMgr)).To(Succeed())
			}()

			k8sClient = mgr.GetClient()

			By("Creating a namespace for the test")
			testNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "cluster-cache-test-"}}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			By("Creating clusters to test with")
			createAndWatchCluster("cluster-1")
			createAndWatchCluster("cluster-2")
			createAndWatchCluster("cluster-3")
		})

		AfterEach(func() {
			By("Deleting any Secrets")
			Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			By("Deleting any Clusters")
			Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			By("Stopping the manager")
			close(doneMgr)
		})

		It("should remove clusterAccessors when clusters are deleted", func() {
			for _, clusterName := range []string{"cluster-1", "cluster-2", "cluster-3"} {
				By(fmt.Sprintf("Deleting cluster %q", clusterName))
				obj := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace.Name,
						Name:      clusterName,
					},
				}
				Expect(k8sClient.Delete(ctx, obj)).To(Succeed())

				By(fmt.Sprintf("Checking cluster %q's clusterAccessor is removed", clusterName))
				Eventually(func() bool { return cct.clusterAccessorExists(util.ObjectKey(obj)) }, timeout).Should(BeFalse())
			}
		})
	})
})
