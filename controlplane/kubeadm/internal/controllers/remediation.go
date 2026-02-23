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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/util"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
)

// reconcileUnhealthyMachines tries to remediate KubeadmControlPlane unhealthy machines
// based on the process described in https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20191017-kubeadm-based-control-plane.md#remediation-using-delete-and-recreate
func (r *KubeadmControlPlaneReconciler) reconcileUnhealthyMachines(ctx context.Context, controlPlane *internal.ControlPlane) (ret ctrl.Result, retErr error) {
	log := ctrl.LoggerFrom(ctx)
	reconciliationTime := time.Now().UTC()

	// Cleanup pending remediation actions not completed for any reasons (e.g. number of current replicas is less or equal to 1)
	// if the underlying machine is now back to healthy / not deleting.
	errList := []error{}
	for _, m := range controlPlane.Machines {
		if !m.DeletionTimestamp.IsZero() {
			continue
		}

		shouldCleanupV1Beta1 := v1beta1conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededV1Beta1Condition) && v1beta1conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition)
		shouldCleanup := conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededCondition) && conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedCondition)

		if !shouldCleanupV1Beta1 && !shouldCleanup {
			continue
		}

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			errList = append(errList, err)
			continue
		}

		if shouldCleanupV1Beta1 {
			v1beta1conditions.Delete(m, clusterv1.MachineOwnerRemediatedV1Beta1Condition)
		}

		if shouldCleanup {
			conditions.Delete(m, clusterv1.MachineOwnerRemediatedCondition)
		}

		if err := patchHelper.Patch(ctx, m, patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			clusterv1.MachineOwnerRemediatedV1Beta1Condition,
		}}, patch.WithOwnedConditions{Conditions: []string{
			clusterv1.MachineOwnerRemediatedCondition,
		}}); err != nil {
			errList = append(errList, err)
		}
	}
	if len(errList) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errList)
	}

	// Cleanup stale RemediationInProgressAnnotation annotations.
	// Check if the annotation is stale; this might happen in case there is a crash in the controller in between
	// when a new Machine is created and the annotation is eventually removed from KCP via defer patch at the end
	// of KCP reconcile.
	if v, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		remediationData, err := RemediationDataFromAnnotation(v)
		if err != nil {
			return ctrl.Result{}, err
		}

		for _, m := range controlPlane.Machines.UnsortedList() {
			if m.CreationTimestamp.After(remediationData.Timestamp.Time) {
				// Remove the annotation tracking that a remediation is in progress (the annotation is stale).
				delete(controlPlane.KCP.Annotations, controlplanev1.RemediationInProgressAnnotation)
				break
			}
		}
	}

	// Gets all machines that have `MachineHealthCheckSucceeded=False` (indicating a problem was detected on the machine)
	// and `MachineOwnerRemediated` is false, indicating that this controller is responsible for performing remediation.
	machinesToBeRemediated := controlPlane.MachinesToBeRemediatedByKCP()

	// If there are no machines to remediated, return so KCP can proceed with other operations (ctrl.Result nil).
	if len(machinesToBeRemediated) == 0 {
		return ctrl.Result{}, nil
	}

	// Select the machine to be remediated, which is the oldest machine to be remediated not yet provisioned (if any)
	// or the oldest machine to be remediated.
	//
	// NOTE: The current solution is considered acceptable for the most frequent use case (only one machine to be remediated),
	// however, in the future this could potentially be improved for the scenario where more than one machine to be remediated exists
	// by considering which machine has lower impact on etcd quorum.
	machineToBeRemediated := getMachineToBeRemediated(machinesToBeRemediated, controlPlane.IsEtcdManaged())
	if machineToBeRemediated == nil {
		return ctrl.Result{}, errors.New("failed to find a Machine to remediate within unhealthy Machines")
	}

	// Returns if the machine is in the process of being deleted.
	if !machineToBeRemediated.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	var initialized bool
	if ptr.Deref(controlPlane.KCP.Status.Initialization.ControlPlaneInitialized, false) {
		initialized = true
	}
	log = log.WithValues("Machine", klog.KObj(machineToBeRemediated), "initialized", initialized)

	// Returns if another remediation is in progress but the new Machine is not yet created.
	// Note: This condition is checked after we check for machines to be remediated and if machineToBeRemediated
	// is not being deleted to avoid unnecessary logs if no further remediation should be done.
	if _, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		log.Info("Another remediation is already in progress. Skipping remediation.")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(machineToBeRemediated, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to Patch the Machine conditions after each reconcileUnhealthyMachines.
		if err := patchHelper.Patch(ctx, machineToBeRemediated,
			patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineOwnerRemediatedV1Beta1Condition,
			}},
			patch.WithOwnedConditions{Conditions: []string{
				clusterv1.MachineOwnerRemediatedCondition,
			}},
		); err != nil {
			log.Error(err, "Failed to patch control plane Machine", "Machine", machineToBeRemediated.Name)
			if retErr == nil {
				retErr = errors.Wrapf(err, "failed to patch control plane Machine %s", machineToBeRemediated.Name)
			}
		}
	}()

	// Before starting remediation, run preflight checks in order to verify it is safe to remediate.
	// If any of the following checks fails, we'll surface the reason in the MachineOwnerRemediated condition.

	if feature.Gates.Enabled(feature.ClusterTopology) {
		// Skip remediation when we expect an upgrade to be propagated from the cluster topology.
		if controlPlane.Cluster.Spec.Topology.IsDefined() && controlPlane.Cluster.Spec.Topology.Version != controlPlane.KCP.Spec.Version {
			message := fmt.Sprintf("KubeadmControlPlane can't remediate while waiting for a version upgrade to %s to be propagated from Cluster.spec.topology", controlPlane.Cluster.Spec.Topology.Version)
			log.Info(fmt.Sprintf("A control plane machine needs remediation, but %s. Skipping remediation", message))
			v1beta1conditions.MarkFalse(machineToBeRemediated,
				clusterv1.MachineOwnerRemediatedV1Beta1Condition,
				clusterv1.WaitingForRemediationV1Beta1Reason,
				clusterv1.ConditionSeverityWarning,
				"%s", message)

			conditions.Set(machineToBeRemediated, metav1.Condition{
				Type:    clusterv1.MachineOwnerRemediatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason,
				Message: message,
			})

			return ctrl.Result{}, nil
		}
	}

	// Check if KCP is allowed to remediate considering retry limits:
	// - Remediation cannot happen because retryPeriod is not yet expired.
	// - KCP already reached MaxRetries limit.
	remediationInProgressData, canRemediate, err := r.checkRetryLimits(log, machineToBeRemediated, controlPlane, reconciliationTime)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !canRemediate {
		// NOTE: log lines and conditions surfacing why it is not possible to remediate are set by checkRetryLimits.
		return ctrl.Result{}, nil
	}

	if ptr.Deref(controlPlane.KCP.Status.Initialization.ControlPlaneInitialized, false) {
		// Executes checks that apply only if the control plane is already initialized; in this case KCP can
		// remediate only if it can safely assume that the operation preserves the operation state of the
		// existing cluster (or at least it doesn't make it worse).

		// The cluster MUST have more than one replica, because this is the smallest cluster size that allows any etcd failure tolerance.
		if controlPlane.Machines.Len() <= 1 {
			log.Info("A control plane machine needs remediation, but the number of current replicas is less or equal to 1. Skipping remediation", "replicas", controlPlane.Machines.Len())
			v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate if current replicas are less or equal to 1")

			conditions.Set(machineToBeRemediated, metav1.Condition{
				Type:    clusterv1.MachineOwnerRemediatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason,
				Message: "KubeadmControlPlane can't remediate if current replicas are less or equal to 1",
			})
			return ctrl.Result{}, nil
		}

		// The cluster MUST NOT have healthy machines still being provisioned. This rule prevents KCP taking actions while the cluster is in a transitional state.
		if controlPlane.HasHealthyMachineStillProvisioning() {
			log.Info("A control plane machine needs remediation, but there are other control-plane machines being provisioned. Skipping remediation")
			v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane waiting for control plane machine provisioning to complete before triggering remediation")

			conditions.Set(machineToBeRemediated, metav1.Condition{
				Type:    clusterv1.MachineOwnerRemediatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason,
				Message: "KubeadmControlPlane waiting for control plane Machine provisioning to complete before triggering remediation",
			})
			return ctrl.Result{}, nil
		}

		// The cluster MUST have no machines with a deletion timestamp. This rule prevents KCP taking actions while the cluster is in a transitional state.
		if controlPlane.HasDeletingMachine() {
			log.Info("A control plane machine needs remediation, but there are other control-plane machines being deleted. Skipping remediation")
			v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane waiting for control plane machine deletion to complete before triggering remediation")

			conditions.Set(machineToBeRemediated, metav1.Condition{
				Type:    clusterv1.MachineOwnerRemediatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason,
				Message: "KubeadmControlPlane waiting for control plane Machine deletion to complete before triggering remediation",
			})
			return ctrl.Result{}, nil
		}

		// At this point we can assume that:
		// - There are Machines to be remediated, remediation is possible because there is more than one control plane Machine,
		//   and remediation is allowed by remediation settings (e.g. retry limits).
		// - No other operations are in progress on control plane Machines:
		//   - There is no another remediation in progress
		//   - There are no Machines still provisioning or deleting
		//
		// Once all the preconditions of remediating a Machine are met, KCP should assess the potential effects of this operation.
		//
		// Most specifically, before deleting the Machine to be remediated, KCP should determine if this operation
		// is going to leave the K8s control plane components and the etcd cluster in operational state or not.
		//
		// NOTE: A similar check will be performed again right before removing the etcd member/removing the pre-terminate hook
		// at the end of the deletion process for control plane Machines.
		if !r.canSafelyRemediateMachine(ctx, controlPlane, machineToBeRemediated) {
			log.Info("A control plane Machine needs remediation, but removing this Machine could result in loosing Kubernetes control plane components or in etcd quorum loss. Skipping remediation")
			v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate this Machine because this could result in loosing Kubernetes control plane components or in etcd quorum loss")

			conditions.Set(machineToBeRemediated, metav1.Condition{
				Type:    clusterv1.MachineOwnerRemediatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason,
				Message: "KubeadmControlPlane can't remediate this Machine because this could result in loosing Kubernetes control plane components or in etcd quorum loss",
			})
			return ctrl.Result{}, nil
		}

		// Start remediating the unhealthy control plane machine by deleting it.
		// A new machine will come up completing the operation as part of the regular reconcile.

		// If the control plane is initialized, before deleting the machine:
		// - if the machine hosts the etcd leader, forward etcd leadership to another machine.
		// - delete the etcd member hosted on the machine being deleted.
		// - remove the etcd member from the kubeadm config map (only for kubernetes version older than v1.22.0)
		workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
		if err != nil {
			log.Error(err, "Failed to create client to workload cluster")
			return ctrl.Result{}, errors.Wrapf(err, "failed to create client to workload cluster")
		}

		// If the machine that is about to be deleted is the etcd leader, move it to the newest member available.
		if controlPlane.IsEtcdManaged() {
			etcdLeaderCandidate := controlPlane.HealthyMachines().Newest()
			if etcdLeaderCandidate == nil {
				log.Info("A control plane machine needs remediation, but there is no healthy machine to forward etcd leadership to")
				v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.RemediationFailedV1Beta1Reason, clusterv1.ConditionSeverityWarning,
					"A control plane machine needs remediation, but there is no healthy machine to forward etcd leadership to. Skipping remediation")

				conditions.Set(machineToBeRemediated, metav1.Condition{
					Type:    clusterv1.MachineOwnerRemediatedCondition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason,
					Message: "KubeadmControlPlane can't remediate this Machine because there is no healthy Machine to forward etcd leadership to",
				})
				return ctrl.Result{}, nil
			}
			if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToBeRemediated, etcdLeaderCandidate); err != nil {
				log.Error(err, "Failed to move etcd leadership to candidate machine", "candidate", klog.KObj(etcdLeaderCandidate))
				v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.RemediationFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "%s", err.Error())

				conditions.Set(machineToBeRemediated, metav1.Condition{
					Type:    clusterv1.MachineOwnerRemediatedCondition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneMachineRemediationInternalErrorReason,
					Message: "Please check controller logs for errors",
				})
				return ctrl.Result{}, err
			}

			// NOTE: etcd member removal will be performed by the kcp-cleanup hook after machine completes drain & all volumes are detached.
		}
	}

	// Delete the machine
	if err := r.Client.Delete(ctx, machineToBeRemediated); err != nil {
		v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.RemediationFailedV1Beta1Reason, clusterv1.ConditionSeverityError, "%s", err.Error())

		conditions.Set(machineToBeRemediated, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneMachineRemediationInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete unhealthy machine %s", machineToBeRemediated.Name)
	}

	// Surface the operation is in progress.
	// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
	// Also, setting DeletionTimestamp doesn't mean the Machine is actually deleted (deletion takes some time).
	log.WithValues(controlPlane.StatusToLogKeyAndValues(nil, machineToBeRemediated)...).
		Info("Deleting Machine (remediating unhealthy Machine)")
	v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.RemediationInProgressV1Beta1Reason, clusterv1.ConditionSeverityWarning, "")

	conditions.Set(machineToBeRemediated, metav1.Condition{
		Type:    clusterv1.MachineOwnerRemediatedCondition,
		Status:  metav1.ConditionFalse,
		Reason:  controlplanev1.KubeadmControlPlaneMachineRemediationMachineDeletingReason,
		Message: "Machine is deleting",
	})

	// Prepare the info for tracking the remediation progress into the RemediationInProgressAnnotation.
	remediationInProgressValue, err := remediationInProgressData.Marshal()
	if err != nil {
		return ctrl.Result{}, err
	}

	// Set annotations tracking remediation details so they can be picked up by the machine
	// that will be created as part of the scale up action that completes the remediation.
	annotations.AddAnnotations(controlPlane.KCP, map[string]string{
		controlplanev1.RemediationInProgressAnnotation: remediationInProgressValue,
	})

	return ctrl.Result{RequeueAfter: time.Millisecond}, nil // Technically there is no need to requeue here. Machine deletion above triggers reconciliation. But we have to return a non-zero Result so reconcile above returns.
}

