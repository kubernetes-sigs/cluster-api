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

package machinepool

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/internal/contract"
	internalversion "sigs.k8s.io/cluster-api/internal/util/version"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	clog "sigs.k8s.io/cluster-api/util/log"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) error {
	var retErr error
	machinePoolMachinesState := machinePoolMachinesStateUnknown
	if s.infraMachinePool != nil {
		var err error
		machinePoolMachinesState, err = s.machinePoolMachinesState()
		if err != nil {
			retErr = fmt.Errorf("determining if there are machine pool machines: %w", err)
		}
	}

	// Existing Machines prove this is a machine-backed pool even if the infrastructure object could not be read.
	if machinePoolMachinesState == machinePoolMachinesStateUnknown &&
		s.getMachinesForMachinePoolSucceeded && len(s.machines) > 0 {
		machinePoolMachinesState = machinePoolMachinesStateSupported
	}

	replicaCountersObserved := false
	switch machinePoolMachinesState {
	case machinePoolMachinesStateNotSupported:
		if s.nodeRefMapObserved && s.infrastructureProviderIDListObserved {
			setReplicas(s.machinePool, false, nil, s.nodeRefMap)
			replicaCountersObserved = true
		}
	case machinePoolMachinesStateSupported:
		if s.getMachinesForMachinePoolSucceeded {
			setReplicas(s.machinePool, true, s.machines, s.nodeRefMap)
			replicaCountersObserved = true
		}
	}

	replicasObserved := s.infrastructureReplicasObserved

	setBootstrapConfigReadyCondition(ctx, s.machinePool, s.bootstrapConfig, s.bootstrapConfigIsNotFound)
	setInfrastructureReadyCondition(ctx, s.machinePool, s.infraMachinePool, s.infraMachinePoolIsNotFound)
	setMachinesReadyCondition(ctx, s.machinePool, s.machines, machinePoolMachinesState, s.getMachinesForMachinePoolSucceeded)
	setMachinesUpToDateCondition(ctx, s.machinePool, s.machines, machinePoolMachinesState, s.getMachinesForMachinePoolSucceeded)
	setRollingOutCondition(ctx, s.machinePool, s.machines, machinePoolMachinesState, s.getMachinesForMachinePoolSucceeded)
	setScalingUpCondition(ctx, s.machinePool, replicasObserved)
	setScalingDownCondition(ctx, s.machinePool, s.machines, s.getMachinesForMachinePoolSucceeded, replicasObserved)
	setRemediatingCondition(ctx, s.machinePool, s.machines, machinePoolMachinesState, s.getMachinesForMachinePoolSucceeded)
	setDeletingCondition(ctx, s.machinePool, s.machines, s.getMachinesForMachinePoolSucceeded)
	setAvailableCondition(ctx, s.machinePool, replicaCountersObserved)

	return retErr
}

