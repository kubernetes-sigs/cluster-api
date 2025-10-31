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
	"slices"
	"strings"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/internal/hooks"
)

// callBeforeClusterUpgradeHook calls the BeforeClusterUpgrade at the beginning of an upgrade.
// NOTE: the hook should be called only at the beginning of an upgrade sequence (it should not be called when in the middle of a multistep upgrade sequence);
// to detect if we are at the beginning of an upgrade, the code checks if the intent to call the AfterClusterUpgrade is not yet tracked.
func (g *generator) callBeforeClusterUpgradeHook(ctx context.Context, s *scope.Scope, currentVersion *string, topologyVersion string) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	if !hooks.IsPending(runtimehooksv1.AfterClusterUpgrade, s.Current.Cluster) {
		var hookAnnotations []string
		for key := range s.Current.Cluster.Annotations {
			if strings.HasPrefix(key, clusterv1.BeforeClusterUpgradeHookAnnotationPrefix) {
				hookAnnotations = append(hookAnnotations, key)
			}
		}
		if len(hookAnnotations) > 0 {
			slices.Sort(hookAnnotations)
			message := fmt.Sprintf("annotations [%s] are set", strings.Join(hookAnnotations, ", "))
			if len(hookAnnotations) == 1 {
				message = fmt.Sprintf("annotation [%s] is set", strings.Join(hookAnnotations, ", "))
			}
			// Add the hook with a response to the tracker so we can later update the condition.
			s.HookResponseTracker.Add(runtimehooksv1.BeforeClusterUpgrade, &runtimehooksv1.BeforeClusterUpgradeResponse{
				CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
					// RetryAfterSeconds needs to be set because having only hooks without RetryAfterSeconds
					// would lead to not updating the condition. We can rely on getting an event when the
					// annotation gets removed so we set twice of the default sync-period to not cause additional reconciles.
					RetryAfterSeconds: 20 * 60,
					CommonResponse: runtimehooksv1.CommonResponse{
						Message: message,
					},
				},
			})

			log.Info(fmt.Sprintf("Cluster upgrade from version %s to version %s is blocked by %s hook (via annotations)", *currentVersion, topologyVersion, runtimecatalog.HookName(runtimehooksv1.BeforeClusterUpgrade)), "hooks", strings.Join(hookAnnotations, ","),
				"ControlPlaneUpgrades", toUpgradeStep(s.UpgradeTracker.ControlPlane.UpgradePlan),
				"WorkersUpgrades", toUpgradeStep(s.UpgradeTracker.MachineDeployments.UpgradePlan, s.UpgradeTracker.MachinePools.UpgradePlan),
			)
			return false, nil
		}

		v1beta1Cluster := &clusterv1beta1.Cluster{}
		// DeepCopy cluster because ConvertFrom has side effects like adding the conversion annotation.
		if err := v1beta1Cluster.ConvertFrom(s.Current.Cluster.DeepCopy()); err != nil {
			return false, errors.Wrap(err, "error converting Cluster to v1beta1 Cluster")
		}

		hookRequest := &runtimehooksv1.BeforeClusterUpgradeRequest{
			Cluster:               *cleanupCluster(v1beta1Cluster),
			FromKubernetesVersion: *currentVersion,
			ToKubernetesVersion:   topologyVersion,
			ControlPlaneUpgrades:  toUpgradeStep(s.UpgradeTracker.ControlPlane.UpgradePlan),
			WorkersUpgrades:       toUpgradeStep(s.UpgradeTracker.MachineDeployments.UpgradePlan, s.UpgradeTracker.MachinePools.UpgradePlan),
		}
		hookResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{}
		if err := g.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.BeforeClusterUpgrade, s.Current.Cluster, hookRequest, hookResponse); err != nil {
			return false, err
		}
		// Add the response to the tracker so we can later update condition or requeue when required.
		s.HookResponseTracker.Add(runtimehooksv1.BeforeClusterUpgrade, hookResponse)

		if hookResponse.RetryAfterSeconds != 0 {
			// Cannot pickup the new version right now. Need to try again later.
			log.Info(fmt.Sprintf("Cluster upgrade from version %s to version %s is blocked by %s hook", *currentVersion, topologyVersion, runtimecatalog.HookName(runtimehooksv1.BeforeClusterUpgrade)),
				"ControlPlaneUpgrades", hookRequest.ControlPlaneUpgrades,
				"WorkersUpgrades", hookRequest.WorkersUpgrades,
			)
			return false, nil
		}
	}
	return true, nil
}

