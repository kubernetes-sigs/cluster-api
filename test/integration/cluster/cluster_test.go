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
	informers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	client "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"
)

var clusterSpec = &clusterv1alpha1.ClusterSpec{
	ClusterNetwork: clusterv1alpha1.ClusterNetworkingConfig{
		ServiceDomain: "mydomain.com",
		Services: clusterv1alpha1.NetworkRanges{
			CIDRBlocks: []string{"10.96.0.0/12"},
		},
		Pods: clusterv1alpha1.NetworkRanges{
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
	var stopper chan struct{}
	var informer cache.SharedIndexInformer
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

		// Create  informer for events in the namespace
		factory := informers.NewSharedInformerFactoryWithOptions(client, 0, informers.WithNamespace(testNamespace))
		informer = factory.Core().V1().Events().Informer()
		stopper = make(chan struct{})

		// Create clusterapi client
		cs, err := clientset.NewForConfig(config)
		Expect(err).ShouldNot(HaveOccurred())
		clusterapi = cs.ClusterV1alpha1().Clusters(testNamespace)
	})

	AfterEach(func() {
		close(stopper)
		client.CoreV1().Namespaces().Delete(testNamespace, &metav1.DeleteOptions{})
	})

	Describe("Create Cluster", func() {
		It("Should trigger an event", func(done Done) {
			// Register handler for cluster events
			events := make(chan *corev1.Event, 0)
			informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					// Guard against miscofigured informer not listening to Events
					Expect(obj).Should(BeAssignableToTypeOf(&corev1.Event{}))

					e := obj.(*corev1.Event)
					if e.InvolvedObject.Kind == "Cluster" {
						events <- e
					}
				},
			})
			go informer.Run(stopper)
			Expect(cache.WaitForCacheSync(stopper, informer.HasSynced)).To(BeTrue())

			// Create Cluster
			cluster := &clusterv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "cluster-",
					Namespace:    testNamespace,
				},
				Spec: *clusterSpec.DeepCopy(),
			}
			_, err := clusterapi.Create(cluster)
			Expect(err).ShouldNot(HaveOccurred())

			Expect(<-events).ShouldNot(BeNil())

			close(done)
		}, TIMEOUT)
	})
})
