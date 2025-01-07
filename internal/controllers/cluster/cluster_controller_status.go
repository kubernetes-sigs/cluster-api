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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	clog "sigs.k8s.io/cluster-api/util/log"
)

func (r *Reconciler) updateStatus(ctx context.Context, s *scope) {
	// Always reconcile the Status.Phase field.
	r.reconcilePhase(ctx, s.cluster)

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
	setControlPlaneReplicas(ctx, s.cluster, s.controlPlane, s.descendants.controlPlaneMachines, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setWorkersReplicas(ctx, s.cluster, expv1.MachinePoolList{}, s.descendants.machineDeployments, s.descendants.machineSets, workerMachines, s.getDescendantsSucceeded)

	// conditions
	setInfrastructureReadyCondition(ctx, s.cluster, s.infraCluster, s.infraClusterIsNotFound)
	setControlPlaneAvailableCondition(ctx, s.cluster, s.controlPlane, s.controlPlaneIsNotFound)
	setControlPlaneInitializedCondition(ctx, s.cluster, s.controlPlane, s.descendants.controlPlaneMachines, s.infraClusterIsNotFound, s.getDescendantsSucceeded)
	setWorkersAvailableCondition(ctx, s.cluster, expv1.MachinePoolList{}, s.descendants.machineDeployments, s.getDescendantsSucceeded)
	setControlPlaneMachinesReadyCondition(ctx, s.cluster, controlPlaneMachines, s.getDescendantsSucceeded)
	setWorkerMachinesReadyCondition(ctx, s.cluster, workerMachines, s.getDescendantsSucceeded)
	setControlPlaneMachinesUpToDateCondition(ctx, s.cluster, controlPlaneMachines, s.getDescendantsSucceeded)
	setWorkerMachinesUpToDateCondition(ctx, s.cluster, workerMachines, s.getDescendantsSucceeded)
	setRollingOutCondition(ctx, s.cluster, s.controlPlane, expv1.MachinePoolList{}, s.descendants.machineDeployments, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setScalingUpCondition(ctx, s.cluster, s.controlPlane, expv1.MachinePoolList{}, s.descendants.machineDeployments, s.descendants.machineSets, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setScalingDownCondition(ctx, s.cluster, s.controlPlane, expv1.MachinePoolList{}, s.descendants.machineDeployments, s.descendants.machineSets, s.controlPlaneIsNotFound, s.getDescendantsSucceeded)
	setRemediatingCondition(ctx, s.cluster, machinesToBeRemediated, unhealthyMachines, s.getDescendantsSucceeded)
	setDeletingCondition(ctx, s.cluster, s.deletingReason, s.deletingMessage)
	setAvailableCondition(ctx, s.cluster)
}

func setControlPlaneReplicas(_ context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, controlPlaneMachines collections.Machines, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	if cluster.Status.V1Beta2 == nil {
		cluster.Status.V1Beta2 = &clusterv1.ClusterV1Beta2Status{}
	}

	if cluster.Status.V1Beta2.ControlPlane == nil {
		cluster.Status.V1Beta2.ControlPlane = &clusterv1.ClusterControlPlaneStatus{}
	}

	// If this cluster is using a control plane object, surface the replica counters reported by it.
	// Note: The cluster API contract does not require that a control plane object has a notion of replicas;
	// if the control plane object does have a notion of replicas, or expected replicas fields are not provided,
	// corresponding replicas will be left empty.
	if cluster.Spec.ControlPlaneRef != nil || cluster.Spec.Topology != nil {
		if controlPlane == nil || controlPlaneIsNotFound {
			cluster.Status.V1Beta2.ControlPlane.Replicas = nil
			cluster.Status.V1Beta2.ControlPlane.ReadyReplicas = nil
			cluster.Status.V1Beta2.ControlPlane.AvailableReplicas = nil
			cluster.Status.V1Beta2.ControlPlane.UpToDateReplicas = nil
			cluster.Status.V1Beta2.ControlPlane.DesiredReplicas = nil
			return
		}

		if replicas, err := contract.ControlPlane().Replicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.V1Beta2.ControlPlane.DesiredReplicas = ptr.To(int32(*replicas))
		}
		if replicas, err := contract.ControlPlane().StatusReplicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.V1Beta2.ControlPlane.Replicas = ptr.To(int32(*replicas))
		}
		if replicas, err := contract.ControlPlane().V1Beta2ReadyReplicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.V1Beta2.ControlPlane.ReadyReplicas = replicas
		}
		if replicas, err := contract.ControlPlane().V1Beta2AvailableReplicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.V1Beta2.ControlPlane.AvailableReplicas = replicas
		}
		if replicas, err := contract.ControlPlane().V1Beta2UpToDateReplicas().Get(controlPlane); err == nil && replicas != nil {
			cluster.Status.V1Beta2.ControlPlane.UpToDateReplicas = replicas
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
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineReadyV1Beta2Condition) {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + 1)
		}
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineAvailableV1Beta2Condition) {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + 1)
		}
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineUpToDateV1Beta2Condition) {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + 1)
		}
	}

	cluster.Status.V1Beta2.ControlPlane.Replicas = replicas
	cluster.Status.V1Beta2.ControlPlane.ReadyReplicas = readyReplicas
	cluster.Status.V1Beta2.ControlPlane.AvailableReplicas = availableReplicas
	cluster.Status.V1Beta2.ControlPlane.UpToDateReplicas = upToDateReplicas

	// There is no concept of desired replicas for stand-alone machines, but for sake of consistency
	// we consider the fact that the machine has been creates as equivalent to the intent to have one replica.
	cluster.Status.V1Beta2.ControlPlane.DesiredReplicas = replicas
}

