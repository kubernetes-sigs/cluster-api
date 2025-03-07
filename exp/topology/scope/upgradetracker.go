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
	MachineDeployments WorkerUpgradeTracker
	MachinePools       WorkerUpgradeTracker
}

// ControlPlaneUpgradeTracker holds the current upgrade status of the Control Plane.
type ControlPlaneUpgradeTracker struct {
	// IsPendingUpgrade is true if the Control Plane version needs to be updated. False otherwise.
	// If IsPendingUpgrade is true it also means the Control Plane is not going to pick up the new version
	// in the current reconcile loop.
	// Example cases when IsPendingUpgrade is set to true:
	// - Upgrade is blocked by BeforeClusterUpgrade hook
	// - Upgrade is blocked because the current ControlPlane is not stable (provisioning OR scaling OR upgrading)
	// - Upgrade is blocked because any of the current MachineDeployments or MachinePools are upgrading.
	IsPendingUpgrade bool

	// IsProvisioning is true if the current Control Plane is being provisioned for the first time. False otherwise.
	IsProvisioning bool

	// IsUpgrading is true if the Control Plane is in the middle of an upgrade.
	// Note: Refer to Control Plane contract for definition of upgrading.
	// IsUpgrading is set to true if the current ControlPlane (ControlPlane at the beginning of the reconcile)
	// is upgrading.
	// Note: IsUpgrading only represents the current ControlPlane state. If the Control Plane is about to pick up the
	// version in the reconcile loop IsUpgrading will not be true, because the current ControlPlane is not upgrading,
	// the desired ControlPlane is.
	// Also look at: IsStartingUpgrade.
	IsUpgrading bool

	// IsStartingUpgrade is true if the Control Plane is picking up the new version in the current reconcile loop.
	// If IsStartingUpgrade is true it implies that the desired Control Plane version and the current Control Plane
	// versions are different.
	IsStartingUpgrade bool
}

// WorkerUpgradeTracker holds the current upgrade status of MachineDeployments or MachinePools.
type WorkerUpgradeTracker struct {
	// pendingCreateTopologyNames is the set of MachineDeployment/MachinePool topology names that are newly added to the
	// Cluster Topology but will not be created in the current reconcile loop.
	// By marking a MachineDeployment/MachinePool topology as pendingCreate we skip creating the MachineDeployment/MachinePool.
	// Nb. We use MachineDeployment/MachinePool topology names instead of MachineDeployment/MachinePool names because the new
	// MachineDeployment/MachinePool names can keep changing for each reconcile loop leading to continuous updates to the
	// TopologyReconciled condition.
	pendingCreateTopologyNames sets.Set[string]

	// pendingUpgradeNames is the set of MachineDeployment/MachinePool names that are not going to pick up the new version
	// in the current reconcile loop.
	// By marking a MachineDeployment/MachinePool as pendingUpgrade we skip reconciling the MachineDeployment/MachinePool.
	pendingUpgradeNames sets.Set[string]

	// deferredNames is the set of MachineDeployment/MachinePool names that are not going to pick up the new version
	// in the current reconcile loop because they are deferred by the user.
	// Note: If a MachineDeployment/MachinePool is marked as deferred it should also be marked as pendingUpgrade.
	deferredNames sets.Set[string]

	// upgradingNames is the set of MachineDeployment/MachinePool names that are upgrading. This set contains the names of
	// MachineDeployments/MachinePools that are currently upgrading and the names of MachineDeployments/MachinePools that
	// will pick up the upgrade in the current reconcile loop.
	// Note: This information is used to:
	// - decide if ControlPlane can be upgraded.
	// - calculate MachineDeployment/MachinePool upgrade concurrency.
	// - update TopologyReconciled Condition.
	// - decide if the AfterClusterUpgrade hook can be called.
	upgradingNames sets.Set[string]

	// maxUpgradeConcurrency defines the maximum number of MachineDeployments/MachinePools that should be in an
	// upgrading state. This includes the MachineDeployments/MachinePools that are currently upgrading and the
	// MachineDeployments/MachinePools that will start the upgrade after the current reconcile loop.
	maxUpgradeConcurrency int
}

