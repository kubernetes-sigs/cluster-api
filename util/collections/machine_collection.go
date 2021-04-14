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

// Modified copy of k8s.io/apimachinery/pkg/util/sets/int64.go
// Modifications
//   - int64 became *clusterv1.Machine
//   - Empty type is removed
//   - Sortable data type is removed in favor of util.MachinesByCreationTimestamp
//   - nil checks added to account for the pointer
//   - Added Filter, AnyFilter, and Oldest methods
//   - Added FromMachineList initializer
//   - Updated Has to also check for equality of Machines
//   - Removed unused methods

package collections

import (
	"sort"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// Machines is a set of Machines.
type Machines map[string]*clusterv1.Machine

// New creates an empty Machines.
func New() Machines {
	return make(Machines)
}

// FromMachines creates a Machines from a list of values.
func FromMachines(machines ...*clusterv1.Machine) Machines {
	ss := make(Machines, len(machines))
	ss.Insert(machines...)
	return ss
}

// FromMachineList creates a Machines from the given MachineList.
func FromMachineList(machineList *clusterv1.MachineList) Machines {
	ss := make(Machines, len(machineList.Items))
	for i := range machineList.Items {
		ss.Insert(&machineList.Items[i])
	}
	return ss
}

// ToMachineList creates a MachineList from the given Machines.
func ToMachineList(machines Machines) clusterv1.MachineList {
	ml := clusterv1.MachineList{}
	for _, m := range machines {
		ml.Items = append(ml.Items, *m)
	}
	return ml
}

// Insert adds items to the set.
func (s Machines) Insert(machines ...*clusterv1.Machine) {
	for i := range machines {
		if machines[i] != nil {
			m := machines[i]
			s[m.Name] = m
		}
	}
}

// Difference returns a copy without machines that are in the given collection.
func (s Machines) Difference(machines Machines) Machines {
	return s.Filter(func(m *clusterv1.Machine) bool {
		_, found := machines[m.Name]
		return !found
	})
}

// SortedByCreationTimestamp returns the machines sorted by creation timestamp.
func (s Machines) SortedByCreationTimestamp() []*clusterv1.Machine {
	res := make(util.MachinesByCreationTimestamp, 0, len(s))
	for _, value := range s {
		res = append(res, value)
	}
	sort.Sort(res)
	return res
}

// UnsortedList returns the slice with contents in random order.
func (s Machines) UnsortedList() []*clusterv1.Machine {
	res := make([]*clusterv1.Machine, 0, len(s))
	for _, value := range s {
		res = append(res, value)
	}
	return res
}

// Len returns the size of the set.
func (s Machines) Len() int {
	return len(s)
}

// newFilteredMachineCollection creates a Machines from a filtered list of values.
func newFilteredMachineCollection(filter Func, machines ...*clusterv1.Machine) Machines {
	ss := make(Machines, len(machines))
	for i := range machines {
		m := machines[i]
		if filter(m) {
			ss.Insert(m)
		}
	}
	return ss
}

// Filter returns a Machines containing only the Machines that match all of the given MachineFilters.
func (s Machines) Filter(filters ...Func) Machines {
	return newFilteredMachineCollection(And(filters...), s.UnsortedList()...)
}

// AnyFilter returns a Machines containing only the Machines that match any of the given MachineFilters.
func (s Machines) AnyFilter(filters ...Func) Machines {
	return newFilteredMachineCollection(Or(filters...), s.UnsortedList()...)
}

// Oldest returns the Machine with the oldest CreationTimestamp.
func (s Machines) Oldest() *clusterv1.Machine {
	if len(s) == 0 {
		return nil
	}
	return s.SortedByCreationTimestamp()[0]
}

// Newest returns the Machine with the most recent CreationTimestamp.
func (s Machines) Newest() *clusterv1.Machine {
	if len(s) == 0 {
		return nil
	}
	return s.SortedByCreationTimestamp()[len(s)-1]
}

// DeepCopy returns a deep copy.
func (s Machines) DeepCopy() Machines {
	result := make(Machines, len(s))
	for _, m := range s {
		result.Insert(m.DeepCopy())
	}
	return result
}

// ConditionGetters returns the slice with machines converted into conditions.Getter.
func (s Machines) ConditionGetters() []conditions.Getter {
	res := make([]conditions.Getter, 0, len(s))
	for _, v := range s {
		value := *v
		res = append(res, &value)
	}
	return res
}

// Names returns a slice of the names of each machine in the collection.
// Useful for logging and test assertions.
func (s Machines) Names() []string {
	names := make([]string, 0, s.Len())
	for _, m := range s {
		names = append(names, m.Name)
	}
	return names
}
