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

package internal

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdutil "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateEtcdConditions is responsible for updating machine conditions reflecting the status of all the etcd members.
// This operation is best effort, in the sense that in case of problems in retrieving member status, it sets
// the condition to Unknown state without returning any error.
func (w *Workload) UpdateEtcdConditions(ctx context.Context, controlPlane *ControlPlane) {
	if controlPlane.IsEtcdManaged() {
		w.updateManagedEtcdConditions(ctx, controlPlane)
		return
	}
	w.updateExternalEtcdConditions(ctx, controlPlane)
}

func (w *Workload) updateExternalEtcdConditions(ctx context.Context, controlPlane *ControlPlane) { //nolint:unparam
	// When KCP is not responsible for external etcd, we are reporting only health at KCP level.
	conditions.MarkTrue(controlPlane.KCP, controlplanev1.EtcdClusterHealthyCondition)

	// TODO: check external etcd for alarms an possibly also for member errors
	// this requires implementing an new type of etcd client generator given that it is not possible to use nodes
	// as a source for the etcd endpoint address; the address of the external etcd should be available on the kubeadm configuration.
}

func (w *Workload) updateManagedEtcdConditions(ctx context.Context, controlPlane *ControlPlane) {
	// NOTE: This methods uses control plane nodes only to get in contact with etcd but then it relies on etcd
	// as ultimate source of truth for the list of members and for their health.
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		conditions.MarkUnknown(controlPlane.KCP, controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterInspectionFailedReason, "Failed to list nodes which are hosting the etcd members")
		for _, m := range controlPlane.Machines {
			conditions.MarkUnknown(m, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to get the node which is hosting the etcd member")
		}
		return
	}

	// Update conditions for etcd members on the nodes.
	var (
		// kcpErrors is used to store errors that can't be reported on any machine.
		kcpErrors []string
		// clusterID is used to store and compare the etcd's cluster id.
		clusterID *uint64
		// members is used to store the list of etcd members and compare with all the other nodes in the cluster.
		members []*etcd.Member
	)

	for _, node := range controlPlaneNodes.Items {
		// Search for the machine corresponding to the node.
		var machine *clusterv1.Machine
		for _, m := range controlPlane.Machines {
			if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
				machine = m
			}
		}

		if machine == nil {
			// If there are machines still provisioning there is the chance that a chance that a node might be linked to a machine soon,
			// otherwise report the error at KCP level given that there is no machine to report on.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("Control plane node %s does not have a corresponding machine", node.Name))
			continue
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
			continue
		}

		// Create the etcd Client for the etcd Pod scheduled on the Node
		etcdClient, err := w.etcdClientGenerator.forFirstAvailableNode(ctx, []string{node.Name})
		if err != nil {
			conditions.MarkUnknown(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to connect to the etcd pod on the %s node: %s", node.Name, err)
			continue
		}
		defer etcdClient.Close()

		// While creating a new client, forFirstAvailableNode retrieves the status for the endpoint; check if the endpoint has errors.
		if len(etcdClient.Errors) > 0 {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member status reports errors: %s", strings.Join(etcdClient.Errors, ", "))
			continue
		}

		// Gets the list etcd members known by this member.
		currentMembers, err := etcdClient.Members(ctx)
		if err != nil {
			// NB. We should never be in here, given that we just received answer to the etcd calls included in forFirstAvailableNode;
			// however, we are considering the calls to Members a signal of etcd not being stable.
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Failed get answer from the etcd member on the %s node", node.Name)
			continue
		}

		// Check if the list of members IDs reported is the same as all other members.
		// NOTE: the first member reporting this information is the baseline for this information.
		if members == nil {
			members = currentMembers
		}
		if !etcdutil.MemberEqual(members, currentMembers) {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member reports the cluster is composed by members %s, but all previously seen etcd members are reporting %s", etcdutil.MemberNames(currentMembers), etcdutil.MemberNames(members))
			continue
		}

		// Retrieve the member and check for alarms.
		// NB. The member for this node always exists given forFirstAvailableNode(node) used above
		member := etcdutil.MemberForName(currentMembers, node.Name)
		if len(member.Alarms) > 0 {
			alarmList := []string{}
			for _, alarm := range member.Alarms {
				switch alarm {
				case etcd.AlarmOK:
					continue
				default:
					alarmList = append(alarmList, etcd.AlarmTypeName[alarm])
				}
			}
			if len(alarmList) > 0 {
				conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member reports alarms: %s", strings.Join(alarmList, ", "))
				continue
			}
		}

		// Check if the member belongs to the same cluster as all other members.
		// NOTE: the first member reporting this information is the baseline for this information.
		if clusterID == nil {
			clusterID = &member.ClusterID
		}
		if *clusterID != member.ClusterID {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member has cluster ID %d, but all previously seen etcd members have cluster ID %d", member.ClusterID, *clusterID)
			continue
		}

		conditions.MarkTrue(machine, controlplanev1.MachineEtcdMemberHealthyCondition)
	}

	// Make sure that the list of etcd members and machines is consistent.
	kcpErrors = compareMachinesAndMembers(controlPlane, members, kcpErrors)

	// Aggregate components error from machines at KCP level
	aggregateFromMachinesToKCP(aggregateFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: []clusterv1.ConditionType{controlplanev1.MachineEtcdMemberHealthyCondition},
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.EtcdClusterHealthyCondition,
		unhealthyReason:   controlplanev1.EtcdClusterUnhealthyReason,
		unknownReason:     controlplanev1.EtcdClusterUnknownReason,
		note:              "etcd member",
	})
}

