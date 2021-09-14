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
	MachineDeployments MachineDeploymentUpgradeTracker
}

// MachineDeploymentUpgradeTracker holds the current upgrade status and makes upgrade
// decisions for MachineDeployments.
type MachineDeploymentUpgradeTracker struct {
	names sets.String
}

// NewUpgradeTracker returns an upgrade tracker with empty tracking information.
func NewUpgradeTracker() *UpgradeTracker {
	return &UpgradeTracker{
		MachineDeployments: MachineDeploymentUpgradeTracker{
			names: sets.NewString(),
		},
	}
}

// Insert adds name to the set of MachineDeployments that will be upgraded.
func (m *MachineDeploymentUpgradeTracker) Insert(name string) {
	m.names.Insert(name)
}

// AllowUpgrade returns true if a MachineDeployment is allowed to upgrade,
// returns false otherwise.
func (m *MachineDeploymentUpgradeTracker) AllowUpgrade() bool {
	return m.names.Len() < maxMachineDeploymentUpgradeConcurrency
}
