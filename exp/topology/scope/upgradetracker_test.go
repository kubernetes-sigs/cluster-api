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
}