// callBeforeControlPlaneUpgradeHook calls the BeforeControlPlaneUpgrade before the control plane picks up a new control plane version,
// no matter if this is an intermediate versions of an upgrade plan or the target version of an upgrade plan.
// NOTE: when an upgrade starts, the hook should be called after the BeforeClusterUpgrade hook.
// NOTE: the hook doesn't need call intent tracking: it is always called before picking up a new control plane version.
func (g *generator) callBeforeControlPlaneUpgradeHook(ctx context.Context, s *scope.Scope, currentVersion *string, nextVersion string) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// NOTE: the hook should always be called before piking up a new version.
	v1beta1Cluster := &clusterv1beta1.Cluster{}
	// DeepCopy cluster because ConvertFrom has side effects like adding the conversion annotation.
	if err := v1beta1Cluster.ConvertFrom(s.Current.Cluster.DeepCopy()); err != nil {
		return false, errors.Wrap(err, "error converting Cluster to v1beta1 Cluster")
	}

	hookRequest := &runtimehooksv1.BeforeControlPlaneUpgradeRequest{
		Cluster:               *cleanupCluster(v1beta1Cluster),
		FromKubernetesVersion: *currentVersion,
		ToKubernetesVersion:   nextVersion,
		ControlPlaneUpgrades:  toUpgradeStep(s.UpgradeTracker.ControlPlane.UpgradePlan),
		WorkersUpgrades:       toUpgradeStep(s.UpgradeTracker.MachineDeployments.UpgradePlan, s.UpgradeTracker.MachinePools.UpgradePlan),
	}
	hookResponse := &runtimehooksv1.BeforeControlPlaneUpgradeResponse{}
	if err := g.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.BeforeControlPlaneUpgrade, s.Current.Cluster, hookRequest, hookResponse); err != nil {
		return false, err
	}
	// Add the response to the tracker so we can later update condition or requeue when required.
	s.HookResponseTracker.Add(runtimehooksv1.BeforeControlPlaneUpgrade, hookResponse)

	if hookResponse.RetryAfterSeconds != 0 {
		// Cannot pickup the new version right now. Need to try again later.
		log.Info(fmt.Sprintf("Control plane upgrade from version %s to version %s is blocked by %s hook", *currentVersion, nextVersion, runtimecatalog.HookName(runtimehooksv1.BeforeControlPlaneUpgrade)),
			"ControlPlaneUpgrades", hookRequest.ControlPlaneUpgrades,
			"WorkersUpgrades", hookRequest.WorkersUpgrades,
		)
		return false, nil
	}

	return true, nil
}

