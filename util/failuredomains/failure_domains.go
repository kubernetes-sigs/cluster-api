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
	"context"
	"fmt"
	"sort"

	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

type failureDomainAggregation struct {
	id            string
	countPriority int
	countAll      int
}
type failureDomainAggregations []failureDomainAggregation

// Len is the number of elements in the collection.
func (f failureDomainAggregations) Len() int {
	return len(f)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (f failureDomainAggregations) Less(i, j int) bool {
	// If a failure domain has less priority machines then the other, it goes first
	if f[i].countPriority < f[j].countPriority {
		return true
	}
	if f[i].countPriority > f[j].countPriority {
		return false
	}

	// If a failure domain has the same number of priority machines then the other,
	// use the number of overall machines to pick which one goes first.
	if f[i].countAll < f[j].countAll {
		return true
	}
	if f[i].countAll > f[j].countAll {
		return false
	}

	// If both failure domain have the same number of priority machines and overall machines, we keep the order
	// in the list which ensure a certain degree of randomness because the list originates from a map.
	// This helps to spread machines e.g. when concurrently working on many clusters.
	return i < j
}

// Swap swaps the elements with indexes i and j.
func (f failureDomainAggregations) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

// PickMost returns the failure domain from which we have to delete a control plane machine, which is the failure domain with most machines and at least one eligible machine in it.
func PickMost(ctx context.Context, failureDomains clusterv1.FailureDomains, allMachines, eligibleMachines collections.Machines) *string {
	aggregations := countByFailureDomain(ctx, failureDomains, allMachines, eligibleMachines)
	if len(aggregations) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(aggregations))
	if len(aggregations) > 0 && aggregations[0].countPriority > 0 {
		return ptr.To(aggregations[0].id)
	}
	return nil
}

// PickFewest returns the failure domain that will be used for placement of a new control plane machine, which is the failure domain with the fewest
// number of up-to-date, not deleted machines.
//
// Ensuring proper spreading of up-to-date, not deleted machines, is the highest priority to achieve ideal spreading of machines
// at stable state/when only up-to-date machines will exist.
//
// In case of tie (more failure domain with the same number of up-to-date, not deleted machines) the failure domain with the fewest number of
// machine overall is picked to ensure a better spreading of machines while the rollout is performed.
func PickFewest(ctx context.Context, failureDomains clusterv1.FailureDomains, allMachines, upToDateMachines collections.Machines) *string {
	aggregations := countByFailureDomain(ctx, failureDomains, allMachines, upToDateMachines)
	if len(aggregations) == 0 {
		return nil
	}
	sort.Sort(aggregations)
	return ptr.To(aggregations[0].id)
}

// countByFailureDomain returns failure domains with the number of machines in it.
// Note: countByFailureDomain computes both the number of machines as well as the number of a subset of machines with higher priority.
// E.g. for deletion out of date machines have higher priority vs other machines.
func countByFailureDomain(ctx context.Context, failureDomains clusterv1.FailureDomains, allMachines, priorityMachines collections.Machines) failureDomainAggregations {
	log := ctrl.LoggerFrom(ctx)

	if len(failureDomains) == 0 {
		return failureDomainAggregations{}
	}

	counters := map[string]failureDomainAggregation{}

	// Initialize the known failure domain keys to find out if an existing machine is in an unsupported failure domain.
	for id := range failureDomains {
		counters[id] = failureDomainAggregation{
			id:            id,
			countPriority: 0,
			countAll:      0,
		}
	}

	// Count how many machines are in each failure domain.
	for _, m := range allMachines {
		if m.Spec.FailureDomain == nil {
			continue
		}
		id := *m.Spec.FailureDomain
		if _, ok := failureDomains[id]; !ok {
			var knownFailureDomains []string
			for failureDomainID := range failureDomains {
				knownFailureDomains = append(knownFailureDomains, failureDomainID)
			}
			log.Info(fmt.Sprintf("Unknown failure domain %q for Machine %s (known failure domains: %v)", id, m.GetName(), knownFailureDomains))
			continue
		}
		a := counters[id]
		a.countAll++
		counters[id] = a
	}

	for _, m := range priorityMachines {
		if m.Spec.FailureDomain == nil {
			continue
		}
		id := *m.Spec.FailureDomain
		if _, ok := failureDomains[id]; !ok {
			continue
		}
		a := counters[id]
		a.countPriority++
		counters[id] = a
	}

	// Collect failure domain aggregations.
	// Note: by creating the list from a map, we get a certain degree of randomness that helps to spread machines
	// e.g. when concurrently working on many clusters.
	aggregations := make(failureDomainAggregations, 0)
	for _, count := range counters {
		aggregations = append(aggregations, count)
	}
	return aggregations
}
