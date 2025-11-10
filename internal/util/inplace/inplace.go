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

// Package inplace provides utils for in-place updates.
package inplace

import (
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/hooks"
)

// IsUpdateInProgress returns true if an in-place update is in progress for one machine.
// Note: an in-place update is considered in progress even if technically it is still starting
//
//	(only the annotation is there, the hook is not yet there), or if it is stopping (only the
//	pending hook is still there, but the annotation is gone).
func IsUpdateInProgress(machine *clusterv1.Machine) bool {
	_, inPlaceUpdateInProgress := machine.Annotations[clusterv1.UpdateInProgressAnnotation]
	hasUpdateMachinePending := hooks.IsPending(runtimehooksv1.UpdateMachine, machine)

	return inPlaceUpdateInProgress || hasUpdateMachinePending
}

// CleanupMachineSpecForDiff cleans up a MachineSpec for diff.
// Note: Please update mdutil.MachineTemplateDeepCopyRolloutFields accordingly if necessary.
func CleanupMachineSpecForDiff(spec *clusterv1.MachineSpec) *clusterv1.MachineSpec {
	spec = spec.DeepCopy()

	// The following fields are set to their zero value so they are omitted from the comparison,
	// because they should never be the reason for an in-place update.

	// Should never change.
	spec.ClusterName = ""

	// Machine: should never change.
	// MachineSet: not responsibility of the in-place update extension.
	spec.Bootstrap = clusterv1.Bootstrap{}
	spec.InfrastructureRef = clusterv1.ContractVersionedObjectReference{}

	// Machine: should never change.
	// MachineSet: should not be set.
	spec.ProviderID = ""

	// Version & FailureDomain should be compared.

	// Fields that are mutated in-place without a rollout.
	spec.MinReadySeconds = nil
	spec.ReadinessGates = nil
	spec.Deletion.NodeDrainTimeoutSeconds = nil
	spec.Deletion.NodeVolumeDetachTimeoutSeconds = nil
	spec.Deletion.NodeDeletionTimeoutSeconds = nil

	return spec
}
