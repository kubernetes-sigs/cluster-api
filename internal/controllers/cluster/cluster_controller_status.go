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

package cluster

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	clog "sigs.k8s.io/cluster-api/util/log"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) error {
	// Always reconcile the Status.Phase field.
	if changed := setPhase(ctx, s.cluster); changed {
		r.recorder.Eventf(s.cluster, corev1.EventTypeNormal, string(s.cluster.Status.GetTypedPhase()), "Cluster %s is %s", s.cluster.Name, string(s.cluster.Status.GetTypedPhase()))
	}

	controlPlaneContractVersion := ""
	if s.controlPlane != nil {
		var err error
		controlPlaneContractVersion, err = contract.GetContractVersion(ctx, r.Client, s.controlPlane.GroupVersionKind().GroupKind())
		if err != nil {
			return err
		}
	}

	// TODO: "expv1.MachinePoolList{}" below should be replaced through "s.descendants.machinePools" once replica counters
	// and Available, ScalingUp and ScalingDown conditions have been implemented for MachinePools.

	// TODO: This should be removed once the UpToDate condition has been implemented for MachinePool Machines
	isMachinePoolMachine := func(machine *clusterv1.Machine) bool {
		_, isMachinePoolMachine := machine.Labels[clusterv1.MachinePoolNameLabel]
		return isMachinePoolMachine
	}
	controlPlaneMachines := s.descendants.controlPlaneMachines
	workerMachines := s.descendants.workerMachines.Filter(collections.Not(isMachinePoolMachine))
	machinesToBeRemediated := s.descendants.machinesToBeRemediated.Filter(collections.Not(isMachinePoolMachine))
	unhealthyMachines := s.descendants.unhealthyMachines.Filter(collections.Not(isMachinePoolMachine))

	// replica counters
	setControlPlaneReplicas(ctx, s.cluster, s.controlPlane, controlPlaneContractVersion, s.descendants.controlPlaneMachines, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setWorkersReplicas(ctx, s.cluster, clusterv1.MachinePoolList{}, s.descendants.machineDeployments, s.descendants.machineSets, workerMachines, s.getDescendantsSucceeded)

	// conditions
	healthCheckingState := r.ClusterCache.GetHealthCheckingState(ctx, client.ObjectKeyFromObject(s.cluster))
	setRemoteConnectionProbeCondition(ctx, s.cluster, healthCheckingState, r.RemoteConnectionGracePeriod)
	setInfrastructureReadyCondition(ctx, s.cluster, s.infraCluster, s.infraClusterIsNotFound)
	setControlPlaneAvailableCondition(ctx, s.cluster, s.controlPlane, s.controlPlaneIsNotFound)
	setControlPlaneInitializedCondition(ctx, s.cluster, s.controlPlane, controlPlaneContractVersion, s.descendants.controlPlaneMachines, s.infraClusterIsNotFound, s.getDescendantsSucceeded)
	setWorkersAvailableCondition(ctx, s.cluster, clusterv1.MachinePoolList{}, s.descendants.machineDeployments, s.getDescendantsSucceeded)
	setControlPlaneMachinesReadyCondition(ctx, s.cluster, controlPlaneMachines, s.getDescendantsSucceeded)
	setWorkerMachinesReadyCondition(ctx, s.cluster, workerMachines, s.getDescendantsSucceeded)
	setControlPlaneMachinesUpToDateCondition(ctx, s.cluster, controlPlaneMachines, s.getDescendantsSucceeded)
	setWorkerMachinesUpToDateCondition(ctx, s.cluster, workerMachines, s.getDescendantsSucceeded)
	setRollingOutCondition(ctx, s.cluster, s.controlPlane, clusterv1.MachinePoolList{}, s.descendants.machineDeployments, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setScalingUpCondition(ctx, s.cluster, s.controlPlane, clusterv1.MachinePoolList{}, s.descendants.machineDeployments, s.descendants.machineSets, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setScalingDownCondition(ctx, s.cluster, s.controlPlane, clusterv1.MachinePoolList{}, s.descendants.machineDeployments, s.descendants.machineSets, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setRemediatingCondition(ctx, s.cluster, machinesToBeRemediated, unhealthyMachines, s.getDescendantsSucceeded)
	setDeletingCondition(ctx, s.cluster, s.deletingReason, s.deletingMessage)
	setAvailableCondition(ctx, s.cluster, s.clusterClass)

	return nil
}

func setPhase(_ context.Context, cluster *clusterv1.Cluster) bool {
	preReconcilePhase := cluster.Status.GetTypedPhase()

	if cluster.Status.Phase == "" {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhasePending)
	}

	if cluster.Spec.InfrastructureRef.IsDefined() || cluster.Spec.ControlPlaneRef.IsDefined() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioning)
	}

	if ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) && cluster.Spec.ControlPlaneEndpoint.IsValid() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseProvisioned)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		cluster.Status.SetTypedPhase(clusterv1.ClusterPhaseDeleting)
	}
	if preReconcilePhase != cluster.Status.GetTypedPhase() {
		return true
	}
	return false
}

