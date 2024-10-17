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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	clog "sigs.k8s.io/cluster-api/util/log"
)

// reconcileStatus reconciles Machine's status during the entire lifecycle of the machine.
// This implies that the code in this function should account for several edge cases e.g. machine being partially provisioned,
// machine being partially deleted but also for running machines being disrupted e.g. by deleting the node.
// Additionally, this func should ensure that the conditions managed by this controller are always set in order to
// comply with the recommendation in the Kubernetes API guidelines.
// Note: v1beta1 conditions are not managed by this func.
func (r *Reconciler) reconcileStatus(_ context.Context, s *scope) {
	// Update the following fields in status from the machines list.
	// - replicas
	// - v1beta2.readyReplicas
	// - v1beta2.availableReplicas
	// Also records the names of machines which are in deletion for more than 30 minutes in s.staleDeletingMachines.
	setReplicas(s)

	// Conditions

	// Update the ScalingUp and ScalingDown condition, require the above setReplicas function.
	setScalingUpCondition(s.machineSet, s.machines, s.bootstrapObjectNotFound, s.infrastructureObjectNotFound)
	setScalingDownCondition(s.machineSet, s.machines, s.bootstrapObjectNotFound, s.infrastructureObjectNotFound, s.owningMachineDeployment, s.staleDeletingMachines)

	// MachinesReady condition: aggregate the Machine's Ready condition
	setMachinesReadyCondition(s.machineSet, s.machines)

	// TODO MachinesUpToDate

	// TODO Deleting
}

func setReplicas(s *scope) {
	ms := s.machineSet
	machines := s.machines
	// Return early when machines is nil. It's not possible to calculate replica counters.
	// Conditions may surface to be unknown in this case.
	if machines == nil {
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
		if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) >= time.Minute*30 {
			s.staleDeletingMachines = append(s.staleDeletingMachines, machine.GetName())
		}
	}

	if ms.Status.V1Beta2 == nil {
		ms.Status.V1Beta2 = &clusterv1.MachineSetV1Beta2Status{}
	}

	ms.Status.Replicas = int32(len(machines))
	ms.Status.V1Beta2.ReadyReplicas = ptr.To(readyReplicas)
	ms.Status.V1Beta2.AvailableReplicas = ptr.To(availableReplicas)
}

func setScalingUpCondition(ms *clusterv1.MachineSet, machines []*clusterv1.Machine, bootstrapObjectNotFound, infrastructureObjectNotFound bool) {
	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingUpErrorV1Beta2Reason,
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
			Reason: clusterv1.MachineSetWaitingForReplicasToBeSetV1Beta2Reason,
		})
		return
	}

	desiredReplicas := *ms.Spec.Replicas
	gotReplicas := ms.Status.Replicas

	referencesMessage := calculateMissingReferencesMessage(bootstrapObjectNotFound, infrastructureObjectNotFound)

	// Not scaling up or in deletion.
	if gotReplicas >= desiredReplicas || !ms.DeletionTimestamp.IsZero() {
		var message string
		if referencesMessage != "" {
			message = fmt.Sprintf("The MachineSet is not scaling up but scaling up would fail because %s", referencesMessage)
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
		message = fmt.Sprintf("%s is blocked because %s", message, referencesMessage)
	}
	// Scaling up.
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingUpV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineSetScalingUpV1Beta2Reason,
		Message: message,
	})
}

func setScalingDownCondition(ms *clusterv1.MachineSet, machines []*clusterv1.Machine, bootstrapObjectNotFound, infrastructureObjectNotFound bool, owner *clusterv1.MachineDeployment, machinesStaleDeleting []string) {
	// If we got unexpected errors in listing the machines (this should happen rarely), surface them
	if machines == nil {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingDownErrorV1Beta2Reason,
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
			Reason: clusterv1.MachineSetWaitingForReplicasToBeSetV1Beta2Reason,
		})
		return
	}

	desiredReplicas := *ms.Spec.Replicas

	// Deletion is equal to 0 desired replicas.
	if !ms.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachineSetScalingDownV1Beta2Reason,
			Message: "MachineSet is getting deleted",
		})
		return
	}

	// Scaling down.
	if int32(len(machines)) > (desiredReplicas) {
		messages := []string{fmt.Sprintf("ScalingDown from %d to %d replicas", len(machines), desiredReplicas)}

		if len(machinesStaleDeleting) > 0 {
			messages = append(messages, fmt.Sprintf("Machines stuck in deletion for more than 30 minutes: %s", clog.StringListToString(machinesStaleDeleting)))
		}

		// Scaling is blocked when the bootstrap or infrastructure objects do not exist.
		if bootstrapObjectNotFound || infrastructureObjectNotFound {
			messages[0] = fmt.Sprintf("%s is blocked because %s", messages[0], calculateMissingReferencesMessage(bootstrapObjectNotFound, infrastructureObjectNotFound))
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
		message = fmt.Sprintf("ScalingDown would fail because %s", calculateMissingReferencesMessage(bootstrapObjectNotFound, infrastructureObjectNotFound))
	}
	v1beta2conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingDownV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineSetNotScalingDownV1Beta2Reason,
		Message: message,
	})
}

func setMachinesReadyCondition(machineSet *clusterv1.MachineSet, machines []*clusterv1.Machine) {
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
		return
	}

	// Overwrite the message for the true case.
	if readyCondition.Status == metav1.ConditionTrue {
		readyCondition.Message = "All Machines are ready."
	}

	v1beta2conditions.Set(machineSet, *readyCondition)
}

func calculateMissingReferencesMessage(bootstrapTemplateNotFound, infraMachineTemplateNotFound bool) string {
	missingPaths := []string{}
	if bootstrapTemplateNotFound {
		missingPaths = append(missingPaths, ".spec.template.spec.bootstrap.configRef")
	}
	if infraMachineTemplateNotFound {
		missingPaths = append(missingPaths, ".spec.template.spec.infrastructureRef")
	}

	if len(missingPaths) == 0 {
		return ""
	}

	if len(missingPaths) == 1 {
		return fmt.Sprintf("the object referenced at %s does not exist", missingPaths[0])
	}

	return fmt.Sprintf("the objects referenced at %s do not exist", strings.Join(missingPaths, " and "))
}
