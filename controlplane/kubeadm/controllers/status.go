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

	"sigs.k8s.io/cluster-api/util/collections"

	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
)

// updateStatus is called after every reconcilitation loop in a defer statement to always make sure we have the
// resource status subresourcs up-to-date.
func (r *KubeadmControlPlaneReconciler) updateStatus(ctx context.Context, kcp *controlplanev1.KubeadmControlPlane, cluster *clusterv1.Cluster) error {
	log := ctrl.LoggerFrom(ctx, "cluster", cluster.Name)

	selector := collections.ControlPlaneSelectorForCluster(cluster.Name)
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	kcp.Status.Selector = selector.String()

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, cluster, collections.OwnedMachines(kcp))
	if err != nil {
		return errors.Wrap(err, "failed to get list of owned machines")
	}

	controlPlane, err := internal.NewControlPlane(ctx, r.Client, cluster, kcp, ownedMachines)
	if err != nil {
		log.Error(err, "failed to initialize control plane")
		return err
	}
	kcp.Status.UpdatedReplicas = int32(len(controlPlane.UpToDateMachines()))

	replicas := int32(len(ownedMachines))
	desiredReplicas := *kcp.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	kcp.Status.Replicas = replicas
	kcp.Status.ReadyReplicas = 0
	kcp.Status.UnavailableReplicas = replicas

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !kcp.DeletionTimestamp.IsZero() {
		return nil
	}

	machinesWithHealthAPIServer := ownedMachines.Filter(collections.HealthyAPIServer())
	lowestVersion := machinesWithHealthAPIServer.LowestVersion()
	if lowestVersion != nil {
		kcp.Status.Version = lowestVersion
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		conditions.MarkFalse(kcp, controlplanev1.ResizedCondition, controlplanev1.ScalingUpReason, clusterv1.ConditionSeverityWarning, "Scaling up control plane to %d replicas (actual %d)", desiredReplicas, replicas)
	// We are scaling down
	case replicas > desiredReplicas:
		conditions.MarkFalse(kcp, controlplanev1.ResizedCondition, controlplanev1.ScalingDownReason, clusterv1.ConditionSeverityWarning, "Scaling down control plane to %d replicas (actual %d)", desiredReplicas, replicas)

		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(kcp, controlplanev1.MachinesCreatedCondition)
	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		readyMachines := ownedMachines.Filter(collections.IsReady())
		if int32(len(readyMachines)) == replicas {
			conditions.MarkTrue(kcp, controlplanev1.ResizedCondition)
		}

		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(kcp, controlplanev1.MachinesCreatedCondition)
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		return errors.Wrap(err, "failed to create remote cluster client")
	}
	status, err := workloadCluster.ClusterStatus(ctx)
	if err != nil {
		return err
	}
	kcp.Status.ReadyReplicas = status.ReadyNodes
	kcp.Status.UnavailableReplicas = replicas - status.ReadyNodes

	// This only gets initialized once and does not change if the kubeadm config map goes away.
	if status.HasKubeadmConfig {
		kcp.Status.Initialized = true
		conditions.MarkTrue(kcp, controlplanev1.AvailableCondition)
	}

	if kcp.Status.ReadyReplicas > 0 {
		kcp.Status.Ready = true
	}

	return nil
}
