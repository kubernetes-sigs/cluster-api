/*
Copyright YEAR The Kubernetes Authors.

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

package v1beta1_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/kubernetes-incubator/apiserver-builder/example/pkg/apis/olympus/v1beta1"
	. "github.com/kubernetes-incubator/apiserver-builder/example/pkg/client/clientset_generated/clientset/typed/olympus/v1beta1"
)

var _ = Describe("Poseidon", func() {
	var instance Poseidon
	var expected Poseidon
	var client PoseidonInterface

	BeforeEach(func() {
		instance = Poseidon{}
		instance.Name = "instance-1"

		expected = instance
	})

	AfterEach(func() {
		client.Delete(instance.Name, &metav1.DeleteOptions{})
	})

	Describe("when sending a storage request", func() {
		Context("for a valid config", func() {
			It("should provide CRUD access to the object", func() {
				client = cs.OlympusV1beta1().Poseidons("poseidon-test-valid")

				By("returning success from the create request")
				actual, err := client.Create(&instance)
				Expect(err).ShouldNot(HaveOccurred())

				By("defaulting the expected fields")
				Expect(actual.Spec).To(Equal(expected.Spec))

				By("returning the item for list requests")
				result, err := client.List(metav1.ListOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0].Spec).To(Equal(expected.Spec))

				By("returning the item for get requests")
				actual, err = client.Get(instance.Name, metav1.GetOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(actual.Spec).To(Equal(expected.Spec))

				By("deleting the item for delete requests")
				err = client.Delete(instance.Name, &metav1.DeleteOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				result, err = client.List(metav1.ListOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Items).To(HaveLen(0))
			})
		})
	})

	Describe("when listing a resource", func() {
		Context("using labels", func() {
			It("shouldn't find the matchings objects because the functions are overriden", func() {
				client = cs.OlympusV1beta1().Poseidons("poseidon-test-valid")

				instance1 := Poseidon{}
				instance1.Name = "instance-1"
				instance1.Spec.Deployment.Name = "i1"
				instance1.Labels = map[string]string{"foo": "1"}
				expected1 := instance1

				instance2 := Poseidon{}
				instance2.Name = "instance-2"
				instance2.Spec.Deployment.Name = "i2"
				instance2.Labels = map[string]string{"foo": "2"}
				expected2 := instance2

				By("returning success from the create request")
				_, err := client.Create(&instance1)
				Expect(err).ShouldNot(HaveOccurred())

				By("returning success from the create request")
				_, err = client.Create(&instance2)
				Expect(err).ShouldNot(HaveOccurred())

				By("returning the item for list requests")
				result, err := client.List(metav1.ListOptions{FieldSelector: "spec.deployment.name=i1"})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0].Name).To(Equal(expected1.Name))

				By("returning the item for list requests")
				result, err = client.List(metav1.ListOptions{FieldSelector: "spec.deployment.name=i2"})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0].Name).To(Equal(expected2.Name))

				By("returning the item for list requests")
				result, err = client.List(metav1.ListOptions{})
				Expect(err).ShouldNot(HaveOccurred())
				Expect(result.Items).To(HaveLen(2))
			})
		})
	})
})
