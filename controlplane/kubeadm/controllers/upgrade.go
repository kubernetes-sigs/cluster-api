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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *KubeadmControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.KubeadmControlPlane,
	ownedMachines internal.FilterableMachineCollection,
	requireUpgrade internal.FilterableMachineCollection,
	controlPlane *internal.ControlPlane,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

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

	if err := workloadCluster.UpdateKubernetesVersionInKubeadmConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update the kubernetes version in the kubeadm config map")
	}

	if kcp.Spec.KubeadmConfigSpec.ClusterConfiguration != nil && kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local != nil {
		meta := kcp.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.Local.ImageMeta
		if err := workloadCluster.UpdateEtcdVersionInKubeadmConfigMap(ctx, meta.ImageRepository, meta.ImageTag); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the etcd version in the kubeadm config map")
		}
	}

	if err := workloadCluster.UpdateKubeletConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to upgrade kubelet config map")
	}

	// If there is not already a Machine that is marked for upgrade, find one and mark it
	selectedForUpgrade := requireUpgrade.Filter(machinefilters.HasAnnotationKey(controlplanev1.SelectedForUpgradeAnnotation))
	if len(selectedForUpgrade) == 0 {
		selectedMachine, err := r.selectMachineForUpgrade(ctx, cluster, requireUpgrade, controlPlane)
		if err != nil {
			logger.Error(err, "failed to select machine for upgrade")
			return ctrl.Result{}, err
		}
		selectedForUpgrade = selectedForUpgrade.Insert(selectedMachine)
	}

	replacementCreated := selectedForUpgrade.Filter(machinefilters.HasAnnotationKey(controlplanev1.UpgradeReplacementCreatedAnnotation))
	if len(replacementCreated) == 0 {
		// TODO: should we also add a check here to ensure that current machines not > kcp.spec.replicas+1?
		// We haven't created a replacement machine for the cluster yet
		// return here to avoid blocking while waiting for the new control plane Machine to come up
		result, err := r.scaleUpControlPlane(ctx, cluster, kcp, ownedMachines, controlPlane)
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := r.markWithAnnotationKey(ctx, selectedForUpgrade.Oldest(), controlplanev1.UpgradeReplacementCreatedAnnotation); err != nil {
			return ctrl.Result{}, err
		}
		return result, nil
	}

	return r.scaleDownControlPlane(ctx, cluster, kcp, ownedMachines, replacementCreated, controlPlane)
}

func (r *KubeadmControlPlaneReconciler) selectMachineForUpgrade(ctx context.Context, _ *clusterv1.Cluster, requireUpgrade internal.FilterableMachineCollection, controlPlane *internal.ControlPlane) (*clusterv1.Machine, error) {
	failureDomain := controlPlane.FailureDomainWithMostMachines(requireUpgrade)

	inFailureDomain := requireUpgrade.Filter(machinefilters.InFailureDomains(failureDomain))
	selected := inFailureDomain.Oldest()

	if err := r.markWithAnnotationKey(ctx, selected, controlplanev1.SelectedForUpgradeAnnotation); err != nil {
		return nil, errors.Wrap(err, "failed to select and mark a machine for upgrade")
	}

	return selected, nil
}
