/*
Copyright 2020 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	clog "sigs.k8s.io/cluster-api/util/log"
)

// updateStatus is called after every reconcilitation loop in a defer statement to always make sure we have the
// resource status subresourcs up-to-date.
func (r *KubeadmControlPlaneReconciler) updateStatus(ctx context.Context, controlPlane *internal.ControlPlane) error {
	selector := collections.ControlPlaneSelectorForCluster(controlPlane.Cluster.Name)
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	controlPlane.KCP.Status.Selector = selector.String()

	upToDateMachines, err := controlPlane.UpToDateMachines()
	if err != nil {
		return errors.Wrapf(err, "failed to update status")
	}
	controlPlane.KCP.Status.UpdatedReplicas = int32(len(upToDateMachines))

	replicas := int32(len(controlPlane.Machines))
	desiredReplicas := *controlPlane.KCP.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	controlPlane.KCP.Status.Replicas = replicas
	controlPlane.KCP.Status.ReadyReplicas = 0
	controlPlane.KCP.Status.UnavailableReplicas = replicas

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !controlPlane.KCP.DeletionTimestamp.IsZero() {
		return nil
	}

	lowestVersion := controlPlane.Machines.LowestVersion()
	if lowestVersion != nil {
		controlPlane.KCP.Status.Version = lowestVersion
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedCondition, controlplanev1.ScalingUpReason, clusterv1.ConditionSeverityWarning, "Scaling up control plane to %d replicas (actual %d)", desiredReplicas, replicas)
	// We are scaling down
	case replicas > desiredReplicas:
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedCondition, controlplanev1.ScalingDownReason, clusterv1.ConditionSeverityWarning, "Scaling down control plane to %d replicas (actual %d)", desiredReplicas, replicas)

		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesCreatedCondition)
	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		readyMachines := controlPlane.Machines.Filter(collections.IsReady())
		if int32(len(readyMachines)) == replicas {
			conditions.MarkTrue(controlPlane.KCP, controlplanev1.ResizedCondition)
		}

		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesCreatedCondition)
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create remote cluster client")
	}
	status, err := workloadCluster.ClusterStatus(ctx)
	if err != nil {
		return err
	}
	controlPlane.KCP.Status.ReadyReplicas = status.ReadyNodes
	controlPlane.KCP.Status.UnavailableReplicas = replicas - status.ReadyNodes

	// This only gets initialized once and does not change if the kubeadm config map goes away.
	if status.HasKubeadmConfig {
		controlPlane.KCP.Status.Initialized = true
		conditions.MarkTrue(controlPlane.KCP, controlplanev1.AvailableCondition)
		v1beta2conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneInitializedV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneInitializedV1Beta2Reason,
		})
	}

	if controlPlane.KCP.Status.ReadyReplicas > 0 {
		controlPlane.KCP.Status.Ready = true
	}

	// Surface lastRemediation data in status.
	// LastRemediation is the remediation currently in progress, in any, or the
	// most recent of the remediation we are keeping track on machines.
	var lastRemediation *RemediationData

	if v, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		remediationData, err := RemediationDataFromAnnotation(v)
		if err != nil {
			return err
		}
		lastRemediation = remediationData
	} else {
		for _, m := range controlPlane.Machines.UnsortedList() {
			if v, ok := m.Annotations[controlplanev1.RemediationForAnnotation]; ok {
				remediationData, err := RemediationDataFromAnnotation(v)
				if err != nil {
					return err
				}
				if lastRemediation == nil || lastRemediation.Timestamp.Time.Before(remediationData.Timestamp.Time) {
					lastRemediation = remediationData
				}
			}
		}
	}

	if lastRemediation != nil {
		controlPlane.KCP.Status.LastRemediation = lastRemediation.ToStatus()
	}
	return nil
}

// updateV1beta2Status reconciles KubeadmControlPlane's status during the entire lifecycle of the object.
// Note: v1beta1 conditions and fields are not managed by this func.
func (r *KubeadmControlPlaneReconciler) updateV1beta2Status(ctx context.Context, controlPlane *internal.ControlPlane) {
	// If the code failed initializing the control plane, do not update the status.
	if controlPlane == nil {
		return
	}

	// Note: some of the status is set on reconcileControlPlaneConditions (EtcdClusterHealthy, ControlPlaneComponentsHealthy conditions),
	// reconcileClusterCertificates (CertificatesAvailable condition), and also in the defer patch at the end of
	// the main reconcile loop (status.ObservedGeneration) etc

	// Note: KCP also sets status on machines in reconcileUnhealthyMachines and reconcileControlPlaneConditions; if for
	// any reason those functions are not called before, e.g. an error, this func relies on existing Machine's condition.

	setReplicas(ctx, controlPlane.KCP, controlPlane.Machines)
	setScalingUpCondition(ctx, controlPlane.KCP, controlPlane.Machines, controlPlane.InfraMachineTemplateIsNotFound, controlPlane.PreflightCheckResults)
	setScalingDownCondition(ctx, controlPlane.KCP, controlPlane.Machines, controlPlane.PreflightCheckResults)
	setMachinesReadyCondition(ctx, controlPlane.KCP, controlPlane.Machines)
	setMachinesUpToDateCondition(ctx, controlPlane.KCP, controlPlane.Machines)
	setRemediatingCondition(ctx, controlPlane.KCP, controlPlane.MachinesToBeRemediatedByKCP(), controlPlane.UnhealthyMachines())

	// TODO: Available, Deleting
}

func setReplicas(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines) {
	var readyReplicas, availableReplicas, upToDateReplicas int32
	for _, machine := range machines {
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineReadyV1Beta2Condition) {
			readyReplicas++
		}
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineAvailableV1Beta2Condition) {
			availableReplicas++
		}
		if v1beta2conditions.IsTrue(machine, clusterv1.MachineUpToDateV1Beta2Condition) {
			upToDateReplicas++
		}
	}

	if kcp.Status.V1Beta2 == nil {
		kcp.Status.V1Beta2 = &controlplanev1.KubeadmControlPlaneV1Beta2Status{}
	}

	kcp.Status.V1Beta2.ReadyReplicas = ptr.To(readyReplicas)
	kcp.Status.V1Beta2.AvailableReplicas = ptr.To(availableReplicas)
	kcp.Status.V1Beta2.UpToDateReplicas = ptr.To(upToDateReplicas)
}

func setScalingUpCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines, infrastructureObjectNotFound bool, preflightChecks internal.PreflightCheckResults) {
	if kcp.Spec.Replicas == nil {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
			Status: metav1.ConditionUnknown,
			Reason: controlplanev1.KubeadmControlPlaneScalingUpWaitingForReplicasSetV1Beta2Reason,
		})
		return
	}

	currentReplicas := int32(len(machines))
	desiredReplicas := *kcp.Spec.Replicas
	if !kcp.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}

	missingReferencesMessage := calculateMissingReferencesMessage(kcp, infrastructureObjectNotFound)

	if currentReplicas >= desiredReplicas {
		var message string
		if missingReferencesMessage != "" {
			message = fmt.Sprintf("Scaling up would be blocked %s", missingReferencesMessage)
		}
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneNotScalingUpV1Beta2Reason,
			Message: message,
		})
		return
	}

	message := fmt.Sprintf("Scaling up from %d to %d replicas", currentReplicas, desiredReplicas)
	if missingReferencesMessage != "" {
		message = fmt.Sprintf("%s is blocked %s", message, missingReferencesMessage)
	}
	messages := []string{message}

	if preflightChecks.HasDeletingMachine {
		messages = append(messages, "waiting for Machine being deleted")
	}

	if preflightChecks.ControlPlaneComponentsNotHealthy {
		messages = append(messages, "waiting for control plane components to be healthy")
	}

	if preflightChecks.EtcdClusterNotHealthy {
		messages = append(messages, "waiting for etcd cluster to be healthy")
	}

	v1beta2conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  controlplanev1.KubeadmControlPlaneScalingUpV1Beta2Reason,
		Message: strings.Join(messages, "; "),
	})
}

func setScalingDownCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines, preflightChecks internal.PreflightCheckResults) {
	if kcp.Spec.Replicas == nil {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
			Status: metav1.ConditionUnknown,
			Reason: controlplanev1.KubeadmControlPlaneScalingDownWaitingForReplicasSetV1Beta2Reason,
		})
		return
	}

	currentReplicas := int32(len(machines))
	desiredReplicas := *kcp.Spec.Replicas
	if !kcp.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}

	if currentReplicas <= desiredReplicas {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: controlplanev1.KubeadmControlPlaneNotScalingDownV1Beta2Reason,
		})
		return
	}

	messages := []string{fmt.Sprintf("Scaling down from %d to %d replicas", currentReplicas, desiredReplicas)}
	if preflightChecks.HasDeletingMachine {
		messages = append(messages, "waiting for Machine being deleted")
	}

	if staleMessage := aggregateStaleMachines(machines); staleMessage != "" {
		messages = append(messages, staleMessage)
	}

	if preflightChecks.ControlPlaneComponentsNotHealthy {
		messages = append(messages, "waiting for control plane components to be healthy")
	}

	if preflightChecks.EtcdClusterNotHealthy {
		messages = append(messages, "waiting for etcd cluster to be healthy")
	}

	v1beta2conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  controlplanev1.KubeadmControlPlaneScalingDownV1Beta2Reason,
		Message: strings.Join(messages, "; "),
	})
}

func setMachinesReadyCondition(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines) {
	if len(machines) == 0 {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneMachinesReadyV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneMachinesReadyNoReplicasV1Beta2Reason,
		})
		return
	}

	readyCondition, err := v1beta2conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineReadyV1Beta2Condition,
		v1beta2conditions.TargetConditionType(controlplanev1.KubeadmControlPlaneMachinesReadyV1Beta2Condition),
	)
	if err != nil {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneMachinesReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachinesReadyInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineReadyV1Beta2Condition))
		return
	}

	v1beta2conditions.Set(kcp, *readyCondition)
}

func setMachinesUpToDateCondition(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines) {
	if len(machines) == 0 {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneMachinesUpToDateV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneMachinesUpToDateNoReplicasV1Beta2Reason,
		})
		return
	}

	upToDateCondition, err := v1beta2conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineUpToDateV1Beta2Condition,
		v1beta2conditions.TargetConditionType(controlplanev1.KubeadmControlPlaneMachinesUpToDateV1Beta2Condition),
	)
	if err != nil {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneMachinesUpToDateV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachinesUpToDateInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineUpToDateV1Beta2Condition))
		return
	}

	v1beta2conditions.Set(kcp, *upToDateCondition)
}

func calculateMissingReferencesMessage(kcp *controlplanev1.KubeadmControlPlane, infraMachineTemplateNotFound bool) string {
	if infraMachineTemplateNotFound {
		return fmt.Sprintf("because %s does not exist", kcp.Spec.MachineTemplate.InfrastructureRef.Kind)
	}
	return ""
}

func setRemediatingCondition(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machinesToBeRemediated, unhealthyMachines collections.Machines) {
	if len(machinesToBeRemediated) == 0 {
		message := aggregateUnhealthyMachines(unhealthyMachines)
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneNotRemediatingV1Beta2Reason,
			Message: message,
		})
		return
	}

	// TODO: Bring together externally remediated machines and owner remediated machines
	remediatingCondition, err := v1beta2conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedV1Beta2Condition,
		v1beta2conditions.TargetConditionType(controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition),
	)
	if err != nil {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneRemediatingInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineOwnerRemediatedV1Beta2Condition))
		return
	}

	v1beta2conditions.Set(kcp, metav1.Condition{
		Type:    remediatingCondition.Type,
		Status:  metav1.ConditionTrue,
		Reason:  controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Reason,
		Message: remediatingCondition.Message,
	})
}

func aggregateStaleMachines(machines collections.Machines) string {
	machineNames := []string{}
	for _, machine := range machines {
		if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) > time.Minute*30 {
			machineNames = append(machineNames, machine.GetName())
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
	message += "in deletion since more than 30m"

	return message
}

func aggregateUnhealthyMachines(machines collections.Machines) string {
	machineNames := []string{}
	for _, machine := range machines {
		machineNames = append(machineNames, machine.GetName())
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
	message += "not healthy (not to be remediated by KCP)"

	return message
}
