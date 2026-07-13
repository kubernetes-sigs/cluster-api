/*
Copyright 2023 The Kubernetes Authors.

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

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewUpgradeTracker(t *testing.T) {
	t.Run("should set the correct value for maxUpgradeConcurrency", func(t *testing.T) {
		tests := []struct {
			name    string
			options []UpgradeTrackerOption
			want    int
		}{
			{
				name:    "should set a default value of 1 if not concurrency is not specified",
				options: nil,
				want:    1,
			},
			{
				name:    "should set the value of 1 if given concurrency is less than 1",
				options: []UpgradeTrackerOption{MaxMDUpgradeConcurrency(0), MaxMPUpgradeConcurrency(0)},
				want:    1,
			},
			{
				name:    "should set the value to the given concurrency if the value is greater than 0",
				options: []UpgradeTrackerOption{MaxMDUpgradeConcurrency(2), MaxMPUpgradeConcurrency(2)},
				want:    2,
			},
		}

		for _, tt := range tests {
			g := NewWithT(t)
			got := NewUpgradeTracker(tt.options...)
			g.Expect(got.MachineDeployments.maxUpgradeConcurrency).To(Equal(tt.want))
			g.Expect(got.MachinePools.maxUpgradeConcurrency).To(Equal(tt.want))
		}
	})

	t.Run("should set the correct value for maxRolloutConcurrency", func(t *testing.T) {
		tests := []struct {
			name     string
			options  []UpgradeTrackerOption
			wantMD   int
			wantPool int
		}{
			{
				name:     "should disable rollout sequencing if concurrency is not specified",
				options:  nil,
				wantMD:   0,
				wantPool: 0,
			},
			{
				name:     "should disable rollout sequencing if concurrency is less than 1",
				options:  []UpgradeTrackerOption{MaxMDRolloutConcurrency(-1)},
				wantMD:   0,
				wantPool: 0,
			},
			{
				name:     "should set MachineDeployment rollout concurrency if the value is greater than 0",
				options:  []UpgradeTrackerOption{MaxMDRolloutConcurrency(2)},
				wantMD:   2,
				wantPool: 0,
			},
		}

		for _, tt := range tests {
			g := NewWithT(t)
			got := NewUpgradeTracker(tt.options...)
			g.Expect(got.MachineDeployments.maxRolloutConcurrency).To(Equal(tt.wantMD))
			g.Expect(got.MachinePools.maxRolloutConcurrency).To(Equal(tt.wantPool))
		}
	})
}

func TestRolloutConcurrencyReached(t *testing.T) {
	t.Run("should always return false if rollout sequencing is disabled", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker()
		tracker.MachineDeployments.MarkRollingOut("md-1")
		tracker.MachineDeployments.MarkUpgrading("md-2")

		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("md-3")).To(BeFalse())
	})

	t.Run("should return true if a rolling out MachineDeployment reaches the limit", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker(MaxMDRolloutConcurrency(1))
		tracker.MachineDeployments.MarkRollingOut("md-1")

		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("md-2")).To(BeTrue())
	})

	t.Run("should not double count MachineDeployments that are both rolling out and upgrading", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker(MaxMDRolloutConcurrency(2))
		tracker.MachineDeployments.MarkRollingOut("md-1")
		tracker.MachineDeployments.MarkUpgrading("md-1")

		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("md-2")).To(BeFalse())

		tracker.MachineDeployments.MarkUpgrading("md-2")
		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("md-3")).To(BeTrue())
	})

	t.Run("should count upgrading MachineDeployments against rollout concurrency", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker(MaxMDRolloutConcurrency(1))
		tracker.MachineDeployments.MarkUpgrading("md-1")

		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("md-2")).To(BeTrue())
	})

	t.Run("should enforce group concurrency independently", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker()
		tracker.MachineDeployments.AddToRolloutGroup("db-1", "database", 1)
		tracker.MachineDeployments.AddToRolloutGroup("db-2", "database", 1)
		tracker.MachineDeployments.AddToRolloutGroup("app-1", "applications", 1)
		tracker.MachineDeployments.AddToRolloutGroup("app-2", "applications", 1)
		tracker.MachineDeployments.MarkRollingOut("db-1")

		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("db-2")).To(BeTrue())
		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("app-1")).To(BeFalse())

		tracker.MachineDeployments.MarkUpgrading("app-1")
		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("app-2")).To(BeTrue())
	})

	t.Run("should enforce overall concurrency across groups", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker(MaxMDRolloutConcurrency(1))
		tracker.MachineDeployments.AddToRolloutGroup("db-1", "database", 1)
		tracker.MachineDeployments.AddToRolloutGroup("app-1", "applications", 1)
		tracker.MachineDeployments.MarkRollingOut("db-1")

		g.Expect(tracker.MachineDeployments.RolloutConcurrencyReached("app-1")).To(BeTrue())
	})
}

func TestUpgradeConcurrencyReachedWithRolloutPolicy(t *testing.T) {
	t.Run("should use rollout groups instead of the legacy global upgrade concurrency", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker()
		tracker.MachineDeployments.AddToRolloutGroup("db-1", "database", 1)
		tracker.MachineDeployments.AddToRolloutGroup("db-2", "database", 1)
		tracker.MachineDeployments.AddToRolloutGroup("app-1", "applications", 1)
		tracker.MachineDeployments.MarkUpgrading("db-1")

		g.Expect(tracker.MachineDeployments.UpgradeConcurrencyReached("db-2")).To(BeTrue())
		g.Expect(tracker.MachineDeployments.UpgradeConcurrencyReached("app-1")).To(BeFalse())
	})

	t.Run("should apply the overall rollout limit to version upgrades", func(t *testing.T) {
		g := NewWithT(t)
		tracker := NewUpgradeTracker(MaxMDRolloutConcurrency(2))
		tracker.MachineDeployments.MarkUpgrading("md-1")

		g.Expect(tracker.MachineDeployments.UpgradeConcurrencyReached("md-2")).To(BeFalse())
		tracker.MachineDeployments.MarkUpgrading("md-2")
		g.Expect(tracker.MachineDeployments.UpgradeConcurrencyReached("md-3")).To(BeTrue())
	})
}

func TestRollingOut(t *testing.T) {
	g := NewWithT(t)
	tracker := NewUpgradeTracker(MaxMDRolloutConcurrency(2))

	tracker.MachineDeployments.MarkRollingOut("md-1", "md-2")
	g.Expect(tracker.MachineDeployments.RollingOutNames()).To(ConsistOf("md-1", "md-2"))
	g.Expect(tracker.MachineDeployments.IsRollingOut("md-1")).To(BeTrue())
	g.Expect(tracker.MachineDeployments.IsRollingOut("md-3")).To(BeFalse())

	tracker.MachineDeployments.MarkUpgrading("md-3")
	g.Expect(tracker.MachineDeployments.IsRollingOut("md-3")).To(BeTrue())
}

func TestPendingRollout(t *testing.T) {
	g := NewWithT(t)
	tracker := NewUpgradeTracker(MaxMDRolloutConcurrency(1))

	g.Expect(tracker.MachineDeployments.IsAnyPendingRollout()).To(BeFalse())
	g.Expect(tracker.MachineDeployments.IsPendingRollout("md-1")).To(BeFalse())

	tracker.MachineDeployments.MarkPendingRollout("md-1")
	g.Expect(tracker.MachineDeployments.IsAnyPendingRollout()).To(BeTrue())
	g.Expect(tracker.MachineDeployments.IsPendingRollout("md-1")).To(BeTrue())
	g.Expect(tracker.MachineDeployments.PendingRolloutNames()).To(ConsistOf("md-1"))
}
