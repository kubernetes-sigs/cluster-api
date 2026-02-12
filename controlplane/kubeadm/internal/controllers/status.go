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
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	clog "sigs.k8s.io/cluster-api/util/log"
)

// updateV1Beta1Status is called after every reconciliation loop in a defer statement to always make sure we have the
// KubeadmControlPlane status up-to-date.
func (r *KubeadmControlPlaneReconciler) updateV1Beta1Status(ctx context.Context, controlPlane *internal.ControlPlane) error {
	upToDateMachines := controlPlane.UpToDateMachines()
	if controlPlane.KCP.Status.Deprecated == nil {
		controlPlane.KCP.Status.Deprecated = &controlplanev1.KubeadmControlPlaneDeprecatedStatus{}
	}
	if controlPlane.KCP.Status.Deprecated.V1Beta1 == nil {
		controlPlane.KCP.Status.Deprecated.V1Beta1 = &controlplanev1.KubeadmControlPlaneV1Beta1DeprecatedStatus{}
	}
	controlPlane.KCP.Status.Deprecated.V1Beta1.UpdatedReplicas = int32(len(upToDateMachines))

	replicas := int32(len(controlPlane.Machines))
	desiredReplicas := *controlPlane.KCP.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	controlPlane.KCP.Status.Deprecated.V1Beta1.ReadyReplicas = 0
	controlPlane.KCP.Status.Deprecated.V1Beta1.UnavailableReplicas = replicas

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !controlPlane.KCP.DeletionTimestamp.IsZero() {
		return nil
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		v1beta1conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedV1Beta1Condition, controlplanev1.ScalingUpV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Scaling up control plane to %d replicas (actual %d)", desiredReplicas, replicas)
	// We are scaling down
	case replicas > desiredReplicas:
		v1beta1conditions.MarkFalse(controlPlane.KCP, controlplanev1.ResizedV1Beta1Condition, controlplanev1.ScalingDownV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Scaling down control plane to %d replicas (actual %d)", desiredReplicas, replicas)

		// This means that there was no error in generating the desired number of machine objects
		v1beta1conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesCreatedV1Beta1Condition)
	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		readyMachines := controlPlane.Machines.Filter(collections.IsReady())
		if int32(len(readyMachines)) == replicas {
			v1beta1conditions.MarkTrue(controlPlane.KCP, controlplanev1.ResizedV1Beta1Condition)
		}

		// This means that there was no error in generating the desired number of machine objects
		v1beta1conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesCreatedV1Beta1Condition)
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create remote cluster client")
	}
	status, err := workloadCluster.ClusterStatus(ctx)
	if err != nil {
		return err
	}
	controlPlane.KCP.Status.Deprecated.V1Beta1.ReadyReplicas = status.ReadyNodes
	controlPlane.KCP.Status.Deprecated.V1Beta1.UnavailableReplicas = replicas - status.ReadyNodes

	if status.HasKubeadmConfig {
		v1beta1conditions.MarkTrue(controlPlane.KCP, controlplanev1.AvailableV1Beta1Condition)
	}
	return nil
}

