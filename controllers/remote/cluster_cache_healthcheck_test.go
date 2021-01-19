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
	"k8s.io/klog/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var _ = Describe("ClusterCache HealthCheck suite", func() {
	Context("when health checking clusters", func() {
		var mgr manager.Manager
		var mgrContext context.Context
		var mgrCancel context.CancelFunc
		var k8sClient client.Client

		var testNamespace *corev1.Namespace
		var testClusterKey client.ObjectKey
		var cct *ClusterCacheTracker
		var cc *stoppableCache

		var testPollInterval = 100 * time.Millisecond
		var testPollTimeout = 50 * time.Millisecond
		var testUnhealthyThreshold = 3

		BeforeEach(func() {
			By("Setting up a new manager")
			var err error
			mgr, err = manager.New(testEnv.Config, manager.Options{
				Scheme:             scheme.Scheme,
				MetricsBindAddress: "0",
			})
			Expect(err).NotTo(HaveOccurred())

			mgrContext, mgrCancel = context.WithCancel(ctx)
			By("Starting the manager")
			go func() {
				Expect(mgr.Start(mgrContext)).To(Succeed())
			}()
			<-testEnv.Manager.Elected()

			k8sClient = mgr.GetClient()

			By("Setting up a ClusterCacheTracker")
			cct, err = NewClusterCacheTracker(klogr.New(), mgr)
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
			testCluster.Status.ControlPlaneInitialized = true
			testCluster.Status.InfrastructureReady = true
			Expect(k8sClient.Status().Update(ctx, testCluster)).To(Succeed())

			By("Creating a test cluster kubeconfig")
			Expect(testEnv.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())

			testClusterKey = util.ObjectKey(testCluster)

			_, cancel := context.WithCancel(ctx)
			cc = &stoppableCache{cancelFunc: cancel}
			cct.clusterAccessors[testClusterKey] = &clusterAccessor{cache: cc}
		})

		AfterEach(func() {
			By("Deleting any Secrets")
			Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			By("Deleting any Clusters")
			Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			By("Stopping the manager")
			cc.cancelFunc()
			mgrCancel()
		})

		It("with a healthy cluster", func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// TODO(community): Fill in these field names.
			go cct.healthCheckCluster(ctx, &healthCheckInput{
				testClusterKey,
				testEnv.Config,
				testPollInterval,
				testPollTimeout,
				testUnhealthyThreshold,
				"/",
			})

			// Make sure this passes for at least two seconds, to give the health check goroutine time to run.
			Consistently(func() bool { return cct.clusterAccessorExists(testClusterKey) }, 2*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		It("with an invalid path", func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// TODO(community): Fill in these field names.
			go cct.healthCheckCluster(ctx,
				&healthCheckInput{
					testClusterKey,
					testEnv.Config,
					testPollInterval,
					testPollTimeout,
					testUnhealthyThreshold,
					"/clusterAccessor",
				})

			// This should succeed after N consecutive failed requests.
			Eventually(func() bool { return cct.clusterAccessorExists(testClusterKey) }, 2*time.Second, 100*time.Millisecond).Should(BeFalse())
		})

		It("with an invalid config", func() {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Set the host to a random free port on localhost
			addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
			Expect(err).ToNot(HaveOccurred())
			l, err := net.ListenTCP("tcp", addr)
			Expect(err).ToNot(HaveOccurred())
			l.Close()

			config := rest.CopyConfig(testEnv.Config)
			config.Host = fmt.Sprintf("http://127.0.0.1:%d", l.Addr().(*net.TCPAddr).Port)

			// TODO(community): Fill in these field names.
			go cct.healthCheckCluster(ctx, &healthCheckInput{
				testClusterKey,
				config,
				testPollInterval,
				testPollTimeout,
				testUnhealthyThreshold,
				"/",
			})

			// This should succeed after N consecutive failed requests.
			Eventually(func() bool { return cct.clusterAccessorExists(testClusterKey) }, 2*time.Second, 100*time.Millisecond).Should(BeFalse())
		})
	})
})
