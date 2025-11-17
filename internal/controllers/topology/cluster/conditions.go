/*
Copyright 2021 The Kubernetes Authors.

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
	"fmt"
	"sort"
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/hooks"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

func (r *Reconciler) reconcileConditions(s *scope.Scope, cluster *clusterv1.Cluster, reconcileErr error) error {
	return r.reconcileTopologyReconciledCondition(s, cluster, reconcileErr)
}

// reconcileTopologyReconciledCondition sets the TopologyReconciled condition on the cluster.
// The TopologyReconciled condition is considered true if spec of all the objects associated with the
// cluster are in sync with the topology defined in the cluster.
// The condition is false under the following conditions:
// - The cluster is paused.
// - An error occurred during the reconcile process of the cluster topology.
// - The ClusterClass has not been successfully reconciled with its current spec.
// - The cluster upgrade has not yet propagated to all the components of the cluster.
//   - For a managed topology cluster the version upgrade is propagated one component at a time.
//     In such a case, since some of the component's spec would be adrift from the topology the
//     topology cannot be considered fully reconciled.
func (r *Reconciler) reconcileTopologyReconciledCondition(s *scope.Scope, cluster *clusterv1.Cluster, reconcileErr error) error {
	// Mark TopologyReconciled as false if the Cluster is paused.
	if ptr.Deref(cluster.Spec.Paused, false) || annotations.HasPaused(cluster) {
		var messages []string
		if ptr.Deref(cluster.Spec.Paused, false) {
			messages = append(messages, "Cluster spec.paused is set to true")
		}
		if annotations.HasPaused(cluster) {
			messages = append(messages, "Cluster has the cluster.x-k8s.io/paused annotation")
		}
		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				clusterv1.TopologyReconciledPausedV1Beta1Reason,
				clusterv1.ConditionSeverityInfo,
				"%s", strings.Join(messages, ", "),
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterTopologyReconciledCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterTopologyReconcilePausedReason,
			Message: strings.Join(messages, ", "),
		})
		return nil
	}

	// If an error occurred during reconciliation set the TopologyReconciled condition to false.
	// Add the error message from the reconcile function to the message of the condition.
	if reconcileErr != nil {
		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				clusterv1.TopologyReconcileFailedV1Beta1Reason,
				clusterv1.ConditionSeverityError,
				// TODO: Add a protection for messages continuously changing leading to Cluster object changes/reconcile.
				"%s", reconcileErr.Error(),
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterTopologyReconciledCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterTopologyReconciledFailedReason,
			// TODO: Add a protection for messages continuously changing leading to Cluster object changes/reconcile.
			Message: reconcileErr.Error(),
		})
		return nil
	}

	// Mark TopologyReconciled as false due to cluster deletion.
	if !cluster.DeletionTimestamp.IsZero() {
		message := "Cluster is deleting"
		if s.HookResponseTracker.AggregateRetryAfter() != 0 {
			message += ". " + s.HookResponseTracker.AggregateMessage("delete")
		}
		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				clusterv1.DeletingV1Beta1Reason,
				clusterv1.ConditionSeverityInfo,
				"%s", message,
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterTopologyReconciledCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterTopologyReconciledDeletingReason,
			Message: message,
		})
		return nil
	}

	// If the ClusterClass `metadata.Generation` doesn't match the `status.ObservedGeneration` requeue as the ClusterClass
	// is not up to date.
	if s.Blueprint != nil && s.Blueprint.ClusterClass != nil &&
		s.Blueprint.ClusterClass.GetGeneration() != s.Blueprint.ClusterClass.Status.ObservedGeneration {
		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				clusterv1.TopologyReconciledClusterClassNotReconciledV1Beta1Reason,
				clusterv1.ConditionSeverityInfo,
				"ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if"+
					".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterTopologyReconciledCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterTopologyReconciledClusterClassNotReconciledReason,
			Message: "ClusterClass not reconciled. If this condition persists please check ClusterClass status. A ClusterClass is reconciled if" +
				".status.observedGeneration == .metadata.generation is true. If this is not the case either ClusterClass reconciliation failed or the ClusterClass is paused",
		})
		return nil
	}

	// If the BeforeClusterCreate hook is blocking, reports it
	if !s.Current.Cluster.Spec.InfrastructureRef.IsDefined() && !s.Current.Cluster.Spec.ControlPlaneRef.IsDefined() {
		if s.HookResponseTracker.AggregateRetryAfter() != 0 {
			v1beta1conditions.Set(cluster,
				v1beta1conditions.FalseCondition(
					clusterv1.TopologyReconciledV1Beta1Condition,
					clusterv1.TopologyReconciledClusterCreatingV1Beta1Reason,
					clusterv1.ConditionSeverityInfo,
					"%s", s.HookResponseTracker.AggregateMessage("Cluster topology creation"),
				),
			)
			conditions.Set(cluster, metav1.Condition{
				Type:    clusterv1.ClusterTopologyReconciledCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.ClusterTopologyReconciledClusterCreatingReason,
				Message: s.HookResponseTracker.AggregateMessage("Cluster topology creation"),
			})
			return nil
		}

		// Note: this should never happen, controlPlane and infrastructure ref should be set at the first reconcile of the topology controller if the hook is not blocking.
		v1beta1conditions.Set(cluster,
			v1beta1conditions.TrueCondition(clusterv1.TopologyReconciledV1Beta1Condition),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterTopologyReconciledCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.ClusterTopologyReconcileSucceededReason,
		})
		return nil
	}

	// If Cluster is updating surface it.
	// Note: intentionally checking all the signal about upgrade in progress to make sure to avoid edge cases.
	if s.UpgradeTracker.ControlPlane.IsPendingUpgrade ||
		s.UpgradeTracker.ControlPlane.IsStartingUpgrade ||
		s.UpgradeTracker.ControlPlane.IsUpgrading ||
		s.UpgradeTracker.MachineDeployments.IsAnyPendingUpgrade() ||
		s.UpgradeTracker.MachineDeployments.IsAnyUpgrading() ||
		s.UpgradeTracker.MachineDeployments.IsAnyUpgradeDeferred() ||
		s.UpgradeTracker.MachineDeployments.IsAnyPendingCreate() ||
		s.UpgradeTracker.MachinePools.IsAnyPendingUpgrade() ||
		s.UpgradeTracker.MachinePools.IsAnyUpgrading() ||
		s.UpgradeTracker.MachinePools.IsAnyUpgradeDeferred() ||
		s.UpgradeTracker.MachinePools.IsAnyPendingCreate() ||
		hooks.IsPending(runtimehooksv1.AfterClusterUpgrade, cluster) {
		// Start building the condition message showing upgrade progress.
		msgBuilder := &strings.Builder{}
		fmt.Fprintf(msgBuilder, "Cluster is upgrading to %s", cluster.Spec.Topology.Version)

		// Setting condition reasons showing upgrade is progress; this will be overridden only
		// when users are blocking upgrade to make further progress, e.g. deferred upgrades
		reason := clusterv1.ClusterTopologyReconciledClusterUpgradingReason
		v1Beta1Reason := clusterv1.TopologyReconciledClusterUpgradingV1Beta1Reason

		cpVersion, err := contract.ControlPlane().Version().Get(s.Desired.ControlPlane.Object)
		if err != nil {
			return errors.Wrap(err, "failed to get control plane spec version")
		}

		// If any of the lifecycle hooks are blocking the upgrade surface it as a first detail.
		if s.HookResponseTracker.IsAnyBlocking() {
			fmt.Fprintf(msgBuilder, "\n  * %s", s.HookResponseTracker.AggregateMessage("upgrade"))
		}

		// If control plane is upgrading surface it, otherwise surface the pending upgrade plan.
		if s.UpgradeTracker.ControlPlane.IsStartingUpgrade || s.UpgradeTracker.ControlPlane.IsUpgrading {
			fmt.Fprintf(msgBuilder, "\n  * %s upgrading to version %s%s", s.Current.ControlPlane.Object.GetKind(), *cpVersion, pendingVersions(s.UpgradeTracker.ControlPlane.UpgradePlan, *cpVersion))
		} else if len(s.UpgradeTracker.ControlPlane.UpgradePlan) > 0 {
			fmt.Fprintf(msgBuilder, "\n  * %s pending upgrade to version %s", s.Current.ControlPlane.Object.GetKind(), strings.Join(s.UpgradeTracker.ControlPlane.UpgradePlan, ", "))
		}

		// If MachineDeployments are upgrading surface it, if MachineDeployments are pending upgrades then surface the upgrade plans.
		upgradingMachineDeploymentNames, pendingMachineDeploymentNames, deferredMachineDeploymentNames := dedupNames(s.UpgradeTracker.MachineDeployments)
		if len(upgradingMachineDeploymentNames) > 0 {
			fmt.Fprintf(msgBuilder, "\n  * %s upgrading to version %s%s", nameList("MachineDeployment", "MachineDeployments", upgradingMachineDeploymentNames), *cpVersion, pendingVersions(s.UpgradeTracker.MachineDeployments.UpgradePlan, *cpVersion))
		}

		if len(pendingMachineDeploymentNames) > 0 && len(s.UpgradeTracker.MachineDeployments.UpgradePlan) > 0 {
			fmt.Fprintf(msgBuilder, "\n  * %s pending upgrade to version %s", nameList("MachineDeployment", "MachineDeployments", pendingMachineDeploymentNames), strings.Join(s.UpgradeTracker.MachineDeployments.UpgradePlan, ", "))
		}

		// If MachineDeployments has been deferred or put on hold, surface it.
		if len(deferredMachineDeploymentNames) > 0 {
			fmt.Fprintf(msgBuilder, "\n  * %s upgrade to version %s deferred using defer-upgrade or hold-upgrade-sequence annotations", nameList("MachineDeployment", "MachineDeployments", deferredMachineDeploymentNames), *cpVersion)
			// If Deferred upgrades are blocking an upgrade, surface it.
			// Note: Hook blocking takes the precedence on this signal.
			if !s.HookResponseTracker.IsAnyBlocking() &&
				(!s.UpgradeTracker.ControlPlane.IsStartingUpgrade && !s.UpgradeTracker.ControlPlane.IsUpgrading) &&
				!s.UpgradeTracker.MachineDeployments.IsAnyUpgrading() && len(pendingMachineDeploymentNames) == 0 {
				reason = clusterv1.ClusterTopologyReconciledMachineDeploymentsUpgradeDeferredReason
				v1Beta1Reason = clusterv1.TopologyReconciledMachineDeploymentsUpgradeDeferredV1Beta1Reason
			}
		}

		// If creation of MachineDeployments has been deferred due to control plane upgrade in progress, surface it.
		if s.UpgradeTracker.MachineDeployments.IsAnyPendingCreate() {
			fmt.Fprintf(msgBuilder, "\n  * %s creation deferred while control plane upgrade is in progress", nameList("MachineDeployment", "MachineDeployments", s.UpgradeTracker.MachineDeployments.PendingCreateTopologyNames()))
		}

		// If MachinePools are upgrading surface it, if MachinePools are pending upgrades then surface the upgrade plans.
		upgradingMachinePoolNames, pendingMachinePoolNames, deferredMachinePoolNames := dedupNames(s.UpgradeTracker.MachinePools)
		if len(upgradingMachinePoolNames) > 0 {
			fmt.Fprintf(msgBuilder, "\n  * %s upgrading to version %s%s", nameList("MachinePool", "MachinePools", upgradingMachinePoolNames), *cpVersion, pendingVersions(s.UpgradeTracker.MachinePools.UpgradePlan, *cpVersion))
		}

		if len(pendingMachinePoolNames) > 0 && len(s.UpgradeTracker.MachinePools.UpgradePlan) > 0 {
			fmt.Fprintf(msgBuilder, "\n  * %s pending upgrade to version %s", nameList("MachinePool", "MachinePools", pendingMachinePoolNames), strings.Join(s.UpgradeTracker.MachinePools.UpgradePlan, ", "))
		}

		// If MachinePools has been deferred or put on hold, surface it.
		if len(deferredMachinePoolNames) > 0 {
			fmt.Fprintf(msgBuilder, "\n  * %s upgrade to version %s deferred using topology.cluster.x-k8s.io/defer-upgrade or hold-upgrade-sequence annotations", nameList("MachinePool", "MachinePools", deferredMachinePoolNames), *cpVersion)
			// If Deferred upgrades are blocking an upgrade, surface it.
			// Note: Hook blocking takes the precedence on this signal.
			if !s.HookResponseTracker.IsAnyBlocking() &&
				(!s.UpgradeTracker.ControlPlane.IsStartingUpgrade && !s.UpgradeTracker.ControlPlane.IsUpgrading) &&
				!s.UpgradeTracker.MachinePools.IsAnyUpgrading() && len(pendingMachinePoolNames) == 0 &&
				reason != clusterv1.ClusterTopologyReconciledMachineDeploymentsUpgradeDeferredReason {
				reason = clusterv1.ClusterTopologyReconciledMachinePoolsUpgradeDeferredReason
				v1Beta1Reason = clusterv1.TopologyReconciledMachinePoolsUpgradeDeferredV1Beta1Reason
			}
		}

		// If creation of MachinePools has been deferred due to control plane upgrade in progress, surface it.
		if s.UpgradeTracker.MachinePools.IsAnyPendingCreate() {
			fmt.Fprintf(msgBuilder, "\n  * %s creation deferred while control plane upgrade is in progress", nameList("MachinePool", "MachinePools", s.UpgradeTracker.MachinePools.PendingCreateTopologyNames()))
		}

		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				v1Beta1Reason,
				clusterv1.ConditionSeverityInfo,
				"%s", msgBuilder.String(),
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterTopologyReconciledCondition,
			Status:  metav1.ConditionFalse,
			Reason:  reason,
			Message: msgBuilder.String(),
		})
		return nil
	}

	// If there are no errors while reconciling and if the topology is not holding out changes
	// we can consider that spec of all the objects is reconciled to match the topology. Set the
	// TopologyReconciled condition to true.
	v1beta1conditions.Set(cluster,
		v1beta1conditions.TrueCondition(clusterv1.TopologyReconciledV1Beta1Condition),
	)
	conditions.Set(cluster, metav1.Condition{
		Type:   clusterv1.ClusterTopologyReconciledCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterTopologyReconcileSucceededReason,
	})
	return nil
}

// pendingVersion return a message with pending version in the upgrad plan.
func pendingVersions(plan []string, version string) string {
	// clone the plan to avoid side effects on the original object.
	planWithoutVersion := []string{}
	for _, v := range plan {
		if v != version {
			planWithoutVersion = append(planWithoutVersion, v)
		}
	}
	if len(planWithoutVersion) > 0 {
		return fmt.Sprintf(" (%s pending)", strings.Join(planWithoutVersion, ", "))
	}
	return ""
}

// dedupNames take care of names that might exist in multiple lists.
func dedupNames(t scope.WorkerUpgradeTracker) ([]string, []string, []string) {
	// upgrading names are preserved
	upgradingSet := sets.Set[string]{}.Insert(t.UpgradingNames()...)
	// upgrading names are removed from deferred names (give precedence to the fact that it is upgrading now)
	deferredSet := sets.Set[string]{}.Insert(t.DeferredUpgradeNames()...).Difference(upgradingSet)
	// upgrading and deferred names are removed from pending names (it is pending if not upgrading or deferred)
	pendingSet := sets.Set[string]{}.Insert(t.PendingUpgradeNames()...).Difference(upgradingSet).Difference(deferredSet)
	return upgradingSet.UnsortedList(), pendingSet.UnsortedList(), deferredSet.UnsortedList()
}

// computeNameList computes list of names from the given list to be shown in conditions.
// It shortens the list to at most 5 names and adds an ellipsis at the end if the list
// has more than 3 elements.
func computeNameList(list []string) any {
	if len(list) > 3 {
		list = append(list[:3], "...")
	}

	return strings.Join(list, ", ")
}

// nameList computes list of names from the given list to be shown in conditions.
// It shortens the list to at most 5 names and adds an ellipsis at the end if the list
// has more than 3 elements.
func nameList(kind, kindPlural string, names []string) any {
	sort.Strings(names)
	switch {
	case len(names) == 1:
		return fmt.Sprintf("%s %s", kind, names[0])
	case len(names) <= 3:
		return fmt.Sprintf("%s %s", kindPlural, strings.Join(names, ", "))
	default:
		return fmt.Sprintf("%s %s, ... (%d more)", kindPlural, strings.Join(names[:3], ", "), len(names)-3)
	}
}