func setControlPlaneReplicas(_ context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, controlPlaneContractVersion string, controlPlaneMachines collections.Machines, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	if cluster.Status.ControlPlane == nil {
		cluster.Status.ControlPlane = &clusterv1.ClusterControlPlaneStatus{}
	}

	// If this cluster is using a control plane object, surface the replica counters reported by it.
	// Note: The cluster API contract does not require that a control plane object has a notion of replicas;
	// if the control plane object does have a notion of replicas, or expected replicas fields are not provided,
	// corresponding replicas will be left empty.
	if cluster.Spec.ControlPlaneRef.IsDefined() || cluster.Spec.Topology.IsDefined() {
		if controlPlane == nil || controlPlaneIsNotFound {
			cluster.Status.ControlPlane.Replicas = nil
			cluster.Status.ControlPlane.ReadyReplicas = nil
			cluster.Status.ControlPlane.AvailableReplicas = nil
			cluster.Status.ControlPlane.UpToDateReplicas = nil
			cluster.Status.ControlPlane.DesiredReplicas = nil
			return
		}

		if replicas, err := contract.ControlPlane().Replicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.ControlPlane.DesiredReplicas = replicas
		}
		if replicas, err := contract.ControlPlane().StatusReplicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.ControlPlane.Replicas = replicas
		}
		if replicas, err := contract.ControlPlane().ReadyReplicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.ControlPlane.ReadyReplicas = replicas
		}
		if controlPlaneContractVersion == "v1beta1" {
			if unavailableReplicas, err := contract.ControlPlane().V1Beta1UnavailableReplicas().Get(controlPlane); err == nil && unavailableReplicas != nil {
				if cluster.Status.ControlPlane.DesiredReplicas != nil {
					if *cluster.Status.ControlPlane.DesiredReplicas >= int32(*unavailableReplicas) {
						cluster.Status.ControlPlane.AvailableReplicas = ptr.To(*cluster.Status.ControlPlane.DesiredReplicas - int32(*unavailableReplicas))
					} else {
						cluster.Status.ControlPlane.AvailableReplicas = ptr.To[int32](0)
					}
				}
			}
		} else {
			if replicas, err := contract.ControlPlane().AvailableReplicas().Get(controlPlane); err == nil && replicas != nil {
				cluster.Status.ControlPlane.AvailableReplicas = replicas
			}
		}

		if replicas, err := contract.ControlPlane().UpToDateReplicas(controlPlaneContractVersion).Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.ControlPlane.UpToDateReplicas = replicas
		}
		return
	}

	// If there was some unexpected errors in listing descendants (this should never happen), do not update replica counters.
	if !getDescendantsSucceeded {
		return
	}

	// Otherwise this cluster control plane is composed by stand-alone machines, so we should count them.
	var replicas, readyReplicas, availableReplicas, upToDateReplicas *int32
	for _, machine := range controlPlaneMachines.UnsortedList() {
		replicas = ptr.To(ptr.Deref(replicas, 0) + 1)
		if conditions.IsTrue(machine, clusterv1.MachineReadyCondition) {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + 1)
		}
		if conditions.IsTrue(machine, clusterv1.MachineAvailableCondition) {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + 1)
		}
		if conditions.IsTrue(machine, clusterv1.MachineUpToDateCondition) {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + 1)
		}
	}

	cluster.Status.ControlPlane.Replicas = replicas
	cluster.Status.ControlPlane.ReadyReplicas = readyReplicas
	cluster.Status.ControlPlane.AvailableReplicas = availableReplicas
	cluster.Status.ControlPlane.UpToDateReplicas = upToDateReplicas

	// There is no concept of desired replicas for stand-alone machines, but for sake of consistency
	// we consider the fact that the machine has been creates as equivalent to the intent to have one replica.
	cluster.Status.ControlPlane.DesiredReplicas = replicas
}

func setWorkersReplicas(_ context.Context, cluster *clusterv1.Cluster, machinePools clusterv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, machineSets clusterv1.MachineSetList, workerMachines collections.Machines, getDescendantsSucceeded bool) {
	if cluster.Status.Workers == nil {
		cluster.Status.Workers = &clusterv1.WorkersStatus{}
	}

	// If there was some unexpected errors in listing descendants (this should never happen), do not update replica counters.
	if !getDescendantsSucceeded {
		return
	}

	var desiredReplicas, currentReplicas, readyReplicas, availableReplicas, upToDateReplicas *int32
	for _, mp := range machinePools.Items {
		if mp.Spec.Replicas != nil {
			desiredReplicas = ptr.To(ptr.Deref(desiredReplicas, 0) + *mp.Spec.Replicas)
		}
		if mp.Status.Replicas != nil {
			currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + *mp.Status.Replicas)
		}
		if mp.Status.ReadyReplicas != nil {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + *mp.Status.ReadyReplicas)
		}
		if mp.Status.AvailableReplicas != nil {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + *mp.Status.AvailableReplicas)
		}
		if mp.Status.UpToDateReplicas != nil {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + *mp.Status.UpToDateReplicas)
		}
	}

	for _, md := range machineDeployments.Items {
		if md.Spec.Replicas != nil {
			desiredReplicas = ptr.To(ptr.Deref(desiredReplicas, 0) + *md.Spec.Replicas)
		}
		if md.Status.Replicas != nil {
			currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + *md.Status.Replicas)
		}
		if md.Status.ReadyReplicas != nil {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + *md.Status.ReadyReplicas)
		}
		if md.Status.AvailableReplicas != nil {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + *md.Status.AvailableReplicas)
		}
		if md.Status.UpToDateReplicas != nil {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + *md.Status.UpToDateReplicas)
		}
	}

	for _, ms := range machineSets.Items {
		if !util.IsOwnedByObject(&ms, cluster) {
			continue
		}
		if ms.Spec.Replicas != nil {
			desiredReplicas = ptr.To(ptr.Deref(desiredReplicas, 0) + *ms.Spec.Replicas)
		}
		if ms.Status.Replicas != nil {
			currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + *ms.Status.Replicas)
		}
		if ms.Status.ReadyReplicas != nil {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + *ms.Status.ReadyReplicas)
		}
		if ms.Status.AvailableReplicas != nil {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + *ms.Status.AvailableReplicas)
		}
		if ms.Status.UpToDateReplicas != nil {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + *ms.Status.UpToDateReplicas)
		}
	}

	for _, m := range workerMachines.UnsortedList() {
		if !util.IsOwnedByObject(m, cluster) {
			continue
		}
		currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + 1)
		if conditions.IsTrue(m, clusterv1.MachineReadyCondition) {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + 1)
		}
		if conditions.IsTrue(m, clusterv1.MachineAvailableCondition) {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + 1)
		}
		if conditions.IsTrue(m, clusterv1.MachineUpToDateCondition) {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + 1)
		}

		// There is no concept of desired replicas for stand-alone machines, but for sake of consistency
		// we consider the fact that the machine has been creates as equivalent to the intent to have one replica.
		desiredReplicas = ptr.To(ptr.Deref(desiredReplicas, 0) + 1)
	}

	cluster.Status.Workers.DesiredReplicas = desiredReplicas
	cluster.Status.Workers.Replicas = currentReplicas
	cluster.Status.Workers.ReadyReplicas = readyReplicas
	cluster.Status.Workers.AvailableReplicas = availableReplicas
	cluster.Status.Workers.UpToDateReplicas = upToDateReplicas
}

