/*
Copyright 2021 The Kubernetes Authors.

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

package scope

import (
	"strconv"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// Scope holds all the information to process a request in the topology/ClusterReconciler controller.
type Scope struct {
	// Blueprint holds all the objects required for computing the desired state of a managed topology.
	Blueprint *ClusterBlueprint

	// Current holds the current state of the managed topology.
	Current *ClusterState

	// Desired holds the desired state of the managed topology.
	Desired *ClusterState

	// UpgradeTracker holds information about ongoing upgrades in the managed topology.
	UpgradeTracker *UpgradeTracker

	// HookResponseTracker holds the hook responses that will be used to
	// calculate a combined reconcile result.
	HookResponseTracker *HookResponseTracker
}

// New returns a new Scope with only the cluster; while processing a request in the topology/ClusterReconciler controller
// additional information will be added about the Cluster blueprint, current state and desired state.
func New(cluster *clusterv1.Cluster) *Scope {
	// enforce TypeMeta values in the Cluster object so we can assume it is always set during reconciliation.
	cluster.APIVersion = clusterv1.GroupVersion.String()
	cluster.Kind = "Cluster"

	// Determine the maximum upgrade concurrency from the annotation on the cluster.
	maxMDUpgradeConcurrency := 1
	maxMPUpgradeConcurrency := 1
	if concurrency, ok := cluster.Annotations[clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation]; ok {
		// The error can be ignored because the webhook ensures that the value is a positive integer.
		maxMDUpgradeConcurrency, _ = strconv.Atoi(concurrency)
		maxMPUpgradeConcurrency, _ = strconv.Atoi(concurrency)
	}
	return &Scope{
		Blueprint: &ClusterBlueprint{},
		Current: &ClusterState{
			Cluster: cluster,
		},
		UpgradeTracker: NewUpgradeTracker(
			MaxMDUpgradeConcurrency(maxMDUpgradeConcurrency),
			MaxMPUpgradeConcurrency(maxMPUpgradeConcurrency),
		),
		HookResponseTracker: NewHookResponseTracker(),
	}
}