// Gets the machine to be remediated, which is the "most broken" among the unhealthy machines, determined as the machine
// having the highest priority issue that other machines have not.
// The following issues are considered (from highest to lowest priority):
// - machine with RemediateMachineAnnotation annotation
// - machine without .status.nodeRef
// - machine with etcd issue or etcd status unknown (etcd member, etcd pod)
// - machine with control plane component issue or status unknown (API server, controller manager, scheduler)
//
// Note: In case of more than one faulty machine the chance to recover mostly depends on the control plane being able to
// successfully create a replacement Machine, because due to scale up preflight checks, this cannot happen if there are
// still issues on the control plane after the first remediation.
// This func tries to maximize those chances of a successful remediation by picking for remediation the "most broken" machine first.
func getMachineToBeRemediated(unhealthyMachines collections.Machines, isEtcdManaged bool) *clusterv1.Machine {
	if unhealthyMachines.Len() == 0 {
		return nil
	}

	machinesToBeRemediated := unhealthyMachines.UnsortedList()
	if len(machinesToBeRemediated) == 1 {
		return machinesToBeRemediated[0]
	}

	sort.Slice(machinesToBeRemediated, func(i, j int) bool {
		return pickMachineToBeRemediated(machinesToBeRemediated[i], machinesToBeRemediated[j], isEtcdManaged)
	})
	return machinesToBeRemediated[0]
}

