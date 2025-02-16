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

package machinedeployment

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	clog "sigs.k8s.io/cluster-api/util/log"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) (retErr error) {
	// Get all Machines controlled by this MachineDeployment.
	var machines, machinesToBeRemediated, unhealthyMachines collections.Machines
	var getMachinesSucceeded bool
	if selectorMap, err := metav1.LabelSelectorAsMap(&s.machineDeployment.Spec.Selector); err == nil {
		machineList := &clusterv1.MachineList{}
		if err := r.Client.List(ctx, machineList, client.InNamespace(s.machineDeployment.Namespace), client.MatchingLabels(selectorMap)); err != nil {
			retErr = errors.Wrap(err, "failed to list machines")
		} else {
			getMachinesSucceeded = true
			machines = collections.FromMachineList(machineList)
			machinesToBeRemediated = machines.Filter(collections.IsUnhealthyAndOwnerRemediated)
			unhealthyMachines = machines.Filter(collections.IsUnhealthy)
		}
	} else {
		retErr = errors.Wrap(err, "failed to convert label selector to a map")
	}

	// If the controller could read MachineSets, update replica counters.
	if s.getAndAdoptMachineSetsForDeploymentSucceeded {
		setReplicas(s.machineDeployment, s.machineSets)
	}

	setAvailableCondition(ctx, s.machineDeployment, s.getAndAdoptMachineSetsForDeploymentSucceeded)

	setRollingOutCondition(ctx, s.machineDeployment, machines, getMachinesSucceeded)
	setScalingUpCondition(ctx, s.machineDeployment, s.machineSets, s.bootstrapTemplateNotFound, s.infrastructureTemplateNotFound, s.getAndAdoptMachineSetsForDeploymentSucceeded)
	setScalingDownCondition(ctx, s.machineDeployment, s.machineSets, machines, s.getAndAdoptMachineSetsForDeploymentSucceeded, getMachinesSucceeded)

	setMachinesReadyCondition(ctx, s.machineDeployment, machines, getMachinesSucceeded)
	setMachinesUpToDateCondition(ctx, s.machineDeployment, machines, getMachinesSucceeded)

	setRemediatingCondition(ctx, s.machineDeployment, machinesToBeRemediated, unhealthyMachines, getMachinesSucceeded)

	setDeletingCondition(ctx, s.machineDeployment, s.machineSets, machines, s.getAndAdoptMachineSetsForDeploymentSucceeded, getMachinesSucceeded)

	return retErr
}

// setReplicas sets replicas in the v1beta2 status.
// Note: this controller computes replicas several time during a reconcile, because those counters are
// used by low level operations to take decisions, but also those decisions might impact the very same the counters
// e.g. scale up MachinesSet is based on counters and it can change the value on MachineSet's replica number;
// as a consequence it is required to compute the counters again before calling scale down machine sets,
// and again to before computing the overall availability of the Machine deployment.
func setReplicas(machineDeployment *clusterv1.MachineDeployment, machineSets []*clusterv1.MachineSet) {
	if machineDeployment.Status.V1Beta2 == nil {
		machineDeployment.Status.V1Beta2 = &clusterv1.MachineDeploymentV1Beta2Status{}
	}

	machineDeployment.Status.V1Beta2.ReadyReplicas = mdutil.GetV1Beta2ReadyReplicaCountForMachineSets(machineSets)
	machineDeployment.Status.V1Beta2.AvailableReplicas = mdutil.GetV1Beta2AvailableReplicaCountForMachineSets(machineSets)
	machineDeployment.Status.V1Beta2.UpToDateReplicas = mdutil.GetV1Beta2UptoDateReplicaCountForMachineSets(machineSets)
}

