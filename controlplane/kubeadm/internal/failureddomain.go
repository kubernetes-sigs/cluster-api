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
	"sort"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
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

// Scope down logr.Logger
type logger interface {
	Info(msg string, keysAndValues ...interface{})
}

// FailureDomainPicker picks a failure domain given a list of failure domains and a list of machines.
type FailureDomainPicker struct {
	Log logger
}

// PickMost returns the failure domain with the most number of machines.
func (f *FailureDomainPicker) PickMost(failureDomains clusterv1.FailureDomains, machineList *clusterv1.MachineList) string {
	aggregations := f.pick(failureDomains, machineList)
	if len(aggregations) == 0 {
		return ""
	}
	sort.Sort(sort.Reverse(aggregations))
	return aggregations[0].id

}

// PickFewest returns the failure domain with the fewest number of machines.
func (f *FailureDomainPicker) PickFewest(failureDomains clusterv1.FailureDomains, machineList *clusterv1.MachineList) string {
	aggregations := f.pick(failureDomains, machineList)
	if len(aggregations) == 0 {
		return ""
	}
	sort.Sort(aggregations)
	return aggregations[0].id
}

func (f *FailureDomainPicker) pick(failureDomains clusterv1.FailureDomains, machineList *clusterv1.MachineList) failureDomainAggregations {
	if len(failureDomains) == 0 {
		return failureDomainAggregations{}
	}
	if machineList == nil {
		return failureDomainAggregations{}
	}
	aggregations := make(failureDomainAggregations, len(failureDomains))

	count := map[string]int{}

	// Initialize the known failure domain keys to find out if an existing machine is in an unsupported failure domain.
	for fd := range failureDomains {
		count[fd] = 0
	}

	// Count how many machines are in each failure domain.
	for _, m := range machineList.Items {
		if m.Spec.FailureDomain == nil {
			continue
		}
		if _, ok := failureDomains[*m.Spec.FailureDomain]; !ok {
			f.Log.Info("unknown failure domain", "machine-name", m.GetName(), "failure-domain", *m.Spec.FailureDomain, "known-failure-domains", failureDomains)
			continue
		}
		count[*m.Spec.FailureDomain]++
	}

	// Gather up tuples of failure domains ids and counts
	i := 0
	for fd, count := range count {
		aggregations[i] = failureDomainAggregation{
			id:    fd,
			count: count,
		}
		i++
	}

	return aggregations
}
