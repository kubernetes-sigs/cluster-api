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
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/version"
)

// ComputeUpgradePlan is responsible to computes the upgrade plan for both control plane and workers
// and to set up the upgrade tracker accordingly when there is an upgrade pending.
//
// The upgrade plan for control plane is the result of a pluggable function that should return all the
// intermediates version a control plan upgrade must go through to reach desired version.
//
// The pluggable function could return also upgrade steps for workers; if not, this func
// will determine the minimal number of workers upgrade steps, thus minimizing impact on workloads and reducing the overall upgrade time.
func ComputeUpgradePlan(ctx context.Context, s *scope.Scope, getUpgradePlan GetUpgradePlanFunc) error {
	// Return early if control plane is not yet created.
	if s.Current.ControlPlane == nil || s.Current.ControlPlane.Object == nil {
		return nil
	}

	// Get desired version, control plane versions and min worker versions
	// NOTE: we consider both machine deployment and machine pools min for computing workers version
	//  because we are going to ask only a single workers upgrade plan.
	desiredVersion := s.Blueprint.Topology.Version
	desiredSemVer, err := semver.ParseTolerant(desiredVersion)
	if err != nil {
		return errors.Wrapf(err, "failed to parse Cluster version %s", desiredVersion)
	}

	controlPlaneVersion := ""
	v, err := contract.ControlPlane().Version().Get(s.Current.ControlPlane.Object)
	if err != nil {
		return errors.Wrap(err, "failed to get the version from control plane spec")
	}
	controlPlaneVersion = *v
	controlPlaneSemVer, err := semver.ParseTolerant(*v)
	if err != nil {
		return errors.Wrapf(err, "failed to parse ControlPlane version %s", *v)
	}

	var minWorkersSemVer *semver.Version
	var minMachineDeploymentsSemVer *semver.Version
	for _, md := range s.Current.MachineDeployments {
		if md.Object.Spec.Template.Spec.Version != "" {
			currentSemVer, err := semver.ParseTolerant(md.Object.Spec.Template.Spec.Version)
			if err != nil {
				return errors.Wrapf(err, "failed to parse version %s of MachineDeployment %s", md.Object.Spec.Template.Spec.Version, md.Object.Name)
			}
			if minMachineDeploymentsSemVer == nil || isLowerThanMinVersion(currentSemVer, *minMachineDeploymentsSemVer, controlPlaneSemVer) {
				minMachineDeploymentsSemVer = &currentSemVer
			}
			if minWorkersSemVer == nil || isLowerThanMinVersion(currentSemVer, *minWorkersSemVer, controlPlaneSemVer) {
				minWorkersSemVer = &currentSemVer
			}
		}
	}

	var minMachinePoolsSemVer *semver.Version
	for _, mp := range s.Current.MachinePools {
		if mp.Object.Spec.Template.Spec.Version != "" {
			currentSemVer, err := semver.ParseTolerant(mp.Object.Spec.Template.Spec.Version)
			if err != nil {
				return errors.Wrapf(err, "failed to parse version %s of MachinePool %s", mp.Object.Spec.Template.Spec.Version, mp.Object.Name)
			}
			if minMachinePoolsSemVer == nil || isLowerThanMinVersion(currentSemVer, *minMachinePoolsSemVer, controlPlaneSemVer) {
				minMachinePoolsSemVer = &currentSemVer
			}
			if minWorkersSemVer == nil || isLowerThanMinVersion(currentSemVer, *minWorkersSemVer, controlPlaneSemVer) {
				minWorkersSemVer = &currentSemVer
			}
		}
	}

	minWorkersVersion := ""
	if minWorkersSemVer != nil {
		minWorkersVersion = fmt.Sprintf("v%s", minWorkersSemVer.String())
	}

	// If both control plane and workers are already at the desired version, there is no need to compute the upgrade plan.
	if controlPlaneSemVer.String() == desiredSemVer.String() && (minWorkersSemVer == nil || minWorkersSemVer.String() == desiredSemVer.String()) {
		return nil
	}

	// At this stage we know that an upgrade is required, then call the pluggable func that returns the upgrade plan.
	controlPlaneUpgradePlan, workersUpgradePlan, err := getUpgradePlan(ctx, desiredVersion, controlPlaneVersion, minWorkersVersion)
	if err != nil {
		return err
	}

	// DefaultAndValidateUpgradePlans validates both control plane and workers upgrade plan.
	// If workers upgrade plan is not specified, default it with the minimal number of workers upgrade steps.
	workersUpgradePlan, err = DefaultAndValidateUpgradePlans(desiredVersion, controlPlaneVersion, minWorkersVersion, controlPlaneUpgradePlan, workersUpgradePlan)
	if err != nil {
		return err
	}

	// Sets the control plane upgrade plan.
	s.UpgradeTracker.ControlPlane.UpgradePlan = controlPlaneUpgradePlan

	// Sets the machine deployment and workers upgrade plan.
	s.UpgradeTracker.MachineDeployments.UpgradePlan = nil
	s.UpgradeTracker.MachinePools.UpgradePlan = nil

	for _, targetVersion := range workersUpgradePlan {
		targetSemVer, err := semver.ParseTolerant(targetVersion)
		if err != nil {
			// Note: this should never happen, all the versions have been validated above.
			return errors.Wrapf(err, "invalid upgrade plan; failed to parse version %s from workers upgrade plan", targetVersion)
		}

		// If there are machineDeployments (minMachineDeploymentsSemVer != nil), the workers upgrade plan apply,
		// but we should drop the first step all the machineDeployments have been already upgraded.
		if minMachineDeploymentsSemVer != nil {
			if targetSemVer.String() != minMachineDeploymentsSemVer.String() {
				s.UpgradeTracker.MachineDeployments.UpgradePlan = append(s.UpgradeTracker.MachineDeployments.UpgradePlan, targetVersion)
			}
		}

		// If there are machinePools (minMachinePoolSemVer != nil), the workers upgrade plan apply,
		// but we should drop the first step all the machinePools have been already upgraded.
		if minMachinePoolsSemVer != nil {
			if targetSemVer.String() != minMachinePoolsSemVer.String() {
				s.UpgradeTracker.MachinePools.UpgradePlan = append(s.UpgradeTracker.MachinePools.UpgradePlan, targetVersion)
			}
		}
	}

	return nil
}

