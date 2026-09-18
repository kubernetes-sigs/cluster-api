/*
Copyright The Kubernetes Authors.

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

package mdutil

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/hooks"
)

// CheckOrCleanupAcknowledgeMove checks if a machine move to a new MachineSet has been acknowledged,
// and if yes, it cleans up machine's PendingAcknowledgeMoveAnnotation.
// Note. PendingAcknowledgeMoveAnnotation is also cleaned up when target MachineSet is not accepting anymore machines from other MS.
func CheckOrCleanupAcknowledgeMove(_ context.Context, ms *clusterv1.MachineSet, machine *clusterv1.Machine) bool {
	// If a machine is not updating in place, or if the in-place update has been already triggered, no-op
	if _, ok := machine.Annotations[clusterv1.UpdateInProgressAnnotation]; !ok || hooks.IsPending(runtimehooksv1.UpdateMachine, machine) {
		return false
	}

	if _, ok := machine.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; ok {
		// Check if this MachineSet is still accepting machines moved from other MachineSets.
		if sourceMSs, ok := ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok && sourceMSs != "" {
			// Get the list of machines acknowledged by the MD controller.
			acknowledgedMoveReplicas := sets.Set[string]{}
			if replicaNames, ok := ms.Annotations[clusterv1.AcknowledgedMoveAnnotation]; ok && replicaNames != "" {
				acknowledgedMoveReplicas.Insert(strings.Split(replicaNames, ",")...)
			}

			// If the current machine is in not yet in the list, it is not possible to trigger in-place yet.
			if !acknowledgedMoveReplicas.Has(machine.Name) {
				return false
			}

			// If the current machine is in the list, drop the annotation.
			delete(machine.Annotations, clusterv1.PendingAcknowledgeMoveAnnotation)
		} else {
			// If this MachineSet is not accepting anymore machines from other MS, e.g. because of after a MD spec
			// change this MS is not anymore the new MS, then drop the PendingAcknowledgeMove annotation.
			// This machine will be treated like any other machine on an old MS, and either deleted
			// or moved to another MS after completing the in-place update.
			delete(machine.Annotations, clusterv1.PendingAcknowledgeMoveAnnotation)
		}
	}

	return true
}

// SyncMachinesPlannerResult stores the result of SyncMachinesPlanner.
type SyncMachinesPlannerResult struct {
	MachineCreationDisabled        bool
	MachinesToAdd                  int
	MachinesPendingAcknowledgeMove []string
	MachinesToMove                 int
	MoveTargetMSName               string
	MoveAffectsAvailability        *bool
	MachinesToDelete               int
}

// SyncMachinesPlanner computes the plan of actions required to reconcile the list of machines controller by a MachineSet
// to its desired spec. Actions are defined in terms of numbers of machines to create, delete or move to another MachineSet.
func SyncMachinesPlanner(_ context.Context, ms *clusterv1.MachineSet, machines []*clusterv1.Machine) (SyncMachinesPlannerResult, error) {
	diff := len(machines) - int(ptr.Deref(ms.Spec.Replicas, 0))
	switch {
	case diff < 0:
		// If there are not enough Machines, create missing Machines unless Machine creation is disabled
		if ms.Annotations != nil {
			if value, ok := ms.Annotations[clusterv1.DisableMachineCreateAnnotation]; ok && value == "true" {
				return SyncMachinesPlannerResult{MachineCreationDisabled: true}, nil
			}
		}
		return SyncMachinesPlannerResult{MachinesToAdd: -diff}, nil

	case diff > 0:
		// if too many replicas, delete or move exceeding machines.

		// If the MachineSet is accepting replicas from other MachineSets (and thus this is the newMS controlled by a MD),
		// detect if there are replicas still pending AcknowledgedMove.
		// Note: replicas still pending AcknowledgeMove should not be counted when computing the numbers of machines to delete, because those machines are not included in ms.Spec.Replicas yet.
		// Without this check, the following logic would try to align the number of replicas to "an incomplete" ms.Spec.Replicas and as a consequence wrongly delete replicas that should be preserved.
		notAcknowledgeMoveReplicas := sets.Set[string]{}
		if sourceMSs, ok := ms.Annotations[clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation]; ok && sourceMSs != "" {
			for _, m := range machines {
				if _, ok := m.Annotations[clusterv1.PendingAcknowledgeMoveAnnotation]; !ok {
					continue
				}
				notAcknowledgeMoveReplicas.Insert(m.Name)
			}
		}

		res := SyncMachinesPlannerResult{}
		if notAcknowledgeMoveReplicas.Len() > 0 {
			res.MachinesPendingAcknowledgeMove = notAcknowledgeMoveReplicas.UnsortedList()
			slices.Sort(res.MachinesPendingAcknowledgeMove)
		}

		machinesToDeleteOrMove := max(len(machines)-notAcknowledgeMoveReplicas.Len()-int(ptr.Deref(ms.Spec.Replicas, 0)), 0)
		if machinesToDeleteOrMove == 0 {
			return res, nil
		}

		// Move machines to the target MachineSet if the current MachineSet is instructed to do so.
		if moveMachinesToMachineSetAnnotationValue, ok := ms.Annotations[clusterv1.MachineSetMoveMachinesToMachineSetAnnotation]; ok && moveMachinesToMachineSetAnnotationValue != "" {
			data := &clusterv1.MachineSetMoveMachinesToMachineSetAnnotationData{}
			// Note: it is required to use UnmarshalMoveMachinesToMachineSetAnnotationData instead of Unmarshal because the legacy format is an invalid JSON.
			if err := UnmarshalMoveMachinesToMachineSetAnnotationData([]byte(moveMachinesToMachineSetAnnotationValue), data); err != nil {
				return SyncMachinesPlannerResult{}, fmt.Errorf("failed to unmarshal %s annotation on %s: %w", clusterv1.MachineSetMoveMachinesToMachineSetAnnotation, ms.Name, err)
			}
			if data.Name != "" {
				// Note: The number of machines actually moved could be less than expected e.g. because some machine still updating in-place from a previous move.
				res.MachinesToMove = machinesToDeleteOrMove
				res.MoveTargetMSName = data.Name
				res.MoveAffectsAvailability = data.AffectsAvailability
				return res, nil
			}
		}

		// Otherwise the current MachineSet is not instructed to move machines to another MachineSet,
		// then delete all the exceeding machines.
		res.MachinesToDelete = machinesToDeleteOrMove
		return res, nil
	}

	return SyncMachinesPlannerResult{}, nil
}
