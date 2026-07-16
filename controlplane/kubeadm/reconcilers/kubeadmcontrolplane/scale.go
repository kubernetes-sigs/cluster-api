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

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg"
	"sigs.k8s.io/cluster-api/util/collections"
	capicontrollerutil "sigs.k8s.io/cluster-api/util/controller"
)

func (r *Reconciler) initializeControlPlane(ctx context.Context, controlPlane *pkg.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	fd, err := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	newMachine, err := r.cloneConfigsAndGenerateMachine(ctx, controlPlane.Cluster, controlPlane.KCP, false, fd)
	if err != nil {
		log.Error(err, "Failed to create initial control plane Machine")
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedInitialization", "Failed to create initial control plane Machine for cluster %s control plane: %v", klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}

	log.WithValues(controlPlane.StatusToLogKeyAndValues(newMachine, nil)...).
		Info(fmt.Sprintf("Machine %s created (init)", klog.KObj(newMachine)),
			"Machine", klog.KObj(newMachine),
			newMachine.Spec.InfrastructureRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.InfrastructureRef.Name),
			newMachine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.Bootstrap.ConfigRef.Name), "desiredReplicas", ptr.Deref(controlPlane.KCP.Spec.Replicas, 0), "replicas", len(controlPlane.Machines))

	return ctrl.Result{}, nil // No need to requeue here. Machine creation above triggers reconciliation.
}

func (r *Reconciler) scaleUpControlPlane(ctx context.Context, controlPlane *pkg.ControlPlane) (ctrl.Result, error) {
	if r.overrideScaleUpControlPlaneFunc != nil {
		return r.overrideScaleUpControlPlaneFunc(ctx, controlPlane)
	}

	log := ctrl.LoggerFrom(ctx)

	// Run preflight checks to ensure that the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	//
	// Important! preflight checks play an important role in ensuring that KCP performs "one operation at time", by forcing
	// the system to wait for the previous operation to complete and the control plane to become stable before starting the next one.
	//
	// Note: before considering scale up/scale up in the context of a rollout/scale up after a remediation, KCP first takes care of completing
	// ongoing delete operations, completing in-place transitions, remediating unhealthy machines and completing on going in-place updates.
	if result := r.preflightChecks(ctx, controlPlane, true); !result.IsZero() {
		return result, nil
	}

	fd, err := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	newMachine, err := r.cloneConfigsAndGenerateMachine(ctx, controlPlane.Cluster, controlPlane.KCP, true, fd)
	if err != nil {
		log.Error(err, "Failed to create additional control plane Machine")
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedScaleUp", "Failed to create additional control plane Machine for cluster %s: %v", klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}

	log.WithValues(controlPlane.StatusToLogKeyAndValues(newMachine, nil)...).
		Info(fmt.Sprintf("Machine %s created (scale up)", klog.KObj(newMachine)),
			"Machine", klog.KObj(newMachine),
			newMachine.Spec.InfrastructureRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.InfrastructureRef.Name),
			newMachine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.Bootstrap.ConfigRef.Name), "desiredReplicas", ptr.Deref(controlPlane.KCP.Spec.Replicas, 0), "replicas", len(controlPlane.Machines))

	return ctrl.Result{}, nil // No need to requeue here. Machine creation above triggers reconciliation.
}