// DefaultAndValidateUpgradePlans validates both control plane and workers upgrade plan.
// If workers upgrade plan is not specified, default it with the minimal number of workers upgrade steps.
func DefaultAndValidateUpgradePlans(desiredVersion string, controlPlaneVersion string, minWorkersVersion string, controlPlaneUpgradePlan []string, workersUpgradePlan []string) ([]string, error) {
	desiredSemVer, err := semver.ParseTolerant(desiredVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse Cluster version %s", desiredVersion)
	}

	controlPlaneSemVer, err := semver.ParseTolerant(controlPlaneVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse ControlPlane version %s", controlPlaneVersion)
	}

	var minWorkersSemVer *semver.Version
	if minWorkersVersion != "" {
		v, err := semver.ParseTolerant(minWorkersVersion)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse min workers version %s", minWorkersVersion)
		}
		minWorkersSemVer = &v
	}

	// Setup for tracking known version for each minors; this info will be used to build intermediate steps for workers when required
	// Note: The control plane might be already one version ahead of workers, we always add current control plane version
	// (it should be used as a target version for workers lagging behind).
	minors := map[uint64]string{}
	minors[controlPlaneSemVer.Minor] = controlPlaneVersion

	// Setup for tracking version order, which is required for disambiguating where there are version with different build numbers
	// and thus it is not possible to determine order (and thus the code relies on the version order in the upgrade plan).
	versionOrder := map[string]int{}

	// Validate the control plane upgrade plan.
	if version.Compare(controlPlaneSemVer, desiredSemVer, version.WithBuildTags()) != 0 {
		currentSemVer := controlPlaneSemVer
		for i, targetVersion := range controlPlaneUpgradePlan {
			versionOrder[targetVersion] = i
			targetSemVer, err := semver.ParseTolerant(targetVersion)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid ControlPlane upgrade plan: item %d; failed to parse version %s", i, targetVersion)
			}

			// Check versions in the control plane upgrade plan are in the right order.
			// Note: we tolerate having one version followed by another with the same major.minor.patch but different build tags (version.Compare==2)
			if version.Compare(targetSemVer, currentSemVer, version.WithBuildTags()) <= 0 {
				return nil, errors.Errorf("invalid ControlPlane upgrade plan: item %d; version %s must be greater than v%s", i, targetVersion, currentSemVer)
			}

			// Check we are not skipping minors.
			if currentSemVer.Minor != targetSemVer.Minor && currentSemVer.Minor+1 != targetSemVer.Minor {
				return nil, errors.Errorf("invalid ControlPlane upgrade plan: item %d; expecting a version with minor %d or %d, found version %s", i, currentSemVer.Minor, currentSemVer.Minor+1, targetVersion)
			}

			minors[targetSemVer.Minor] = targetVersion
			currentSemVer = targetSemVer
		}
		if version.Compare(currentSemVer, desiredSemVer, version.WithBuildTags()) != 0 {
			return nil, errors.Errorf("invalid ControlPlane upgrade plan: item %d; control plane upgrade plan must end with version %s, found %s", len(controlPlaneUpgradePlan)-1, desiredVersion, fmt.Sprintf("v%s", currentSemVer))
		}
	} else if len(controlPlaneUpgradePlan) > 0 {
		return nil, errors.New("invalid ControlPlane upgrade plan: control plane is already at the desired version")
	}

	// Defaults and validate the workers upgrade plan.
	if minWorkersSemVer != nil && version.Compare(*minWorkersSemVer, desiredSemVer, version.WithBuildTags()) != 0 {
		if len(controlPlaneUpgradePlan) > 0 {
			// Check that the workers upgrade plan only includes the same versions considered for the control plane upgrade plan,
			// plus the control plane version to handle the case that CP already completed its upgrade.
			if diff := sets.New(workersUpgradePlan...).Difference(sets.New(controlPlaneUpgradePlan...).Insert(controlPlaneVersion)); len(diff) > 0 {
				return nil, errors.Errorf("invalid workers upgrade plan: versions %s doesn't match any versions in the control plane upgrade plan nor the control plane version", strings.Join(diff.UnsortedList(), ","))
			}
		}

		// If the workers upgrade plan is empty, default it by adding:
		// - upgrade steps whenever required to prevent violation of version skew rules
		// - an upgrade step at the end of the upgrade sequence
		if len(workersUpgradePlan) == 0 {
			currentMinor := minWorkersSemVer.Minor
			targetMinor := desiredSemVer.Minor
			for i := range targetMinor - currentMinor {
				if i > 0 && i%3 == 0 {
					targetVersion, ok := minors[currentMinor+i]
					if !ok {
						// Note: this should never happen, all the versions in the workers upgrade plan should exist in versionOrder, which is
						// derived from control plane upgrade plan + current control plane version (a superset of the versions in the workers upgrade plan)
						return nil, errors.Wrapf(err, "invalid upgrade plan; unable to identify version for minor %d", currentMinor+i)
					}
					workersUpgradePlan = append(workersUpgradePlan, targetVersion)
				}
			}
			if len(workersUpgradePlan) == 0 || workersUpgradePlan[len(workersUpgradePlan)-1] != desiredVersion {
				workersUpgradePlan = append(workersUpgradePlan, desiredVersion)
			}
		}

		// Validate the workers upgrade plan.
		currentSemVer := *minWorkersSemVer
		currentMinor := currentSemVer.Minor
		for i, targetVersion := range workersUpgradePlan {
			targetSemVer, err := semver.ParseTolerant(targetVersion)
			if err != nil {
				return nil, errors.Wrapf(err, "invalid workers upgrade plan, item %d; failed to parse version %s", i, targetVersion)
			}

			// Check versions in the workers upgrade plan are in the right order.
			cmp := version.Compare(targetSemVer, currentSemVer, version.WithBuildTags())
			switch {
			case cmp <= 0:
				return nil, errors.Errorf("invalid workers upgrade plan, item %d; version %s must be greater than v%s", i, targetVersion, currentSemVer)
			case cmp == 2:
				// In the case of same major.minor.patch but different build tags (version.Compare==2), check if
				// versions are in the same order than in the control plane upgrade plan.
				targetVersionOrder, ok := versionOrder[targetVersion]
				if !ok {
					// Note: this should never happen, all the versions in the workers upgrade plan should exist in versionOrder, which is
					// derived from control plane upgrade plan + current control plane version (a superset of the versions in the workers upgrade plan)
					return nil, errors.Errorf("invalid workers upgrade plan, item %d; failer to determine version %s order", i, targetVersion)
				}
				currentVersionOrder, ok := versionOrder[fmt.Sprintf("v%s", currentSemVer)]
				if !ok {
					// Note: this should never happen, all the versions in the workers upgrade plan should exist in versionOrder, which is
					// derived from control plane upgrade plan + current control plane version (a superset of the versions in the workers upgrade plan)
					return nil, errors.Errorf("failer to determine version v%s order", currentSemVer)
				}
				if targetVersionOrder < currentVersionOrder {
					return nil, errors.Errorf("invalid workers upgrade plan, item %d; version %s must be after v%s", i, targetVersion, currentSemVer)
				}
			}

			targetMinor := targetSemVer.Minor
			if targetMinor-currentMinor > 3 {
				return nil, errors.Errorf("invalid workers upgrade plan, item %d; workers cannot go from minor %d to minor %d, an intermediate step is required to comply with Kubernetes version skew rules", i, currentMinor, targetMinor)
			}

			currentSemVer = targetSemVer
			currentMinor = currentSemVer.Minor
		}
		if version.Compare(currentSemVer, desiredSemVer, version.WithBuildTags()) != 0 {
			return nil, errors.Errorf("invalid workers upgrade plan, item %d; workers upgrade plan must end with version %s, found %s", len(workersUpgradePlan)-1, desiredVersion, fmt.Sprintf("v%s", currentSemVer))
		}
	} else if len(workersUpgradePlan) > 0 {
		return nil, errors.New("invalid worker upgrade plan; there are no workers or workers already at the desired version")
	}

	return workersUpgradePlan, nil
}

