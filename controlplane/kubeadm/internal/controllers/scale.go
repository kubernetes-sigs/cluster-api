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
	"strings"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func (r *KubeadmControlPlaneReconciler) initializeControlPlane(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	bootstrapSpec := controlPlane.InitialControlPlaneConfig()
	fd := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err := r.cloneConfigsAndGenerateMachine(ctx, controlPlane.Cluster, controlPlane.KCP, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create initial control plane Machine")
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedInitialization", "Failed to create initial control plane Machine for cluster %s control plane: %v", klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *KubeadmControlPlaneReconciler) scaleUpControlPlane(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Run preflight checks to ensure that the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	if result, err := r.preflightChecks(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Create the bootstrap configuration
	bootstrapSpec := controlPlane.JoinControlPlaneConfig()
	fd := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err := r.cloneConfigsAndGenerateMachine(ctx, controlPlane.Cluster, controlPlane.KCP, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create additional control plane Machine")
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedScaleUp", "Failed to create additional control plane Machine for cluster % control plane: %v", klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are other operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *KubeadmControlPlaneReconciler) scaleDownControlPlane(
	ctx context.Context,
	controlPlane *internal.ControlPlane,
	outdatedMachines collections.Machines,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Pick the Machine that we should scale down.
	machineToDelete, err := selectMachineForScaleDown(ctx, controlPlane, outdatedMachines)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to select machine for scale down")
	}

	// Run preflight checks ensuring the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	// Given that we're scaling down, we can exclude the machineToDelete from the preflight checks.
	if result, err := r.preflightChecks(ctx, controlPlane, machineToDelete); err != nil || !result.IsZero() {
		return result, err
	}

	workloadCluster, err := controlPlane.GetWorkloadCluster(ctx)
	if err != nil {
		logger.Error(err, "Failed to create client to workload cluster")
		return ctrl.Result{}, errors.Wrapf(err, "failed to create client to workload cluster")
	}

	if machineToDelete == nil {
		logger.Info("Failed to pick control plane Machine to delete")
		return ctrl.Result{}, errors.New("failed to pick control plane Machine to delete")
	}

	// If KCP should manage etcd, If etcd leadership is on machine that is about to be deleted, move it to the newest member available.
	if controlPlane.IsEtcdManaged() {
		etcdLeaderCandidate := controlPlane.Machines.Newest()
		if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToDelete, etcdLeaderCandidate); err != nil {
			logger.Error(err, "Failed to move leadership to candidate machine", "candidate", etcdLeaderCandidate.Name)
			return ctrl.Result{}, err
		}
		if err := workloadCluster.RemoveEtcdMemberForMachine(ctx, machineToDelete); err != nil {
			logger.Error(err, "Failed to remove etcd member for machine")
			return ctrl.Result{}, err
		}
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	if err := workloadCluster.RemoveMachineFromKubeadmConfigMap(ctx, machineToDelete, parsedVersion); err != nil {
		logger.Error(err, "Failed to remove machine from kubeadm ConfigMap")
		return ctrl.Result{}, err
	}

	logger = logger.WithValues("Machine", klog.KObj(machineToDelete))
	if err := r.Client.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to delete control plane machine")
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedScaleDown",
			"Failed to delete control plane Machine %s for cluster %s control plane: %v", machineToDelete.Name, klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

// preflightChecks checks if the control plane is stable before proceeding with a scale up/scale down operation,
// where stable means that:
// - There are no machine deletion in progress
// - All the health conditions on KCP are true.
// - All the health conditions on the control plane machines are true.
// If the control plane is not passing preflight checks, it requeue.
//
// NOTE: this func uses KCP conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *KubeadmControlPlaneReconciler) preflightChecks(ctx context.Context, controlPlane *internal.ControlPlane, excludeFor ...*clusterv1.Machine) (ctrl.Result, error) { //nolint:unparam
	logger := ctrl.LoggerFrom(ctx)

	// If there is no KCP-owned control-plane machines, then control-plane has not been initialized yet,
	// so it is considered ok to proceed.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}, nil
	}

	// If there are deleting machines, wait for the operation to complete.
	if controlPlane.HasDeletingMachine() {
		logger.Info("Waiting for machines to be deleted", "Machines", strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(), ", "))
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Check machine health conditions; if there are conditions with False or Unknown, then wait.
	allMachineHealthConditions := []clusterv1.ConditionType{
		controlplanev1.MachineAPIServerPodHealthyCondition,
		controlplanev1.MachineControllerManagerPodHealthyCondition,
		controlplanev1.MachineSchedulerPodHealthyCondition,
	}
	if controlPlane.IsEtcdManaged() {
		allMachineHealthConditions = append(allMachineHealthConditions,
			controlplanev1.MachineEtcdPodHealthyCondition,
			controlplanev1.MachineEtcdMemberHealthyCondition,
		)
	}
	machineErrors := []error{}

loopmachines:
	for _, machine := range controlPlane.Machines {
		for _, excluded := range excludeFor {
			// If this machine should be excluded from the individual
			// health check, continue the out loop.
			if machine.Name == excluded.Name {
				continue loopmachines
			}
		}

		if machine.Status.NodeRef == nil {
			// The conditions will only ever be set on a Machine if we're able to correlate a Machine to a Node.
			// Correlating Machines to Nodes requires the nodeRef to be set.
			// Instead of confusing users with errors about that the conditions are not set, let's point them
			// towards the unset nodeRef (which is the root cause of the conditions not being there).
			machineErrors = append(machineErrors, errors.Errorf("Machine %s does not have a corresponding Node yet (Machine.status.nodeRef not set)", machine.Name))
		} else {
			for _, condition := range allMachineHealthConditions {
				if err := preflightCheckCondition("Machine", machine, condition); err != nil {
					machineErrors = append(machineErrors, err)
				}
			}
		}
	}
	if len(machineErrors) > 0 {
		aggregatedError := kerrors.NewAggregate(machineErrors)
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "ControlPlaneUnhealthy",
			"Waiting for control plane to pass preflight checks to continue reconciliation: %v", aggregatedError)
		logger.Info("Waiting for control plane to pass preflight checks", "failures", aggregatedError.Error())

		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func preflightCheckCondition(kind string, obj conditions.Getter, condition clusterv1.ConditionType) error {
	c := conditions.Get(obj, condition)
	if c == nil {
		return errors.Errorf("%s %s does not have %s condition", kind, obj.GetName(), condition)
	}
	if c.Status == corev1.ConditionFalse {
		return errors.Errorf("%s %s reports %s condition is false (%s, %s)", kind, obj.GetName(), condition, c.Severity, c.Message)
	}
	if c.Status == corev1.ConditionUnknown {
		return errors.Errorf("%s %s reports %s condition is unknown (%s)", kind, obj.GetName(), condition, c.Message)
	}
	return nil
}

func selectMachineForScaleDown(ctx context.Context, controlPlane *internal.ControlPlane, outdatedMachines collections.Machines) (*clusterv1.Machine, error) {
	machines := controlPlane.Machines
	switch {
	case controlPlane.MachineWithDeleteAnnotation(outdatedMachines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(outdatedMachines)
	case controlPlane.MachineWithDeleteAnnotation(machines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(machines)
	case controlPlane.UnhealthyMachinesWithNonMHCUnhealthyCondition(outdatedMachines).Len() > 0:
		machines = controlPlane.UnhealthyMachinesWithNonMHCUnhealthyCondition(outdatedMachines)
	case outdatedMachines.Len() > 0:
		machines = outdatedMachines
	}
	return controlPlane.MachineInFailureDomainWithMostMachines(ctx, machines)
}
