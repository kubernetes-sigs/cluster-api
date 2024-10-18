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
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	clog "sigs.k8s.io/cluster-api/util/log"
)

// reconcileV1Beta2Status reconciles Machine's status during the entire lifecycle of the machine.
// This implies that the code in this function should account for several edge cases e.g. machine being partially provisioned,
// machine being partially deleted but also for running machines being disrupted e.g. by deleting the node.
// Additionally, this func should ensure that the conditions managed by this controller are always set in order to
// comply with the recommendation in the Kubernetes API guidelines.
// Note: v1beta1 conditions are not managed by this func.
func (r *Reconciler) reconcileV1Beta2Status(_ context.Context, s *scope) {
	// Update the following fields in status from the machines list.
	// - replicas
	// - v1beta2.readyReplicas
	// - v1beta2.availableReplicas
	// Also records the names of machines which are in deletion for more than 30 minutes in s.staleDeletingMachines.
	setReplicas(s.machineSet, s.machines)

	// Conditions

	// Update the ScalingUp and ScalingDown condition, require the above setReplicas function.
	setScalingUpCondition(s.machineSet, s.machines, s.bootstrapObjectNotFound, s.infrastructureObjectNotFound)
	setScalingDownCondition(s.machineSet, s.machines, s.bootstrapObjectNotFound, s.infrastructureObjectNotFound, s.owningMachineDeployment)

	// MachinesReady condition: aggregate the Machine's Ready condition
	setMachinesReadyCondition(s.machineSet, s.machines)

	// MachinesUpToDate condition: aggregate the Machine's UpToDate condition
	setMachinesUpToDateCondition(s.machineSet, s.machines)

	// TODO Deleting
}

func setReplicas(ms *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	// Return early when machines is nil. It's not possible to calculate replica counters.
	// Conditions may surface to be unknown in this case.
	if machines == nil {
		return
	}

	var readyReplicas, availableReplicas int32
	for _, machine := range machines {
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineReadyV1Beta2Condition) {
			readyReplicas++
		}
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineAvailableV1Beta2Condition) {
			availableReplicas++
		}
	}

	if ms.Status.V1Beta2 == nil {
		ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
	}

	ms.Status.V1Beta2.ReadyReplicas = ptr.To(readyReplicas)
	ms.Status.V1Beta2.AvailableReplicas = ptr.To(availableReplicas)
}

func setScalingUpCondition(ms *clusterv1.MachineSet, machines []*clusterv1.Machine, bootstrapObjectNotFound, infrastructureObjectNotFound bool) {
	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingUpInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should happen rarely).
	// This could e.g. be the case when using autoscaling with MachineSets.
	if ms.Spec.Replicas == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:   clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status: metav1.ConditionUnknown,
			Reason: clusterv1.MachineSetScalingUpWaitingForReplicasToBeSetV1Beta2Reason,
		})
		return
	}

	desiredReplicas := *ms.Spec.Replicas
	if !ms.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	gotReplicas := ms.Status.Replicas

	referencesMessage := calculateMissingReferencesMessage(ms, bootstrapObjectNotFound, infrastructureObjectNotFound)

	// Not scaling up or in deletion.
	if gotReplicas >= desiredReplicas {
		var message string
		if referencesMessage != "" {
			message = fmt.Sprintf("ScalingUp can't happen %s", referencesMessage)
		}
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetNotScalingUpV1Beta2Reason,
			Message: message,
		})
		return
	}

	message := fmt.Sprintf("ScalingUp from %d to %d replicas", gotReplicas, desiredReplicas)
	if referencesMessage != "" {
		message = fmt.Sprintf("%s is blocked %s", message, referencesMessage)
	}
	// Scaling up.
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineSetScalingUpV1Beta2Reason,
		Message: message,
	})
}