// UpgradeTrackerOptions contains the options for NewUpgradeTracker.
type UpgradeTrackerOptions struct {
	maxMDUpgradeConcurrency int
	maxMPUpgradeConcurrency int
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

// MaxMPUpgradeConcurrency sets the upper limit for the number of Machine Pools that can upgrade
// concurrently.
type MaxMPUpgradeConcurrency int

// ApplyToUpgradeTracker applies the given UpgradeTrackerOptions.
func (m MaxMPUpgradeConcurrency) ApplyToUpgradeTracker(options *UpgradeTrackerOptions) {
	options.maxMPUpgradeConcurrency = int(m)
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
	if options.maxMPUpgradeConcurrency < 1 {
		// The concurrency should be at least 1.
		options.maxMPUpgradeConcurrency = 1
	}
	return &UpgradeTracker{
		MachineDeployments: WorkerUpgradeTracker{
			pendingCreateTopologyNames: sets.Set[string]{},
			pendingUpgradeNames:        sets.Set[string]{},
			deferredNames:              sets.Set[string]{},
			upgradingNames:             sets.Set[string]{},
			maxUpgradeConcurrency:      options.maxMDUpgradeConcurrency,
		},
		MachinePools: WorkerUpgradeTracker{
			pendingCreateTopologyNames: sets.Set[string]{},
			pendingUpgradeNames:        sets.Set[string]{},
			deferredNames:              sets.Set[string]{},
			upgradingNames:             sets.Set[string]{},
			maxUpgradeConcurrency:      options.maxMPUpgradeConcurrency,
		},
	}
}

// IsControlPlaneStable returns true is the ControlPlane is stable.
func (t *ControlPlaneUpgradeTracker) IsControlPlaneStable() bool {
	// If the current control plane is provisioning it is not considered stable.
	if t.IsProvisioning {
		return false
	}

	// If the current control plane is upgrading it is not considered stable.
	if t.IsUpgrading {
		return false
	}

	// Check if we are about to upgrade the control plane. Since the control plane is about to start its upgrade process
	// it cannot be considered stable.
	if t.IsStartingUpgrade {
		return false
	}

	// If the ControlPlane is pending picking up an upgrade then it is not yet at the desired state and
	// cannot be considered stable.
	if t.IsPendingUpgrade {
		return false
	}

	return true
}

// MarkUpgrading marks a MachineDeployment/MachinePool as currently upgrading or about to upgrade.
func (m *WorkerUpgradeTracker) MarkUpgrading(names ...string) {
	for _, name := range names {
		m.upgradingNames.Insert(name)
	}
}

// UpgradingNames returns the list of machine deployments that are upgrading or
// are about to upgrade.
func (m *WorkerUpgradeTracker) UpgradingNames() []string {
	return sets.List(m.upgradingNames)
}

// UpgradeConcurrencyReached returns true if the number of MachineDeployments/MachinePools upgrading is at the concurrency limit.
func (m *WorkerUpgradeTracker) UpgradeConcurrencyReached() bool {
	return m.upgradingNames.Len() >= m.maxUpgradeConcurrency
}

// MarkPendingCreate marks a machine deployment topology that is pending to be created.
// This is generally used to capture machine deployments that are yet to be created
// because the control plane is not yet stable.
func (m *WorkerUpgradeTracker) MarkPendingCreate(mdTopologyName string) {
	m.pendingCreateTopologyNames.Insert(mdTopologyName)
}

// IsPendingCreate returns true is the MachineDeployment/MachinePool topology is marked as pending create.
func (m *WorkerUpgradeTracker) IsPendingCreate(mdTopologyName string) bool {
	return m.pendingCreateTopologyNames.Has(mdTopologyName)
}

// IsAnyPendingCreate returns true if any of the machine deployments are pending
// to be created. Returns false, otherwise.
func (m *WorkerUpgradeTracker) IsAnyPendingCreate() bool {
	return len(m.pendingCreateTopologyNames) != 0
}

// PendingCreateTopologyNames returns the list of machine deployment topology names that
// are pending create.
func (m *WorkerUpgradeTracker) PendingCreateTopologyNames() []string {
	return sets.List(m.pendingCreateTopologyNames)
}

// MarkPendingUpgrade marks a machine deployment as in need of an upgrade.
// This is generally used to capture machine deployments that have not yet
// picked up the topology version.
func (m *WorkerUpgradeTracker) MarkPendingUpgrade(name string) {
	m.pendingUpgradeNames.Insert(name)
}

// IsPendingUpgrade returns true is the MachineDeployment/MachinePool marked as pending upgrade.
func (m *WorkerUpgradeTracker) IsPendingUpgrade(name string) bool {
	return m.pendingUpgradeNames.Has(name)
}

// IsAnyPendingUpgrade returns true if any of the machine deployments are pending
// an upgrade. Returns false, otherwise.
func (m *WorkerUpgradeTracker) IsAnyPendingUpgrade() bool {
	return len(m.pendingUpgradeNames) != 0
}

// PendingUpgradeNames returns the list of machine deployment names that
// are pending an upgrade.
func (m *WorkerUpgradeTracker) PendingUpgradeNames() []string {
	return sets.List(m.pendingUpgradeNames)
}

// MarkDeferredUpgrade marks that the upgrade for a MachineDeployment/MachinePool
// has been deferred.
func (m *WorkerUpgradeTracker) MarkDeferredUpgrade(name string) {
	m.deferredNames.Insert(name)
}

// DeferredUpgradeNames returns the list of MachineDeployment/MachinePool names for
// which the upgrade has been deferred.
func (m *WorkerUpgradeTracker) DeferredUpgradeNames() []string {
	return sets.List(m.deferredNames)
}

// DeferredUpgrade returns true if the upgrade has been deferred for any of the
// MachineDeployments/MachinePools. Returns false, otherwise.
func (m *WorkerUpgradeTracker) DeferredUpgrade() bool {
	return len(m.deferredNames) != 0
}