func setAvailableCondition(_ context.Context, machineDeployment *clusterv1.MachineDeployment, getAndAdoptMachineSetsForDeploymentSucceeded bool) {
	// If we got unexpected errors in listing the machine sets (this should never happen), surface them.
	if !getAndAdoptMachineSetsForDeploymentSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentAvailableInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should never happen).
	if machineDeployment.Spec.Replicas == nil {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentAvailableWaitingForReplicasSetV1Beta2Reason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	// Surface if .status.v1beta2.availableReplicas is not yet set.
	if machineDeployment.Status.V1Beta2 == nil || machineDeployment.Status.V1Beta2.AvailableReplicas == nil {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentAvailableWaitingForAvailableReplicasSetV1Beta2Reason,
			Message: "Waiting for status.v1beta2.availableReplicas set",
		})
		return
	}

	// minReplicasNeeded will be equal to md.Spec.Replicas when the strategy is not RollingUpdateMachineDeploymentStrategyType.
	minReplicasNeeded := *(machineDeployment.Spec.Replicas) - mdutil.MaxUnavailable(*machineDeployment)

	if *machineDeployment.Status.V1Beta2.AvailableReplicas >= minReplicasNeeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:   clusterv1.MachineDeploymentAvailableV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineDeploymentAvailableV1Beta2Reason,
		})
		return
	}

	message := fmt.Sprintf("%d available replicas, at least %d required", *machineDeployment.Status.V1Beta2.AvailableReplicas, minReplicasNeeded)
	if machineDeployment.Spec.Strategy != nil && mdutil.IsRollingUpdate(machineDeployment) && machineDeployment.Spec.Strategy.RollingUpdate != nil {
		message += fmt.Sprintf(" (spec.strategy.rollout.maxUnavailable is %s, spec.replicas is %d)", machineDeployment.Spec.Strategy.RollingUpdate.MaxUnavailable, *machineDeployment.Spec.Replicas)
	}

	if !machineDeployment.DeletionTimestamp.IsZero() {
		message = "Deletion in progress"
	}
	v1beta2conditions.Set(machineDeployment, metav1.Condition{
		Type:    clusterv1.MachineDeploymentAvailableV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineDeploymentNotAvailableV1Beta2Reason,
		Message: message,
	})
}

func setRollingOutCondition(_ context.Context, machineDeployment *clusterv1.MachineDeployment, machines collections.Machines, getMachinesSucceeded bool) {
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getMachinesSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentRollingOutInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Count machines rolling out and collect reasons why a rollout is happening.
	// Note: The code below collects all the reasons for which at least a machine is rolling out; under normal circumstances
	// all the machines are rolling out for the same reasons, however, in case of changes to
	// the MD before a previous changes is not fully rolled out, there could be machines rolling out for
	// different reasons.
	rollingOutReplicas := 0
	rolloutReasons := sets.Set[string]{}
	for _, machine := range machines {
		upToDateCondition := v1beta2conditions.Get(machine, clusterv1.MachineUpToDateV1Beta2Condition)
		if upToDateCondition == nil || upToDateCondition.Status != metav1.ConditionFalse {
			continue
		}
		rollingOutReplicas++
		if upToDateCondition.Message != "" {
			rolloutReasons.Insert(strings.Split(upToDateCondition.Message, "\n")...)
		}
	}

	if rollingOutReplicas == 0 {
		var message string
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineDeploymentNotRollingOutV1Beta2Reason,
			Message: message,
		})
		return
	}

	// Rolling out.
	message := fmt.Sprintf("Rolling out %d not up-to-date replicas", rollingOutReplicas)
	if rolloutReasons.Len() > 0 {
		// Surface rollout reasons ensuring that if there is a version change, it goes first.
		reasons := rolloutReasons.UnsortedList()
		sort.Slice(reasons, func(i, j int) bool {
			if strings.HasPrefix(reasons[i], "* Version") && !strings.HasPrefix(reasons[j], "* Version") {
				return true
			}
			if !strings.HasPrefix(reasons[i], "* Version") && strings.HasPrefix(reasons[j], "* Version") {
				return false
			}
			return reasons[i] < reasons[j]
		})
		message += fmt.Sprintf("\n%s", strings.Join(reasons, "\n"))
	}
	v1beta2conditions.Set(machineDeployment, metav1.Condition{
		Type:    clusterv1.MachineDeploymentRollingOutV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineDeploymentRollingOutV1Beta2Reason,
		Message: message,
	})
}

