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
//   - Added NewFilterableMachineCollectionFromMachineList initializer
//   - Updated Has to also check for equality of Machines
//   - Removed unused methods

package internal

import (
	"sort"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
	"sigs.k8s.io/cluster-api/util"
)

// FilterableMachineCollection is a set of Machines
type FilterableMachineCollection map[string]*clusterv1.Machine

// NewFilterableMachineCollection creates a FilterableMachineCollection from a list of values.
func NewFilterableMachineCollection(machines ...*clusterv1.Machine) FilterableMachineCollection {
	ss := make(FilterableMachineCollection, len(machines))
	ss.Insert(machines...)
	return ss
}

// NewFilterableMachineCollectionFromMachineList creates a FilterableMachineCollection from the given MachineList
func NewFilterableMachineCollectionFromMachineList(machineList *clusterv1.MachineList) FilterableMachineCollection {
	ss := make(FilterableMachineCollection, len(machineList.Items))
	if machineList != nil {
		for i := range machineList.Items {
			ss.Insert(&machineList.Items[i])
		}
	}
	return ss
}

// Insert adds items to the set.
func (s FilterableMachineCollection) Insert(machines ...*clusterv1.Machine) FilterableMachineCollection {
	for i := range machines {
		if machines[i] != nil {
			m := machines[i]
			s[m.Name] = m
		}
	}
	return s
}

// list returns the contents as a sorted Machine slice.
func (s FilterableMachineCollection) list() []*clusterv1.Machine {
	res := make(util.MachinesByCreationTimestamp, 0, len(s))
	for _, value := range s {
		res = append(res, value)
	}
	sort.Sort(res)
	return res
}

// unsortedList returns the slice with contents in random order.
func (s FilterableMachineCollection) unsortedList() []*clusterv1.Machine {
	res := make([]*clusterv1.Machine, 0, len(s))
	for _, value := range s {
		res = append(res, value)
	}
	return res
}

// Len returns the size of the set.
func (s FilterableMachineCollection) Len() int {
	return len(s)
}

func newFilteredMachineCollection(filter machinefilters.Func, machines ...*clusterv1.Machine) FilterableMachineCollection {
	ss := make(FilterableMachineCollection, len(machines))
	for i := range machines {
		m := machines[i]
		if filter(m) {
			ss.Insert(m)
		}
	}
	return ss
}

// Filter returns a FilterableMachineCollection containing only the Machines that match all of the given MachineFilters
func (s FilterableMachineCollection) Filter(filters ...machinefilters.Func) FilterableMachineCollection {
	return newFilteredMachineCollection(machinefilters.And(filters...), s.unsortedList()...)
}

// AnyFilter returns a FilterableMachineCollection containing only the Machines that match any of the given MachineFilters
func (s FilterableMachineCollection) AnyFilter(filters ...machinefilters.Func) FilterableMachineCollection {
	return newFilteredMachineCollection(machinefilters.Or(filters...), s.unsortedList()...)
}

// Oldest returns the Machine with the oldest CreationTimestamp
func (s FilterableMachineCollection) Oldest() *clusterv1.Machine {
	if len(s) == 0 {
		return nil
	}
	return s.list()[0]
}

// Newest returns the Machine with the most recent CreationTimestamp
func (s FilterableMachineCollection) Newest() *clusterv1.Machine {
	if len(s) == 0 {
		return nil
	}
	return s.list()[len(s)-1]
}

// DeepCopy returns a deep copy
func (s FilterableMachineCollection) DeepCopy() FilterableMachineCollection {
	result := make(FilterableMachineCollection, len(s))
	for _, m := range s {
		result.Insert(m.DeepCopy())
	}
	return result
}
