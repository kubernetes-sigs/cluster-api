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
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	clog "sigs.k8s.io/cluster-api/util/log"
)

// updateStatus updates MachineSet's status.
// Additionally, this func should ensure that the conditions managed by this controller are always set in order to
// comply with the recommendation in the Kubernetes API guidelines.
func (r *Reconciler) updateStatus(ctx context.Context, s *scope) error {
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	selector, err := metav1.LabelSelectorAsSelector(&s.machineSet.Spec.Selector)
	if err != nil {
		return errors.Wrapf(err, "failed to update status for MachineSet %s", klog.KObj(s.machineSet))
	}
	s.machineSet.Status.Selector = selector.String()

	// Update replica counter fields in status from the machines list.
	setReplicas(ctx, s.machineSet, s.machines, s.getAndAdoptMachinesForMachineSetSucceeded)

	// Conditions

	// Update the ScalingUp and ScalingDown condition.
	setScalingUpCondition(ctx, s.machineSet, s.machines, s.bootstrapObjectNotFound, s.infrastructureObjectNotFound, s.getAndAdoptMachinesForMachineSetSucceeded, s.scaleUpPreflightCheckErrMessages)
	setScalingDownCondition(ctx, s.machineSet, s.machines, s.getAndAdoptMachinesForMachineSetSucceeded)

	// MachinesReady condition: aggregate the Machine's Ready condition.
	setMachinesReadyCondition(ctx, s.machineSet, s.machines, s.getAndAdoptMachinesForMachineSetSucceeded)

	// MachinesUpToDate condition: aggregate the Machine's UpToDate condition.
	setMachinesUpToDateCondition(ctx, s.machineSet, s.machines, s.getAndAdoptMachinesForMachineSetSucceeded)

	machines := collections.FromMachines(s.machines...)
	machinesToBeRemediated := machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
	unhealthyMachines := machines.Filter(collections.IsUnhealthy)
	setRemediatingCondition(ctx, s.machineSet, machinesToBeRemediated, unhealthyMachines, s.getAndAdoptMachinesForMachineSetSucceeded)

	setDeletingCondition(ctx, s.machineSet, s.machines, s.getAndAdoptMachinesForMachineSetSucceeded)

	return nil
}

func setReplicas(_ context.Context, ms *clusterv1.MachineSet, machines []*clusterv1.Machine, getAndAdoptMachinesForMachineSetSucceeded bool) {
	// Return early when getAndAdoptMachinesForMachineSetSucceeded is false because it's not possible to calculate replica counters.
	if !getAndAdoptMachinesForMachineSetSucceeded {
		return
	}

	var readyReplicas, availableReplicas, upToDateReplicas int32
	for _, machine := range machines {
		if conditions.IsTrue(machine, clusterv1.MachineReadyCondition) {
			readyReplicas++
		}
		if conditions.IsTrue(machine, clusterv1.MachineAvailableCondition) {
			availableReplicas++
		}
		if conditions.IsTrue(machine, clusterv1.MachineUpToDateCondition) {
			upToDateReplicas++
		}
	}

	ms.Status.Replicas = ptr.To(int32(len(machines)))
	ms.Status.ReadyReplicas = ptr.To(readyReplicas)
	ms.Status.AvailableReplicas = ptr.To(availableReplicas)
	ms.Status.UpToDateReplicas = ptr.To(upToDateReplicas)
}

func setScalingUpCondition(_ context.Context, ms *clusterv1.MachineSet, machines []*clusterv1.Machine, bootstrapObjectNotFound, infrastructureObjectNotFound, getAndAdoptMachinesForMachineSetSucceeded bool, scaleUpPreflightCheckErrMessages []string) {
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getAndAdoptMachinesForMachineSetSucceeded {
		conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingUpInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should never happen).
	if ms.Spec.Replicas == nil {
		conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingUpWaitingForReplicasSetReason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	desiredReplicas := *ms.Spec.Replicas
	if !ms.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	currentReplicas := int32(len(machines))

	missingReferencesMessage := calculateMissingReferencesMessage(ms, bootstrapObjectNotFound, infrastructureObjectNotFound)

	if currentReplicas >= desiredReplicas {
		var message string
		// Only surface this message if the MachineSet is not deleting.
		if ms.DeletionTimestamp.IsZero() && missingReferencesMessage != "" {
			message = fmt.Sprintf("Scaling up would be blocked because %s", missingReferencesMessage)
		}
		conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingUpCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetNotScalingUpReason,
			Message: message,
		})
		return
	}

	// Scaling up.
	message := fmt.Sprintf("Scaling up from %d to %d replicas", currentReplicas, desiredReplicas)
	if missingReferencesMessage != "" || len(scaleUpPreflightCheckErrMessages) > 0 {
		listMessages := make([]string, len(scaleUpPreflightCheckErrMessages))
		for i, msg := range scaleUpPreflightCheckErrMessages {
			listMessages[i] = fmt.Sprintf("* %s", msg)
		}
		if missingReferencesMessage != "" {
			listMessages = append(listMessages, fmt.Sprintf("* %s", missingReferencesMessage))
		}
		message += fmt.Sprintf(" is blocked because:\n%s", strings.Join(listMessages, "\n"))
	}
	conditions.Set(ms, metav1.Condition{
		Type:    clusterv1.MachineSetScalingUpCondition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineSetScalingUpReason,
		Message: message,
	})
}

