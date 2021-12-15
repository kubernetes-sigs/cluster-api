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

// Package failuredomains implements FailureDomain utility functions.
package failuredomains

import (
	"sort"

	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/pointer"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

type failureDomainAggregation struct {
	id    string
	count int
}
type failureDomainAggregations []failureDomainAggregation

// Len is the number of elements in the collection.
func (f failureDomainAggregations) Len() int {
	return len(f)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (f failureDomainAggregations) Less(i, j int) bool {
	return f[i].count < f[j].count
}

// Swap swaps the elements with indexes i and j.
func (f failureDomainAggregations) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

// PickMost returns a failure domain that is in machines and has most of the group of machines on.
func PickMost(failureDomains clusterv1.FailureDomains, groupMachines, machines collections.Machines) *string {
	// orderDescending sorts failure domains according to all machines belonging to the group.
	fds := orderDescending(failureDomains, groupMachines)
	for _, fd := range fds {
		for _, m := range machines {
			if m.Spec.FailureDomain == nil {
				continue
			}
			if *m.Spec.FailureDomain == fd.id {
				return &fd.id
			}
		}
	}
	return nil
}

// orderDescending returns the sorted failure domains in decreasing order.
func orderDescending(failureDomains clusterv1.FailureDomains, machines collections.Machines) failureDomainAggregations {
	aggregations := pick(failureDomains, machines)
	if len(aggregations) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(aggregations))
	return aggregations
}

// PickFewest returns the failure domain with the fewest number of machines.
func PickFewest(failureDomains clusterv1.FailureDomains, machines collections.Machines) *string {
	aggregations := pick(failureDomains, machines)
	if len(aggregations) == 0 {
		return nil
	}
	sort.Sort(aggregations)
	return pointer.StringPtr(aggregations[0].id)
}

func pick(failureDomains clusterv1.FailureDomains, machines collections.Machines) failureDomainAggregations {
	if len(failureDomains) == 0 {
		return failureDomainAggregations{}
	}

	counters := map[string]int{}

	// Initialize the known failure domain keys to find out if an existing machine is in an unsupported failure domain.
	for fd := range failureDomains {
		counters[fd] = 0
	}

	// Count how many machines are in each failure domain.
	for _, m := range machines {
		if m.Spec.FailureDomain == nil {
			continue
		}
		id := *m.Spec.FailureDomain
		if _, ok := failureDomains[id]; !ok {
			klogr.New().Info("unknown failure domain", "machine-name", m.GetName(), "failure-domain-id", id, "known-failure-domains", failureDomains)
			continue
		}
		counters[id]++
	}

	aggregations := make(failureDomainAggregations, 0)

	// Gather up tuples of failure domains ids and counts
	for fd, count := range counters {
		aggregations = append(aggregations, failureDomainAggregation{id: fd, count: count})
	}

	return aggregations
}
