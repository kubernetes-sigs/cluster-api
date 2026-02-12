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
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func (r *KubeadmControlPlaneReconciler) initializeControlPlane(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
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
		Info(fmt.Sprintf("Machine %s created (init)", newMachine.Name),
			"Machine", klog.KObj(newMachine),
			newMachine.Spec.InfrastructureRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.InfrastructureRef.Name),
			newMachine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.Bootstrap.ConfigRef.Name))

	return ctrl.Result{}, nil // No need to requeue here. Machine creation above triggers reconciliation.
}

func (r *KubeadmControlPlaneReconciler) scaleUpControlPlane(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	if r.overrideScaleUpControlPlaneFunc != nil {
		return r.overrideScaleUpControlPlaneFunc(ctx, controlPlane)
	}

	log := ctrl.LoggerFrom(ctx)

	// Run preflight checks to ensure that the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
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
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedScaleUp", "Failed to create additional control plane Machine for cluster % control plane: %v", klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}

	log.WithValues(controlPlane.StatusToLogKeyAndValues(newMachine, nil)...).
		Info(fmt.Sprintf("Machine %s created (scale up)", newMachine.Name),
			"Machine", klog.KObj(newMachine),
			newMachine.Spec.InfrastructureRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.InfrastructureRef.Name),
			newMachine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(newMachine.Namespace, newMachine.Spec.Bootstrap.ConfigRef.Name))

	return ctrl.Result{}, nil // No need to requeue here. Machine creation above triggers reconciliation.
}

func (r *KubeadmControlPlaneReconciler) scaleDownControlPlane(
	ctx context.Context,
	controlPlane *internal.ControlPlane,
	machineToDelete *clusterv1.Machine,
) (ctrl.Result, error) {
	if r.overrideScaleDownControlPlaneFunc != nil {
		return r.overrideScaleDownControlPlaneFunc(ctx, controlPlane, machineToDelete)
	}

	log := ctrl.LoggerFrom(ctx)

	// Run preflight checks ensuring the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
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
		etcdLeaderCandidate := controlPlane.Machines.Newest()
		if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToDelete, etcdLeaderCandidate); err != nil {
			log.Error(err, "Failed to move leadership to candidate machine", "candidate", etcdLeaderCandidate.Name)
			return ctrl.Result{}, err
		}

		// NOTE: etcd member removal will be performed by the kcp-cleanup hook after machine completes drain & all volumes are detached.
	}

	if err := r.Client.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to delete control plane machine")
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "FailedScaleDown",
			"Failed to delete control plane Machine %s for cluster %s control plane: %v", machineToDelete.Name, klog.KObj(controlPlane.Cluster), err)
		return ctrl.Result{}, err
	}
	// Note: We intentionally log after Delete because we want this log line to show up only after DeletionTimestamp has been set.
	// Also, setting DeletionTimestamp doesn't mean the Machine is actually deleted (deletion takes some time).
	log.WithValues(controlPlane.StatusToLogKeyAndValues(nil, machineToDelete)...).
		Info(fmt.Sprintf("Machine %s deleting (scale down)", machineToDelete.Name), "Machine", klog.KObj(machineToDelete))

	return ctrl.Result{}, nil // No need to requeue here. Machine deletion above triggers reconciliation.
}

