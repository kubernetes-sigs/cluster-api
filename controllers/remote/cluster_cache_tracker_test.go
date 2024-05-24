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
	"testing"

	"github.com/davecgh/go-spew/spew"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func mapper(_ context.Context, i client.Object) []reconcile.Request {
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: i.GetNamespace(),
				Name:      getMappedName(i.GetName()),
			},
		},
	}
}

func getMappedName(name string) string {
	return fmt.Sprintf("mapped-%s", name)
}

func TestClusterCacheTracker(t *testing.T) {
	t.Run("watching", func(t *testing.T) {
		var (
			mgr        manager.Manager
			mgrContext context.Context
			mgrCancel  context.CancelFunc
			cct        *ClusterCacheTracker
			k8sClient  client.Client
			c          *testController
			w          Watcher
			clusterA   *clusterv1.Cluster
		)

		setup := func(t *testing.T, g *WithT) *corev1.Namespace {
			t.Helper()

			t.Log("Setting up a new manager")
			var err error
			mgr, err = manager.New(env.Config, manager.Options{
				Scheme: scheme.Scheme,
				Metrics: metricsserver.Options{
					BindAddress: "0",
				},
			})
			g.Expect(err).ToNot(HaveOccurred())

			c = &testController{
				ch: make(chan string),
			}
			w, err = ctrl.NewControllerManagedBy(mgr).For(&clusterv1.MachineDeployment{}).Build(c)
			g.Expect(err).ToNot(HaveOccurred())

			mgrContext, mgrCancel = context.WithCancel(ctx)
			t.Log("Starting the manager")
			go func() {
				g.Expect(mgr.Start(mgrContext)).To(Succeed())
			}()
			<-mgr.Elected()

			k8sClient = mgr.GetClient()

			t.Log("Setting up a ClusterCacheTracker")
			cct, err = NewClusterCacheTracker(mgr, ClusterCacheTrackerOptions{
				Indexes: []Index{NodeProviderIDIndex},
			})
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Creating a namespace for the test")
			ns, err := env.CreateNamespace(ctx, "cluster-cache-tracker-test")
			g.Expect(err).ToNot(HaveOccurred())

			t.Log("Creating a test cluster")
			clusterA = &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns.GetName(),
					Name:      "test-cluster",
				},
			}
			g.Expect(k8sClient.Create(ctx, clusterA)).To(Succeed())
			conditions.MarkTrue(clusterA, clusterv1.ControlPlaneInitializedCondition)
			clusterA.Status.InfrastructureReady = true
			g.Expect(k8sClient.Status().Update(ctx, clusterA)).To(Succeed())

			t.Log("Creating a test cluster kubeconfig")
			g.Expect(env.CreateKubeconfigSecret(ctx, clusterA)).To(Succeed())

			return ns
		}

		teardown := func(t *testing.T, g *WithT, ns *corev1.Namespace) {
			t.Helper()
			defer close(c.ch)

			t.Log("Deleting any Secrets")
			g.Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			t.Log("Deleting any Clusters")
			g.Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			g.Expect(<-c.ch).To(Equal(getMappedName(clusterA.Name)))
			g.Consistently(c.ch).ShouldNot(Receive())
			t.Log("Deleting Namespace")
			g.Expect(env.Delete(ctx, ns)).To(Succeed())
			t.Log("Stopping the manager")
			mgrCancel()
		}

		t.Run("with the same name should succeed and not have duplicates", func(t *testing.T) {
			g := NewWithT(t)
			ns := setup(t, g)
			defer teardown(t, g, ns)

			t.Log("Creating the watch")
			g.Expect(cct.Watch(ctx, WatchInput{
				Name:         "watch1",
				Cluster:      util.ObjectKey(clusterA),
				Watcher:      w,
				Kind:         &clusterv1.Cluster{},
				EventHandler: handler.EnqueueRequestsFromMapFunc(mapper),
			})).To(Succeed())

			t.Log("Waiting to receive the watch notification")
			g.Expect(<-c.ch).To(Equal(getMappedName(clusterA.Name)))

			t.Log("Ensuring no additional watch notifications arrive")
			g.Consistently(c.ch).ShouldNot(Receive())

			t.Log("Updating the cluster")
			clusterA.Annotations = map[string]string{
				"update1": "1",
			}
			g.Expect(k8sClient.Update(ctx, clusterA)).To(Succeed())

			t.Log("Waiting to receive the watch notification")
			g.Expect(<-c.ch).To(Equal(getMappedName(clusterA.Name)))

			t.Log("Ensuring no additional watch notifications arrive")
			g.Consistently(c.ch).ShouldNot(Receive())

			t.Log("Creating the same watch a second time")
			g.Expect(cct.Watch(ctx, WatchInput{
				Name:         "watch1",
				Cluster:      util.ObjectKey(clusterA),
				Watcher:      w,
				Kind:         &clusterv1.Cluster{},
				EventHandler: handler.EnqueueRequestsFromMapFunc(mapper),
			})).To(Succeed())

			t.Log("Ensuring no additional watch notifications arrive")
			g.Consistently(c.ch).ShouldNot(Receive())

			t.Log("Updating the cluster")
			clusterA.Annotations["update1"] = "2"
			g.Expect(k8sClient.Update(ctx, clusterA)).To(Succeed())

			t.Log("Waiting to receive the watch notification")
			g.Expect(<-c.ch).To(Equal(getMappedName(clusterA.Name)))

			t.Log("Ensuring no additional watch notifications arrive")
			g.Consistently(c.ch).ShouldNot(Receive())
		})
	})

	t.Run("runningOnWorkloadCluster", func(t *testing.T) {
		tests := []struct {
			name                      string
			currentControllerMetadata *metav1.ObjectMeta
			clusterObjects            []client.Object
			expected                  bool
		}{
			{
				name: "should return true if the controller is running on the workload cluster",
				currentControllerMetadata: &metav1.ObjectMeta{
					Name:      "controller-pod",
					Namespace: "controller-pod-namespace",
					UID:       types.UID("controller-pod-uid"),
				},
				clusterObjects: []client.Object{
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "controller-pod",
							Namespace: "controller-pod-namespace",
							UID:       types.UID("controller-pod-uid"),
						},
					},
				},
				expected: true,
			},
			{
				name: "should return false if the controller is not running on the workload cluster: name mismatch",
				currentControllerMetadata: &metav1.ObjectMeta{
					Name:      "controller-pod",
					Namespace: "controller-pod-namespace",
					UID:       types.UID("controller-pod-uid"),
				},
				clusterObjects: []client.Object{
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "controller-pod-mismatch",
							Namespace: "controller-pod-namespace",
							UID:       types.UID("controller-pod-uid"),
						},
					},
				},
				expected: false,
			},
			{
				name: "should return false if the controller is not running on the workload cluster: namespace mismatch",
				currentControllerMetadata: &metav1.ObjectMeta{
					Name:      "controller-pod",
					Namespace: "controller-pod-namespace",
					UID:       types.UID("controller-pod-uid"),
				},
				clusterObjects: []client.Object{
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "controller-pod",
							Namespace: "controller-pod-namespace-mismatch",
							UID:       types.UID("controller-pod-uid"),
						},
					},
				},
				expected: false,
			},
			{
				name: "should return false if the controller is not running on the workload cluster: uid mismatch",
				currentControllerMetadata: &metav1.ObjectMeta{
					Name:      "controller-pod",
					Namespace: "controller-pod-namespace",
					UID:       types.UID("controller-pod-uid"),
				},
				clusterObjects: []client.Object{
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "controller-pod",
							Namespace: "controller-pod-namespace",
							UID:       types.UID("controller-pod-uid-mismatch"),
						},
					},
				},
				expected: false,
			},
			{
				name: "should return false if the controller is not running on the workload cluster: no pod in cluster",
				currentControllerMetadata: &metav1.ObjectMeta{
					Name:      "controller-pod",
					Namespace: "controller-pod-namespace",
					UID:       types.UID("controller-pod-uid"),
				},
				clusterObjects: []client.Object{},
				expected:       false,
			},
			{
				name:                      "should return false if the controller is not running on the workload cluster: no controller metadata",
				currentControllerMetadata: nil,
				clusterObjects: []client.Object{
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "controller-pod",
							Namespace: "controller-pod-namespace",
							UID:       types.UID("controller-pod-uid"),
						},
					},
				},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				c := fake.NewClientBuilder().WithObjects(tt.clusterObjects...).Build()

				cct := &ClusterCacheTracker{
					controllerPodMetadata: tt.currentControllerMetadata,
				}

				found, err := cct.runningOnWorkloadCluster(ctx, c, client.ObjectKey{Name: "test-cluster", Namespace: "test-namespace"})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(found).To(Equal(tt.expected))
			})
		}
	})
}

type testController struct {
	ch chan string
}

func (c *testController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	spew.Dump(req)
	select {
	case <-ctx.Done():
	case c.ch <- req.Name:
	}
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
