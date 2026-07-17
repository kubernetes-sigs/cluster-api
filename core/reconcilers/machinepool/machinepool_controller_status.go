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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	internalversion "sigs.k8s.io/cluster-api/internal/util/version"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) error {
	var retErr error
	var machinePoolMachinesStateErr error
	machinePoolMachinesState := machinePoolMachinesStateUnknown
	if s.infraMachinePool != nil {
		machinePoolMachinesState, machinePoolMachinesStateErr = s.machinePoolMachinesState()
		if machinePoolMachinesStateErr != nil {
			retErr = fmt.Errorf("determining if there are machine pool machines: %w", machinePoolMachinesStateErr)
		}
	}

	// Existing Machines prove this is a machine-backed pool even if the infrastructure object could not be read.
	if machinePoolMachinesState == machinePoolMachinesStateUnknown &&
		s.getMachinesForMachinePoolSucceeded && len(s.machines) > 0 {
		machinePoolMachinesState = machinePoolMachinesStateSupported
	}

	switch machinePoolMachinesState {
	case machinePoolMachinesStateNotSupported:
		setReplicas(s.machinePool, false, nil, s.nodeRefMap)
	case machinePoolMachinesStateSupported:
		if s.getMachinesForMachinePoolSucceeded {
			setReplicas(s.machinePool, true, s.machines, s.nodeRefMap)
		}
	}

	setMachinesUpToDateCondition(ctx, s.machinePool, s.machines, machinePoolMachinesState, machinePoolMachinesStateErr, s.getMachinesForMachinePoolSucceeded)

	return retErr
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

func machinesUpToDateConditionCanBeSet(mp *clusterv1.MachinePool, machinePoolMachinesState machinePoolMachinesState, machinePoolMachinesStateErr error, getMachinesSucceeded bool) bool {
	switch machinePoolMachinesState {
	case machinePoolMachinesStateNotSupported:
		conditions.Delete(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
		return false
	case machinePoolMachinesStateUnknown:
		if machinePoolMachinesStateErr != nil || !getMachinesSucceeded {
			conditions.Set(mp, metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return false
		}
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolMachineSupportUnknownReason,
			Message: "Machine support could not be determined",
		})
		return false
	case machinePoolMachinesStateSupported:
		if !getMachinesSucceeded {
			conditions.Set(mp, metav1.Condition{
				Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
				Message: "Please check controller logs for errors",
			})
			return false
		}
	}

	return true
}

func setMachinesUpToDateCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, machinePoolMachinesState machinePoolMachinesState, machinePoolMachinesStateErr error, getMachinesSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// Machines are not listed during deletion, so leave the last reported condition unchanged.
	if !mp.DeletionTimestamp.IsZero() && !getMachinesSucceeded {
		return
	}

	if !machinesUpToDateConditionCanBeSet(mp, machinePoolMachinesState, machinePoolMachinesStateErr, getMachinesSucceeded) {
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
