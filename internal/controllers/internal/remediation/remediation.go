/*
Copyright 2025 The Kubernetes Authors.

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

// Package collections contains helpers for remediation related to machine health checks.
package remediation

import (
	"sort"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/annotations"
)

// SortMachinesToRemediate returns the machines to be remediated in the following order
//   - Machines with RemediateMachineAnnotation annotation if any,
//   - Machines failing to come up first because
//     there is a chance that they are not hosting any workloads (minimize disruption).
//
// We are trying to remediate machines failing to come up first because
// there is a chance that they are not hosting any workloads (minimize disruption).
func SortMachinesToRemediate(machines []*clusterv1.Machine) {
	sort.SliceStable(machines, func(i, j int) bool {
		if annotations.HasRemediateMachine(machines[i]) && !annotations.HasRemediateMachine(machines[j]) {
			return true
		}
		if !annotations.HasRemediateMachine(machines[i]) && annotations.HasRemediateMachine(machines[j]) {
			return false
		}
		// Use newest (and Name) as a tie-breaker criteria.
		if machines[i].CreationTimestamp.Equal(&machines[j].CreationTimestamp) {
			return machines[i].Name < machines[j].Name
		}
		return machines[i].CreationTimestamp.After(machines[j].CreationTimestamp.Time)
	})
}
