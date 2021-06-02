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
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestClusterCacheHealthCheck(t *testing.T) {
	t.Run("when health checking clusters", func(t *testing.T) {
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

		setup := func(t *testing.T, g *WithT) {
			t.Log("Setting up a new manager")
			var err error
			mgr, err = manager.New(env.Config, manager.Options{
				Scheme:             scheme.Scheme,
				MetricsBindAddress: "0",
			})
			g.Expect(err).NotTo(HaveOccurred())

			mgrContext, mgrCancel = context.WithCancel(ctx)
			t.Log("Starting the manager")
			go func() {
				g.Expect(mgr.Start(mgrContext)).To(Succeed())
			}()
			<-env.Manager.Elected()

			k8sClient = mgr.GetClient()

			t.Log("Setting up a ClusterCacheTracker")
			cct, err = NewClusterCacheTracker(mgr, ClusterCacheTrackerOptions{Log: klogr.New()})
			g.Expect(err).NotTo(HaveOccurred())

			t.Log("Creating a namespace for the test")
			testNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "cluster-cache-test-"}}
			g.Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			t.Log("Creating a test cluster")
			testCluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: testNamespace.GetName(),
				},
			}
			g.Expect(k8sClient.Create(ctx, testCluster)).To(Succeed())
			conditions.MarkTrue(testCluster, clusterv1.ControlPlaneInitializedCondition)
			testCluster.Status.InfrastructureReady = true
			g.Expect(k8sClient.Status().Update(ctx, testCluster)).To(Succeed())

			t.Log("Creating a test cluster kubeconfig")
			g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())

			testClusterKey = util.ObjectKey(testCluster)

			_, cancel := context.WithCancel(ctx)
			cc = &stoppableCache{cancelFunc: cancel}
			cct.clusterAccessors[testClusterKey] = &clusterAccessor{cache: cc}
		}

		teardown := func(t *testing.T, g *WithT) {
			t.Log("Deleting any Secrets")
			g.Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			t.Log("Deleting any Clusters")
			g.Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			t.Log("Stopping the manager")
			cc.cancelFunc()
			mgrCancel()
		}

		t.Run("with a healthy cluster", func(t *testing.T) {
			g := NewWithT(t)
			setup(t, g)
			defer teardown(t, g)

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// TODO(community): Fill in these field names.
			go cct.healthCheckCluster(ctx, &healthCheckInput{
				testClusterKey,
				env.Config,
				testPollInterval,
				testPollTimeout,
				testUnhealthyThreshold,
				"/",
			})

			// Make sure this passes for at least two seconds, to give the health check goroutine time to run.
			g.Consistently(func() bool { return cct.clusterAccessorExists(testClusterKey) }, 2*time.Second, 100*time.Millisecond).Should(BeTrue())
		})

		t.Run("with an invalid path", func(t *testing.T) {
			g := NewWithT(t)
			setup(t, g)
			defer teardown(t, g)

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// TODO(community): Fill in these field names.
			go cct.healthCheckCluster(ctx,
				&healthCheckInput{
					testClusterKey,
					env.Config,
					testPollInterval,
					testPollTimeout,
					testUnhealthyThreshold,
					"/clusterAccessor",
				})

			// This should succeed after N consecutive failed requests.
			g.Eventually(func() bool { return cct.clusterAccessorExists(testClusterKey) }, 2*time.Second, 100*time.Millisecond).Should(BeFalse())
		})

		t.Run("with an invalid config", func(t *testing.T) {
			g := NewWithT(t)
			setup(t, g)
			defer teardown(t, g)

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Set the host to a random free port on localhost
			addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
			g.Expect(err).NotTo(HaveOccurred())
			l, err := net.ListenTCP("tcp", addr)
			g.Expect(err).NotTo(HaveOccurred())
			l.Close()

			config := rest.CopyConfig(env.Config)
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
			g.Eventually(func() bool { return cct.clusterAccessorExists(testClusterKey) }, 2*time.Second, 100*time.Millisecond).Should(BeFalse())
		})
	})
}