func setRemoteConnectionProbeCondition(_ context.Context, cluster *clusterv1.Cluster, healthCheckingState clustercache.HealthCheckingState, remoteConnectionGracePeriod time.Duration) {
	// ClusterCache did not try to connect often enough yet, either during controller startup or when a new Cluster is created.
	if healthCheckingState.LastProbeSuccessTime.IsZero() && healthCheckingState.ConsecutiveFailures < 5 {
		// If condition is not set, set it.
		if !conditions.Has(cluster, clusterv1.ClusterRemoteConnectionProbeCondition) {
			conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterRemoteConnectionProbeCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterRemoteConnectionProbeFailedReason,
				Message: "Remote connection not established yet",
			})
		}
		return
	}

	if time.Since(healthCheckingState.LastProbeSuccessTime) > remoteConnectionGracePeriod {
		var msg string
		if healthCheckingState.LastProbeSuccessTime.IsZero() {
			msg = "Remote connection probe failed"
		} else {
			msg = fmt.Sprintf("Remote connection probe failed, probe last succeeded at %s", healthCheckingState.LastProbeSuccessTime.Format(time.RFC3339))
		}
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemoteConnectionProbeCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterRemoteConnectionProbeFailedReason,
			Message: msg,
		})
		return
	}

	conditions.Set(cluster, metav1.Condition{
		Type:   clusterv1.ClusterRemoteConnectionProbeCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterRemoteConnectionProbeSucceededReason,
	})
}

func setInfrastructureReadyCondition(_ context.Context, cluster *clusterv1.Cluster, infraCluster *unstructured.Unstructured, infraClusterIsNotFound bool) {
	// infrastructure is not yet set and the cluster is using ClusterClass.
	if !cluster.Spec.InfrastructureRef.IsDefined() && cluster.Spec.Topology.IsDefined() {
		message := ""
		if cluster.DeletionTimestamp.IsZero() {
			message = "Waiting for cluster topology to be reconciled" //nolint:goconst // Not making this a constant for now
		}
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterInfrastructureDoesNotExistReason,
			Message: message,
		})
		return
	}

	if infraCluster != nil {
		infrastructureProvisioned := ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false)
		ready, err := conditions.NewMirrorConditionFromUnstructured(
			infraCluster,
			contract.InfrastructureCluster().ReadyConditionType(), conditions.TargetConditionType(clusterv1.ClusterInfrastructureReadyCondition),
			conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(infrastructureProvisioned),
				Reason:  fallbackReason(infrastructureProvisioned, clusterv1.ClusterInfrastructureReadyReason, clusterv1.ClusterInfrastructureNotReadyReason),
				Message: infrastructureReadyFallBackMessage(cluster.Spec.InfrastructureRef.Kind, infrastructureProvisioned),
			},
		)
		if err != nil {
			conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterInfrastructureInvalidConditionReportedReason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if ready.Reason == conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.ClusterInfrastructureReadyReason
		}
		conditions.Set(cluster, *ready)
		return
	}

	// If we got errors in reading the infra cluster (this should happen rarely), surface them
	if !infraClusterIsNotFound {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterInfrastructureInternalErrorReason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileInfrastructure.
		})
		return
	}

	// Infra cluster missing when the cluster is deleting.
	if !cluster.DeletionTimestamp.IsZero() {
		if ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
			conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDeletedReason,
				Message: fmt.Sprintf("%s has been deleted", cluster.Spec.InfrastructureRef.Kind),
			})
			return
		}

		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterInfrastructureDoesNotExistReason,
			Message: fmt.Sprintf("%s does not exist", cluster.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// Report an issue if infra cluster missing after the cluster has been initialized (and the cluster is still running).
	if ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterInfrastructureDeletedReason,
			Message: fmt.Sprintf("%s has been deleted while the cluster still exists", cluster.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// If the cluster is not deleting, and infra cluster object does not exist yet,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the cluster and all the objects referenced by it (provisioning yet to start/started, but status.InfrastructureReady not yet set).
	conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterInfrastructureReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.ClusterInfrastructureDoesNotExistReason,
		Message: fmt.Sprintf("%s does not exist", cluster.Spec.InfrastructureRef.Kind),
	})
}

