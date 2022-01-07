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

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func (r *Reconciler) reconcileConditions(s *scope.Scope, cluster *clusterv1.Cluster, reconcileErr error) error {
	return r.reconcileTopologyReconciledCondition(s, cluster, reconcileErr)
}

// reconcileTopologyReconciledCondition sets the TopologyReconciled condition on the cluster.
// The TopologyReconciled condition is considered true if spec of all the objects associated with the
// cluster are in sync with the topology defined in the cluster.
// The condition is false under the following conditions:
// - An error occurred during the reconcile process of the cluster topology.
// - The cluster upgrade has not yet propagated to all the components of the cluster.
//   - For a managed topology cluster the version upgrade is propagated one component at a time.
//     In such a case, since some of the component's spec would be adrift from the topology the
//     topology cannot be considered fully reconciled.
func (r *Reconciler) reconcileTopologyReconciledCondition(s *scope.Scope, cluster *clusterv1.Cluster, reconcileErr error) error {
	// If an error occurred during reconciliation set the TopologyReconciled condition to false.
	// Add the error message from the reconcile function to the message of the condition.
	if reconcileErr != nil {
		conditions.Set(
			cluster,
			conditions.FalseCondition(
				clusterv1.TopologyReconciledCondition,
				clusterv1.TopologyReconcileFailedReason,
				clusterv1.ConditionSeverityError,
				reconcileErr.Error(),
			),
		)
		return nil
	}

	// If either the Control Plane or any of the MachineDeployments are still pending to pick up the new version (generally
	// happens when upgrading the cluster) then the topology is not considered as fully reconciled.
	if s.UpgradeTracker.ControlPlane.PendingUpgrade || s.UpgradeTracker.MachineDeployments.PendingUpgrade() {
		msgBuilder := &strings.Builder{}
		var reason string
		if s.UpgradeTracker.ControlPlane.PendingUpgrade {
			msgBuilder.WriteString(fmt.Sprintf("Control plane upgrade to %s on hold. ", s.Blueprint.Topology.Version))
			reason = clusterv1.TopologyReconciledControlPlaneUpgradePendingReason
		} else {
			msgBuilder.WriteString(fmt.Sprintf("MachineDeployment(s) %s upgrade to version %s on hold. ",
				strings.Join(s.UpgradeTracker.MachineDeployments.PendingUpgradeNames(), ", "),
				s.Blueprint.Topology.Version,
			))
			reason = clusterv1.TopologyReconciledMachineDeploymentsUpgradePendingReason
		}

		switch {
		case s.UpgradeTracker.ControlPlane.IsProvisioning:
			msgBuilder.WriteString("Control plane is completing initial provisioning")

		case s.UpgradeTracker.ControlPlane.IsUpgrading:
			cpVersion, err := contract.ControlPlane().Version().Get(s.Current.ControlPlane.Object)
			if err != nil {
				return errors.Wrap(err, "failed to get control plane spec version")
			}
			msgBuilder.WriteString(fmt.Sprintf("Control plane is upgrading to version %s", *cpVersion))

		case s.UpgradeTracker.ControlPlane.IsScaling:
			msgBuilder.WriteString("Control plane is reconciling desired replicas")

		case s.Current.MachineDeployments.IsAnyRollingOut():
			msgBuilder.WriteString(fmt.Sprintf("MachineDeployment(s) %s are rolling out", strings.Join(
				s.UpgradeTracker.MachineDeployments.RolloutNames(), ", ",
			)))
		}

		conditions.Set(
			cluster,
			conditions.FalseCondition(
				clusterv1.TopologyReconciledCondition,
				reason,
				clusterv1.ConditionSeverityInfo,
				msgBuilder.String(),
			),
		)
		return nil
	}

	// If there are no errors while reconciling and if the topology is not holding out changes
	// we can consider that spec of all the objects is reconciled to match the topology. Set the
	// TopologyReconciled condition to true.
	conditions.Set(
		cluster,
		conditions.TrueCondition(clusterv1.TopologyReconciledCondition),
	)

	return nil
}