// pickMachineToBeRemediated returns true if machine i should be remediated before machine j.
func pickMachineToBeRemediated(i, j *clusterv1.Machine, isEtcdManaged bool) bool {
	// If one machine has the RemediateMachineAnnotation annotation, remediate first.
	if annotations.HasRemediateMachine(i) && !annotations.HasRemediateMachine(j) {
		return true
	}
	if !annotations.HasRemediateMachine(i) && annotations.HasRemediateMachine(j) {
		return false
	}

	// if one machine does not have a node ref, we assume that provisioning failed and there is no CP components at all,
	// so remediate first; also without a node, it is not possible to get further info about status.
	if !i.Status.NodeRef.IsDefined() && j.Status.NodeRef.IsDefined() {
		return true
	}
	if i.Status.NodeRef.IsDefined() && !j.Status.NodeRef.IsDefined() {
		return false
	}

	// if one machine has unhealthy etcd member or pod, remediate first.
	if isEtcdManaged {
		if p := pickMachineToBeRemediatedByConditionState(i, j, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition); p != nil {
			return *p
		}
		if p := pickMachineToBeRemediatedByConditionState(i, j, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition); p != nil {
			return *p
		}

		// Note: in the future we might consider etcd leadership and kubelet status to prevent being stuck when it is not possible
		// to forward leadership, but this requires further investigation and most probably also to surface a few additional info in the controlPlane object.
	}

	// if one machine has unhealthy control plane component, remediate first.
	if p := pickMachineToBeRemediatedByConditionState(i, j, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition); p != nil {
		return *p
	}
	if p := pickMachineToBeRemediatedByConditionState(i, j, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition); p != nil {
		return *p
	}
	if p := pickMachineToBeRemediatedByConditionState(i, j, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition); p != nil {
		return *p
	}

	// Use oldest (and Name) as a tie-breaker criteria.
	if i.CreationTimestamp.Equal(&j.CreationTimestamp) {
		return i.Name < j.Name
	}
	return i.CreationTimestamp.Before(&j.CreationTimestamp)
}