func setScalingDownCondition(_ context.Context, ms *clusterv1.MachineSet, machines []*clusterv1.Machine, getAndAdoptMachinesForMachineSetSucceeded bool) {
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getAndAdoptMachinesForMachineSetSucceeded {
		conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingDownInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should never happen).
	if ms.Spec.Replicas == nil {
		conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetScalingDownWaitingForReplicasSetReason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	desiredReplicas := *ms.Spec.Replicas
	if !ms.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	currentReplicas := int32(len(machines))

	// Scaling down.
	if currentReplicas > desiredReplicas {
		message := fmt.Sprintf("Scaling down from %d to %d replicas", currentReplicas, desiredReplicas)
		staleMessage := aggregateStaleMachines(machines)
		if staleMessage != "" {
			message += fmt.Sprintf("\n* %s", staleMessage)
		}
		conditions.Set(ms, metav1.Condition{
			Type:    clusterv1.MachineSetScalingDownCondition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachineSetScalingDownReason,
			Message: message,
		})
		return
	}

	// Not scaling down.
	conditions.Set(ms, metav1.Condition{
		Type:   clusterv1.MachineSetScalingDownCondition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.MachineSetNotScalingDownReason,
	})
}

func setMachinesReadyCondition(ctx context.Context, machineSet *clusterv1.MachineSet, machines []*clusterv1.Machine, getAndAdoptMachinesForMachineSetSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getAndAdoptMachinesForMachineSetSucceeded {
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesReadyInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machines) == 0 {
		conditions.Set(machineSet, metav1.Condition{
			Type:   clusterv1.MachineSetMachinesReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineSetMachinesReadyNoReplicasReason,
		})
		return
	}

	readyCondition, err := conditions.NewAggregateCondition(
		machines, clusterv1.MachineReadyCondition,
		conditions.TargetConditionType(clusterv1.MachineSetMachinesReadyCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.MachineSetMachinesNotReadyReason,
					clusterv1.MachineSetMachinesReadyUnknownReason,
					clusterv1.MachineSetMachinesReadyReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate Machine's Ready conditions")
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesReadyInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(machineSet, *readyCondition)
}

func setMachinesUpToDateCondition(ctx context.Context, machineSet *clusterv1.MachineSet, machinesSlice []*clusterv1.Machine, getAndAdoptMachinesForMachineSetSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getAndAdoptMachinesForMachineSetSucceeded {
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesUpToDateCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesUpToDateInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	machines := collections.FromMachines(machinesSlice...).Filter(func(machine *clusterv1.Machine) bool {
		return conditions.Has(machine, clusterv1.MachineUpToDateCondition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second
	})

	if len(machines) == 0 {
		conditions.Set(machineSet, metav1.Condition{
			Type:   clusterv1.MachineSetMachinesUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineSetMachinesUpToDateNoReplicasReason,
		})
		return
	}

	upToDateCondition, err := conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineUpToDateCondition,
		conditions.TargetConditionType(clusterv1.MachineSetMachinesUpToDateCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.MachineSetMachinesNotUpToDateReason,
					clusterv1.MachineSetMachinesUpToDateUnknownReason,
					clusterv1.MachineSetMachinesUpToDateReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate Machine's UpToDate conditions")
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetMachinesUpToDateCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetMachinesUpToDateInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(machineSet, *upToDateCondition)
}

func setRemediatingCondition(ctx context.Context, machineSet *clusterv1.MachineSet, machinesToBeRemediated, unhealthyMachines collections.Machines, getAndAdoptMachinesForMachineSetSucceeded bool) {
	if !getAndAdoptMachinesForMachineSetSucceeded {
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetRemediatingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetRemediatingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machinesToBeRemediated) == 0 {
		message := aggregateUnhealthyMachines(unhealthyMachines)
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetRemediatingCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineSetNotRemediatingReason,
			Message: message,
		})
		return
	}

	remediatingCondition, err := conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedCondition,
		conditions.TargetConditionType(clusterv1.MachineSetRemediatingCondition),
		// Note: in case of the remediating conditions it is not required to use a CustomMergeStrategy/ComputeReasonFunc
		// because we are considering only machinesToBeRemediated (and we can pin the reason when we set the condition).
	)
	if err != nil {
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetRemediatingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetRemediatingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineOwnerRemediatedCondition))
		return
	}

	conditions.Set(machineSet, metav1.Condition{
		Type:    remediatingCondition.Type,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineSetRemediatingReason,
		Message: remediatingCondition.Message,
	})
}

func setDeletingCondition(_ context.Context, machineSet *clusterv1.MachineSet, machines []*clusterv1.Machine, getAndAdoptMachinesForMachineSetSucceeded bool) {
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getAndAdoptMachinesForMachineSetSucceeded {
		conditions.Set(machineSet, metav1.Condition{
			Type:    clusterv1.MachineSetDeletingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineSetDeletingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if machineSet.DeletionTimestamp.IsZero() {
		conditions.Set(machineSet, metav1.Condition{
			Type:   clusterv1.MachineSetDeletingCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineSetNotDeletingReason,
		})
		return
	}

	message := ""
	if len(machines) > 0 {
		if len(machines) == 1 {
			message = fmt.Sprintf("Deleting %d Machine", len(machines))
		} else {
			message = fmt.Sprintf("Deleting %d Machines", len(machines))
		}
		staleMessage := aggregateStaleMachines(machines)
		if staleMessage != "" {
			message += fmt.Sprintf("\n* %s", staleMessage)
		}
	}
	if message == "" {
		message = "Deletion completed"
	}
	conditions.Set(machineSet, metav1.Condition{
		Type:    clusterv1.MachineSetDeletingCondition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineSetDeletingReason,
		Message: message,
	})
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
		return fmt.Sprintf("%s does not exist", missingObjects[0])
	}

	return fmt.Sprintf("%s do not exist", strings.Join(missingObjects, " and "))
}

func aggregateStaleMachines(machines []*clusterv1.Machine) string {
	if len(machines) == 0 {
		return ""
	}

	machineNames := []string{}
	delayReasons := sets.Set[string]{}
	for _, machine := range machines {
		if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) > time.Minute*15 {
			machineNames = append(machineNames, machine.GetName())

			deletingCondition := conditions.Get(machine, clusterv1.MachineDeletingCondition)
			if deletingCondition != nil &&
				deletingCondition.Status == metav1.ConditionTrue &&
				deletingCondition.Reason == clusterv1.MachineDeletingDrainingNodeReason &&
				machine.Status.Deletion != nil &&
				!machine.Status.Deletion.NodeDrainStartTime.Time.IsZero() &&
				time.Since(machine.Status.Deletion.NodeDrainStartTime.Time) > 5*time.Minute {
				if strings.Contains(deletingCondition.Message, "cannot evict pod as it would violate the pod's disruption budget.") {
					delayReasons.Insert("PodDisruptionBudgets")
				}
				if strings.Contains(deletingCondition.Message, "deletionTimestamp set, but still not removed from the Node") {
					delayReasons.Insert("Pods not terminating")
				}
				if strings.Contains(deletingCondition.Message, "failed to evict Pod") {
					delayReasons.Insert("Pod eviction errors")
				}
				if strings.Contains(deletingCondition.Message, "waiting for completion") {
					delayReasons.Insert("Pods not completed yet")
				}
			}
		}
	}

	if len(machineNames) == 0 {
		return ""
	}

	message := "Machine"
	if len(machineNames) > 1 {
		message += "s"
	}

	sort.Strings(machineNames)
	message += " " + clog.ListToString(machineNames, func(s string) string { return s }, 3)

	if len(machineNames) == 1 {
		message += " is "
	} else {
		message += " are "
	}
	message += "in deletion since more than 15m"
	if len(delayReasons) > 0 {
		reasonList := []string{}
		for _, r := range []string{"PodDisruptionBudgets", "Pods not terminating", "Pod eviction errors", "Pods not completed yet"} {
			if delayReasons.Has(r) {
				reasonList = append(reasonList, r)
			}
		}
		message += fmt.Sprintf(", delay likely due to %s", strings.Join(reasonList, ", "))
	}

	return message
}

func aggregateUnhealthyMachines(machines collections.Machines) string {
	if len(machines) == 0 {
		return ""
	}

	machineNames := machines.Names()

	if len(machineNames) == 0 {
		return ""
	}

	message := "Machine"
	if len(machineNames) > 1 {
		message += "s"
	}

	sort.Strings(machineNames)
	message += " " + clog.ListToString(machineNames, func(s string) string { return s }, 3)

	if len(machineNames) == 1 {
		message += " is "
	} else {
		message += " are "
	}
	message += "not healthy (not to be remediated by MachineSet)"

	return message
}
