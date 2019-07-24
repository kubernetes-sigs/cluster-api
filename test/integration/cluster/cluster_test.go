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
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha2"
	clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha2"
)

var clusterSpec = &clusterv1.ClusterSpec{
	ClusterNetwork: &clusterv1.ClusterNetworkingConfig{
		ServiceDomain: "mydomain.com",
		Services: clusterv1.NetworkRanges{
			CIDRBlocks: []string{"10.96.0.0/12"},
		},
		Pods: clusterv1.NetworkRanges{
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
	var clusterapi client.ClusterInterface
	var client *kubernetes.Clientset
	var testNamespace string

	BeforeEach(func() {
		// Load configuration
		kubeconfig := os.Getenv("KUBECONFIG")
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		Expect(err).ShouldNot(HaveOccurred())

		// Create kubernetes client
		client, err = kubernetes.NewForConfig(config)
		Expect(err).ShouldNot(HaveOccurred())

		// Create namespace for test
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "clusterapi-test-"}}
		ns, err = client.CoreV1().Namespaces().Create(ns)
		Expect(err).ShouldNot(HaveOccurred())
		testNamespace = ns.ObjectMeta.Name

		// Create clusterapi client
		cs, err := clientset.NewForConfig(config)
		Expect(err).ShouldNot(HaveOccurred())
		clusterapi = cs.ClusterV1alpha2().Clusters(testNamespace)
	})

	AfterEach(func() {
		client.CoreV1().Namespaces().Delete(testNamespace, &metav1.DeleteOptions{})
	})

	Describe("Create Cluster", func() {
		It("Should reach to pending phase after creation", func(done Done) {
			// Create Cluster
			cluster := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "cluster-",
					Namespace:    testNamespace,
				},
				Spec: *clusterSpec.DeepCopy(),
			}

			var err error
			cluster, err = clusterapi.Create(cluster)
			Expect(err).ShouldNot(HaveOccurred())
			Eventually(func() bool {
				cluster, err = clusterapi.Get(cluster.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}
				return cluster.Status.Phase != nil && *cluster.Status.Phase == "pending"
			}).Should(BeTrue())

			close(done)
		}, TIMEOUT)
	})
})