func setControlPlaneAvailableCondition(_ context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, controlPlaneIsNotFound bool) {
	// control plane is not yet set and the cluster is using ClusterClass.
	if !cluster.Spec.ControlPlaneRef.IsDefined() && cluster.Spec.Topology.IsDefined() {
		message := ""
		if cluster.DeletionTimestamp.IsZero() {
			message = "Waiting for cluster topology to be reconciled"
		}
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
			Message: message,
		})
		return
	}

	if controlPlane != nil {
		controlPlaneInitialized := ptr.Deref(cluster.Status.Initialization.ControlPlaneInitialized, false)
		available, err := conditions.NewMirrorConditionFromUnstructured(
			controlPlane,
			contract.ControlPlane().AvailableConditionType(), conditions.TargetConditionType(clusterv1.ClusterControlPlaneAvailableCondition),
			conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(controlPlaneInitialized),
				Reason:  fallbackReason(controlPlaneInitialized, clusterv1.ClusterControlPlaneAvailableReason, clusterv1.ClusterControlPlaneNotAvailableReason),
				Message: controlPlaneAvailableFallBackMessage(cluster.Spec.ControlPlaneRef.Kind, controlPlaneInitialized),
			},
		)
		if err != nil {
			conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInvalidConditionReportedReason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if available.Reason == conditions.NoReasonReported && available.Status == metav1.ConditionTrue {
			available.Reason = clusterv1.ClusterControlPlaneAvailableReason
		}
		conditions.Set(cluster, *available)
		return
	}

	// If we got errors in reading the control plane (this should happen rarely), surface them
	if !controlPlaneIsNotFound {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterControlPlaneInternalErrorReason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileControlPlane.
		})
		return
	}

	// Infra cluster missing when the cluster is deleting.
	if !cluster.DeletionTimestamp.IsZero() {
		if ptr.Deref(cluster.Status.Initialization.ControlPlaneInitialized, false) {
			conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDeletedReason,
				Message: fmt.Sprintf("%s has been deleted", cluster.Spec.ControlPlaneRef.Kind),
			})
			return
		}

		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
			Message: fmt.Sprintf("%s does not exist", cluster.Spec.ControlPlaneRef.Kind),
		})
		return
	}

	// Report an issue if control plane missing after the cluster has been initialized (and the cluster is still running).
	if ptr.Deref(cluster.Status.Initialization.ControlPlaneInitialized, false) {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneDeletedReason,
			Message: fmt.Sprintf("%s has been deleted while the cluster still exists", cluster.Spec.ControlPlaneRef.Kind),
		})
		return
	}

	// If the cluster is not deleting, and control plane object does not exist yet,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the cluster and all the objects referenced by it (provisioning yet to start/started, but status.ControlPlaneReady not yet set).
	conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterControlPlaneAvailableCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
		Message: fmt.Sprintf("%s does not exist", cluster.Spec.ControlPlaneRef.Kind),
	})
}

func setControlPlaneInitializedCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, contractVersion string, controlPlaneMachines collections.Machines, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// No-op if control plane is already initialized.
	if conditions.IsTrue(cluster, clusterv1.ClusterControlPlaneInitializedCondition) {
		return
	}

	// control plane is not yet set and the cluster is using ClusterClass.
	if !cluster.Spec.ControlPlaneRef.IsDefined() && cluster.Spec.Topology.IsDefined() {
		message := ""
		if cluster.DeletionTimestamp.IsZero() {
			message = "Waiting for cluster topology to be reconciled"
		}
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneInitializedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
			Message: message,
		})
		return
	}

	// If this cluster is using a control plane object, get control plane initialized from this object.
	if cluster.Spec.ControlPlaneRef.IsDefined() {
		if controlPlane == nil {
			if !controlPlaneIsNotFound {
				conditions.Set(cluster, metav1.Condition{
					Type:    clusterv1.ClusterControlPlaneInitializedCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorReason,
					Message: "Please check controller logs for errors",
				})
				return
			}

			conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistReason,
				Message: fmt.Sprintf("%s does not exist", cluster.Spec.ControlPlaneRef.Kind),
			})
			return
		}

		// Determine if the ControlPlane is provisioned.
		initialized, err := contract.ControlPlane().Initialized(contractVersion).Get(controlPlane)
		if err != nil {
			if !errors.Is(err, contract.ErrFieldNotFound) {
				log.Error(err, fmt.Sprintf("Failed to get status.initialized from %s", cluster.Spec.ControlPlaneRef.Kind))
				conditions.Set(cluster, metav1.Condition{
					Type:    clusterv1.ClusterControlPlaneInitializedCondition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorReason,
					Message: "Please check controller logs for errors",
				})
				return
			}
			initialized = ptr.To(false)
		}

		if *initialized {
			conditions.Set(cluster, metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedReason,
			})
			return
		}

		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneInitializedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneNotInitializedReason,
			Message: "Control plane not yet initialized",
		})
		return
	}

	// Otherwise this cluster control plane is composed by stand-alone machines, and initialized is assumed true
	// when at least one of those machines has a node.

	if !getDescendantsSucceeded {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneInitializedCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(controlPlaneMachines.Filter(collections.HasNode())) > 0 {
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterControlPlaneInitializedCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.ClusterControlPlaneInitializedReason,
		})
		return
	}

	conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterControlPlaneInitializedCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.ClusterControlPlaneNotInitializedReason,
		Message: "Waiting for the first control plane machine to have status.nodeRef set",
	})
}

