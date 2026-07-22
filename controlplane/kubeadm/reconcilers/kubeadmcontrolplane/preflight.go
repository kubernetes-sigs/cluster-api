/*
Copyright 2026 The Kubernetes Authors.

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
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/pkg"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

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
func (r *Reconciler) preflightChecks(ctx context.Context, controlPlane *pkg.ControlPlane, isScaleUp bool, excludeFor ...*clusterv1.Machine) ctrl.Result {
	if r.overridePreflightChecksFunc != nil {
		return r.overridePreflightChecksFunc(ctx, controlPlane, excludeFor...)
	}

	// Reset PreflightCheckResults in case this function is called multiple times (e.g. for in-place update code paths)
	// Note: The PreflightCheckResults field is only written by this func, so this is safe.
	controlPlane.PreflightCheckResults = pkg.PreflightCheckResults{}

	log := ctrl.LoggerFrom(ctx)

	// If there is no KCP-owned control-plane machines, then control-plane has not been initialized yet,
	// so it is considered ok to proceed.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}
	}

	if feature.Gates.Enabled(feature.ClusterTopology) && controlPlane.Cluster.Spec.Topology.IsDefined() {
		// Block when we expect an upgrade to be propagated for topology clusters.
		// NOTE: in case the cluster is performing an upgrade, allow creation of machines for the intermediate step.
		hasSameVersionOfCurrentUpgradeStep := false
		if version, ok := controlPlane.Cluster.GetAnnotations()[clusterv1.ClusterTopologyUpgradeStepAnnotation]; ok && version != "" {
			hasSameVersionOfCurrentUpgradeStep = version == controlPlane.KCP.Spec.Version
		}

		if controlPlane.Cluster.Spec.Topology.Version != controlPlane.KCP.Spec.Version && !hasSameVersionOfCurrentUpgradeStep {
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
func (r *Reconciler) checkHealthiness(_ context.Context, controlPlane *pkg.ControlPlane, excludeFor []*clusterv1.Machine) error {
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
			machineErrors = append(machineErrors, pkgerrors.Errorf("Machine %s does not have a corresponding Node yet (Machine.status.nodeRef not set)", machine.Name))

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
func (r *Reconciler) checkHealthinessWhileRemediationInProgress(ctx context.Context, controlPlane *pkg.ControlPlane) error {
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
		allErrors = append(allErrors, pkgerrors.New("cannot add a new control plane Machine when there are no control plane Machines with all Kubernetes control plane components in healthy state. Please check Kubernetes control plane component status"))
	}

	// Check target etcd cluster.
	if controlPlane.IsEtcdManaged() {
		if len(controlPlane.EtcdMembers) == 0 {
			allErrors = append(allErrors, pkgerrors.New("cannot check etcd cluster health before scale up, etcd member list is empty"))
		} else if !r.targetEtcdClusterHealthy(ctx, controlPlane, addEtcdMember, etcdMemberToBeDeleted) {
			allErrors = append(allErrors, pkgerrors.New("adding a new control plane Machine can lead to etcd quorum loss. Please check the etcd status"))
			controlPlane.PreflightCheckResults.EtcdClusterNotHealthy = true
		}
	}

	return kerrors.NewAggregate(allErrors)
}

func preflightCheckCondition(kind string, obj *clusterv1.Machine, conditionType string) error {
	c := conditions.Get(obj, conditionType)
	if c == nil {
		return pkgerrors.Errorf("%s %s does not have %s condition", kind, obj.GetName(), conditionType)
	}
	if c.Status == metav1.ConditionFalse {
		return pkgerrors.Errorf("%s %s reports %s condition is false (%s)", kind, obj.GetName(), conditionType, c.Message)
	}
	if c.Status == metav1.ConditionUnknown {
		return pkgerrors.Errorf("%s %s reports %s condition is unknown (%s)", kind, obj.GetName(), conditionType, c.Message)
	}
	return nil
}
