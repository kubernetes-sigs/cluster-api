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

	"github.com/blang/semver"
	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *KubeadmControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.KubeadmControlPlane,
	controlPlane *internal.ControlPlane,
	machinesRequireUpgrade collections.Machines,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	if kcp.Spec.RolloutStrategy == nil || kcp.Spec.RolloutStrategy.RollingUpdate == nil {
		return ctrl.Result{}, errors.New("rolloutStrategy is not set")
	}

	// TODO: handle reconciliation of etcd members and kubeadm config in case they get out of sync with cluster

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))
		return ctrl.Result{}, err
	}

	parsedVersion, err := semver.ParseTolerant(kcp.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kcp.Spec.Version)
	}

	if err := workloadCluster.ReconcileKubeletRBACRole(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile the remote kubelet RBAC role")
	}

	if err := workloadCluster.ReconcileKubeletRBACBinding(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile the remote kubelet RBAC binding")
	}

	// Ensure kubeadm cluster role  & bindings for v1.18+
	// as per https://github.com/kubernetes/kubernetes/commit/b117a928a6c3f650931bdac02a41fca6680548c4
	if err := workloadCluster.AllowBootstrapTokensToGetNodes(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to set role and role binding for kubeadm")
	}

	if err := workloadCluster.UpdateKubernetesVersionInKubeadmConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update the kubernetes version in the kubeadm config map")
	}

	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil {
		imageRepository := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ImageRepository
		if err := workloadCluster.UpdateImageRepositoryInKubeadmConfigMap(ctx, imageRepository, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the image repository in the kubeadm config map")
		}
	}

	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil && kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local != nil {
		meta := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ImageMeta
		if err := workloadCluster.UpdateEtcdVersionInKubeadmConfigMap(ctx, meta.ImageRepository, meta.ImageTag, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the etcd version in the kubeadm config map")
		}
	}

	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil {
		if err := workloadCluster.UpdateAPIServerInKubeadmConfigMap(ctx, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.APIServer, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update api server in the kubeadm config map")
		}

		if err := workloadCluster.UpdateControllerManagerInKubeadmConfigMap(ctx, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.ControllerManager, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update controller manager in the kubeadm config map")
		}

		if err := workloadCluster.UpdateSchedulerInKubeadmConfigMap(ctx, kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Scheduler, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update scheduler in the kubeadm config map")
		}
	}

	if err := workloadCluster.UpdateKubeletConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to upgrade kubelet config map")
	}

	switch kcp.Spec.RolloutStrategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate
		// We can ignore MaxUnavailable because we are enforcing health checks before we get here.
		maxNodes := *kcp.Spec.Replicas + int32(kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntValue())
		if int32(controlPlane.Machines.Len()) < maxNodes {
			// scaleUp ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
			return r.scaleUpControlPlane(ctx, cluster, kcp, controlPlane)
		}
		return r.scaleDownControlPlane(ctx, cluster, kcp, controlPlane, machinesRequireUpgrade)
	default:
		logger.Info("RolloutStrategy type is not set to RollingUpdateStrategyType, unable to determine the strategy for rolling out machines")
		return ctrl.Result{}, nil
	}
}
