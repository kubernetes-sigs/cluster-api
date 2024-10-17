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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
)

// reconcileStatus reconciles Machine's status during the entire lifecycle of the machine.
// This implies that the code in this function should account for several edge cases e.g. machine being partially provisioned,
// machine being partially deleted but also for running machines being disrupted e.g. by deleting the node.
// Additionally, this func should ensure that the conditions managed by this controller are always set in order to
// comply with the recommendation in the Kubernetes API guidelines.
// Note: v1beta1 conditions are not managed by this func.
func (r *Reconciler) reconcileStatus(ctx context.Context, s *scope) {
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
	setNodeHealthyAndReadyConditions(ctx, s.machine, s.node)

	// Updates Machine status not observed from Bootstrap Config, InfraMachine or Node (update Machine's own status).
	// Note: some of the status are set in reconcileCertificateExpiry (e.g.status.CertificatesExpiryDate),
	// in reconcileDelete (e.g. status.Deletion nested fields), and also in the defer patch at the end of the main reconcile loop (status.ObservedGeneration) etc.
	// Note: also other controllers adds conditions to the machine object (machine's owner controller sets the UpToDate condition,
	// MHC controller sets HealthCheckSucceeded and OwnerRemediated conditions, KCP sets conditions about etcd and control plane pods).

	// TODO: Set the uptodate condition for standalone pods

	setReadyCondition(ctx, s.machine)

	setAvailableCondition(ctx, s.machine)

	// TODO: Update the Deleting condition.

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
				Message: fmt.Sprintf("%s status.ready is %t", machine.Spec.Bootstrap.ConfigRef.Kind, machine.Status.BootstrapReady),
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

func setInfrastructureReadyCondition(_ context.Context, machine *clusterv1.Machine, infraMachine *unstructured.Unstructured, infraMachineIsNotFound bool) {
	if infraMachine != nil {
		if err := v1beta2conditions.SetMirrorConditionFromUnstructured(
			infraMachine, machine,
			contract.InfrastructureMachine().ReadyConditionType(), v1beta2conditions.TargetConditionType(clusterv1.MachineInfrastructureReadyV1Beta2Condition),
			v1beta2conditions.FallbackCondition{
				Status:  v1beta2conditions.BoolToStatus(machine.Status.InfrastructureReady),
				Reason:  clusterv1.MachineInfrastructureReadyNoReasonReportedV1Beta2Reason,
				Message: fmt.Sprintf("%s status.ready is %t", machine.Spec.InfrastructureRef.Kind, machine.Status.InfrastructureReady),
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

func setNodeHealthyAndReadyConditions(ctx context.Context, machine *clusterv1.Machine, node *corev1.Node) {
	// TODO: handle disconnected clusters when the new ClusterCache is merged

	if node != nil {
		var nodeReady *metav1.Condition
		for _, condition := range node.Status.Conditions {
			if condition.Type != corev1.NodeReady {
				continue
			}

			message := ""
			if condition.Message != "" {
				message = fmt.Sprintf("%s (from Node)", condition.Message)
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
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
				Message: fmt.Sprintf("Node %s has been deleted", machine.Status.NodeRef.Name),
			})

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
				Message: fmt.Sprintf("Node %s has been deleted", machine.Status.NodeRef.Name),
			})
			return
		}

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
			Message: "Node does not exist",
		})

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
			Message: "Node does not exist",
		})
		return
	}

	// Report an issue if node missing after being initialized.
	if machine.Status.NodeRef != nil {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse, // setting to false to keep it consistent with node below.
			Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
			Message: fmt.Sprintf("Node %s has been deleted while the machine still exists", machine.Status.NodeRef.Name),
		})

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status:  metav1.ConditionFalse, // setting to false to give more relevance in the ready condition summary.
			Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
			Message: fmt.Sprintf("Node %s has been deleted while the machine still exists", machine.Status.NodeRef.Name),
		})
		return
	}

	// If the machine is at the end of the provisioning phase, with ProviderID set, but still waiting
	// for a matching Node to exists, surface this.
	if ptr.Deref(machine.Spec.ProviderID, "") != "" {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
			Message: fmt.Sprintf("Waiting for a Node with spec.providerID %s to exist", *machine.Spec.ProviderID),
		})

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
			Message: fmt.Sprintf("Waiting for a Node with spec.providerID %s to exist", *machine.Spec.ProviderID),
		})
		return
	}

	// If the machine is at the beginning of the provisioning phase, with ProviderID not yet set, surface this.
	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
		Status:  metav1.ConditionUnknown,
		Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
		Message: fmt.Sprintf("Waiting for %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind),
	})

	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
		Status:  metav1.ConditionUnknown,
		Reason:  clusterv1.MachineNodeDoesNotExistV1Beta2Reason,
		Message: fmt.Sprintf("Waiting for %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind),
	})
}