// pickMachineToBeRemediatedByConditionState returns true if condition t report issue on machine i and not on machine j,
// false if the vice-versa apply, or nil if condition t doesn't provide a discriminating criteria for picking one machine or another for remediation.
func pickMachineToBeRemediatedByConditionState(i, j *clusterv1.Machine, conditionType string) *bool {
	iCondition := conditions.IsTrue(i, conditionType)
	jCondition := conditions.IsTrue(j, conditionType)

	if !iCondition && jCondition {
		return ptr.To(true)
	}
	if iCondition && !jCondition {
		return ptr.To(false)
	}
	return nil
}

// checkRetryLimits checks if KCP is allowed to remediate considering retry limits:
// - Remediation cannot happen because retryPeriod is not yet expired.
// - KCP already reached the maximum number of retries for a machine.
// NOTE: Counting the number of retries is required In order to prevent infinite remediation e.g. in case the
// first Control Plane machine is failing due to quota issue.
func (r *KubeadmControlPlaneReconciler) checkRetryLimits(log logr.Logger, machineToBeRemediated *clusterv1.Machine, controlPlane *internal.ControlPlane, reconciliationTime time.Time) (*RemediationData, bool, error) {
	// Get last remediation info from the machine.
	var lastRemediationData *RemediationData
	if value, ok := machineToBeRemediated.Annotations[controlplanev1.RemediationForAnnotation]; ok {
		l, err := RemediationDataFromAnnotation(value)
		if err != nil {
			return nil, false, err
		}
		lastRemediationData = l
	}

	remediationInProgressData := &RemediationData{
		Machine:    machineToBeRemediated.Name,
		Timestamp:  metav1.Time{Time: reconciliationTime},
		RetryCount: 0,
	}

	// If there is no last remediation, this is the first try of a new retry sequence.
	if lastRemediationData == nil {
		return remediationInProgressData, true, nil
	}

	// Gets MinHealthyPeriodSeconds and RetryPeriodSeconds from the remediation strategy, or use defaults.
	minHealthyPeriod := time.Duration(controlplanev1.DefaultMinHealthyPeriodSeconds) * time.Second
	if controlPlane.KCP.Spec.Remediation.MinHealthyPeriodSeconds != nil {
		minHealthyPeriod = time.Duration(*controlPlane.KCP.Spec.Remediation.MinHealthyPeriodSeconds) * time.Second
	}
	retryPeriod := time.Duration(ptr.Deref(controlPlane.KCP.Spec.Remediation.RetryPeriodSeconds, 0)) * time.Second

	// Gets the timestamp of the last remediation; if missing, default to a value
	// that ensures both MinHealthyPeriodSeconds and RetryPeriodSeconds are expired.
	// NOTE: this could potentially lead to executing more retries than expected or to executing retries before than
	// expected, but this is considered acceptable when the system recovers from someone/something changes or deletes
	// the RemediationForAnnotation on Machines.
	lastRemediationTime := reconciliationTime.Add(-2 * max(minHealthyPeriod, retryPeriod))
	if !lastRemediationData.Timestamp.IsZero() {
		lastRemediationTime = lastRemediationData.Timestamp.Time
	}

	// Once we get here we already know that there was a last remediation for the Machine.
	// If the current remediation is happening before minHealthyPeriod is expired, then KCP considers this
	// as a remediation for the same previously unhealthy machine.
	// NOTE: If someone/something changes the RemediationForAnnotation on Machines (e.g. changes the lastRemediation time),
	// this could potentially lead to executing more retries than expected, but this is considered acceptable in such a case.
	var retryForSameMachineInProgress bool
	if lastRemediationTime.Add(minHealthyPeriod).After(reconciliationTime) {
		retryForSameMachineInProgress = true
		log = log.WithValues("remediationRetryFor", klog.KRef(machineToBeRemediated.Namespace, lastRemediationData.Machine))
	}

	// If the retry for the same machine is not in progress, this is the first try of a new retry sequence.
	if !retryForSameMachineInProgress {
		return remediationInProgressData, true, nil
	}

	// If the remediation is for the same machine, carry over the retry count.
	remediationInProgressData.RetryCount = lastRemediationData.RetryCount

	// Check if remediation can happen because retryPeriod is passed.
	if lastRemediationTime.Add(retryPeriod).After(reconciliationTime) {
		log.Info(fmt.Sprintf("A control plane machine needs remediation, but the operation already failed in the latest %s. Skipping remediation", retryPeriod))
		v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate this machine because the operation already failed in the latest %s (RetryPeriodSeconds)", retryPeriod)

		conditions.Set(machineToBeRemediated, metav1.Condition{
			Type:    clusterv1.MachineOwnerRemediatedCondition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneMachineRemediationDeferredReason,
			Message: fmt.Sprintf("KubeadmControlPlane can't remediate this machine because the operation already failed in the latest %s (RetryPeriodSeconds)", retryPeriod),
		})
		return remediationInProgressData, false, nil
	}

	// Check if remediation can happen because of maxRetry is not reached yet, if defined.
	if controlPlane.KCP.Spec.Remediation.MaxRetry != nil {
		maxRetry := int(*controlPlane.KCP.Spec.Remediation.MaxRetry)
		if remediationInProgressData.RetryCount >= maxRetry {
			log.Info(fmt.Sprintf("A control plane machine needs remediation, but the operation already failed %d times (MaxRetry %d). Skipping remediation", remediationInProgressData.RetryCount, maxRetry))
			v1beta1conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedV1Beta1Condition, clusterv1.WaitingForRemediationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "KubeadmControlPlane can't remediate this machine because the operation already failed %d times (MaxRetry)", maxRetry)

			conditions.Set(machineToBeRemediated, metav1.Condition{
				Type:    clusterv1.MachineOwnerRemediatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineCannotBeRemediatedReason,
				Message: fmt.Sprintf("KubeadmControlPlane can't remediate this machine because the operation already failed %d times (MaxRetry)", maxRetry),
			})
			return remediationInProgressData, false, nil
		}
	}

	// All the check passed, increase the remediation retry count.
	remediationInProgressData.RetryCount++

	return remediationInProgressData, true, nil
}