// updateStatus reconciles KubeadmControlPlane's status during the entire lifecycle of the object.
func (r *KubeadmControlPlaneReconciler) updateStatus(ctx context.Context, controlPlane *internal.ControlPlane) error {
	// If the code failed initializing the control plane, do not update the status.
	if controlPlane == nil {
		return nil
	}

	// Note: some of the status is set on reconcileControlPlaneAndMachinesConditions (EtcdClusterHealthy, ControlPlaneComponentsHealthy conditions),
	// reconcileClusterCertificates (CertificatesAvailable condition), and also in the defer patch at the end of
	// the main reconcile loop (status.ObservedGeneration) etc

	// Note: KCP also sets status on machines in reconcileUnhealthyMachines and reconcileControlPlaneAndMachinesConditions; if for
	// any reason those functions are not called before, e.g. an error, this func relies on existing Machine's condition.

	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	selector := collections.ControlPlaneSelectorForCluster(controlPlane.Cluster.Name)
	controlPlane.KCP.Status.Selector = selector.String()

	// Set status.version with the lowest K8s version from CP machines.
	lowestVersion := controlPlane.Machines.LowestVersion()
	if lowestVersion != "" {
		controlPlane.KCP.Status.Version = lowestVersion
	}

	allErrors := []error{}
	if err := setControlPlaneInitialized(ctx, controlPlane); err != nil {
		allErrors = append(allErrors, err)
	}
	setReplicas(ctx, controlPlane.KCP, controlPlane.Machines)
	setInitializedCondition(ctx, controlPlane.KCP)
	setRollingOutCondition(ctx, controlPlane.KCP, controlPlane.Machines)
	setScalingUpCondition(ctx, controlPlane.Cluster, controlPlane.KCP, controlPlane.Machines, controlPlane.InfraMachineTemplateIsNotFound, controlPlane.PreflightCheckResults)
	setScalingDownCondition(ctx, controlPlane.Cluster, controlPlane.KCP, controlPlane.Machines, controlPlane.PreflightCheckResults)
	setMachinesReadyCondition(ctx, controlPlane.KCP, controlPlane.Machines)
	setMachinesUpToDateCondition(ctx, controlPlane.KCP, controlPlane.Machines)
	setRemediatingCondition(ctx, controlPlane.KCP, controlPlane.MachinesToBeRemediatedByKCP(), controlPlane.UnhealthyMachines())
	setDeletingCondition(ctx, controlPlane.KCP, controlPlane.DeletingReason, controlPlane.DeletingMessage)
	setAvailableCondition(ctx, controlPlane.KCP, controlPlane.IsEtcdManaged(), controlPlane.EtcdMembers, controlPlane.EtcdMembersAndMachinesAreMatching, controlPlane.Machines)
	if err := setLastRemediation(ctx, controlPlane); err != nil {
		allErrors = append(allErrors, err)
	}
	return kerrors.NewAggregate(allErrors)
}

// setControlPlaneInitialized surface control plane initialized when it is possible to check that the Kubeadm config exists in the workload cluster;
// this is considered a proxy information about the API Server being up and running and kubeadm init successfully completed.
// Note: This only gets initialized once and does not change if the kubeadm config map goes away.
func setControlPlaneInitialized(ctx context.Context, controlPlane *internal.ControlPlane) error {
	if !ptr.Deref(controlPlane.KCP.Status.Initialization.ControlPlaneInitialized, false) {
		// If the control plane has only one machine, and this machine is marked for remediation or in the process of deleting,
		// do not check for control plane initialized.
		// This prevents an issue that happens if kubeadm init completes in the short timeframe between when machine deletion is triggered
		// to when the machine goes away; this issue, if not properly handled, will lead to an inconsistent state where
		// cluster is initialized, no CP machine exists, and the replacement CP machine fails when trying to join.
		if len(controlPlane.Machines) == 1 {
			m := controlPlane.Machines.UnsortedList()[0]
			if collections.IsUnhealthyAndOwnerRemediated(m) {
				return nil
			}
			if !m.DeletionTimestamp.IsZero() {
				return nil
			}
		}

		workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create remote cluster client")
		}
		status, err := workloadCluster.ClusterStatus(ctx)
		if err != nil {
			return err
		}

		if status.HasKubeadmConfig {
			controlPlane.KCP.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
		}
	}
	return nil
}

func setReplicas(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines) {
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

	kcp.Status.Replicas = ptr.To(int32(len(machines)))
	kcp.Status.ReadyReplicas = ptr.To(readyReplicas)
	kcp.Status.AvailableReplicas = ptr.To(availableReplicas)
	kcp.Status.UpToDateReplicas = ptr.To(upToDateReplicas)
}

func setInitializedCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane) {
	if ptr.Deref(kcp.Status.Initialization.ControlPlaneInitialized, false) {
		conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneInitializedReason,
		})
		return
	}

	conditions.Set(kcp, metav1.Condition{
		Type:   controlplanev1.KubeadmControlPlaneInitializedCondition,
		Status: metav1.ConditionFalse,
		Reason: controlplanev1.KubeadmControlPlaneNotInitializedReason,
	})
}

func setRollingOutCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines) {
	// Count machines rolling out and collect reasons why a rollout is happening.
	// Note: The code below collects all the reasons for which at least a machine is rolling out; under normal circumstances
	// all the machines are rolling out for the same reasons, however, in case of changes to KCP
	// before a previous changes is not fully rolled out, there could be machines rolling out for
	// different reasons.
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
		var message string
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneRollingOutCondition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneNotRollingOutReason,
			Message: message,
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
	conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneRollingOutCondition,
		Status:  metav1.ConditionTrue,
		Reason:  controlplanev1.KubeadmControlPlaneRollingOutReason,
		Message: message,
	})
}

func setScalingUpCondition(_ context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines, infrastructureObjectNotFound bool, preflightChecks internal.PreflightCheckResults) {
	if kcp.Spec.Replicas == nil {
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneScalingUpCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneScalingUpWaitingForReplicasSetReason,
			Message: "Waiting for spec.replicas set",
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
			message = fmt.Sprintf("Scaling up would be blocked because %s", missingReferencesMessage)
		}
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneScalingUpCondition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneNotScalingUpReason,
			Message: message,
		})
		return
	}

	message := fmt.Sprintf("Scaling up from %d to %d replicas", currentReplicas, desiredReplicas)

	additionalMessages := getPreflightMessages(cluster, preflightChecks)
	if missingReferencesMessage != "" {
		additionalMessages = append(additionalMessages, fmt.Sprintf("* %s", missingReferencesMessage))
	}

	if len(additionalMessages) > 0 {
		message += fmt.Sprintf(" is blocked because:\n%s", strings.Join(additionalMessages, "\n"))
	}

	conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneScalingUpCondition,
		Status:  metav1.ConditionTrue,
		Reason:  controlplanev1.KubeadmControlPlaneScalingUpReason,
		Message: message,
	})
}

func setScalingDownCondition(_ context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines, preflightChecks internal.PreflightCheckResults) {
	if kcp.Spec.Replicas == nil {
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneScalingDownCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneScalingDownWaitingForReplicasSetReason,
			Message: "Waiting for spec.replicas set",
		})
		return
	}

	currentReplicas := int32(len(machines))
	desiredReplicas := *kcp.Spec.Replicas
	if !kcp.DeletionTimestamp.IsZero() {
		desiredReplicas = 0
	}

	if currentReplicas <= desiredReplicas {
		conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneScalingDownCondition,
			Status: metav1.ConditionFalse,
			Reason: controlplanev1.KubeadmControlPlaneNotScalingDownReason,
		})
		return
	}

	message := fmt.Sprintf("Scaling down from %d to %d replicas", currentReplicas, desiredReplicas)

	additionalMessages := getPreflightMessages(cluster, preflightChecks)
	if staleMessage := aggregateStaleMachines(machines); staleMessage != "" {
		additionalMessages = append(additionalMessages, fmt.Sprintf("* %s", staleMessage))
	}

	if len(additionalMessages) > 0 {
		message += fmt.Sprintf(" is blocked because:\n%s", strings.Join(additionalMessages, "\n"))
	}

	conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneScalingDownCondition,
		Status:  metav1.ConditionTrue,
		Reason:  controlplanev1.KubeadmControlPlaneScalingDownReason,
		Message: message,
	})
}

func setMachinesReadyCondition(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines) {
	if len(machines) == 0 {
		conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneMachinesReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneMachinesReadyNoReplicasReason,
		})
		return
	}

	readyCondition, err := conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineReadyCondition,
		conditions.TargetConditionType(controlplanev1.KubeadmControlPlaneMachinesReadyCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					controlplanev1.KubeadmControlPlaneMachinesNotReadyReason,
					controlplanev1.KubeadmControlPlaneMachinesReadyUnknownReason,
					controlplanev1.KubeadmControlPlaneMachinesReadyReason,
				)),
			),
		},
	)
	if err != nil {
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneMachinesReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachinesReadyInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineReadyCondition))
		return
	}

	conditions.Set(kcp, *readyCondition)
}

