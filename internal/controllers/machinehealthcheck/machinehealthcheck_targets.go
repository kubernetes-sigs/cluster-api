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

package machinehealthcheck

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
)

const (
	// Event types.

	// EventMachineMarkedUnhealthy is emitted when machine was successfully marked as unhealthy.
	EventMachineMarkedUnhealthy string = "MachineMarkedUnhealthy"
	// EventDetectedUnhealthy is emitted in case a machine or its associated
	// node was detected unhealthy.
	EventDetectedUnhealthy string = "DetectedUnhealthy"
)

var (
	// We allow users to disable the nodeStartupTimeout by setting the duration to 0.
	disabledNodeStartupTimeout = metav1.Duration{Duration: time.Duration(0)}
)

// healthCheckTarget contains the information required to perform a health check
// on the node to determine if any remediation is required.
type healthCheckTarget struct {
	Cluster     *clusterv1.Cluster
	Machine     *clusterv1.Machine
	Node        *corev1.Node
	MHC         *clusterv1.MachineHealthCheck
	patchHelper *patch.Helper
	nodeMissing bool
}

// Get the node name if the target has a node.
func (t *healthCheckTarget) nodeName() string {
	if t.Node != nil {
		return t.Node.GetName()
	}
	return ""
}

// needsRemediation determines whether a given target needs remediation.
// The machine will need remediation if any of the following are true:
// - The Machine has the remediate machine annotation
// - Any condition on the machine matches the configured checks and exceeds the timeout
// - The Machine did not get a node before `timeoutForMachineToHaveNode` elapses
// - The Node has been deleted but the Machine still references it
// - Any condition on the node matches the configured checks and exceeds the timeout
//
// Machine conditions are always evaluated first and consistently across all scenarios
// (node missing, node startup timeout, node exists) to ensure comprehensive health checking.
// When multiple issues are detected, error messages are merged to provide complete visibility.
//
// Returns true if remediation is needed, and a duration indicating when to recheck if remediation
// is not immediately required. The target should be requeued after this duration.
func (t *healthCheckTarget) needsRemediation(logger logr.Logger, timeoutForMachineToHaveNode metav1.Duration) (bool, time.Duration) {
	var nextCheckTimes []time.Duration
	now := time.Now()

	if annotations.HasRemediateMachine(t.Machine) {
		v1beta1conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, clusterv1.HasRemediateMachineAnnotationV1Beta1Reason, clusterv1.ConditionSeverityWarning, "Marked for remediation via remediate-machine annotation")
		logger.V(3).Info("Target is marked for remediation via remediate-machine annotation")

		conditions.Set(t.Machine, metav1.Condition{
			Type:    clusterv1.MachineHealthCheckSucceededCondition,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1.MachineHealthCheckHasRemediateAnnotationReason,
			Message: "Health check failed: marked for remediation via cluster.x-k8s.io/remediate-machine annotation",
		})
		return true, time.Duration(0)
	}

	// Don't penalize any Machine/Node if the control plane has not been initialized
	// Exception of this rule are control plane machine itself, so the first control plane machine can be remediated.
	if !conditions.IsTrue(t.Cluster, clusterv1.ClusterControlPlaneInitializedCondition) && !util.IsControlPlaneMachine(t.Machine) {
		logger.V(5).Info("Not evaluating target health because the control plane has not yet been initialized")
		// Return a nextCheck time of 0 because we'll get requeued when the Cluster is updated.
		return false, 0
	}

	// Don't penalize any Machine/Node if the cluster infrastructure is not ready.
	if !conditions.IsTrue(t.Cluster, clusterv1.ClusterInfrastructureReadyCondition) {
		logger.V(5).Info("Not evaluating target health because the cluster infrastructure is not ready")
		// Return a nextCheck time of 0 because we'll get requeued when the Cluster is updated.
		return false, 0
	}

	// Collect all unhealthy conditions (both node and machine) to provide comprehensive status
	// Always evaluate machine conditions regardless of node state for consistent behavior
	var (
		unhealthyNodeMessages    []string
		unhealthyMachineMessages []string
	)

	// Always check machine conditions first, regardless of node state
	for _, c := range t.MHC.Spec.Checks.UnhealthyMachineConditions {
		machineCondition := getMachineCondition(t.Machine, c.Type)

		// Skip when current machine condition is different from the one reported
		// in the MachineHealthCheck.
		if machineCondition == nil || machineCondition.Status != c.Status {
			continue
		}

		// If the machine condition has been in the unhealthy state for longer than the
		// timeout, mark as unhealthy and collect the message.
		timeoutSecondsDuration := time.Duration(ptr.Deref(c.TimeoutSeconds, 0)) * time.Second

		if machineCondition.LastTransitionTime.Add(timeoutSecondsDuration).Before(now) {
			unhealthyMachineMessages = append(unhealthyMachineMessages, fmt.Sprintf("Condition %s on Machine is reporting status %s for more than %s", c.Type, c.Status, timeoutSecondsDuration.String()))
			logger.V(3).Info("Target is unhealthy: machine condition is in state longer than allowed timeout", "condition", c.Type, "state", c.Status, "timeout", timeoutSecondsDuration.String())
			continue
		}

		durationUnhealthy := now.Sub(machineCondition.LastTransitionTime.Time)
		nextCheck := timeoutSecondsDuration - durationUnhealthy + time.Second
		if nextCheck > 0 {
			nextCheckTimes = append(nextCheckTimes, nextCheck)
		}
	}

	// Machine has Status.NodeRef set, although we couldn't find the node in the workload cluster.
	if t.nodeMissing {
		logger.V(3).Info("Target is unhealthy: node is missing")

		// Always merge node missing message with any unhealthy machine conditions
		nodeMissingMessage := fmt.Sprintf("Node %s has been deleted", t.Machine.Status.NodeRef.Name)
		allMessages := append([]string{nodeMissingMessage}, unhealthyMachineMessages...)

		reason := clusterv1.MachineHealthCheckNodeDeletedReason
		v1beta1Reason := clusterv1.NodeNotFoundV1Beta1Reason
		if len(unhealthyMachineMessages) > 0 {
			reason = clusterv1.MachineHealthCheckUnhealthyMachineReason
			v1beta1Reason = clusterv1.UnhealthyMachineConditionV1Beta1Reason
		}

		// For v1beta2 we use a single-line message prefixed with "Health check failed: "
		conditionMessage := fmt.Sprintf("Health check failed: %s", strings.Join(allMessages, "; "))
		conditions.Set(t.Machine, metav1.Condition{
			Type:    clusterv1.MachineHealthCheckSucceededCondition,
			Status:  metav1.ConditionFalse,
			Reason:  reason,
			Message: conditionMessage,
		})

		// For v1beta1 keep the existing format
		if len(unhealthyMachineMessages) > 0 {
			v1beta1conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, v1beta1Reason, clusterv1.ConditionSeverityWarning, "%s", strings.Join(allMessages, "; "))
		} else {
			v1beta1conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, v1beta1Reason, clusterv1.ConditionSeverityWarning, "")
		}
		return true, time.Duration(0)
	}

	// the node has not been set yet
	if t.Node == nil {
		// Check if we already have unhealthy machine conditions that should trigger remediation
		if len(unhealthyMachineMessages) > 0 {
			reason := clusterv1.MachineHealthCheckUnhealthyMachineReason
			v1beta1Reason := clusterv1.UnhealthyMachineConditionV1Beta1Reason

			// For v1beta2 we use a single-line message prefixed with "Health check failed: "
			conditionMessage := fmt.Sprintf("Health check failed: %s", strings.Join(unhealthyMachineMessages, "; "))
			conditions.Set(t.Machine, metav1.Condition{
				Type:    clusterv1.MachineHealthCheckSucceededCondition,
				Status:  metav1.ConditionFalse,
				Reason:  reason,
				Message: conditionMessage,
			})

			// For v1beta1 keep the existing semicolon-separated format (no prefix).
			v1beta1conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, v1beta1Reason, clusterv1.ConditionSeverityWarning, "%s", strings.Join(unhealthyMachineMessages, "; "))

			return true, time.Duration(0)
		}

		if timeoutForMachineToHaveNode == disabledNodeStartupTimeout {
			// Startup timeout is disabled so no need to go any further.
			// No node yet to check conditions, can return early here.
			return false, minDuration(nextCheckTimes)
		}

		controlPlaneInitialized := conditions.GetLastTransitionTime(t.Cluster, clusterv1.ClusterControlPlaneInitializedCondition)
		clusterInfraReady := conditions.GetLastTransitionTime(t.Cluster, clusterv1.ClusterInfrastructureReadyCondition)
		machineInfraReady := conditions.GetLastTransitionTime(t.Machine, clusterv1.MachineInfrastructureReadyCondition)
		machineCreationTime := t.Machine.CreationTimestamp.Time

		// Use the latest of the following timestamps.
		comparisonTime := machineCreationTime
		logger.V(5).Info("Determining comparison time",
			"machineCreationTime", machineCreationTime,
			"clusterInfraReadyTime", clusterInfraReady,
			"controlPlaneInitializedTime", controlPlaneInitialized,
			"machineInfraReadyTime", machineInfraReady,
		)
		if conditions.IsTrue(t.Cluster, clusterv1.ClusterControlPlaneInitializedCondition) && controlPlaneInitialized != nil && controlPlaneInitialized.After(comparisonTime) {
			comparisonTime = controlPlaneInitialized.Time
		}
		if conditions.IsTrue(t.Cluster, clusterv1.ClusterInfrastructureReadyCondition) && clusterInfraReady != nil && clusterInfraReady.After(comparisonTime) {
			comparisonTime = clusterInfraReady.Time
		}
		if conditions.IsTrue(t.Machine, clusterv1.MachineInfrastructureReadyCondition) && machineInfraReady != nil && machineInfraReady.After(comparisonTime) {
			comparisonTime = machineInfraReady.Time
		}
		logger.V(5).Info("Using comparison time", "time", comparisonTime)

		timeoutDuration := timeoutForMachineToHaveNode.Duration
		if comparisonTime.Add(timeoutForMachineToHaveNode.Duration).Before(now) {
			// Node startup timeout - merge with any unhealthy machine conditions
			nodeTimeoutMessage := fmt.Sprintf("Node failed to report startup in %s", timeoutDuration)
			allMessages := append([]string{nodeTimeoutMessage}, unhealthyMachineMessages...)

			reason := clusterv1.MachineHealthCheckNodeStartupTimeoutReason
			v1beta1Reason := clusterv1.NodeStartupTimeoutV1Beta1Reason
			if len(unhealthyMachineMessages) > 0 {
				reason = clusterv1.MachineHealthCheckUnhealthyMachineReason
				v1beta1Reason = clusterv1.UnhealthyMachineConditionV1Beta1Reason
			}

			logger.V(3).Info("Target is unhealthy: machine has no node", "duration", timeoutDuration)

			// For v1beta2 we use a single-line message prefixed with "Health check failed: "
			conditionMessage := fmt.Sprintf("Health check failed: %s", strings.Join(allMessages, "; "))
			conditions.Set(t.Machine, metav1.Condition{
				Type:    clusterv1.MachineHealthCheckSucceededCondition,
				Status:  metav1.ConditionFalse,
				Reason:  reason,
				Message: conditionMessage,
			})

			// For v1beta1 keep the existing format
			if len(unhealthyMachineMessages) > 0 {
				v1beta1conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, v1beta1Reason, clusterv1.ConditionSeverityWarning, "%s", strings.Join(allMessages, "; "))
			} else {
				v1beta1conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, v1beta1Reason, clusterv1.ConditionSeverityWarning, "Node failed to report startup in %s", timeoutDuration)
			}
			return true, time.Duration(0)
		}

		durationUnhealthy := now.Sub(comparisonTime)
		nextCheck := timeoutDuration - durationUnhealthy + time.Second
		if nextCheck > 0 {
			nextCheckTimes = append(nextCheckTimes, nextCheck)
		}

		return false, minDuration(nextCheckTimes)
	}

	// check node conditions (only when node is available)
	for _, c := range t.MHC.Spec.Checks.UnhealthyNodeConditions {
		nodeCondition := getNodeCondition(t.Node, c.Type)

		// Skip when current node condition is different from the one reported
		// in the MachineHealthCheck.
		if nodeCondition == nil || nodeCondition.Status != c.Status {
			continue
		}

		// If the node condition has been in the unhealthy state for longer than the
		// timeout, mark as unhealthy and collect the message.
		timeoutSecondsDuration := time.Duration(ptr.Deref(c.TimeoutSeconds, 0)) * time.Second

		if nodeCondition.LastTransitionTime.Add(timeoutSecondsDuration).Before(now) {
			unhealthyNodeMessages = append(unhealthyNodeMessages, fmt.Sprintf("Condition %s on Node is reporting status %s for more than %s", c.Type, c.Status, timeoutSecondsDuration.String()))
			logger.V(3).Info("Target is unhealthy: node condition is in state longer than allowed timeout", "condition", c.Type, "state", c.Status, "timeout", timeoutSecondsDuration.String())
			continue
		}

		durationUnhealthy := now.Sub(nodeCondition.LastTransitionTime.Time)
		nextCheck := timeoutSecondsDuration - durationUnhealthy + time.Second
		if nextCheck > 0 {
			nextCheckTimes = append(nextCheckTimes, nextCheck)
		}
	}

	if len(unhealthyNodeMessages) > 0 || len(unhealthyMachineMessages) > 0 {
		reason := clusterv1.MachineHealthCheckUnhealthyNodeReason
		v1beta1Reason := clusterv1.UnhealthyNodeConditionV1Beta1Reason
		if len(unhealthyMachineMessages) > 0 {
			reason = clusterv1.MachineHealthCheckUnhealthyMachineReason
			v1beta1Reason = clusterv1.UnhealthyMachineConditionV1Beta1Reason
		}

		// Combine all messages into a single comprehensive message
		allMessages := append(unhealthyNodeMessages, unhealthyMachineMessages...)
		// For v1beta2 we use a single-line message prefixed with "Health check failed: "
		// for compatibility with existing tests and other condition messages.
		conditionMessage := fmt.Sprintf("Health check failed: %s", strings.Join(allMessages, "; "))
		conditions.Set(t.Machine, metav1.Condition{
			Type:    clusterv1.MachineHealthCheckSucceededCondition,
			Status:  metav1.ConditionFalse,
			Reason:  reason,
			Message: conditionMessage,
		})

		// For v1beta1 keep the existing semicolon-separated format (no prefix).
		v1beta1conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition, v1beta1Reason, clusterv1.ConditionSeverityWarning, "%s", strings.Join(allMessages, "; "))

		return true, time.Duration(0)
	}

	return false, minDuration(nextCheckTimes)
}

