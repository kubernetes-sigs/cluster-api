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

	"github.com/blang/semver"
	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// reconcileUnhealthyMachines tries to remediate KubeadmControlPlane unhealthy machines
// based on the process described in https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191017-kubeadm-based-control-plane.md#remediation-using-delete-and-recreate
func (r *KubeadmControlPlaneReconciler) reconcileUnhealthyMachines(ctx context.Context, controlPlane *internal.ControlPlane) (ret ctrl.Result, retErr error) {
	logger := r.Log.WithValues("namespace", controlPlane.KCP.Namespace, "kubeadmControlPlane", controlPlane.KCP.Name, "cluster", controlPlane.Cluster.Name)

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

	patchHelper, err := patch.NewHelper(machineToBeRemediated, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to Patch the Machine conditions after each reconcileUnhealthyMachines.
		if err := patchHelper.Patch(ctx, machineToBeRemediated, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.MachineOwnerRemediatedCondition,
		}}); err != nil {
			logger.Error(err, "Failed to patch control plane Machine", "machine", machineToBeRemediated.Name)
			if retErr == nil {
				retErr = errors.Wrapf(err, "failed to patch control plane Machine %s", machineToBeRemediated.Name)
			}
		}
	}()

	// Before starting remediation, run preflight checks in order to verify it is safe to remediate.
	// If any of the following checks fails, we'll surface the reason in the MachineOwnerRemediated condition.

	desiredReplicas := int(*controlPlane.KCP.Spec.Replicas)

	// The cluster MUST have more than one replica, because this is the smallest cluster size that allows any etcd failure tolerance.
	if controlPlane.Machines.Len() <= 1 {
		logger.Info("A control plane machine needs remediation, but the number of current replicas is less or equal to 1. Skipping remediation", "UnhealthyMachine", machineToBeRemediated.Name, "Replicas", controlPlane.Machines.Len())
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate if current replicas are less or equal then 1")
		return ctrl.Result{}, nil
	}

	// The number of replicas MUST be equal to or greater than the desired replicas. This rule ensures that when the cluster
	// is missing replicas, we skip remediation and instead perform regular scale up/rollout operations first.
	if controlPlane.Machines.Len() < desiredReplicas {
		logger.Info("A control plane machine needs remediation, but the current number of replicas is lower that expected. Skipping remediation", "UnhealthyMachine", machineToBeRemediated.Name, "Replicas", desiredReplicas, "CurrentReplicas", controlPlane.Machines.Len())
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP waiting for having at least %d control plane machines before triggering remediation", desiredReplicas)
		return ctrl.Result{}, nil
	}

	// The cluster MUST have no machines with a deletion timestamp. This rule prevents KCP taking actions while the cluster is in a transitional state.
	if controlPlane.HasDeletingMachine() {
		logger.Info("A control plane machine needs remediation, but there are other control-plane machines being deleted. Skipping remediation", "UnhealthyMachine", machineToBeRemediated.Name)
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP waiting for control plane machine deletion to complete before triggering remediation")
		return ctrl.Result{}, nil
	}

	// Remediation MUST preserve etcd quorum. This rule ensures that we will not remove a member that would result in etcd
	// losing a majority of members and thus become unable to field new requests.
	if controlPlane.IsEtcdManaged() {
		canSafelyRemediate, err := r.canSafelyRemoveEtcdMember(ctx, controlPlane, machineToBeRemediated)
		if err != nil {
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityError, err.Error())
			return ctrl.Result{}, err
		}
		if !canSafelyRemediate {
			logger.Info("A control plane machine needs remediation, but removing this machine could result in etcd quorum loss. Skipping remediation", "UnhealthyMachine", machineToBeRemediated.Name)
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate this machine because this could result in etcd loosing quorum")
			return ctrl.Result{}, nil
		}
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster))
	if err != nil {
		logger.Error(err, "Failed to create client to workload cluster")
		return ctrl.Result{}, errors.Wrapf(err, "failed to create client to workload cluster")
	}

	// If the machine that is about to be deleted is the etcd leader, move it to the newest member available.
	if controlPlane.IsEtcdManaged() {
		etcdLeaderCandidate := controlPlane.HealthyMachines().Newest()
		if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToBeRemediated, etcdLeaderCandidate); err != nil {
			logger.Error(err, "Failed to move leadership to candidate machine", "candidate", etcdLeaderCandidate.Name)
			return ctrl.Result{}, err
		}
		if err := workloadCluster.RemoveEtcdMemberForMachine(ctx, machineToBeRemediated); err != nil {
			logger.Error(err, "Failed to remove etcd member for machine")
			return ctrl.Result{}, err
		}
	}

	kubernetesVersion := controlPlane.KCP.Spec.Version
	parsedVersion, err := semver.ParseTolerant(kubernetesVersion)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kubernetesVersion)
	}
	if err := workloadCluster.RemoveMachineFromKubeadmConfigMap(ctx, machineToBeRemediated, parsedVersion); err != nil {
		logger.Error(err, "Failed to remove machine from kubeadm ConfigMap")
		return ctrl.Result{}, err
	}

	if err := r.Client.Delete(ctx, machineToBeRemediated); err != nil {
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete unhealthy machine %s", machineToBeRemediated.Name)
	}

	logger.Info("Remediating unhealthy machine", "UnhealthyMachine", machineToBeRemediated.Name)
	conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationInProgressReason, clusterv1.ConditionSeverityWarning, "")
	return ctrl.Result{Requeue: true}, nil
}

