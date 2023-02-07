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
	"time"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
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
	healthyMachines := controlPlane.HealthyMachines()
	for _, m := range healthyMachines {
		if conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededCondition) &&
			conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedCondition) &&
			m.DeletionTimestamp.IsZero() {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				errList = append(errList, errors.Wrapf(err, "failed to get PatchHelper for machine %s", m.Name))
				continue
			}

			conditions.Delete(m, clusterv1.MachineOwnerRemediatedCondition)

			if err := patchHelper.Patch(ctx, m, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineOwnerRemediatedCondition,
			}}); err != nil {
				errList = append(errList, errors.Wrapf(err, "failed to patch machine %s", m.Name))
			}
		}
	}
	if len(errList) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errList)
	}

	// Gets all machines that have `MachineHealthCheckSucceeded=False` (indicating a problem was detected on the machine)
	// and `MachineOwnerRemediated` present, indicating that this controller is responsible for performing remediation.
	unhealthyMachines := controlPlane.UnhealthyMachines()

	// If there are no unhealthy machines, return so KCP can proceed with other operations (ctrl.Result nil).
	if len(unhealthyMachines) == 0 {
		return ctrl.Result{}, nil
	}

	// Select the machine to be remediated, which is the oldest machine marked as unhealthy.
	//
	// NOTE: The current solution is considered acceptable for the most frequent use case (only one unhealthy machine),
	// however, in the future this could potentially be improved for the scenario where more than one unhealthy machine exists
	// by considering which machine has lower impact on etcd quorum.
	machineToBeRemediated := unhealthyMachines.Oldest()

	// Returns if the machine is in the process of being deleted.
	if !machineToBeRemediated.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", klog.KObj(machineToBeRemediated), "initialized", controlPlane.KCP.Status.Initialized)

	// Returns if another remediation is in progress but the new Machine is not yet created.
	// Note: This condition is checked after we check for unhealthy Machines and if machineToBeRemediated
	// is being deleted to avoid unnecessary logs if no further remediation should be done.
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
		if err := patchHelper.Patch(ctx, machineToBeRemediated, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.MachineOwnerRemediatedCondition,
		}}); err != nil {
			log.Error(err, "Failed to patch control plane Machine", "Machine", machineToBeRemediated.Name)
			if retErr == nil {
				retErr = errors.Wrapf(err, "failed to patch control plane Machine %s", machineToBeRemediated.Name)
			}
		}
	}()

	// Before starting remediation, run preflight checks in order to verify it is safe to remediate.
	// If any of the following checks fails, we'll surface the reason in the MachineOwnerRemediated condition.

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

	if controlPlane.KCP.Status.Initialized {
		// Executes checks that apply only if the control plane is already initialized; in this case KCP can
		// remediate only if it can safely assume that the operation preserves the operation state of the
		// existing cluster (or at least it doesn't make it worse).

		// The cluster MUST have more than one replica, because this is the smallest cluster size that allows any etcd failure tolerance.
		if controlPlane.Machines.Len() <= 1 {
			log.Info("A control plane machine needs remediation, but the number of current replicas is less or equal to 1. Skipping remediation", "Replicas", controlPlane.Machines.Len())
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate if current replicas are less or equal to 1")
			return ctrl.Result{}, nil
		}

		// The cluster MUST have no machines with a deletion timestamp. This rule prevents KCP taking actions while the cluster is in a transitional state.
		if controlPlane.HasDeletingMachine() {
			log.Info("A control plane machine needs remediation, but there are other control-plane machines being deleted. Skipping remediation")
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP waiting for control plane machine deletion to complete before triggering remediation")
			return ctrl.Result{}, nil
		}

		// Remediation MUST preserve etcd quorum. This rule ensures that KCP will not remove a member that would result in etcd
		// losing a majority of members and thus become unable to field new requests.
		if controlPlane.IsEtcdManaged() {
			canSafelyRemediate, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, machineToBeRemediated)
			if err != nil {
				conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityError, err.Error())
				return ctrl.Result{}, err
			}
			if !canSafelyRemediate {
				log.Info("A control plane machine needs remediation, but removing this machine could result in etcd quorum loss. Skipping remediation")
				conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate this machine because this could result in etcd loosing quorum")
				return ctrl.Result{}, nil
			}
		}

		// Start remediating the unhealthy control plane machine by deleting it.
		// A new machine will come up completing the operation as part of the regular reconcile.

		// If the control plane is initialized, before deleting the machine:
		// - if the machine hosts the etcd leader, forward etcd leadership to another machine.
		// - delete the etcd member hosted on the machine being deleted.
		// - remove the etcd member from the kubeadm config map (only for kubernetes version older than v1.22.0)
		workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster))
		if err != nil {
			log.Error(err, "Failed to create client to workload cluster")
			return ctrl.Result{}, errors.Wrapf(err, "failed to create client to workload cluster")
		}

		// If the machine that is about to be deleted is the etcd leader, move it to the newest member available.
		if controlPlane.IsEtcdManaged() {
			etcdLeaderCandidate := controlPlane.HealthyMachines().Newest()
			if etcdLeaderCandidate == nil {
				log.Info("A control plane machine needs remediation, but there is no healthy machine to forward etcd leadership to")
				conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityWarning,
					"A control plane machine needs remediation, but there is no healthy machine to forward etcd leadership to. Skipping remediation")
				return ctrl.Result{}, nil
			}
			if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToBeRemediated, etcdLeaderCandidate); err != nil {
				log.Error(err, "Failed to move etcd leadership to candidate machine", "candidate", klog.KObj(etcdLeaderCandidate))
				conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityError, err.Error())
				return ctrl.Result{}, err
			}
			if err := workloadCluster.RemoveEtcdMemberForMachine(ctx, machineToBeRemediated); err != nil {
				log.Error(err, "Failed to remove etcd member for machine")
				conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityError, err.Error())
				return ctrl.Result{}, err
			}
		}

		parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.KCP.Spec.Version)
		}

		if err := workloadCluster.RemoveMachineFromKubeadmConfigMap(ctx, machineToBeRemediated, parsedVersion); err != nil {
			log.Error(err, "Failed to remove machine from kubeadm ConfigMap")
			return ctrl.Result{}, err
		}
	}

	// Delete the machine
	if err := r.Client.Delete(ctx, machineToBeRemediated); err != nil {
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete unhealthy machine %s", machineToBeRemediated.Name)
	}

	// Surface the operation is in progress.
	log.Info("Remediating unhealthy machine")
	conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationInProgressReason, clusterv1.ConditionSeverityWarning, "")

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

	return ctrl.Result{Requeue: true}, nil
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

	// Gets MinHealthyPeriod and RetryPeriod from the remediation strategy, or use defaults.
	minHealthyPeriod := controlplanev1.DefaultMinHealthyPeriod
	if controlPlane.KCP.Spec.RemediationStrategy != nil && controlPlane.KCP.Spec.RemediationStrategy.MinHealthyPeriod != nil {
		minHealthyPeriod = controlPlane.KCP.Spec.RemediationStrategy.MinHealthyPeriod.Duration
	}
	retryPeriod := time.Duration(0)
	if controlPlane.KCP.Spec.RemediationStrategy != nil {
		retryPeriod = controlPlane.KCP.Spec.RemediationStrategy.RetryPeriod.Duration
	}

	// Gets the timestamp of the last remediation; if missing, default to a value
	// that ensures both MinHealthyPeriod and RetryPeriod are expired.
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
	// NOTE: If someone/something changes the RemediationForAnnotation on Machines (e.g. changes the Timestamp),
	// this could potentially lead to executing more retries than expected, but this is considered acceptable in such a case.
	var retryForSameMachineInProgress bool
	if lastRemediationTime.Add(minHealthyPeriod).After(reconciliationTime) {
		retryForSameMachineInProgress = true
		log = log.WithValues("RemediationRetryFor", klog.KRef(machineToBeRemediated.Namespace, lastRemediationData.Machine))
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
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate this machine because the operation already failed in the latest %s (RetryPeriod)", retryPeriod)
		return remediationInProgressData, false, nil
	}

	// Check if remediation can happen because of maxRetry is not reached yet, if defined.
	if controlPlane.KCP.Spec.RemediationStrategy != nil && controlPlane.KCP.Spec.RemediationStrategy.MaxRetry != nil {
		maxRetry := int(*controlPlane.KCP.Spec.RemediationStrategy.MaxRetry)
		if remediationInProgressData.RetryCount >= maxRetry {
			log.Info(fmt.Sprintf("A control plane machine needs remediation, but the operation already failed %d times (MaxRetry %d). Skipping remediation", remediationInProgressData.RetryCount, maxRetry))
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate this machine because the operation already failed %d times (MaxRetry)", maxRetry)
			return remediationInProgressData, false, nil
		}
	}

	// All the check passed, increase the remediation retry count.
	remediationInProgressData.RetryCount++

	return remediationInProgressData, true, nil
}