func isLowerThanMinVersion(v, minVersion, controlPlaneSemVer semver.Version) bool {
	switch cmp := version.Compare(v, minVersion, version.WithBuildTags()); cmp {
	case -1:
		// v is lower than minVersion
		return true
	case 2:
		// v is different from minVersion, but it is not po cannot determine order;
		// use control plane version to resolve: MD/MP version can either be equal to control plane version or the older version before the upgrade,
		// so v is considered lower than minVersion when different from control plane version.
		return v.String() != controlPlaneSemVer.String()
	default:
		return false
	}
}

// GetUpgradePlanFunc defines the signature for a func that returns the upgrade plan for control plane and workers.
//
// The upgrade plan for control plane must be a list of intermediate version the control plane must go through
// to reach the desired version. The following rules apply:
// - there should be at least one version for every minor between currentControlPlaneVersion (excluded) and desiredVersion (included).
// - each version must be:
//   - greater than currentControlPlaneVersion (or with a different build number)
//   - greater than the previous version in the list (or with a different build number)
//   - less or equal to desiredVersion (or with a different build number)
//   - the last version in the plan must be equal to the desired version
//
// The upgrade plan for workers instead in most cases could be left to empty, because the system will automatically
// determine the minimal number of workers upgrade steps, thus minimizing impact on workloads and reducing
// the overall upgrade time.
//
// If instead for any reason the GetUpgradePlanFunc returns a custom upgrade path for workers, the following rules apply:
// - each version must be:
//   - equal to currentControlPlaneVersion or to one of the versions in the control plane upgrade plan.
//   - greater than current min worker - MachineDeployment & MachinePool - version (or with a different build number)
//   - greater than the previous version in the list (or with a different build number)
//   - less or equal to the desiredVersion (or with a different build number)
//   - in case of versions with the same major/minor/patch version but different build number, also the order
//     of those versions must be the same for control plane and worker upgrade plan.
//   - the last version in the plan must be equal to the desired version
//   - the upgrade plane must have all the intermediate version which workers must go through to avoid breaking rules
//     defining the max version skew between control plane and workers.
type GetUpgradePlanFunc func(_ context.Context, desiredVersion, currentControlPlaneVersion, currentMinWorkersVersion string) ([]string, []string, error)