// canSafelyRemediateMachine determine if remediating a Machine will leave the Kubernetes control plane components and the etcd cluster in operational state or not.
func (r *KubeadmControlPlaneReconciler) canSafelyRemediateMachine(ctx context.Context, controlPlane *internal.ControlPlane, machineToBeRemediated *clusterv1.Machine) bool {
	log := ctrl.LoggerFrom(ctx)

	// In order to determine if remediation a Machine will leave the Kubernetes control plane components and the etcd cluster in operational state or not,
	// KCP must determine how the list of Machines after remediation and consider the health of the corresponding Kubernetes control plane components/etcd members.
	//
	// Target list of Machines will have current Machines -1 Machine (the machineToBeRemediated).
	// As a consequence:
	// - Kubernetes control plane components on the Machine being remediated is going to be deleted, no Kubernetes control plane components are going to be added.
	kubernetesControlPlaneToBeDeleted := machineToBeRemediated.Name
	addKubernetesControlPlane := false

	// Check id the target Kubernetes control plane will have at least one set of operational Kubernetes control plane components.
	if !r.targetKubernetesControlPlaneComponentsHealthy(ctx, controlPlane, addKubernetesControlPlane, kubernetesControlPlaneToBeDeleted) {
		return false
	}

	// If etcd is not managed, no other checks are required.
	if !controlPlane.IsEtcdManaged() {
		return true
	}

	// etcd member on the Machine being remediated is going to be deleted, no etcd member are going to be added.
	etcdMemberToBeDeleted := r.tryGetEtcdMemberName(ctx, controlPlane, machineToBeRemediated)
	addEtcdMember := false

	// If it was not possible to get the etcd member, no other checks can be performed.
	// it is not possible to determine if etcd will never show up due to the issue on the machine being remediated,
	// or if it will show up later. In this case KCP continue with remediation, which is considered as user intent
	// (no matter if expressed as MHC configuration or via the manual remediation annotation).
	// Note: reconcile reconcilePreTerminateHook will try to perform this check again.
	if etcdMemberToBeDeleted == "" {
		return true
	}

	// Check target etcd cluster.
	if len(controlPlane.EtcdMembers) == 0 {
		log.Info("cannot check etcd cluster health before remediation, etcd member list is empty")
		return false
	}
	return r.targetEtcdClusterHealthy(ctx, controlPlane, addEtcdMember, etcdMemberToBeDeleted)
}