func setScalingDownCondition(ms *clusterv1.MachineSet, machines []*clusterv1.Machine, bootstrapObjectNotFound, infrastructureObjectNotFound bool, owner *clusterv1.MachineDeployment) {
	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingDownInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should happen rarely).
	// This could e.g. be the case when using autoscaling with MachineSets.
	if ms.Spec.Replicas == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:   clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status: metav1.ConditionUnknown,
			Reason: clusterv1.MachineSetScalingDownWaitingForReplicasToBeSetV1Beta2Reason,
		})
		return
	}

	desiredReplicas := *ms.Spec.Replicas
	// Deletion is equal to 0 desired replicas.
	if !ms.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}

	// Scaling down.
	if int32(len(machines)) > (desiredReplicas) {
		messages := []string{fmt.Sprintf("ScalingDown from %d to %d replicas", len(machines), desiredReplicas)}

		machinesStaleDeleting := []string{}

		for _, machine := range machines {
			if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) >= time.Minute*30 {
				machinesStaleDeleting = append(machinesStaleDeleting, machine.GetName())
			}
		}

		if len(machinesStaleDeleting) > 0 {
			messages = append(messages, fmt.Sprintf("Machines stuck in deletion for more than 30 minutes: %s", clog.StringListToString(machinesStaleDeleting)))
		}

		// Scaling is blocked when the bootstrap or infrastructure objects do not exist.
		if bootstrapObjectNotFound || infrastructureObjectNotFound {
			messages[0] = fmt.Sprintf("%s is blocked %s", messages[0], calculateMissingReferencesMessage(ms, bootstrapObjectNotFound, infrastructureObjectNotFound))
		}

		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachineSetScalingDownV1Beta2Reason,
			Message: strings.Join(messages, ", "),
		})
		return
	}

	// Not scaling down.
	var message string
	// The referenced objects are required to exist for the current MachineSet.
	// Add a message if that's not the case.
	if isCurrentMachineSet(ms, owner) && (bootstrapObjectNotFound || infrastructureObjectNotFound) {
		message = fmt.Sprintf("ScalingDown can't happen %s", calculateMissingReferencesMessage(ms, bootstrapObjectNotFound, infrastructureObjectNotFound))
	}
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineSetNotScalingDownV1Beta2Reason,
		Message: message,
	})
}

func setMachinesReadyCondition(machineSet *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		v1beta2conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesReadyV1Beta2Condition,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machines) == 0 {
		v1beta2conditions.Set(machineSet, metav1.Condition{
			Type:   clusterv1.MachineSetMachinesReadyV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineSetMachinesReadyNoReplicasV1Beta2Reason,
		})
		return
	}

	readyCondition, err := v1beta2conditions.NewAggregateCondition(
		machines, clusterv1.MachineReadyV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineSetMachinesReadyV1Beta2Condition),
	)
	if err != nil {
		v1beta2conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesReadyInvalidConditionReportedV1Beta2Reason,
			Message: err.Error(),
		})
		return
	}

	v1beta2conditions.Set(machineSet, *readyCondition)
}

func setMachinesUpToDateCondition(machineSet *clusterv1.MachineSet, machines []*clusterv1.Machine) {
	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		v1beta2conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machines) == 0 {
		v1beta2conditions.Set(machineSet, metav1.Condition{
			Type:   clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineSetMachinesUpToDateNoReplicasV1Beta2Reason,
		})
		return
	}

	upToDateCondition, err := v1beta2conditions.NewAggregateCondition(
		machines, clusterv1.MachinesUpToDateV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineSetMachinesUpToDateV1Beta2Condition),
	)
	if err != nil {
		v1beta2conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesUpToDateV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesUpToDateInvalidConditionReportedV1Beta2Reason,
			Message: err.Error(),
		})
		return
	}

	v1beta2conditions.Set(machineSet, *upToDateCondition)
}

func calculateMissingReferencesMessage(ms *clusterv1.MachineSet, bootstrapTemplateNotFound, infraMachineTemplateNotFound bool) string {
	missingObjects := []string{}
	if bootstrapTemplateNotFound {
		missingObjects = append(missingObjects, ms.Spec.Template.Spec.Bootstrap.ConfigRef.Kind)
	}
	if infraMachineTemplateNotFound {
		missingObjects = append(missingObjects, ms.Spec.Template.Spec.InfrastructureRef.Kind)
	}

	if len(missingObjects) == 0 {
		return ""
	}

	if len(missingObjects) == 1 {
		return fmt.Sprintf("because %s does not exist", missingObjects[0])
	}

	return fmt.Sprintf("because %s do not exist", strings.Join(missingObjects, " and "))
}
