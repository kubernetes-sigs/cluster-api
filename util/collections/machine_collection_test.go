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

package collections_test

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

func TestMachineCollection(t *testing.T) {
	t.Run("SortedByAge", func(t *testing.T) {
		t.Run("should return the same number of machines as are in the collection", func(t *testing.T) {
			g := NewWithT(t)
			collection := machines()
			sortedMachines := collection.SortedByCreationTimestamp()
			g.Expect(sortedMachines).To(HaveLen(len(collection)))
			g.Expect(sortedMachines[0].Name).To(Equal("machine-1"))
			g.Expect(sortedMachines[len(sortedMachines)-1].Name).To(Equal("machine-5"))
		})
	})
	t.Run("Difference", func(t *testing.T) {
		t.Run("should return the collection with elements of the second collection removed", func(t *testing.T) {
			g := NewWithT(t)
			collection := machines()
			c2 := collection.Filter(func(m *clusterv1.Machine) bool {
				return m.Name != "machine-1"
			})
			c3 := collection.Difference(c2)
			// does not mutate
			g.Expect(collection.Names()).To(ContainElement("machine-1"))
			g.Expect(c3.Names()).To(ConsistOf("machine-1"))
		})
	})
	t.Run("Names", func(t *testing.T) {
		t.Run("should return a slice of names of each machine in the collection", func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(collections.New().Names()).To(BeEmpty())
			g.Expect(collections.FromMachines(machine("1"), machine("2")).Names()).To(ConsistOf("1", "2"))
		})
	})
}

func TestMachinesLowestVersion(t *testing.T) {
	tests := []struct {
		name     string
		machines collections.Machines
		expected *string
	}{
		{
			name:     "return empty for empty machines collection",
			machines: collections.New(),
			expected: nil,
		},
		{
			name: "return empty if machines dont have version",
			machines: func() collections.Machines {
				machines := collections.New()
				machines.Insert(&clusterv1.Machine{})
				return machines
			}(),
			expected: nil,
		},
		{
			name: "return lowest version from machines",
			machines: func() collections.Machines {
				machines := collections.New()
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-1"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String("1.20"),
				}})
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-2"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String("1.19.8"),
				}})
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-3"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String(""),
				}})
				return machines
			}(),
			expected: pointer.String("1.19.8"),
		},
		{
			name: "return lowest version from machines with pre release versions",
			machines: func() collections.Machines {
				machines := collections.New()
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-1"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String("1.20.1"),
				}})
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-2"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String("1.20.1-alpha.1"),
				}})
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-3"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String(""),
				}})
				return machines
			}(),
			expected: pointer.String("1.20.1-alpha.1"),
		},
		{
			name: "return lowest version from machines with build identifier versions",
			machines: func() collections.Machines {
				machines := collections.New()
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-1"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String("1.20.1+xyz.2"),
				}})
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-2"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String("1.20.1+xyz.1"),
				}})
				machines.Insert(&clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-3"}, Spec: clusterv1.MachineSpec{
					Version: pointer.String(""),
				}})
				return machines
			}(),
			expected: pointer.String("1.20.1+xyz.1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.machines.LowestVersion()).To(Equal(tt.expected))
		})
	}
}

/* Helper functions to build machine objects for tests. */

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

func machines() collections.Machines {
	return collections.Machines{
		"machine-4": machine("machine-4", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 04, 02, 03, 04, 05, 06, time.UTC)})),
		"machine-5": machine("machine-5", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 05, 02, 03, 04, 05, 06, time.UTC)})),
		"machine-2": machine("machine-2", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 02, 02, 03, 04, 05, 06, time.UTC)})),
		"machine-1": machine("machine-1", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 01, 02, 03, 04, 05, 06, time.UTC)})),
		"machine-3": machine("machine-3", withCreationTimestamp(metav1.Time{Time: time.Date(2018, 03, 02, 03, 04, 05, 06, time.UTC)})),
	}
}