func setReplicas(mp *clusterv1.MachinePool, hasMachinePoolMachines bool, machines []*clusterv1.Machine, nodeRefMap map[string]*corev1.Node) {
	if !hasMachinePoolMachines {
		var readyReplicas, availableReplicas int32
		minReadySeconds := ptr.Deref(mp.Spec.Template.Spec.MinReadySeconds, 0)
		now := metav1.Now()
		for _, providerID := range mp.Spec.ProviderIDList {
			node := nodeRefMap[providerID]
			if node == nil || !noderefutil.IsNodeReady(node) {
				continue
			}
			readyReplicas++
			if noderefutil.IsNodeAvailable(node, minReadySeconds, now) {
				availableReplicas++
			}
		}

		mp.Status.ReadyReplicas = ptr.To(readyReplicas)
		mp.Status.AvailableReplicas = ptr.To(availableReplicas)
		// UpToDateReplicas is not reported for MachinePools without Machines.
		mp.Status.UpToDateReplicas = nil
		mp.Status.Versions = versionsFromNodeRefs(mp.Status.NodeRefs, nodeRefMap)

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

	mp.Status.ReadyReplicas = ptr.To(readyReplicas)
	mp.Status.AvailableReplicas = ptr.To(availableReplicas)
	mp.Status.UpToDateReplicas = ptr.To(upToDateReplicas)
	mp.Status.Versions = internalversion.VersionsFromMachines(machines)
}

func versionsFromNodeRefs(nodeRefs []corev1.ObjectReference, nodeRefMap map[string]*corev1.Node) []clusterv1.StatusVersion {
	if len(nodeRefs) == 0 || len(nodeRefMap) == 0 {
		return nil
	}

	nodesByName := map[string]*corev1.Node{}
	for _, node := range nodeRefMap {
		if node == nil {
			continue
		}
		nodesByName[node.Name] = node
	}

	versions := []clusterv1.StatusVersion{}
	for _, nodeRef := range nodeRefs {
		node := nodesByName[nodeRef.Name]
		if node == nil || node.Status.NodeInfo.KubeletVersion == "" {
			continue
		}
		versions = append(versions, clusterv1.StatusVersion{
			Version:  node.Status.NodeInfo.KubeletVersion,
			Replicas: 1,
		})
	}

	return internalversion.AggregateStatusVersions(versions)
}

func setBootstrapConfigReadyCondition(_ context.Context, mp *clusterv1.MachinePool, bootstrapConfig *unstructured.Unstructured, bootstrapConfigIsNotFound bool) {
	if !mp.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolBootstrapConfigReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachinePoolBootstrapDataSecretProvidedReason,
		})
		return
	}

	if bootstrapConfig != nil {
		dataSecretCreated := ptr.Deref(mp.Status.Initialization.BootstrapDataSecretCreated, false)
		ready, err := conditions.NewMirrorConditionFromUnstructured(
			bootstrapConfig,
			contract.Bootstrap().ReadyConditionType(), conditions.TargetConditionType(clusterv1.MachinePoolBootstrapConfigReadyCondition),
			conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(dataSecretCreated),
				Reason:  fallbackReason(dataSecretCreated, clusterv1.MachinePoolBootstrapConfigReadyReason, clusterv1.MachinePoolBootstrapConfigNotReadyReason),
				Message: objectReadyFallbackMessage(mp.Spec.Template.Spec.Bootstrap.ConfigRef.Kind, "status.initialization.dataSecretCreated", dataSecretCreated),
			},
		)
		if err != nil {
			conditions.Set(mp, metav1.Condition{
				Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolBootstrapConfigInvalidConditionReportedReason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if ready.Reason == conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.MachinePoolBootstrapConfigReadyReason
		}
		conditions.Set(mp, *ready)
		return
	}

	// The bootstrap config is not re-read while the MachinePool is deleting (reconcileBootstrap does not
	// run in the delete reconcile), so leave the condition unchanged.
	if !mp.DeletionTimestamp.IsZero() {
		return
	}

	// If we got unexpected errors in reading the bootstrap config (this should happen rarely), surface them.
	if !bootstrapConfigIsNotFound {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolBootstrapConfigInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// The bootstrap config does not exist. If it existed before, surface that it has been deleted.
	if ptr.Deref(mp.Status.Initialization.BootstrapDataSecretCreated, false) {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachinePoolBootstrapConfigDeletedReason,
			Message: fmt.Sprintf("%s has been deleted", mp.Spec.Template.Spec.Bootstrap.ConfigRef.Kind),
		})
		return
	}

	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachinePoolBootstrapConfigDoesNotExistReason,
		Message: fmt.Sprintf("%s does not exist", mp.Spec.Template.Spec.Bootstrap.ConfigRef.Kind),
	})
}