func compareMachinesAndMembers(controlPlane *ControlPlane, members []*etcd.Member, kcpErrors []string) []string {
	// NOTE: We run this check only if we actually know the list of members, otherwise the first for loop
	// could generate a false negative when reporting missing etcd members.
	if members == nil {
		return kcpErrors
	}

	// Check Machine -> Etcd member.
	for _, machine := range controlPlane.Machines {
		if machine.Status.NodeRef == nil {
			continue
		}
		found := false
		for _, member := range members {
			if machine.Status.NodeRef.Name == member.Name {
				found = true
				break
			}
		}
		if !found {
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Missing etcd member")
		}
	}

	// Check Etcd member -> Machine.
	for _, member := range members {
		found := false
		for _, machine := range controlPlane.Machines {
			if machine.Status.NodeRef != nil && machine.Status.NodeRef.Name == member.Name {
				found = true
				break
			}
		}
		if !found {
			name := member.Name
			if name == "" {
				name = fmt.Sprintf("%d (Name not yet assigned)", member.ID)
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("etcd member %s does not have a corresponding machine", name))
		}
	}
	return kcpErrors
}

// UpdateStaticPodConditions is responsible for updating machine conditions reflecting the status of all the control plane
// components running in a static pod generated by kubeadm. This operation is best effort, in the sense that in case
// of problems in retrieving the pod status, it sets the condition to Unknown state without returning any error.
func (w *Workload) UpdateStaticPodConditions(ctx context.Context, controlPlane *ControlPlane) {
	allMachinePodConditions := []clusterv1.ConditionType{
		controlplanev1.MachineAPIServerPodHealthyCondition,
		controlplanev1.MachineControllerManagerPodHealthyCondition,
		controlplanev1.MachineSchedulerPodHealthyCondition,
	}
	if controlPlane.IsEtcdManaged() {
		allMachinePodConditions = append(allMachinePodConditions, controlplanev1.MachineEtcdPodHealthyCondition)
	}

	// NOTE: this fun uses control plane nodes from the workload cluster as a source of truth for the current state.
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		for i := range controlPlane.Machines {
			machine := controlPlane.Machines[i]
			for _, condition := range allMachinePodConditions {
				conditions.MarkUnknown(machine, condition, controlplanev1.PodInspectionFailedReason, "Failed to get the node which is hosting this component: %v", err)
			}
		}
		conditions.MarkUnknown(controlPlane.KCP, controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsInspectionFailedReason, "Failed to list nodes which are hosting control plane components: %v", err)
		return
	}

	// Update conditions for control plane components hosted as static pods on the nodes.
	var kcpErrors []string

	for _, node := range controlPlaneNodes.Items {
		// Search for the machine corresponding to the node.
		var machine *clusterv1.Machine
		for _, m := range controlPlane.Machines {
			if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
				machine = m
				break
			}
		}

		// If there is no machine corresponding to a node, determine if this is an error or not.
		if machine == nil {
			// If there are machines still provisioning there is the chance that a chance that a node might be linked to a machine soon,
			// otherwise report the error at KCP level given that there is no machine to report on.
			if hasProvisioningMachine(controlPlane.Machines) {
				continue
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("Control plane node %s does not have a corresponding machine", node.Name))
			continue
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
			for _, condition := range allMachinePodConditions {
				conditions.MarkFalse(machine, condition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
			}
			continue
		}

		// If the node is Unreachable, information about static pods could be stale so set all conditions to unknown.
		if nodeHasUnreachableTaint(node) {
			// NOTE: We are assuming unreachable as a temporary condition, leaving to MHC
			// the responsibility to determine if the node is unhealthy or not.
			for _, condition := range allMachinePodConditions {
				conditions.MarkUnknown(machine, condition, controlplanev1.PodInspectionFailedReason, "Node is unreachable")
			}
			continue
		}

		// Otherwise updates static pod based conditions reflecting the status of the underlying object generated by kubeadm.
		w.updateStaticPodCondition(ctx, machine, node, "kube-apiserver", controlplanev1.MachineAPIServerPodHealthyCondition)
		w.updateStaticPodCondition(ctx, machine, node, "kube-controller-manager", controlplanev1.MachineControllerManagerPodHealthyCondition)
		w.updateStaticPodCondition(ctx, machine, node, "kube-scheduler", controlplanev1.MachineSchedulerPodHealthyCondition)
		if controlPlane.IsEtcdManaged() {
			w.updateStaticPodCondition(ctx, machine, node, "etcd", controlplanev1.MachineEtcdPodHealthyCondition)
		}
	}

	// If there are provisioned machines without corresponding nodes, report this as a failing conditions with SeverityError.
	for i := range controlPlane.Machines {
		machine := controlPlane.Machines[i]
		if machine.Status.NodeRef == nil {
			continue
		}
		found := false
		for _, node := range controlPlaneNodes.Items {
			if machine.Status.NodeRef.Name == node.Name {
				found = true
				break
			}
		}
		if !found {
			for _, condition := range allMachinePodConditions {
				conditions.MarkFalse(machine, condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing node")
			}
		}
	}

	// Aggregate components error from machines at KCP level.
	aggregateFromMachinesToKCP(aggregateFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: allMachinePodConditions,
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.ControlPlaneComponentsHealthyCondition,
		unhealthyReason:   controlplanev1.ControlPlaneComponentsUnhealthyReason,
		unknownReason:     controlplanev1.ControlPlaneComponentsUnknownReason,
		note:              "control plane",
	})
}