func setWorkersReplicas(_ context.Context, cluster *clusterv1.Cluster, machinePools expv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, machineSets clusterv1.MachineSetList, workerMachines collections.Machines, getDescendantsSucceeded bool) {
	if cluster.Status.V1Beta2 == nil {
		cluster.Status.V1Beta2 = &clusterv1.ClusterV1Beta2Status{}
	}

	if cluster.Status.V1Beta2.Workers == nil {
		cluster.Status.V1Beta2.Workers = &clusterv1.WorkersStatus{}
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
		currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + mp.Status.Replicas)
		if mp.Status.V1Beta2 != nil && mp.Status.V1Beta2.ReadyReplicas != nil {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + *mp.Status.V1Beta2.ReadyReplicas)
		}
		if mp.Status.V1Beta2 != nil && mp.Status.V1Beta2.AvailableReplicas != nil {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + *mp.Status.V1Beta2.AvailableReplicas)
		}
		if mp.Status.V1Beta2 != nil && mp.Status.V1Beta2.UpToDateReplicas != nil {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + *mp.Status.V1Beta2.UpToDateReplicas)
		}
	}

	for _, md := range machineDeployments.Items {
		if md.Spec.Replicas != nil {
			desiredReplicas = ptr.To(ptr.Deref(desiredReplicas, 0) + *md.Spec.Replicas)
		}
		currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + md.Status.Replicas)
		if md.Status.V1Beta2 != nil && md.Status.V1Beta2.ReadyReplicas != nil {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + *md.Status.V1Beta2.ReadyReplicas)
		}
		if md.Status.V1Beta2 != nil && md.Status.V1Beta2.AvailableReplicas != nil {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + *md.Status.V1Beta2.AvailableReplicas)
		}
		if md.Status.V1Beta2 != nil && md.Status.V1Beta2.UpToDateReplicas != nil {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + *md.Status.V1Beta2.UpToDateReplicas)
		}
	}

	for _, ms := range machineSets.Items {
		if !util.IsOwnedByObject(&ms, cluster) {
			continue
		}
		if ms.Spec.Replicas != nil {
			desiredReplicas = ptr.To(ptr.Deref(desiredReplicas, 0) + *ms.Spec.Replicas)
		}
		currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + ms.Status.Replicas)
		if ms.Status.V1Beta2 != nil && ms.Status.V1Beta2.ReadyReplicas != nil {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + *ms.Status.V1Beta2.ReadyReplicas)
		}
		if ms.Status.V1Beta2 != nil && ms.Status.V1Beta2.AvailableReplicas != nil {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + *ms.Status.V1Beta2.AvailableReplicas)
		}
		if ms.Status.V1Beta2 != nil && ms.Status.V1Beta2.UpToDateReplicas != nil {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + *ms.Status.V1Beta2.UpToDateReplicas)
		}
	}

	for _, m := range workerMachines.UnsortedList() {
		if !util.IsOwnedByObject(m, cluster) {
			continue
		}
		currentReplicas = ptr.To(ptr.Deref(currentReplicas, 0) + 1)
		if v1beta2conditions.IsTrue(m, clusterv1.MachineReadyV1Beta2Condition) {
			readyReplicas = ptr.To(ptr.Deref(readyReplicas, 0) + 1)
		}
		if v1beta2conditions.IsTrue(m, clusterv1.MachineAvailableV1Beta2Condition) {
			availableReplicas = ptr.To(ptr.Deref(availableReplicas, 0) + 1)
		}
		if v1beta2conditions.IsTrue(m, clusterv1.MachineUpToDateV1Beta2Condition) {
			upToDateReplicas = ptr.To(ptr.Deref(upToDateReplicas, 0) + 1)
		}

		// There is no concept of desired replicas for stand-alone machines, but for sake of consistency
		// we consider the fact that the machine has been creates as equivalent to the intent to have one replica.
		desiredReplicas = ptr.To(ptr.Deref(desiredReplicas, 0) + 1)
	}

	cluster.Status.V1Beta2.Workers.DesiredReplicas = desiredReplicas
	cluster.Status.V1Beta2.Workers.Replicas = currentReplicas
	cluster.Status.V1Beta2.Workers.ReadyReplicas = readyReplicas
	cluster.Status.V1Beta2.Workers.AvailableReplicas = availableReplicas
	cluster.Status.V1Beta2.Workers.UpToDateReplicas = upToDateReplicas
}

