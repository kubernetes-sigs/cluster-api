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
	"strings"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/internal/contract"
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

	// Mark TopologyReconciled as false due to cluster deletion.
	if !cluster.DeletionTimestamp.IsZero() {
		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				clusterv1.DeletedV1Beta1Reason,
				clusterv1.ConditionSeverityInfo,
				"",
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterTopologyReconciledCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.ClusterTopologyReconciledDeletingReason,
			Message: "Cluster is deleting",
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

	// If any of the lifecycle hooks are blocking any part of the reconciliation then topology
	// is not considered as fully reconciled.
	if s.HookResponseTracker.AggregateRetryAfter() != 0 {
		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				clusterv1.TopologyReconciledHookBlockingV1Beta1Reason,
				clusterv1.ConditionSeverityInfo,
				// TODO: Add a protection for messages continuously changing leading to Cluster object changes/reconcile.
				"%s", s.HookResponseTracker.AggregateMessage(),
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:   clusterv1.ClusterTopologyReconciledCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.ClusterTopologyReconciledHookBlockingReason,
			// TODO: Add a protection for messages continuously changing leading to Cluster object changes/reconcile.
			Message: s.HookResponseTracker.AggregateMessage(),
		})
		return nil
	}

	// The topology is not considered as fully reconciled if one of the following is true:
	// * either the Control Plane or any of the MachineDeployments/MachinePools are still pending to pick up the new version
	//  (generally happens when upgrading the cluster)
	// * when there are MachineDeployments/MachinePools for which the upgrade has been deferred
	// * when new MachineDeployments/MachinePools are pending to be created
	//  (generally happens when upgrading the cluster)
	if s.UpgradeTracker.ControlPlane.IsPendingUpgrade ||
		s.UpgradeTracker.MachineDeployments.IsAnyPendingCreate() ||
		s.UpgradeTracker.MachineDeployments.IsAnyPendingUpgrade() ||
		s.UpgradeTracker.MachineDeployments.DeferredUpgrade() ||
		s.UpgradeTracker.MachinePools.IsAnyPendingCreate() ||
		s.UpgradeTracker.MachinePools.IsAnyPendingUpgrade() ||
		s.UpgradeTracker.MachinePools.DeferredUpgrade() {
		msgBuilder := &strings.Builder{}
		var reason string
		var v1beta2Reason string

		// TODO(ykakarap): Evaluate potential improvements to building the condition. Multiple causes can trigger the
		// condition to be false at the same time (Example: ControlPlane.IsPendingUpgrade and MachineDeployments.IsAnyPendingCreate can
		// occur at the same time). Find better wording and `Reason` for the condition so that the condition can be rich
		// with all the relevant information.
		switch {
		case s.UpgradeTracker.ControlPlane.IsPendingUpgrade:
			fmt.Fprintf(msgBuilder, "Control plane rollout and upgrade to version %s on hold.", s.Blueprint.Topology.Version)
			reason = clusterv1.TopologyReconciledControlPlaneUpgradePendingV1Beta1Reason
			v1beta2Reason = clusterv1.ClusterTopologyReconciledControlPlaneUpgradePendingReason
		case s.UpgradeTracker.MachineDeployments.IsAnyPendingUpgrade():
			fmt.Fprintf(msgBuilder, "MachineDeployment(s) %s rollout and upgrade to version %s on hold.",
				computeNameList(s.UpgradeTracker.MachineDeployments.PendingUpgradeNames()),
				s.Blueprint.Topology.Version,
			)
			reason = clusterv1.TopologyReconciledMachineDeploymentsUpgradePendingV1Beta1Reason
			v1beta2Reason = clusterv1.ClusterTopologyReconciledMachineDeploymentsUpgradePendingReason
		case s.UpgradeTracker.MachineDeployments.IsAnyPendingCreate():
			fmt.Fprintf(msgBuilder, "MachineDeployment(s) for Topologies %s creation on hold.",
				computeNameList(s.UpgradeTracker.MachineDeployments.PendingCreateTopologyNames()),
			)
			reason = clusterv1.TopologyReconciledMachineDeploymentsCreatePendingV1Beta1Reason
			v1beta2Reason = clusterv1.ClusterTopologyReconciledMachineDeploymentsCreatePendingReason
		case s.UpgradeTracker.MachineDeployments.DeferredUpgrade():
			fmt.Fprintf(msgBuilder, "MachineDeployment(s) %s rollout and upgrade to version %s deferred.",
				computeNameList(s.UpgradeTracker.MachineDeployments.DeferredUpgradeNames()),
				s.Blueprint.Topology.Version,
			)
			reason = clusterv1.TopologyReconciledMachineDeploymentsUpgradeDeferredV1Beta1Reason
			v1beta2Reason = clusterv1.ClusterTopologyReconciledMachineDeploymentsUpgradeDeferredReason
		case s.UpgradeTracker.MachinePools.IsAnyPendingUpgrade():
			fmt.Fprintf(msgBuilder, "MachinePool(s) %s rollout and upgrade to version %s on hold.",
				computeNameList(s.UpgradeTracker.MachinePools.PendingUpgradeNames()),
				s.Blueprint.Topology.Version,
			)
			reason = clusterv1.TopologyReconciledMachinePoolsUpgradePendingV1Beta1Reason
			v1beta2Reason = clusterv1.ClusterTopologyReconciledMachinePoolsUpgradePendingReason
		case s.UpgradeTracker.MachinePools.IsAnyPendingCreate():
			fmt.Fprintf(msgBuilder, "MachinePool(s) for Topologies %s creation on hold.",
				computeNameList(s.UpgradeTracker.MachinePools.PendingCreateTopologyNames()),
			)
			reason = clusterv1.TopologyReconciledMachinePoolsCreatePendingV1Beta1Reason
			v1beta2Reason = clusterv1.ClusterTopologyReconciledMachinePoolsCreatePendingReason
		case s.UpgradeTracker.MachinePools.DeferredUpgrade():
			fmt.Fprintf(msgBuilder, "MachinePool(s) %s rollout and upgrade to version %s deferred.",
				computeNameList(s.UpgradeTracker.MachinePools.DeferredUpgradeNames()),
				s.Blueprint.Topology.Version,
			)
			reason = clusterv1.TopologyReconciledMachinePoolsUpgradeDeferredV1Beta1Reason
			v1beta2Reason = clusterv1.ClusterTopologyReconciledMachinePoolsUpgradeDeferredReason
		}

		switch {
		case s.UpgradeTracker.ControlPlane.IsProvisioning:
			msgBuilder.WriteString(" Control plane is completing initial provisioning")

		case s.UpgradeTracker.ControlPlane.IsUpgrading:
			cpVersion, err := contract.ControlPlane().Version().Get(s.Current.ControlPlane.Object)
			if err != nil {
				return errors.Wrap(err, "failed to get control plane spec version")
			}
			fmt.Fprintf(msgBuilder, " Control plane is upgrading to version %s", *cpVersion)

		case len(s.UpgradeTracker.MachineDeployments.UpgradingNames()) > 0:
			fmt.Fprintf(msgBuilder, " MachineDeployment(s) %s are upgrading",
				computeNameList(s.UpgradeTracker.MachineDeployments.UpgradingNames()),
			)

		case len(s.UpgradeTracker.MachinePools.UpgradingNames()) > 0:
			fmt.Fprintf(msgBuilder, " MachinePool(s) %s are upgrading",
				computeNameList(s.UpgradeTracker.MachinePools.UpgradingNames()),
			)
		}

		v1beta1conditions.Set(cluster,
			v1beta1conditions.FalseCondition(
				clusterv1.TopologyReconciledV1Beta1Condition,
				reason,
				clusterv1.ConditionSeverityInfo,
				"%s", msgBuilder.String(),
			),
		)
		conditions.Set(cluster, metav1.Condition{
			Type:    clusterv1.ClusterTopologyReconciledCondition,
			Status:  metav1.ConditionFalse,
			Reason:  v1beta2Reason,
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

// computeNameList computes list of names from the given list to be shown in conditions.
// It shortens the list to at most 5 names and adds an ellipsis at the end if the list
// has more than 5 elements.
func computeNameList(list []string) any {
	if len(list) > 5 {
		list = append(list[:5], "...")
	}

	return strings.Join(list, ", ")
}
