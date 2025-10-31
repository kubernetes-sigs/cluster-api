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

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// updateStatus update Machine's status.
// This implies that the code in this function should account for several edge cases e.g. machine being partially provisioned,
// machine being partially deleted but also for running machines being disrupted e.g. by deleting the node.
// Additionally, this func should ensure that the conditions managed by this controller are always set in order to
// comply with the recommendation in the Kubernetes API guidelines.
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
	healthCheckingState := r.ClusterCache.GetHealthCheckingState(ctx, client.ObjectKeyFromObject(s.cluster))
	setNodeHealthyAndReadyConditions(ctx, s.cluster, s.machine, s.node, s.nodeGetError, healthCheckingState, r.RemoteConditionsGracePeriod)

	// Updates Machine status not observed from Bootstrap Config, InfraMachine or Node (update Machine's own status).
	// Note: some of the status are set in reconcileCertificateExpiry (e.g.status.CertificatesExpiryDate),
	// in reconcileDelete (e.g. status.Deletion nested fields), and also in the defer patch at the end of the main reconcile loop (status.ObservedGeneration) etc.
	// Note: also other controllers adds conditions to the machine object (machine's owner controller sets the UpToDate condition,
	// MHC controller sets HealthCheckSucceeded and OwnerRemediated conditions, KCP sets conditions about etcd and control plane pods).
	setDeletingCondition(ctx, s.machine, s.reconcileDeleteExecuted, s.deletingReason, s.deletingMessage)
	setUpdatingCondition(ctx, s.machine, s.updatingReason, s.updatingMessage)
	setReadyCondition(ctx, s.machine)
	setAvailableCondition(ctx, s.machine)

	setMachinePhaseAndLastUpdated(ctx, s.machine)
}

func setBootstrapReadyCondition(_ context.Context, machine *clusterv1.Machine, bootstrapConfig *unstructured.Unstructured, bootstrapConfigIsNotFound bool) {
	if !machine.Spec.Bootstrap.ConfigRef.IsDefined() {
		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineBootstrapConfigReadyCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineBootstrapDataSecretProvidedReason,
		})
		return
	}

	if bootstrapConfig != nil {
		dataSecretCreated := ptr.Deref(machine.Status.Initialization.BootstrapDataSecretCreated, false)
		ready, err := conditions.NewMirrorConditionFromUnstructured(
			bootstrapConfig,
			contract.Bootstrap().ReadyConditionType(), conditions.TargetConditionType(clusterv1.MachineBootstrapConfigReadyCondition),
			conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(dataSecretCreated),
				Reason:  fallbackReason(dataSecretCreated, clusterv1.MachineBootstrapConfigReadyReason, clusterv1.MachineBootstrapConfigNotReadyReason),
				Message: bootstrapConfigReadyFallBackMessage(machine.Spec.Bootstrap.ConfigRef.Kind, dataSecretCreated),
			},
		)
		if err != nil {
			conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineBootstrapConfigReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineBootstrapConfigInvalidConditionReportedReason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if ready.Reason == conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.MachineBootstrapConfigReadyReason
		}
		conditions.Set(machine, *ready)
		return
	}

	// If we got unexpected errors in reading the bootstrap config (this should happen rarely), surface them
	if !bootstrapConfigIsNotFound {
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineBootstrapConfigReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineBootstrapConfigInternalErrorReason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileBootstrap.
		})
		return
	}

	// Bootstrap config missing when the machine is deleting and we know that the BootstrapConfig actually existed.
	if !machine.DeletionTimestamp.IsZero() && ptr.Deref(machine.Status.Initialization.BootstrapDataSecretCreated, false) {
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineBootstrapConfigReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineBootstrapConfigDeletedReason,
			Message: fmt.Sprintf("%s has been deleted", machine.Spec.Bootstrap.ConfigRef.Kind),
		})
		return
	}

	// If the machine is not deleting, and boostrap config object does not exist,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the machine and all the objects referenced by it (provisioning yet to start/started, but status.nodeRef not yet set).
	// - when the machine has been provisioned
	conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineBootstrapConfigReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineBootstrapConfigDoesNotExistReason,
		Message: fmt.Sprintf("%s does not exist", machine.Spec.Bootstrap.ConfigRef.Kind),
	})
}

func fallbackReason(status bool, trueReason, falseReason string) string {
	if status {
		return trueReason
	}
	return falseReason
}

