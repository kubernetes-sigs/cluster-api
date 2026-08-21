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
package pinning

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
)

// enabled returns true if the worker version pinning feature gate is enabled.
// All funcs in this package go through it so the gate is evaluated consistently.
func enabled() bool {
	return feature.Gates.Enabled(feature.ClusterTopologyWorkerVersionPinning)
}

// MachineDeploymentVersion returns the version pinned on the given MachineDeploymentTopology,
// or "" if the feature gate is disabled or no version is pinned.
func MachineDeploymentVersion(mdTopology clusterv1.MachineDeploymentTopology) string {
	if !enabled() {
		return ""
	}
	return mdTopology.Version
}

// MachinePoolVersion returns the version pinned on the given MachinePoolTopology,
// or "" if the feature gate is disabled or no version is pinned.
func MachinePoolVersion(mpTopology clusterv1.MachinePoolTopology) string {
	if !enabled() {
		return ""
	}
	return mpTopology.Version
}

// MachineDeploymentVersionByTopologyName returns the version pinned on the MachineDeploymentTopology
// with the given name, or "" if the feature gate is disabled, the topology does not exist or no
// version is pinned.
func MachineDeploymentVersionByTopologyName(topology clusterv1.Topology, mdTopologyName string) string {
	if !enabled() {
		return ""
	}
	for _, mdTopology := range topology.Workers.MachineDeployments {
		if mdTopology.Name == mdTopologyName {
			return mdTopology.Version
		}
	}
	return ""
}

// MachinePoolVersionByTopologyName returns the version pinned on the MachinePoolTopology with the
// given name, or "" if the feature gate is disabled, the topology does not exist or no version is
// pinned.
func MachinePoolVersionByTopologyName(topology clusterv1.Topology, mpTopologyName string) string {
	if !enabled() {
		return ""
	}
	for _, mpTopology := range topology.Workers.MachinePools {
		if mpTopology.Name == mpTopologyName {
			return mpTopology.Version
		}
	}
	return ""
}

// MachineDeploymentVersionByLabels returns the version pinned on the MachineDeploymentTopology
// owning the object with the given labels, or "" if the object is not owned by a topology
// MachineDeployment or no version is pinned.
// Labels are the labels of an object derived from a MachineDeployment, e.g. a MachineSet.
func MachineDeploymentVersionByLabels(cluster *clusterv1.Cluster, labels map[string]string) string {
	if !enabled() || cluster == nil || !cluster.Spec.Topology.IsDefined() {
		return ""
	}
	mdTopologyName, ok := labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel]
	if !ok {
		return ""
	}
	return MachineDeploymentVersionByTopologyName(cluster.Spec.Topology, mdTopologyName)
}

// MachinePoolVersionByLabels returns the version pinned on the MachinePoolTopology owning the
// object with the given labels, or "" if the object is not owned by a topology MachinePool or no
// version is pinned.
func MachinePoolVersionByLabels(cluster *clusterv1.Cluster, labels map[string]string) string {
	if !enabled() || cluster == nil || !cluster.Spec.Topology.IsDefined() {
		return ""
	}
	mpTopologyName, ok := labels[clusterv1.ClusterTopologyMachinePoolNameLabel]
	if !ok {
		return ""
	}
	return MachinePoolVersionByTopologyName(cluster.Spec.Topology, mpTopologyName)
}
