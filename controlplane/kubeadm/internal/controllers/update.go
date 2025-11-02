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

package controllers

import (
	"context"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
)

func (r *KubeadmControlPlaneReconciler) updateControlPlane(
	ctx context.Context,
	controlPlane *internal.ControlPlane,
	machinesNeedingRollout collections.Machines,
	machinesUpToDateResults map[string]internal.UpToDateResult,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// TODO: handle reconciliation of etcd members and kubeadm config in case they get out of sync with cluster

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		log.Error(err, "failed to get remote client for workload cluster", "Cluster", klog.KObj(controlPlane.Cluster))
		return ctrl.Result{}, errors.Wrapf(err, "failed to update control plane")
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to update control plane: failed to parse Kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	// Ensure kubeadm clusterRoleBinding for v1.29+ as per https://github.com/kubernetes/kubernetes/pull/121305
	if err := workloadCluster.AllowClusterAdminPermissions(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update control plane: failed to set cluster-admin ClusterRoleBinding for kubeadm")
	}

	kubeadmCMMutators := make([]func(*bootstrapv1.ClusterConfiguration), 0)

	if controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.IsDefined() {
		// Get the imageRepository or the correct value if nothing is set and a migration is necessary.
		imageRepository := internal.ImageRepositoryFromClusterConfig(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration)

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
		return ctrl.Result{}, errors.Wrapf(err, "failed to update control plane")
	}

	switch controlPlane.KCP.Spec.Rollout.Strategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate
		res, err := r.rollingUpdate(ctx, controlPlane, machinesNeedingRollout, machinesUpToDateResults)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to update control plane")
		}
		return res, nil
	default:
		log.Info("RolloutStrategy type is not set to RollingUpdate, unable to determine the strategy for rolling out machines")
		return ctrl.Result{}, nil
	}
}

func (r *KubeadmControlPlaneReconciler) rollingUpdate(
	ctx context.Context,
	controlPlane *internal.ControlPlane,
	machinesNeedingRollout collections.Machines,
	machinesUpToDateResults map[string]internal.UpToDateResult,
) (ctrl.Result, error) {
	currentReplicas := int32(controlPlane.Machines.Len())
	currentUpToDateReplicas := int32(controlPlane.UpToDateMachines().Len())
	desiredReplicas := *controlPlane.KCP.Spec.Replicas
	maxSurge := int32(controlPlane.KCP.Spec.Rollout.Strategy.RollingUpdate.MaxSurge.IntValue())
	// Note: As MaxSurge is validated to be either 0 or 1, maxReplicas will be either desiredReplicas or desiredReplicas+1.
	maxReplicas := desiredReplicas + maxSurge

	// If currentReplicas < maxReplicas we have to scale up
	// Note: This is done to ensure we have as many Machines as allowed during rollout to maximize fault tolerance.
	if currentReplicas < maxReplicas {
		// Note: scaleUpControlPlane ensures that we don't continue scaling up while waiting for Machines to have NodeRefs.
		return r.scaleUpControlPlane(ctx, controlPlane)
	}

	// If currentReplicas >= maxReplicas we have to scale down.
	// Note: If we are already at or above the maximum Machines we have to in-place update or delete a Machine
	// to make progress with the update (as we cannot create additional new Machines above the maximum).

	// Pick the Machine that we should in-place update or scale down.
	machineToInPlaceUpdateOrScaleDown, err := selectMachineForInPlaceUpdateOrScaleDown(ctx, controlPlane, machinesNeedingRollout)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to select next Machine for rollout")
	}
	machineUpToDateResult, ok := machinesUpToDateResults[machineToInPlaceUpdateOrScaleDown.Name]
	if !ok {
		// Note: This should never happen as we store results for all Machines in machinesUpToDateResults.
		return ctrl.Result{}, errors.Errorf("failed to check if Machine %s is UpToDate", machineToInPlaceUpdateOrScaleDown.Name)
	}

	// If the selected Machine is eligible for in-place update and we don't already have enough up-to-date replicas, try in-place update.
	// Note: To be safe we only try an in-place update when we would otherwise delete a Machine. This ensures we could
	// afford if the in-place update fails and the Machine becomes unavailable (and eventually MHC kicks in and the Machine is recreated).
	if feature.Gates.Enabled(feature.InPlaceUpdates) &&
		machineUpToDateResult.EligibleForInPlaceUpdate &&
		currentUpToDateReplicas < desiredReplicas {
		fallbackToScaleDown, res, err := r.tryInPlaceUpdate(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown, machineUpToDateResult)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !res.IsZero() {
			return res, nil
		}
		if fallbackToScaleDown {
			return r.scaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown)
		}
		// In-place update triggered
		return ctrl.Result{}, nil // Note: Requeue is not needed, changes to Machines trigger another reconcile.
	}
	return r.scaleDownControlPlane(ctx, controlPlane, machineToInPlaceUpdateOrScaleDown)
}