func setScalingUpCondition(_ context.Context, machineDeployment *clusterv1.MachineDeployment, machineSets []*clusterv1.MachineSet, bootstrapObjectNotFound, infrastructureObjectNotFound, getAndAdoptMachineSetsForDeploymentSucceeded bool) {
	// If we got unexpected errors in listing the machine sets (this should never happen), surface them.
	if !getAndAdoptMachineSetsForDeploymentSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentScalingUpInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should never happen).
	if machineDeployment.Spec.Replicas == nil {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentScalingUpWaitingForReplicasSetV1Beta2Reason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	desiredReplicas := *machineDeployment.Spec.Replicas
	if !machineDeployment.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	currentReplicas := mdutil.GetActualReplicaCountForMachineSets(machineSets)

	missingReferencesMessage := calculateMissingReferencesMessage(machineDeployment, bootstrapObjectNotFound, infrastructureObjectNotFound)

	if currentReplicas >= desiredReplicas {
		var message string
		// Only surface this message if the MachineDeployment is not deleting.
		if machineDeployment.DeletionTimestamp.IsZero() && missingReferencesMessage != "" {
			message = fmt.Sprintf("Scaling up would be blocked %s", missingReferencesMessage)
		}
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineDeploymentNotScalingUpV1Beta2Reason,
			Message: message,
		})
		return
	}

	// Scaling up.
	message := fmt.Sprintf("Scaling up from %d to %d replicas", currentReplicas, desiredReplicas)
	if missingReferencesMessage != "" {
		message += fmt.Sprintf(" is blocked %s", missingReferencesMessage)
	}
	v1beta2conditions.Set(machineDeployment, metav1.Condition{
		Type:    clusterv1.MachineDeploymentScalingUpV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineDeploymentScalingUpV1Beta2Reason,
		Message: message,
	})
}

func setScalingDownCondition(_ context.Context, machineDeployment *clusterv1.MachineDeployment, machineSets []*clusterv1.MachineSet, machines collections.Machines, getAndAdoptMachineSetsForDeploymentSucceeded, getMachinesSucceeded bool) {
	// If we got unexpected errors in listing the machines sets (this should never happen), surface them.
	if !getAndAdoptMachineSetsForDeploymentSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentScalingDownInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should never happen).
	if machineDeployment.Spec.Replicas == nil {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentScalingDownWaitingForReplicasSetV1Beta2Reason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	desiredReplicas := *machineDeployment.Spec.Replicas
	if !machineDeployment.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	currentReplicas := mdutil.GetActualReplicaCountForMachineSets(machineSets)

	// Scaling down.
	if currentReplicas > desiredReplicas {
		message := fmt.Sprintf("Scaling down from %d to %d replicas", currentReplicas, desiredReplicas)
		if getMachinesSucceeded {
			staleMessage := aggregateStaleMachines(machines)
			if staleMessage != "" {
				message += fmt.Sprintf("\n* %s", staleMessage)
			}
		}
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachineDeploymentScalingDownV1Beta2Reason,
			Message: message,
		})
		return
	}

	// Not scaling down.
	v1beta2conditions.Set(machineDeployment, metav1.Condition{
		Type:   clusterv1.MachineDeploymentScalingDownV1Beta2Condition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.MachineDeploymentNotScalingDownV1Beta2Reason,
	})
}

func setMachinesReadyCondition(ctx context.Context, machineDeployment *clusterv1.MachineDeployment, machines collections.Machines, getMachinesSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getMachinesSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentMachinesReadyInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machines) == 0 {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:   clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineDeploymentMachinesReadyNoReplicasV1Beta2Reason,
		})
		return
	}

	readyCondition, err := v1beta2conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineReadyV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition),
		// Using a custom merge strategy to override reasons applied during merge.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.MachineDeploymentMachinesNotReadyV1Beta2Reason,
					clusterv1.MachineDeploymentMachinesReadyUnknownV1Beta2Reason,
					clusterv1.MachineDeploymentMachinesReadyV1Beta2Reason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate Machine's Ready conditions")
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentMachinesReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentMachinesReadyInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	v1beta2conditions.Set(machineDeployment, *readyCondition)
}

func setMachinesUpToDateCondition(ctx context.Context, machineDeployment *clusterv1.MachineDeployment, machines collections.Machines, getMachinesSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)
	// If we got unexpected errors in listing the machines (this should never happen), surface them.
	if !getMachinesSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentMachinesUpToDateInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	machines = machines.Filter(func(machine *clusterv1.Machine) bool {
		return v1beta2conditions.Has(machine, clusterv1.MachineUpToDateV1Beta2Condition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second
	})

	if len(machines) == 0 {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:   clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineDeploymentMachinesUpToDateNoReplicasV1Beta2Reason,
		})
		return
	}

	upToDateCondition, err := v1beta2conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineUpToDateV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition),
		// Using a custom merge strategy to override reasons applied during merge.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.MachineDeploymentMachinesNotUpToDateV1Beta2Reason,
					clusterv1.MachineDeploymentMachinesUpToDateUnknownV1Beta2Reason,
					clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Reason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate Machine's UpToDate conditions")
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentMachinesUpToDateV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentMachinesUpToDateInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	v1beta2conditions.Set(machineDeployment, *upToDateCondition)
}