func setInfrastructureReadyCondition(_ context.Context, cluster *clusterv1.Cluster, infraCluster *unstructured.Unstructured, infraClusterIsNotFound bool) {
	// infrastructure is not yet set and the cluster is using ClusterClass.
	if cluster.Spec.InfrastructureRef == nil && cluster.Spec.Topology != nil {
		message := ""
		if cluster.DeletionTimestamp.IsZero() {
			message = "Waiting for cluster topology to be reconciled" //nolint:goconst // Not making this a constant for now
		}
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason,
			Message: message,
		})
		return
	}

	if infraCluster != nil {
		ready, err := v1beta2conditions.NewMirrorConditionFromUnstructured(
			infraCluster,
			contract.InfrastructureCluster().ReadyConditionType(), v1beta2conditions.TargetConditionType(clusterv1.ClusterInfrastructureReadyV1Beta2Condition),
			v1beta2conditions.FallbackCondition{
				Status:  v1beta2conditions.BoolToStatus(cluster.Status.InfrastructureReady),
				Reason:  fallbackReason(cluster.Status.InfrastructureReady, clusterv1.ClusterInfrastructureReadyV1Beta2Reason, clusterv1.ClusterInfrastructureNotReadyV1Beta2Reason),
				Message: infrastructureReadyFallBackMessage(cluster.Spec.InfrastructureRef.Kind, cluster.Status.InfrastructureReady),
			},
		)
		if err != nil {
			v1beta2conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterInfrastructureInvalidConditionReportedV1Beta2Reason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if ready.Reason == v1beta2conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.ClusterInfrastructureReadyV1Beta2Reason
		}
		v1beta2conditions.Set(cluster, *ready)
		return
	}

	// If we got errors in reading the infra cluster (this should happen rarely), surface them
	if !infraClusterIsNotFound {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterInfrastructureInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileInfrastructure.
		})
		return
	}

	// Infra cluster missing when the cluster is deleting.
	if !cluster.DeletionTimestamp.IsZero() {
		if cluster.Status.InfrastructureReady {
			v1beta2conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterInfrastructureDeletedV1Beta2Reason,
				Message: fmt.Sprintf("%s has been deleted", cluster.Spec.InfrastructureRef.Kind),
			})
			return
		}

		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason,
			Message: fmt.Sprintf("%s does not exist", cluster.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// Report an issue if infra cluster missing after the cluster has been initialized (and the cluster is still running).
	if cluster.Status.InfrastructureReady {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterInfrastructureDeletedV1Beta2Reason,
			Message: fmt.Sprintf("%s has been deleted while the cluster still exists", cluster.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// If the cluster is not deleting, and infra cluster object does not exist yet,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the cluster and all the objects referenced by it (provisioning yet to start/started, but status.InfrastructureReady not yet set).
	v1beta2conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason,
		Message: fmt.Sprintf("%s does not exist", cluster.Spec.InfrastructureRef.Kind),
	})
}

