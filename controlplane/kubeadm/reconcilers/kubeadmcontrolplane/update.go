/*
Copyright 2020 The Kubernetes Authors.

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

package kubeadmcontrolplane

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	pkgerrors "github.com/pkg/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
)

func (r *Reconciler) updateControlPlane(
	ctx context.Context,
	controlPlane *pkg.ControlPlane,
	machinesNeedingRollout collections.Machines,
	machinesUpToDateResults map[string]pkg.UpToDateResult,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// TODO: handle reconciliation of etcd members and kubeadm config in case they get out of sync with cluster

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		log.Error(err, "failed to get remote client for workload cluster", "Cluster", klog.KObj(controlPlane.Cluster))
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to update control plane")
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to update control plane: failed to parse Kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	// Creates ClusterRoleBinding and ClusterRoles introduced by new versions of kubeadm.
	if err := workloadCluster.EnsureKubeadmPermissions(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "failed to update control plane: failed to set cluster-admin ClusterRoleBinding for kubeadm")
	}

	kubeadmCMMutators := make([]func(*bootstrapv1.ClusterConfiguration), 0)

	if controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.IsDefined() {
		// Get the imageRepository or the correct value if nothing is set and a migration is necessary.
		imageRepository := pkg.ImageRepositoryFromClusterConfig(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration)

		kubeadmCMMutators = append(kubeadmCMMutators,
			workloadCluster.UpdateImageRepositoryInKubeadmConfigMap(imageRepository),
			workloadCluster.UpdateFeatureGatesInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec, parsedVersion),
			workloadCluster.UpdateAPIServerInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer),
			workloadCluster.UpdateControllerManagerInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager),
			workloadCluster.UpdateSchedulerInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Scheduler),
			workloadCluster.UpdateCertificateValidityPeriodDays(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.CertificateValidityPeriodDays),
			workloadCluster.UpdateEncryptionAlgorithm(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.EncryptionAlgorithm))

		// Etcd local and external are mutually exclusive and they cannot be switched, once set.
		if controlPlane.IsEtcdManaged() {
			kubeadmCMMutators = append(kubeadmCMMutators,
				workloadCluster.UpdateEtcdLocalInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local))
		} else {
			kubeadmCMMutators = append(kubeadmCMMutators,
				workloadCluster.UpdateEtcdExternalInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External))
		}
	}

	// collectively update Kubeadm config map
	if err = workloadCluster.UpdateClusterConfiguration(ctx, parsedVersion, kubeadmCMMutators...); err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to update control plane")
	}

	switch controlPlane.KCP.Spec.Rollout.Strategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate
		res, err := r.rollingUpdate(ctx, controlPlane, machinesNeedingRollout, machinesUpToDateResults)
		if err != nil {
			return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to update control plane")
		}
		return res, nil
	default:
		log.Info("RolloutStrategy type is not set to RollingUpdate, unable to determine the strategy for rolling out machines")
		return ctrl.Result{}, nil
	}
}

func (r *Reconciler) rollingUpdate(
	ctx context.Context,
	controlPlane *pkg.ControlPlane,
	machinesNeedingRollout collections.Machines,
	machinesUpToDateResults map[string]pkg.UpToDateResult,
) (ctrl.Result, error) {
	currentReplicas := int32(controlPlane.Machines.Len())
	currentUpToDateReplicas := int32(controlPlane.UpToDateMachines().Len())
	desiredReplicas := *controlPlane.KCP.Spec.Replicas

	// Pick the Machine that should be in-place updated or scaled down.
	// Note: rollingUpdate is only called if len(machinesNeedingRollout) > 0.
	machineToInPlaceUpdateOrScaleDown, err := selectMachineForInPlaceUpdateOrScaleDown(ctx, controlPlane, machinesNeedingRollout)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrap(err, "failed to select next Machine for rollout")
	}
	machineUpToDateResult, ok := machinesUpToDateResults[machineToInPlaceUpdateOrScaleDown.Name]
	if !ok {
		// Note: This should never happen as we store results for all Machines in machinesUpToDateResults.
		return ctrl.Result{}, pkgerrors.Errorf("failed to check if Machine %s is UpToDate", machineToInPlaceUpdateOrScaleDown.Name)
	}

	// There are already enough up-to-date replicas, so let's delete the remaining not up-to-date replicas.
	if currentUpToDateReplicas >= desiredReplicas {
		return r.scaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, true)
	}

	// Always scale up after we deleted an unhealthy Machine for remediation.
	// If the remediation annotation is set here we have to create a new Machine to complete the
	// remediation process (delete+create) before continuing with the rollout process.
	if _, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		return r.scaleUpControlPlane(ctx, controlPlane, true)
	}

	// As MaxSurge is validated to be either 0 or 1:
	// * minReplicas will be either desiredReplicas-1 or desiredReplicas.
	// * maxReplicas will be either desiredReplicas or desiredReplicas+1.
	// So overall [min,max] will be:
	// * maxSurge: 0 => [desiredReplicas-1,desiredReplicas]
	// * maxSurge: 1 => [desiredReplicas,desiredReplicas+1]
	maxSurge := int32(controlPlane.KCP.Spec.Rollout.Strategy.RollingUpdate.MaxSurge.IntValue())
	minReplicas := desiredReplicas + maxSurge - 1
	maxReplicas := desiredReplicas + maxSurge

	// After we handled the cases above the priorities are now:
	// * Get into or stay within the [min,max] range
	// * Prefer in-place updates over scale up/down
	//
	// Example 1: spec.replicas = 3, maxSurge: 1, minReplicas: 3 maxReplicas: 4
	// case                           | currentReplicas | action
	// currentReplicas < minReplicas  | <3              | scale up
	// currentReplicas == minReplicas | 3               | in-place update or fallback to scale up (in-place update only if affectsAvailability:false)
	// currentReplicas == maxReplicas | 4               | in-place update or fallback to scale down
	// currentReplicas > maxReplicas  | >4              | scale down
	//
	// Example 2: spec.replicas = 3, maxSurge: 0, minReplicas: 2 maxReplicas: 3
	// case                           | currentReplicas | action
	// currentReplicas < minReplicas  | <2              | scale up
	// currentReplicas == minReplicas | 2               | scale up (we are below spec.replicas so we can scale up, which is safer than in-place update)
	// currentReplicas == maxReplicas | 3               | in-place update or fallback to scale down
	// currentReplicas > maxReplicas  | >3              | scale down
	switch {
	case currentReplicas < minReplicas:
		// If currentReplicas < minReplicas, we have to scale up.
		return r.scaleUpControlPlane(ctx, controlPlane, true)
	case currentReplicas == minReplicas && currentReplicas < desiredReplicas:
		// If currentReplicas == minReplicas && currentReplicas < desiredReplicas, we have to scale up.
		return r.scaleUpControlPlane(ctx, controlPlane, true)
	case currentReplicas == minReplicas:
		// If currentReplicas == minReplicas, we can try in-place update if it does not affect availability, otherwise we scale up.
		return r.inPlaceUpdateOrScaleUpControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, machineUpToDateResult)
	case currentReplicas == maxReplicas:
		// If currentReplicas == maxReplicas, we can try in-place update, otherwise we scale down.
		return r.inPlaceUpdateOrScaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, machineUpToDateResult)
	case currentReplicas > maxReplicas:
		// If currentReplicas > maxReplicas, we have to scale down.
		return r.scaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, true)
	}

	return ctrl.Result{}, fmt.Errorf("unexpected state, currentReplicas %d, minReplicas %d, maxReplicas %d", currentReplicas, minReplicas, maxReplicas)
}

func (r *Reconciler) inPlaceUpdateOrScaleUpControlPlane(ctx context.Context, controlPlane *pkg.ControlPlane, machineToInPlaceUpdate *clusterv1.Machine, machineUpToDateResult pkg.UpToDateResult) (ctrl.Result, error) {
	// If either the InPlaceUpdates feature is not enabled or the Machine is not eligible for in-place update, scale up.
	if !feature.Gates.Enabled(feature.InPlaceUpdates) || !machineUpToDateResult.EligibleForInPlaceUpdate {
		return r.scaleUpControlPlane(ctx, controlPlane, true)
	}

	// Now we know that the feature gate is enabled and the Machine is eligible for in-place update, so let's try in-place update.

	// Run preflight checks to ensure that the control plane is stable before proceeding with the in-place update.
	//
	// Important! preflight checks play an important role in ensuring that KCP performs "one operation at time", by forcing
	// the system to wait for the previous operation to complete and the control plane to become stable before starting the next one.
	//
	// Note: before considering in-place updates, KCP first takes care of completing
	// ongoing delete operations, completing in-place transitions, remediating unhealthy machines.
	if resultForAllMachines := r.preflightChecks(ctx, controlPlane, false); !resultForAllMachines.succeeded {
		return r.handlePreflightCheckResults(ctx, controlPlane, resultForAllMachines), nil
	}

	// Note: Usually canUpdateMachine is only called once for a single Machine rollout.
	// If it returns true, the code below will mark the in-place update as in progress via
	// UpdateInProgressAnnotation. From this point forward we are not going to call canUpdateMachine again.
	// If it returns false, we are going to fall back to scale up which will create another Machine.
	// This will usually lead to a deletion of machineToInPlaceUpdate in the next step and a second call
	// to canUpdateMachine as part of the scale down for this Machine.
	// We only have to repeat the canUpdateMachine call as part of a scale up if the write call to set UpdateInProgressAnnotation
	// fails or if we fail to create the Machine.
	canUpdateMachineResult, err := r.canUpdateMachine(ctx, machineToInPlaceUpdate, machineUpToDateResult)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to determine if Machine %s can be updated in-place", machineToInPlaceUpdate.Name)
	}

	if canUpdateMachineResult.canUpdateMachine && !canUpdateMachineResult.affectsAvailability {
		// Note: Requeue is not needed, changes to Machines trigger another reconcile.
		return ctrl.Result{}, r.triggerInPlaceUpdate(ctx, controlPlane, machineToInPlaceUpdate, machineUpToDateResult, canUpdateMachineResult.affectsAvailability)
	}

	// Note: No need to run preflightChecks again, they already succeeded.
	return r.scaleUpControlPlane(ctx, controlPlane, false)
}

func (r *Reconciler) inPlaceUpdateOrScaleDownControlPlane(ctx context.Context, controlPlane *pkg.ControlPlane, machineToInPlaceUpdateOrScaleDown *clusterv1.Machine, machineUpToDateResult pkg.UpToDateResult) (ctrl.Result, error) {
	// If either the InPlaceUpdates feature is not enabled or the Machine is not eligible for in-place update, scale down.
	if !feature.Gates.Enabled(feature.InPlaceUpdates) || !machineUpToDateResult.EligibleForInPlaceUpdate {
		return r.scaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, true)
	}

	// Now we know that the feature gate is enabled and the Machine is eligible for in-place update, so let's try in-place update.

	// Run preflight checks to ensure that the control plane is stable before proceeding with the in-place update.
	//
	// Important! preflight checks play an important role in ensuring that KCP performs "one operation at time", by forcing
	// the system to wait for the previous operation to complete and the control plane to become stable before starting the next one.
	//
	// Note: before considering in-place updates, KCP first takes care of completing
	// ongoing delete operations, completing in-place transitions, remediating unhealthy machines.
	if resultForAllMachines := r.preflightChecks(ctx, controlPlane, false); !resultForAllMachines.succeeded {
		// If the control plane is not stable, check if the issues are only for machineToInPlaceUpdateOrScaleDown.
		if result := r.preflightChecks(ctx, controlPlane, false, machineToInPlaceUpdateOrScaleDown); result.succeeded {
			// The issues are only for machineToInPlaceUpdateOrScaleDown, fallback to scale down.
			// Note: The consequence of this is that a Machine with issues is scaled down and not in-place updated.
			// Note: No need to run preflightChecks again, they already succeeded.
			return r.scaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, false)
		}

		// The preflight checks also fail when excluding machineToInPlaceUpdateOrScaleDown, return.
		return r.handlePreflightCheckResults(ctx, controlPlane, resultForAllMachines), nil
	}

	// Note: Usually canUpdateMachine is only called once for a single Machine rollout.
	// If it returns true, the code below will mark the in-place update as in progress via
	// UpdateInProgressAnnotation. From this point forward we are not going to call canUpdateMachine again.
	// If it returns false, we are going to fall back to scale down which will delete the Machine.
	// We only have to repeat the canUpdateMachine call if the write call to set UpdateInProgressAnnotation
	// fails or if we fail to delete the Machine.
	canUpdateMachineResult, err := r.canUpdateMachine(ctx, machineToInPlaceUpdateOrScaleDown, machineUpToDateResult)
	if err != nil {
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to determine if Machine %s can be updated in-place", machineToInPlaceUpdateOrScaleDown.Name)
	}

	if canUpdateMachineResult.canUpdateMachine {
		// Note: Requeue is not needed, changes to Machines trigger another reconcile.
		return ctrl.Result{}, r.triggerInPlaceUpdate(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, machineUpToDateResult, canUpdateMachineResult.affectsAvailability)
	}

	// Note: No need to run preflightChecks again, they already succeeded.
	return r.scaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, false)
}