func bootstrapConfigReadyFallBackMessage(kind string, ready bool) string {
	if ready {
		return ""
	}
	return fmt.Sprintf("%s status.initialization.dataSecretCreated is %t", kind, ready)
}

func setInfrastructureReadyCondition(_ context.Context, machine *clusterv1.Machine, infraMachine *unstructured.Unstructured, infraMachineIsNotFound bool) {
	if infraMachine != nil {
		infrastructureProvisioned := ptr.Deref(machine.Status.Initialization.InfrastructureProvisioned, false)
		ready, err := conditions.NewMirrorConditionFromUnstructured(
			infraMachine,
			contract.InfrastructureMachine().ReadyConditionType(), conditions.TargetConditionType(clusterv1.MachineInfrastructureReadyCondition),
			conditions.FallbackCondition{
				Status:  conditions.BoolToStatus(infrastructureProvisioned),
				Reason:  fallbackReason(infrastructureProvisioned, clusterv1.MachineInfrastructureReadyReason, clusterv1.MachineInfrastructureNotReadyReason),
				Message: infrastructureReadyFallBackMessage(machine.Spec.InfrastructureRef.Kind, infrastructureProvisioned),
			},
		)
		if err != nil {
			conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineInfrastructureInvalidConditionReportedReason,
				Message: err.Error(),
			})
			return
		}

		// In case condition has NoReasonReported and status true, we assume it is a v1beta1 condition
		// and replace the reason with something less confusing.
		if ready.Reason == conditions.NoReasonReported && ready.Status == metav1.ConditionTrue {
			ready.Reason = clusterv1.MachineInfrastructureReadyReason
		}
		conditions.Set(machine, *ready)
		return
	}

	// If we got errors in reading the infra machine (this should happen rarely), surface them
	if !infraMachineIsNotFound {
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineInfrastructureReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineInfrastructureInternalErrorReason,
			Message: "Please check controller logs for errors",
			// NOTE: the error is logged by reconcileInfrastructure.
		})
		return
	}

	// Infra machine missing when the machine is deleting.
	// NOTE: in case an accidental deletion happens before volume detach is completed, the Node hosted on the Machine
	// will be considered unreachable Machine deletion will complete.
	if !machine.DeletionTimestamp.IsZero() {
		if ptr.Deref(machine.Status.Initialization.InfrastructureProvisioned, false) {
			conditions.Set(machine, metav1.Condition{
				Type:    clusterv1.MachineInfrastructureReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  clusterv1.MachineInfrastructureDeletedReason,
				Message: fmt.Sprintf("%s has been deleted", machine.Spec.InfrastructureRef.Kind),
			})
			return
		}

		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineInfrastructureReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineInfrastructureDoesNotExistReason,
			Message: fmt.Sprintf("%s does not exist", machine.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// Report an issue if infra machine missing after the machine has been initialized (and the machine is still running).
	if ptr.Deref(machine.Status.Initialization.InfrastructureProvisioned, false) {
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineInfrastructureReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineInfrastructureDeletedReason,
			Message: fmt.Sprintf("%s has been deleted while the Machine still exists", machine.Spec.InfrastructureRef.Kind),
		})
		return
	}

	// If the machine is not deleting, and infra machine object does not exist yet,
	// surface this fact. This could happen when:
	// - when applying the yaml file with the machine and all the objects referenced by it (provisioning yet to start/started, but status.InfrastructureReady not yet set).
	conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineInfrastructureReadyCondition,
		Status:  metav1.ConditionFalse,
		Reason:  clusterv1.MachineInfrastructureDoesNotExistReason,
		Message: fmt.Sprintf("%s does not exist", machine.Spec.InfrastructureRef.Kind),
	})
}

func infrastructureReadyFallBackMessage(kind string, ready bool) string {
	if ready {
		return ""
	}
	return fmt.Sprintf("%s status.initialization.provisioned is %t", kind, ready)
}