func hasProvisioningMachine(machines collections.Machines) bool {
	for _, machine := range machines {
		if machine.Status.NodeRef == nil {
			return true
		}
	}
	return false
}

// nodeHasUnreachableTaint returns true if the node has is unreachable from the node controller.
func nodeHasUnreachableTaint(node corev1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == corev1.TaintNodeUnreachable && taint.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}
	return false
}

// updateStaticPodCondition is responsible for updating machine conditions reflecting the status of a component running
// in a static pod generated by kubeadm. This operation is best effort, in the sense that in case of problems
// in retrieving the pod status, it sets the condition to Unknown state without returning any error.
func (w *Workload) updateStaticPodCondition(ctx context.Context, machine *clusterv1.Machine, node corev1.Node, component string, staticPodCondition clusterv1.ConditionType) {
	// If node ready is unknown there is a good chance that kubelet is not updating mirror pods, so we consider pod status
	// to be unknown as well without further investigations.
	if nodeReadyUnknown(node) {
		conditions.MarkUnknown(machine, staticPodCondition, controlplanev1.PodInspectionFailedReason, "Node Ready condition is unknown, pod data might be stale")
		return
	}

	podKey := ctrlclient.ObjectKey{
		Namespace: metav1.NamespaceSystem,
		Name:      staticPodName(component, node.Name),
	}

	pod := corev1.Pod{}
	if err := w.Client.Get(ctx, podKey, &pod); err != nil {
		// If there is an error getting the Pod, do not set any conditions.
		if apierrors.IsNotFound(err) {
			conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodMissingReason, clusterv1.ConditionSeverityError, "Pod %s is missing", podKey.Name)
			return
		}
		conditions.MarkUnknown(machine, staticPodCondition, controlplanev1.PodInspectionFailedReason, "Failed to get pod status")
		return
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		// PodPending means the pod has been accepted by the system, but one or more of the containers
		// has not been started. This logic is trying to surface more details about what is happening in this phase.

		// Check if the container is still to be scheduled
		// NOTE: This should never happen for static pods, however this check is implemented for completeness.
		if podCondition(pod, corev1.PodScheduled) != corev1.ConditionTrue {
			conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting to be scheduled")
			return
		}

		// Check if the container is still running init containers
		// NOTE: As of today there are not init containers in static pods generated by kubeadm, however this check is implemented for completeness.
		if podCondition(pod, corev1.PodInitialized) != corev1.ConditionTrue {
			conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Running init containers")
			return
		}

		// If there are no error from containers, report provisioning without further details.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "")
	case corev1.PodRunning:
		// PodRunning means the pod has been bound to a node and all of the containers have been started.
		// At least one container is still running or is in the process of being restarted.
		// This logic is trying to determine if we are actually running or if we are in an intermediate state
		// like e.g. a container is retarted.

		// PodReady condition means the pod is able to service requests
		if podCondition(pod, corev1.PodReady) == corev1.ConditionTrue {
			conditions.MarkTrue(machine, staticPodCondition)
			return
		}

		// Surface wait message from containers.
		// Exception: Since default "restartPolicy" = "Always", a container that exited with error will be in waiting state (not terminated state)
		// with "CrashLoopBackOff" reason and its LastTerminationState will be non-nil.
		var containerWaitingMessages []string
		terminatedWithError := false
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.LastTerminationState.Terminated != nil && containerStatus.LastTerminationState.Terminated.ExitCode != 0 {
				terminatedWithError = true
			}
			if containerStatus.State.Waiting != nil {
				containerWaitingMessages = append(containerWaitingMessages, containerStatus.State.Waiting.Reason)
			}
		}
		if len(containerWaitingMessages) > 0 {
			if terminatedWithError {
				conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, strings.Join(containerWaitingMessages, ", "))
				return
			}
			// Note: Some error cases cannot be caught when container state == "Waiting",
			// e.g., "waiting.reason: ErrImagePull" is an error, but since LastTerminationState does not exist, this cannot be differentiated from "PodProvisioningReason"
			conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, strings.Join(containerWaitingMessages, ", "))
			return
		}

		// Surface errors message from containers.
		var containerTerminatedMessages []string
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.State.Terminated != nil {
				containerTerminatedMessages = append(containerTerminatedMessages, containerStatus.State.Terminated.Reason)
			}
		}
		if len(containerTerminatedMessages) > 0 {
			conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, strings.Join(containerTerminatedMessages, ", "))
			return
		}

		// If the pod is not yet ready, most probably it is waiting for startup or readiness probes.
		// Report this as part of the provisioning process because the corresponding control plane component is not ready yet.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting for startup or readiness probes")
	case corev1.PodSucceeded:
		// PodSucceeded means that all containers in the pod have voluntarily terminated
		// with a container exit code of 0, and the system is not going to restart any of these containers.
		// NOTE: This should never happen for the static pods running control plane components.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated")
	case corev1.PodFailed:
		// PodFailed means that all containers in the pod have terminated, and at least one container has
		// terminated in a failure (exited with a non-zero exit code or was stopped by the system).
		// NOTE: This should never happen for the static pods running control plane components.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated")
	case corev1.PodUnknown:
		// PodUnknown means that for some reason the state of the pod could not be obtained, typically due
		// to an error in communicating with the host of the pod.
		conditions.MarkUnknown(machine, staticPodCondition, controlplanev1.PodInspectionFailedReason, "Pod is reporting unknown status")
	}
}

