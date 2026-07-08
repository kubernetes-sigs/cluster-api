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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/contract"
	internalversion "sigs.k8s.io/cluster-api/internal/util/version"
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

	setReplicas(s.machinePool, hasMachinePoolMachines, s.machines, s.nodeRefMap)

	setBootstrapConfigReadyCondition(ctx, s.machinePool, s.bootstrapConfig, s.bootstrapConfigIsNotFound)
	setInfrastructureReadyCondition(ctx, s.machinePool, s.infraMachinePool, s.infraMachinePoolIsNotFound)
	setMachinesUpToDateCondition(ctx, s.machinePool, s.machines, hasMachinePoolMachines)

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
