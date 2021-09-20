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
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Event types.

	// EventMachineMarkedUnhealthy is emitted when machine was successfully marked as unhealthy.
	EventMachineMarkedUnhealthy string = "MachineMarkedUnhealthy"
	// EventDetectedUnhealthy is emitted in case a node associated with a
	// machine was detected unhealthy.
	EventDetectedUnhealthy string = "DetectedUnhealthy"
)

var (
	// We allow users to disable the nodeStartupTimeout by setting the duration to 0.
	disabledNodeStartupTimeout = clusterv1.ZeroDuration
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

func (t *healthCheckTarget) string() string {
	return fmt.Sprintf("%s/%s/%s/%s",
		t.MHC.GetNamespace(),
		t.MHC.GetName(),
		t.Machine.GetName(),
		t.nodeName(),
	)
}

// Get the node name if the target has a node.
func (t *healthCheckTarget) nodeName() string {
	if t.Node != nil {
		return t.Node.GetName()
	}
	return ""
}

// Determine whether or not a given target needs remediation.
// The node will need remediation if any of the following are true:
// - The Machine has failed for some reason
// - The Machine did not get a node before `timeoutForMachineToHaveNode` elapses
// - The Node has gone away
// - Any condition on the node is matched for the given timeout
// If the target doesn't currently need rememdiation, provide a duration after
// which the target should next be checked.
// The target should be requeued after this duration.
func (t *healthCheckTarget) needsRemediation(logger logr.Logger, timeoutForMachineToHaveNode metav1.Duration) (bool, time.Duration) {
	var nextCheckTimes []time.Duration
	now := time.Now()

	if t.Machine.Status.FailureReason != nil {
		conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "FailureReason: %v", t.Machine.Status.FailureReason)
		logger.V(3).Info("Target is unhealthy", "failureReason", t.Machine.Status.FailureReason)
		return true, time.Duration(0)
	}

	if t.Machine.Status.FailureMessage != nil {
		conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.MachineHasFailureReason, clusterv1.ConditionSeverityWarning, "FailureMessage: %v", t.Machine.Status.FailureMessage)
		logger.V(3).Info("Target is unhealthy", "failureMessage", t.Machine.Status.FailureMessage)
		return true, time.Duration(0)
	}

	// the node does not exist
	if t.nodeMissing {
		logger.V(3).Info("Target is unhealthy: node is missing")
		conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.NodeNotFoundReason, clusterv1.ConditionSeverityWarning, "")
		return true, time.Duration(0)
	}

	// Don't penalize any Machine/Node if the control plane has not been initialized.
	if !conditions.IsTrue(t.Cluster, clusterv1.ControlPlaneInitializedCondition) {
		logger.V(3).Info("Not evaluating target health because the control plane has not yet been initialized")
		// Return a nextCheck time of 0 because we'll get requeued when the Cluster is updated.
		return false, 0
	}

	// Don't penalize any Machine/Node if the cluster infrastructure is not ready.
	if !conditions.IsTrue(t.Cluster, clusterv1.InfrastructureReadyCondition) {
		logger.V(3).Info("Not evaluating target health because the cluster infrastructure is not ready")
		// Return a nextCheck time of 0 because we'll get requeued when the Cluster is updated.
		return false, 0
	}

	// the node has not been set yet
	if t.Node == nil {
		if timeoutForMachineToHaveNode == disabledNodeStartupTimeout {
			// Startup timeout is disabled so no need to go any further.
			// No node yet to check conditions, can return early here.
			return false, 0
		}

		controlPlaneInitializedTime := conditions.GetLastTransitionTime(t.Cluster, clusterv1.ControlPlaneInitializedCondition).Time
		clusterInfraReadyTime := conditions.GetLastTransitionTime(t.Cluster, clusterv1.InfrastructureReadyCondition).Time
		machineCreationTime := t.Machine.CreationTimestamp.Time

		// Use the latest of the 3 times
		comparisonTime := machineCreationTime
		logger.V(3).Info("Determining comparison time", "machineCreationTime", machineCreationTime, "clusterInfraReadyTime", clusterInfraReadyTime, "controlPlaneInitializedTime", controlPlaneInitializedTime)
		if controlPlaneInitializedTime.After(comparisonTime) {
			comparisonTime = controlPlaneInitializedTime
		}
		if clusterInfraReadyTime.After(comparisonTime) {
			comparisonTime = clusterInfraReadyTime
		}
		logger.V(3).Info("Using comparison time", "time", comparisonTime)

		if comparisonTime.Add(timeoutForMachineToHaveNode.Duration).Before(now) {
			conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.NodeStartupTimeoutReason, clusterv1.ConditionSeverityWarning, "Node failed to report startup in %s", timeoutForMachineToHaveNode.String())
			logger.V(3).Info("Target is unhealthy: machine has no node", "duration", timeoutForMachineToHaveNode.String())
			return true, time.Duration(0)
		}

		durationUnhealthy := now.Sub(comparisonTime)
		nextCheck := timeoutForMachineToHaveNode.Duration - durationUnhealthy + time.Second

		return false, nextCheck
	}

	// check conditions
	for _, c := range t.MHC.Spec.UnhealthyConditions {
		nodeCondition := getNodeCondition(t.Node, c.Type)

		// Skip when current node condition is different from the one reported
		// in the MachineHealthCheck.
		if nodeCondition == nil || nodeCondition.Status != c.Status {
			continue
		}

		// If the condition has been in the unhealthy state for longer than the
		// timeout, return true with no requeue time.
		if nodeCondition.LastTransitionTime.Add(c.Timeout.Duration).Before(now) {
			conditions.MarkFalse(t.Machine, clusterv1.MachineHealthCheckSuccededCondition, clusterv1.UnhealthyNodeConditionReason, clusterv1.ConditionSeverityWarning, "Condition %s on node is reporting status %s for more than %s", c.Type, c.Status, c.Timeout.Duration.String())
			logger.V(3).Info("Target is unhealthy: condition is in state longer than allowed timeout", "condition", c.Type, "state", c.Status, "timeout", c.Timeout.Duration.String())
			return true, time.Duration(0)
		}

		durationUnhealthy := now.Sub(nodeCondition.LastTransitionTime.Time)
		nextCheck := c.Timeout.Duration - durationUnhealthy + time.Second
		if nextCheck > 0 {
			nextCheckTimes = append(nextCheckTimes, nextCheck)
		}
	}
	return false, minDuration(nextCheckTimes)
}

