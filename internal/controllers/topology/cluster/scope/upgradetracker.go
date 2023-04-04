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
	pendingNames                           sets.Set[string]
	deferredNames                          sets.Set[string]
	upgradingNames                         sets.Set[string]
	rollingOutNames                        sets.Set[string]
	holdUpgrades                           bool
	maxMachineDeploymentUpgradeConcurrency int
}

// UpgradeTrackerOptions contains the options for NewUpgradeTracker.
type UpgradeTrackerOptions struct {
	maxMDUpgradeConcurrency int
}

// UpgradeTrackerOption returns an option for the NewUpgradeTracker function.
type UpgradeTrackerOption interface {
	ApplyToUpgradeTracker(options *UpgradeTrackerOptions)
}

// MaxMDUpgradeConcurrency sets the upper limit for the number of Machine Deployments that can upgrade
// concurrently.
type MaxMDUpgradeConcurrency int

// ApplyToUpgradeTracker applies the given UpgradeTrackerOptions.
func (m MaxMDUpgradeConcurrency) ApplyToUpgradeTracker(options *UpgradeTrackerOptions) {
	options.maxMDUpgradeConcurrency = int(m)
}

// NewUpgradeTracker returns an upgrade tracker with empty tracking information.
func NewUpgradeTracker(opts ...UpgradeTrackerOption) *UpgradeTracker {
	options := &UpgradeTrackerOptions{}
	for _, o := range opts {
		o.ApplyToUpgradeTracker(options)
	}
	if options.maxMDUpgradeConcurrency < 1 {
		// The concurrency should be at least 1.
		options.maxMDUpgradeConcurrency = 1
	}
	return &UpgradeTracker{
		MachineDeployments: MachineDeploymentUpgradeTracker{
			pendingNames:                           sets.Set[string]{},
			deferredNames:                          sets.Set[string]{},
			rollingOutNames:                        sets.Set[string]{},
			upgradingNames:                         sets.Set[string]{},
			maxMachineDeploymentUpgradeConcurrency: options.maxMDUpgradeConcurrency,
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

// MarkUpgradingAndRollingOut marks a MachineDeployment as currently upgrading or about to upgrade.
// NOTE: Marking a MachineDeployment as upgrading also marks it as RollingOut.
func (m *MachineDeploymentUpgradeTracker) MarkUpgradingAndRollingOut(names ...string) {
	for _, name := range names {
		m.upgradingNames.Insert(name)
		m.rollingOutNames.Insert(name)
	}
}

// RolloutNames returns the list of machine deployments that are rolling out or
// are about to rollout.
// Note: This also includes the names of MachineDeployments that are upgrading
// MachineDeployments are also considered rolling out.
func (m *MachineDeploymentUpgradeTracker) RolloutNames() []string {
	return sets.List(m.rollingOutNames)
}

// UpgradingNames returns the list of machine deployments that are upgrading or
// are about to upgrade.
func (m *MachineDeploymentUpgradeTracker) UpgradingNames() []string {
	return sets.List(m.upgradingNames)
}

// HoldUpgrades is used to set if any subsequent upgrade operations should be paused,
// e.g. because a AfterControlPlaneUpgrade hook response asked to do so.
// If HoldUpgrades is called with `true` then AllowUpgrade would return false.
func (m *MachineDeploymentUpgradeTracker) HoldUpgrades(val bool) {
	m.holdUpgrades = val
}

// AllowUpgrade returns true if a MachineDeployment is allowed to upgrade,
// returns false otherwise.
// Note: If AllowUpgrade returns true the machine deployment will pick up
// the topology version. This will eventually trigger a machine deployment
// rollout.
func (m *MachineDeploymentUpgradeTracker) AllowUpgrade() bool {
	if m.holdUpgrades {
		return false
	}
	return m.upgradingNames.Len() < m.maxMachineDeploymentUpgradeConcurrency
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
	return sets.List(m.pendingNames)
}

// PendingUpgrade returns true if any of the machine deployments are pending
// an upgrade. Returns false, otherwise.
func (m *MachineDeploymentUpgradeTracker) PendingUpgrade() bool {
	return len(m.pendingNames) != 0
}

// MarkDeferredUpgrade marks that the upgrade for a MachineDeployment
// has been deferred.
func (m *MachineDeploymentUpgradeTracker) MarkDeferredUpgrade(name string) {
	m.deferredNames.Insert(name)
}

// DeferredUpgradeNames returns the list of MachineDeployment names for
// which the upgrade has been deferred.
func (m *MachineDeploymentUpgradeTracker) DeferredUpgradeNames() []string {
	return sets.List(m.deferredNames)
}

// DeferredUpgrade returns true if the upgrade has been deferred for any of the
// MachineDeployments. Returns false, otherwise.
func (m *MachineDeploymentUpgradeTracker) DeferredUpgrade() bool {
	return len(m.deferredNames) != 0
}
