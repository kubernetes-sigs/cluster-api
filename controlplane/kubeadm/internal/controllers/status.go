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
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	clog "sigs.k8s.io/cluster-api/util/log"
)

// updateStatus is called after every reconciliation loop in a defer statement to always make sure we have the
// KubeadmControlPlane status up-to-date.
func (r *KubeadmControlPlaneReconciler) updateStatus(ctx context.Context, controlPlane *internal.ControlPlane) error {
	selector := collections.ControlPlaneSelectorForCluster(controlPlane.Cluster.Name)
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	controlPlane.KCP.Status.Selector = selector.String()

	upToDateMachines := controlPlane.UpToDateMachines()
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

// updateV1Beta2Status reconciles KubeadmControlPlane's status during the entire lifecycle of the object.
// Note: v1beta1 conditions and fields are not managed by this func.
func (r *KubeadmControlPlaneReconciler) updateV1Beta2Status(ctx context.Context, controlPlane *internal.ControlPlane) {
	// If the code failed initializing the control plane, do not update the status.
	if controlPlane == nil {
		return
	}

	// Note: some of the status is set on reconcileControlPlaneAndMachinesConditions (EtcdClusterHealthy, ControlPlaneComponentsHealthy conditions),
	// reconcileClusterCertificates (CertificatesAvailable condition), and also in the defer patch at the end of
	// the main reconcile loop (status.ObservedGeneration) etc

	// Note: KCP also sets status on machines in reconcileUnhealthyMachines and reconcileControlPlaneAndMachinesConditions; if for
	// any reason those functions are not called before, e.g. an error, this func relies on existing Machine's condition.

	setReplicas(ctx, controlPlane.KCP, controlPlane.Machines)
	setInitializedCondition(ctx, controlPlane.KCP)
	setScalingUpCondition(ctx, controlPlane.KCP, controlPlane.Machines, controlPlane.InfraMachineTemplateIsNotFound, controlPlane.PreflightCheckResults)
	setScalingDownCondition(ctx, controlPlane.KCP, controlPlane.Machines, controlPlane.PreflightCheckResults)
	setMachinesReadyCondition(ctx, controlPlane.KCP, controlPlane.Machines)
	setMachinesUpToDateCondition(ctx, controlPlane.KCP, controlPlane.Machines)
	setRemediatingCondition(ctx, controlPlane.KCP, controlPlane.MachinesToBeRemediatedByKCP(), controlPlane.UnhealthyMachines())
	setDeletingCondition(ctx, controlPlane.KCP, controlPlane.DeletingReason, controlPlane.DeletingMessage)
	setAvailableCondition(ctx, controlPlane.KCP, controlPlane.IsEtcdManaged(), controlPlane.EtcdMembers, controlPlane.EtcdMembersAgreeOnMemberList, controlPlane.EtcdMembersAgreeOnClusterID, controlPlane.EtcdMembersAndMachinesAreMatching, controlPlane.Machines)
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

func setInitializedCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane) {
	if kcp.Status.Initialized {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneInitializedV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneInitializedV1Beta2Reason,
		})
		return
	}

	v1beta2conditions.Set(kcp, metav1.Condition{
		Type:   controlplanev1.KubeadmControlPlaneInitializedV1Beta2Condition,
		Status: metav1.ConditionFalse,
		Reason: controlplanev1.KubeadmControlPlaneNotInitializedV1Beta2Reason,
	})
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
		// Using a custom merge strategy to override reasons applied during merge.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					controlplanev1.KubeadmControlPlaneMachinesNotReadyV1Beta2Reason,
					controlplanev1.KubeadmControlPlaneMachinesReadyUnknownV1Beta2Reason,
					controlplanev1.KubeadmControlPlaneMachinesReadyV1Beta2Reason,
				)),
			),
		},
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
		// Using a custom merge strategy to override reasons applied during merge.
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: v1beta2conditions.DefaultMergeStrategy(
				v1beta2conditions.ComputeReasonFunc(v1beta2conditions.GetDefaultComputeMergeReasonFunc(
					controlplanev1.KubeadmControlPlaneMachinesNotUpToDateV1Beta2Reason,
					controlplanev1.KubeadmControlPlaneMachinesUpToDateUnknownV1Beta2Reason,
					controlplanev1.KubeadmControlPlaneMachinesUpToDateV1Beta2Reason,
				)),
			),
		},
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

	remediatingCondition, err := v1beta2conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedV1Beta2Condition,
		v1beta2conditions.TargetConditionType(controlplanev1.KubeadmControlPlaneRemediatingV1Beta2Condition),
		// Note: in case of the remediating conditions it is not required to use a CustomMergeStrategy/ComputeReasonFunc
		// because we are considering only machinesToBeRemediated (and we can pin the reason when we set the condition).
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

func setDeletingCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, deletingReason, deletingMessage string) {
	if kcp.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneDeletingV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: controlplanev1.KubeadmControlPlaneDeletingDeletionTimestampNotSetV1Beta2Reason,
		})
		return
	}

	v1beta2conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneDeletingV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  deletingReason,
		Message: deletingMessage,
	})
}

func setAvailableCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, etcdIsManaged bool, etcdMembers []*etcd.Member, etcdMembersAgreeOnMemberList, etcdMembersAgreeOnClusterID, etcdMembersAndMachinesAreMatching bool, machines collections.Machines) {
	if !kcp.Status.Initialized {
		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneNotAvailableV1Beta2Reason,
			Message: "Control plane not yet initialized",
		})
		return
	}

	if etcdIsManaged {
		if etcdMembers == nil {
			// In case the control plane just initialized, give some more time before reporting failed to get etcd members.
			// Note: Two minutes is the time after which we assume that not getting the list of etcd members is an actual problem.
			if c := v1beta2conditions.Get(kcp, controlplanev1.KubeadmControlPlaneInitializedV1Beta2Condition); c != nil &&
				c.Status == metav1.ConditionTrue &&
				time.Since(c.LastTransitionTime.Time) < 2*time.Minute {
				v1beta2conditions.Set(kcp, metav1.Condition{
					Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneNotAvailableV1Beta2Reason,
					Message: "Waiting for etcd to report the list of members",
				})
				return
			}

			v1beta2conditions.Set(kcp, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneAvailableInspectionFailedV1Beta2Reason,
				Message: "Failed to get etcd members",
			})
			return
		}

		if !etcdMembersAgreeOnMemberList {
			v1beta2conditions.Set(kcp, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneNotAvailableV1Beta2Reason,
				Message: "At least one etcd member reports a list of etcd members different than the list reported by other members",
			})
			return
		}

		if !etcdMembersAgreeOnClusterID {
			v1beta2conditions.Set(kcp, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneNotAvailableV1Beta2Reason,
				Message: "At least one etcd member reports a cluster ID different than the cluster ID reported by other members",
			})
			return
		}

		if !etcdMembersAndMachinesAreMatching {
			v1beta2conditions.Set(kcp, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneNotAvailableV1Beta2Reason,
				Message: "The list of etcd members does not match the list of Machines and Nodes",
			})
			return
		}
	}

	// Determine control plane availability looking at machines conditions, which at this stage are
	// already surfacing status from etcd member and all control plane pods hosted on every machine.
	// Note: we intentionally use the number of etcd members to determine the etcd quorum because
	// etcd members might not match with machines, e.g. while provisioning a new machine.
	etcdQuorum := (len(etcdMembers) / 2.0) + 1
	k8sControlPlaneHealthy := 0
	k8sControlPlaneNotHealthy := 0
	etcdMembersHealthy := 0
	etcdMembersNotHealthy := 0
	for _, machine := range machines {
		// if external etcd, only look at the status of the K8s control plane components on this machine.
		if !etcdIsManaged {
			if v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition) &&
				v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition) &&
				v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition) {
				k8sControlPlaneHealthy++
			} else if shouldSurfaceWhenAvailableTrue(machine,
				controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition,
				controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition,
				controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition) {
				k8sControlPlaneNotHealthy++
			}
			continue
		}

		// Otherwise, etcd is managed.
		// In this case, when looking at the k8s control plane we should consider how kubeadm layouts control plane components,
		// and more specifically:
		// - API server on one machine only connect to the local etcd member
		// - ControllerManager and scheduler on a machine connect to the local API server (not to the control plane endpoint)
		// As a consequence, we consider the K8s control plane on this machine healthy only if everything is healthy.

		if v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition) {
			etcdMembersHealthy++
		} else if shouldSurfaceWhenAvailableTrue(machine,
			controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition) {
			etcdMembersNotHealthy++
		}

		if v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition) &&
			v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition) &&
			v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition) &&
			v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition) &&
			v1beta2conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition) {
			k8sControlPlaneHealthy++
		} else if shouldSurfaceWhenAvailableTrue(machine,
			controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition,
			controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition,
			controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition,
			controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
			controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition) {
			k8sControlPlaneNotHealthy++
		}
	}

	if kcp.DeletionTimestamp.IsZero() &&
		(!etcdIsManaged || etcdMembersHealthy >= etcdQuorum) &&
		k8sControlPlaneHealthy >= 1 &&
		v1beta2conditions.IsTrue(kcp, controlplanev1.KubeadmControlPlaneCertificatesAvailableV1Beta2Condition) {
		messages := []string{}

		if etcdIsManaged && etcdMembersNotHealthy > 0 {
			switch len(etcdMembers) - etcdMembersNotHealthy {
			case 1:
				messages = append(messages, fmt.Sprintf("* 1 of %d etcd members is healthy, at least %d required for etcd quorum", len(etcdMembers), etcdQuorum))
			default:
				messages = append(messages, fmt.Sprintf("* %d of %d etcd members are healthy, at least %d required for etcd quorum", len(etcdMembers)-etcdMembersNotHealthy, len(etcdMembers), etcdQuorum))
			}
		}

		if k8sControlPlaneNotHealthy > 0 {
			switch len(machines) - k8sControlPlaneNotHealthy {
			case 1:
				messages = append(messages, fmt.Sprintf("* 1 of %d Machines has healthy control plane components, at least 1 required", len(machines)))
			default:
				messages = append(messages, fmt.Sprintf("* %d of %d Machines have healthy control plane components, at least 1 required", len(machines)-k8sControlPlaneNotHealthy, len(machines)))
			}
		}

		v1beta2conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  controlplanev1.KubeadmControlPlaneAvailableV1Beta2Reason,
			Message: strings.Join(messages, "\n"),
		})
		return
	}

	messages := []string{}
	if !kcp.DeletionTimestamp.IsZero() {
		messages = append(messages, "* Control plane metadata.deletionTimestamp is set")
	}

	if !v1beta2conditions.IsTrue(kcp, controlplanev1.KubeadmControlPlaneCertificatesAvailableV1Beta2Condition) {
		messages = append(messages, "* Control plane certificates are not available")
	}

	if etcdIsManaged && etcdMembersHealthy < etcdQuorum {
		switch etcdMembersHealthy {
		case 0:
			messages = append(messages, fmt.Sprintf("* There are no healthy etcd member, at least %d required for etcd quorum", etcdQuorum))
		case 1:
			messages = append(messages, fmt.Sprintf("* 1 of %d etcd members is healthy, at least %d required for etcd quorum", len(etcdMembers), etcdQuorum))
		default:
			messages = append(messages, fmt.Sprintf("* %d of %d etcd members are healthy, at least %d required for etcd quorum", etcdMembersHealthy, len(etcdMembers), etcdQuorum))
		}
	}

	if k8sControlPlaneHealthy < 1 {
		messages = append(messages, "* There are no Machines with healthy control plane components, at least 1 required")
	}

	v1beta2conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneAvailableV1Beta2Condition,
		Status:  metav1.ConditionFalse,
		Reason:  controlplanev1.KubeadmControlPlaneNotAvailableV1Beta2Reason,
		Message: strings.Join(messages, "\n"),
	})
}