func setInfrastructureReadyCondition(_ context.Context, mp *clusterv1.MachinePool, infraMachinePool *unstructured.Unstructured, infraMachinePoolIsNotFound bool) {
	if infraMachinePool != nil {
		infrastructureProvisioned := ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false)
		ready, err := conditions.NewMirrorConditionFromUnstructured(
			infraMachinePool,
			contract.InfrastructureMachine().ReadyConditionType(), conditions.TargetConditionType(clusterv1.MachinePoolInfrastructureReadyCondition),
			conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(infrastructureProvisioned),
				Reason:  fallbackReason(infrastructureProvisioned, clusterv1.MachinePoolInfrastructureReadyReason, clusterv1.MachinePoolInfrastructureNotReadyReason),
				Message: objectReadyFallbackMessage(mp.Spec.Template.Spec.InfrastructureRef.Kind, "status.initialization.provisioned", infrastructureProvisioned),
			},
		)
		if err != nil {
			conditions.Set(mp, metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolInfrastructureInvalidConditionReportedReason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if ready.Reason == conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.MachinePoolInfrastructureReadyReason
		}
		conditions.Set(mp, *ready)
		return
	}

	// The infrastructure object is not re-read while the MachinePool is deleting (reconcileInfrastructure
	// does not run in the delete reconcile), so leave the condition unchanged.
	if !mp.DeletionTimestamp.IsZero() {
		return
	}

	// If we got errors in reading the infra machine pool (this should happen rarely), surface them.
	if !infraMachinePoolIsNotFound {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolInfrastructureInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	// Infra machine pool missing after the MachinePool has been initialized.
	if ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachinePoolInfrastructureDeletedReason,
			Message: fmt.Sprintf("%s has been deleted", mp.Spec.Template.Spec.InfrastructureRef.Kind),
		})
		return
	}

	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachinePoolInfrastructureDoesNotExistReason,
		Message: fmt.Sprintf("%s does not exist", mp.Spec.Template.Spec.InfrastructureRef.Kind),
	})
}

func fallbackReason(status bool, trueReason, falseReason string) string {
	if status {
		return trueReason
	}
	return falseReason
}

func objectReadyFallbackMessage(kind, field string, ready bool) string {
	if ready {
		return ""
	}
	return fmt.Sprintf("%s %s is %t", kind, field, ready)
}

