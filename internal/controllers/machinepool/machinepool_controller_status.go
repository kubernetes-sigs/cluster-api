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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/conditions"
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

	setReplicas(s.machinePool, hasMachinePoolMachines, s.machines)

	// Set MachinesUpToDate condition to surface when machines are not up-to-date with the spec.
	setMachinesUpToDateCondition(ctx, s.machinePool, s.machines, hasMachinePoolMachines)

	return nil
}

func setReplicas(mp *clusterv1.MachinePool, hasMachinePoolMachines bool, machines []*clusterv1.Machine) {
	if !hasMachinePoolMachines {
		// If we don't have machinepool machine then calculate the values differently
		mp.Status.ReadyReplicas = mp.Status.Replicas
		mp.Status.AvailableReplicas = mp.Status.Replicas
		mp.Status.UpToDateReplicas = mp.Spec.Replicas

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
}

// setMachinesUpToDateCondition sets the MachinesUpToDate condition on the MachinePool.
func setMachinesUpToDateCondition(ctx context.Context, mp *clusterv1.MachinePool, machines []*clusterv1.Machine, hasMachinePoolMachines bool) {
	log := ctrl.LoggerFrom(ctx)

	// For MachinePool providers that don't use Machine objects (like managed pools),
	// we derive the MachinesUpToDate condition from the infrastructure status.
	// If InfrastructureReady is False, machines are not up-to-date.
	if !hasMachinePoolMachines {
		// Check if infrastructure is ready by looking at the initialization status
		if !ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) {
			log.V(4).Info("setting MachinesUpToDate to false for machineless MachinePool: infrastructure not yet provisioned")
			conditions.Set(mp, metav1.Condition{
				Type:    clusterv1.MachinesUpToDateCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.NotUpToDateReason,
				Message: "Infrastructure is not yet provisioned",
			})
			return
		}

		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinesUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.UpToDateReason,
		})
		return
	}

	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	filteredMachines := make([]*clusterv1.Machine, 0, len(machines))
	for _, machine := range machines {
		if conditions.Has(machine, clusterv1.MachineUpToDateCondition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second {
			filteredMachines = append(filteredMachines, machine)
		}
	}

	if len(filteredMachines) == 0 {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinesUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.NoReplicasReason,
		})
		return
	}

	upToDateCondition, err := conditions.NewAggregateCondition(
		filteredMachines, clusterv1.MachineUpToDateCondition,
		conditions.TargetConditionType(clusterv1.MachinesUpToDateCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.NotUpToDateReason,
					clusterv1.UpToDateUnknownReason,
					clusterv1.UpToDateReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate Machine's UpToDate conditions")
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinesUpToDateCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.InternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(mp, *upToDateCondition)
}