func setMachinesUpToDateCondition(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machines collections.Machines) {
	// Only consider Machines that have an UpToDate condition or are older than 10s.
	// This is done to ensure the MachinesUpToDate condition doesn't flicker after a new Machine is created,
	// because it can take a bit until the UpToDate condition is set on a new Machine.
	machines = machines.Filter(func(machine *clusterv1.Machine) bool {
		return conditions.Has(machine, clusterv1.MachineUpToDateCondition) || time.Since(machine.CreationTimestamp.Time) > 10*time.Second
	})

	if len(machines) == 0 {
		conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneMachinesUpToDateCondition,
			Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneMachinesUpToDateNoReplicasReason,
		})
		return
	}

	upToDateCondition, err := conditions.NewAggregateCondition(
		machines.UnsortedList(), clusterv1.MachineUpToDateCondition,
		conditions.TargetConditionType(controlplanev1.KubeadmControlPlaneMachinesUpToDateCondition),
		// Using a custom merge strategy to override reasons applied during merge.
		conditions.CustomMergeStrategy{
			MergeStrategy: conditions.DefaultMergeStrategy(
				conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
					controlplanev1.KubeadmControlPlaneMachinesNotUpToDateReason,
					controlplanev1.KubeadmControlPlaneMachinesUpToDateUnknownReason,
					controlplanev1.KubeadmControlPlaneMachinesUpToDateReason,
				)),
			),
		},
	)
	if err != nil {
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneMachinesUpToDateCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachinesUpToDateInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineUpToDateCondition))
		return
	}

	conditions.Set(kcp, *upToDateCondition)
}

func calculateMissingReferencesMessage(kcp *controlplanev1.KubeadmControlPlane, infraMachineTemplateNotFound bool) string {
	if infraMachineTemplateNotFound {
		return fmt.Sprintf("%s does not exist", kcp.Spec.MachineTemplate.Spec.InfrastructureRef.Kind)
	}
	return ""
}

func setRemediatingCondition(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, machinesToBeRemediated, unhealthyMachines collections.Machines) {
	if len(machinesToBeRemediated) == 0 {
		message := aggregateUnhealthyMachines(unhealthyMachines)
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneRemediatingCondition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneNotRemediatingReason,
			Message: message,
		})
		return
	}

	remediatingCondition, err := conditions.NewAggregateCondition(
		machinesToBeRemediated.UnsortedList(), clusterv1.MachineOwnerRemediatedCondition,
		conditions.TargetConditionType(controlplanev1.KubeadmControlPlaneRemediatingCondition),
		// Note: in case of the remediating conditions it is not required to use a CustomMergeStrategy/ComputeReasonFunc
		// because we are considering only machinesToBeRemediated (and we can pin the reason when we set the condition).
	)
	if err != nil {
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneRemediatingCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneRemediatingInternalErrorReason,
			Message: "Please check controller logs for errors",
		})

		log := ctrl.LoggerFrom(ctx)
		log.Error(err, fmt.Sprintf("Failed to aggregate Machine's %s conditions", clusterv1.MachineOwnerRemediatedCondition))
		return
	}

	conditions.Set(kcp, metav1.Condition{
		Type:    remediatingCondition.Type,
		Status:  metav1.ConditionTrue,
		Reason:  controlplanev1.KubeadmControlPlaneRemediatingReason,
		Message: remediatingCondition.Message,
	})
}

func setDeletingCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, deletingReason, deletingMessage string) {
	if kcp.DeletionTimestamp.IsZero() {
		conditions.Set(kcp, metav1.Condition{
			Type:   controlplanev1.KubeadmControlPlaneDeletingCondition,
			Status: metav1.ConditionFalse,
			Reason: controlplanev1.KubeadmControlPlaneNotDeletingReason,
		})
		return
	}

	conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneDeletingCondition,
		Status:  metav1.ConditionTrue,
		Reason:  deletingReason,
		Message: deletingMessage,
	})
}

