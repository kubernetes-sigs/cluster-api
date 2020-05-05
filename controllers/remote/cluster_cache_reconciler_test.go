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
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	gtypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ClusterCache Reconciler suite", func() {
	Context("When running the ClusterCacheReconciler", func() {
		var mgr manager.Manager
		var doneMgr chan struct{}
		var cct *ClusterCacheTracker
		var k8sClient client.Client

		var testNamespace *corev1.Namespace
		var clusterRequest1, clusterRequest2, clusterRequest3 reconcile.Request
		var clusterCache1, clusterCache2, clusterCache3 *clusterCache

		// createAndWatchCluster creates a new cluster and ensures the clusterCacheTracker is watching the cluster
		createAndWatchCluster := func(clusterName string) (reconcile.Request, *clusterCache) {
			By(fmt.Sprintf("Creating a cluster %q", clusterName))
			testCluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: testNamespace.GetName(),
				},
			}
			Expect(k8sClient.Create(ctx, testCluster)).To(Succeed())

			// Check the cluster can be fetch from the API server
			testClusterKey := util.ObjectKey(testCluster)
			Eventually(func() error {
				return k8sClient.Get(ctx, testClusterKey, &clusterv1.Cluster{})
			}, timeout).Should(Succeed())

			By("Creating a test cluster kubeconfig")
			Expect(kubeconfig.CreateEnvTestSecret(k8sClient, cfg, testCluster)).To(Succeed())
			// Check the secret can be fetch from the API server
			secretKey := client.ObjectKey{Namespace: testNamespace.GetName(), Name: fmt.Sprintf("%s-kubeconfig", testCluster.GetName())}
			Eventually(func() error {
				return k8sClient.Get(ctx, secretKey, &corev1.Secret{})
			}, timeout).Should(Succeed())

			watcher, _ := newTestWatcher()
			kind := &corev1.Node{}
			eventHandler := &handler.Funcs{}

			By("Calling watch on the test cluster")
			// TODO: Make this test not rely on Watch doing its job?
			Expect(cct.Watch(ctx, WatchInput{
				Cluster:      testClusterKey,
				Watcher:      watcher,
				Kind:         kind,
				CacheOptions: cache.Options{},
				EventHandler: eventHandler,
				Predicates: []predicate.Predicate{
					&predicate.ResourceVersionChangedPredicate{},
					&predicate.GenerationChangedPredicate{},
				},
			})).To(Succeed())

			// Check that a cache was created for the cluster
			cc := func() *clusterCache {
				cct.clusterCachesLock.RLock()
				defer cct.clusterCachesLock.RUnlock()
				return cct.clusterCaches[testClusterKey]
			}()
			Expect(cc).ToNot(BeNil())
			return reconcile.Request{NamespacedName: testClusterKey}, cc
		}

		BeforeEach(func() {
			By("Setting up a new manager")
			var err error
			mgr, err = manager.New(cfg, manager.Options{
				Scheme:             scheme.Scheme,
				MetricsBindAddress: "0",
			})
			Expect(err).NotTo(HaveOccurred())

			doneMgr = make(chan struct{})
			By("Starting the manager")
			go func() {
				Expect(mgr.Start(doneMgr)).To(Succeed())
			}()

			k8sClient = mgr.GetClient()

			By("Setting up a ClusterCacheTracker")
			cct, err = NewClusterCacheTracker(log.NullLogger{}, mgr)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a namespace for the test")
			testNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "cluster-cache-test-"}}
			Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			By("Starting the ClusterCacheReconciler")
			r, err := NewClusterCacheReconciler(
				&log.NullLogger{},
				mgr,
				controller.Options{},
				cct,
			)
			Expect(err).ToNot(HaveOccurred())

			By("Creating clusters to test with")
			clusterRequest1, clusterCache1 = createAndWatchCluster("cluster-1")
			clusterRequest2, clusterCache2 = createAndWatchCluster("cluster-2")
			clusterRequest3, clusterCache3 = createAndWatchCluster("cluster-3")

			// Manually call Reconcile to ensure the Reconcile is completed before assertions
			_, err = r.Reconcile(clusterRequest1)
			Expect(err).ToNot(HaveOccurred())
			_, err = r.Reconcile(clusterRequest2)
			Expect(err).ToNot(HaveOccurred())
			_, err = r.Reconcile(clusterRequest3)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			By("Deleting any Secrets")
			Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			By("Deleting any Clusters")
			Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			By("Stopping the manager")
			close(doneMgr)
		})

		type clusterKeyAndCache struct {
			key   client.ObjectKey
			cache *clusterCache
		}

		type clusterDeletedInput struct {
			stoppedClusters func() []clusterKeyAndCache
			runningClusters func() []clusterKeyAndCache
		}

		DescribeTable("when clusters are deleted", func(in *clusterDeletedInput) {
			for _, cluster := range in.stoppedClusters() {
				By(fmt.Sprintf("Deleting cluster %q", cluster.key.Name))
				obj := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cluster.key.Name,
						Namespace: cluster.key.Namespace,
					},
				}
				Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
			}

			// Check the stopped clustser's caches eventually stop
			for _, cluster := range in.stoppedClusters() {
				By(fmt.Sprintf("Checking cluster %q's cache is stopped", cluster.key.Name))
				cc := cluster.cache
				Eventually(func() bool {
					cc.lock.Lock()
					defer cc.lock.Unlock()
					return cc.stopped
				}, timeout).Should(BeTrue())
			}

			// Check the running cluster's caches are still running
			for _, cluster := range in.runningClusters() {
				By(fmt.Sprintf("Checking cluster %q's cache is running", cluster.key.Name))
				cc := cluster.cache
				Consistently(func() bool {
					cc.lock.Lock()
					defer cc.lock.Unlock()
					return cc.stopped
				}).Should(BeFalse())
			}

			By("Checking deleted Cluster's Caches are removed", func() {
				matchers := []gtypes.GomegaMatcher{}
				for _, cluster := range in.stoppedClusters() {
					matchers = append(matchers, Not(HaveKey(cluster.key)))
				}
				for _, cluster := range in.runningClusters() {
					matchers = append(matchers, HaveKeyWithValue(cluster.key, cluster.cache))
				}
				matchers = append(matchers, HaveLen(len(in.runningClusters())))

				Eventually(func() map[client.ObjectKey]*clusterCache {
					cct.clusterCachesLock.RLock()
					defer cct.clusterCachesLock.RUnlock()
					return cct.clusterCaches
				}, timeout).Should(SatisfyAll(matchers...))
			})

			By("Checking deleted Cluster's Watches are removed", func() {
				matchers := []gtypes.GomegaMatcher{}
				for _, cluster := range in.stoppedClusters() {
					matchers = append(matchers, Not(HaveKey(cluster.key)))
				}
				for _, cluster := range in.runningClusters() {
					matchers = append(matchers, HaveKeyWithValue(cluster.key, Not(BeEmpty())))
				}
				matchers = append(matchers, HaveLen(len(in.runningClusters())))

				Eventually(func() map[client.ObjectKey]map[watchInfo]struct{} {
					cct.watchesLock.RLock()
					defer cct.watchesLock.RUnlock()
					return cct.watches
				}(), timeout).Should(SatisfyAll(matchers...))
			})
		},
			Entry("when no clusters deleted", &clusterDeletedInput{
				stoppedClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{}
				},
				runningClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{
						{
							key:   clusterRequest1.NamespacedName,
							cache: clusterCache1,
						},
						{
							key:   clusterRequest2.NamespacedName,
							cache: clusterCache2,
						},
						{
							key:   clusterRequest3.NamespacedName,
							cache: clusterCache3,
						},
					}
				},
			}),
			Entry("when test-cluster-1 is deleted", &clusterDeletedInput{
				stoppedClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{
						{
							key:   clusterRequest1.NamespacedName,
							cache: clusterCache1,
						},
					}
				},
				runningClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{
						{
							key:   clusterRequest2.NamespacedName,
							cache: clusterCache2,
						},
						{
							key:   clusterRequest3.NamespacedName,
							cache: clusterCache3,
						},
					}
				},
			}),
			Entry("when test-cluster-2 is deleted", &clusterDeletedInput{
				stoppedClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{
						{
							key:   clusterRequest2.NamespacedName,
							cache: clusterCache2,
						},
					}
				},
				runningClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{
						{
							key:   clusterRequest1.NamespacedName,
							cache: clusterCache1,
						},
						{
							key:   clusterRequest3.NamespacedName,
							cache: clusterCache3,
						},
					}
				},
			}),
			Entry("when test-cluster-1 and test-cluster-3 are deleted", &clusterDeletedInput{
				stoppedClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{
						{
							key:   clusterRequest1.NamespacedName,
							cache: clusterCache1,
						},
						{
							key:   clusterRequest3.NamespacedName,
							cache: clusterCache3,
						},
					}
				},
				runningClusters: func() []clusterKeyAndCache {
					return []clusterKeyAndCache{
						{
							key:   clusterRequest2.NamespacedName,
							cache: clusterCache2,
						},
					}
				},
			}),
		)
	})
})