// max calculates the maximum duration.
func max(x, y time.Duration) time.Duration {
	if x < y {
		return y
	}
	return x
}

// canSafelyRemoveEtcdMember assess if it is possible to remove the member hosted on the machine to be remediated
// without loosing etcd quorum.
//
// The answer mostly depend on the existence of other failing members on top of the one being deleted, and according
// to the etcd fault tolerance specification (see https://etcd.io/docs/v3.3/faq/#what-is-failure-tolerance):
//   - 3 CP cluster does not tolerate additional failing members on top of the one being deleted (the target
//     cluster size after deletion is 2, fault tolerance 0)
//   - 5 CP cluster tolerates 1 additional failing members on top of the one being deleted (the target
//     cluster size after deletion is 4, fault tolerance 1)
//   - 7 CP cluster tolerates 2 additional failing members on top of the one being deleted (the target
//     cluster size after deletion is 6, fault tolerance 2)
//   - etc.
//
// NOTE: this func assumes the list of members in sync with the list of machines/nodes, it is required to call reconcileEtcdMembers
// as well as reconcileControlPlaneConditions before this.
func (r *KubeadmControlPlaneReconciler) canSafelyRemoveEtcdMember(ctx context.Context, controlPlane *internal.ControlPlane, machineToBeRemediated *clusterv1.Machine) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, client.ObjectKey{
		Namespace: controlPlane.Cluster.Namespace,
		Name:      controlPlane.Cluster.Name,
	})
	if err != nil {
		return false, errors.Wrapf(err, "failed to get client for workload cluster %s", controlPlane.Cluster.Name)
	}

	// Gets the etcd status

	// This makes it possible to have a set of etcd members status different from the MHC unhealthy/unhealthy conditions.
	etcdMembers, err := workloadCluster.EtcdMembers(ctx)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get etcdStatus for workload cluster %s", controlPlane.Cluster.Name)
	}

	currentTotalMembers := len(etcdMembers)

	log.Info("etcd cluster before remediation",
		"currentTotalMembers", currentTotalMembers,
		"currentMembers", etcdMembers)

	// Projects the target etcd cluster after remediation, considering all the etcd members except the one being remediated.
	targetTotalMembers := 0
	targetUnhealthyMembers := 0

	healthyMembers := []string{}
	unhealthyMembers := []string{}
	for _, etcdMember := range etcdMembers {
		// Skip the machine to be deleted because it won't be part of the target etcd cluster.
		if machineToBeRemediated.Status.NodeRef != nil && machineToBeRemediated.Status.NodeRef.Name == etcdMember {
			continue
		}

		// Include the member in the target etcd cluster.
		targetTotalMembers++

		// Search for the machine corresponding to the etcd member.
		var machine *clusterv1.Machine
		for _, m := range controlPlane.Machines {
			if m.Status.NodeRef != nil && m.Status.NodeRef.Name == etcdMember {
				machine = m
				break
			}
		}

		// If an etcd member does not have a corresponding machine it is not possible to retrieve etcd member health,
		// so KCP is assuming the worst scenario and considering the member unhealthy.
		//
		// NOTE: This should not happen given that KCP is running reconcileEtcdMembers before calling this method.
		if machine == nil {
			log.Info("An etcd member does not have a corresponding machine, assuming this member is unhealthy", "MemberName", etcdMember)
			targetUnhealthyMembers++
			unhealthyMembers = append(unhealthyMembers, fmt.Sprintf("%s (no machine)", etcdMember))
			continue
		}

		// Check member health as reported by machine's health conditions
		if !conditions.IsTrue(machine, controlplanev1.MachineEtcdMemberHealthyCondition) {
			targetUnhealthyMembers++
			unhealthyMembers = append(unhealthyMembers, fmt.Sprintf("%s (%s)", etcdMember, machine.Name))
			continue
		}

		healthyMembers = append(healthyMembers, fmt.Sprintf("%s (%s)", etcdMember, machine.Name))
	}

	// See https://etcd.io/docs/v3.3/faq/#what-is-failure-tolerance for fault tolerance formula explanation.
	targetQuorum := (targetTotalMembers / 2.0) + 1
	canSafelyRemediate := targetTotalMembers-targetUnhealthyMembers >= targetQuorum

	log.Info(fmt.Sprintf("etcd cluster projected after remediation of %s", machineToBeRemediated.Name),
		"healthyMembers", healthyMembers,
		"unhealthyMembers", unhealthyMembers,
		"targetTotalMembers", targetTotalMembers,
		"targetQuorum", targetQuorum,
		"targetUnhealthyMembers", targetUnhealthyMembers,
		"canSafelyRemediate", canSafelyRemediate)

	return canSafelyRemediate, nil
}

