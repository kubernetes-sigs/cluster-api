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
	"sigs.k8s.io/cluster-api/util/collections"
)

func (r *KubeadmControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	controlPlane *internal.ControlPlane,
	machinesRequireUpgrade collections.Machines,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	// TODO: handle reconciliation of etcd members and kubeadm config in case they get out of sync with cluster

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		logger.Error(err, "failed to get remote client for workload cluster", "Cluster", klog.KObj(controlPlane.Cluster))
		return ctrl.Result{}, err
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	// Ensure kubeadm clusterRoleBinding for v1.29+ as per https://github.com/kubernetes/kubernetes/pull/121305
	if err := workloadCluster.AllowClusterAdminPermissions(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to set cluster-admin ClusterRoleBinding for kubeadm")
	}

	kubeadmCMMutators := make([]func(*bootstrapv1.ClusterConfiguration), 0)

	if controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration != nil {
		// Get the imageRepository or the correct value if nothing is set and a migration is necessary.
		imageRepository := internal.ImageRepositoryFromClusterConfig(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration)

		kubeadmCMMutators = append(kubeadmCMMutators,
			workloadCluster.UpdateImageRepositoryInKubeadmConfigMap(imageRepository),
			workloadCluster.UpdateFeatureGatesInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec, parsedVersion),
			workloadCluster.UpdateAPIServerInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer),
			workloadCluster.UpdateControllerManagerInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager),
			workloadCluster.UpdateSchedulerInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Scheduler),
			workloadCluster.UpdateCertificateValidityPeriodDays(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.CertificateValidityPeriodDays))

		// Etcd local and external are mutually exclusive and they cannot be switched, once set.
		if controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local != nil {
			kubeadmCMMutators = append(kubeadmCMMutators,
				workloadCluster.UpdateEtcdLocalInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local))
		} else {
			kubeadmCMMutators = append(kubeadmCMMutators,
				workloadCluster.UpdateEtcdExternalInKubeadmConfigMap(controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External))
		}
	}

	// collectively update Kubeadm config map
	if err = workloadCluster.UpdateClusterConfiguration(ctx, parsedVersion, kubeadmCMMutators...); err != nil {
		return ctrl.Result{}, err
	}

	switch controlPlane.KCP.Spec.Rollout.Strategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate
		// We can ignore MaxUnavailable because we are enforcing health checks before we get here.
		maxNodes := *controlPlane.KCP.Spec.Replicas + int32(controlPlane.KCP.Spec.Rollout.Strategy.RollingUpdate.MaxSurge.IntValue())
		if int32(controlPlane.Machines.Len()) < maxNodes {
			// scaleUp ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
			return r.scaleUpControlPlane(ctx, controlPlane)
		}
		return r.scaleDownControlPlane(ctx, controlPlane, machinesRequireUpgrade)
	default:
		logger.Info("RolloutStrategy type is not set to RollingUpdate, unable to determine the strategy for rolling out machines")
		return ctrl.Result{}, nil
	}
}
