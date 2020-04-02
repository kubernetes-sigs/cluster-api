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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/machinefilters"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *KubeadmControlPlaneReconciler) initializeControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	bootstrapSpec := controlPlane.InitialControlPlaneConfig()
	fd := controlPlane.FailureDomainWithFewestMachines()
	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create initial control plane Machine")
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedInitialization", "Failed to create initial control plane Machine for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *KubeadmControlPlaneReconciler) scaleUpControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KubeadmControlPlane, _ internal.FilterableMachineCollection, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	if err := r.reconcileHealth(ctx, cluster, kcp, controlPlane); err != nil {
		return ctrl.Result{}, &capierrors.RequeueAfterError{RequeueAfter: healthCheckFailedRequeueAfter}
	}

	// Create the bootstrap configuration
	bootstrapSpec := controlPlane.JoinControlPlaneConfig()
	fd := controlPlane.FailureDomainWithFewestMachines()
	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create additional control plane Machine")
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedScaleUp", "Failed to create additional control plane Machine for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are other operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *KubeadmControlPlaneReconciler) scaleDownControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.KubeadmControlPlane,
	ownedMachines internal.FilterableMachineCollection,
	selectedMachines internal.FilterableMachineCollection,
	controlPlane *internal.ControlPlane,
) (ctrl.Result, error) {
	logger := controlPlane.Logger()

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "Failed to create client to workload cluster")
		return ctrl.Result{}, errors.Wrapf(err, "failed to create client to workload cluster")
	}

	// Wait for any delete in progress to complete before deleting another Machine
	if controlPlane.HasDeletingMachine() {
		return ctrl.Result{}, &capierrors.RequeueAfterError{RequeueAfter: deleteRequeueAfter}
	}

	if err := r.reconcileHealth(ctx, cluster, kcp, controlPlane); err != nil {
		return ctrl.Result{}, &capierrors.RequeueAfterError{RequeueAfter: healthCheckFailedRequeueAfter}
	}

	// If there is not already a Machine that is marked for scale down, find one and mark it
	markedForDeletion := selectedMachines.Filter(machinefilters.HasAnnotationKey(controlplanev1.DeleteForScaleDownAnnotation))
	if len(markedForDeletion) == 0 {
		machineToMark, err := r.selectAndMarkMachine(ctx, selectedMachines, controlplanev1.DeleteForScaleDownAnnotation, controlPlane)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to select and mark machine for scale down")
		}
		markedForDeletion.Insert(machineToMark)
	}

	machineToDelete := markedForDeletion.Oldest()
	if machineToDelete == nil {
		logger.Info("Failed to pick control plane Machine to delete")
		return ctrl.Result{}, errors.New("failed to pick control plane Machine to delete")
	}

	// If etcd leadership is on machine that is about to be deleted, move it to the newest member available.
	etcdLeaderCandidate := ownedMachines.Newest()
	if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToDelete, etcdLeaderCandidate); err != nil {
		logger.Error(err, "Failed to move leadership to candidate machine", "candidate", etcdLeaderCandidate.Name)
		return ctrl.Result{}, err
	}
	if err := workloadCluster.RemoveEtcdMemberForMachine(ctx, machineToDelete); err != nil {
		logger.Error(err, "Failed to remove etcd member for machine")
		return ctrl.Result{}, err
	}

	if !machinefilters.HasAnnotationKey(controlplanev1.ScaleDownConfigMapEntryRemovedAnnotation)(machineToDelete) {
		if err := r.managementCluster.TargetClusterControlPlaneIsHealthy(ctx, util.ObjectKey(cluster), kcp.Name); err != nil {
			logger.V(2).Info("Waiting for control plane to pass control plane health check before removing a control plane machine", "cause", err)
			r.recorder.Eventf(kcp, corev1.EventTypeWarning, "ControlPlaneUnhealthy",
				"Waiting for control plane to pass control plane health check before removing a control plane machine: %v", err)
			return ctrl.Result{}, &capierrors.RequeueAfterError{RequeueAfter: healthCheckFailedRequeueAfter}

		}
		if err := workloadCluster.RemoveMachineFromKubeadmConfigMap(ctx, machineToDelete); err != nil {
			logger.Error(err, "Failed to remove machine from kubeadm ConfigMap")
			return ctrl.Result{}, err
		}
		if err := r.markWithAnnotationKey(ctx, machineToDelete, controlplanev1.ScaleDownConfigMapEntryRemovedAnnotation); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to mark machine %s as having config map entry removed", machineToDelete.Name)
		}
	}

	logger = logger.WithValues("machine", machineToDelete)
	if err := r.Client.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to delete control plane machine")
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedScaleDown",
			"Failed to delete control plane Machine %s for cluster %s/%s control plane: %v", machineToDelete.Name, cluster.Namespace, cluster.Name, err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}