func setWorkersAvailableCondition(ctx context.Context, cluster *clusterv1.Cluster, machinePools clusterv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in listing descendants (this should never happen), surface it.
	if !getDescendantsSucceeded {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterWorkersAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterWorkersAvailableInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machinePools.Items)+len(machineDeployments.Items) == 0 {
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterWorkersAvailableCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.ClusterWorkersAvailableNoWorkersReason,
		})
		return
	}

	ws := make([]aggregationWrapper, 0, len(machinePools.Items)+len(machineDeployments.Items))
	for _, mp := range machinePools.Items {
		ws = append(ws, aggregationWrapper{mp: &mp})
	}
	for _, md := range machineDeployments.Items {
		ws = append(ws, aggregationWrapper{md: &md})
	}

	workersAvailableCondition, err := conditions.NewAggregateCondition(
		ws, clusterv1.AvailableCondition,
		conditions.TargetConditionType(clusterv1.ClusterWorkersAvailableCondition),
		// Using a custom merge strategy to override reasons applied during merge
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterWorkersNotAvailableReason,
					clusterv1.ClusterWorkersAvailableUnknownReason,
					clusterv1.ClusterWorkersAvailableReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate MachinePool and MachineDeployment's Available conditions")
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterWorkersAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterWorkersAvailableInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(cluster, *workersAvailableCondition)
}

func setControlPlaneMachinesReadyCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	machinesConditionSetter{
		condition:                   clusterv1.ClusterControlPlaneMachinesReadyCondition,
		machineAggregationCondition: clusterv1.MachineReadyCondition,
		internalErrorReason:         clusterv1.ClusterControlPlaneMachinesReadyInternalErrorReason,
		noReplicasReason:            clusterv1.ClusterControlPlaneMachinesReadyNoReplicasReason,
		issueReason:                 clusterv1.ClusterControlPlaneMachinesNotReadyReason,
		unknownReason:               clusterv1.ClusterControlPlaneMachinesReadyUnknownReason,
		infoReason:                  clusterv1.ClusterControlPlaneMachinesReadyReason,
	}.setMachinesCondition(ctx, cluster, machines, getDescendantsSucceeded)
}

func setWorkerMachinesReadyCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	machinesConditionSetter{
		condition:                   clusterv1.ClusterWorkerMachinesReadyCondition,
		machineAggregationCondition: clusterv1.MachineReadyCondition,
		internalErrorReason:         clusterv1.ClusterWorkerMachinesReadyInternalErrorReason,
		noReplicasReason:            clusterv1.ClusterWorkerMachinesReadyNoReplicasReason,
		issueReason:                 clusterv1.ClusterWorkerMachinesNotReadyReason,
		unknownReason:               clusterv1.ClusterWorkerMachinesReadyUnknownReason,
		infoReason:                  clusterv1.ClusterWorkerMachinesReadyReason,
	}.setMachinesCondition(ctx, cluster, machines, getDescendantsSucceeded)
}

func setControlPlaneMachinesUpToDateCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	machines = machines.Filter(func(machine *clusterv1.Machine) bool {
		return conditions.Has(machine, clusterv1.MachineUpToDateCondition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second
	})

	machinesConditionSetter{
		condition:                   clusterv1.ClusterControlPlaneMachinesUpToDateCondition,
		machineAggregationCondition: clusterv1.MachineUpToDateCondition,
		internalErrorReason:         clusterv1.ClusterControlPlaneMachinesUpToDateInternalErrorReason,
		noReplicasReason:            clusterv1.ClusterControlPlaneMachinesUpToDateNoReplicasReason,
		issueReason:                 clusterv1.ClusterControlPlaneMachinesNotUpToDateReason,
		unknownReason:               clusterv1.ClusterControlPlaneMachinesUpToDateUnknownReason,
		infoReason:                  clusterv1.ClusterControlPlaneMachinesUpToDateReason,
	}.setMachinesCondition(ctx, cluster, machines, getDescendantsSucceeded)
}

func setWorkerMachinesUpToDateCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	machines = machines.Filter(func(machine *clusterv1.Machine) bool {
		return conditions.Has(machine, clusterv1.MachineUpToDateCondition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second
	})

	machinesConditionSetter{
		condition:                   clusterv1.ClusterWorkerMachinesUpToDateCondition,
		machineAggregationCondition: clusterv1.MachineUpToDateCondition,
		internalErrorReason:         clusterv1.ClusterWorkerMachinesUpToDateInternalErrorReason,
		noReplicasReason:            clusterv1.ClusterWorkerMachinesUpToDateNoReplicasReason,
		issueReason:                 clusterv1.ClusterWorkerMachinesNotUpToDateReason,
		unknownReason:               clusterv1.ClusterWorkerMachinesUpToDateUnknownReason,
		infoReason:                  clusterv1.ClusterWorkerMachinesUpToDateReason,
	}.setMachinesCondition(ctx, cluster, machines, getDescendantsSucceeded)
}

type machinesConditionSetter struct {
	condition                   string
	machineAggregationCondition string
	internalErrorReason         string
	noReplicasReason            string
	issueReason                 string
	unknownReason               string
	infoReason                  string
}