func machinePoolMachineConditionCanBeSet(mp *clusterv1.MachinePool, conditionType, internalErrorReason string, machinePoolMachinesState machinePoolMachinesState, getMachinesSucceeded bool) bool {
	switch machinePoolMachinesState {
	case machinePoolMachinesStateNotSupported:
		conditions.Delete(mp, conditionType)
		return false
	case machinePoolMachinesStateUnknown:
		if !getMachinesSucceeded {
			conditions.Set(mp, metav1.Condition{
				Type:    conditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  internalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return false
		}
		conditions.Set(mp, metav1.Condition{
			Type:    conditionType,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolMachineSupportUnknownReason,
			Message: "Machine support could not be determined",
		})
		return false
	case machinePoolMachinesStateSupported:
		if !getMachinesSucceeded {
			conditions.Set(mp, metav1.Condition{
				Type:    conditionType,
				Status:  metav1.ConditionUnknown,
				Reason:  internalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return false
		}
	}

	return true
}

func setMachinesReadyCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, machinePoolMachinesState machinePoolMachinesState, getMachinesSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	if !machinePoolMachineConditionCanBeSet(
		mp,
		clusterv1.MachinePoolMachinesReadyCondition,
		clusterv1.MachinePoolMachinesReadyInternalErrorReason,
		machinePoolMachinesState,
		getMachinesSucceeded,
	) {
		return
	}

	if len(machines) == 0 {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolMachinesReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachinePoolMachinesReadyNoReplicasReason,
		})
		return
	}

	readyCondition, err := conditions.NewAggregateCondition(
		machines, clusterv1.MachineReadyCondition,
		conditions.TargetConditionType(clusterv1.MachinePoolMachinesReadyCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.MachinePoolMachinesNotReadyReason,
					clusterv1.MachinePoolMachinesReadyUnknownReason,
					clusterv1.MachinePoolMachinesReadyReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate Machine's Ready conditions")
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolMachinesReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolMachinesReadyInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(mp, *readyCondition)
}

func setMachinesUpToDateCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, machinePoolMachinesState machinePoolMachinesState, getMachinesSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	if !machinePoolMachineConditionCanBeSet(
		mp,
		clusterv1.MachinePoolMachinesUpToDateCondition,
		clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
		machinePoolMachinesState,
		getMachinesSucceeded,
	) {
		return
	}

	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	upToDateMachines := make([]*clusterv1.Machine, 0, len(machines))
	for _, machine := range machines {
		if conditions.Has(machine, clusterv1.MachineUpToDateCondition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second {
			upToDateMachines = append(upToDateMachines, machine)
		}
	}

	if len(upToDateMachines) == 0 {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachinePoolMachinesUpToDateNoReplicasReason,
		})
		return
	}

	upToDateCondition, err := conditions.NewAggregateCondition(
		upToDateMachines, clusterv1.MachineUpToDateCondition,
		conditions.TargetConditionType(clusterv1.MachinePoolMachinesUpToDateCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.MachinePoolMachinesNotUpToDateReason,
					clusterv1.MachinePoolMachinesUpToDateUnknownReason,
					clusterv1.MachinePoolMachinesUpToDateReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate Machine's UpToDate conditions")
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(mp, *upToDateCondition)
}

func setRollingOutCondition(_ context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, machinePoolMachinesState machinePoolMachinesState, getMachinesSucceeded bool) {
	if !machinePoolMachineConditionCanBeSet(
		mp,
		clusterv1.MachinePoolRollingOutCondition,
		clusterv1.MachinePoolRollingOutInternalErrorReason,
		machinePoolMachinesState,
		getMachinesSucceeded,
	) {
		return
	}

	// Count machines rolling out and collect reasons why a rollout is happening.
	// Note: The code below collects all the reasons for which at least a machine is rolling out; under normal circumstances
	// all the machines are rolling out for the same reasons, however, in case of changes to the MachinePool before a
	// previous change is fully rolled out, there could be machines rolling out for different reasons.
	rollingOutReplicas := 0
	rolloutReasons := sets.Set[string]{}
	for _, machine := range machines {
		upToDateCondition := conditions.Get(machine, clusterv1.MachineUpToDateCondition)
		if upToDateCondition == nil || upToDateCondition.Status != metav1.ConditionFalse {
			continue
		}
		rollingOutReplicas++
		if upToDateCondition.Message != "" {
			rolloutReasons.Insert(strings.Split(upToDateCondition.Message, "\n")...)
		}
	}

	if rollingOutReplicas == 0 {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolRollingOutCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachinePoolNotRollingOutReason,
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
	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolRollingOutCondition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachinePoolRollingOutReason,
		Message: message,
	})
}

func setRemediatingCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, machinePoolMachinesState machinePoolMachinesState, getMachinesSucceeded bool) {
	if !machinePoolMachineConditionCanBeSet(
		mp,
		clusterv1.MachinePoolRemediatingCondition,
		clusterv1.MachinePoolRemediatingInternalErrorReason,
		machinePoolMachinesState,
		getMachinesSucceeded,
	) {
		return
	}

	allMachines := collections.FromMachines(machines...)
	machinesToBeRemediated := allMachines.Filter(collections.IsUnhealthyAndOwnerRemediated)
	unhealthyMachines := allMachines.Filter(collections.IsUnhealthy)

	if len(machinesToBeRemediated) == 0 {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolRemediatingCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachinePoolNotRemediatingReason,
			Message: aggregateUnhealthyMachines(unhealthyMachines),
		})
		return
	}

	remediatingCondition, err := conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedCondition,
		conditions.TargetConditionType(clusterv1.MachinePoolRemediatingCondition),
		// Note: in case of the remediating conditions it is not required to use a CustomMergeStrategy/ComputeReasonFunc
		// because we are considering only machinesToBeRemediated (and we can pin the reason when we set the condition).
	)
	if err != nil {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolRemediatingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolRemediatingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineOwnerRemediatedCondition))
		return
	}

	conditions.Set(mp, metav1.Condition{
		Type:    remediatingCondition.Type,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachinePoolRemediatingReason,
		Message: remediatingCondition.Message,
	})
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
	message += "not healthy (not to be remediated by MachinePool)"

	return message
}

