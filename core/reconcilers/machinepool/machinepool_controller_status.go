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
	contractapi "sigs.k8s.io/cluster-api/internal/contract/api"
	internalversion "sigs.k8s.io/cluster-api/internal/util/version"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) error {
	setBootstrapConfigReadyCondition(ctx, s.machinePool, s.bootstrapConfig, s.bootstrapConfigIsNotFound)
	setInfrastructureReadyCondition(ctx, s.machinePool, s.infraMachinePool, s.infraMachinePoolIsNotFound, s.infrastructureProvisioned)

	log := ctrl.LoggerFrom(ctx)

	if s.infraMachinePool == nil {
		log.V(4).Info("infra machine pool isn't set, skipping setting status")
		return nil
	}
	hasMachinePoolMachines, err := s.hasMachinePoolMachines()
	if err != nil {
		return fmt.Errorf("determining if there are machine pool machines: %w", err)
	}

	if !hasMachinePoolMachines || s.getMachinesForMachinePoolSucceeded {
		setReplicas(s.machinePool, hasMachinePoolMachines, s.machines, s.nodeRefMap)
	}

	setMachinesUpToDateCondition(ctx, s.machinePool, s.machines, hasMachinePoolMachines, s.getMachinesForMachinePoolSucceeded)

	return nil
}

func setBootstrapConfigReadyCondition(_ context.Context, mp *clusterv1.MachinePool, bootstrapConfig contractapi.BootstrapConfig, bootstrapConfigIsNotFound bool) {
	if !mp.Spec.Template.Spec.Bootstrap.ConfigRef.IsDefined() {
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolBootstrapConfigReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachinePoolBootstrapDataSecretProvidedReason,
		})
		return
	}

	if bootstrapConfig != nil {
		dataSecretCreated := bootstrapConfig.GetDataSecretCreated()
		ready := conditions.NewMirrorCondition(
			bootstrapConfig,
			clusterv1.ReadyCondition,
			conditions.TargetConditionType(clusterv1.MachinePoolBootstrapConfigReadyCondition),
			conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(dataSecretCreated),
				Reason:  fallbackReason(dataSecretCreated, clusterv1.MachinePoolBootstrapConfigReadyReason, clusterv1.MachinePoolBootstrapConfigNotReadyReason),
				Message: objectReadyFallbackMessage(mp.Spec.Template.Spec.Bootstrap.ConfigRef.Kind, "status.initialization.dataSecretCreated", dataSecretCreated),
			},
		)
		// Legacy True conditions may omit the reason.
		if ready.Reason == conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.MachinePoolBootstrapConfigReadyReason
		}
		conditions.Set(mp, *ready)
		return
	}

	// Bootstrap is not reconciled during deletion, so preserve the last reported condition.
	if !mp.DeletionTimestamp.IsZero() {
		return
	}

	if !bootstrapConfigIsNotFound {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolBootstrapConfigInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	reason := clusterv1.MachinePoolBootstrapConfigDoesNotExistReason
	message := fmt.Sprintf("%s does not exist", mp.Spec.Template.Spec.Bootstrap.ConfigRef.Kind)
	if ptr.Deref(mp.Status.Initialization.BootstrapDataSecretCreated, false) {
		reason = clusterv1.MachinePoolBootstrapConfigDeletedReason
		message = fmt.Sprintf("%s has been deleted", mp.Spec.Template.Spec.Bootstrap.ConfigRef.Kind)
	}
	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolBootstrapConfigReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
}

func setInfrastructureReadyCondition(_ context.Context, mp *clusterv1.MachinePool, infraMachinePool *unstructured.Unstructured, infraMachinePoolIsNotFound bool, infrastructureProvisioned *bool) {
	if infraMachinePool != nil {
		options := []conditions.MirrorOption{
			conditions.TargetConditionType(clusterv1.MachinePoolInfrastructureReadyCondition),
			conditions.FallbackCondition{
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolInfrastructureInternalErrorReason,
				Message: "Please check controller logs for errors",
			},
		}
		if infrastructureProvisioned != nil {
			provisioned := *infrastructureProvisioned
			options = append(options, conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(provisioned),
				Reason:  fallbackReason(provisioned, clusterv1.MachinePoolInfrastructureReadyReason, clusterv1.MachinePoolInfrastructureNotReadyReason),
				Message: objectReadyFallbackMessage(mp.Spec.Template.Spec.InfrastructureRef.Kind, "status.initialization.provisioned", provisioned),
			})
		}
		ready, err := conditions.NewMirrorConditionFromUnstructured(infraMachinePool, clusterv1.ReadyCondition, options...)
		if err != nil {
			conditions.Set(mp, metav1.Condition{
				Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachinePoolInfrastructureInvalidConditionReportedReason,
				Message: err.Error(),
			})
			return
		}
		// Legacy True conditions may omit the reason.
		if ready.Reason == conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.MachinePoolInfrastructureReadyReason
		}
		conditions.Set(mp, *ready)
		return
	}

	// Infrastructure is not reconciled during deletion, so preserve the last reported condition.
	if !mp.DeletionTimestamp.IsZero() {
		return
	}

	if !infraMachinePoolIsNotFound {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolInfrastructureInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	reason := clusterv1.MachinePoolInfrastructureDoesNotExistReason
	message := fmt.Sprintf("%s does not exist", mp.Spec.Template.Spec.InfrastructureRef.Kind)
	if ptr.Deref(mp.Status.Initialization.InfrastructureProvisioned, false) {
		reason = clusterv1.MachinePoolInfrastructureDeletedReason
		message = fmt.Sprintf("%s has been deleted while the MachinePool still exists", mp.Spec.Template.Spec.InfrastructureRef.Kind)
	}
	conditions.Set(mp, metav1.Condition{
		Type:    clusterv1.MachinePoolInfrastructureReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
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

func setMachinesUpToDateCondition(ctx context.Context, mp *clusterv1.MachinePool, machinesSlice []*clusterv1.Machine, hasMachinePoolMachines, getMachinesSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	if !hasMachinePoolMachines {
		conditions.Delete(mp, clusterv1.MachinePoolMachinesUpToDateCondition)
		return
	}

	if !getMachinesSucceeded {
		conditions.Set(mp, metav1.Condition{
			Type:    clusterv1.MachinePoolMachinesUpToDateCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachinePoolMachinesUpToDateInternalErrorReason,
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
		conditions.Set(mp, metav1.Condition{
			Type:   clusterv1.MachinePoolMachinesUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachinePoolMachinesUpToDateNoReplicasReason,
		})
		return
	}

	upToDateCondition, err := conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineUpToDateCondition,
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