func setNodeHealthyAndReadyConditions(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, node *corev1.Node, nodeGetErr error, healthCheckingState clustercache.HealthCheckingState, remoteConditionsGracePeriod time.Duration) {
	if !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeInspectionFailedReason,
			"Waiting for Cluster status.initialization.infrastructureProvisioned to be true")
		return
	}

	controlPlaneInitialized := conditions.Get(cluster, clusterv1.ClusterControlPlaneInitializedCondition)
	if controlPlaneInitialized == nil || controlPlaneInitialized.Status != metav1.ConditionTrue {
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeInspectionFailedReason,
			"Waiting for Cluster control plane to be initialized")
		return
	}

	// ClusterCache did not try to connect often enough yet, either during controller startup or when a new Cluster is created.
	if healthCheckingState.LastProbeSuccessTime.IsZero() && healthCheckingState.ConsecutiveFailures < 5 {
		// If conditions are not set, set them to ConnectionDown.
		// Note: This will allow to keep reporting last known status in case there are temporary connection errors.
		if !conditions.Has(machine, clusterv1.MachineNodeReadyCondition) ||
			!conditions.Has(machine, clusterv1.MachineNodeHealthyCondition) {
			setNodeConditions(machine, metav1.ConditionUnknown,
				clusterv1.MachineNodeConnectionDownReason,
				"Remote connection not established yet")
		}
		return
	}

	// Remote conditions grace period is counted from the later of last probe success and control plane initialized.
	if time.Since(maxTime(healthCheckingState.LastProbeSuccessTime, controlPlaneInitialized.LastTransitionTime.Time)) > remoteConditionsGracePeriod {
		// Overwrite conditions to ConnectionDown.
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeConnectionDownReason,
			lastProbeSuccessMessage(healthCheckingState.LastProbeSuccessTime))
		return
	}

	if nodeGetErr != nil {
		if errors.Is(nodeGetErr, clustercache.ErrClusterNotConnected) {
			// If conditions are not set, set them to ConnectionDown.
			// Note: This will allow to keep reporting last known status in case there are temporary connection errors.
			// However, if connection errors persist more than remoteConditionsGracePeriod, conditions will be overridden.
			if !conditions.Has(machine, clusterv1.MachineNodeReadyCondition) ||
				!conditions.Has(machine, clusterv1.MachineNodeHealthyCondition) {
				setNodeConditions(machine, metav1.ConditionUnknown,
					clusterv1.MachineNodeConnectionDownReason,
					lastProbeSuccessMessage(healthCheckingState.LastProbeSuccessTime))
			}
			return
		}

		// Overwrite conditions to InternalError.
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeInternalErrorReason,
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
			if condition.Status != corev1.ConditionTrue && condition.Message != "" {
				message = fmt.Sprintf("* Node.Ready: %s", condition.Message)
			}

			reason := ""
			switch condition.Status {
			case corev1.ConditionFalse:
				reason = clusterv1.MachineNodeNotReadyReason
			case corev1.ConditionUnknown:
				reason = clusterv1.MachineNodeReadyUnknownReason
			case corev1.ConditionTrue:
				reason = clusterv1.MachineNodeReadyReason
			}
			nodeReady = &metav1.Condition{
				Type:               clusterv1.MachineNodeReadyCondition,
				Status:             metav1.ConditionStatus(condition.Status),
				LastTransitionTime: condition.LastTransitionTime,
				Reason:             reason,
				Message:            message,
			}
		}

		if nodeReady == nil {
			nodeReady = &metav1.Condition{
				Type:    clusterv1.MachineNodeReadyCondition,
				Status:  metav1.ConditionUnknown,
				Reason:  clusterv1.MachineNodeReadyUnknownReason,
				Message: "* Node.Ready: Condition not yet reported",
			}
		}
		conditions.Set(machine, *nodeReady)

		status, reason, message := summarizeNodeConditions(ctx, node)
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineNodeHealthyCondition,
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
		if machine.Status.NodeRef.IsDefined() {
			setNodeConditions(machine, metav1.ConditionFalse,
				clusterv1.MachineNodeDeletedReason,
				fmt.Sprintf("Node %s has been deleted", machine.Status.NodeRef.Name))
			return
		}

		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeDoesNotExistReason,
			"Node does not exist")
		return
	}

	// Report an issue if node missing after being initialized.
	if machine.Status.NodeRef.IsDefined() {
		// Setting MachineNodeHealthyCondition to False to give it more relevance in the Ready condition summary.
		// Setting MachineNodeReadyCondition to False to keep it consistent with MachineNodeHealthyCondition.
		setNodeConditions(machine, metav1.ConditionFalse,
			clusterv1.MachineNodeDeletedReason,
			fmt.Sprintf("Node %s has been deleted while the Machine still exists", machine.Status.NodeRef.Name))
		return
	}

	// If the machine is at the end of the provisioning phase, with ProviderID set, but still waiting
	// for a matching Node to exists, surface this.
	if machine.Spec.ProviderID != "" {
		setNodeConditions(machine, metav1.ConditionUnknown,
			clusterv1.MachineNodeInspectionFailedReason,
			fmt.Sprintf("Waiting for a Node with spec.providerID %s to exist", machine.Spec.ProviderID))
		return
	}

	// If the machine is at the beginning of the provisioning phase, with ProviderID not yet set, surface this.
	setNodeConditions(machine, metav1.ConditionUnknown,
		clusterv1.MachineNodeInspectionFailedReason,
		fmt.Sprintf("Waiting for %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind))
}

func setNodeConditions(machine *clusterv1.Machine, status metav1.ConditionStatus, reason, msg string) {
	for _, conditionType := range []string{clusterv1.MachineNodeReadyCondition, clusterv1.MachineNodeHealthyCondition} {
		conditions.Set(machine, metav1.Condition{
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

// summarizeNodeConditions summarizes a Node's conditions (NodeReady, NodeMemoryPressure, NodeDiskPressure, NodePIDPressure).
// the summary is computed in way that is similar to how conditions.NewSummaryCondition works, but in this case the
// implementation is simpler/less flexible and it surfaces only issues & unknown conditions.
func summarizeNodeConditions(_ context.Context, node *corev1.Node) (metav1.ConditionStatus, string, string) {
	semanticallyFalseStatus := 0
	unknownStatus := 0

	conditionCount := 0
	conditionMessages := sets.Set[string]{}
	messages := []string{}
	for _, conditionType := range []corev1.NodeConditionType{corev1.NodeReady, corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure} {
		var condition *corev1.NodeCondition
		for _, c := range node.Status.Conditions {
			if c.Type == conditionType {
				condition = &c
			}
		}
		if condition == nil {
			messages = append(messages, fmt.Sprintf("* Node.%s: Condition not yet reported", conditionType))
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
				conditionCount++
				conditionMessages.Insert(m)
				messages = append(messages, fmt.Sprintf("* Node.%s: %s", condition.Type, m))
				if condition.Status == corev1.ConditionUnknown {
					unknownStatus++
					continue
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
				conditionCount++
				conditionMessages.Insert(m)
				messages = append(messages, fmt.Sprintf("* Node.%s: %s", condition.Type, m))
				if condition.Status == corev1.ConditionUnknown {
					unknownStatus++
					continue
				}
				semanticallyFalseStatus++
			}
		}
	}

	if conditionCount > 1 && len(conditionMessages) == 1 {
		messages = []string{fmt.Sprintf("* Node.AllConditions: %s", conditionMessages.UnsortedList()[0])}
	}
	message := strings.Join(messages, "\n")
	if semanticallyFalseStatus > 0 {
		return metav1.ConditionFalse, clusterv1.MachineNodeNotHealthyReason, message
	}
	if unknownStatus > 0 {
		return metav1.ConditionUnknown, clusterv1.MachineNodeHealthUnknownReason, message
	}
	return metav1.ConditionTrue, clusterv1.MachineNodeHealthyReason, ""
}

type machineConditionCustomMergeStrategy struct {
	machine                        *clusterv1.Machine
	negativePolarityConditionTypes []string
}

func (c machineConditionCustomMergeStrategy) Merge(operation conditions.MergeOperation, mergeConditions []conditions.ConditionWithOwnerInfo, conditionTypes []string) (status metav1.ConditionStatus, reason, message string, err error) {
	return conditions.DefaultMergeStrategy(
		// While machine is deleting, treat unknown conditions from external objects as info (it is ok that those objects have been deleted at this stage).
		conditions.GetPriorityFunc(func(condition metav1.Condition) conditions.MergePriority {
			if !c.machine.DeletionTimestamp.IsZero() {
				if condition.Type == clusterv1.MachineBootstrapConfigReadyCondition && (condition.Reason == clusterv1.MachineBootstrapConfigDeletedReason || condition.Reason == clusterv1.MachineBootstrapConfigDoesNotExistReason) {
					return conditions.InfoMergePriority
				}
				if condition.Type == clusterv1.MachineInfrastructureReadyCondition && (condition.Reason == clusterv1.MachineInfrastructureDeletedReason || condition.Reason == clusterv1.MachineInfrastructureDoesNotExistReason) {
					return conditions.InfoMergePriority
				}
				if condition.Type == clusterv1.MachineNodeHealthyCondition && (condition.Reason == clusterv1.MachineNodeDeletedReason || condition.Reason == clusterv1.MachineNodeDoesNotExistReason) {
					return conditions.InfoMergePriority
				}
				// Note: MachineNodeReadyCondition is not relevant for the summary.
			}
			return conditions.GetDefaultMergePriorityFunc(c.negativePolarityConditionTypes...)(condition)
		}),
		// Group readiness gates for control plane and etcd conditions when they have the same messages.
		conditions.SummaryMessageTransformFunc(transformControlPlaneAndEtcdConditions),
		// Use custom reasons.
		conditions.ComputeReasonFunc(conditions.GetDefaultComputeMergeReasonFunc(
			clusterv1.MachineNotReadyReason,
			clusterv1.MachineReadyUnknownReason,
			clusterv1.MachineReadyReason,
		)),
	).Merge(operation, mergeConditions, conditionTypes)
}

// transformControlPlaneAndEtcdConditions Group readiness gates for control plane conditions when they have the same messages.
// Note: the implementation is based on KCP conditions, but ideally other control plane implementation could
// take benefit from this optimization by naming conditions with APIServer, ControllerManager, Scheduler prefix.
// In future we might consider to do something similar for etcd conditions.
func transformControlPlaneAndEtcdConditions(messages []string) []string {
	isControlPlaneCondition := func(c string) bool {
		if strings.HasPrefix(c, "* APIServer") {
			return true
		}
		if strings.HasPrefix(c, "* ControllerManager") {
			return true
		}
		if strings.HasPrefix(c, "* Scheduler") {
			return true
		}
		// Note. Etcd pod healthy is considered as part of control plane components in KCP
		// because it is not part of the etcd cluster.
		// Might be in future we want to make this check more strictly tight to KCP machines e.g. by checking the machine's owner;
		// for now, we consider checking for the exact condition name as an acceptable trade off (same below).
		if c == "* EtcdPodHealthy" {
			return true
		}
		return false
	}

	// Loop trough summary message.
	out := []string{}
	controlPlaneConditionsCount := 0
	controlPlaneMsg := ""
	for _, m := range messages {
		// Summary message are in the form of "* Condition: Message"; the following code
		// figure out if this is a control plane condition.
		sep := strings.Index(m, ":")
		if sep == -1 {
			out = append(out, m)
			continue
		}
		c, msg := m[:sep], m[sep+1:]
		if !isControlPlaneCondition(c) {
			// If the condition isn't a control plane condition, add to the output the message as is.
			out = append(out, m)
			continue
		}

		// If the condition is the first control plane condition we meet, add to the output
		// a message replacing the condition name with a placeholder for the control plane components
		if controlPlaneMsg == "" {
			controlPlaneMsg = msg
			out = append(out, fmt.Sprintf("* Control plane components:%s", msg))
			controlPlaneConditionsCount++
			continue
		}

		// If the condition is not the first control plane condition we meet, if the message is
		// different from the previous control plane condition, we can't group control plane components
		// so we return the same list of messages we got in input.
		// otherwise, continue looping into conditions.
		if controlPlaneMsg != msg {
			return messages
		}
		controlPlaneConditionsCount++
	}

	// If we met only 1 control plane component, return the same list of messages we got in input
	// so we are going to show the condition name instead of the placeholder for the control plane components.
	if controlPlaneConditionsCount == 1 {
		return messages
	}
	return out
}

func setDeletingCondition(_ context.Context, machine *clusterv1.Machine, reconcileDeleteExecuted bool, deletingReason, deletingMessage string) {
	if machine.DeletionTimestamp.IsZero() {
		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineDeletingCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineNotDeletingReason,
		})
		return
	}

	if !reconcileDeleteExecuted {
		// Don't update the Deleting condition if reconcileDelete was not executed (e.g.
		// because of rate-limiting).
		return
	}

	conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineDeletingCondition,
		Status:  metav1.ConditionTrue,
		Reason:  deletingReason,
		Message: deletingMessage,
	})
}

func setUpdatingCondition(_ context.Context, machine *clusterv1.Machine, updatingReason, updatingMessage string) {
	if updatingReason == "" {
		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineUpdatingCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineNotUpdatingReason,
		})
		return
	}

	conditions.Set(machine, metav1.Condition{
		Type:    clusterv1.MachineUpdatingCondition,
		Status:  metav1.ConditionTrue,
		Reason:  updatingReason,
		Message: updatingMessage,
	})
}

