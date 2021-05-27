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

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func TestClusterCacheReconciler(t *testing.T) {
	t.Run("When running the ClusterCacheReconciler", func(t *testing.T) {
		var (
			mgr           manager.Manager
			mgrContext    context.Context
			mgrCancel     context.CancelFunc
			cct           *ClusterCacheTracker
			k8sClient     client.Client
			testNamespace *corev1.Namespace
		)

		// createAndWatchCluster creates a new cluster and ensures the clusterCacheTracker has a clusterAccessor for it
		createAndWatchCluster := func(clusterName string, g *WithT) {
			t.Log(fmt.Sprintf("Creating a cluster %q", clusterName))
			testCluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: testNamespace.GetName(),
				},
			}
			g.Expect(k8sClient.Create(ctx, testCluster)).To(Succeed())

			// Check the cluster can be fetched from the API server
			testClusterKey := util.ObjectKey(testCluster)
			g.Eventually(func() error {
				return k8sClient.Get(ctx, testClusterKey, &clusterv1.Cluster{})
			}, timeout).Should(Succeed())

			t.Log("Creating a test cluster kubeconfig")
			g.Expect(env.CreateKubeconfigSecret(ctx, testCluster)).To(Succeed())

			// Check the secret can be fetched from the API server
			secretKey := client.ObjectKey{Namespace: testNamespace.GetName(), Name: fmt.Sprintf("%s-kubeconfig", testCluster.GetName())}
			g.Eventually(func() error {
				return k8sClient.Get(ctx, secretKey, &corev1.Secret{})
			}, timeout).Should(Succeed())

			t.Log("Creating a clusterAccessor for the cluster")
			_, err := cct.GetClient(ctx, testClusterKey)
			g.Expect(err).NotTo(HaveOccurred())
		}

		setup := func(t *testing.T, g *WithT) {
			t.Log("Setting up a new manager")
			var err error
			mgr, err = manager.New(env.Config, manager.Options{
				Scheme:             scheme.Scheme,
				MetricsBindAddress: "0",
			})
			g.Expect(err).NotTo(HaveOccurred())

			t.Log("Setting up a ClusterCacheTracker")
			cct, err = NewClusterCacheTracker(mgr, ClusterCacheTrackerOptions{})
			g.Expect(err).NotTo(HaveOccurred())

			t.Log("Creating the ClusterCacheReconciler")
			r := &ClusterCacheReconciler{
				Log:     log.NullLogger{},
				Client:  mgr.GetClient(),
				Tracker: cct,
			}
			g.Expect(r.SetupWithManager(ctx, mgr, controller.Options{})).To(Succeed())

			t.Log("Starting the manager")
			mgrContext, mgrCancel = context.WithCancel(ctx)
			go func() {
				g.Expect(mgr.Start(mgrContext)).To(Succeed())
			}()
			<-env.Manager.Elected()

			k8sClient = mgr.GetClient()

			t.Log("Creating a namespace for the test")
			testNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "cluster-cache-test-"}}
			g.Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())

			t.Log("Creating clusters to test with")
			createAndWatchCluster("cluster-1", g)
			createAndWatchCluster("cluster-2", g)
			createAndWatchCluster("cluster-3", g)
		}

		teardown := func(t *testing.T, g *WithT) {
			t.Log("Deleting any Secrets")
			g.Expect(cleanupTestSecrets(ctx, k8sClient)).To(Succeed())
			t.Log("Deleting any Clusters")
			g.Expect(cleanupTestClusters(ctx, k8sClient)).To(Succeed())
			t.Log("Stopping the manager")
			mgrCancel()
		}

		t.Run("should remove clusterAccessors when clusters are deleted", func(t *testing.T) {
			g := NewWithT(t)
			setup(t, g)
			defer teardown(t, g)

			for _, clusterName := range []string{"cluster-1", "cluster-2", "cluster-3"} {
				t.Log(fmt.Sprintf("Deleting cluster %q", clusterName))
				obj := &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace.Name,
						Name:      clusterName,
					},
				}
				g.Expect(k8sClient.Delete(ctx, obj)).To(Succeed())

				t.Log(fmt.Sprintf("Checking cluster %q's clusterAccessor is removed", clusterName))
				g.Eventually(func() bool { return cct.clusterAccessorExists(util.ObjectKey(obj)) }, timeout).Should(BeFalse())
			}
		})
	})
}
