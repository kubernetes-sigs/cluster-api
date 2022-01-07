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

package scope

import "k8s.io/apimachinery/pkg/util/sets"

const maxMachineDeploymentUpgradeConcurrency = 1

// UpgradeTracker is a helper to capture the upgrade status and make upgrade decisions.
type UpgradeTracker struct {
	ControlPlane       ControlPlaneUpgradeTracker
	MachineDeployments MachineDeploymentUpgradeTracker
}

// ControlPlaneUpgradeTracker holds the current upgrade status of the Control Plane.
type ControlPlaneUpgradeTracker struct {
	// PendingUpgrade is true if the control plane version needs to be updated. False otherwise.
	PendingUpgrade bool

	// IsProvisioning is true if the control plane is being provisioned for the first time. False otherwise.
	IsProvisioning bool

	// IsUpgrading is true if the control plane is in the middle of an upgrade.
	// Note: Refer to control plane contract for definition of upgrading.
	IsUpgrading bool

	// IsScaling is true if the control plane is in the middle of a scale operation.
	// Note: Refer to control plane contract for definition of scaling.
	IsScaling bool
}

// MachineDeploymentUpgradeTracker holds the current upgrade status and makes upgrade
// decisions for MachineDeployments.
type MachineDeploymentUpgradeTracker struct {
	pendingNames    sets.String
	rollingOutNames sets.String
}

// NewUpgradeTracker returns an upgrade tracker with empty tracking information.
func NewUpgradeTracker() *UpgradeTracker {
	return &UpgradeTracker{
		MachineDeployments: MachineDeploymentUpgradeTracker{
			pendingNames:    sets.NewString(),
			rollingOutNames: sets.NewString(),
		},
	}
}

// MarkRollingOut marks a MachineDeployment as currently rolling out or
// is about to rollout.
// NOTE: We are using Rollout because this includes upgrades and also other changes
// that could imply Machines being created/deleted; in both cases we should wait for
// the operation to complete before moving to the next step of the Cluster upgrade.
func (m *MachineDeploymentUpgradeTracker) MarkRollingOut(names ...string) {
	for _, name := range names {
		m.rollingOutNames.Insert(name)
	}
}

// RolloutNames returns the list of machine deployments that are rolling out or
// are about to rollout.
func (m *MachineDeploymentUpgradeTracker) RolloutNames() []string {
	return m.rollingOutNames.List()
}

// AllowUpgrade returns true if a MachineDeployment is allowed to upgrade,
// returns false otherwise.
// Note: If AllowUpgrade returns true the machine deployment will pick up
// the topology version. This will eventually trigger a machine deployment
// rollout.
func (m *MachineDeploymentUpgradeTracker) AllowUpgrade() bool {
	return m.rollingOutNames.Len() < maxMachineDeploymentUpgradeConcurrency
}

// MarkPendingUpgrade marks a machine deployment as in need of an upgrade.
// This is generally used to capture machine deployments that have not yet
// picked up the topology version.
func (m *MachineDeploymentUpgradeTracker) MarkPendingUpgrade(name string) {
	m.pendingNames.Insert(name)
}

// PendingUpgradeNames returns the list of machine deployment names that
// are pending an upgrade.
func (m *MachineDeploymentUpgradeTracker) PendingUpgradeNames() []string {
	return m.pendingNames.List()
}

// PendingUpgrade returns true if any of the machine deployments are pending
// an upgrade. Returns false, otherwise.
func (m *MachineDeploymentUpgradeTracker) PendingUpgrade() bool {
	return len(m.pendingNames) != 0
}
