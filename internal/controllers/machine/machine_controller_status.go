/*
Copyright 2025 The Kubernetes Authors.

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
	"k8s.io/klog/v2"
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
	// Note: the Following Status fields are managed in reconcileBootstrap.
	// - status.BootstrapReady
	// - status.Addresses
	// - status.FailureReason
	// - status.FailureMessage
	setBootstrapReadyCondition(ctx, s.machine, s.bootstrapConfig)

	// Update status from the InfraMachine external resource.
	// Note: the Following Status field are managed in reconcileInfrastructure.
	// - status.InfrastructureReady
	// - status.FailureReason
	// - status.FailureMessage
	setInfrastructureReadyCondition(ctx, s.machine, s.infraMachine)

	// Update status from the Node external resource.
	// Note: the Following Status field are managed in reconcileNode.
	// - status.NodeRef
	// - status.NodeInfo
	setNodeHealthyAndReadyConditions(ctx, s.machine, s.node)

	// Updates Machine status not observed from Bootstrap Config, InfraMachine or Node (update Machine's own status).
	//  Note:
	//	- status.CertificatesExpiryDate is managed in reconcileCertificateExpiry.
	//  - status.ObservedGeneration is updated by the defer patch at the end of the main reconcile loop.
	//  - status.Deletion nested fields are updated in reconcileDelete.
	//  - UpToDate condition is set by machine's owner controller. // TODO: compute UpToDate for stand alone machines
	//  - HealthCheckSucceeded is set by the MHC controller.
	//  - OwnerRemediated conditions is set by the MHC controller, but the it is updated by the controller owning the machine
	//    while it carries over the remediation process.

	setReadyCondition(ctx, s.machine)

	setAvailableCondition(ctx, s.machine)

	// TODO: Update the Deleting condition.

	setPausedCondition(s)

	setMachinePhaseAndLastUpdated(ctx, s.machine)
}

func setBootstrapReadyCondition(_ context.Context, machine *clusterv1.Machine, bootstrapConfig *unstructured.Unstructured) {
	if machine.Spec.Bootstrap.ConfigRef == nil {
		if ptr.Deref(machine.Spec.Bootstrap.DataSecretName, "") != "" {
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineBootstrapDataSecretDataSecretUserProvidedV1Beta2Reason,
			})
			return
		}

		// Note: validation web hooks should prevent invalid configuration to happen.
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineBootstrapInvalidConfigV1Beta2Reason,
			Message: "either spec.bootstrap.configRef must be set or spec.bootstrap.dataSecretName must not be empty",
		})
		return
	}

	if bootstrapConfig != nil {
		if err := v1beta2conditions.SetMirrorConditionFromUnstructured(
			bootstrapConfig, machine,
			contract.Bootstrap().ReadyConditionType(), v1beta2conditions.TargetConditionType(clusterv1.MachineBootstrapConfigReadyV1Beta2Condition),
			v1beta2conditions.FallbackCondition{
				Status: v1beta2conditions.BoolToStatus(machine.Status.BootstrapReady),
				Reason: clusterv1.MachineBootstrapConfigReadyNoV1Beta2ReasonReported,
			},
		); err != nil {
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineBootstrapConfigInvalidConditionReportedV1Beta2Reason,
				Message: fmt.Sprintf("%s %s reports an invalid %s condition: %s", machine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(machine.Namespace, machine.Spec.Bootstrap.ConfigRef.Name), contract.Bootstrap().ReadyConditionType(), err.Error()),
			})
		}
		return
	}

	// Tolerate Bootstrap config missing when the machine is deleting.
	// NOTE: this code assumes that Bootstrap config deletion has been initiated by the controller itself,
	// and thus this state is reported as Deleted instead of NotFound.
	if !machine.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineBootstrapConfigDeletedV1Beta2Reason,
			Message: fmt.Sprintf("%s %s has been deleted", machine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(machine.Namespace, machine.Spec.Bootstrap.ConfigRef.Name)),
		})
		return
	}

	// If the machine is not deleting, and boostrap config object does not exist yet,
	// surface the fact that the controller is waiting for the bootstrap config to exist, which could
	// happen when creating the machine. However, this state should be treated as an error if it last indefinitely.
	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
		Status:  metav1.ConditionUnknown,
		Reason:  clusterv1.MachineBootstrapConfigNotFoundV1Beta2Reason,
		Message: fmt.Sprintf("waiting for %s %s to exist", machine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(machine.Namespace, machine.Spec.Bootstrap.ConfigRef.Name)),
	})
}

func setInfrastructureReadyCondition(_ context.Context, machine *clusterv1.Machine, infraMachine *unstructured.Unstructured) {
	if infraMachine != nil {
		if err := v1beta2conditions.SetMirrorConditionFromUnstructured(
			infraMachine, machine,
			contract.InfrastructureMachine().ReadyConditionType(), v1beta2conditions.TargetConditionType(clusterv1.MachineInfrastructureReadyV1Beta2Condition),
			v1beta2conditions.FallbackCondition{
				Status: v1beta2conditions.BoolToStatus(machine.Status.InfrastructureReady),
				Reason: clusterv1.MachineInfrastructureReadyNoV1Beta2ReasonReported,
			},
		); err != nil {
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureInvalidConditionReportedV1Beta2Reason,
				Message: fmt.Sprintf("%s %s reports an invalid %s condition: %s", machine.Spec.Bootstrap.ConfigRef.Kind, klog.KRef(machine.Namespace, machine.Spec.Bootstrap.ConfigRef.Name), contract.InfrastructureMachine().ReadyConditionType(), err.Error()),
			})
		}
		return
	}

	// Tolerate infra machine missing when the machine is deleting.
	// NOTE: this code assumes that infra machine deletion has been initiated by the controller itself,
	// and thus this state is reported as Deleted instead of NotFound.
	if !machine.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineInfrastructureReadyV1Beta2Condition,
			Status: metav1.ConditionUnknown,
			Reason: clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
		})
		return
	}

	// Report an issue if infra machine missing after the machine has been initialized.
	if machine.Status.InfrastructureReady {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineInfrastructureReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineInfrastructureDeletedV1Beta2Reason,
			Message: fmt.Sprintf("%s %s has been deleted while the machine still exist", machine.Spec.Bootstrap.ConfigRef.Kind, machine.Name),
		})
		return
	}

	// If the machine is not deleting, and infra machine object does not exist yet,
	// surface the fact that the controller is waiting for the infra machine to exist, which could
	// happen when creating the machine. However, this state should be treated as an error if it last indefinitely.
	v1beta2conditions.Set(machine, metav1.Condition{
		Type:   clusterv1.MachineBootstrapConfigReadyV1Beta2Condition,
		Status: metav1.ConditionUnknown,
		Reason: clusterv1.MachineInfrastructureNotFoundV1Beta2Reason,
	})
}

func setNodeHealthyAndReadyConditions(ctx context.Context, machine *clusterv1.Machine, node *corev1.Node) {
	if node != nil {
		var nodeReady *metav1.Condition
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				nodeReady = &metav1.Condition{
					Type:               clusterv1.MachineNodeReadyV1Beta2Condition,
					Status:             metav1.ConditionStatus(condition.Status),
					LastTransitionTime: condition.LastTransitionTime,
					Reason:             condition.Reason,
					Message:            condition.Message,
				}
			}
		}

		if nodeReady == nil {
			nodeReady = &metav1.Condition{
				Type:   clusterv1.MachineNodeReadyV1Beta2Condition,
				Status: metav1.ConditionUnknown,
				Reason: v1beta2conditions.NotYetReportedReason,
			}
		}
		v1beta2conditions.Set(machine, *nodeReady)

		status, reason, message := summarizeNodeV1Beta2Conditions(ctx, node)
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status:  metav1.ConditionStatus(status),
			Reason:  reason,
			Message: message,
		})

		return
	}

	// Tolerate node missing when the machine is deleting.
	// NOTE: controllers always assume that node deletion has been initiated by the controller itself,
	// and thus this state is reported as Deleted instead of NotFound.
	if !machine.DeletionTimestamp.IsZero() {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineNodeReadyV1Beta2Condition,
			Status: metav1.ConditionUnknown,
			Reason: clusterv1.MachineNodeDeletedV1Beta2Reason,
		})

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status: metav1.ConditionUnknown,
			Reason: clusterv1.MachineNodeDeletedV1Beta2Reason,
		})
		return
	}

	// Report an issue if node missing after being initialized.
	if machine.Status.NodeRef != nil {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
			Message: fmt.Sprintf("Node %s has been deleted while the machine still exist", machine.Status.NodeRef.Name),
		})

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineNodeDeletedV1Beta2Reason,
			Message: fmt.Sprintf("Node %s has been deleted while the machine still exist", machine.Status.NodeRef.Name),
		})
		return
	}

	if ptr.Deref(machine.Spec.ProviderID, "") != "" {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
			Message: fmt.Sprintf("Waiting for a node with Provider ID %s to exist", *machine.Spec.ProviderID),
		})

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
			Message: fmt.Sprintf("Waiting for a node with Provider ID %s to exist", *machine.Spec.ProviderID),
		})
		return
	}

	// Surface the fact that the controller is waiting for the bootstrap config to exist, which could
	// happen when creating the machine. However, this state should be treated as an error if it last indefinitely.
	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineNodeReadyV1Beta2Condition,
		Status:  metav1.ConditionUnknown,
		Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
		Message: fmt.Sprintf("Waiting for %s %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind, klog.KRef(machine.Spec.InfrastructureRef.Namespace, machine.Spec.InfrastructureRef.Name)),
	})

	v1beta2conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineNodeHealthyV1Beta2Condition,
		Status:  metav1.ConditionUnknown,
		Reason:  clusterv1.MachineNodeNotFoundV1Beta2Reason,
		Message: fmt.Sprintf("Waiting for %s %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind, klog.KRef(machine.Spec.InfrastructureRef.Namespace, machine.Spec.InfrastructureRef.Name)),
	})
}

func summarizeNodeV1Beta2Conditions(_ context.Context, node *corev1.Node) (corev1.ConditionStatus, string, string) {
	semanticallyFalseStatus := 0
	unknownStatus := 0

	message := ""
	issueReason := ""
	unknownReason := ""
	for _, condition := range node.Status.Conditions {
		switch condition.Type {
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
			if condition.Status != corev1.ConditionFalse {
				message += fmt.Sprintf("Node's %s condition is %s", condition.Type, condition.Status) + ". "
				if condition.Status == corev1.ConditionUnknown {
					if unknownReason == "" {
						unknownReason = condition.Reason
					} else {
						unknownReason = v1beta2conditions.MultipleUnknownReportedReason
					}
					unknownStatus++
					continue
				}
				if issueReason == "" {
					issueReason = condition.Reason
				} else {
					issueReason = v1beta2conditions.MultipleIssuesReportedReason
				}
				semanticallyFalseStatus++
			}
		case corev1.NodeReady:
			if condition.Status != corev1.ConditionTrue {
				message += fmt.Sprintf("Node's %s condition is %s", condition.Type, condition.Status) + ". "
				if condition.Status == corev1.ConditionUnknown {
					if unknownReason == "" {
						unknownReason = condition.Reason
					} else {
						unknownReason = v1beta2conditions.MultipleUnknownReportedReason
					}
					unknownStatus++
					continue
				}
				if issueReason == "" {
					issueReason = condition.Reason
				} else {
					issueReason = v1beta2conditions.MultipleIssuesReportedReason
				}
				semanticallyFalseStatus++
			}
		}
	}
	message = strings.TrimSuffix(message, ". ")
	if semanticallyFalseStatus > 0 {
		return corev1.ConditionFalse, issueReason, message
	}
	if semanticallyFalseStatus+unknownStatus > 0 {
		return corev1.ConditionUnknown, unknownReason, message
	}
	return corev1.ConditionTrue, v1beta2conditions.MultipleInfoReportedReason, message
}

func setReadyCondition(ctx context.Context, machine *clusterv1.Machine) {
	log := ctrl.LoggerFrom(ctx)

	forConditionTypes := v1beta2conditions.ForConditionTypes{
		// TODO: add machine deleting once implemented.
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
		// TODO: think about the step counter
	)
	if err != nil || readyCondition == nil {
		// Note, this could only happen if we hit edge cases in computing the summary, which should not happen due to the fact
		// that we are passing a non empty list of ForConditionTypes.
		log.Error(err, "failed to set ready condition")
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

	if time.Now().Add(0).After(readyCondition.LastTransitionTime.Time) {
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineAvailableV1Beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineWaitingForMinReadySecondsV1Beta2Reason,
		})
		return
	}

	v1beta2conditions.Set(machine, metav1.Condition{
		Type:   clusterv1.MachineAvailableV1Beta2Condition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.MachineAvailableV1Beta2Reason,
	})
}

func setPausedCondition(s *scope) {
	// Note: If we hit this code, the controller is reconciling and this Paused condition must be set to false.
	v1beta2conditions.Set(s.machine, metav1.Condition{
		Type:   clusterv1.MachinePausedV1Beta2Condition,
		Status: metav1.ConditionFalse,
		Reason: "NotPaused", // TODO: create a const.
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