func setControlPlaneAvailableCondition(_ context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, controlPlaneIsNotFound bool) {
	// control plane is not yet set and the cluster is using ClusterClass.
	if cluster.Spec.ControlPlaneRef == nil && cluster.Spec.Topology != nil {
		message := ""
		if cluster.DeletionTimestamp.IsZero() {
			message = "Waiting for cluster topology to be reconciled"
		}
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
			Message: message,
		})
		return
	}

	if controlPlane != nil {
		available, err := v1beta2conditions.NewMirrorConditionFromUnstructured(
			controlPlane,
			contract.ControlPlane().AvailableConditionType(), v1beta2conditions.TargetConditionType(clusterv1.ClusterControlPlaneAvailableV1Beta2Condition),
			v1beta2conditions.FallbackCondition{
				Status:  v1beta2conditions.BoolToStatus(cluster.Status.ControlPlaneReady),
				Reason:  fallbackReason(cluster.Status.ControlPlaneReady, clusterv1.ClusterControlPlaneAvailableV1Beta2Reason, clusterv1.ClusterControlPlaneNotAvailableV1Beta2Reason),
				Message: controlPlaneAvailableFallBackMessage(cluster.Spec.ControlPlaneRef.Kind, cluster.Status.ControlPlaneReady),
			},
		)
		if err != nil {
			v1beta2conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInvalidConditionReportedV1Beta2Reason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if available.Reason == v1beta2conditions.NoReasonReported && available.Status == metav1.ConditionTrue {
			available.Reason = clusterv1.ClusterControlPlaneAvailableV1Beta2Reason
		}
		v1beta2conditions.Set(cluster, *available)
		return
	}

	// If we got errors in reading the control plane (this should happen rarely), surface them
	if !controlPlaneIsNotFound {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterControlPlaneInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileControlPlane.
		})
		return
	}

	// Infra cluster missing when the cluster is deleting.
	if !cluster.DeletionTimestamp.IsZero() {
		if cluster.Status.ControlPlaneReady {
			v1beta2conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterControlPlaneDeletedV1Beta2Reason,
				Message: fmt.Sprintf("%s has been deleted", cluster.Spec.ControlPlaneRef.Kind),
			})
			return
		}

		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
			Message: fmt.Sprintf("%s does not exist", cluster.Spec.ControlPlaneRef.Kind),
		})
		return
	}

	// Report an issue if control plane missing after the cluster has been initialized (and the cluster is still running).
	if cluster.Status.ControlPlaneReady {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneDeletedV1Beta2Reason,
			Message: fmt.Sprintf("%s has been deleted while the cluster still exists", cluster.Spec.ControlPlaneRef.Kind),
		})
		return
	}

	// If the cluster is not deleting, and control plane object does not exist yet,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the cluster and all the objects referenced by it (provisioning yet to start/started, but status.ControlPlaneReady not yet set).
	v1beta2conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
		Message: fmt.Sprintf("%s does not exist", cluster.Spec.ControlPlaneRef.Kind),
	})
}

func setControlPlaneInitializedCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, controlPlaneMachines collections.Machines, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// No-op if control plane is already initialized.
	if v1beta2conditions.IsTrue(cluster, clusterv1.ClusterControlPlaneInitializedV1Beta2Condition) {
		return
	}

	// control plane is not yet set and the cluster is using ClusterClass.
	if cluster.Spec.ControlPlaneRef == nil && cluster.Spec.Topology != nil {
		message := ""
		if cluster.DeletionTimestamp.IsZero() {
			message = "Waiting for cluster topology to be reconciled"
		}
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
			Message: message,
		})
		return
	}

	// If this cluster is using a control plane object, get control plane initialized from this object.
	if cluster.Spec.ControlPlaneRef != nil {
		if controlPlane == nil {
			if !controlPlaneIsNotFound {
				v1beta2conditions.Set(cluster, metav1.Condition{
					Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
					Status:  metav1.ConditionUnknown,
					Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorV1Beta2Reason,
					Message: "Please check controller logs for errors",
				})
				return
			}

			v1beta2conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason,
				Message: fmt.Sprintf("%s does not exist", cluster.Spec.ControlPlaneRef.Kind),
			})
			return
		}

		initialized, err := external.IsInitialized(controlPlane)
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to get status.initialized from %s", cluster.Spec.ControlPlaneRef.Kind))
			v1beta2conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorV1Beta2Reason,
				Message: "Please check controller logs for errors",
			})
			return
		}

		if initialized {
			v1beta2conditions.Set(cluster, metav1.Condition{
				Type:   clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterControlPlaneInitializedV1Beta2Reason,
			})
			return
		}

		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterControlPlaneNotInitializedV1Beta2Reason,
			Message: "Control plane not yet initialized",
		})
		return
	}

	// Otherwise this cluster control plane is composed by stand-alone machines, and initialized is assumed true
	// when at least one of those machines has a node.

	if !getDescendantsSucceeded {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterControlPlaneInitializedInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(controlPlaneMachines.Filter(collections.HasNode())) > 0 {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.ClusterControlPlaneInitializedV1Beta2Reason,
		})
		return
	}

	v1beta2conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterControlPlaneInitializedV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.ClusterControlPlaneNotInitializedV1Beta2Reason,
		Message: "Waiting for the first control plane machine to have status.nodeRef set",
	})
}