// shouldSurfaceWhenAvailableTrue defines when a control plane components/etcd issue should surface when
// Available condition is true.
// The main goal of this check is to avoid to surface false negatives/flakes, and thus it requires that
// an issue exists for at least more than 10 seconds before surfacing it.
func shouldSurfaceWhenAvailableTrue(machine *clusterv1.Machine, conditionTypes ...string) bool {
	// Get the min time when one of the conditions in input transitioned to false or unknown.
	var t *time.Time
	for _, conditionType := range conditionTypes {
		c := v1beta2conditions.Get(machine, conditionType)
		if c == nil {
			continue
		}
		if c.Status == metav1.ConditionTrue {
			continue
		}
		if t == nil {
			t = ptr.To(c.LastTransitionTime.Time)
		}
		t = ptr.To(minTime(*t, c.LastTransitionTime.Time))
	}

	if t != nil {
		if time.Since(*t) > 10*time.Second {
			return true
		}
	}
	return false
}

func minTime(t1, t2 time.Time) time.Time {
	if t1.After(t2) {
		return t2
	}
	return t1
}

func aggregateStaleMachines(machines collections.Machines) string {
	if len(machines) == 0 {
		return ""
	}

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
	message += "not healthy (not to be remediated by KubeadmControlPlane)"

	return message
}