// callAfterControlPlaneUpgradeHook calls the AfterControlPlaneUpgrade after the control plane upgrade is completed,
// no matter if this is an intermediate versions of an upgrade plan or the target version of an upgrade plan.
// NOTE: computeControlPlaneVersion records intent to call this hook when picking up a new control plane version.
func (g *generator) callAfterControlPlaneUpgradeHook(ctx context.Context, s *scope.Scope, currentVersion *string) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Call the hook only if we are tracking the intent to do so. If it is not tracked it means we don't need to call the
	// hook because we didn't go through an upgrade or we already called the hook after the upgrade.
	if hooks.IsPending(runtimehooksv1.AfterControlPlaneUpgrade, s.Current.Cluster) {
		v1beta1Cluster := &clusterv1beta1.Cluster{}
		// DeepCopy cluster because ConvertFrom has side effects like adding the conversion annotation.
		if err := v1beta1Cluster.ConvertFrom(s.Current.Cluster.DeepCopy()); err != nil {
			return false, errors.Wrap(err, "error converting Cluster to v1beta1 Cluster")
		}

		// Call all the registered extension for the hook.
		hookRequest := &runtimehooksv1.AfterControlPlaneUpgradeRequest{
			Cluster:              *cleanupCluster(v1beta1Cluster),
			KubernetesVersion:    *currentVersion,
			ControlPlaneUpgrades: toUpgradeStep(s.UpgradeTracker.ControlPlane.UpgradePlan),
			WorkersUpgrades:      toUpgradeStep(s.UpgradeTracker.MachineDeployments.UpgradePlan, s.UpgradeTracker.MachinePools.UpgradePlan),
		}
		hookResponse := &runtimehooksv1.AfterControlPlaneUpgradeResponse{}
		if err := g.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.AfterControlPlaneUpgrade, s.Current.Cluster, hookRequest, hookResponse); err != nil {
			return false, err
		}
		// Add the response to the tracker so we can later update condition or requeue when required.
		s.HookResponseTracker.Add(runtimehooksv1.AfterControlPlaneUpgrade, hookResponse)

		if hookResponse.RetryAfterSeconds != 0 {
			log.Info(fmt.Sprintf("Cluster Upgrade is blocked after control plane upgrade to version %s by %s hook", *currentVersion, runtimecatalog.HookName(runtimehooksv1.AfterControlPlaneUpgrade)),
				"ControlPlaneUpgrades", hookRequest.ControlPlaneUpgrades,
				"WorkersUpgrades", hookRequest.WorkersUpgrades,
			)
			return false, nil
		}
		if err := hooks.MarkAsDone(ctx, g.Client, s.Current.Cluster, runtimehooksv1.AfterControlPlaneUpgrade); err != nil {
			return false, err
		}
	}
	return true, nil
}

// callBeforeWorkersUpgradeHook calls the BeforeWorkersUpgrade before workers starts picking up a new worker version,
// no matter if this is an intermediate versions of an upgrade plan or the target version of an upgrade plan.
// NOTE: computeControlPlaneVersion records intent to call this hook when picking up a new control plane version
// that exists also in the workers upgrade plan.
func (g *generator) callBeforeWorkersUpgradeHook(ctx context.Context, s *scope.Scope, currentVersion *string, nextVersion string) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Call the hook only if we are tracking the intent to do so. If it is not tracked it means we don't need to call the
	// hook because we didn't go through an upgrade or we already called the hook after the upgrade.
	if hooks.IsPending(runtimehooksv1.BeforeWorkersUpgrade, s.Current.Cluster) {
		v1beta1Cluster := &clusterv1beta1.Cluster{}
		// DeepCopy cluster because ConvertFrom has side effects like adding the conversion annotation.
		if err := v1beta1Cluster.ConvertFrom(s.Current.Cluster.DeepCopy()); err != nil {
			return false, errors.Wrap(err, "error converting Cluster to v1beta1 Cluster")
		}

		hookRequest := &runtimehooksv1.BeforeWorkersUpgradeRequest{
			Cluster:               *cleanupCluster(v1beta1Cluster),
			FromKubernetesVersion: *currentVersion,
			ToKubernetesVersion:   nextVersion,
			ControlPlaneUpgrades:  toUpgradeStep(s.UpgradeTracker.ControlPlane.UpgradePlan),
			WorkersUpgrades:       toUpgradeStep(s.UpgradeTracker.MachineDeployments.UpgradePlan, s.UpgradeTracker.MachinePools.UpgradePlan),
		}
		hookResponse := &runtimehooksv1.BeforeWorkersUpgradeResponse{}
		if err := g.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.BeforeWorkersUpgrade, s.Current.Cluster, hookRequest, hookResponse); err != nil {
			return false, err
		}
		// Add the response to the tracker so we can later update condition or requeue when required.
		s.HookResponseTracker.Add(runtimehooksv1.BeforeWorkersUpgrade, hookResponse)

		if hookResponse.RetryAfterSeconds != 0 {
			// Cannot pickup the new version right now. Need to try again later.
			log.Info(fmt.Sprintf("Workers upgrade from version %s to version %s is blocked by %s hook", *currentVersion, nextVersion, runtimecatalog.HookName(runtimehooksv1.BeforeWorkersUpgrade)),
				"ControlPlaneUpgrades", hookRequest.ControlPlaneUpgrades,
				"WorkersUpgrades", hookRequest.WorkersUpgrades,
			)
			return false, nil
		}
		if err := hooks.MarkAsDone(ctx, g.Client, s.Current.Cluster, runtimehooksv1.BeforeWorkersUpgrade); err != nil {
			return false, err
		}
	}

	return true, nil
}