// summarizeNodeV1Beta2Conditions summarizes a Node's conditions (NodeReady, NodeMemoryPressure, NodeDiskPressure, NodePIDPressure).
// the summary is computed in way that is similar to how v1beta2conditions.NewSummaryCondition works, but in this case the
// implementation is simpler/less flexible and it surfaces only issues & unknown conditions.
func summarizeNodeV1Beta2Conditions(_ context.Context, node *corev1.Node) (metav1.ConditionStatus, string, string) {
	semanticallyFalseStatus := 0
	unknownStatus := 0

	message := ""
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
			message += fmt.Sprintf("Node %s: condition not yet reported", conditionType) + "; "
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
				message += fmt.Sprintf("Node %s: condition is %s", condition.Type, condition.Status) + "; "
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
				message += fmt.Sprintf("Node %s: condition is %s", condition.Type, condition.Status) + "; "
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

	message = strings.TrimSuffix(message, "; ")
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
	machine *clusterv1.Machine
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
		return v1beta2conditions.GetDefaultMergePriorityFunc(nil)(condition)
	}).Merge(conditions, conditionTypes)
}

func setReadyCondition(ctx context.Context, machine *clusterv1.Machine) {
	log := ctrl.LoggerFrom(ctx)

	forConditionTypes := v1beta2conditions.ForConditionTypes{
		// TODO: add machine deleting condition once implemented.
		clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
		clusterv1.MachineInfrastructureReadyV1Beta2Condition,
		clusterv1.MachineNodeHealthyV1Beta2Condition,
		clusterv1.MachineHealthCheckSucceededV1Beta2Condition,
	}
	for _, g := range machine.Spec.ReadinessGates {
		forConditionTypes = append(forConditionTypes, g.ConditionType)
	}
	readyCondition, err := v1beta2conditions.NewSummaryCondition(machine, clusterv1.MachineReadyV1Beta2Condition, forConditionTypes,
		v1beta2conditions.IgnoreTypesIfMissing{clusterv1.MachineHealthCheckSucceededV1Beta2Condition},
		v1beta2conditions.CustomMergeStrategy{
			MergeStrategy: machineConditionCustomMergeStrategy{machine: machine},
		},
	)
	if err != nil || readyCondition == nil {
		// Note, this could only happen if we hit edge cases in computing the summary, which should not happen due to the fact
		// that we are passing a non empty list of ForConditionTypes.
		log.Error(err, "Failed to set ready condition")
		readyCondition = &metav1.Condition{
			Type:    clusterv1.MachineReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineErrorComputingReadyV1Beta2Reason,
			Message: "Please check controller logs for errors",
		}
	}

	v1beta2conditions.Set(machine, *readyCondition)
}

func setAvailableCondition(_ context.Context, machine *clusterv1.Machine) {
	readyCondition := v1beta2conditions.Get(machine, clusterv1.MachineReadyV1Beta2Condition)

	if readyCondition == nil {
		// NOTE: this should never happen given that setReadyCondition is called before this method and
		// it always add a ready condition.
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineAvailableV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineReadyNotYetReportedV1Beta2Reason,
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
