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

package internal

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func TestMachineCollection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Machine Collection Suite")
}

var _ = Describe("Machine Collection", func() {
	Describe("FilterableMachineCollection", func() {
		var collection FilterableMachineCollection
		BeforeEach(func() {
			collection = FilterableMachineCollection{
				"machine-4": machine("machine-4", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 04, 02, 03, 04, 05, 06, time.UTC)})),
				"machine-5": machine("machine-5", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 05, 02, 03, 04, 05, 06, time.UTC)})),
				"machine-2": machine("machine-2", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 02, 02, 03, 04, 05, 06, time.UTC)})),
				"machine-1": machine("machine-1", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 01, 02, 03, 04, 05, 06, time.UTC)})),
				"machine-3": machine("machine-3", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 03, 02, 03, 04, 05, 06, time.UTC)})),
			}
		})
		Describe("SortedByAge", func() {
			It("should return the same number of machines as are in the collection", func() {
				sortedMachines := collection.SortedByCreationTimestamp()
				Expect(sortedMachines).To(HaveLen(len(collection)))
				Expect(sortedMachines[0].Name).To(Equal("machine-1"))
				Expect(sortedMachines[len(sortedMachines)-1].Name).To(Equal("machine-5"))
			})
		})
		Describe("Difference", func() {
			It("returns the collection with elements of the second collection removed", func() {
				c2 := collection.Filter(func(m *clusterv1.Machine) bool {
					return m.Name != "machine-1"
				})
				c3 := collection.Difference(c2)

				// does not mutate
				Expect(collection.Names()).To(ContainElement("machine-1"))
				Expect(c3.Names()).To(ConsistOf("machine-1"))
			})
		})

		Describe("Names", func() {
			It("returns a slice of names of each machine in the collection", func() {
				Expect(NewFilterableMachineCollection().Names()).To(BeEmpty())
				Expect(NewFilterableMachineCollection(machine("1"), machine("2")).Names()).To(ConsistOf("1", "2"))
			})
		})
	})
})

/* Helper functions to build machine objects for tests */

type machineOpt func(*clusterv1.Machine)

func withCreationTimestamp(timestamp metav1.Time) machineOpt {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = timestamp
	}
}

func machine(name string, opts ...machineOpt) *clusterv1.Machine {
	m := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}