func setWorkersAvailableCondition(ctx context.Context, cluster *clusterv1.Cluster, machinePools expv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in listing descendants (this should never happen), surface it.
	if !getDescendantsSucceeded {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterWorkersAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterWorkersAvailableInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machinePools.Items)+len(machineDeployments.Items) == 0 {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterWorkersAvailableV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.ClusterWorkersAvailableNoWorkersV1Beta2Reason,
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

	workersAvailableCondition, err := v1beta2conditions.NewAggregateCondition(
		ws, clusterv1.AvailableV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.ClusterWorkersAvailableV1Beta2Condition),
		// Using a custom merge strategy to override reasons applied during merge
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterWorkersNotAvailableV1Beta2Reason,
					clusterv1.ClusterWorkersAvailableUnknownV1Beta2Reason,
					clusterv1.ClusterWorkersAvailableV1Beta2Reason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate MachinePool and MachineDeployment's Available conditions")
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterWorkersAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterWorkersAvailableInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	v1beta2conditions.Set(cluster, *workersAvailableCondition)
}

func setControlPlaneMachinesReadyCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	machinesConditionSetter{
		condition:                   clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Condition,
		machineAggregationCondition: clusterv1.MachineReadyV1Beta2Condition,
		internalErrorReason:         clusterv1.ClusterControlPlaneMachinesReadyInternalErrorV1Beta2Reason,
		noReplicasReason:            clusterv1.ClusterControlPlaneMachinesReadyNoReplicasV1Beta2Reason,
		issueReason:                 clusterv1.ClusterControlPlaneMachinesNotReadyV1Beta2Reason,
		unknownReason:               clusterv1.ClusterControlPlaneMachinesReadyUnknownV1Beta2Reason,
		infoReason:                  clusterv1.ClusterControlPlaneMachinesReadyV1Beta2Reason,
	}.setMachinesCondition(ctx, cluster, machines, getDescendantsSucceeded)
}

func setWorkerMachinesReadyCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	machinesConditionSetter{
		condition:                   clusterv1.ClusterWorkerMachinesReadyV1Beta2Condition,
		machineAggregationCondition: clusterv1.MachineReadyV1Beta2Condition,
		internalErrorReason:         clusterv1.ClusterWorkerMachinesReadyInternalErrorV1Beta2Reason,
		noReplicasReason:            clusterv1.ClusterWorkerMachinesReadyNoReplicasV1Beta2Reason,
		issueReason:                 clusterv1.ClusterWorkerMachinesNotReadyV1Beta2Reason,
		unknownReason:               clusterv1.ClusterWorkerMachinesReadyUnknownV1Beta2Reason,
		infoReason:                  clusterv1.ClusterWorkerMachinesReadyV1Beta2Reason,
	}.setMachinesCondition(ctx, cluster, machines, getDescendantsSucceeded)
}

func setControlPlaneMachinesUpToDateCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	machines = machines.Filter(func(machine *clusterv1.Machine) bool {
		return v1beta2conditions.Has(machine, clusterv1.MachineUpToDateV1Beta2Condition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second
	})

	machinesConditionSetter{
		condition:                   clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Condition,
		machineAggregationCondition: clusterv1.MachineUpToDateV1Beta2Condition,
		internalErrorReason:         clusterv1.ClusterControlPlaneMachinesUpToDateInternalErrorV1Beta2Reason,
		noReplicasReason:            clusterv1.ClusterControlPlaneMachinesUpToDateNoReplicasV1Beta2Reason,
		issueReason:                 clusterv1.ClusterControlPlaneMachinesNotUpToDateV1Beta2Reason,
		unknownReason:               clusterv1.ClusterControlPlaneMachinesUpToDateUnknownV1Beta2Reason,
		infoReason:                  clusterv1.ClusterControlPlaneMachinesUpToDateV1Beta2Reason,
	}.setMachinesCondition(ctx, cluster, machines, getDescendantsSucceeded)
}

func setWorkerMachinesUpToDateCondition(ctx context.Context, cluster *clusterv1.Cluster, machines collections.Machines, getDescendantsSucceeded bool) {
	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	machines = machines.Filter(func(machine *clusterv1.Machine) bool {
		return v1beta2conditions.Has(machine, clusterv1.MachineUpToDateV1Beta2Condition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second
	})

	machinesConditionSetter{
		condition:                   clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Condition,
		machineAggregationCondition: clusterv1.MachineUpToDateV1Beta2Condition,
		internalErrorReason:         clusterv1.ClusterWorkerMachinesUpToDateInternalErrorV1Beta2Reason,
		noReplicasReason:            clusterv1.ClusterWorkerMachinesUpToDateNoReplicasV1Beta2Reason,
		issueReason:                 clusterv1.ClusterWorkerMachinesNotUpToDateV1Beta2Reason,
		unknownReason:               clusterv1.ClusterWorkerMachinesUpToDateUnknownV1Beta2Reason,
		infoReason:                  clusterv1.ClusterWorkerMachinesUpToDateV1Beta2Reason,
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
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    s.condition,
			Status:  metav1.ConditionUnknown,
			Reason:  s.internalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machines) == 0 {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   s.condition,
			Status: metav1.ConditionTrue,
			Reason: s.noReplicasReason,
		})
		return
	}

	machinesCondition, err := v1beta2conditions.NewAggregateCondition(
		machines.UnsortedList(), s.machineAggregationCondition,
		v1beta2conditions.TargetConditionType(s.condition),
		// Using a custom merge strategy to override reasons applied during merge
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					s.issueReason,
					s.unknownReason,
					s.infoReason,
				)),
			),
		},
	)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", s.machineAggregationCondition))
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    s.condition,
			Status:  metav1.ConditionUnknown,
			Reason:  s.internalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	v1beta2conditions.Set(cluster, *machinesCondition)
}