// preflightChecks checks if the control plane is stable before proceeding with a scale up/scale down operation,
// where stable means that:
// - There are no machine deletion in progress
// - All the health conditions on KCP are true.
// - All the health conditions on the control plane machines are true.
// If the control plane is not passing preflight checks, it requeue.
//
// NOTE: this func uses KCP conditions, it is required to call reconcileControlPlaneAndMachinesConditions before this.
func (r *KubeadmControlPlaneReconciler) preflightChecks(ctx context.Context, controlPlane *internal.ControlPlane, isScaleUp bool, excludeFor ...*clusterv1.Machine) ctrl.Result {
	if r.overridePreflightChecksFunc != nil {
		return r.overridePreflightChecksFunc(ctx, controlPlane, excludeFor...)
	}

	// Reset PreflightCheckResults in case this function is called multiple times (e.g. for in-place update code paths)
	// Note: The PreflightCheckResults field is only written by this func, so this is safe.
	controlPlane.PreflightCheckResults = internal.PreflightCheckResults{}

	log := ctrl.LoggerFrom(ctx)

	// If there is no KCP-owned control-plane machines, then control-plane has not been initialized yet,
	// so it is considered ok to proceed.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}
	}

	if feature.Gates.Enabled(feature.ClusterTopology) {
		// Block when we expect an upgrade to be propagated for topology clusters.
		// NOTE: in case the cluster is performing an upgrade, allow creation of machines for the intermediate step.
		hasSameVersionOfCurrentUpgradeStep := false
		if version, ok := controlPlane.Cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok && version != "" {
			hasSameVersionOfCurrentUpgradeStep = version == controlPlane.KCP.Spec.Version
		}

		if controlPlane.Cluster.Spec.Topology.IsDefined() && controlPlane.Cluster.Spec.Topology.Version != controlPlane.KCP.Spec.Version && !hasSameVersionOfCurrentUpgradeStep {
			v := controlPlane.Cluster.Spec.Topology.Version
			if version, ok := controlPlane.Cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok && version != "" {
				v = version
			}
			log.Info(fmt.Sprintf("Waiting for a version upgrade to %s to be propagated", v))
			controlPlane.PreflightCheckResults.TopologyVersionMismatch = true
			// Slow down reconcile frequency, as deferring a version upgrade waits for slow processes,
			// e.g. workers are completing a previous upgrade step.
			r.controller.DeferNextReconcileForObject(controlPlane.KCP, time.Now().Add(5*time.Second))
			return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
		}
	}

	// If certificates are missing, can't join a new machine
	if isScaleUp && !conditions.IsTrue(controlPlane.KCP, controlplanev1.KubeadmControlPlaneCertificatesAvailableCondition) {
		controlPlane.PreflightCheckResults.CertificateMissing = true
		log.Info("Certificates are missing or unknown, can't join a new machine")
		// Slow down reconcile frequency, user intervention is required to fix the problem.
		r.controller.DeferNextReconcileForObject(controlPlane.KCP, time.Now().Add(5*time.Second))
		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
	}

	// If there are deleting machines, wait for the operation to complete.
	if controlPlane.HasDeletingMachine() {
		controlPlane.PreflightCheckResults.HasDeletingMachine = true
		log.Info("Waiting for machines to be deleted", "machines", strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(), ", "))
		// Slow down reconcile frequency, deletion is a slow process.
		r.controller.DeferNextReconcileForObject(controlPlane.KCP, time.Now().Add(5*time.Second))
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}
	}

	// Check machine health conditions; if there are conditions with False or Unknown, then wait.
	allMachineHealthConditions := []string{
		controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
		controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
		controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
	}
	if controlPlane.IsEtcdManaged() {
		allMachineHealthConditions = append(allMachineHealthConditions,
			controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition,
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

		if !machine.Status.NodeRef.IsDefined() {
			// The conditions will only ever be set on a Machine if we're able to correlate a Machine to a Node.
			// Correlating Machines to Nodes requires the nodeRef to be set.
			// Instead of confusing users with errors about that the conditions are not set, let's point them
			// towards the unset nodeRef (which is the root cause of the conditions not being there).
			machineErrors = append(machineErrors, errors.Errorf("Machine %s does not have a corresponding Node yet (Machine.status.nodeRef not set)", machine.Name))

			controlPlane.PreflightCheckResults.ControlPlaneComponentsNotHealthy = true
			if controlPlane.IsEtcdManaged() {
				controlPlane.PreflightCheckResults.EtcdClusterNotHealthy = true
			}
		} else {
			for _, condition := range allMachineHealthConditions {
				if err := preflightCheckCondition("Machine", machine, condition); err != nil {
					if condition == controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition {
						controlPlane.PreflightCheckResults.EtcdClusterNotHealthy = true
					} else {
						controlPlane.PreflightCheckResults.ControlPlaneComponentsNotHealthy = true
					}
					machineErrors = append(machineErrors, err)
				}
			}
		}
	}
	if len(machineErrors) > 0 {
		aggregatedError := kerrors.NewAggregate(machineErrors)
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "ControlPlaneUnhealthy",
			"Waiting for control plane to pass preflight checks to continue reconciliation: %v", aggregatedError)
		log.Info("Waiting for control plane to pass preflight checks", "failures", aggregatedError.Error())
		// Slow down reconcile frequency, it takes some time before control plane components stabilize
		// after a new Machine is created. Similarly, if there are issues on running Machines, it
		// usually takes some time to get back to normal state.
		r.controller.DeferNextReconcileForObject(controlPlane.KCP, time.Now().Add(5*time.Second))
		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
	}

	return ctrl.Result{}
}

func preflightCheckCondition(kind string, obj *clusterv1.Machine, conditionType string) error {
	c := conditions.Get(obj, conditionType)
	if c == nil {
		return errors.Errorf("%s %s does not have %s condition", kind, obj.GetName(), conditionType)
	}
	if c.Status == metav1.ConditionFalse {
		return errors.Errorf("%s %s reports %s condition is false (%s)", kind, obj.GetName(), conditionType, c.Message)
	}
	if c.Status == metav1.ConditionUnknown {
		return errors.Errorf("%s %s reports %s condition is unknown (%s)", kind, obj.GetName(), conditionType, c.Message)
	}
	return nil
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
func selectMachineForInPlaceUpdateOrScaleDown(ctx context.Context, controlPlane *internal.ControlPlane, outdatedMachines collections.Machines) (*clusterv1.Machine, error) {
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