func setReadyCondition(ctx context.Context, machine *clusterv1.Machine) {
	log := ctrl.LoggerFrom(ctx)

	forConditionTypes := conditions.ForConditionTypes{
		clusterv1.MachineDeletingCondition,
		clusterv1.MachineBootstrapConfigReadyCondition,
		clusterv1.MachineInfrastructureReadyCondition,
		clusterv1.MachineNodeHealthyCondition,
		clusterv1.MachineHealthCheckSucceededCondition,
	}
	negativePolarityConditionTypes := []string{clusterv1.MachineDeletingCondition}
	for _, g := range machine.Spec.ReadinessGates {
		forConditionTypes = append(forConditionTypes, g.ConditionType)
		if g.Polarity == clusterv1.NegativePolarityCondition {
			negativePolarityConditionTypes = append(negativePolarityConditionTypes, g.ConditionType)
		}
	}

	summaryOpts := []conditions.SummaryOption{
		forConditionTypes,
		// Tolerate HealthCheckSucceeded to not exist.
		conditions.IgnoreTypesIfMissing{
			clusterv1.MachineHealthCheckSucceededCondition,
		},
		// Using a custom merge strategy to override reasons applied during merge and to ignore some
		// info message so the ready condition aggregation in other resources is less noisy.
		conditions.CustomMergeStrategy{
			MergeStrategy: machineConditionCustomMergeStrategy{
				machine: machine,
				// Instruct merge to consider Deleting condition with negative polarity,
				negativePolarityConditionTypes: negativePolarityConditionTypes,
			},
		},
		// Instruct summary to consider Deleting condition with negative polarity.
		conditions.NegativePolarityConditionTypes{
			clusterv1.MachineDeletingCondition,
		},
	}

	// Add overrides for conditions we want to surface in the Ready condition with slightly different messages,
	// mostly to improve when we will aggregate the Ready condition from many machines on MS, MD etc.
	var overrideConditions conditions.OverrideConditions
	if !machine.DeletionTimestamp.IsZero() {
		overrideConditions = append(overrideConditions, calculateDeletingConditionForSummary(machine))
	}

	if len(overrideConditions) > 0 {
		summaryOpts = append(summaryOpts, overrideConditions)
	}

	readyCondition, err := conditions.NewSummaryCondition(machine, clusterv1.MachineReadyCondition, summaryOpts...)

	if err != nil {
		// Note, this could only happen if we hit edge cases in computing the summary, which should not happen due to the fact
		// that we are passing a non empty list of ForConditionTypes.
		log.Error(err, "Failed to set Ready condition")
		readyCondition = &metav1.Condition{
			Type:    clusterv1.MachineReadyCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineReadyInternalErrorReason,
			Message: "Please check controller logs for errors",
		}
	}

	conditions.Set(machine, *readyCondition)
}

