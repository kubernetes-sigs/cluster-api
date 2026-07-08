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
	"sigs.k8s.io/cluster-api/internal/contract"
	internalversion "sigs.k8s.io/cluster-api/internal/util/version"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	clog "sigs.k8s.io/cluster-api/util/log"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) error {
	log := ctrl.LoggerFrom(ctx)

	if s.infraMachinePool == nil {
		log.V(4).Info("infra machine pool isn't set, skipping setting status")
		return nil
	}
	hasMachinePoolMachines, err := s.hasMachinePoolMachines()
	if err != nil {
		return fmt.Errorf("determining if there are machine pool machines: %w", err)
	}

	setReplicas(s.machinePool, hasMachinePoolMachines, s.machines, s.nodeRefMap)

	setBootstrapConfigReadyCondition(ctx, s.machinePool, s.bootstrapConfig, s.bootstrapConfigIsNotFound)
	setInfrastructureReadyCondition(ctx, s.machinePool, s.infraMachinePool, s.infraMachinePoolIsNotFound)
	setMachinesReadyCondition(ctx, s.machinePool, s.machines, hasMachinePoolMachines)
	setMachinesUpToDateCondition(ctx, s.machinePool, s.machines, hasMachinePoolMachines)
	setRollingOutCondition(ctx, s.machinePool, s.machines, hasMachinePoolMachines)
	setRemediatingCondition(ctx, s.machinePool, s.machines, hasMachinePoolMachines)

	return nil
}

func setReplicas(mp *clusterv1.MachinePool, hasMachinePoolMachines bool, machines []*clusterv1.Machine, nodeRefMap map[string]*corev1.Node) {
	if !hasMachinePoolMachines {
		// If we don't have machinepool machine then calculate the values differently
		mp.Status.ReadyReplicas = mp.Status.Replicas
		mp.Status.AvailableReplicas = mp.Status.Replicas
		mp.Status.UpToDateReplicas = mp.Spec.Replicas
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

	// Bootstrap config missing when the MachinePool is deleting and we know that the BootstrapConfig actually existed.
	if !mp.DeletionTimestamp.IsZero() && ptr.Deref(mp.Status.Initialization.BootstrapDataSecretCreated, false) {
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

func setMachinesReadyCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, hasMachinePoolMachines bool) {
	log := ctrl.LoggerFrom(ctx)

	// If the MachinePool doesn't have Machines (e.g. managed pools), we cannot compute this aggregate condition.
	if !hasMachinePoolMachines {
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

func setMachinesUpToDateCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, hasMachinePoolMachines bool) {
	log := ctrl.LoggerFrom(ctx)

	// If the MachinePool doesn't have Machines (e.g. managed pools), we cannot compute this aggregate condition.
	if !hasMachinePoolMachines {
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

func setRollingOutCondition(_ context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, hasMachinePoolMachines bool) {
	// If the MachinePool doesn't have Machines (e.g. managed pools), we cannot compute this aggregate condition.
	if !hasMachinePoolMachines {
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

func setRemediatingCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, hasMachinePoolMachines bool) {
	// If the MachinePool doesn't have Machines (e.g. managed pools), we cannot compute this aggregate condition.
	if !hasMachinePoolMachines {
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