// callAfterWorkersUpgradeHook calls the AfterWorkersUpgrade after the worker upgrade is completed,
// no matter if this is an intermediate versions of an upgrade plan or the target version of an upgrade plan.
// NOTE: computeControlPlaneVersion records intent to call this hook when picking up a new control plane version
// that exists also in the workers upgrade plan.
func (g *generator) callAfterWorkersUpgradeHook(ctx context.Context, s *scope.Scope, currentVersion *string) (bool, error) {
	log := ctrl.LoggerFrom(ctx)

	// Call the hook only if we are tracking the intent to do so. If it is not tracked it means we don't need to call the
	// hook because we didn't go through an upgrade or we already called the hook after the upgrade.
	if hooks.IsPending(runtimehooksv1.AfterWorkersUpgrade, s.Current.Cluster) {
		v1beta1Cluster := &clusterv1beta1.Cluster{}
		// DeepCopy cluster because ConvertFrom has side effects like adding the conversion annotation.
		if err := v1beta1Cluster.ConvertFrom(s.Current.Cluster.DeepCopy()); err != nil {
			return false, errors.Wrap(err, "error converting Cluster to v1beta1 Cluster")
		}

		// Call all the registered extension for the hook.
		hookRequest := &runtimehooksv1.AfterWorkersUpgradeRequest{
			Cluster:              *cleanupCluster(v1beta1Cluster),
			KubernetesVersion:    *currentVersion,
			ControlPlaneUpgrades: toUpgradeStep(s.UpgradeTracker.ControlPlane.UpgradePlan),
			WorkersUpgrades:      toUpgradeStep(s.UpgradeTracker.MachineDeployments.UpgradePlan, s.UpgradeTracker.MachinePools.UpgradePlan),
		}
		hookResponse := &runtimehooksv1.AfterWorkersUpgradeResponse{}
		if err := g.RuntimeClient.CallAllExtensions(ctx, runtimehooksv1.AfterWorkersUpgrade, s.Current.Cluster, hookRequest, hookResponse); err != nil {
			return false, err
		}
		// Add the response to the tracker so we can later update condition or requeue when required.
		s.HookResponseTracker.Add(runtimehooksv1.AfterWorkersUpgrade, hookResponse)

		if hookResponse.RetryAfterSeconds != 0 {
			log.Info(fmt.Sprintf("Cluster upgrade is blocked after workers upgrade to version %s by %s hook", *currentVersion, runtimecatalog.HookName(runtimehooksv1.AfterWorkersUpgrade)),
				"ControlPlaneUpgrades", hookRequest.ControlPlaneUpgrades,
				"WorkersUpgrades", hookRequest.WorkersUpgrades,
			)
			return false, nil
		}
		if err := hooks.MarkAsDone(ctx, g.Client, s.Current.Cluster, runtimehooksv1.AfterWorkersUpgrade); err != nil {
			return false, err
		}
	}
	return true, nil
}

// toUpgradeStep converts a list of version to a list of upgrade steps.
// Note. when called for workers, the function will receive in input two plans one for the MachineDeployments if any, the other for MachinePools if any.
// Considering that both plans, if defined, have to be equal, the function picks the first one not empty.
func toUpgradeStep(plans ...[]string) []runtimehooksv1.UpgradeStepInfo {
	var steps []runtimehooksv1.UpgradeStepInfo
	for _, plan := range plans {
		if len(plan) != 0 {
			for _, step := range plan {
				steps = append(steps, runtimehooksv1.UpgradeStepInfo{Version: step})
			}
			break
		}
	}
	return steps
}