// getTargetsFromMHC uses the MachineHealthCheck's selector to fetch machines
// and their nodes targeted by the health check, ready for health checking.
func (r *Reconciler) getTargetsFromMHC(ctx context.Context, logger logr.Logger, clusterClient client.Reader, cluster *clusterv1.Cluster, mhc *clusterv1.MachineHealthCheck) ([]healthCheckTarget, error) {
	machines, err := r.getMachinesFromMHC(ctx, mhc)
	if err != nil {
		return nil, errors.Wrap(err, "error getting machines from MachineHealthCheck")
	}
	if len(machines) == 0 {
		return nil, nil
	}

	targets := []healthCheckTarget{}
	for k := range machines {
		logger := logger.WithValues("Machine", klog.KObj(&machines[k]))
		skip, reason := shouldSkipRemediation(&machines[k])
		if skip {
			logger.Info("Skipping remediation", "reason", reason)
			continue
		}

		patchHelper, err := patch.NewHelper(&machines[k], r.Client)
		if err != nil {
			return nil, err
		}
		target := healthCheckTarget{
			Cluster:     cluster,
			MHC:         mhc,
			Machine:     &machines[k],
			patchHelper: patchHelper,
		}
		if clusterClient != nil {
			node, err := r.getNodeFromMachine(ctx, clusterClient, target.Machine)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					return nil, errors.Wrap(err, "error getting node")
				}

				// A node has been seen for this machine, but it no longer exists
				target.nodeMissing = true
			}
			target.Node = node
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// getMachinesFromMHC fetches Machines matched by the MachineHealthCheck's
// label selector.
func (r *Reconciler) getMachinesFromMHC(ctx context.Context, mhc *clusterv1.MachineHealthCheck) ([]clusterv1.Machine, error) {
	selector, err := metav1.LabelSelectorAsSelector(metav1.CloneSelectorAndAddLabel(
		&mhc.Spec.Selector, clusterv1.ClusterNameLabel, mhc.Spec.ClusterName,
	))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build selector")
	}

	var machineList clusterv1.MachineList
	if err := r.Client.List(
		ctx,
		&machineList,
		client.MatchingLabelsSelector{Selector: selector},
		client.InNamespace(mhc.GetNamespace()),
	); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}
	return machineList.Items, nil
}