func setAvailableCondition(_ context.Context, kcp *controlplanev1.KubeadmControlPlane, etcdIsManaged bool, etcdMembers []*etcd.Member, etcdMembersAndMachinesAreMatching bool, machines collections.Machines) {
	if !ptr.Deref(kcp.Status.Initialization.ControlPlaneInitialized, false) {
		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneAvailableCondition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneNotAvailableReason,
			Message: "Control plane not yet initialized",
		})
		return
	}

	if etcdIsManaged {
		if etcdMembers == nil {
			// In case the control plane just initialized, give some more time before reporting failed to get etcd members.
			// Note: Two minutes is the time after which we assume that not getting the list of etcd members is an actual problem.
			if c := conditions.Get(kcp, controlplanev1.KubeadmControlPlaneInitializedCondition); c != nil &&
				c.Status == metav1.ConditionTrue &&
				time.Since(c.LastTransitionTime.Time) < 2*time.Minute {
				conditions.Set(kcp, metav1.Condition{
					Type:    controlplanev1.KubeadmControlPlaneAvailableCondition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneNotAvailableReason,
					Message: "Waiting for etcd to report the list of members",
				})
				return
			}

			conditions.Set(kcp, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneAvailableCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneAvailableInspectionFailedReason,
				Message: "Failed to get etcd members",
			})
			return
		}

		if !etcdMembersAndMachinesAreMatching {
			conditions.Set(kcp, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneAvailableCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneNotAvailableReason,
				Message: "The list of etcd members does not match the list of Machines and Nodes",
			})
			return
		}
	}

	// Determine control plane availability looking at machines conditions, which at this stage are
	// already surfacing status from etcd member and all control plane pods hosted on every machine.
	k8sControlPlaneHealthy := 0
	k8sControlPlaneNotHealthy := 0
	k8sControlPlaneNotHealthyButNotReportedYet := 0

	for _, machine := range machines {
		// Ignore machines without a provider ID yet (which also implies infrastructure not ready).
		// Note: this avoids some noise when a new machine is provisioning; it is not possible to delay further
		// because the etcd member might join the cluster / control plane components might start even before
		// kubelet registers the node to the API server (e.g. in case kubelet has issues to register itself).
		if machine.Spec.ProviderID == "" {
			continue
		}

		// if external etcd, only look at the status of the K8s control plane components on this machine.
		if !etcdIsManaged {
			if conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition) &&
				conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition) &&
				conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition) {
				k8sControlPlaneHealthy++
			} else if shouldSurfaceWhenAvailableTrue(machine,
				controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
				controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
				controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition) {
				k8sControlPlaneNotHealthy++
			} else {
				k8sControlPlaneNotHealthyButNotReportedYet++
			}
			continue
		}

		// Otherwise, etcd is managed.
		// In this case, when looking at the k8s control plane we should consider how kubeadm layouts control plane components,
		// and more specifically:
		// - API server on one machine only connect to the local etcd member
		// - ControllerManager and scheduler on a machine connect to the local API server (not to the control plane endpoint)
		// As a consequence, we consider the K8s control plane on this machine healthy only if everything is healthy.
		if conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition) &&
			conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition) &&
			conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition) &&
			conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition) &&
			conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition) {
			k8sControlPlaneHealthy++
		} else if shouldSurfaceWhenAvailableTrue(machine,
			controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition) {
			k8sControlPlaneNotHealthy++
		} else {
			k8sControlPlaneNotHealthyButNotReportedYet++
		}
	}

	// Determine etcd members availability by using etcd members as a source of truth because
	// etcd members might not match with machines, e.g. while provisioning a new machine.
	// Also in this case, we leverage info on machines to determine member health.
	votingEtcdMembers := 0
	learnerEtcdMembers := 0
	etcdMembersHealthy := 0
	etcdMembersNotHealthy := 0
	etcdMembersNotHealthyButNotReportedYet := 0

	if etcdIsManaged {
		// Maps machines to members
		memberToMachineMap := map[string]*clusterv1.Machine{}
		provisioningMachines := []*clusterv1.Machine{}
		for _, machine := range machines {
			if !machine.Status.NodeRef.IsDefined() {
				provisioningMachines = append(provisioningMachines, machine)
				continue
			}
			for _, member := range etcdMembers {
				if machine.Status.NodeRef.Name == member.Name {
					memberToMachineMap[member.Name] = machine
					break
				}
			}
		}

		for _, etcdMember := range etcdMembers {
			// Note. We consider etcd without a name yet as learners, because this prevents them to impact quorum (this is
			// a temporary state that usually goes away very quickly).
			if etcdMember.IsLearner || etcdMember.Name == "" {
				learnerEtcdMembers++
			} else {
				votingEtcdMembers++
			}

			// In case the etcd member does not have yet a name it is not possible to find a corresponding machine,
			// but we consider the node being healthy because this is a transient state that usually goes away quickly.
			if etcdMember.Name == "" {
				etcdMembersHealthy++
				continue
			}

			// Look for the corresponding machine.
			machine := memberToMachineMap[etcdMember.Name]
			if machine == nil {
				// If there is only one provisioning machine (a machine yet without the node name), considering that KCP
				// only creates one machine at time, we can make the assumption this is the machine hosting the etcd member without a match
				if len(provisioningMachines) == 1 {
					machine = provisioningMachines[0]
					provisioningMachines = nil
				} else {
					// In case we cannot match an etcd member with a machine, we consider this an issue (it should
					// never happen with KCP).
					etcdMembersNotHealthy++
					continue
				}
			}

			// Otherwise read the status of the etcd member from he EtcdMemberHealthy condition.
			if conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition) {
				etcdMembersHealthy++
			} else if shouldSurfaceWhenAvailableTrue(machine,
				controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition) {
				etcdMembersNotHealthy++
			} else {
				etcdMembersNotHealthyButNotReportedYet++
			}
		}
	}
	etcdQuorum := (votingEtcdMembers / 2.0) + 1

	// If the control plane and etcd (if managed are available), set the condition to true taking care of surfacing partial unavailability if any.
	if kcp.DeletionTimestamp.IsZero() &&
		(!etcdIsManaged || etcdMembersHealthy >= etcdQuorum) &&
		k8sControlPlaneHealthy >= 1 &&
		conditions.IsTrue(kcp, controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition) {
		messages := []string{}

		if etcdIsManaged && etcdMembersNotHealthy > 0 {
			etcdLearnersMsg := ""
			if learnerEtcdMembers > 0 {
				etcdLearnersMsg = fmt.Sprintf(" %d learner etcd member,", learnerEtcdMembers)
			}

			// Note: When Available is true, we surface failures only after 10s they exist to avoid flakes;
			// Accordingly for this message NotHealthyButNotReportedYet sums up to Healthy.
			etcdMembersHealthyAndNotHealthyButNotReportedYet := etcdMembersHealthy + etcdMembersNotHealthyButNotReportedYet
			switch etcdMembersHealthyAndNotHealthyButNotReportedYet {
			case 1:
				messages = append(messages, fmt.Sprintf("* 1 of %d etcd members is healthy,%s at least %d healthy member required for etcd quorum", len(etcdMembers), etcdLearnersMsg, etcdQuorum))
			default:
				messages = append(messages, fmt.Sprintf("* %d of %d etcd members are healthy,%s at least %d healthy member required for etcd quorum", etcdMembersHealthyAndNotHealthyButNotReportedYet, len(etcdMembers), etcdLearnersMsg, etcdQuorum))
			}
		}

		if k8sControlPlaneNotHealthy > 0 {
			// Note: When Available is true, we surface failures only after 10s they exist to avoid flakes;
			// Accordingly for this message NotHealthyButNotReportedYet sums up to Healthy.
			k8sControlPlaneHealthyAndNotHealthyButNotReportedYet := k8sControlPlaneHealthy + k8sControlPlaneNotHealthyButNotReportedYet
			switch k8sControlPlaneHealthyAndNotHealthyButNotReportedYet {
			case 1:
				messages = append(messages, fmt.Sprintf("* 1 of %d Machines has healthy control plane components, at least 1 required", len(machines)))
			default:
				messages = append(messages, fmt.Sprintf("* %d of %d Machines have healthy control plane components, at least 1 required", k8sControlPlaneHealthyAndNotHealthyButNotReportedYet, len(machines)))
			}
		}

		conditions.Set(kcp, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneAvailableCondition,
			Status:  metav1.ConditionTrue,
			Reason:  controlplanev1.KubeadmControlPlaneAvailableReason,
			Message: strings.Join(messages, "\n"),
		})
		return
	}

	messages := []string{}
	if !kcp.DeletionTimestamp.IsZero() {
		messages = append(messages, "* Control plane metadata.deletionTimestamp is set")
	}

	if !conditions.IsTrue(kcp, controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition) {
		messages = append(messages, "* Control plane certificates are not available")
	}

	if etcdIsManaged && etcdMembersHealthy < etcdQuorum {
		etcdLearnersMsg := ""
		if learnerEtcdMembers > 0 {
			etcdLearnersMsg = fmt.Sprintf(" %d learner etcd member,", learnerEtcdMembers)
		}
		switch etcdMembersHealthy {
		case 0:
			messages = append(messages, fmt.Sprintf("* There are no healthy etcd member,%s at least %d healthy member required for etcd quorum", etcdLearnersMsg, etcdQuorum))
		case 1:
			messages = append(messages, fmt.Sprintf("* 1 of %d etcd members is healthy,%s at least %d healthy member required for etcd quorum", len(etcdMembers), etcdLearnersMsg, etcdQuorum))
		default:
			messages = append(messages, fmt.Sprintf("* %d of %d etcd members are healthy,%s at least %d healthy member required for etcd quorum", etcdMembersHealthy, len(etcdMembers), etcdLearnersMsg, etcdQuorum))
		}
	}

	if k8sControlPlaneHealthy < 1 {
		messages = append(messages, "* There are no Machines with healthy control plane components, at least 1 required")
	}

	conditions.Set(kcp, metav1.Condition{
		Type:    controlplanev1.KubeadmControlPlaneAvailableCondition,
		Status:  metav1.ConditionFalse,
		Reason:  controlplanev1.KubeadmControlPlaneNotAvailableReason,
		Message: strings.Join(messages, "\n"),
	})
}