func setRemediatingCondition(ctx context.Context, cluster *clusterv1.Cluster, machinesToBeRemediated, unhealthyMachines collections.Machines, getMachinesSucceeded bool) {
	if !getMachinesSucceeded {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemediatingV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRemediatingInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if len(machinesToBeRemediated) == 0 {
		message := aggregateUnhealthyMachines(unhealthyMachines)
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemediatingV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterNotRemediatingV1Beta2Reason,
			Message: message,
		})
		return
	}

	remediatingCondition, err := v1beta2conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.ClusterRemediatingV1Beta2Condition),
		// Note: in case of the remediating conditions it is not required to use a CustomMergeStrategy/ComputeReasonFunc
		// because we are considering only machinesToBeRemediated (and we can pin the reason when we set the condition).
	)
	if err != nil {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRemediatingV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRemediatingInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineOwnerRemediatedV1Beta2Condition))
		return
	}

	v1beta2conditions.Set(cluster, metav1.Condition{
		Type:    remediatingCondition.Type,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1.ClusterRemediatingV1Beta2Reason,
		Message: remediatingCondition.Message,
	})
}

func setRollingOutCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, machinePools expv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in getting control plane or listing descendants (this should never happen), surface it.
	if (cluster.Spec.ControlPlaneRef != nil && controlPlane == nil && !controlPlaneIsNotFound) || !getDescendantsSucceeded {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRollingOutV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRollingOutInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if controlPlane == nil && len(machinePools.Items)+len(machineDeployments.Items) == 0 {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterRollingOutV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotRollingOutV1Beta2Reason,
		})
		return
	}

	ws := make([]aggregationWrapper, 0, len(machinePools.Items)+len(machineDeployments.Items)+1)
	if controlPlane != nil {
		// control plane is considered only if it is reporting the condition (the contract does not require conditions to be reported)
		// Note: this implies that it won't surface as "Conditions RollingOut not yet reported from ...".
		if c, err := v1beta2conditions.UnstructuredGet(controlPlane, clusterv1.RollingOutV1Beta2Condition); err == nil && c != nil {
			ws = append(ws, aggregationWrapper{cp: controlPlane})
		}
	}
	for _, mp := range machinePools.Items {
		ws = append(ws, aggregationWrapper{mp: &mp})
	}
	for _, md := range machineDeployments.Items {
		ws = append(ws, aggregationWrapper{md: &md})
	}

	rollingOutCondition, err := v1beta2conditions.NewAggregateCondition(
		ws, clusterv1.RollingOutV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.ClusterRollingOutV1Beta2Condition),
		// Instruct aggregate to consider RollingOut condition with negative polarity.
		v1beta2conditions.NegativePolarityConditionTypes{clusterv1.RollingOutV1Beta2Condition},
		// Using a custom merge strategy to override reasons applied during merge and to ensure merge
		// takes into account the fact the RollingOut has negative polarity.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.TargetConditionHasPositivePolarity(false),
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterRollingOutV1Beta2Reason,
					clusterv1.ClusterRollingOutUnknownV1Beta2Reason,
					clusterv1.ClusterNotRollingOutV1Beta2Reason,
				)),
				v1beta2conditions.GetPriorityFunc(v1beta2conditions.GetDefaultMergePriorityFunc(clusterv1.RollingOutV1Beta2Condition)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate ControlPlane, MachinePool, MachineDeployment's RollingOut conditions")
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterRollingOutV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterRollingOutInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	v1beta2conditions.Set(cluster, *rollingOutCondition)
}

func setScalingUpCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, machinePools expv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, machineSets clusterv1.MachineSetList, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in getting control plane or listing descendants (this should never happen), surface it.
	if (cluster.Spec.ControlPlaneRef != nil && controlPlane == nil && !controlPlaneIsNotFound) || !getDescendantsSucceeded {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingUpInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if controlPlane == nil && len(machinePools.Items)+len(machineDeployments.Items) == 0 {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterScalingUpV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotScalingUpV1Beta2Reason,
		})
		return
	}

	ws := make([]aggregationWrapper, 0, len(machinePools.Items)+len(machineDeployments.Items)+1)
	if controlPlane != nil {
		// control plane is considered only if it is reporting the condition (the contract does not require conditions to be reported)
		// Note: this implies that it won't surface as "Conditions ScalingUp not yet reported from ...".
		if c, err := v1beta2conditions.UnstructuredGet(controlPlane, clusterv1.ScalingUpV1Beta2Condition); err == nil && c != nil {
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

	scalingUpCondition, err := v1beta2conditions.NewAggregateCondition(
		ws, clusterv1.ScalingUpV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.ClusterScalingUpV1Beta2Condition),
		// Instruct aggregate to consider ScalingUp condition with negative polarity.
		v1beta2conditions.NegativePolarityConditionTypes{clusterv1.ScalingUpV1Beta2Condition},
		// Using a custom merge strategy to override reasons applied during merge and to ensure merge
		// takes into account the fact the ScalingUp has negative polarity.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.TargetConditionHasPositivePolarity(false),
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterScalingUpV1Beta2Reason,
					clusterv1.ClusterScalingUpUnknownV1Beta2Reason,
					clusterv1.ClusterNotScalingUpV1Beta2Reason,
				)),
				v1beta2conditions.GetPriorityFunc(v1beta2conditions.GetDefaultMergePriorityFunc(clusterv1.ScalingUpV1Beta2Condition)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate ControlPlane, MachinePool, MachineDeployment, MachineSet's ScalingUp conditions")
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingUpV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingUpInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	v1beta2conditions.Set(cluster, *scalingUpCondition)
}

func setScalingDownCondition(ctx context.Context, cluster *clusterv1.Cluster, controlPlane *unstructured.Unstructured, machinePools expv1.MachinePoolList, machineDeployments clusterv1.MachineDeploymentList, machineSets clusterv1.MachineSetList, controlPlaneIsNotFound bool, getDescendantsSucceeded bool) {
	log := ctrl.LoggerFrom(ctx)

	// If there was some unexpected errors in getting control plane or listing descendants (this should never happen), surface it.
	if (cluster.Spec.ControlPlaneRef != nil && controlPlane == nil && !controlPlaneIsNotFound) || !getDescendantsSucceeded {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingDownInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if controlPlane == nil && len(machinePools.Items)+len(machineDeployments.Items) == 0 {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterScalingDownV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotScalingDownV1Beta2Reason,
		})
		return
	}

	ws := make([]aggregationWrapper, 0, len(machinePools.Items)+len(machineDeployments.Items)+1)
	if controlPlane != nil {
		// control plane is considered only if it is reporting the condition (the contract does not require conditions to be reported)
		// Note: this implies that it won't surface as "Conditions ScalingDown not yet reported from ...".
		if c, err := v1beta2conditions.UnstructuredGet(controlPlane, clusterv1.ScalingDownV1Beta2Condition); err == nil && c != nil {
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

	scalingDownCondition, err := v1beta2conditions.NewAggregateCondition(
		ws, clusterv1.ScalingDownV1Beta2Condition,
		v1beta2conditions.TargetConditionType(clusterv1.ClusterScalingDownV1Beta2Condition),
		// Instruct aggregate to consider ScalingDown condition with negative polarity.
		v1beta2conditions.NegativePolarityConditionTypes{clusterv1.ScalingDownV1Beta2Condition},
		// Using a custom merge strategy to override reasons applied during merge and to ensure merge
		// takes into account the fact the ScalingDown has negative polarity.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.TargetConditionHasPositivePolarity(false),
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					clusterv1.ClusterScalingDownV1Beta2Reason,
					clusterv1.ClusterScalingDownUnknownV1Beta2Reason,
					clusterv1.ClusterNotScalingDownV1Beta2Reason,
				)),
				v1beta2conditions.GetPriorityFunc(v1beta2conditions.GetDefaultMergePriorityFunc(clusterv1.ScalingDownV1Beta2Condition)),
			),
		},
	)
	if err != nil {
		log.Error(err, "Failed to aggregate ControlPlane, MachinePool, MachineDeployment, MachineSet's ScalingDown conditions")
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterScalingDownV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterScalingDownInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	v1beta2conditions.Set(cluster, *scalingDownCondition)
}

func setDeletingCondition(_ context.Context, cluster *clusterv1.Cluster, deletingReason, deletingMessage string) {
	if cluster.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterDeletingV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterNotDeletingV1Beta2Reason,
		})
		return
	}

	v1beta2conditions.Set(cluster, metav1.Condition{
		Type:    clusterv1.ClusterDeletingV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  deletingReason,
		Message: deletingMessage,
	})
}

type clusterConditionCustomMergeStrategy struct {
	cluster                        *clusterv1.Cluster
	negativePolarityConditionTypes []string
}