// getNodeFromMachine fetches the node from a local or remote cluster for a
// given machine.
func (r *Reconciler) getNodeFromMachine(ctx context.Context, clusterClient client.Reader, machine *clusterv1.Machine) (*corev1.Node, error) {
	if !machine.Status.NodeRef.IsDefined() {
		return nil, nil
	}

	node := &corev1.Node{}
	nodeKey := types.NamespacedName{
		Name: machine.Status.NodeRef.Name,
	}

	// if it cannot find a node, send a nil node back...
	if err := clusterClient.Get(ctx, nodeKey, node); err != nil {
		return nil, err
	}
	return node, nil
}

// healthCheckTargets health checks a slice of targets
// and gives a data to measure the average health.
func (r *Reconciler) healthCheckTargets(targets []healthCheckTarget, logger logr.Logger, timeoutForMachineToHaveNode metav1.Duration) ([]healthCheckTarget, []healthCheckTarget, []time.Duration) {
	var nextCheckTimes []time.Duration
	var unhealthy []healthCheckTarget
	var healthy []healthCheckTarget

	for _, t := range targets {
		logger := logger.WithValues("Machine", klog.KObj(t.Machine), "Node", klog.KObj(t.Node))
		logger.V(3).Info("Health checking target")
		needsRemediation, nextCheck := t.needsRemediation(logger, timeoutForMachineToHaveNode)

		if needsRemediation {
			unhealthy = append(unhealthy, t)
			continue
		}

		if nextCheck > 0 {
			logger.V(3).Info("Target is likely to go unhealthy", "timeUntilUnhealthy", nextCheck.Truncate(time.Second).String())
			r.recorder.Eventf(
				t.Machine,
				corev1.EventTypeNormal,
				EventDetectedUnhealthy,
				"Machine %s (Node %s) is failing machine health check rules and it is likely to go unhealthy",
				klog.KObj(t.Machine),
				t.nodeName(),
			)
			nextCheckTimes = append(nextCheckTimes, nextCheck)
			continue
		}

		if t.Machine.DeletionTimestamp.IsZero() && t.Node != nil {
			v1beta1conditions.MarkTrue(t.Machine, clusterv1.MachineHealthCheckSucceededV1Beta1Condition)

			conditions.Set(t.Machine, metav1.Condition{
				Type:   clusterv1.MachineHealthCheckSucceededCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.MachineHealthCheckSucceededReason,
			})
			healthy = append(healthy, t)
		}
	}
	return healthy, unhealthy, nextCheckTimes
}

// getNodeCondition returns node condition by type.
func getNodeCondition(node *corev1.Node, conditionType corev1.NodeConditionType) *corev1.NodeCondition {
	for _, cond := range node.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

// getMachineCondition returns machine condition by type.
func getMachineCondition(node *clusterv1.Machine, conditionType string) *metav1.Condition {
	for _, cond := range node.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

func minDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return time.Duration(0)
	}

	minDuration := durations[0]
	// Ignore first element as that is already minDuration
	for _, nc := range durations[1:] {
		if nc < minDuration {
			minDuration = nc
		}
	}
	return minDuration
}

// shouldSkipRemediation checks if the machine should be skipped for remediation.
// Returns true if it should be skipped along with the reason for skipping.
func shouldSkipRemediation(m *clusterv1.Machine) (bool, string) {
	if annotations.HasPaused(m) {
		return true, fmt.Sprintf("machine has %q annotation", clusterv1.PausedAnnotation)
	}

	if annotations.HasSkipRemediation(m) {
		return true, fmt.Sprintf("machine has %q annotation", clusterv1.MachineSkipRemediationAnnotation)
	}

	return false, ""
}