// tryGetEtcdMemberName tries to get the name of the etcd member hosted on a Machine, which is the same of the Node name.
// When Node name might be not yet surfaced on the Machine, this code tries to retrieve it (best effort).
func (r *KubeadmControlPlaneReconciler) tryGetEtcdMemberName(ctx context.Context, controlPlane *internal.ControlPlane, deletingMachine *clusterv1.Machine) string {
	log := ctrl.LoggerFrom(ctx)

	if deletingMachine.Status.NodeRef.Name != "" {
		return deletingMachine.Status.NodeRef.Name
	}

	var providerID, node string
	if deletingMachine.Spec.ProviderID != "" {
		providerID = deletingMachine.Spec.ProviderID
	} else if infraMachine := controlPlane.InfraResources[deletingMachine.Name]; infraMachine != nil {
		if providerIDFromInfraMachine, err := contract.InfrastructureMachine().ProviderID().Get(infraMachine); err == nil {
			providerID = *providerIDFromInfraMachine
		}
	}
	if providerID != "" {
		remoteClient, err := r.ClusterCache.GetClient(ctx, client.ObjectKeyFromObject(controlPlane.Cluster))
		if err != nil {
			log.Error(err, fmt.Sprintf("Failed to get cluster client while retrieving the etcd member name for %s", deletingMachine.Name))
		} else {
			nodeList := corev1.NodeList{}
			if err := remoteClient.List(ctx, &nodeList, client.MatchingFields{index.NodeProviderIDField: providerID}); err == nil {
				if len(nodeList.Items) == 1 {
					node = nodeList.Items[0].Name
				}
			}
		}
	}
	return node
}

