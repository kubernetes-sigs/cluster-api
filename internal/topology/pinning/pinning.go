/*
Copyright 2026 The Kubernetes Authors.

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

// Package pinning provides helpers to determine if a MachineDeployment/MachinePool
// pins its own Kubernetes version instead of following Cluster.spec.topology.version.
//
// Note: the ClusterTopologyWorkerVersionPinning feature gate only controls if a version can be set,
// which is enforced by the Cluster webhook. A version that is already set is always honored, so that
// disabling the feature gate never changes the version of a MachineDeployment/MachinePool.
package pinning

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

// MachineDeploymentTopologyVersion returns the version pinned on the MachineDeploymentTopology with
// the given name, or "" if the topology does not exist or no version is pinned.
func MachineDeploymentTopologyVersion(topology clusterv1.Topology, mdTopologyName string) string {
	for _, mdTopology := range topology.Workers.MachineDeployments {
		if mdTopology.Name == mdTopologyName {
			return mdTopology.Version
		}
	}
	return ""
}

// MachinePoolTopologyVersion returns the version pinned on the MachinePoolTopology with the given
// name, or "" if the topology does not exist or no version is pinned.
func MachinePoolTopologyVersion(topology clusterv1.Topology, mpTopologyName string) string {
	for _, mpTopology := range topology.Workers.MachinePools {
		if mpTopology.Name == mpTopologyName {
			return mpTopology.Version
		}
	}
	return ""
}
