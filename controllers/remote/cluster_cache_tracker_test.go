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
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var _ = Describe("ClusterCache Tracker suite", func() {
	Context("When Watch is called for a cluster", func() {
		type assertWatchInput struct {
			tracker      *ClusterCacheTracker
			clusterKey   client.ObjectKey
			watcher      Watcher
			kind         runtime.Object
			eventHandler handler.EventHandler
			predicates   []predicate.Predicate
			watcherInfo  chan testWatcherInfo
			watchCount   int
		}

		var assertWatch = func(i *assertWatchInput) {
			It("should create a running cache for the cluster", func() {
				cache, ok := func() (*clusterCache, bool) {
					i.tracker.clusterCachesLock.RLock()
					defer i.tracker.clusterCachesLock.RUnlock()
					cc, ok := i.tracker.clusterCaches[i.clusterKey]
					return cc, ok
				}()
				Expect(ok).To(BeTrue())
				stop := make(chan struct{})
				Expect(func() bool {
					cache.lock.Lock()
					defer cache.lock.Unlock()
					return cache.stopped
				}()).To(BeFalse())
				Expect(func() chan struct{} {
					cache.lock.Lock()
					defer cache.lock.Unlock()
					return cache.stop
				}()).ToNot(BeNil())
				// WaitForCacheSync returns false if Start was not called
				Expect(cache.Cache.WaitForCacheSync(stop)).To(BeTrue())
			})

			It("should add a watchInfo for the watch", func() {
				expectedInfo := watchInfo{
					watcher:      i.watcher,
					kind:         i.kind,
					eventHandler: i.eventHandler,
				}
				Expect(func() map[watchInfo]struct{} {
					i.tracker.watchesLock.RLock()
					defer i.tracker.watchesLock.RUnlock()
					return i.tracker.watches[i.clusterKey]
				}()).To(HaveKey(Equal(expectedInfo)))
			})

			It("should call the Watcher with the correct values", func() {
				infos := []testWatcherInfo{}

				watcherInfoLen := len(i.watcherInfo)
				for j := 0; j < watcherInfoLen; j++ {
					infos = append(infos, <-i.watcherInfo)
				}

				Expect(infos).To(ContainElement(
					Equal(testWatcherInfo{
						handler:    i.eventHandler,
						predicates: i.predicates,
					}),
				))
			})

			It("should call the Watcher the expected number of times", func() {
				Expect(i.watcherInfo).Should(HaveLen(i.watchCount))
			})
		}

		var mgr manager.Manager
		var doneMgr chan struct{}
		var cct *ClusterCacheTracker
		var k8sClient client.Client

		var testNamespace *corev1.Namespace
		var input assertWatchInput

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

			By("Creating a test cluster")
			testCluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: testNamespace.GetName(),
				},
			}
			Expect(k8sClient.Create(ctx, testCluster)).To(Succeed())

			By("Creating a test cluster kubeconfig")
			Expect(kubeconfig.CreateEnvTestSecret(k8sClient, cfg, testCluster)).To(Succeed())

			testClusterKey := util.ObjectKey(testCluster)
			watcher, watcherInfo := newTestWatcher()
			kind := &corev1.Node{}
			eventHandler := &handler.Funcs{}

			input = assertWatchInput{
				cct,
				testClusterKey,
				watcher,
				kind,
				eventHandler,
				[]predicate.Predicate{
					&predicate.ResourceVersionChangedPredicate{},
					&predicate.GenerationChangedPredicate{},
				},
				watcherInfo,
				1,
			}

			By("Calling watch on the test cluster")
			Expect(cct.Watch(ctx, WatchInput{
				Cluster:      input.clusterKey,
				Watcher:      input.watcher,
				Kind:         input.kind,
				CacheOptions: cache.Options{},
				EventHandler: input.eventHandler,
				Predicates:   input.predicates,
			})).To(Succeed())
		})

		AfterEach(func() {
			By("Deleting any Secrets")
			Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			By("Deleting any Clusters")
			Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			By("Stopping the manager")
			close(doneMgr)
		})

		Context("when watch is called for a cluster", func() {
			assertWatch(&input)
		})

		Context("when Watch is called for a second time with the same input", func() {
			BeforeEach(func() {
				By("Calling watch on the test cluster")
				Expect(cct.Watch(ctx, WatchInput{
					Cluster:      input.clusterKey,
					Watcher:      input.watcher,
					Kind:         input.kind,
					CacheOptions: cache.Options{},
					EventHandler: input.eventHandler,
					Predicates:   input.predicates,
				})).To(Succeed())
			})

			assertWatch(&input)
		})

		Context("when watch is called with a different Kind", func() {
			BeforeEach(func() {
				configMapKind := &corev1.ConfigMap{}
				input.kind = configMapKind
				input.watchCount = 2

				By("Calling watch on the test cluster")
				Expect(cct.Watch(ctx, WatchInput{
					Cluster:      input.clusterKey,
					Watcher:      input.watcher,
					Kind:         input.kind,
					CacheOptions: cache.Options{},
					EventHandler: input.eventHandler,
					Predicates:   input.predicates,
				})).To(Succeed())
			})

			assertWatch(&input)
		})

		Context("when watch is called with a different EventHandler", func() {
			BeforeEach(func() {
				input.eventHandler = &handler.Funcs{
					CreateFunc: func(_ event.CreateEvent, _ workqueue.RateLimitingInterface) {},
				}
				input.watchCount = 2

				By("Calling watch on the test cluster")
				Expect(cct.Watch(ctx, WatchInput{
					Cluster:      input.clusterKey,
					Watcher:      input.watcher,
					Kind:         input.kind,
					CacheOptions: cache.Options{},
					EventHandler: input.eventHandler,
					Predicates:   input.predicates,
				})).To(Succeed())
			})

			assertWatch(&input)
		})

		Context("when watch is called with different Predicates", func() {
			BeforeEach(func() {
				input.predicates = []predicate.Predicate{}
				input.watchCount = 1

				By("Calling watch on the test cluster")
				Expect(cct.Watch(ctx, WatchInput{
					Cluster:      input.clusterKey,
					Watcher:      input.watcher,
					Kind:         input.kind,
					CacheOptions: cache.Options{},
					EventHandler: input.eventHandler,
					Predicates:   input.predicates,
				})).To(Succeed())
			})

			It("does not call the Watcher a second time", func() {
				Expect(input.watcherInfo).Should(HaveLen(1))
			})
		})

		Context("when watch is called with different Cluster", func() {
			var differentClusterInput assertWatchInput

			BeforeEach(func() {
				// Copy the input so we can check both clusters at once
				differentClusterInput = assertWatchInput{
					cct,
					input.clusterKey,
					input.watcher,
					input.kind,
					input.eventHandler,
					[]predicate.Predicate{
						&predicate.ResourceVersionChangedPredicate{},
						&predicate.GenerationChangedPredicate{},
					},
					input.watcherInfo,
					1,
				}

				By("Creating a different cluster")
				testCluster := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "different-cluster",
						Namespace: testNamespace.GetName(),
					},
				}
				Expect(k8sClient.Create(ctx, testCluster)).To(Succeed())

				By("Creating a test cluster kubeconfig")
				Expect(kubeconfig.CreateEnvTestSecret(k8sClient, cfg, testCluster)).To(Succeed())
				// Check the secret can be fetch from the API server
				secretKey := client.ObjectKey{Namespace: testNamespace.GetName(), Name: fmt.Sprintf("%s-kubeconfig", testCluster.GetName())}
				Eventually(func() error {
					return k8sClient.Get(ctx, secretKey, &corev1.Secret{})
				}, timeout).Should(Succeed())

				differentClusterInput.clusterKey = util.ObjectKey(testCluster)
				differentClusterInput.watchCount = 2

				input.watchCount = 2

				By("Calling watch on the test cluster")
				Expect(cct.Watch(ctx, WatchInput{
					Cluster:      differentClusterInput.clusterKey,
					Watcher:      differentClusterInput.watcher,
					Kind:         differentClusterInput.kind,
					CacheOptions: cache.Options{},
					EventHandler: differentClusterInput.eventHandler,
					Predicates:   differentClusterInput.predicates,
				})).To(Succeed())
			})

			// Check conditions for both clusters are still satisfied
			assertWatch(&input)
			assertWatch(&differentClusterInput)

			It("should have independent caches for each cluster", func() {
				testClusterCache, ok := func() (*clusterCache, bool) {
					cct.clusterCachesLock.RLock()
					defer cct.clusterCachesLock.RUnlock()
					cc, ok := cct.clusterCaches[input.clusterKey]
					return cc, ok
				}()
				Expect(ok).To(BeTrue())
				differentClusterCache, ok := func() (*clusterCache, bool) {
					cct.clusterCachesLock.RLock()
					defer cct.clusterCachesLock.RUnlock()
					cc, ok := cct.clusterCaches[differentClusterInput.clusterKey]
					return cc, ok
				}()
				Expect(ok).To(BeTrue())
				// Check stop channels are different, so that they can be stopped independently
				Expect(testClusterCache.stop).ToNot(Equal(differentClusterCache.stop))
				// Caches should also be different as they were constructed for different clusters
				// Check the memory locations to assert independence
				Expect(fmt.Sprintf("%p", testClusterCache.Cache)).ToNot(Equal(fmt.Sprintf("%p", differentClusterCache.Cache)))
			})
		})
	})
})