// getTargetsFromMHC uses the MachineHealthCheck's selector to fetch machines
// and their nodes targeted by the health check, ready for health checking.
func (r *MachineHealthCheckReconciler) getTargetsFromMHC(ctx context.Context, logger logr.Logger, clusterClient client.Reader, cluster *clusterv1.Cluster, mhc *clusterv1.MachineHealthCheck) ([]healthCheckTarget, error) {
	machines, err := r.getMachinesFromMHC(ctx, mhc)
	if err != nil {
		return nil, errors.Wrap(err, "error getting machines from MachineHealthCheck")
	}
	if len(machines) == 0 {
		return nil, nil
	}

	targets := []healthCheckTarget{}
	for k := range machines {
		skip, reason := shouldSkipRemediation(&machines[k])
		if skip {
			logger.Info("skipping remediation", "machine", machines[k].Name, "reason", reason)
			continue
		}

		patchHelper, err := patch.NewHelper(&machines[k], r.Client)
		if err != nil {
			return nil, errors.Wrap(err, "unable to initialize patch helper")
		}
		target := healthCheckTarget{
			Cluster:     cluster,
			MHC:         mhc,
			Machine:     &machines[k],
			patchHelper: patchHelper,
		}
		node, err := r.getNodeFromMachine(ctx, clusterClient, target.Machine)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, errors.Wrap(err, "error getting node")
			}

			// A node has been seen for this machine, but it no longer exists
			target.nodeMissing = true
		}
		target.Node = node
		targets = append(targets, target)
	}
	return targets, nil
}

// getMachinesFromMHC fetches Machines matched by the MachineHealthCheck's
// label selector.
func (r *MachineHealthCheckReconciler) getMachinesFromMHC(ctx context.Context, mhc *clusterv1.MachineHealthCheck) ([]clusterv1.Machine, error) {
	selector, err := metav1.LabelSelectorAsSelector(metav1.CloneSelectorAndAddLabel(
		&mhc.Spec.Selector, clusterv1.ClusterLabelName, mhc.Spec.ClusterName,
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
func (r *MachineHealthCheckReconciler) getNodeFromMachine(ctx context.Context, clusterClient client.Reader, machine *clusterv1.Machine) (*corev1.Node, error) {
	if machine.Status.NodeRef == nil {
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
func (r *MachineHealthCheckReconciler) healthCheckTargets(targets []healthCheckTarget, logger logr.Logger, timeoutForMachineToHaveNode metav1.Duration) ([]healthCheckTarget, []healthCheckTarget, []time.Duration) {
	var nextCheckTimes []time.Duration
	var unhealthy []healthCheckTarget
	var healthy []healthCheckTarget

	for _, t := range targets {
		logger = logger.WithValues("Target", t.string())
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
				"Machine %v has unhealthy node %v",
				t.string(),
				t.nodeName(),
			)
			nextCheckTimes = append(nextCheckTimes, nextCheck)
			continue
		}

		if t.Machine.DeletionTimestamp.IsZero() && t.Node != nil {
			conditions.MarkTrue(t.Machine, clusterv1.MachineHealthCheckSuccededCondition)
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
	if annotations.HasPausedAnnotation(m) {
		return true, fmt.Sprintf("machine has %q annotation", clusterv1.PausedAnnotation)
	}

	if annotations.HasSkipRemediationAnnotation(m) {
		return true, fmt.Sprintf("machine has %q annotation", clusterv1.MachineSkipRemediationAnnotation)
	}

	return false, ""
}
