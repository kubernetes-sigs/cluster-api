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

// preflightChecks checks if the control plane is stable before proceeding with a in-place update, scale up or scale down operation.
// Under normal circumstances, a control stable is considered stable when:
// - There are no machine deletion in progress
// - All the health conditions on KCP are true.
// - All the health conditions on the control plane machines are true.
// In a few specific case, preflight checks are less demanding e.g. when scaling up after a remediation, KCP is required
// to allow the operation even if the control plane is not fully stable, thus allowing the system to recover when there are multiple failures.
//
// If the control plane is not passing preflight checks, it requeue.
//
// Note: This check leverage the information collected in reconcileControlPlaneAndMachinesConditions at the beginning of reconcile;
// the info are also used to compute status.Conditions.
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
		if version, ok := controlPlane.Cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok {
			hasSameVersionOfCurrentUpgradeStep = version == controlPlane.KCP.Spec.Version
		}

		if controlPlane.Cluster.Spec.Topology.IsDefined() && controlPlane.Cluster.Spec.Topology.Version != controlPlane.KCP.Spec.Version && !hasSameVersionOfCurrentUpgradeStep {
			v := controlPlane.Cluster.Spec.Topology.Version
			if version, ok := controlPlane.Cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok {
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

	// At this point we can assume that:
	// - No control plane Machines are being deleted
	// - There are no blockers for joining a machine in case of scale up (e.g. missing certificates, or kubeadm version skew)
	//
	// Next steps is to assess the potential effects of the scale up/scale down operation we are running preflight checks for.
	//
	// Most specifically, KCP should determine if this operation is going to leave the Kubernetes control plane components
	// and the etcd cluster in operational state or not.
	// This check will return false if there are machines not yet fully provisioned or if there are etcd members still in learner mode.
	err := r.checkHealthiness(ctx, controlPlane, excludeFor)

	// If the control plane doesn't meet the "fully stable" criteria, and the control plane is trying to perform a scale up after
	// a machine deletion due to remediation, perform a more precise check on Kubernetes control plane components and etcd members,
	// thus allowing the system to recover also when there are multiple failures.
	if _, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok && isScaleUp && err != nil {
		log.Info("Performing checks to allow creation of a replacement machine while remediation is in progress")
		err = r.checkHealthinessWhileRemediationInProgress(ctx, controlPlane)
	}

	if err != nil {
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "ControlPlaneUnhealthy",
			"Waiting for control plane to pass preflight checks to continue reconciliation: %v", err)
		log.Info("Waiting for control plane to pass preflight checks", "failures", err.Error())
		// Slow down reconcile frequency, it takes some time before control plane components stabilize
		// after a new Machine is created. Similarly, if there are issues on running Machines, it
		// usually takes some time to get back to normal state.
		r.controller.DeferNextReconcileForObject(controlPlane.KCP, time.Now().Add(5*time.Second))
		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}
	}

	return ctrl.Result{}
}

// checkHealthiness verifies if the control plane is fully stable checking that all Kubernetes control plane components and etcd members are ok.
// When performing a scale down operation, the deleting machine is ignored.
func (r *KubeadmControlPlaneReconciler) checkHealthiness(_ context.Context, controlPlane *internal.ControlPlane, excludeFor []*clusterv1.Machine) error {
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
	return kerrors.NewAggregate(machineErrors)
}

// checkHealthinessWhileRemediationInProgress verifies if the Kubernetes control plane components and etcd members are healthy enough
// to allow the creation of the replacement Machine after one control plane Machine has been deleted because unhealthy.
// Note: In this case it is not required to check if there are machines not yet fully provisioned, because remediation
// can start only when all the machines are provisioned (already checked before setting remediation in progress, and
// after that only machine deletion could happen).
func (r *KubeadmControlPlaneReconciler) checkHealthinessWhileRemediationInProgress(ctx context.Context, controlPlane *internal.ControlPlane) error {
	allErrors := []error{}

	// make sure we reset the flags for surfacing prefligh checks in conditions from scratch.
	controlPlane.PreflightCheckResults.ControlPlaneComponentsNotHealthy = false
	controlPlane.PreflightCheckResults.EtcdClusterNotHealthy = false

	// Considering this func is only called before scaling up after one has been deleted due to remediation,
	// we can assume that the target cluster will have current Machines +1 new Machine (the replacement machine).
	//
	// As a consequence:
	// - one Kubernetes control plane components is going to be added, no Kubernetes control plane components are going to be deleted.
	addKubernetesControlPlane := true
	kubernetesControlPlaneToBeDeleted := ""
	// - one etcd member is going to be added, no etcd member are going to be deleted.
	addEtcdMember := true
	etcdMemberToBeDeleted := ""

	// Check id the target Kubernetes control plane will have at least one set of operational Kubernetes control plane components.
	if !r.targetKubernetesControlPlaneComponentsHealthy(ctx, controlPlane, addKubernetesControlPlane, kubernetesControlPlaneToBeDeleted) {
		controlPlane.PreflightCheckResults.ControlPlaneComponentsNotHealthy = true
		allErrors = append(allErrors, errors.New("cannot add a new control plane Machine when there are no control plane Machines with all Kubernetes control plane components in healthy state. Please check Kubernetes control plane component status"))
	}

	// Check target etcd cluster.
	if controlPlane.IsEtcdManaged() {
		if len(controlPlane.EtcdMembers) == 0 {
			allErrors = append(allErrors, errors.New("cannot check etcd cluster health before scale up, etcd member list is empty"))
		} else if !r.targetEtcdClusterHealthy(ctx, controlPlane, addEtcdMember, etcdMemberToBeDeleted) {
			allErrors = append(allErrors, errors.New("adding a new control plane Machine can lead to etcd quorum loss. Please check the etcd status"))
			controlPlane.PreflightCheckResults.EtcdClusterNotHealthy = true
		}
	}

	return kerrors.NewAggregate(allErrors)
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