// setLastRemediation surface lastRemediation data in status.
// LastRemediation is the remediation currently in progress, if any, or the
// most recent of the remediation we are keeping track on machines.
func setLastRemediation(_ context.Context, controlPlane *internal.ControlPlane) error {
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

// shouldSurfaceWhenAvailableTrue defines when a control plane components/etcd issue should surface when
// Available condition is true.
// The main goal of this check is to avoid to surface false negatives/flakes, and thus it requires that
// an issue exists for at least more than 10 seconds before surfacing it.
func shouldSurfaceWhenAvailableTrue(machine *clusterv1.Machine, conditionTypes ...string) bool {
	// Get the min time when one of the conditions in input transitioned to false or unknown.
	var t *time.Time
	for _, conditionType := range conditionTypes {
		c := conditions.Get(machine, conditionType)
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

func getPreflightMessages(cluster *clusterv1.Cluster, preflightChecks internal.PreflightCheckResults) []string {
	additionalMessages := []string{}
	if preflightChecks.TopologyVersionMismatch {
		v := cluster.Spec.Topology.Version
		if version, ok := cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok && version != "" {
			v = version
		}
		additionalMessages = append(additionalMessages, fmt.Sprintf("* waiting for a version upgrade to %s to be propagated", v))
	}

	if preflightChecks.HasDeletingMachine {
		additionalMessages = append(additionalMessages, "* waiting for a control plane Machine to complete deletion")
	}

	if preflightChecks.CertificateMissing {
		additionalMessages = append(additionalMessages, "* cluster certificates are missing or unknown")
	}

	if preflightChecks.ControlPlaneComponentsNotHealthy {
		additionalMessages = append(additionalMessages, "* waiting for control plane components to become healthy")
	}

	if preflightChecks.EtcdClusterNotHealthy {
		additionalMessages = append(additionalMessages, "* waiting for etcd cluster to become healthy")
	}
	return additionalMessages
}

func aggregateStaleMachines(machines collections.Machines) string {
	if len(machines) == 0 {
		return ""
	}

	machineNames := []string{}
	delayReasons := sets.Set[string]{}
	for _, machine := range machines {
		if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) > time.Minute*15 {
			machineNames = append(machineNames, machine.GetName())

			deletingCondition := conditions.Get(machine, clusterv1.MachineDeletingCondition)
			if deletingCondition != nil &&
				deletingCondition.Status == metav1.ConditionTrue &&
				deletingCondition.Reason == clusterv1.MachineDeletingDrainingNodeReason &&
				machine.Status.Deletion != nil && !machine.Status.Deletion.NodeDrainStartTime.IsZero() && time.Since(machine.Status.Deletion.NodeDrainStartTime.Time) > 5*time.Minute {
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