func (r *Reconciler) scaleDownControlPlane(
	ctx context.Context,
	controlPlane *pkg.ControlPlane,
	machineToDelete *clusterv1.Machine,
) (ctrl.Result, error) {
	if r.overrideScaleDownControlPlaneFunc != nil {
		return r.overrideScaleDownControlPlaneFunc(ctx, controlPlane, machineToDelete)
	}

	log := ctrl.LoggerFrom(ctx)

	// Run preflight checks ensuring the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	//
	// Important! preflight checks play an important role in ensuring that KCP performs "one operation at time", by forcing
	// the system to wait for the previous operation to complete and the control plane to become stable before starting the next one.
	//
	// Note: before considering scale down/scale down in the context of a rollout, KCP first takes care of completing
	// ongoing delete operations, completing in-place transitions, remediating unhealthy machines and completing on going in-place updates.
	//
	// Given that we're scaling down, we can exclude the machineToDelete from the preflight checks.
	if result := r.preflightChecks(ctx, controlPlane, false, machineToDelete); !result.IsZero() {
		return result, nil
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		log.Error(err, "Failed to create client to workload cluster")
		return ctrl.Result{}, errors.Wrapf(err, "failed to create client to workload cluster")
	}

	if machineToDelete == nil {
		log.Info("Failed to pick control plane Machine to delete")
		return ctrl.Result{}, errors.New("failed to pick control plane Machine to delete")
	}

	// If KCP should manage etcd, If etcd leadership is on machine that is about to be deleted, move it to the newest member available.
	if controlPlane.IsEtcdManaged() {
		// We cannot perform any etcd operation without a list of nodes.
		if controlPlane.NodeListError != nil {
			return ctrl.Result{}, errors.Wrap(controlPlane.NodeListError, "unable to forward etcd leadership")
		}

		if err := r.forwardEtcdLeadership(ctx, workloadCluster, controlPlane, machineToDelete); err != nil {
			return ctrl.Result{}, err
		}

		// NOTE: etcd member removal will be performed by the kcp-cleanup hook after machine completes drain & all volumes are detached.
	}

	if deletedMachine, err := r.machineClientWithDeleteResponse.Delete(ctx, machineToDelete); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to delete control plane machine")
			r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedScaleDown",
				"Failed to delete control plane Machine %s for cluster %s control plane: %v", machineToDelete.Name, klog.KObj(controlPlane.Cluster), err)
			return ctrl.Result{}, err
		}
	} else if deletedMachine != nil {
		r.controller.DeferNextReconcileUntilCacheUpToDate(controlPlane.KCP, capicontrollerutil.StructuredObject(clusterv1.GroupVersion, "Machine"), deletedMachine.GetResourceVersion())
	}

	// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
	// Also, setting DeletionTimestamp doesn't mean the Machine is actually deleted (deletion takes some time).
	log.WithValues(controlPlane.StatusToLogKeyAndValues(nil, machineToDelete)...).
		Info(fmt.Sprintf("Machine %s deleting (scale down)", klog.KObj(machineToDelete)), "Machine", klog.KObj(machineToDelete), "desiredReplicas", ptr.Deref(controlPlane.KCP.Spec.Replicas, 0), "replicas", len(controlPlane.Machines))

	return ctrl.Result{}, nil // No need to requeue here. Machine deletion above triggers reconciliation.
}

// selectMachineForInPlaceUpdateOrScaleDown select a machine candidate for scaling down or for in-place update.
// The selection is a two phase process:
//
// In the first phase it selects a subset of machines eligible for deletion:
// - if there are outdated machines with the delete machine annotation, use them as eligible subset (priority to user requests, part 1)
// - if there are machines (also not outdated) with the delete machine annotation, use them (priority to user requests, part 2)
// - if there are outdated machines with unhealthy control plane components, use them (priority to restore control plane health)
// - if there are outdated machines  consider all the outdated machines as eligible subset (rollout)
// - otherwise consider all the machines
//
// Once the subset of machines eligible for deletion is identified, one machine is picked out of this subset by
// selecting the machine in the failure domain with most machines (including both eligible and not eligible machines).
func selectMachineForInPlaceUpdateOrScaleDown(ctx context.Context, controlPlane *pkg.ControlPlane, outdatedMachines collections.Machines) (*clusterv1.Machine, error) {
	// Select the subset of machines eligible for scale down.
	var eligibleMachines collections.Machines
	switch {
	case controlPlane.MachineWithDeleteAnnotation(outdatedMachines).Len() > 0:
		eligibleMachines = controlPlane.MachineWithDeleteAnnotation(outdatedMachines)
	case controlPlane.MachineWithDeleteAnnotation(controlPlane.Machines).Len() > 0:
		eligibleMachines = controlPlane.MachineWithDeleteAnnotation(controlPlane.Machines)
	case controlPlane.UnhealthyMachinesWithUnhealthyControlPlaneComponents(outdatedMachines).Len() > 0:
		eligibleMachines = controlPlane.UnhealthyMachinesWithUnhealthyControlPlaneComponents(outdatedMachines)
	case outdatedMachines.Len() > 0:
		eligibleMachines = outdatedMachines
	default:
		eligibleMachines = controlPlane.Machines
	}

	// Pick an eligible machine from the failure domain with most machines in (including both eligible and not eligible machines)
	return controlPlane.MachineInFailureDomainWithMostMachines(ctx, eligibleMachines)
}