// calculateDeletingConditionForSummary calculates a Deleting condition for the calculation of the Ready condition
// (which is done via a summary). This is necessary to avoid including the verbose details of the Deleting condition
// message in the summary.
// This is also important to ensure we have a limited amount of unique messages across Machines thus allowing to
// nicely aggregate Ready conditions from many Machines into the MachinesReady condition of e.g. the MachineSet.
// For the same reason we are only surfacing messages with "more than 15m" instead of using the exact durations.
// 15 minutes is a duration after which we assume it makes sense to emphasize that Node drains and waiting for volume
// detach are still in progress.
func calculateDeletingConditionForSummary(machine *clusterv1.Machine) conditions.ConditionWithOwnerInfo {
	deletingCondition := conditions.Get(machine, clusterv1.MachineDeletingCondition)

	msg := "Machine deletion in progress"
	if deletingCondition != nil {
		msg = fmt.Sprintf("Machine deletion in progress, stage: %s", deletingCondition.Reason)
		if !machine.GetDeletionTimestamp().IsZero() && time.Since(machine.GetDeletionTimestamp().Time) > time.Minute*15 {
			msg = fmt.Sprintf("Machine deletion in progress since more than 15m, stage: %s", deletingCondition.Reason)
			if deletingCondition.Reason == clusterv1.MachineDeletingDrainingNodeReason &&
				!machine.Status.Deletion.NodeDrainStartTime.Time.IsZero() &&
				time.Since(machine.Status.Deletion.NodeDrainStartTime.Time) > 5*time.Minute {
				delayReasons := []string{}
				if strings.Contains(deletingCondition.Message, "cannot evict pod as it would violate the pod's disruption budget.") {
					delayReasons = append(delayReasons, "PodDisruptionBudgets")
				}
				if strings.Contains(deletingCondition.Message, "deletionTimestamp set, but still not removed from the Node") {
					delayReasons = append(delayReasons, "Pods not terminating")
				}
				if strings.Contains(deletingCondition.Message, "failed to evict Pod") {
					delayReasons = append(delayReasons, "Pod eviction errors")
				}
				if strings.Contains(deletingCondition.Message, "waiting for completion") {
					delayReasons = append(delayReasons, "Pods not completed yet")
				}
				if len(delayReasons) > 0 {
					msg += fmt.Sprintf(", delay likely due to %s", strings.Join(delayReasons, ", "))
				}
			}
		}
	}

	return conditions.ConditionWithOwnerInfo{
		OwnerResource: conditions.ConditionOwnerInfo{
			Kind: "Machine",
			Name: machine.Name,
		},
		Condition: metav1.Condition{
			Type:    clusterv1.MachineDeletingCondition,
			Status:  metav1.ConditionTrue,
			Reason:  clusterv1.MachineDeletingReason,
			Message: msg,
		},
	}
}