// targetEtcdClusterHealthy assess if it is possible to transition to the target state of the etcd cluster
// without loosing etcd quorum.
//
// The result of the assessment mostly depend on the existence of failing members in the target cluster.
// E.g. according to the etcd fault tolerance specification (see https://etcd.io/docs/v3.3/faq/#what-is-failure-tolerance)
//
//   - 3 CP etcd cluster does not tolerate additional failing members on top of the one being deleted (the target
//     cluster size after deletion is 2, fault tolerance 0)
//   - 5 CP etcd cluster tolerates 1 additional failing members on top of the one being deleted (the target
//     cluster size after deletion is 4, fault tolerance 1)
//   - 7 CP etcd cluster tolerates 2 additional failing members on top of the one being deleted (the target
//     cluster size after deletion is 6, fault tolerance 2)
//   - etc.
//
// Similar considerations must be taken into account when adding new etcd members, because this operation
// might also increase the number of failing members in the etcd clusters.
//
// Note: This check leverage the information collected in reconcileControlPlaneAndMachinesConditions at the beginning of reconcile;
// the info are also used to compute status.Conditions.
//
// Note: This check is performed on the actual list of members, so it account also for cases where
// there is a mis-alignment between number of Machines, corresponding Nodes and etcd members.
func (r *KubeadmControlPlaneReconciler) targetEtcdClusterHealthy(ctx context.Context, controlPlane *internal.ControlPlane, addEtcdMember bool, etcdMemberToBeDeleted string) bool {
	log := ctrl.LoggerFrom(ctx)

	currentTotalMembers := len(controlPlane.EtcdMembers)

	log.Info("etcd cluster current state",
		"currentTotalMembers", currentTotalMembers,
		"currentMembers", util.MemberNames(controlPlane.EtcdMembers))

	// Projects the target etcd cluster after required changes.
	healthyMembers := []string{}
	unhealthyMembers := []string{}

	targetTotalMembers := 0
	targetLearnerMembers := 0
	targetVotingMembers := 0

	// When assessing the impact of adding new members, KCP always assume the worst case, that is the new etcd members won't be healthy.
	// This is why the additional etcd member is added to unhealthyMembers; the only exception is when there is zero or one etcd member
	// in the cluster, because otherwise it won't be possible to scale up from 0 to 1 and from 1 to 2 (with tot members 2, tolerance to failure is still 0).
	// Note: the new member is always considered in total members amd total voting members, no matter we assuming it will be healthy or not.
	if addEtcdMember {
		targetTotalMembers = 1
		targetVotingMembers = 1
		if len(controlPlane.EtcdMembers) > 1 {
			unhealthyMembers = append(unhealthyMembers, "1 new member (worst case)")
		} else {
			healthyMembers = append(healthyMembers, "1 new member (best case)")
		}
	}

	for _, etcdMember := range controlPlane.EtcdMembers {
		// Consider members without a name as a learner (they are still starting up).
		if etcdMember.Name == "" {
			targetTotalMembers++
			targetLearnerMembers++
			unhealthyMembers = append(unhealthyMembers, "1 member starting (worst case)")
			continue
		}

		// Skip the etcd member to be deleted because it won't be part of the target etcd cluster.
		if etcdMemberToBeDeleted == etcdMember.Name {
			continue
		}

		// Include the member in the target etcd cluster.
		targetTotalMembers++
		if etcdMember.IsLearner {
			targetLearnerMembers++
		} else {
			targetVotingMembers++
		}

		// Search for the machine corresponding to the etcd member.
		var machine *clusterv1.Machine
		for _, m := range controlPlane.Machines {
			if m.Status.NodeRef.IsDefined() && m.Status.NodeRef.Name == etcdMember.Name {
				machine = m
				break
			}
		}

		// If an etcd member does not have a corresponding Machine, it is not possible to retrieve etcd member health,
		// computed in reconcileControlPlaneAndMachinesConditions. Fallback on checking etcd alarms only.
		// Note: members alarms are only a subset of the checks included in the EtcdMemberHealthyCondition.
		if machine == nil {
			hasAlarms := false
			for _, alarm := range controlPlane.EtcdMembersAlarms {
				if alarm.Type == etcd.AlarmOK {
					continue
				}

				if alarm.MemberID != etcdMember.ID {
					continue
				}
				hasAlarms = true
				break
			}
			if hasAlarms {
				log.Info("An etcd member does not have a corresponding Machine, the member is reporting alarms", "memberName", etcdMember.Name)
				unhealthyMembers = append(unhealthyMembers, fmt.Sprintf("%s (no machine)", etcdMember.Name))
				continue
			}

			log.Info("An etcd member does not have a corresponding Machine, assuming member is healthy because it has no alarms", "memberName", etcdMember.Name)
			healthyMembers = append(healthyMembers, fmt.Sprintf("%s (no machine)", etcdMember.Name))
			continue
		}

		// Check etcd member health as reported by Machine's eEtcdMemberHealthy condition.
		if !conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition) {
			unhealthyMembers = append(unhealthyMembers, fmt.Sprintf("%s (%s)", etcdMember.Name, machine.Name))
			continue
		}

		healthyMembers = append(healthyMembers, fmt.Sprintf("%s (%s)", etcdMember.Name, machine.Name))
	}

	// See https://etcd.io/docs/v3.3/faq/#what-is-failure-tolerance for fault tolerance formula explanation.
	targetQuorum := (targetVotingMembers / 2.0) + 1
	canSafelyTransitionToTargetState := targetVotingMembers-len(unhealthyMembers) >= targetQuorum

	// Force the result to false in case there are learner etcd members; KCP should wait for those members being promoted
	// before taking further steps.
	if targetLearnerMembers > 0 {
		canSafelyTransitionToTargetState = false
	}

	operations := []string{}
	if etcdMemberToBeDeleted != "" {
		operations = append(operations, fmt.Sprintf("removal of etcdMember %s", etcdMemberToBeDeleted))
	}
	if addEtcdMember {
		operations = append(operations, "addition of 1 etcdMember")
	}
	log.Info(fmt.Sprintf("etcd cluster considering %s", strings.Join(operations, ",")),
		"healthyMembers", healthyMembers,
		"unhealthyMembers", unhealthyMembers,
		"totalMembers", targetTotalMembers,
		"totalLearnerMembers", targetLearnerMembers,
		"totalVotingMembers", targetVotingMembers,
		"quorum", targetQuorum,
		"totalHealthyMembers", len(healthyMembers),
		"totalUnhealthyMembers", len(unhealthyMembers),
		"canSafelyTransitionToTargetState", canSafelyTransitionToTargetState)

	return canSafelyTransitionToTargetState
}