// RemediationData struct is used to keep track of information stored in the RemediationInProgressAnnotation in KCP
// during remediation and then into the RemediationForAnnotation on the replacement machine once it is created.
type RemediationData struct {
	// Machine is the machine name of the latest machine being remediated.
	Machine string `json:"machine"`

	// Timestamp is when last remediation happened. It is represented in RFC3339 form and is in UTC.
	Timestamp metav1.Time `json:"timestamp"`

	// RetryCount used to keep track of remediation retry for the last remediated machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	RetryCount int `json:"retryCount"`
}

// RemediationDataFromAnnotation gets RemediationData from an annotation value.
func RemediationDataFromAnnotation(value string) (*RemediationData, error) {
	ret := &RemediationData{}
	if err := json.Unmarshal([]byte(value), ret); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal value %s for %s annotation", value, clusterv1.RemediationInProgressReason)
	}
	return ret, nil
}

// Marshal an RemediationData into an annotation value.
func (r *RemediationData) Marshal() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", errors.Wrapf(err, "failed to marshal value for %s annotation", clusterv1.RemediationInProgressReason)
	}
	return string(b), nil
}

// ToStatus converts a RemediationData into a LastRemediationStatus struct.
func (r *RemediationData) ToStatus() *controlplanev1.LastRemediationStatus {
	return &controlplanev1.LastRemediationStatus{
		Machine:    r.Machine,
		Timestamp:  r.Timestamp,
		RetryCount: int32(r.RetryCount),
	}
}