// canSafelyRemoveEtcdMember assess if it is possible to remove the member hosted on the machine to be remediated
// without loosing etcd quorum.
//
// The answer mostly depend on the existence of other failing members on top of the one being deleted, and according
// to the etcd fault tolerance specification (see https://etcd.io/docs/v3.3/faq/#what-is-failure-tolerance):
// - 3 CP cluster does not tolerate additional failing members on top of the one being deleted (the target
//   cluster size after deletion is 2, fault tolerance 0)
// - 5 CP cluster tolerates 1 additional failing members on top of the one being deleted (the target
//   cluster size after deletion is 4, fault tolerance 1)
// - 7 CP cluster tolerates 2 additional failing members on top of the one being deleted (the target
//   cluster size after deletion is 6, fault tolerance 2)
// - etc.
//
// NOTE: this func assumes the list of members in sync with the list of machines/nodes, it is required to call reconcileEtcdMembers
// and well as reconcileControlPlaneConditions before this.
func (r *KubeadmControlPlaneReconciler) canSafelyRemoveEtcdMember(ctx context.Context, controlPlane *internal.ControlPlane, machineToBeRemediated *clusterv1.Machine) (bool, error) {
	logger := r.Log.WithValues("namespace", controlPlane.KCP.Namespace, "kubeadmControlPlane", controlPlane.KCP.Name, "cluster", controlPlane.Cluster.Name)

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

	logger.Info("etcd cluster before remediation",
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

		// If an etcd member does not have a corresponding machine, it is not possible to retrieve etcd member health
		// so we are assuming the worst scenario and considering the member unhealthy.
		//
		// NOTE: This should not happen given that we are running reconcileEtcdMembers before calling this method.
		if machine == nil {
			logger.Info("An etcd member does not have a corresponding machine, assuming this member is unhealthy", "MemberName", etcdMember)
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

	logger.Info(fmt.Sprintf("etcd cluster projected after remediation of %s", machineToBeRemediated.Name),
		"healthyMembers", healthyMembers,
		"unhealthyMembers", unhealthyMembers,
		"targetTotalMembers", targetTotalMembers,
		"targetQuorum", targetQuorum,
		"targetUnhealthyMembers", targetUnhealthyMembers,
		"canSafelyRemediate", canSafelyRemediate)

	return canSafelyRemediate, nil
}
