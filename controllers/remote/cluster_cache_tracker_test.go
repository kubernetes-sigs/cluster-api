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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func mapper(i handler.MapObject) []reconcile.Request {
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: i.Meta.GetNamespace(),
				Name:      "mapped-" + i.Meta.GetName(),
			},
		},
	}
}

var _ = Describe("ClusterCache Tracker suite", func() {
	Describe("watching", func() {
		var (
			mgr           manager.Manager
			doneMgr       chan struct{}
			cct           *ClusterCacheTracker
			k8sClient     client.Client
			testNamespace *corev1.Namespace
			c             *testController
			w             Watcher
			clusterA      *clusterv1.Cluster
		)

		BeforeEach(func() {
			By("Setting up a new manager")
			var err error
			mgr, err = manager.New(testEnv.Config, manager.Options{
				Scheme:             scheme.Scheme,
				MetricsBindAddress: "0",
			})
			Expect(err).NotTo(HaveOccurred())

			c = &testController{
				ch: make(chan string),
			}
			w, err = ctrl.NewControllerManagedBy(mgr).For(&clusterv1.MachineDeployment{}).Build(c)
			Expect(err).To(BeNil())

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
			clusterA = &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   testNamespace.GetName(),
					Name:        "test-cluster",
					Annotations: make(map[string]string),
				},
			}
			Expect(k8sClient.Create(ctx, clusterA)).To(Succeed())
			clusterA.Status.ControlPlaneInitialized = true
			clusterA.Status.InfrastructureReady = true
			Expect(k8sClient.Status().Update(ctx, clusterA)).To(Succeed())

			By("Creating a test cluster kubeconfig")
			Expect(testEnv.CreateKubeconfigSecret(clusterA)).To(Succeed())
		})

		AfterEach(func() {
			By("Deleting any Secrets")
			Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			By("Deleting any Clusters")
			Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			By("Stopping the manager")
			close(doneMgr)
		})

		It("with the same name should succeed and not have duplicates", func() {
			By("Creating the watch")
			Expect(cct.Watch(ctx, WatchInput{
				Name:         "watch1",
				Cluster:      util.ObjectKey(clusterA),
				Watcher:      w,
				Kind:         &clusterv1.Cluster{},
				EventHandler: &handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(mapper)},
			})).To(Succeed())

			By("Waiting to receive the watch notification")
			Expect(<-c.ch).To(Equal("mapped-" + clusterA.Name))

			By("Ensuring no additional watch notifications arrive")
			Consistently(func() int {
				return len(c.ch)
			}).Should(Equal(0))

			By("Updating the cluster")
			clusterA.Annotations["update1"] = "1"
			Expect(k8sClient.Update(ctx, clusterA)).Should(Succeed())

			By("Waiting to receive the watch notification")
			Expect(<-c.ch).To(Equal("mapped-" + clusterA.Name))

			By("Ensuring no additional watch notifications arrive")
			Consistently(func() int {
				return len(c.ch)
			}).Should(Equal(0))

			By("Creating the same watch a second time")
			Expect(cct.Watch(ctx, WatchInput{
				Name:         "watch1",
				Cluster:      util.ObjectKey(clusterA),
				Watcher:      w,
				Kind:         &clusterv1.Cluster{},
				EventHandler: &handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(mapper)},
			})).To(Succeed())

			By("Ensuring no additional watch notifications arrive")
			Consistently(func() int {
				return len(c.ch)
			}).Should(Equal(0))

			By("Updating the cluster")
			clusterA.Annotations["update1"] = "2"
			Expect(k8sClient.Update(ctx, clusterA)).Should(Succeed())

			By("Waiting to receive the watch notification")
			Expect(<-c.ch).To(Equal("mapped-" + clusterA.Name))

			By("Ensuring no additional watch notifications arrive")
			Consistently(func() int {
				return len(c.ch)
			}).Should(Equal(0))
		})
	})
})

type testController struct {
	ch chan string
}

func (c *testController) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	c.ch <- req.Name
	return ctrl.Result{}, nil
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
