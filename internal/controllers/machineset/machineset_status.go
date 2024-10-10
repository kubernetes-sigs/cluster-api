/*
Copyright 2024 The Kubernetes Authors.

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

package machineset

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

// reconcileStatus reconciles Machine's status during the entire lifecycle of the machine.
// This implies that the code in this function should account for several edge cases e.g. machine being partially provisioned,
// machine being partially deleted but also for running machines being disrupted e.g. by deleting the node.
// Additionally, this func should ensure that the conditions managed by this controller are always set in order to
// comply with the recommendation in the Kubernetes API guidelines.
// Note: v1beta1 conditions are not managed by this func.
func (r *Reconciler) reconcileStatus(_ context.Context, s *scope) {
	// Update the following fields in status from the machines list.
	// - ReadyReplicas
	// - AvailableReplicas
	// Also updates the following conditions:
	// - ScalingUp
	// - ScalingDown
	setReplicas(s.machineSet, s.machines)
	setScalingUpCondition(s.machineSet, s.machines)
	setScalingDownCondition(s.machineSet, s.machines)

	// Conditions

	// MachinesReady
	if err := v1beta2conditions.SetAggregateCondition(s.machines, s.machineSet,
		clusterv1.MachineReadyV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineSetMachinesReadyV1Beta2Condition),
	); err != nil {
		v1beta2conditions.Set(s.machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  "TODOFailedDuringAggregation",
			Message: "something TODO at MachinesReady",
		})
	}

	// MachinesUpToDate
	if err := v1beta2conditions.SetAggregateCondition(s.machines, s.machineSet,
		clusterv1.MachinesUpToDateV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineSetMachinesUpToDateV1Beta2Condition),
	); err != nil {
		v1beta2conditions.Set(s.machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  "TODOFailedDuringAggregation",
			Message: "something TODO on MachinesUpToDate",
		})
	}

	// Paused
	// TODO(chrischdi): setPausedCondition(s.machineSet)

	// Deleting
	setDeletingCondition(s.machineSet)
}

func setReplicas(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	if machines == nil {
		// TODO: what about replicas fields? Not touch?!
		return
	}

	var readyReplicas, availableReplicas int32
	for _, machine := range machines {
		if meta.IsStatusConditionTrue(machine.GetV1Beta2Conditions(), clusterv1.MachineReadyV1Beta2Condition) {
			readyReplicas++
		}
		if meta.IsStatusConditionTrue(machine.GetV1Beta2Conditions(), clusterv1.MachineAvailableV1Beta2Condition) {
			availableReplicas++
		}
	}

	ms.Status.V1Beta2.ReadyReplicas = ptr.To(readyReplicas)
	ms.Status.V1Beta2.AvailableReplicas = ptr.To(availableReplicas)
}

func setScalingUpCondition(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	if machines == nil {
		// Question: should we only do this if the condition does not exist?
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  "InternalError", // TODO: create a const.
			Message: "Please check controller logs for errors",
		})
		return
	}

	desiredReplicas := *ms.Spec.Replicas
	availableReplicas := *ms.Status.V1Beta2.AvailableReplicas

	if availableReplicas < desiredReplicas {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  "NotAllAvailable", // TODO: create a const.
			Message: fmt.Sprintf("The MachineSet currently has %d/%d available replicas", availableReplicas, desiredReplicas),
		})
		return
	}

	v1beta2conditions.Set(ms, metav1.Condition{
		Type:   clusterv1.MachineSetScalingUpV1Beta2Condition,
		Status: metav1.ConditionFalse,
		Reason: "NotScalingUp", // TODO: create a const.
	})
}

func setScalingDownCondition(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	if machines == nil {
		// Question: should we only do this if the condition does not exist?
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  "InternalError", // TODO: create a const.
			Message: "Please check controller logs for errors",
		})

		// TODO: what about replicas fields? Not touch?
		return
	}

	desiredReplicas := *ms.Spec.Replicas

	if int32(len(machines)) <= (desiredReplicas) {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: "NotScalingDown", // TODO: create a const.
		})
		return
	}

	if !ms.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  "Deleting", // TODO: create a const.
			Message: "The MachineSet is in deletion",
		})
		return
	}
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  "TooManyReplicas", // TODO: create a const.
		Message: fmt.Sprintf("The MachineSet currently has %d replicas but should only have %d", len(machines), desiredReplicas),
	})
}

// func setPausedCondition(ms *clusterv1.MachineSet) {
// 	v1beta2conditions.Set(ms, metav1.Condition{
// 		Type:   clusterv1.MachineSetPausedV1Beta2Condition,
// 		Status: metav1.ConditionFalse,
// 		Reason: "NotPaused", // TODO: create a const.
// 	})
// }

func setDeletingCondition(ms *clusterv1.MachineSet) {
	if !ms.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:   clusterv1.MachineSetDeletingV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: "DeletionTimestampSet", // TODO: create a const.
		})
		return
	}
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:   clusterv1.MachineSetDeletingV1Beta2Condition,
		Status: metav1.ConditionFalse,
		Reason: "NoDeletionTimestamp", // TODO: create a const.
	})
}