func nodeReadyUnknown(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionUnknown
		}
	}
	return false
}

func podCondition(pod corev1.Pod, condition corev1.PodConditionType) corev1.ConditionStatus {
	for _, c := range pod.Status.Conditions {
		if c.Type == condition {
			return c.Status
		}
	}
	return corev1.ConditionUnknown
}

type aggregateFromMachinesToKCPInput struct {
	controlPlane      *ControlPlane
	machineConditions []clusterv1.ConditionType
	kcpErrors         []string
	condition         clusterv1.ConditionType
	unhealthyReason   string
	unknownReason     string
	note              string
}

// aggregateFromMachinesToKCP aggregates a group of conditions from machines to KCP.
// NOTE: this func follows the same aggregation rules used by conditions.Merge thus giving priority to
// errors, then warning, info down to unknown.
func aggregateFromMachinesToKCP(input aggregateFromMachinesToKCPInput) {
	// Aggregates machines for condition status.
	// NB. A machine could be assigned to many groups, but only the group with the highest severity will be reported.
	kcpMachinesWithErrors := sets.NewString()
	kcpMachinesWithWarnings := sets.NewString()
	kcpMachinesWithInfo := sets.NewString()
	kcpMachinesWithTrue := sets.NewString()
	kcpMachinesWithUnknown := sets.NewString()

	for i := range input.controlPlane.Machines {
		machine := input.controlPlane.Machines[i]
		for _, condition := range input.machineConditions {
			if machineCondition := conditions.Get(machine, condition); machineCondition != nil {
				switch machineCondition.Status {
				case corev1.ConditionTrue:
					kcpMachinesWithTrue.Insert(machine.Name)
				case corev1.ConditionFalse:
					switch machineCondition.Severity {
					case clusterv1.ConditionSeverityInfo:
						kcpMachinesWithInfo.Insert(machine.Name)
					case clusterv1.ConditionSeverityWarning:
						kcpMachinesWithWarnings.Insert(machine.Name)
					case clusterv1.ConditionSeverityError:
						kcpMachinesWithErrors.Insert(machine.Name)
					}
				case corev1.ConditionUnknown:
					kcpMachinesWithUnknown.Insert(machine.Name)
				}
			}
		}
	}

	// In case of at least one machine with errors or KCP level errors (nodes without machines), report false, error.
	if len(kcpMachinesWithErrors) > 0 {
		input.kcpErrors = append(input.kcpErrors, fmt.Sprintf("Following machines are reporting %s errors: %s", input.note, strings.Join(kcpMachinesWithErrors.List(), ", ")))
	}
	if len(input.kcpErrors) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityError, strings.Join(input.kcpErrors, "; "))
		return
	}

	// In case of no errors and at least one machine with warnings, report false, warnings.
	if len(kcpMachinesWithWarnings) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityWarning, "Following machines are reporting %s warnings: %s", input.note, strings.Join(kcpMachinesWithWarnings.List(), ", "))
		return
	}

	// In case of no errors, no warning, and at least one machine with info, report false, info.
	if len(kcpMachinesWithWarnings) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityWarning, "Following machines are reporting %s info: %s", input.note, strings.Join(kcpMachinesWithInfo.List(), ", "))
		return
	}

	// In case of no errors, no warning, no Info, and at least one machine with true conditions, report true.
	if len(kcpMachinesWithTrue) > 0 {
		conditions.MarkTrue(input.controlPlane.KCP, input.condition)
		return
	}

	// Otherwise, if there is at least one machine with unknown, report unknown.
	if len(kcpMachinesWithUnknown) > 0 {
		conditions.MarkUnknown(input.controlPlane.KCP, input.condition, input.unknownReason, "Following machines are reporting unknown %s status: %s", input.note, strings.Join(kcpMachinesWithUnknown.List(), ", "))
		return
	}

	// This last case should happen only if there are no provisioned machines, and thus without conditions.
	// So there will be no condition at KCP level too.
}