func (s machinesConditionSetter) setMachinesCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in listing descendants (this should never happen), surface it.
	if !getDescendantsSucceeded {
		conditions.Set(cluster, metav1.Condition{
			Type:    s.condition,
			Status:  metav1.ConditionUnknown,
			Reason:  s.internalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machines) == 0 {
		conditions.Set(cluster, metav1.Condition{
			Type:   s.condition,
			Status: metav1.ConditionTrue,
			Reason: s.noReplicasReason,
		})
		return
	}

	machinesCondition, err := conditions.NewAggregateCondition(
		machines.UnsortedList(), s.machineAggregationCondition,
		conditions.TargetConditionType(s.condition),
		// Using a custom merge strategy to override reasons applied during merge
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					s.issueReason,
					s.unknownReason,
					s.infoReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", s.machineAggregationCondition))
		conditions.Set(cluster, metav1.Condition{
			Type:    s.condition,
			Status:  metav1.ConditionUnknown,
			Reason:  s.internalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(cluster, *machinesCondition)
}

func setRemediatingCondition(ctx context.Context, cluster *clusterv1.Cluster, machinesToBeRemediated, unhealthyMachines collections.Machines, getMachinesSucceeded bool) {
	if !getMachinesSucceeded {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemediatingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRemediatingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machinesToBeRemediated) == 0 {
		message := aggregateUnhealthyMachines(unhealthyMachines)
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemediatingCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterNotRemediatingReason,
			Message: message,
		})
		return
	}

	remediatingCondition, err := conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedCondition,
		conditions.TargetConditionType(clusterv1.ClusterRemediatingCondition),
		// Note: in case of the remediating conditions it is not required to use a CustomMergeStrategy/ComputeReasonFunc
		// because we are considering only machinesToBeRemediated (and we can pin the reason when we set the condition).
	)
	if err != nil {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemediatingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRemediatingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineOwnerRemediatedCondition))
		return
	}

	conditions.Set(cluster, metav1.Condition{
		Type:    remediatingCondition.Type,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.ClusterRemediatingReason,
		Message: remediatingCondition.Message,
	})
}

func setRollingOutCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, machinePools clusterv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in getting control plane or listing descendants (this should never happen), surface it.
	if (cluster.Spec.ControlPlaneRef.IsDefined() && controlPlane == nil && !controlPlaneIsNotFound) || !getDescendantsSucceeded {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRollingOutCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRollingOutInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	ws := make([]aggregationWrapper, 0, len(machinePools.Items)+len(machineDeployments.Items)+1)
	if controlPlane != nil {
		// control plane is considered only if it is reporting the condition (the contract does not require conditions to be reported)
		// Note: this implies that it won't surface as "Conditions RollingOut not yet reported from ...".
		if c, err := conditions.UnstructuredGet(controlPlane, clusterv1.RollingOutCondition); err == nil && c != nil {
			ws = append(ws, aggregationWrapper{cp: controlPlane})
		}
	}
	for _, mp := range machinePools.Items {
		ws = append(ws, aggregationWrapper{mp: &mp})
	}
	for _, md := range machineDeployments.Items {
		ws = append(ws, aggregationWrapper{md: &md})
	}

	if len(ws) == 0 {
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterRollingOutCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotRollingOutReason,
		})
		return
	}

	rollingOutCondition, err := conditions.NewAggregateCondition(
		ws, clusterv1.RollingOutCondition,
		conditions.TargetConditionType(clusterv1.ClusterRollingOutCondition),
		// Instruct aggregate to consider RollingOut condition with negative polarity.
		conditions.NegativePolarityConditionTypes{clusterv1.RollingOutCondition},
		// Using a custom merge strategy to override reasons applied during merge and to ensure merge
		// takes into account the fact the RollingOut has negative polarity.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.TargetConditionHasPositivePolarity(false),
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterRollingOutReason,
					clusterv1.ClusterRollingOutUnknownReason,
					clusterv1.ClusterNotRollingOutReason,
				)),
				conditions.GetPriorityFunc(conditions.GetDefaultMergePriorityFunc(clusterv1.RollingOutCondition)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate ControlPlane, MachinePool, MachineDeployment's RollingOut conditions")
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRollingOutCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRollingOutInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(cluster, *rollingOutCondition)
}

func setScalingUpCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, machinePools clusterv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, machineSets clusterv1.MachineSetList, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in getting control plane or listing descendants (this should never happen), surface it.
	if (cluster.Spec.ControlPlaneRef.IsDefined() && controlPlane == nil && !controlPlaneIsNotFound) || !getDescendantsSucceeded {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingUpCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingUpInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	ws := make([]aggregationWrapper, 0, len(machinePools.Items)+len(machineDeployments.Items)+1)
	if controlPlane != nil {
		// control plane is considered only if it is reporting the condition (the contract does not require conditions to be reported)
		// Note: this implies that it won't surface as "Conditions ScalingUp not yet reported from ...".
		if c, err := conditions.UnstructuredGet(controlPlane, clusterv1.ScalingUpCondition); err == nil && c != nil {
			ws = append(ws, aggregationWrapper{cp: controlPlane})
		}
	}
	for _, mp := range machinePools.Items {
		ws = append(ws, aggregationWrapper{mp: &mp})
	}
	for _, md := range machineDeployments.Items {
		ws = append(ws, aggregationWrapper{md: &md})
	}
	for _, ms := range machineSets.Items {
		if !util.IsOwnedByObject(&ms, cluster) {
			continue
		}
		ws = append(ws, aggregationWrapper{ms: &ms})
	}

	if len(ws) == 0 {
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterScalingUpCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotScalingUpReason,
		})
		return
	}

	scalingUpCondition, err := conditions.NewAggregateCondition(
		ws, clusterv1.ScalingUpCondition,
		conditions.TargetConditionType(clusterv1.ClusterScalingUpCondition),
		// Instruct aggregate to consider ScalingUp condition with negative polarity.
		conditions.NegativePolarityConditionTypes{clusterv1.ScalingUpCondition},
		// Using a custom merge strategy to override reasons applied during merge and to ensure merge
		// takes into account the fact the ScalingUp has negative polarity.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.TargetConditionHasPositivePolarity(false),
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterScalingUpReason,
					clusterv1.ClusterScalingUpUnknownReason,
					clusterv1.ClusterNotScalingUpReason,
				)),
				conditions.GetPriorityFunc(conditions.GetDefaultMergePriorityFunc(clusterv1.ScalingUpCondition)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate ControlPlane, MachinePool, MachineDeployment, MachineSet's ScalingUp conditions")
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingUpCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingUpInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(cluster, *scalingUpCondition)
}

func setScalingDownCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, machinePools clusterv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, machineSets clusterv1.MachineSetList, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in getting control plane or listing descendants (this should never happen), surface it.
	if (cluster.Spec.ControlPlaneRef.IsDefined() && controlPlane == nil && !controlPlaneIsNotFound) || !getDescendantsSucceeded {
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingDownCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingDownInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	ws := make([]aggregationWrapper, 0, len(machinePools.Items)+len(machineDeployments.Items)+1)
	if controlPlane != nil {
		// control plane is considered only if it is reporting the condition (the contract does not require conditions to be reported)
		// Note: this implies that it won't surface as "Conditions ScalingDown not yet reported from ...".
		if c, err := conditions.UnstructuredGet(controlPlane, clusterv1.ScalingDownCondition); err == nil && c != nil {
			ws = append(ws, aggregationWrapper{cp: controlPlane})
		}
	}
	for _, mp := range machinePools.Items {
		ws = append(ws, aggregationWrapper{mp: &mp})
	}
	for _, md := range machineDeployments.Items {
		ws = append(ws, aggregationWrapper{md: &md})
	}
	for _, ms := range machineSets.Items {
		if !util.IsOwnedByObject(&ms, cluster) {
			continue
		}
		ws = append(ws, aggregationWrapper{ms: &ms})
	}

	if len(ws) == 0 {
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterScalingDownCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotScalingDownReason,
		})
		return
	}

	scalingDownCondition, err := conditions.NewAggregateCondition(
		ws, clusterv1.ScalingDownCondition,
		conditions.TargetConditionType(clusterv1.ClusterScalingDownCondition),
		// Instruct aggregate to consider ScalingDown condition with negative polarity.
		conditions.NegativePolarityConditionTypes{clusterv1.ScalingDownCondition},
		// Using a custom merge strategy to override reasons applied during merge and to ensure merge
		// takes into account the fact the ScalingDown has negative polarity.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.TargetConditionHasPositivePolarity(false),
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterScalingDownReason,
					clusterv1.ClusterScalingDownUnknownReason,
					clusterv1.ClusterNotScalingDownReason,
				)),
				conditions.GetPriorityFunc(conditions.GetDefaultMergePriorityFunc(clusterv1.ScalingDownCondition)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate ControlPlane, MachinePool, MachineDeployment, MachineSet's ScalingDown conditions")
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingDownCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingDownInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	conditions.Set(cluster, *scalingDownCondition)
}

func setDeletingCondition(_ context.Context, cluster *clusterv1.Cluster, deletingReason, deletingMessage string) {
	if cluster.DeletionTimestamp.IsZero() {
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterDeletingCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotDeletingReason,
		})
		return
	}

	conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterDeletingCondition,
		Status:  metav1.ConditionTrue,
		Reason:  deletingReason,
		Message: deletingMessage,
	})
}

type clusterConditionCustomMergeStrategy struct {
	cluster                        *clusterv1.Cluster
	negativePolarityConditionTypes []string
}

func (c clusterConditionCustomMergeStrategy) Merge(operation conditions.MergeOperation, mergeConditions []conditions.ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error) {
	return conditions.DefaultMergeStrategy(conditions.GetPriorityFunc(
		func(condition metav1.Condition) conditions.MergePriority {
			// While cluster is deleting, treat unknown conditions from external objects as info (it is ok that those objects have been deleted at this stage).
			if !c.cluster.DeletionTimestamp.IsZero() {
				if condition.Type == clusterv1.ClusterInfrastructureReadyCondition && (condition.Reason == clusterv1.ClusterInfrastructureDeletedReason || condition.Reason == clusterv1.ClusterInfrastructureDoesNotExistReason) {
					return conditions.InfoMergePriority
				}
				if condition.Type == clusterv1.ClusterControlPlaneAvailableCondition && (condition.Reason == clusterv1.ClusterControlPlaneDeletedReason || condition.Reason == clusterv1.ClusterControlPlaneDoesNotExistReason) {
					return conditions.InfoMergePriority
				}
			}

			// Treat all reasons except TopologyReconcileFailed and ClusterClassNotReconciled of TopologyReconciled condition as info.
			if condition.Type == clusterv1.ClusterTopologyReconciledCondition && condition.Status == metav1.ConditionFalse &&
				condition.Reason != clusterv1.ClusterTopologyReconciledFailedReason && condition.Reason != clusterv1.ClusterTopologyReconciledClusterClassNotReconciledReason {
				return conditions.InfoMergePriority
			}
			return conditions.GetDefaultMergePriorityFunc(c.negativePolarityConditionTypes...)(condition)
		}),
		conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
			clusterv1.ClusterNotAvailableReason,
			clusterv1.ClusterAvailableUnknownReason,
			clusterv1.ClusterAvailableReason,
		)),
	).Merge(operation, mergeConditions, conditionTypes)
}

