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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nodeControlPlaneLabel = "node-role.kubernetes.io/master"

	// Event types

	EventMachineDeletionFailed string = "MachineDeletionFailed"
	// EventMachineDeleted is emitted when machine was successfully remediated
	// by deleting its Machine object
	EventMachineDeleted string = "MachineDeleted"
	// EventDetectedUnhealthy is emitted in case a node associated with a
	// machine was detected unhealthy
	EventDetectedUnhealthy string = "DetectedUnhealthy"
)

// healthCheckTarget contains the information required to perform a health check
// on the node to determine if any remediation is required.
type healthCheckTarget struct {
	Machine     *clusterv1.Machine
	Node        *corev1.Node
	MHC         *clusterv1.MachineHealthCheck
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

// Get the node name if the target has a node
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
func (t *healthCheckTarget) needsRemediation(logger logr.Logger, timeoutForMachineToHaveNode time.Duration) (bool, time.Duration) {
	var nextCheckTimes []time.Duration
	now := time.Now()

	// machine has failed
	if t.Machine.Status.FailureReason != nil {
		logger.V(3).Info("Target is unhealthy", "reason", t.Machine.Status.FailureReason)
		return true, time.Duration(0)
	}

	// the node does not exist
	if t.nodeMissing {
		return true, time.Duration(0)
	}

	// the node has not been set yet
	if t.Node == nil {
		// status not updated yet
		if t.Machine.Status.LastUpdated == nil {
			return false, timeoutForMachineToHaveNode
		}
		if t.Machine.Status.LastUpdated.Add(timeoutForMachineToHaveNode).Before(now) {
			logger.V(3).Info("Target is unhealthy: machine has no node", "duration", timeoutForMachineToHaveNode.String())
			return true, time.Duration(0)
		}
		durationUnhealthy := now.Sub(t.Machine.Status.LastUpdated.Time)
		nextCheck := timeoutForMachineToHaveNode - durationUnhealthy + time.Second
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
func (r *MachineHealthCheckReconciler) getTargetsFromMHC(clusterClient client.Client, cluster *clusterv1.Cluster, mhc *clusterv1.MachineHealthCheck) ([]healthCheckTarget, error) {
	machines, err := r.getMachinesFromMHC(mhc)
	if err != nil {
		return nil, errors.Wrap(err, "error getting machines from MachineHealthCheck")
	}
	if len(machines) == 0 {
		return nil, nil
	}

	targets := []healthCheckTarget{}
	for k := range machines {
		target := healthCheckTarget{
			MHC:     mhc,
			Machine: &machines[k],
		}
		node, err := r.getNodeFromMachine(clusterClient, target.Machine)
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

//getMachinesFromMHC fetches Machines matched by the MachineHealthCheck's
// label selector
func (r *MachineHealthCheckReconciler) getMachinesFromMHC(mhc *clusterv1.MachineHealthCheck) ([]clusterv1.Machine, error) {
	selector, err := metav1.LabelSelectorAsSelector(&mhc.Spec.Selector)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build selector")
	}

	options := client.ListOptions{
		LabelSelector: selector,
		Namespace:     mhc.GetNamespace(),
	}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.Background(), machineList, &options); err != nil {
		return nil, errors.Wrap(err, "failed to list machines")
	}
	return machineList.Items, nil
}

// getNodeFromMachine fetches the node from a local or remote cluster for a
// given machine.
func (r *MachineHealthCheckReconciler) getNodeFromMachine(clusterClient client.Client, machine *clusterv1.Machine) (*corev1.Node, error) {
	if machine.Status.NodeRef == nil {
		return nil, nil
	}

	node := &corev1.Node{}
	nodeKey := types.NamespacedName{
		Name: machine.Status.NodeRef.Name,
	}
	err := clusterClient.Get(context.TODO(), nodeKey, node)
	return node, err
}

// healthCheckTargets health checks a slice of targets
// and gives a data to measure the average health
func (r *MachineHealthCheckReconciler) healthCheckTargets(targets []healthCheckTarget, logger logr.Logger, timeoutForMachineToHaveNode time.Duration) (int, []healthCheckTarget, []time.Duration) {
	var nextCheckTimes []time.Duration
	var needRemediationTargets []healthCheckTarget
	var currentHealthy int

	for _, t := range targets {
		logger = logger.WithValues("Target", t.string())
		logger.V(3).Info("Health checking target")
		needsRemediation, nextCheck := t.needsRemediation(logger, timeoutForMachineToHaveNode)

		if needsRemediation {
			needRemediationTargets = append(needRemediationTargets, t)
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

		if t.Machine.DeletionTimestamp.IsZero() {
			currentHealthy++
		}
	}
	return currentHealthy, needRemediationTargets, nextCheckTimes
}

// getNodeCondition returns node condition by type
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

// remediate deletes the Machine if it is owned by a controller
func (t *healthCheckTarget) remediate(ctx context.Context, logger logr.Logger, c client.Client, r record.EventRecorder) error {
	logger = logger.WithValues("target", t.string())
	logger.Info("Starting remediation for target")

	// If the machine is not owned by a controller, it should be skipped
	hasControllerOwner, err := t.hasControllerOwner()
	if err != nil {
		return fmt.Errorf("%s: unable to determine Machine owners: %v", t.string(), err)
	}
	if !hasControllerOwner {
		logger.Info("Target has no controller owner, skipping remediation")
		return nil
	}

	logger.Info("Deleting target machine")
	if err := c.Delete(ctx, t.Machine); err != nil {
		r.Eventf(
			t.Machine,
			corev1.EventTypeWarning,
			EventMachineDeletionFailed,
			"Machine %v remediation failed: unable to delete Machine object: %v",
			t.string(),
			err,
		)
		return fmt.Errorf("%s: failed to delete machine: %v", t.string(), err)
	}
	r.Eventf(
		t.Machine,
		corev1.EventTypeNormal,
		EventMachineDeleted,
		"Machine %v has been remediated by requesting to delete Machine object",
		t.string(),
	)

	return nil
}

// hasControllerOwner checks whether the target's Machine is owned by a controller
func (t *healthCheckTarget) hasControllerOwner() (bool, error) {
	ownerRefs := t.Machine.ObjectMeta.GetOwnerReferences()
	for _, or := range ownerRefs {
		gv, err := schema.ParseGroupVersion(or.APIVersion)
		if err != nil {
			return false, err
		}

		if gv.Group == clusterv1.GroupVersion.Group || gv.Group == controlplanev1.GroupVersion.Group {
			if or.Controller != nil && *or.Controller {
				return true, nil
			}
		}
	}
	return false, nil
}