func setRemediatingCondition(ctx context.Context, machineDeployment *clusterv1.MachineDeployment, machinesToBeRemediated, unhealthyMachines collections.Machines, getMachinesSucceeded bool) {
	if !getMachinesSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentRemediatingInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machinesToBeRemediated) == 0 {
		message := aggregateUnhealthyMachines(unhealthyMachines)
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineDeploymentNotRemediatingV1Beta2Reason,
			Message: message,
		})
		return
	}

	remediatingCondition, err := v1beta2conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.MachineDeploymentRemediatingV1Beta2Condition),
		// Note: in case of the remediating conditions it is not required to use a CustomMergeStrategy/ComputeReasonFunc
		// because we are considering only machinesToBeRemediated (and we can pin the reason when we set the condition).
	)
	if err != nil {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentRemediatingV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentRemediatingInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineOwnerRemediatedV1Beta2Condition))
		return
	}

	v1beta2conditions.Set(machineDeployment, metav1.Condition{
		Type:    remediatingCondition.Type,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineDeploymentRemediatingV1Beta2Reason,
		Message: remediatingCondition.Message,
	})
}

func setDeletingCondition(_ context.Context, machineDeployment *clusterv1.MachineDeployment, machineSets []*clusterv1.MachineSet, machines collections.Machines, getAndAdoptMachineSetsForDeploymentSucceeded, getMachinesSucceeded bool) {
	// If we got unexpected errors in listing the machines sets or machines (this should never happen), surface them.
	if !getAndAdoptMachineSetsForDeploymentSucceeded || !getMachinesSucceeded {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:    clusterv1.MachineDeploymentDeletingV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineDeploymentDeletingInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if machineDeployment.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(machineDeployment, metav1.Condition{
			Type:   clusterv1.MachineDeploymentDeletingV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineDeploymentNotDeletingV1Beta2Reason,
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
	if len(machines) == 0 && len(machineSets) > 0 {
		// Note: this should not happen or happen for a very short time while the finalizer is removed.
		message = fmt.Sprintf("Deleting %d MachineSets", len(machineSets))
	}
	if message == "" {
		message = "Deletion completed"
	}
	v1beta2conditions.Set(machineDeployment, metav1.Condition{
		Type:    clusterv1.MachineDeploymentDeletingV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachineDeploymentDeletingV1Beta2Reason,
		Message: message,
	})
}

func calculateMissingReferencesMessage(machineDeployment *clusterv1.MachineDeployment, bootstrapTemplateNotFound, infraMachineTemplateNotFound bool) string {
	missingObjects := []string{}
	if bootstrapTemplateNotFound {
		missingObjects = append(missingObjects, machineDeployment.Spec.Template.Spec.Bootstrap.ConfigRef.Kind)
	}
	if infraMachineTemplateNotFound {
		missingObjects = append(missingObjects, machineDeployment.Spec.Template.Spec.InfrastructureRef.Kind)
	}

	if len(missingObjects) == 0 {
		return ""
	}

	if len(missingObjects) == 1 {
		return fmt.Sprintf("because %s does not exist", missingObjects[0])
	}

	return fmt.Sprintf("because %s do not exist", strings.Join(missingObjects, " and "))
}

func aggregateStaleMachines(machines collections.Machines) string {
	if len(machines) == 0 {
		return ""
	}

	machineNames := []string{}
	delayReasons := sets.Set[string]{}
	for _, machine := range machines {
		if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) > time.Minute*15 {
			machineNames = append(machineNames, machine.GetName())

			deletingCondition := v1beta2conditions.Get(machine, clusterv1.MachineDeletingV1Beta2Condition)
			if deletingCondition != nil &&
				deletingCondition.Status == metav1.ConditionTrue &&
				deletingCondition.Reason == clusterv1.MachineDeletingDrainingNodeV1Beta2Reason &&
				machine.Status.Deletion != nil && time.Since(machine.Status.Deletion.NodeDrainStartTime.Time) > 5*time.Minute {
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
	message += "not healthy (not to be remediated by MachineDeployment/MachineSet)"

	return message
}