// targetKubernetesControlPlaneComponentsHealthy assess if it is possible to transition to the target state of the Kubernetes control plane components
// while preserving at least one fully operational set of Kubernetes control plane components, which is also required to allow Machine join.
//
// This operation takes into account how kubeadm is wiring up control plane components and more specifically:
// - API server on one machine only connect to the local etcd member
// - ControllerManager and scheduler on a machine connect to the local API server (not to the control plane endpoint)
// - KCP enables KubeletLocalMode.
//
// As a consequence, we consider the Kubernetes control plane on this machine healthy only if everything is healthy.
//
// Note: This check leverage the information collected in reconcileControlPlaneAndMachinesConditions at the beginning of reconcile;
// the info are also used to compute status.conditions.
//
// Note: When etcd is managed also the etcd pod is included in the check.
func (r *KubeadmControlPlaneReconciler) targetKubernetesControlPlaneComponentsHealthy(ctx context.Context, controlPlane *internal.ControlPlane, addKubernetesControlPlane bool, kubernetesControlPlaneToBeDeleted string) bool {
	log := ctrl.LoggerFrom(ctx)

	log.Info("Kubernetes control plane components current state",
		"currentTotalControlPlanes", len(controlPlane.Machines),
		"currentControlPlanes", controlPlane.Machines.Names())

	totControlPlanes := 0

	healthyControlPlanes := []string{}
	unhealthyControlPlanes := []string{}

	// When assessing the impact of adding new Kubernetes control plane instances, KCP always assume the worst case, that is the new set of
	// Kubernetes control plane components won't be healthy; the only exception is when there are no Machines
	// in the cluster, because otherwise it won't be possible to scale up from 0 to 1.
	if addKubernetesControlPlane {
		totControlPlanes = 1
		if len(controlPlane.Machines) > 0 {
			unhealthyControlPlanes = append(unhealthyControlPlanes, "1 new machine (worst case)")
		} else {
			healthyControlPlanes = append(healthyControlPlanes, "1 new machine (best case)")
		}
	}

	for _, machine := range controlPlane.Machines {
		if kubernetesControlPlaneToBeDeleted == machine.Name {
			continue
		}
		totControlPlanes++

		if !conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition) ||
			!conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition) ||
			!conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition) {
			unhealthyControlPlanes = append(unhealthyControlPlanes, machine.Name)
			continue
		}
		if controlPlane.IsEtcdManaged() {
			if !conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition) {
				unhealthyControlPlanes = append(unhealthyControlPlanes, machine.Name)
				continue
			}
		}

		healthyControlPlanes = append(healthyControlPlanes, machine.Name)
	}

	canSafelyTransitionToTargetState := len(healthyControlPlanes) >= 1

	operations := []string{}
	if kubernetesControlPlaneToBeDeleted != "" {
		operations = append(operations, fmt.Sprintf("removal of Machine %s", kubernetesControlPlaneToBeDeleted))
	}
	if addKubernetesControlPlane {
		operations = append(operations, "addition of 1 Machine")
	}
	log.Info(fmt.Sprintf("Kubernetes control plane components considering %s", strings.Join(operations, ",")),
		"healthyControlPlanes", healthyControlPlanes,
		"unhealthyControlPlanes", unhealthyControlPlanes,
		"totalControlPlanes", totControlPlanes,
		"totalHealthyControlPlanes", len(healthyControlPlanes),
		"totalUnhealthyControlPlanes", len(unhealthyControlPlanes),
		"canSafelyTransitionToTargetState", canSafelyTransitionToTargetState)

	return canSafelyTransitionToTargetState
}

// RemediationData struct is used to keep track of information stored in the RemediationInProgressAnnotation in KCP
// during remediation and then into the RemediationForAnnotation on the replacement machine once it is created.
type RemediationData struct {
	// machine is the machine name of the latest machine being remediated.
	Machine string `json:"machine"`

	// timestamp is when last remediation happened. It is represented in RFC3339 form and is in UTC.
	Timestamp metav1.Time `json:"timestamp"`

	// retryCount used to keep track of remediation retry for the last remediated machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	RetryCount int `json:"retryCount"`
}

// RemediationDataFromAnnotation gets RemediationData from an annotation value.
func RemediationDataFromAnnotation(value string) (*RemediationData, error) {
	ret := &RemediationData{}
	if err := json.Unmarshal([]byte(value), ret); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal value %s for %s annotation", value, clusterv1.RemediationInProgressV1Beta1Reason)
	}
	return ret, nil
}

// Marshal an RemediationData into an annotation value.
func (r *RemediationData) Marshal() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal value for %s annotation", clusterv1.RemediationInProgressV1Beta1Reason)
	}
	return string(b), nil
}

// ToStatus converts a RemediationData into a LastRemediationStatus struct.
func (r *RemediationData) ToStatus() controlplanev1.LastRemediationStatus {
	return controlplanev1.LastRemediationStatus{
		Machine:    r.Machine,
		Time:       r.Timestamp,
		RetryCount: ptr.To(int32(r.RetryCount)),
	}
}
