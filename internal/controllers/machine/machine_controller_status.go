/*
Copyright 2024 The Kubernetes Authors.

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

package machine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

// updateStatus update Machine's status.
// This implies that the code in this function should account for several edge cases e.g. machine being partially provisioned,
// machine being partially deleted but also for running machines being disrupted e.g. by deleting the node.
// Additionally, this func should ensure that the conditions managed by this controller are always set in order to
// comply with the recommendation in the Kubernetes API guidelines.
// Note: v1beta1 conditions are not managed by this func.
func (r *Reconciler) updateStatus(ctx context.Context, s *scope) {
	// Update status from the Bootstrap Config external resource.
	// Note: some of the status fields derived from the Bootstrap Config are managed in reconcileBootstrap, e.g. status.BootstrapReady, etc.
	// here we are taking care only of the delta (condition).
	setBootstrapReadyCondition(ctx, s.machine, s.bootstrapConfig, s.bootstrapConfigIsNotFound)

	// Update status from the InfraMachine external resource.
	// Note: some of the status fields derived from the InfraMachine are managed in reconcileInfrastructure, e.g. status.InfrastructureReady, etc.
	// here we are taking care only of the delta (condition).
	setInfrastructureReadyCondition(ctx, s.machine, s.infraMachine, s.infraMachineIsNotFound)

	// Update status from the Node external resource.
	// Note: some of the status fields are managed in reconcileNode, e.g. status.NodeRef, etc.
	// here we are taking care only of the delta (condition).
	lastProbeSuccessTime := r.ClusterCache.GetLastProbeSuccessTimestamp(ctx, client.ObjectKeyFromObject(s.cluster))
	setNodeHealthyAndReadyConditions(ctx, s.cluster, s.machine, s.node, s.nodeGetError, lastProbeSuccessTime, r.RemoteConditionsGracePeriod)

	// Updates Machine status not observed from Bootstrap Config, InfraMachine or Node (update Machine's own status).
	// Note: some of the status are set in reconcileCertificateExpiry (e.g.status.CertificatesExpiryDate),
	// in reconcileDelete (e.g. status.Deletion nested fields), and also in the defer patch at the end of the main reconcile loop (status.ObservedGeneration) etc.
	// Note: also other controllers adds conditions to the machine object (machine's owner controller sets the UpToDate condition,
	// MHC controller sets HealthCheckSucceeded and OwnerRemediated conditions, KCP sets conditions about etcd and control plane pods).

	// TODO: Set the uptodate condition for standalone pods

	setDeletingCondition(ctx, s.machine, s.reconcileDeleteExecuted, s.deletingReason, s.deletingMessage)

	setReadyCondition(ctx, s.machine)

	setAvailableCondition(ctx, s.machine)

	setMachinePhaseAndLastUpdated(ctx, s.machine)
}

func setBootstrapReadyCondition(_ context.Context, machine *clusterv1.Machine, bootstrapConfig *unstructured.Unstructured, bootstrapConfigIsNotFound bool) {
	if machine.Spec.Bootstrap.ConfigRef == nil {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineBootstrapDataSecretProvidedV1Beta2Reason,
		})
		return
	}

	if bootstrapConfig != nil {
		if err := v1beta2conditions.SetMirrorConditionFromUnstructured(
			bootstrapConfig, machine,
			contract.Bootstrap().ReadyConditionType(), v1beta2conditions.TargetConditionType(clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
			v1beta2conditions.FallbackCondition{
				Status:  v1beta2conditions.BoolToStatus(machine.Status.BootstrapReady),
				Reason:  clusterv1.MachineBootstrapConfigReadyNoReasonReportedV1Beta2Reason,
				Message: bootstrapConfigReadyFallBackMessage(machine.Spec.Bootstrap.ConfigRef.Kind, machine.Status.BootstrapReady),
			},
		); err != nil {
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineBootstrapConfigInvalidConditionReportedV1Beta2Reason,
				Message: err.Error(),
			})
		}
		return
	}

	// If we got unexpected errors in reading the bootstrap config (this should happen rarely), surface them
	if !bootstrapConfigIsNotFound {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineBootstrapConfigInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileBootstrap.
		})
		return
	}

	// Bootstrap config missing when the machine is deleting and we know that the BootstrapConfig actually existed.
	if !machine.DeletionTimestamp.IsZero() && machine.Status.BootstrapReady {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineBootstrapConfigDeletedV1Beta2Reason,
			Message: fmt.Sprintf("%s has been deleted", machine.Spec.Bootstrap.ConfigRef.Kind),
		})
		return
	}

	// If the machine is not deleting, and boostrap config object does not exist,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the machine and all the objects referenced by it (provisioning yet to start/started, but status.nodeRef not yet set).
	// - when the machine has been provisioned
	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
		Status:  metav1.ConditionUnknown,
		Reason:  clusterv1.MachineBootstrapConfigDoesNotExistV1Beta2Reason,
		Message: fmt.Sprintf("%s does not exist", machine.Spec.Bootstrap.ConfigRef.Kind),
	})
}

func bootstrapConfigReadyFallBackMessage(kind string, ready bool) string {
	return fmt.Sprintf("%s status.ready is %t", kind, ready)
}

func setInfrastructureReadyCondition(_ context.Context, machine *clusterv1.Machine, infraMachine *unstructured.Unstructured, infraMachineIsNotFound bool) {
	if infraMachine != nil {
		if err := v1beta2conditions.SetMirrorConditionFromUnstructured(
			infraMachine, machine,
			contract.InfrastructureMachine().ReadyConditionType(), v1beta2conditions.TargetConditionType(clusterv1.MachineInfrastructureReadyV1Beta2Condition),
			v1beta2conditions.FallbackCondition{
				Status:  v1beta2conditions.BoolToStatus(machine.Status.InfrastructureReady),
				Reason:  clusterv1.MachineInfrastructureReadyNoReasonReportedV1Beta2Reason,
				Message: infrastructureReadyFallBackMessage(machine.Spec.InfrastructureRef.Kind, machine.Status.InfrastructureReady),
			},
		); err != nil {
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureInvalidConditionReportedV1Beta2Reason,
				Message: err.Error(),
			})
		}
		return
	}

	// If we got errors in reading the infra machine (this should happen rarely), surface them
	if !infraMachineIsNotFound {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineInfrastructureInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileInfrastructure.
		})
		return
	}

	// Infra machine missing when the machine is deleting.
	// NOTE: in case an accidental deletion happens before volume detach is completed, the Node hosted on the Machine
	// will be considered unreachable Machine deletion will complete.
	if !machine.DeletionTimestamp.IsZero() {
		if machine.Status.InfrastructureReady {
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
				Message: fmt.Sprintf("%s has been deleted", machine.Spec.InfrastructureRef.Kind),
			})
			return
		}

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineInfrastructureDoesNotExistV1Beta2Reason,
			Message: fmt.Sprintf("%s does not exist", machine.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// Report an issue if infra machine missing after the machine has been initialized (and the machine is still running).
	if machine.Status.InfrastructureReady {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse, // setting to false to give more relevance in the ready condition summary.
			Reason:  clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
			Message: fmt.Sprintf("%s has been deleted while the machine still exists", machine.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// If the machine is not deleting, and infra machine object does not exist yet,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the machine and all the objects referenced by it (provisioning yet to start/started, but status.InfrastructureReady not yet set).
	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
		Status:  metav1.ConditionUnknown,
		Reason:  clusterv1.MachineInfrastructureDoesNotExistV1Beta2Reason,
		Message: fmt.Sprintf("%s does not exist", machine.Spec.InfrastructureRef.Kind),
	})
}

func infrastructureReadyFallBackMessage(kind string, ready bool) string {
	return fmt.Sprintf("%s status.ready is %t", kind, ready)
}

func setNodeHealthyAndReadyConditions(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, node *corev1.Node, nodeGetErr error, lastProbeSuccessTime time.Time, remoteConditionsGracePeriod time.Duration) {
	if !cluster.Status.InfrastructureReady {
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
			"Waiting for Cluster status.infrastructureReady to be true")
		return
	}

	controlPlaneInitialized := conditions.Get(cluster, clusterv1.ControlPlaneInitializedCondition)
	if controlPlaneInitialized == nil || controlPlaneInitialized.Status != corev1.ConditionTrue {
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeInspectionFailedV1Beta2Reason,
			"Waiting for Cluster control plane to be initialized")
		return
	}

	// Remote conditions grace period is counted from the later of last probe success and control plane initialized.
	if time.Since(maxTime(lastProbeSuccessTime, controlPlaneInitialized.LastTransitionTime.Time)) > remoteConditionsGracePeriod {
		// Overwrite conditions to ConnectionDown.
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeConnectionDownV1Beta2Reason,
			lastProbeSuccessMessage(lastProbeSuccessTime))
		return
	}

	if nodeGetErr != nil {
		if errors.Is(nodeGetErr, clustercache.ErrClusterNotConnected) {
			// If conditions are not set, set them to ConnectionDown.
			// Note: This will allow to keep reporting last known status in case there are temporary connection errors.
			// However, if connection errors persist more than remoteConditionsGracePeriod, conditions will be overridden.
			if !v1beta2conditions.Has(machine, clusterv1.MachineNodeReadyV1Beta2Condition) ||
				!v1beta2conditions.Has(machine, clusterv1.MachineNodeHealthyV1Beta2Condition) {
				setNodeConditions(machine, metav1.ConditionUnknown,
					clusterv1.MachineNodeConnectionDownV1Beta2Reason,
					lastProbeSuccessMessage(lastProbeSuccessTime))
			}
			return
		}

		// Overwrite conditions to InternalError.
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeInternalErrorV1Beta2Reason,
			"Please check controller logs for errors")
		// NOTE: the error is logged by reconcileNode.
		return
	}

	if node != nil {
		var nodeReady *metav1.Condition
		for _, condition := range node.Status.Conditions {
			if condition.Type != corev1.NodeReady {
				continue
			}

			message := ""
			if condition.Message != "" {
				message = fmt.Sprintf("* Node.Ready: %s", condition.Message)
			}
			reason := condition.Reason
			if reason == "" {
				reason = clusterv1.NoReasonReportedV1Beta2Reason
			}
			nodeReady = &metav1.Condition{
				Type:               clusterv1.MachineNodeReadyV1Beta2Condition,
				Status:             metav1.ConditionStatus(condition.Status),
				LastTransitionTime: condition.LastTransitionTime,
				Reason:             reason,
				Message:            message,
			}
		}

		if nodeReady == nil {
			nodeReady = &metav1.Condition{
				Type:   clusterv1.MachineNodeReadyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: clusterv1.MachineNodeConditionNotYetReportedV1Beta2Reason,
			}
		}
		v1beta2conditions.Set(machine, *nodeReady)

		status, reason, message := summarizeNodeV1Beta2Conditions(ctx, node)
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status:  status,
			Reason:  reason,
			Message: message,
		})

		return
	}

	// Node missing when the machine is deleting.
	// NOTE: in case an accidental deletion happens before volume detach is completed, the Node
	// will be considered unreachable Machine deletion will complete.
	if !machine.DeletionTimestamp.IsZero() {
		if machine.Status.NodeRef != nil {
			setNodeConditions(machine, metav1.ConditionUnknown,
				clusterv1.MachineNodeDeletedV1Beta2Reason,
				fmt.Sprintf("Node %s has been deleted", machine.Status.NodeRef.Name))
			return
		}

		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
			"Node does not exist")
		return
	}

	// Report an issue if node missing after being initialized.
	if machine.Status.NodeRef != nil {
		// Setting MachineNodeHealthyV1Beta2Condition to False to give it more relevance in the Ready condition summary.
		// Setting MachineNodeReadyV1Beta2Condition to False to keep it consistent with MachineNodeHealthyV1Beta2Condition.
		setNodeConditions(machine, metav1.ConditionFalse,
			clusterv1.MachineNodeDeletedV1Beta2Reason,
			fmt.Sprintf("Node %s has been deleted while the machine still exists", machine.Status.NodeRef.Name))
		return
	}

	// If the machine is at the end of the provisioning phase, with ProviderID set, but still waiting
	// for a matching Node to exists, surface this.
	if ptr.Deref(machine.Spec.ProviderID, "") != "" {
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
			fmt.Sprintf("Waiting for a Node with spec.providerID %s to exist", *machine.Spec.ProviderID))
		return
	}

	// If the machine is at the beginning of the provisioning phase, with ProviderID not yet set, surface this.
	setNodeConditions(machine, metav1.ConditionUnknown,
		clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
		fmt.Sprintf("Waiting for %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind))
}

func setNodeConditions(machine *clusterv1.Machine, status metav1.ConditionStatus, reason, msg string) {
	for _, conditionType := range []string{clusterv1.MachineNodeReadyV1Beta2Condition, clusterv1.MachineNodeHealthyV1Beta2Condition} {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    conditionType,
			Status:  status,
			Reason:  reason,
			Message: msg,
		})
	}
}

func lastProbeSuccessMessage(lastProbeSuccessTime time.Time) string {
	if lastProbeSuccessTime.IsZero() {
		return ""
	}
	return fmt.Sprintf("Last successful probe at %s", lastProbeSuccessTime.Format(time.RFC3339))
}

func maxTime(t1, t2 time.Time) time.Time {
	if t1.After(t2) {
		return t1
	}
	return t2
}

// summarizeNodeV1Beta2Conditions summarizes a Node's conditions (NodeReady, NodeMemoryPressure, NodeDiskPressure, NodePIDPressure).
// the summary is computed in way that is similar to how v1beta2conditions.NewSummaryCondition works, but in this case the
// implementation is simpler/less flexible and it surfaces only issues & unknown conditions.
func summarizeNodeV1Beta2Conditions(_ context.Context, node *corev1.Node) (metav1.ConditionStatus, string, string) {
	semanticallyFalseStatus := 0
	unknownStatus := 0

	messages := []string{}
	issueReason := ""
	unknownReason := ""
	for _, conditionType := range []corev1.NodeConditionType{corev1.NodeReady, corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure} {
		var condition *corev1.NodeCondition
		for _, c := range node.Status.Conditions {
			if c.Type == conditionType {
				condition = &c
			}
		}
		if condition == nil {
			messages = append(messages, fmt.Sprintf("* Node.%s: Condition not yet reported", conditionType))
			if unknownStatus == 0 {
				unknownReason = clusterv1.MachineNodeConditionNotYetReportedV1Beta2Reason
			} else {
				unknownReason = v1beta2conditions.MultipleUnknownReportedReason
			}
			unknownStatus++
			continue
		}

		switch condition.Type {
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
			if condition.Status != corev1.ConditionFalse {
				m := condition.Message
				if m == "" {
					m = fmt.Sprintf("Condition is %s", condition.Status)
				}
				messages = append(messages, fmt.Sprintf("* Node.%s: %s", condition.Type, m))
				if condition.Status == corev1.ConditionUnknown {
					if unknownStatus == 0 {
						unknownReason = condition.Reason
					} else {
						unknownReason = v1beta2conditions.MultipleUnknownReportedReason
					}
					unknownStatus++
					continue
				}
				if semanticallyFalseStatus == 0 {
					issueReason = condition.Reason
				} else {
					issueReason = v1beta2conditions.MultipleIssuesReportedReason
				}
				semanticallyFalseStatus++
				continue
			}
		case corev1.NodeReady:
			if condition.Status != corev1.ConditionTrue {
				m := condition.Message
				if m == "" {
					m = fmt.Sprintf("Condition is %s", condition.Status)
				}
				messages = append(messages, fmt.Sprintf("* Node.%s: %s", condition.Type, m))
				if condition.Status == corev1.ConditionUnknown {
					if unknownStatus == 0 {
						unknownReason = condition.Reason
					} else {
						unknownReason = v1beta2conditions.MultipleUnknownReportedReason
					}
					unknownStatus++
					continue
				}
				if semanticallyFalseStatus == 0 {
					issueReason = condition.Reason
				} else {
					issueReason = v1beta2conditions.MultipleIssuesReportedReason
				}
				semanticallyFalseStatus++
			}
		}
	}

	message := strings.Join(messages, "\n")
	if semanticallyFalseStatus > 0 {
		if issueReason == "" {
			issueReason = v1beta2conditions.NoReasonReported
		}
		return metav1.ConditionFalse, issueReason, message
	}
	if unknownStatus > 0 {
		if unknownReason == "" {
			unknownReason = v1beta2conditions.NoReasonReported
		}
		return metav1.ConditionUnknown, unknownReason, message
	}
	return metav1.ConditionTrue, v1beta2conditions.MultipleInfoReportedReason, ""
}

type machineConditionCustomMergeStrategy struct {
	machine                        *clusterv1.Machine
	negativePolarityConditionTypes sets.Set[string]
}

func (c machineConditionCustomMergeStrategy) Merge(conditions []v1beta2conditions.ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error) {
	return v1beta2conditions.DefaultMergeStrategyWithCustomPriority(func(condition metav1.Condition) v1beta2conditions.MergePriority {
		// While machine is deleting, treat unknown conditions from external objects as info (it is ok that those objects have been deleted at this stage).
		if !c.machine.DeletionTimestamp.IsZero() {
			if condition.Type == clusterv1.MachineBootstrapConfigReadyV1Beta2Condition && condition.Status == metav1.ConditionUnknown && (condition.Reason == clusterv1.MachineBootstrapConfigDeletedV1Beta2Reason || condition.Reason == clusterv1.MachineBootstrapConfigDoesNotExistV1Beta2Reason) {
				return v1beta2conditions.InfoMergePriority
			}
			if condition.Type == clusterv1.MachineInfrastructureReadyV1Beta2Condition && condition.Status == metav1.ConditionUnknown && (condition.Reason == clusterv1.MachineInfrastructureDeletedV1Beta2Reason || condition.Reason == clusterv1.MachineInfrastructureDoesNotExistV1Beta2Reason) {
				return v1beta2conditions.InfoMergePriority
			}
			if condition.Type == clusterv1.MachineNodeHealthyV1Beta2Condition && condition.Status == metav1.ConditionUnknown && (condition.Reason == clusterv1.MachineNodeDeletedV1Beta2Reason || condition.Reason == clusterv1.MachineNodeDoesNotExistV1Beta2Reason) {
				return v1beta2conditions.InfoMergePriority
			}
			// Note: MachineNodeReadyV1Beta2Condition is not relevant for the summary.
		}
		return v1beta2conditions.GetDefaultMergePriorityFunc(c.negativePolarityConditionTypes)(condition)
	}).Merge(conditions, conditionTypes)
}

func setDeletingCondition(_ context.Context, machine *clusterv1.Machine, reconcileDeleteExecuted bool, deletingReason, deletingMessage string) {
	if machine.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineDeletingV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineDeletingDeletionTimestampNotSetV1Beta2Reason,
		})
		return
	}

	if !reconcileDeleteExecuted {
		// Don't update the Deleting condition if reconcileDelete was not executed (e.g.
		// because of rate-limiting).
		return
	}

	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineDeletingV1Beta2Condition,
		Status:  metav1.ConditionTrue,
		Reason:  deletingReason,
		Message: deletingMessage,
	})
}

func setReadyCondition(ctx context.Context, machine *clusterv1.Machine) {
	log := ctrl.LoggerFrom(ctx)

	forConditionTypes := v1beta2conditions.ForConditionTypes{
		clusterv1.MachineDeletingV1Beta2Condition,
		clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
		clusterv1.MachineInfrastructureReadyV1Beta2Condition,
		clusterv1.MachineNodeHealthyV1Beta2Condition,
		clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
	}
	for _, g := range machine.Spec.ReadinessGates {
		forConditionTypes = append(forConditionTypes, g.ConditionType)
	}

	summaryOpts := []v1beta2conditions.SummaryOption{
		forConditionTypes,
		v1beta2conditions.IgnoreTypesIfMissing{
			clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
		},
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: machineConditionCustomMergeStrategy{
				machine:                        machine,
				negativePolarityConditionTypes: sets.Set[string]{}.Insert(clusterv1.MachineDeletingV1Beta2Condition),
			},
		},
		v1beta2conditions.NegativePolarityConditionTypes{
			clusterv1.MachineDeletingV1Beta2Condition,
		},
	}

	// Add overrides for conditions we want to surface in the Ready condition with slightly different messages,
	// mostly to improve when we will aggregate the Ready condition from many machines on MS, MD etc.
	var overrideConditions v1beta2conditions.OverrideConditions
	if !machine.DeletionTimestamp.IsZero() {
		overrideConditions = append(overrideConditions, calculateDeletingConditionForSummary(machine))
	}

	if infrastructureReadyCondition := calculateInfrastructureReadyForSummary(machine); infrastructureReadyCondition != nil {
		overrideConditions = append(overrideConditions, *infrastructureReadyCondition)
	}

	if bootstrapReadyCondition := calculateBootstrapConfigReadyForSummary(machine); bootstrapReadyCondition != nil {
		overrideConditions = append(overrideConditions, *bootstrapReadyCondition)
	}

	if len(overrideConditions) > 0 {
		summaryOpts = append(summaryOpts, overrideConditions)
	}

	readyCondition, err := v1beta2conditions.NewSummaryCondition(machine, clusterv1.MachineReadyV1Beta2Condition, summaryOpts...)

	if err != nil {
		// Note, this could only happen if we hit edge cases in computing the summary, which should not happen due to the fact
		// that we are passing a non empty list of ForConditionTypes.
		log.Error(err, "Failed to set Ready condition")
		readyCondition = &metav1.Condition{
			Type:    clusterv1.MachineReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineReadyInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		}
	}

	v1beta2conditions.Set(machine, *readyCondition)
}

// calculateDeletingConditionForSummary calculates a Deleting condition for the calculation of the Ready condition
// (which is done via a summary). This is necessary to avoid including the verbose details of the Deleting condition
// message in the summary.
// This is also important to ensure we have a limited amount of unique messages across Machines thus allowing to
// nicely aggregate Ready conditions from many Machines into the MachinesReady condition of e.g. the MachineSet.
// For the same reason we are only surfacing messages with "more than 30m" instead of using the exact durations.
// 30 minutes is a duration after which we assume it makes sense to emphasize that Node drains and waiting for volume
// detach are still in progress.
func calculateDeletingConditionForSummary(machine *clusterv1.Machine) v1beta2conditions.ConditionWithOwnerInfo {
	deletingCondition := v1beta2conditions.Get(machine, clusterv1.MachineDeletingV1Beta2Condition)

	var msg string
	switch {
	case deletingCondition == nil:
		// NOTE: this should never happen given that setDeletingCondition is called before this method and
		// it always adds a Deleting condition.
		msg = "Machine deletion in progress"
	case deletingCondition.Reason == clusterv1.MachineDeletingDrainingNodeV1Beta2Reason &&
		machine.Status.Deletion != nil && machine.Status.Deletion.NodeDrainStartTime != nil &&
		time.Since(machine.Status.Deletion.NodeDrainStartTime.Time) > 30*time.Minute:
		msg = fmt.Sprintf("Machine deletion in progress, stage: %s (since more than 30m)", deletingCondition.Reason)
	case deletingCondition.Reason == clusterv1.MachineDeletingWaitingForVolumeDetachV1Beta2Reason &&
		machine.Status.Deletion != nil && machine.Status.Deletion.WaitForNodeVolumeDetachStartTime != nil &&
		time.Since(machine.Status.Deletion.WaitForNodeVolumeDetachStartTime.Time) > 30*time.Minute:
		msg = fmt.Sprintf("Machine deletion in progress, stage: %s (since more than 30m)", deletingCondition.Reason)
	default:
		msg = fmt.Sprintf("Machine deletion in progress, stage: %s", deletingCondition.Reason)
	}

	return v1beta2conditions.ConditionWithOwnerInfo{
		OwnerResource: v1beta2conditions.ConditionOwnerInfo{
			Kind: "Machine",
			Name: machine.Name,
		},
		Condition: metav1.Condition{
			Type:    clusterv1.MachineDeletingV1Beta2Condition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachineDeletingV1Beta2Reason,
			Message: msg,
		},
	}
}

func calculateInfrastructureReadyForSummary(machine *clusterv1.Machine) *v1beta2conditions.ConditionWithOwnerInfo {
	infrastructureReadyCondition := v1beta2conditions.Get(machine, clusterv1.MachineInfrastructureReadyV1Beta2Condition)

	if infrastructureReadyCondition == nil {
		return nil
	}

	message := infrastructureReadyCondition.Message
	if infrastructureReadyCondition.Status == metav1.ConditionTrue && infrastructureReadyCondition.Message == infrastructureReadyFallBackMessage(machine.Spec.InfrastructureRef.Kind, machine.Status.InfrastructureReady) {
		message = ""
	}

	return &v1beta2conditions.ConditionWithOwnerInfo{
		OwnerResource: v1beta2conditions.ConditionOwnerInfo{
			Kind: "Machine",
			Name: machine.Name,
		},
		Condition: metav1.Condition{
			Type:    infrastructureReadyCondition.Type,
			Status:  infrastructureReadyCondition.Status,
			Reason:  infrastructureReadyCondition.Reason,
			Message: message,
		},
	}
}

func calculateBootstrapConfigReadyForSummary(machine *clusterv1.Machine) *v1beta2conditions.ConditionWithOwnerInfo {
	bootstrapConfigReadyCondition := v1beta2conditions.Get(machine, clusterv1.MachineBootstrapConfigReadyV1Beta2Condition)
	if bootstrapConfigReadyCondition == nil {
		return nil
	}

	message := bootstrapConfigReadyCondition.Message
	if bootstrapConfigReadyCondition.Status == metav1.ConditionTrue && machine.Spec.Bootstrap.ConfigRef != nil && bootstrapConfigReadyCondition.Message == bootstrapConfigReadyFallBackMessage(machine.Spec.Bootstrap.ConfigRef.Kind, machine.Status.BootstrapReady) {
		message = ""
	}

	return &v1beta2conditions.ConditionWithOwnerInfo{
		OwnerResource: v1beta2conditions.ConditionOwnerInfo{
			Kind: "Machine",
			Name: machine.Name,
		},
		Condition: metav1.Condition{
			Type:    bootstrapConfigReadyCondition.Type,
			Status:  bootstrapConfigReadyCondition.Status,
			Reason:  bootstrapConfigReadyCondition.Reason,
			Message: message,
		},
	}
}

func setAvailableCondition(ctx context.Context, machine *clusterv1.Machine) {
	log := ctrl.LoggerFrom(ctx)
	readyCondition := v1beta2conditions.Get(machine, clusterv1.MachineReadyV1Beta2Condition)

	if readyCondition == nil {
		// NOTE: this should never happen given that setReadyCondition is called before this method and
		// it always add a ready condition.
		log.Error(errors.New("Ready condition must be set before setting the available condition"), "Failed to set Available condition")
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineAvailableInternalErrorV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if readyCondition.Status != metav1.ConditionTrue {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineAvailableV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineNotReadyV1Beta2Reason,
		})
		return
	}

	if time.Since(readyCondition.LastTransitionTime.Time) >= 0*time.Second { // TODO: use MinReadySeconds as soon as it is available (and fix corresponding unit test)
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineAvailableV1Beta2Condition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineAvailableV1Beta2Reason,
		})
		return
	}

	v1beta2conditions.Set(machine, metav1.Condition{
		Type:   clusterv1.MachineAvailableV1Beta2Condition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.MachineWaitingForMinReadySecondsV1Beta2Reason,
	})
}

func setMachinePhaseAndLastUpdated(_ context.Context, m *clusterv1.Machine) {
	originalPhase := m.Status.Phase

	// Set the phase to "pending" if nil.
	if m.Status.Phase == "" {
		m.Status.SetTypedPhase(clusterv1.MachinePhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if m.Status.BootstrapReady && !m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioning)
	}

	// Set the phase to "provisioned" if there is a provider ID.
	if m.Spec.ProviderID != nil {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioned)
	}

	// Set the phase to "running" if there is a NodeRef field and infrastructure is ready.
	if m.Status.NodeRef != nil && m.Status.InfrastructureReady {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseRunning)
	}

	// Set the phase to "failed" if any of Status.FailureReason or Status.FailureMessage is not-nil.
	if m.Status.FailureReason != nil || m.Status.FailureMessage != nil {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseFailed)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !m.DeletionTimestamp.IsZero() {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseDeleting)
	}

	// If the phase has changed, update the LastUpdated timestamp
	if m.Status.Phase != originalPhase {
		now := metav1.Now()
		m.Status.LastUpdated = &now
	}
}