type testWatchFunc = func(source.Source, handler.EventHandler, ...predicate.Predicate) error

type testWatcher struct {
	watchInfo chan testWatcherInfo
}

type testWatcherInfo struct {
	handler    handler.EventHandler
	predicates []predicate.Predicate
}

func (t *testWatcher) Watch(s source.Source, h handler.EventHandler, ps ...predicate.Predicate) error {
	t.watchInfo <- testWatcherInfo{
		handler:    h,
		predicates: ps,
	}
	return nil
}

func newTestWatcher() (Watcher, chan testWatcherInfo) {
	watchInfo := make(chan testWatcherInfo, 5)
	return &testWatcher{
		watchInfo: watchInfo,
	}, watchInfo
}

func cleanupTestSecrets(ctx context.Context, c client.Client) error {
	secretList := &corev1.SecretList{}
	if err := c.List(ctx, secretList); err != nil {
		return err
	}
	for _, secret := range secretList.Items {
		s := secret
		if err := c.Delete(ctx, &s); err != nil {
			return err
		}
	}
	return nil
}

func cleanupTestClusters(ctx context.Context, c client.Client) error {
	clusterList := &clusterv1.ClusterList{}
	if err := c.List(ctx, clusterList); err != nil {
		return err
	}
	for _, cluster := range clusterList.Items {
		o := cluster
		if err := c.Delete(ctx, &o); err != nil {
			return err
		}
	}
	return nil
}