func setAvailableCondition(ctx context.Context, cluster *clusterv1.Cluster, clusterClass *clusterv1.ClusterClass) {
	log := ctrl.LoggerFrom(ctx)

	forConditionTypes := conditions.ForConditionTypes{
		clusterv1.ClusterDeletingCondition,
		clusterv1.ClusterRemoteConnectionProbeCondition,
		clusterv1.ClusterInfrastructureReadyCondition,
		clusterv1.ClusterControlPlaneAvailableCondition,
		clusterv1.ClusterWorkersAvailableCondition,
		clusterv1.ClusterTopologyReconciledCondition,
	}
	negativePolarityConditionTypes := []string{clusterv1.ClusterDeletingCondition}
	availabilityGates := cluster.Spec.AvailabilityGates
	if availabilityGates == nil && clusterClass != nil {
		availabilityGates = clusterClass.Spec.AvailabilityGates
	}
	for _, g := range availabilityGates {
		forConditionTypes = append(forConditionTypes, g.ConditionType)
		if g.Polarity == clusterv1.NegativePolarityCondition {
			negativePolarityConditionTypes = append(negativePolarityConditionTypes, g.ConditionType)
		}
	}

	summaryOpts := []conditions.SummaryOption{
		forConditionTypes,
		// Instruct summary to consider Deleting condition with negative polarity.
		conditions.NegativePolarityConditionTypes{clusterv1.ClusterDeletingCondition},
		// Using a custom merge strategy to override reasons applied during merge and to ignore some
		// info message so the available condition is less noisy.
		conditions.CustomMergeStrategy{
			MergeStrategy: clusterConditionCustomMergeStrategy{
				cluster: cluster,
				// Instruct merge to consider Deleting condition with negative polarity,
				negativePolarityConditionTypes: negativePolarityConditionTypes,
			},
		},
	}
	if !cluster.Spec.Topology.IsDefined() {
		summaryOpts = append(summaryOpts, conditions.IgnoreTypesIfMissing{clusterv1.ClusterTopologyReconciledCondition})
	}

	availableCondition, err := conditions.NewSummaryCondition(cluster, clusterv1.ClusterAvailableCondition, summaryOpts...)

	if err != nil {
		// Note, this could only happen if we hit edge cases in computing the summary, which should not happen due to the fact
		// that we are passing a non empty list of ForConditionTypes.
		log.Error(err, "Failed to set Available condition")
		availableCondition = &metav1.Condition{
			Type:    clusterv1.ClusterAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterAvailableInternalErrorReason,
			Message: "Please check controller logs for errors",
		}
	}

	conditions.Set(cluster, *availableCondition)
}

func fallbackReason(status bool, trueReason, falseReason string) string {
	if status {
		return trueReason
	}
	return falseReason
}

func infrastructureReadyFallBackMessage(kind string, ready bool) string {
	if ready {
		return ""
	}
	return fmt.Sprintf("%s status.initialization.provisioned is %t", kind, ready)
}

func controlPlaneAvailableFallBackMessage(kind string, ready bool) string {
	if ready {
		return ""
	}
	return fmt.Sprintf("%s status.initialization.controlPlaneInitialized is %t", kind, ready)
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
	message += "not healthy (not to be remediated)"

	return message
}

type aggregationWrapper struct {
	cp *unstructured.Unstructured
	mp *clusterv1.MachinePool
	md *clusterv1.MachineDeployment
	ms *clusterv1.MachineSet
}

func (w aggregationWrapper) GetObjectKind() schema.ObjectKind {
	switch {
	case w.cp != nil:
		return w.cp.GetObjectKind()
	case w.mp != nil:
		w.mp.APIVersion = clusterv1.GroupVersion.String()
		w.mp.Kind = "MachinePool"
		return w.mp.GetObjectKind()
	case w.md != nil:
		w.md.APIVersion = clusterv1.GroupVersion.String()
		w.md.Kind = "MachineDeployment"
		return w.md.GetObjectKind()
	case w.ms != nil:
		w.ms.APIVersion = clusterv1.GroupVersion.String()
		w.ms.Kind = "MachineSet"
		return w.ms.GetObjectKind()
	}
	panic("not supported")
}

func (w aggregationWrapper) DeepCopyObject() runtime.Object {
	panic("not supported")
}

func (w aggregationWrapper) GetConditions() []metav1.Condition {
	switch {
	case w.cp != nil:
		if c, err := conditions.UnstructuredGetAll(w.cp); err == nil && c != nil {
			return c
		}
		return nil
	case w.mp != nil:
		return w.mp.GetConditions()
	case w.md != nil:
		return w.md.GetConditions()
	case w.ms != nil:
		return w.ms.GetConditions()
	}
	panic("not supported")
}

func (w aggregationWrapper) GetName() string {
	switch {
	case w.cp != nil:
		return w.cp.GetName()
	case w.mp != nil:
		return w.mp.GetName()
	case w.md != nil:
		return w.md.GetName()
	case w.ms != nil:
		return w.ms.GetName()
	}
	panic("not supported")
}

func (w aggregationWrapper) GetLabels() map[string]string {
	switch {
	case w.cp != nil:
		return w.cp.GetLabels()
	case w.mp != nil:
		return w.mp.GetLabels()
	case w.md != nil:
		return w.md.GetLabels()
	case w.ms != nil:
		return w.ms.GetLabels()
	}
	panic("not supported")
}