func (c clusterConditionCustomMergeStrategy) Merge(conditions []v1beta2conditions.ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error) {
	return v1beta2conditions.DefaultMergeStrategy(v1beta2conditions.GetPriorityFunc(
		func(condition metav1.Condition) v1beta2conditions.MergePriority {
			// While cluster is deleting, treat unknown conditions from external objects as info (it is ok that those objects have been deleted at this stage).
			if !c.cluster.DeletionTimestamp.IsZero() {
				if condition.Type == clusterv1.ClusterInfrastructureReadyV1Beta2Condition && (condition.Reason == clusterv1.ClusterInfrastructureDeletedV1Beta2Reason || condition.Reason == clusterv1.ClusterInfrastructureDoesNotExistV1Beta2Reason) {
					return v1beta2conditions.InfoMergePriority
				}
				if condition.Type == clusterv1.ClusterControlPlaneAvailableV1Beta2Condition && (condition.Reason == clusterv1.ClusterControlPlaneDeletedV1Beta2Reason || condition.Reason == clusterv1.ClusterControlPlaneDoesNotExistV1Beta2Reason) {
					return v1beta2conditions.InfoMergePriority
				}
			}

			// Treat all reasons except TopologyReconcileFailed and ClusterClassNotReconciled of TopologyReconciled condition as info.
			if condition.Type == clusterv1.ClusterTopologyReconciledV1Beta2Condition && condition.Status == metav1.ConditionFalse &&
				condition.Reason != clusterv1.ClusterTopologyReconciledFailedV1Beta2Reason && condition.Reason != clusterv1.ClusterTopologyReconciledClusterClassNotReconciledV1Beta2Reason {
				return v1beta2conditions.InfoMergePriority
			}
			return v1beta2conditions.GetDefaultMergePriorityFunc(c.negativePolarityConditionTypes...)(condition)
		}),
		v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
			clusterv1.ClusterNotAvailableV1Beta2Reason,
			clusterv1.ClusterAvailableUnknownV1Beta2Reason,
			clusterv1.ClusterAvailableV1Beta2Reason,
		)),
	).Merge(conditions, conditionTypes)
}

func setAvailableCondition(ctx context.Context, cluster *clusterv1.Cluster) {
	log := ctrl.LoggerFrom(ctx)

	forConditionTypes := v1beta2conditions.ForConditionTypes{
		clusterv1.ClusterDeletingV1Beta2Condition,
		clusterv1.ClusterRemoteConnectionProbeV1Beta2Condition,
		clusterv1.ClusterInfrastructureReadyV1Beta2Condition,
		clusterv1.ClusterControlPlaneAvailableV1Beta2Condition,
		clusterv1.ClusterWorkersAvailableV1Beta2Condition,
		clusterv1.ClusterTopologyReconciledV1Beta2Condition,
	}
	for _, g := range cluster.Spec.AvailabilityGates {
		forConditionTypes = append(forConditionTypes, g.ConditionType)
	}

	summaryOpts := []v1beta2conditions.SummaryOption{
		forConditionTypes,
		// Instruct summary to consider Deleting condition with negative polarity.
		v1beta2conditions.NegativePolarityConditionTypes{clusterv1.ClusterDeletingV1Beta2Condition},
		// Using a custom merge strategy to override reasons applied during merge and to ignore some
		// info message so the available condition is less noisy.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: clusterConditionCustomMergeStrategy{
				cluster: cluster,
				// Instruct merge to consider Deleting condition with negative polarity,
				negativePolarityConditionTypes: []string{clusterv1.ClusterDeletingV1Beta2Condition},
			},
		},
	}
	if cluster.Spec.Topology == nil {
		summaryOpts = append(summaryOpts, v1beta2conditions.IgnoreTypesIfMissing{clusterv1.ClusterTopologyReconciledV1Beta2Condition})
	}

	availableCondition, err := v1beta2conditions.NewSummaryCondition(cluster, clusterv1.ClusterAvailableV1Beta2Condition, summaryOpts...)

	if err != nil {
		// Note, this could only happen if we hit edge cases in computing the summary, which should not happen due to the fact
		// that we are passing a non empty list of ForConditionTypes.
		log.Error(err, "Failed to set Available condition")
		availableCondition = &metav1.Condition{
			Type:    clusterv1.ClusterAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.ClusterAvailableInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		}
	}

	v1beta2conditions.Set(cluster, *availableCondition)
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
	return fmt.Sprintf("%s status.ready is %t", kind, ready)
}

func controlPlaneAvailableFallBackMessage(kind string, ready bool) string {
	if ready {
		return ""
	}
	return fmt.Sprintf("%s status.ready is %t", kind, ready)
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
	mp *expv1.MachinePool
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

func (w aggregationWrapper) GetV1Beta2Conditions() []metav1.Condition {
	switch {
	case w.cp != nil:
		if c, err := v1beta2conditions.UnstructuredGetAll(w.cp); err == nil && c != nil {
			return c
		}
		return nil
	case w.mp != nil:
		return w.mp.GetV1Beta2Conditions()
	case w.md != nil:
		return w.md.GetV1Beta2Conditions()
	case w.ms != nil:
		return w.ms.GetV1Beta2Conditions()
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
