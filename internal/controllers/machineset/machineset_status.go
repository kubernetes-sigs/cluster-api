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
	if s.machineSet.Status.V1Beta2 != nil {
		s.machineSet.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
	}

	// Update the following fields in status from the machines list.
	// - ReadyReplicas
	// - AvailableReplicas
	// Also updates the following conditions:
	// - ScalingUp
	// - ScalingDown
	setReplicas(s.machineSet, s.machines)

	// Conditions

	// Update the ScalingUp and ScalingDown condition, which requires the above setReplicas function.
	setScalingUpCondition(s.machineSet, s.machines)
	setScalingDownCondition(s.machineSet, s.machines)

	// MachinesReady
	setMachinesReadyCondition(s.machineSet, s.machines)

	// TODO MachinesUpToDate

	// TODO Deleting
}

func setReplicas(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	if machines == nil {
		// TODO(chrischdi) ask: what todo if listing machines failed? Not touch replicas?
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
	// TODO(chrischdi) ask: should we surface when scaling won't work due to missing bootstraptemplate or inframachinetemplate?

	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		// TODO(chrischdi) ask: should we only do this only if the condition does not exist or always?
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.InternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if ms.Spec.Replicas == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.InternalErrorV1Beta2Reason,
			Message: ".Spec.Replicas is nil, please check controller logs for errors",
		})
		return
	}
	desiredReplicas := *ms.Spec.Replicas

	// In case of deletion the desired replicas are 0.
	// TODO(chrischdi): what should ScalingUp report when deleting? Currently it would report "not scaling up"
	if !ms.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}

	// AvailableReplicas should always be set (this should never happen), surface if it is not the case.
	if ms.Status.V1Beta2.AvailableReplicas == nil {
		// Should never happen because AvailableReplicas get set in setReplicas and that should always be the case
		// when machines is not nil.
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.InternalErrorV1Beta2Reason,
			Message: "AvailableReplicas is expected to be set, please check controller logs for errors",
		})
		return
	}

	availableReplicas := *ms.Status.V1Beta2.AvailableReplicas

	// Not scaling up.
	if availableReplicas >= desiredReplicas {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetNotScalingUpReasonV1Beta2Reason,
			Message: "MachineSet is not scaling up",
		})
		return
	}

	// Scaling up.
	// TODO(chrischdi) ask: should we surface more and/or split this up? E.g.:
	// * summary of machines which are not available
	// * separate reasons for when machines need to get created vs only waiting for them to get available
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineSetNotAllAvailableReasonV1Beta2Reason, // Question: Should this maybe simply "ScalingUp" instead?
		Message: fmt.Sprintf("The MachineSet currently has %d/%d available replicas", availableReplicas, desiredReplicas),
	})
}

func setScalingDownCondition(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		// TODO(chrischdi) ask: should we only do this only if the condition does not exist or always?
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.InternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if ms.Spec.Replicas == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.InternalErrorV1Beta2Reason,
			Message: ".Spec.Replicas is nil, please check controller logs for errors",
		})
		return
	}
	desiredReplicas := *ms.Spec.Replicas

	// Deletion
	if !ms.DeletionTimestamp.IsZero() {
		// TODO(chrischdi) ask: surface more?
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachineSetDeletingReasonV1Beta2Reason,
			Message: "The MachineSet is in deletion",
		})
		return
	}

	// Not Scaling down
	if int32(len(machines)) <= (desiredReplicas) {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetNotScalingDownReasonV1Beta2Reason,
			Message: "MachineSet is not scaling down",
		})
		return
	}

	// Scaling down
	// TODO(chrischdi) ask: surface more? e.g.:
	// * x machines deleting and summarize their Deleting condition?
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineSetTooManyReplicasReasonV1Beta2Reason, // Question: Should this maybe simply "ScalingDown" instead?
		Message: fmt.Sprintf("The MachineSet currently has %d replicas but should only have %d", len(machines), desiredReplicas),
	})
}

func setMachinesReadyCondition(machineSet *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	// TODO(chrischdi) asks:
	// * what if scaling up (not all machines exist yet)
	// * what if scaling down (machines are getting removed)

	readyCondition, err := v1beta2conditions.NewAggregateCondition(
		machines, clusterv1.MachineReadyV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineSetMachinesReadyV1Beta2Condition),
	)
	if err != nil {
		v1beta2conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachineInvalidConditionReportedV1Beta2Reason,
			Message: err.Error(),
		})
	}

	// Overwrite the message for the true case.
	if readyCondition.Status == metav1.ConditionTrue {
		readyCondition.Message = "All Machines are ready."
	}

	v1beta2conditions.Set(machineSet, *readyCondition)
}
