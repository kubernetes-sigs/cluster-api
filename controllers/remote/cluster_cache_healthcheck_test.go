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
	"net"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("ClusterCache HealthCheck suite", func() {
	Context("when health checking clusters", func() {
		var mgr manager.Manager
		var doneMgr chan struct{}
		var k8sClient client.Client

		var testNamespace *corev1.Namespace
		var testClusterKey client.ObjectKey
		var cct *ClusterCacheTracker
		var cc *clusterCache

		var testPollInterval = 100 * time.Millisecond
		var testPollTimeout = 50 * time.Millisecond
		var testUnhealthyThreshold = 3

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

			testClusterKey = util.ObjectKey(testCluster)

			cc = &clusterCache{
				stop: make(chan struct{}),
			}
			cct.clusterCaches[testClusterKey] = cc
		})

		AfterEach(func() {
			By("Deleting any Secrets")
			Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			By("Deleting any Clusters")
			Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			By("Stopping the manager")
			close(doneMgr)
		})

		It("with a healthy cluster", func() {
			// Ensure the context times out after the consistently below
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			config := rest.CopyConfig(cfg)
			go cct.healthCheckCluster(&healthCheckInput{ctx.Done(), testClusterKey, config, testPollInterval, testPollTimeout, testUnhealthyThreshold, "/"})

			// This should succed after 1 second, approx 10 requests to the API
			Consistently(func() *clusterCache {
				cct.clusterCachesLock.RLock()
				defer cct.clusterCachesLock.RUnlock()
				return cct.clusterCaches[testClusterKey]
			}).ShouldNot(BeNil())
			Expect(func() bool {
				cc.lock.Lock()
				defer cc.lock.Unlock()
				return cc.stopped
			}()).To(BeFalse())
		})

		It("with an invalid path", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			config := rest.CopyConfig(cfg)
			go cct.healthCheckCluster(&healthCheckInput{ctx.Done(), testClusterKey, config, testPollInterval, testPollTimeout, testUnhealthyThreshold, "/foo"})

			// This should succeed after 3 consecutive failed requests
			Eventually(func() *clusterCache {
				cct.clusterCachesLock.RLock()
				defer cct.clusterCachesLock.RUnlock()
				return cct.clusterCaches[testClusterKey]
			}).Should(BeNil())
			Expect(func() bool {
				cc.lock.Lock()
				defer cc.lock.Unlock()
				return cc.stopped
			}()).To(BeTrue())
		})

		It("with an invalid config", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			config := rest.CopyConfig(cfg)

			// Set the host to a random free port on localhost
			addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
			Expect(err).ToNot(HaveOccurred())
			l, err := net.ListenTCP("tcp", addr)
			Expect(err).ToNot(HaveOccurred())
			l.Close()
			config.Host = fmt.Sprintf("http://127.0.0.1:%d", l.Addr().(*net.TCPAddr).Port)

			go cct.healthCheckCluster(&healthCheckInput{ctx.Done(), testClusterKey, config, testPollInterval, testPollTimeout, testUnhealthyThreshold, "/"})

			// This should succeed after 3 consecutive failed requests
			Eventually(func() *clusterCache {
				cct.clusterCachesLock.RLock()
				defer cct.clusterCachesLock.RUnlock()
				return cct.clusterCaches[testClusterKey]
			}).Should(BeNil())
			Expect(func() bool {
				cc.lock.Lock()
				defer cc.lock.Unlock()
				return cc.stopped
			}()).To(BeTrue())
		})
	})
})