func setAvailableCondition(ctx context.Context, machine *clusterv1.Machine) {
	log := ctrl.LoggerFrom(ctx)
	readyCondition := conditions.Get(machine, clusterv1.MachineReadyCondition)

	if readyCondition == nil {
		// NOTE: this should never happen given that setReadyCondition is called before this method and
		// it always add a ready condition.
		log.Error(errors.New("Ready condition must be set before setting the available condition"), "Failed to set Available condition")
		conditions.Set(machine, metav1.Condition{
			Type:    clusterv1.MachineAvailableCondition,
			Status:  metav1.ConditionUnknown,
			Reason:  clusterv1.MachineAvailableInternalErrorReason,
			Message: "Please check controller logs for errors",
		})
		return
	}

	if readyCondition.Status != metav1.ConditionTrue {
		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineAvailableCondition,
			Status: metav1.ConditionFalse,
			Reason: clusterv1.MachineNotReadyReason,
		})
		return
	}

	if time.Since(readyCondition.LastTransitionTime.Time) >= time.Duration(ptr.Deref(machine.Spec.MinReadySeconds, 0))*time.Second {
		conditions.Set(machine, metav1.Condition{
			Type:   clusterv1.MachineAvailableCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.MachineAvailableReason,
		})
		return
	}

	conditions.Set(machine, metav1.Condition{
		Type:   clusterv1.MachineAvailableCondition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.MachineWaitingForMinReadySecondsReason,
	})
}

func setMachinePhaseAndLastUpdated(_ context.Context, m *clusterv1.Machine) {
	originalPhase := m.Status.Phase

	// Set the phase to "pending" if nil.
	if m.Status.Phase == "" {
		m.Status.SetTypedPhase(clusterv1.MachinePhasePending)
	}

	// Set the phase to "provisioning" if bootstrap is ready and the infrastructure isn't.
	if ptr.Deref(m.Status.Initialization.BootstrapDataSecretCreated, false) && !ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false) {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioning)
	}

	// Set the phase to "provisioned" if there is a provider ID.
	if m.Spec.ProviderID != "" {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseProvisioned)
	}

	// Set the phase to "running" if there is a NodeRef field and infrastructure is ready.
	if m.Status.NodeRef.IsDefined() && ptr.Deref(m.Status.Initialization.InfrastructureProvisioned, false) {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseRunning)
	}

	// Set the phase to "deleting" if the deletion timestamp is set.
	if !m.DeletionTimestamp.IsZero() {
		m.Status.SetTypedPhase(clusterv1.MachinePhaseDeleting)
	}

	// If the phase has changed, update the LastUpdated timestamp
	if m.Status.Phase != originalPhase {
		m.Status.LastUpdated = metav1.Now()
	}
}
