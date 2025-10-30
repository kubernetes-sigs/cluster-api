/*
Copyright 2025 The Kubernetes Authors.

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

package desiredstate

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	utilfeature "k8s.io/component-base/featuregate/testing"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestIsLowerThanMinVersion(t *testing.T) {
	tests := []struct {
		name                string
		v                   semver.Version
		minVersion          semver.Version
		controlPlaneVersion semver.Version
		want                bool
	}{
		{
			name:                "Equal",
			controlPlaneVersion: semver.MustParse("1.30.1"), // control plane at 1.30.1
			minVersion:          semver.MustParse("1.30.1"), // first machine deployment at 1.30.1
			v:                   semver.MustParse("1.30.1"), // second machine deployment at 1.30.1
			want:                false,
		},
		{
			name:                "Lower",
			controlPlaneVersion: semver.MustParse("1.30.1"), // control plane at 1.30.1
			minVersion:          semver.MustParse("1.30.1"), // first machine deployment at 1.30.1
			v:                   semver.MustParse("1.29.0"), // second machine deployment still at 1.29.0
			want:                true,
		},
		{
			name:                "Equal pre-release",
			controlPlaneVersion: semver.MustParse("1.30.1-alpha.1"), // control plane at 1.30.1-alpha.1
			minVersion:          semver.MustParse("1.30.1-alpha.1"), // first machine deployment at 1.30.1-alpha.1
			v:                   semver.MustParse("1.30.1-alpha.1"), // second machine deployment at 1.30.1-alpha.1
			want:                false,
		},
		{
			name:                "Lower pre-release",
			controlPlaneVersion: semver.MustParse("1.30.1-alpha.1"), // control plane at 1.30.1-alpha.1
			minVersion:          semver.MustParse("1.30.1-alpha.1"), // first machine deployment at 1.30.1-alpha.1
			v:                   semver.MustParse("1.30.1-alpha.0"), // second machine deployment still at 1.30.1-alpha.0
			want:                true,
		},
		{
			name:                "Equal pre-release",
			controlPlaneVersion: semver.MustParse("1.31.1+foo.2-bar.1"), // control plane at 1.31.1+foo.2-bar.1
			minVersion:          semver.MustParse("1.31.1+foo.2-bar.1"), // first machine deployment at 1.31.1+foo.2-bar.1
			v:                   semver.MustParse("1.31.1+foo.2-bar.1"), // second machine deployment at 1.31.1+foo.2-bar.1
			want:                false,
		},
		{
			name:                "Lower pre-release",
			controlPlaneVersion: semver.MustParse("1.31.1+foo.2-bar.1"), // control plane at 1.31.1+foo.2-bar.1
			minVersion:          semver.MustParse("1.31.1+foo.2-bar.1"), // first machine deployment at 1.31.1+foo.2-bar.1
			v:                   semver.MustParse("1.31.1+foo.1-bar.1"), // second machine deployment still at 1.31.1+foo.1-bar.1
			want:                true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := isLowerThanMinVersion(tt.v, tt.minVersion, tt.controlPlaneVersion)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestComputeUpgradePlan(t *testing.T) {
	tests := []struct {
		name                             string
		topologyVersion                  string
		controlPlaneVersion              string
		machineDeploymentVersion         string
		machinePoolVersion               string
		F                                GetUpgradePlanFunc
		wantControlPlaneUpgradePlan      []string
		wantMachineDeploymentUpgradePlan []string
		wantMachinePoolUpgradePlan       []string
		wantErr                          bool
		wantErrMessage                   string
	}{
		// return early
		{
			name:            "No op if control plane dose not exists",
			topologyVersion: "v1.33.1",
		},
		{
			name:                     "No op if everything is up to date",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.33.1",
			machinePoolVersion:       "v1.33.1",
		},

		// validation errors
		{
			name:                "Fails for invalid control plane version",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "foo",
			wantErr:             true,
			wantErrMessage:      "failed to parse ControlPlane version foo: Invalid character(s) found in major number \"0foo\"",
		},
		{
			name:                "Fails if control plane upgrade plan has invalid versions",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"foo"}, nil, nil
			},
			wantErr:        true,
			wantErrMessage: "invalid ControlPlane upgrade plan: item 0; failed to parse version foo: Invalid character(s) found in major number \"0foo\"",
		},
		{
			name:                "Fails if control plane upgrade plan starts with the wrong minor (too old)",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.30.0"}, nil, nil // v1.31 expected
			},
			wantErr:        true,
			wantErrMessage: "invalid ControlPlane upgrade plan: item 0; version v1.30.0 must be greater than v1.30.0",
		},
		{
			name:                "Fails if control plane upgrade plan starts with the wrong minor (too new)",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.1"}, nil, nil // v1.31 expected
			},
			wantErr:        true,
			wantErrMessage: "invalid ControlPlane upgrade plan: item 0; expecting a version with minor 30 or 31, found version v1.32.1",
		},
		{
			name:                "Fails if control plane upgrade plan has a downgrade",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.30.0"}, nil, nil // v1.31 -> v1.30 is a downgrade!
			},
			wantErr:        true,
			wantErrMessage: "invalid ControlPlane upgrade plan: item 1; version v1.30.0 must be greater than v1.31.1",
		},
		{
			name:                "Fails if control plane upgrade plan doesn't end with the target version (stops before the target version)",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0"}, nil, nil // v1.33 missing
			},
			wantErr:        true,
			wantErrMessage: "invalid ControlPlane upgrade plan: control plane upgrade plan must end with version v1.33.1, ends with v1.32.0 instead",
		},
		{
			name:                "Fails if control plane upgrade plan doesn't end with the target version (goes past the target version)",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1", "v1.34.1"}, nil, nil // v1.34 is after the target version
			},
			wantErr:        true,
			wantErrMessage: "invalid ControlPlane upgrade plan: control plane upgrade plan must end with version v1.33.1, ends with v1.34.1 instead",
		},
		{
			name:                     "Fails if control plane upgrade plan is returned but control plane is already up to date",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.33.1"}, nil, nil // control plane is already up to date
			},
			wantErr:        true,
			wantErrMessage: "invalid ControlPlane upgrade plan: control plane is already at the desired version",
		},
		{
			name:                     "Fails if workers plan has versions not included in control plane upgrade plan",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1"}, []string{"v1.32.2"}, nil // v1.32.2 is not a version in the control plane upgrade plan
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan: versions v1.32.2 doesn't match any versions in the control plane upgrade plan nor the control plane version",
		},
		{
			name:                     "Fails if workers plan has versions in the wrong order",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1"}, []string{"v1.33.1", "v1.32.0"}, nil // v1.33 -> v1.32 is a downgrade!
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan, item 1; version v1.32.0 must be greater than v1.33.1",
		},
		{
			name:                     "Fails if workers plan has invalid versions",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"foo"}, nil
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan, item 0; failed to parse version foo: Invalid character(s) found in major number \"0foo\"",
		},
		{
			name:                     "Fails if workers plan starts with the wrong minor (too old)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.30.0"}, nil // v1.30.0 is the current min worker version
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan, item 0; version v1.30.0 must be greater than v1.30.0",
		},
		{
			name:                     "Fails if workers plan doesn't end with the target version (stops before the target version)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.32.1"}, nil // v1.32.1 is before the target version
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan; workers upgrade plan must end with version v1.33.1, ends with v1.32.1 instead",
		},
		{
			name:                     "Fails if workers plan doesn't end with the target version (goes past the target version)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.31.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.34.1"}, nil // v1.34.1 is past the target version
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan; workers upgrade plan must end with version v1.33.1, ends with v1.34.1 instead",
		},
		{
			name:                     "Fails if workers plan doesn't comply with Kubernetes version skew versions",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.34.1",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.34.1"}, nil // workers cannot go from minor 30 to minor 34, an intermediate step is required
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan, item 0; workers cannot go from minor 30 (v1.30.0) to minor 34 (v1.34.1), an intermediate step is required to comply with Kubernetes version skew rules",
		},
		{
			name:                "Fails if there are no workers and a worker upgrade plan is provided",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1"}, []string{"v1.35.1"}, nil // there should not be worker update plan
			},
			wantErr:        true,
			wantErrMessage: "invalid worker upgrade plan; there are no workers or workers already at the desired version",
		},

		// upgrade sequence 1: CP only
		{
			name:                "Return control plane upgrade plan, empty machine deployment and machine pool upgrade plan",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan: []string{"v1.31.1", "v1.32.0", "v1.33.1"},
		},
		{
			name:                "Return control plane upgrade plan, empty machine deployment and machine pool upgrade plan (after CP upgrade to 1.31)",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan: []string{"v1.32.0", "v1.33.1"},
		},
		{
			name:                "Return control plane upgrade plan, empty machine deployment and machine pool upgrade plan (after CP upgrade to 1.32)",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.32.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.33.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan: []string{"v1.33.1"},
		},
		{
			name:                "Return control plane upgrade plan, empty machine deployment and machine pool upgrade plan (after CP upgrade to 1.33)",
			topologyVersion:     "v1.33.1",
			controlPlaneVersion: "v1.33.1",
		},

		// upgrade sequence 2: CP, MD (no MP); defer to the system computing the workers upgrade plan
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version",
			topologyVersion:          "v1.32.0",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1", "v1.32.0"},
			wantMachineDeploymentUpgradePlan: []string{"v1.32.0"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version (after CP upgrade to 1.31)",
			topologyVersion:          "v1.32.0",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0"},
			wantMachineDeploymentUpgradePlan: []string{"v1.32.0"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version (after CP upgrade to 1.32)",
			topologyVersion:          "v1.32.0",
			controlPlaneVersion:      "v1.32.0",
			machineDeploymentVersion: "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, nil, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.32.0"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version (after CP and MD upgrade to 1.32)",
			topologyVersion:          "v1.32.0",
			controlPlaneVersion:      "v1.32.0",
			machineDeploymentVersion: "v1.32.0",
		},

		// upgrade sequence 3: CP, MP (no MD); defer to the system computing the workers upgrade plan
		{
			name:                "Return control plane and machine pool upgrade plan with last version",
			topologyVersion:     "v1.32.0",
			controlPlaneVersion: "v1.30.0",
			machinePoolVersion:  "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0"}, nil, nil
			},
			wantControlPlaneUpgradePlan: []string{"v1.31.1", "v1.32.0"},
			wantMachinePoolUpgradePlan:  []string{"v1.32.0"},
		},
		{
			name:                "Return control plane and machine pool upgrade plan with last version (after CP upgrade to 1.31)",
			topologyVersion:     "v1.32.0",
			controlPlaneVersion: "v1.31.1",
			machinePoolVersion:  "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0"}, nil, nil
			},
			wantControlPlaneUpgradePlan: []string{"v1.32.0"},
			wantMachinePoolUpgradePlan:  []string{"v1.32.0"},
		},
		{
			name:                "Return control plane and machine pool upgrade plan with last version (after CP upgrade to 1.32)",
			topologyVersion:     "v1.32.0",
			controlPlaneVersion: "v1.32.0",
			machinePoolVersion:  "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, nil, nil
			},
			wantControlPlaneUpgradePlan: nil,
			wantMachinePoolUpgradePlan:  []string{"v1.32.0"},
		},
		{
			name:                "Return control plane and machine pool upgrade plan with last version (after CP and MP upgrade to 1.32)",
			topologyVersion:     "v1.32.0",
			controlPlaneVersion: "v1.32.0",
			machinePoolVersion:  "v1.32.0",
		},

		// upgrade sequence 3: CP, MD, MP; defer to the system computing the workers upgrade plan, an additional worker upgrade step required to respect version skew
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1", "v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1", "v1.32.0", "v1.33.1", "v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1", "v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1", "v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP upgrade to 1.31)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1", "v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0", "v1.33.1", "v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1", "v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1", "v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP upgrade to 1.32)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.32.0",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.33.1", "v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.33.1", "v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1", "v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1", "v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP upgrade to 1.33)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1", "v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1", "v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP and MD upgrade to 1.33)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.33.1",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1", "v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1", "v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP, MD and MP upgrade to 1.33)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.33.1",
			machinePoolVersion:       "v1.33.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP upgrade to 1.34)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.34.1",
			machineDeploymentVersion: "v1.33.1",
			machinePoolVersion:       "v1.33.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, nil, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP and MD upgrade to 1.34)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.34.1",
			machineDeploymentVersion: "v1.34.1",
			machinePoolVersion:       "v1.33.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, nil, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions (after CP, MD and MP upgrade to 1.34)",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.34.1",
			machineDeploymentVersion: "v1.34.1",
			machinePoolVersion:       "v1.34.1",
		},

		// upgrade sequence 4: CP, MD, MP; workers upgrade plan provided, force worker upgrades to an intermediate K8s version (even if not necessary)
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1"}, []string{"v1.31.1", "v1.33.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1", "v1.32.0", "v1.33.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1", "v1.33.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.31.1", "v1.33.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version (after CP upgrade to v1.31)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1"}, []string{"v1.31.1", "v1.33.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0", "v1.33.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1", "v1.33.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.31.1", "v1.33.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version (after CP and MD upgrade to v1.31)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1"}, []string{"v1.31.1", "v1.33.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0", "v1.33.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1", "v1.33.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.31.1", "v1.33.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version (after CP, MD and MP upgrade to v1.31)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1"}, []string{"v1.33.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0", "v1.33.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version (after CP upgrade to v1.32)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.32.0",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.33.1"}, []string{"v1.33.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.33.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version (after CP upgrade to v1.33)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.33.1"}, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version (after CP and MD upgrade to v1.33)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.33.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.33.1"}, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.33.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan custom worker version + last version (after CP, MD and MP upgrade to v1.33)",
			topologyVersion:          "v1.33.1",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.33.1",
			machinePoolVersion:       "v1.33.1",
		},

		// upgrade sequence 5: CP, MD, MP; workers upgrade plan provided, force worker upgrades to an intermediate K8s version (even if not necessary) + additional worker upgrade to respect version skew
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"}, []string{"v1.31.1", "v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1", "v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1", "v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.31.1", "v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP upgrade to v1.31)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"}, []string{"v1.31.1", "v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1", "v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.31.1", "v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP and MD upgrade to v1.31)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"}, []string{"v1.31.1", "v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1", "v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.31.1", "v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP, MD and MP upgrade to v1.31)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.31.1",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"}, []string{"v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.0", "v1.33.1", "v1.34.2", "v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP upgrade to v1.32)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.32.0",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.33.1", "v1.34.2", "v1.35.2"}, []string{"v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.33.1", "v1.34.2", "v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP upgrade to v1.33)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.33.1",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.34.2", "v1.35.2"}, []string{"v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.34.2", "v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP upgrade to v1.34)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.34.2",
			machineDeploymentVersion: "v1.31.1",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.35.2"}, []string{"v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP and MD upgrade to v1.34)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.34.2",
			machineDeploymentVersion: "v1.34.2",
			machinePoolVersion:       "v1.31.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.35.2"}, []string{"v1.34.2", "v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.34.2", "v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.34.2", "v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP, MD and MP upgrade to v1.34)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.34.2",
			machineDeploymentVersion: "v1.34.2",
			machinePoolVersion:       "v1.34.2",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.35.2"}, []string{"v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.35.2"},
			wantMachineDeploymentUpgradePlan: []string{"v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP upgrade to v1.35)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.35.2",
			machineDeploymentVersion: "v1.34.2",
			machinePoolVersion:       "v1.34.2",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP and MD upgrade to v1.35)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.35.2",
			machineDeploymentVersion: "v1.35.2",
			machinePoolVersion:       "v1.34.2",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, []string{"v1.35.2"}, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.35.2"},
			wantMachinePoolUpgradePlan:       []string{"v1.35.2"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan with custom worker version + version skew versions + last version (after CP, MD and MP upgrade to v1.35)",
			topologyVersion:          "v1.35.2",
			controlPlaneVersion:      "v1.35.2",
			machineDeploymentVersion: "v1.35.2",
			machinePoolVersion:       "v1.35.2",
		},

		// allows upgrades plans with many patch version for the same minor
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.31.2", "v1.32.0", "v1.33.1", "v1.33.2", "v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1", "v1.31.2", "v1.32.0", "v1.33.1", "v1.33.2", "v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.2", "v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.2", "v1.34.1"},
		},

		// allows upgrades with pre-release versions
		{
			name:                     "Return control plane and machine deployment upgrade plan with last version + version skew versions",
			topologyVersion:          "v1.34.1",
			controlPlaneVersion:      "v1.30.0",
			machineDeploymentVersion: "v1.30.0",
			machinePoolVersion:       "v1.30.0",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1", "v1.32.0", "v1.33.2", "v1.34.1-beta.0", "v1.34.1-rc.0", "v1.34.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1", "v1.32.0", "v1.33.2", "v1.34.1-beta.0", "v1.34.1-rc.0", "v1.34.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.33.2", "v1.34.1"},
			wantMachinePoolUpgradePlan:       []string{"v1.33.2", "v1.34.1"},
		},

		// upgrade sequence 6: when using build tags
		{
			name:                     "Return control plane and machine deployment upgrade plan when using build tags",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.30.0+foo.1-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.32.1+foo.1-bar.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan when using build tags (after CP upgrade to v1.31.1+foo.1-bar.1)",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.31.1+foo.1-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.32.1+foo.1-bar.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan when using build tags (after CP upgrade to v1.31.1+foo.2-bar.1)",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.31.1+foo.2-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.32.1+foo.1-bar.1"}, nil, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.32.1+foo.1-bar.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.32.1+foo.1-bar.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan when using build tags (after CP upgrade to v1.32.1+foo.1-bar.1)",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.32.1+foo.1-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, nil, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: []string{"v1.32.1+foo.1-bar.1"},
		},
		{
			name:                     "Return control plane and machine deployment upgrade plan when using build tags (after CP and MD upgrade to v1.32.1+foo.1-bar.1)",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.32.1+foo.1-bar.1",
			machineDeploymentVersion: "v1.32.1+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return nil, nil, nil
			},
			wantControlPlaneUpgradePlan:      nil,
			wantMachineDeploymentUpgradePlan: nil,
		},

		// fails if control plane plan and workers upgrade plan do not agree on ordering of version with the same major.minor.patch but different build tags
		{
			name:                     "Fails if control plane plan and workers upgrade plan do not agree on ordering",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.30.0+foo.1-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, []string{"v1.31.1+foo.2-bar.1", "v1.31.1+foo.1-bar.1", "v1.32.1+foo.1-bar.1"}, nil
			},
			wantErr:        true,
			wantErrMessage: "invalid workers upgrade plan, item 1; version v1.31.1+foo.1-bar.1 must be before v1.31.1+foo.2-bar.1",
		},
		{
			name:                     "Pass if control plane plan and workers upgrade plan do agree on ordering (chained upgrades)",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.30.0+foo.1-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"},
		},
		{
			name:                     "Pass if control plane plan and workers upgrade plan do agree on ordering (chained upgrades)",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.31.1+foo.1-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"},
		},
		{
			name:                     "Pass if control plane plan and workers upgrade plan do agree on ordering (chained upgrade when skipping versions)",
			topologyVersion:          "v1.32.1+foo.1-bar.1",
			controlPlaneVersion:      "v1.30.0+foo.1-bar.1",
			machineDeploymentVersion: "v1.30.0+foo.1-bar.1",
			F: func(_ context.Context, _, _, _ string) ([]string, []string, error) {
				return []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"}, []string{"v1.31.1+foo.1-bar.1", "v1.32.1+foo.1-bar.1"}, nil
			},
			wantControlPlaneUpgradePlan:      []string{"v1.31.1+foo.1-bar.1", "v1.31.1+foo.2-bar.1", "v1.32.1+foo.1-bar.1"},
			wantMachineDeploymentUpgradePlan: []string{"v1.31.1+foo.1-bar.1", "v1.32.1+foo.1-bar.1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{Topology: clusterv1.Topology{
					Version: tt.topologyVersion,
				}},
				Current:        &scope.ClusterState{},
				UpgradeTracker: scope.NewUpgradeTracker(),
			}
			s.Current.ControlPlane = &scope.ControlPlaneState{}
			if tt.controlPlaneVersion != "" {
				s.Current.ControlPlane.Object = builder.ControlPlane("test1", "cp1").
					WithSpecFields(map[string]interface{}{
						"spec.version": tt.controlPlaneVersion,
					}).Build()
			}
			if tt.machineDeploymentVersion != "" {
				s.Current.MachineDeployments = map[string]*scope.MachineDeploymentState{
					"md1": {
						Object: builder.MachineDeployment("test1", "md1").WithVersion(tt.machineDeploymentVersion).Build(),
					},
				}
			}
			if tt.machinePoolVersion != "" {
				s.Current.MachinePools = map[string]*scope.MachinePoolState{
					"mp1": {
						Object: builder.MachinePool("test1", "mp1").WithVersion(tt.machinePoolVersion).Build(),
					},
				}
			}

			err := ComputeUpgradePlan(ctx, s, tt.F)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())

				if tt.F != nil {
					computedControlPlaneUpgradePlan, _, err := tt.F(nil, "", "", "")
					g.Expect(err).ToNot(HaveOccurred())
					// Ensure the computed control plane upgrade plan is not modified later in ComputeUpgradePlan.
					g.Expect(computedControlPlaneUpgradePlan).To(Equal(tt.wantControlPlaneUpgradePlan))
				}
			}
			g.Expect(s.UpgradeTracker.ControlPlane.UpgradePlan).To(Equal(tt.wantControlPlaneUpgradePlan))
			g.Expect(s.UpgradeTracker.MachineDeployments.UpgradePlan).To(Equal(tt.wantMachineDeploymentUpgradePlan))
			g.Expect(s.UpgradeTracker.MachinePools.UpgradePlan).To(Equal(tt.wantMachinePoolUpgradePlan))
		})
	}
}

func TestGetUpgradePlanOneMinor(t *testing.T) {
	tests := []struct {
		name                        string
		desiredVersion              string
		currentControlPlaneVersion  string
		wantControlPlaneUpgradePlan []string
		wantWorkersUpgradePlan      []string
		wantErr                     bool
	}{
		{
			name:                        "return empty plans if everything is up to date",
			desiredVersion:              "v1.31.0",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: nil,
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "return control plane upgrade plan",
			desiredVersion:              "v1.32.0",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: []string{"v1.32.0"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "return control plane upgrade plan with pre-release version",
			desiredVersion:              "v1.32.0-alpha.2",
			currentControlPlaneVersion:  "v1.32.0-alpha.1",
			wantControlPlaneUpgradePlan: []string{"v1.32.0-alpha.2"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "return control plane upgrade plan with build tags",
			desiredVersion:              "v1.32.0+foo.2-bar.1",
			currentControlPlaneVersion:  "v1.32.0+foo.1-bar.1",
			wantControlPlaneUpgradePlan: []string{"v1.32.0+foo.2-bar.1"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "fails when upgrading by more than one version",
			desiredVersion:              "v1.33.0",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: nil,
			wantWorkersUpgradePlan:      nil,
			wantErr:                     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			controlPlaneUpgradePlan, workersUpgradePlan, err := GetUpgradePlanOneMinor(ctx, tt.desiredVersion, tt.currentControlPlaneVersion, "")
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(controlPlaneUpgradePlan).To(Equal(tt.wantControlPlaneUpgradePlan))
			g.Expect(workersUpgradePlan).To(Equal(tt.wantWorkersUpgradePlan))
		})
	}
}

func TestGetUpgradePlanFromClusterClassVersions(t *testing.T) {
	tests := []struct {
		name                        string
		desiredVersion              string
		clusterClassVersions        []string
		currentControlPlaneVersion  string
		wantControlPlaneUpgradePlan []string
		wantWorkersUpgradePlan      []string
		wantErr                     bool
	}{
		{
			name:                        "return empty plans if everything is up to date",
			clusterClassVersions:        []string{"v1.31.0"},
			desiredVersion:              "v1.31.0",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: nil,
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "return control plane upgrade plan for one minor",
			clusterClassVersions:        []string{"v1.31.0", "v1.32.0", "v1.32.1"},
			desiredVersion:              "v1.32.0",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: []string{"v1.32.0"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "return control plane upgrade plan for more than one minor",
			clusterClassVersions:        []string{"v1.31.0", "v1.32.0", "v1.33.0"},
			desiredVersion:              "v1.33.0",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: []string{"v1.32.0", "v1.33.0"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "pick latest for every minor",
			clusterClassVersions:        []string{"v1.31.0", "v1.31.1", "v1.32.0", "v1.32.2", "v1.32.3", "v1.33.0", "v1.33.1"},
			desiredVersion:              "v1.33.1",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: []string{"v1.32.3", "v1.33.1"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "pick latest for every minor with pro-release",
			clusterClassVersions:        []string{"v1.31.0", "v1.31.1", "v1.32.0-alpha.0", "v1.32.0-alpha.1", "v1.32.0-alpha.2", "v1.33.0", "v1.33.1"},
			desiredVersion:              "v1.33.1",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: []string{"v1.32.0-alpha.2", "v1.33.1"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "pick latest for every minor with build tags",
			clusterClassVersions:        []string{"v1.31.0", "v1.31.1", "v1.32.0+foo.1-bar.1", "v1.32.0+foo.2-bar.1", "v1.32.0+foo.2-bar.2", "v1.33.0", "v1.33.1"},
			desiredVersion:              "v1.33.1",
			currentControlPlaneVersion:  "v1.31.0",
			wantControlPlaneUpgradePlan: []string{"v1.32.0+foo.2-bar.2", "v1.33.1"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},

		// Kubernetes versions in CC is set when clusters already exists

		{
			name:                        "best effort control plane upgrade plan if current version is not on the list of Kubernetes versions (happy path, case 1)",
			clusterClassVersions:        []string{"v1.31.1", "v1.32.0", "v1.33.0"},
			desiredVersion:              "v1.33.0",
			currentControlPlaneVersion:  "v1.31.0", // v1.31.0 is not in the kubernetes versions, but it is older than the first version on the same minor
			wantControlPlaneUpgradePlan: []string{"v1.32.0", "v1.33.0"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "best effort control plane upgrade plan if current version is not on the list of Kubernetes versions (happy path, case 2)",
			clusterClassVersions:        []string{"v1.31.0", "v1.32.0", "v1.33.0"},
			desiredVersion:              "v1.33.0",
			currentControlPlaneVersion:  "v1.31.2", // v1.31.0 is not in the kubernetes versions, but it is newer than the first version on the same minor
			wantControlPlaneUpgradePlan: []string{"v1.32.0", "v1.33.0"},
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},
		{
			name:                        "best effort control plane upgrade plan if current version is not on the list of Kubernetes versions (not happy path)",
			clusterClassVersions:        []string{"v1.32.0", "v1.33.0"},
			desiredVersion:              "v1.33.0",
			currentControlPlaneVersion:  "v1.30.2",
			wantControlPlaneUpgradePlan: []string{"v1.32.0", "v1.33.0"}, // No version for 1.31, ComputeUpgradePlan will fail
			wantWorkersUpgradePlan:      nil,
			wantErr:                     false,
		},

		// TODO: Kubernetes versions in CC is set after setting target version (when an upgrade is in progress)

	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			f := GetUpgradePlanFromClusterClassVersions(tt.clusterClassVersions)
			controlPlaneUpgradePlan, workersUpgradePlan, err := f(nil, tt.desiredVersion, tt.currentControlPlaneVersion, "")
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(controlPlaneUpgradePlan).To(Equal(tt.wantControlPlaneUpgradePlan))
			g.Expect(workersUpgradePlan).To(Equal(tt.wantWorkersUpgradePlan))
		})
	}
}

func TestGetUpgradePlanFromExtension(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Enable RuntimeSDK feature flag
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	// Create a test cluster
	cluster := builder.Cluster("test-namespace", "test-cluster").Build()

	// Create catalog and register runtime hooks
	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)

	// Create fake runtime client
	extensionResponse := &runtimehooksv1.GenerateUpgradePlanResponse{
		CommonResponse: runtimehooksv1.CommonResponse{
			Status: runtimehooksv1.ResponseStatusSuccess,
		},
		ControlPlaneUpgrades: []runtimehooksv1.UpgradeStep{
			{Version: "v1.32.0"},
			{Version: "v1.33.0"},
		},
		WorkersUpgrades: []runtimehooksv1.UpgradeStep{
			{Version: "v1.33.0"},
		},
	}
	fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
		WithCatalog(catalog).
		WithCallExtensionResponses(map[string]runtimehooksv1.ResponseObject{
			"test-extension": extensionResponse,
		}).
		Build()

	// Call GetUpgradePlanFromExtension
	f := GetUpgradePlanFromExtension(fakeRuntimeClient, cluster, "test-extension")
	controlPlaneUpgradePlan, workersUpgradePlan, err := f(ctx, "v1.33.0", "v1.31.0", "v1.31.0")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(controlPlaneUpgradePlan).To(Equal([]string{"v1.32.0", "v1.33.0"}))
	g.Expect(workersUpgradePlan).To(Equal([]string{"v1.33.0"}))
}

func TestGetUpgradePlanFromExtension_Errors(t *testing.T) {
	tests := []struct {
		name                       string
		enableRuntimeSDK           bool
		extensionError             error
		desiredVersion             string
		currentControlPlaneVersion string
		currentMinWorkersVersion   string
		wantErrMessage             string
	}{
		{
			name:                       "fails when RuntimeSDK feature flag is disabled",
			enableRuntimeSDK:           false,
			desiredVersion:             "v1.33.0",
			currentControlPlaneVersion: "v1.31.0",
			currentMinWorkersVersion:   "v1.31.0",
			wantErrMessage:             "can not use GenerateUpgradePlan extension \"test-extension\" if RuntimeSDK feature flag is disabled",
		},
		{
			name:                       "fails when extension call returns error",
			enableRuntimeSDK:           true,
			desiredVersion:             "v1.33.0",
			currentControlPlaneVersion: "v1.31.0",
			currentMinWorkersVersion:   "v1.31.0",
			extensionError:             errors.New("extension call error"),
			wantErrMessage:             "failed to call GenerateUpgradePlan extension \"test-extension\": extension call error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			// Enable/disable RuntimeSDK feature flag
			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, tt.enableRuntimeSDK)

			// Create a test cluster
			cluster := builder.Cluster("test-namespace", "test-cluster").Build()

			// Create catalog and register runtime hooks
			catalog := runtimecatalog.New()
			_ = runtimehooksv1.AddToCatalog(catalog)

			// Create fake runtime client
			fakeRuntimeClientBuilder := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog)
			if tt.extensionError != nil {
				fakeRuntimeClientBuilder.WithCallExtensionValidations(func(_ string, _ runtimehooksv1.RequestObject) error {
					return tt.extensionError
				})
			}
			fakeRuntimeClient := fakeRuntimeClientBuilder.Build()

			// Call GetUpgradePlanFromExtension
			f := GetUpgradePlanFromExtension(fakeRuntimeClient, cluster, "test-extension")
			_, _, err := f(ctx, tt.desiredVersion, tt.currentControlPlaneVersion, tt.currentMinWorkersVersion)

			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(Equal(tt.wantErrMessage))
		})
	}
}
