/*
Copyright 2018 The Kubernetes Authors.

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

package machine_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "k8s.io/kube-deploy/ext-apiserver/pkg/apis/cluster/v1alpha1"
	. "k8s.io/kube-deploy/ext-apiserver/pkg/client/clientset_generated/clientset/typed/cluster/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Machine controller", func() {
	var instance Machine
	var expectedKey string
	var client MachineInterface
	var before chan struct{}
	var after chan struct{}

	BeforeEach(func() {
		instance = Machine{}
		instance.Name = "instance-1"
		expectedKey = "default/instance-1"
	})

	AfterEach(func() {
		client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when creating a new object", func() {
		It("invoke the reconcile method", func() {
			cluster := Cluster{}
			cluster.Name = "cluster-1"
			_, err := cs.ClusterV1alpha1().Clusters("default").Create(&cluster)
			Expect(err).ShouldNot(HaveOccurred())
			client = cs.ClusterV1alpha1().Machines("default")
			before = make(chan struct{})
			after = make(chan struct{})
			var aftOnce, befOnce sync.Once

			actualKey := ""
			var actualErr error = nil

			// Setup test callbacks to be called when the message is reconciled
			controller.BeforeReconcile = func(key string) {
				actualKey = key
				befOnce.Do(func() { close(before) })
			}
			controller.AfterReconcile = func(key string, err error) {
				actualKey = key
				actualErr = err
				aftOnce.Do(func() { close(after) })
			}

			// Create an instance
			_, err = client.Create(&instance)
			Expect(err).ShouldNot(HaveOccurred())

			// Verify reconcile function is called against the correct key
			select {
			case <-before:
				Expect(actualKey).To(Equal(expectedKey))
				Expect(actualErr).ShouldNot(HaveOccurred())
			case <-time.After(time.Second * 2):
				Fail("reconcile never called")
			}

			select {
			case <-after:
				Expect(actualKey).To(Equal(expectedKey))
				Expect(actualErr).ShouldNot(HaveOccurred())
			case <-time.After(time.Second * 2):
				Fail("reconcile never finished")
			}
		})
	})
})