func aggregateStaleMachines(machines []*clusterv1.Machine) string {
	if len(machines) == 0 {
		return ""
	}

	machineNames := []string{}
	delayReasons := sets.Set[string]{}
	for _, machine := range machines {
		if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) > 15*time.Minute {
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

func setScalingUpCondition(_ context.Context, mp *clusterv1.MachinePool, replicasObserved bool) {
	if !replicasObserved {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolScalingUpCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolScalingUpReplicasNotObservedReason,
			Message: "status.replicas was not observed",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should never happen).
	if mp.Spec.Replicas == nil {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolScalingUpCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolScalingUpWaitingForReplicasSetReason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	desiredReplicas := *mp.Spec.Replicas
	if !mp.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	currentReplicas := ptr.Deref(mp.Status.Replicas, 0)

	if currentReplicas >= desiredReplicas {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolScalingUpCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachinePoolNotScalingUpReason,
		})
		return
	}

	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolScalingUpCondition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachinePoolScalingUpReason,
		Message: fmt.Sprintf("Scaling up from %d to %d replicas", currentReplicas, desiredReplicas),
	})
}

func setScalingDownCondition(_ context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, getMachinesSucceeded, replicasObserved bool) {
	if !replicasObserved {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolScalingDownCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolScalingDownReplicasNotObservedReason,
			Message: "status.replicas was not observed",
		})
		return
	}

	// Surface if .spec.replicas is not yet set (this should never happen).
	if mp.Spec.Replicas == nil {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolScalingDownCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolScalingDownWaitingForReplicasSetReason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	desiredReplicas := *mp.Spec.Replicas
	if !mp.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}
	currentReplicas := ptr.Deref(mp.Status.Replicas, 0)

	if currentReplicas > desiredReplicas {
		message := fmt.Sprintf("Scaling down from %d to %d replicas", currentReplicas, desiredReplicas)
		if getMachinesSucceeded {
			staleMessage := aggregateStaleMachines(machines)
			if staleMessage != "" {
				message += fmt.Sprintf("\n* %s", staleMessage)
			}
		}
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolScalingDownCondition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachinePoolScalingDownReason,
			Message: message,
		})
		return
	}

	conditions.Set(mp, metav1.Condition{
		Type:   clusterv1.MachinePoolScalingDownCondition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.MachinePoolNotScalingDownReason,
	})
}

func setDeletingCondition(_ context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, getMachinesSucceeded bool) {
	if mp.DeletionTimestamp.IsZero() {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolDeletingCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachinePoolNotDeletingReason,
		})
		return
	}

	if !getMachinesSucceeded {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolDeletingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolDeletingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	message := "Deletion in progress"
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
	} else if len(mp.Status.NodeRefs) > 0 {
		if len(mp.Status.NodeRefs) == 1 {
			message = "Deleting 1 Node"
		} else {
			message = fmt.Sprintf("Deleting %d Nodes", len(mp.Status.NodeRefs))
		}
	}
	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolDeletingCondition,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.MachinePoolDeletingReason,
		Message: message,
	})
}

func setAvailableCondition(_ context.Context, mp *clusterv1.MachinePool, replicaCountersObserved bool) {
	// Surface if .spec.replicas is not yet set (this should never happen).
	if mp.Spec.Replicas == nil {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolAvailableWaitingForReplicasSetReason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	if !replicaCountersObserved {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolAvailableReplicaCountersNotObservedReason,
			Message: "Replica counters were not observed",
		})
		return
	}

	// Surface if .status.availableReplicas is not yet set.
	if mp.Status.AvailableReplicas == nil {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolAvailableWaitingForAvailableReplicasSetReason,
			Message: "Waiting for status.availableReplicas set",
		})
		return
	}

	if !conditions.IsTrue(mp, clusterv1.MachinePoolInfrastructureReadyCondition) {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachinePoolNotAvailableReason,
			Message: "Infrastructure not ready",
		})
		return
	}

	if *mp.Status.AvailableReplicas >= *mp.Spec.Replicas {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolAvailableCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachinePoolAvailableReason,
		})
		return
	}

	message := fmt.Sprintf("%d available replicas, at least %d required", *mp.Status.AvailableReplicas, *mp.Spec.Replicas)
	if !mp.DeletionTimestamp.IsZero() {
		message = "Deletion in progress"
	}
	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolAvailableCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachinePoolNotAvailableReason,
		Message: message,
	})
}