// GetUpgradePlanOneMinor returns an upgrade plan to reach the next minor.
// The workers upgrade plan will be left empty, thus deferring to ComputeUpgradePlan to compute it.
// NOTE: This is the func the system is going to use when there are no Kubernetes versions or UpgradePlan hook
// defined in the ClusterClass. In this scenario, only upgrade by one minor is supported (same as before implementing chained upgrades).
func GetUpgradePlanOneMinor(_ context.Context, desiredVersion, currentControlPlaneVersion, _ string) ([]string, []string, error) {
	desiredSemVer, err := semver.ParseTolerant(desiredVersion)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse desired version")
	}

	currentControlPlaneSemVer, err := semver.ParseTolerant(currentControlPlaneVersion)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse current ControlPlane version")
	}

	if currentControlPlaneSemVer.String() == desiredSemVer.String() {
		return nil, nil, nil
	}

	if desiredSemVer.Minor > currentControlPlaneSemVer.Minor+1 {
		return nil, nil, errors.Errorf("cannot compute an upgrade plan from %s to %s", currentControlPlaneVersion, desiredVersion)
	}

	return []string{desiredVersion}, nil, nil
}

// GetUpgradePlanFromClusterClassVersions returns an upgrade plan based on versions defined on a ClusterClass.
// The control plane plan will use the latest version for each minor in between currentControlPlaneVersion and desiredVersion;
// workers upgrade plan will be left empty, thus deferring to ComputeUpgradePlan to compute the most efficient plan.
// NOTE: This is the func the system is going to use when there are Kubernetes versions defined in the ClusterClass.
func GetUpgradePlanFromClusterClassVersions(clusterClassVersions []string) func(_ context.Context, desiredVersion, currentControlPlaneVersion, _ string) ([]string, []string, error) {
	return func(_ context.Context, desiredVersion, currentControlPlaneVersion, _ string) ([]string, []string, error) {
		desiredSemVer, err := semver.ParseTolerant(desiredVersion)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to parse desired version")
		}

		currentControlPlaneSemVer, err := semver.ParseTolerant(currentControlPlaneVersion)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to parse current ControlPlane version")
		}

		if currentControlPlaneSemVer.String() == desiredSemVer.String() {
			return nil, nil, nil
		}

		// Pick all the known kubernetes versions starting from control plane version (excluded) to desired version.
		upgradePlan := []string{}
		start := false
		end := false
		for _, v := range clusterClassVersions {
			semV, err := semver.ParseTolerant(v)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to parse version %s", v)
			}
			if (start && !end) || (!start && semV.Minor > currentControlPlaneSemVer.Minor) {
				upgradePlan = append(upgradePlan, v)
			}
			if semV.String() == currentControlPlaneSemVer.String() || version.Compare(currentControlPlaneSemVer, semV, version.WithBuildTags()) < 0 {
				start = true
			}
			if semV.String() == desiredSemVer.String() || version.Compare(desiredSemVer, semV, version.WithBuildTags()) < 0 {
				end = true
			}
		}

		// In case there is more than one version for one minor, drop all the versions for one minor except the last.
		simplifiedUpgradePlan := []string{}
		currentMinor := currentControlPlaneSemVer.Minor
		for _, v := range upgradePlan {
			semV, err := semver.ParseTolerant(v)
			if err != nil {
				return nil, nil, errors.Wrapf(err, "failed to parse version %s", v)
			}
			if semV.Minor > currentMinor {
				simplifiedUpgradePlan = append(simplifiedUpgradePlan, v)
			}
			if semV.Minor == currentMinor && len(simplifiedUpgradePlan) > 0 {
				simplifiedUpgradePlan[len(simplifiedUpgradePlan)-1] = v
			}
			currentMinor = semV.Minor
		}
		return simplifiedUpgradePlan, nil, nil
	}
}
