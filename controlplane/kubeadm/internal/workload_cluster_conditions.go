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
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdutil "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta2conditions "sigs.k8s.io/cluster-api/util/conditions/v1beta2"
	clog "sigs.k8s.io/cluster-api/util/log"
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

func (w *Workload) updateExternalEtcdConditions(_ context.Context, controlPlane *ControlPlane) {
	// When KCP is not responsible for external etcd, we are reporting only health at KCP level.
	conditions.MarkTrue(controlPlane.KCP, controlplanev1.EtcdClusterHealthyCondition)

	// Note: KCP is going to stop setting the `EtcdClusterHealthy` condition to true in case of external etcd.
	// This will allow tools managing the external etcd instance to use the `EtcdClusterHealthy` to report back status into
	// the KubeadmControlPlane if they want to.
	// As soon as the v1beta1 condition above will be removed, we should drop this func entirely.
}

func (w *Workload) updateManagedEtcdConditions(ctx context.Context, controlPlane *ControlPlane) {
	log := ctrl.LoggerFrom(ctx)

	// Read control plane nodes (instead of using node ref from the machines) so we will avoid trying to connect to nodes
	// that have been incidentally deleted; also this allows to detect nodes without a machine.
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		for _, m := range controlPlane.Machines {
			conditions.MarkUnknown(m, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to get the Node which is hosting the etcd member")

			v1beta2conditions.Set(m, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason,
				Message: "Failed to get the Node hosting the etcd member",
			})
		}

		conditions.MarkUnknown(controlPlane.KCP, controlplanev1.EtcdClusterHealthyCondition, controlplanev1.EtcdClusterInspectionFailedReason, "Failed to list Nodes which are hosting the etcd members")

		v1beta2conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneEtcdClusterInspectionFailedV1Beta2Reason,
			Message: "Failed to get Nodes hosting the etcd cluster",
		})
		return
	}

	// Update etcd member healthy conditions for provisioning machines (machines without a node yet, and thus without a matching etcd member).
	provisioningMachines := controlPlane.Machines.Filter(collections.Not(collections.HasNode()))
	for _, machine := range provisioningMachines {
		var msg string
		if ptr.Deref(machine.Spec.ProviderID, "") != "" {
			// If the machine is at the end of the provisioning phase, with ProviderID set, but still waiting
			// for a matching Node to exists, surface this.
			msg = fmt.Sprintf("Waiting for a Node with spec.providerID %s to exist", *machine.Spec.ProviderID)
		} else {
			// If the machine is at the beginning of the provisioning phase, with ProviderID not yet set, surface this.
			msg = fmt.Sprintf("Waiting for %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind)
		}
		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason,
			Message: msg,
		})
	}

	// Update etcd member healthy conditions for machines being deleted (machines where we cannot rely on the status of kubelet/etcd member).
	for _, machine := range controlPlane.Machines.Filter(collections.HasDeletionTimestamp) {
		conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberDeletingV1Beta2Reason,
			Message: "Machine is deleting",
		})
	}

	// Update etcd member healthy conditions for machines not provisioning or deleting.
	// This is implemented by reading info about members and alarms from etcd.
	machinesNotProvisioningOrDeleting := controlPlane.Machines.Filter(collections.And(collections.HasNode(), collections.Not(collections.HasDeletionTimestamp)))
	currentMembers, alarms, err := w.getCurrentEtcdMembersAndAlarms(ctx, machinesNotProvisioningOrDeleting, controlPlaneNodes)
	if err == nil {
		controlPlane.EtcdMembers = currentMembers

		for _, machine := range machinesNotProvisioningOrDeleting {
			// Retrieve the member hosted on the machine.
			// If not found, report the issue on the machine.
			member := etcdutil.MemberForName(currentMembers, machine.Status.NodeRef.Name)
			if member == nil {
				conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member reports the cluster is composed by members %s, but the member hosted on this Machine is not included", etcdutil.MemberNames(currentMembers))

				v1beta2conditions.Set(machine, metav1.Condition{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason,
					Message: fmt.Sprintf("Etcd reports the cluster is composed by %s, but the etcd member hosted on this Machine is not included", etcdutil.MemberNames(currentMembers)),
				})
				continue
			}

			// Check for alarms for the etcd member
			alarmList := []string{}
			for _, alarm := range alarms {
				if alarm.MemberID != member.ID {
					continue
				}

				switch alarm.Type {
				case etcd.AlarmOK:
					continue
				default:
					alarmList = append(alarmList, etcd.AlarmTypeName[alarm.Type])
				}
			}
			if len(alarmList) > 0 {
				conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd member reports alarms: %s", strings.Join(alarmList, ", "))

				v1beta2conditions.Set(machine, metav1.Condition{
					Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason,
					Message: fmt.Sprintf("Etcd reports alarms: %s", strings.Join(alarmList, ", ")),
				})
				continue
			}

			// Otherwise consider the member healthy
			conditions.MarkTrue(machine, controlplanev1.MachineEtcdMemberHealthyCondition)

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:   controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Reason,
			})
		}
	} else {
		// In case of errors, getCurrentEtcdMembersAndAlarms sets etcd member healthy conditions with the proper message;
		// KCP still have to aggregate conditions set by  in case of errors + continue to reconcile at best effort, so we only log this error.
		log.Error(err, "Failed to get current etcd members and alarms")
	}

	// Check if the list of etcd members and machines match each other.
	// In case there are errors that we cannot link to a specific machine, consider them as kcp errors.
	membersAndMachinesAreMatching, membersAndMachinesCompareErrors := compareMachinesAndMembers(controlPlane, controlPlaneNodes, controlPlane.EtcdMembers)
	controlPlane.EtcdMembersAndMachinesAreMatching = membersAndMachinesAreMatching
	kcpErrors := membersAndMachinesCompareErrors

	// Report error at KCP level if there are nodes without a corresponding machine.
	// Note: Skip this check if there are machines still provisioning because there is the chance that a node might be linked to a machine soon.
	if len(provisioningMachines) == 0 {
		for _, node := range controlPlaneNodes.Items {
			isNodeRef := false
			for _, m := range controlPlane.Machines {
				if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
					isNodeRef = true
					break
				}
			}
			if !isNodeRef {
				kcpErrors = append(kcpErrors, fmt.Sprintf("Control plane Node %s does not have a corresponding Machine", node.Name))
			}
		}
	}

	// Aggregate components error from machines at KCP level
	aggregateConditionsFromMachinesToKCP(aggregateConditionsFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: []clusterv1.ConditionType{controlplanev1.MachineEtcdMemberHealthyCondition},
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.EtcdClusterHealthyCondition,
		unhealthyReason:   controlplanev1.EtcdClusterUnhealthyReason,
		unknownReason:     controlplanev1.EtcdClusterUnknownReason,
		note:              "etcd member",
	})

	aggregateV1Beta2ConditionsFromMachinesToKCP(aggregateV1Beta2ConditionsFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: []string{controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition},
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Condition,
		falseReason:       controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyV1Beta2Reason,
		unknownReason:     controlplanev1.KubeadmControlPlaneEtcdClusterHealthUnknownV1Beta2Reason,
		trueReason:        controlplanev1.KubeadmControlPlaneEtcdClusterHealthyV1Beta2Reason,
		note:              "etcd member",
	})
}

func unwrapAll(err error) error {
	for {
		newErr := errors.Unwrap(err)
		if newErr == nil {
			break
		}
		err = newErr
	}
	return err
}

// getCurrentEtcdMembersAndAlarms returns the current list of etcd member and alarms.
// Considering that the underlying etcd SDK calls (MemberList and AlarmList) requires quorum across all etcd members, it is possible
// to run those calls towards any etcd Pod hosting an etcd member.
func (w *Workload) getCurrentEtcdMembersAndAlarms(ctx context.Context, machines collections.Machines, nodes *corev1.NodeList) ([]*etcd.Member, []etcd.MemberAlarm, error) {
	// Get the list of nodes hosting an etcd member sorted by the last known etcd health,
	// so the client generator in the following line will try to connect first to nodes with higher chance to answer.
	nodeNames := getNodeNamesSortedByLastKnownEtcdHealth(nodes, machines)
	if len(nodeNames) == 0 {
		return nil, nil, nil
	}

	// Create the etcd Client for one of the etcd Pods running on the given nodes.
	etcdClient, err := w.etcdClientGenerator.forFirstAvailableNode(ctx, nodeNames)
	if err != nil {
		for _, m := range machines {
			conditions.MarkUnknown(m, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberInspectionFailedReason, "Failed to connect to etcd: %s", err)

			v1beta2conditions.Set(m, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason,
				Message: fmt.Sprintf("Failed to connect to etcd: %s", unwrapAll(err)),
			})
		}
		return nil, nil, errors.Wrapf(err, "failed to get an etcd client for %s Nodes", strings.Join(nodeNames, ","))
	}
	defer etcdClient.Close()

	// While creating a new client, forFirstAvailableNode also reads the status for the endpoint we are connected to; check if the endpoint has errors.
	if len(etcdClient.Errors) > 0 {
		for _, m := range machines {
			conditions.MarkFalse(m, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Etcd endpoint %s reports errors: %s", etcdClient.Endpoint, strings.Join(etcdClient.Errors, ", "))

			v1beta2conditions.Set(m, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason,
				Message: fmt.Sprintf("Etcd endpoint %s reports errors: %s", etcdClient.Endpoint, strings.Join(etcdClient.Errors, ", ")),
			})
		}
		return nil, nil, errors.Errorf("etcd endpoint %s reports errors: %s", etcdClient.Endpoint, strings.Join(etcdClient.Errors, ", "))
	}

	// Gets the list of etcd members in the cluster.
	currentMembers, err := etcdClient.Members(ctx)
	if err != nil {
		for _, m := range machines {
			conditions.MarkFalse(m, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Failed to get etcd members")

			v1beta2conditions.Set(m, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason,
				Message: fmt.Sprintf("Failed to get etcd members: %s", unwrapAll(err)),
			})
		}
		return nil, nil, errors.Wrapf(err, "failed to get etcd members")
	}

	// Gets the list of etcd alarms.
	alarms, err := etcdClient.Alarms(ctx)
	if err != nil {
		for _, m := range machines {
			conditions.MarkFalse(m, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Failed to get etcd alarms")

			v1beta2conditions.Set(m, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberInspectionFailedV1Beta2Reason,
				Message: fmt.Sprintf("Failed to get etcd alarms: %s", unwrapAll(err)),
			})
		}
		return nil, nil, errors.Wrapf(err, "failed to get etcd alarms")
	}

	return currentMembers, alarms, nil
}

// getNodeNamesSortedByLastKnownEtcdHealth return the list of nodes hosting an etcd member sorted by the last known etcd health.
// Note: sorting by last known etcd health is a best effort operations; only nodes with a corresponding machine are considered.
func getNodeNamesSortedByLastKnownEtcdHealth(nodes *corev1.NodeList, machines collections.Machines) []string {
	// Get the list of nodes and the corresponding MachineEtcdMemberHealthyCondition
	eligibleNodes := sets.Set[string]{}
	nodeEtcdHealthyCondition := map[string]clusterv1.Condition{}

	for _, node := range nodes.Items {
		var machine *clusterv1.Machine
		for _, m := range machines {
			if m.Status.NodeRef != nil && m.Status.NodeRef.Name == node.Name {
				machine = m
				break
			}
		}
		// Ignore nodes without a corresponding machine (this should never happen).
		if machine == nil {
			continue
		}

		eligibleNodes.Insert(node.Name)
		if c := conditions.Get(machine, controlplanev1.MachineEtcdMemberHealthyCondition); c != nil {
			nodeEtcdHealthyCondition[node.Name] = *c
			continue
		}
		nodeEtcdHealthyCondition[node.Name] = clusterv1.Condition{
			Type:   controlplanev1.MachineEtcdMemberHealthyCondition,
			Status: corev1.ConditionUnknown,
		}
	}

	// Sort by nodes by last known etcd member health.
	nodeNames := eligibleNodes.UnsortedList()
	sort.Slice(nodeNames, func(i, j int) bool {
		iCondition := nodeEtcdHealthyCondition[nodeNames[i]]
		jCondition := nodeEtcdHealthyCondition[nodeNames[j]]

		// Nodes with last known etcd healthy members goes first, because most likely we can connect to them again.
		// NOTE: This isn't always true, it is a best effort assumption (e.g. kubelet might have issues preventing connection to an healthy member to be established).
		if iCondition.Status == corev1.ConditionTrue && jCondition.Status != corev1.ConditionTrue {
			return true
		}
		if iCondition.Status != corev1.ConditionTrue && jCondition.Status == corev1.ConditionTrue {
			return false
		}

		// Note: we are not making assumption on the chances to connect when last known etcd health is FALSE and UNKNOWN.

		// Otherwise pick randomly one of the nodes to avoid trying to connect always to the same nodes first.
		// Note: the list originate from set.UnsortedList which internally uses a Map, and we consider this enough as a randomizer.
		return i < j
	})
	return nodeNames
}

func compareMachinesAndMembers(controlPlane *ControlPlane, nodes *corev1.NodeList, members []*etcd.Member) (bool, []string) {
	membersAndMachinesAreMatching := true
	var kcpErrors []string

	// If it failed to get members, consider the check failed in case there is at least a machine already provisioned.
	if members == nil {
		if len(controlPlane.Machines.Filter(collections.HasNode())) > 0 {
			membersAndMachinesAreMatching = false
		}
		return membersAndMachinesAreMatching, nil
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
			// Surface there is a machine without etcd member on machine's EtcdMemberHealthy condition.
			// The same info will also surface into the EtcdClusterHealthy condition on kcp.
			conditions.MarkFalse(machine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "Missing etcd member")

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyV1Beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyV1Beta2Reason,
				Message: fmt.Sprintf("Etcd doesn't have an etcd member for Node %s", machine.Status.NodeRef.Name),
			})

			// Instead, surface there is a machine without etcd member on kcp's' Available condition
			// only if the machine is not deleting and the node exists by more than two minutes
			// (this prevents the condition to flick during scale up operations).
			// Note: Two minutes is the time after which we expect the system to detect the new etcd member on the machine.
			if machine.DeletionTimestamp.IsZero() {
				oldNode := false
				if nodes != nil {
					for _, node := range nodes.Items {
						if machine.Status.NodeRef.Name == node.Name && time.Since(node.CreationTimestamp.Time) > 2*time.Minute {
							oldNode = true
						}
					}
				}
				if oldNode {
					membersAndMachinesAreMatching = false
				}
			}
		}
	}

	// Check Etcd member -> Machine.
	for _, member := range members {
		found := false
		hasProvisioningMachine := false
		for _, machine := range controlPlane.Machines {
			if machine.Status.NodeRef == nil {
				hasProvisioningMachine = true
				continue
			}
			if machine.Status.NodeRef.Name == member.Name {
				found = true
				break
			}
		}
		if !found {
			// Surface there is an etcd member without a machine into the EtcdClusterHealthy condition on kcp.
			name := member.Name
			if name == "" {
				name = fmt.Sprintf("%d (Name not yet assigned)", member.ID)
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("Etcd member %s does not have a corresponding Machine", name))

			// Instead, surface there is an etcd member without a machine on kcp's Available condition
			// only if there are no provisioning machines (this prevents the condition to flick during scale up operations).
			if !hasProvisioningMachine {
				membersAndMachinesAreMatching = false
			}
		}
	}
	return membersAndMachinesAreMatching, kcpErrors
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

	allMachinePodV1beta2Conditions := []string{
		controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition,
		controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition,
		controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition,
	}
	if controlPlane.IsEtcdManaged() {
		allMachinePodV1beta2Conditions = append(allMachinePodV1beta2Conditions, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition)
	}

	// NOTE: this fun uses control plane nodes from the workload cluster as a source of truth for the current state.
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		for i := range controlPlane.Machines {
			machine := controlPlane.Machines[i]
			for _, condition := range allMachinePodConditions {
				conditions.MarkUnknown(machine, condition, controlplanev1.PodInspectionFailedReason, "Failed to get the Node which is hosting this component: %v", err)
			}

			for _, condition := range allMachinePodV1beta2Conditions {
				v1beta2conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
					Message: fmt.Sprintf("Failed to get the Node hosting the Pod: %s", err.Error()),
				})
			}
		}

		conditions.MarkUnknown(controlPlane.KCP, controlplanev1.ControlPlaneComponentsHealthyCondition, controlplanev1.ControlPlaneComponentsInspectionFailedReason, "Failed to list Nodes which are hosting control plane components: %v", err)

		v1beta2conditions.Set(controlPlane.KCP, metav1.Condition{
			Type:    controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneControlPlaneComponentsInspectionFailedV1Beta2Reason,
			Message: fmt.Sprintf("Failed to get Nodes hosting control plane components: %s", err.Error()),
		})
		return
	}

	// Update conditions for control plane components hosted as static pods on the nodes.
	var kcpErrors []string

	provisioningMachines := controlPlane.Machines.Filter(collections.Not(collections.HasNode()))
	for _, machine := range provisioningMachines {
		for _, condition := range allMachinePodV1beta2Conditions {
			var msg string
			if ptr.Deref(machine.Spec.ProviderID, "") != "" {
				// If the machine is at the end of the provisioning phase, with ProviderID set, but still waiting
				// for a matching Node to exists, surface this.
				msg = fmt.Sprintf("Waiting for a Node with spec.providerID %s to exist", *machine.Spec.ProviderID)
			} else {
				// If the machine is at the beginning of the provisioning phase, with ProviderID not yet set, surface this.
				msg = fmt.Sprintf("Waiting for %s to report spec.providerID", machine.Spec.InfrastructureRef.Kind)
			}
			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    condition,
				Status:  metav1.ConditionUnknown,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
				Message: msg,
			})
		}
	}

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
			// If there are machines still provisioning there is the chance that a node might be linked to a machine soon,
			// otherwise report the error at KCP level given that there is no machine to report on.
			if len(provisioningMachines) > 0 {
				continue
			}
			kcpErrors = append(kcpErrors, fmt.Sprintf("Control plane Node %s does not have a corresponding Machine", node.Name))
			continue
		}

		// If the machine is deleting, report all the conditions as deleting
		if !machine.ObjectMeta.DeletionTimestamp.IsZero() {
			for _, condition := range allMachinePodConditions {
				conditions.MarkFalse(machine, condition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
			}

			for _, condition := range allMachinePodV1beta2Conditions {
				v1beta2conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodDeletingV1Beta2Reason,
					Message: "Machine is deleting",
				})
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

			for _, condition := range allMachinePodV1beta2Conditions {
				v1beta2conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
					Message: fmt.Sprintf("Node %s is unreachable", node.Name),
				})
			}
			continue
		}

		// Otherwise updates static pod based conditions reflecting the status of the underlying object generated by kubeadm.
		w.updateStaticPodCondition(ctx, machine, node, "kube-apiserver", controlplanev1.MachineAPIServerPodHealthyCondition, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyV1Beta2Condition)
		w.updateStaticPodCondition(ctx, machine, node, "kube-controller-manager", controlplanev1.MachineControllerManagerPodHealthyCondition, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyV1Beta2Condition)
		w.updateStaticPodCondition(ctx, machine, node, "kube-scheduler", controlplanev1.MachineSchedulerPodHealthyCondition, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyV1Beta2Condition)
		if controlPlane.IsEtcdManaged() {
			w.updateStaticPodCondition(ctx, machine, node, "etcd", controlplanev1.MachineEtcdPodHealthyCondition, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyV1Beta2Condition)
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
				conditions.MarkFalse(machine, condition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "Missing Node")
			}

			for _, condition := range allMachinePodV1beta2Conditions {
				v1beta2conditions.Set(machine, metav1.Condition{
					Type:    condition,
					Status:  metav1.ConditionUnknown,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
					Message: fmt.Sprintf("Node %s does not exist", machine.Status.NodeRef.Name),
				})
			}
		}
	}

	// Aggregate components error from machines at KCP level.
	aggregateConditionsFromMachinesToKCP(aggregateConditionsFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: allMachinePodConditions,
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.ControlPlaneComponentsHealthyCondition,
		unhealthyReason:   controlplanev1.ControlPlaneComponentsUnhealthyReason,
		unknownReason:     controlplanev1.ControlPlaneComponentsUnknownReason,
		note:              "control plane",
	})

	aggregateV1Beta2ConditionsFromMachinesToKCP(aggregateV1Beta2ConditionsFromMachinesToKCPInput{
		controlPlane:      controlPlane,
		machineConditions: allMachinePodV1beta2Conditions,
		kcpErrors:         kcpErrors,
		condition:         controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Condition,
		falseReason:       controlplanev1.KubeadmControlPlaneControlPlaneComponentsNotHealthyV1Beta2Reason,
		unknownReason:     controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthUnknownV1Beta2Reason,
		trueReason:        controlplanev1.KubeadmControlPlaneControlPlaneComponentsHealthyV1Beta2Reason,
		note:              "control plane",
	})
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
func (w *Workload) updateStaticPodCondition(ctx context.Context, machine *clusterv1.Machine, node corev1.Node, component string, staticPodCondition clusterv1.ConditionType, staticPodV1beta2Condition string) {
	log := ctrl.LoggerFrom(ctx)

	// If node ready is unknown there is a good chance that kubelet is not updating mirror pods, so we consider pod status
	// to be unknown as well without further investigations.
	if nodeReadyUnknown(node) {
		conditions.MarkUnknown(machine, staticPodCondition, controlplanev1.PodInspectionFailedReason, "Node Ready condition is Unknown, Pod data might be stale")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    staticPodV1beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
			Message: "Node Ready condition is Unknown, Pod data might be stale",
		})
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

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    staticPodV1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodDoesNotExistV1Beta2Reason,
				Message: "Pod does not exist",
			})
			return
		}
		conditions.MarkUnknown(machine, staticPodCondition, controlplanev1.PodInspectionFailedReason, "Failed to get Pod status")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    staticPodV1beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
			Message: "Please check controller logs for errors",
		})

		log.Error(err, fmt.Sprintf("Failed to get Pod %s", klog.KRef(podKey.Namespace, podKey.Name)))
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

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    staticPodV1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: "Waiting to be scheduled",
			})
			return
		}

		// Check if the container is still running init containers
		// NOTE: As of today there are not init containers in static pods generated by kubeadm, however this check is implemented for completeness.
		if podCondition(pod, corev1.PodInitialized) != corev1.ConditionTrue {
			conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Running init containers")

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    staticPodV1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: "Running init containers",
			})
			return
		}

		// If there are no error from containers, report provisioning without further details.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:   staticPodV1beta2Condition,
			Status: metav1.ConditionFalse,
			Reason: controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
		})
	case corev1.PodRunning:
		// PodRunning means the pod has been bound to a node and all of the containers have been started.
		// At least one container is still running or is in the process of being restarted.
		// This logic is trying to determine if we are actually running or if we are in an intermediate state
		// like e.g. a container is retarted.

		// PodReady condition means the pod is able to service requests
		if podCondition(pod, corev1.PodReady) == corev1.ConditionTrue {
			conditions.MarkTrue(machine, staticPodCondition)

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:   staticPodV1beta2Condition,
				Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningV1Beta2Reason,
			})
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

				v1beta2conditions.Set(machine, metav1.Condition{
					Type:    staticPodV1beta2Condition,
					Status:  metav1.ConditionFalse,
					Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
					Message: strings.Join(containerWaitingMessages, ", "),
				})
				return
			}
			// Note: Some error cases cannot be caught when container state == "Waiting",
			// e.g., "waiting.reason: ErrImagePull" is an error, but since LastTerminationState does not exist, this cannot be differentiated from "PodProvisioningReason"
			conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, strings.Join(containerWaitingMessages, ", "))

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    staticPodV1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
				Message: strings.Join(containerWaitingMessages, ", "),
			})
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

			v1beta2conditions.Set(machine, metav1.Condition{
				Type:    staticPodV1beta2Condition,
				Status:  metav1.ConditionFalse,
				Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
				Message: strings.Join(containerTerminatedMessages, ", "),
			})
			return
		}

		// If the pod is not yet ready, most probably it is waiting for startup or readiness probes.
		// Report this as part of the provisioning process because the corresponding control plane component is not ready yet.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodProvisioningReason, clusterv1.ConditionSeverityInfo, "Waiting for startup or readiness probes")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    staticPodV1beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneMachinePodProvisioningV1Beta2Reason,
			Message: "Waiting for startup or readiness probes",
		})
	case corev1.PodSucceeded:
		// PodSucceeded means that all containers in the pod have voluntarily terminated
		// with a container exit code of 0, and the system is not going to restart any of these containers.
		// NOTE: This should never happen for the static pods running control plane components.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    staticPodV1beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
			Message: "All the containers have been terminated",
		})
	case corev1.PodFailed:
		// PodFailed means that all containers in the pod have terminated, and at least one container has
		// terminated in a failure (exited with a non-zero exit code or was stopped by the system).
		// NOTE: This should never happen for the static pods running control plane components.
		conditions.MarkFalse(machine, staticPodCondition, controlplanev1.PodFailedReason, clusterv1.ConditionSeverityError, "All the containers have been terminated")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    staticPodV1beta2Condition,
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.KubeadmControlPlaneMachinePodFailedV1Beta2Reason,
			Message: "All the containers have been terminated",
		})
	case corev1.PodUnknown:
		// PodUnknown means that for some reason the state of the pod could not be obtained, typically due
		// to an error in communicating with the host of the pod.
		conditions.MarkUnknown(machine, staticPodCondition, controlplanev1.PodInspectionFailedReason, "Pod is reporting Unknown status")

		v1beta2conditions.Set(machine, metav1.Condition{
			Type:    staticPodV1beta2Condition,
			Status:  metav1.ConditionUnknown,
			Reason:  controlplanev1.KubeadmControlPlaneMachinePodInspectionFailedV1Beta2Reason,
			Message: "Pod is reporting Unknown status",
		})
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

type aggregateConditionsFromMachinesToKCPInput struct {
	controlPlane      *ControlPlane
	machineConditions []clusterv1.ConditionType
	kcpErrors         []string
	condition         clusterv1.ConditionType
	unhealthyReason   string
	unknownReason     string
	note              string
}

// aggregateConditionsFromMachinesToKCP aggregates a group of conditions from machines to KCP.
// NOTE: this func follows the same aggregation rules used by conditions.Merge thus giving priority to
// errors, then warning, info down to unknown.
func aggregateConditionsFromMachinesToKCP(input aggregateConditionsFromMachinesToKCPInput) {
	// Aggregates machines for condition status.
	// NB. A machine could be assigned to many groups, but only the group with the highest severity will be reported.
	kcpMachinesWithErrors := sets.Set[string]{}
	kcpMachinesWithWarnings := sets.Set[string]{}
	kcpMachinesWithInfo := sets.Set[string]{}
	kcpMachinesWithTrue := sets.Set[string]{}
	kcpMachinesWithUnknown := sets.Set[string]{}

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
		input.kcpErrors = append(input.kcpErrors, fmt.Sprintf("Following Machines are reporting %s errors: %s", input.note, strings.Join(sets.List(kcpMachinesWithErrors), ", ")))
	}
	if len(input.kcpErrors) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityError, strings.Join(input.kcpErrors, "; "))
		return
	}

	// In case of no errors and at least one machine with warnings, report false, warnings.
	if len(kcpMachinesWithWarnings) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityWarning, "Following Machines are reporting %s warnings: %s", input.note, strings.Join(sets.List(kcpMachinesWithWarnings), ", "))
		return
	}

	// In case of no errors, no warning, and at least one machine with info, report false, info.
	if len(kcpMachinesWithInfo) > 0 {
		conditions.MarkFalse(input.controlPlane.KCP, input.condition, input.unhealthyReason, clusterv1.ConditionSeverityInfo, "Following Machines are reporting %s info: %s", input.note, strings.Join(sets.List(kcpMachinesWithInfo), ", "))
		return
	}

	// In case of no errors, no warning, no Info, and at least one machine with true conditions, report true.
	if len(kcpMachinesWithTrue) > 0 {
		conditions.MarkTrue(input.controlPlane.KCP, input.condition)
		return
	}

	// Otherwise, if there is at least one machine with unknown, report unknown.
	if len(kcpMachinesWithUnknown) > 0 {
		conditions.MarkUnknown(input.controlPlane.KCP, input.condition, input.unknownReason, "Following Machines are reporting unknown %s status: %s", input.note, strings.Join(sets.List(kcpMachinesWithUnknown), ", "))
		return
	}

	// This last case should happen only if there are no provisioned machines, and thus without conditions.
	// So there will be no condition at KCP level too.
}

type aggregateV1Beta2ConditionsFromMachinesToKCPInput struct {
	controlPlane      *ControlPlane
	machineConditions []string
	kcpErrors         []string
	condition         string
	trueReason        string
	unknownReason     string
	falseReason       string
	note              string
}

// aggregateV1Beta2ConditionsFromMachinesToKCP aggregates a group of conditions from machines to KCP.
// Note: the aggregation is computed in way that is similar to how v1beta2conditions.NewAggregateCondition works, but in this case the
// implementation is simpler/less flexible and it surfaces only issues & unknown conditions.
func aggregateV1Beta2ConditionsFromMachinesToKCP(input aggregateV1Beta2ConditionsFromMachinesToKCPInput) {
	// Aggregates machines for condition status.
	// NB. A machine could be assigned to many groups, but only the group with the highest severity will be reported.
	kcpMachinesWithErrors := sets.Set[string]{}
	kcpMachinesWithUnknown := sets.Set[string]{}
	kcpMachinesWithInfo := sets.Set[string]{}

	messageMap := map[string][]string{}
	for i := range input.controlPlane.Machines {
		machine := input.controlPlane.Machines[i]
		machineMessages := []string{}
		conditionCount := 0
		conditionMessages := sets.Set[string]{}
		for _, condition := range input.machineConditions {
			if machineCondition := v1beta2conditions.Get(machine, condition); machineCondition != nil {
				conditionCount++
				conditionMessages.Insert(machineCondition.Message)
				switch machineCondition.Status {
				case metav1.ConditionTrue:
					kcpMachinesWithInfo.Insert(machine.Name)
				case metav1.ConditionFalse:
					kcpMachinesWithErrors.Insert(machine.Name)
					m := machineCondition.Message
					if m == "" {
						m = fmt.Sprintf("condition is %s", machineCondition.Status)
					}
					machineMessages = append(machineMessages, fmt.Sprintf("  * %s: %s", machineCondition.Type, m))
				case metav1.ConditionUnknown:
					// Ignore unknown when the machine doesn't have a provider ID yet (which also implies infrastructure not ready).
					// Note: this avoids some noise when a new machine is provisioning; it is not possible to delay further
					// because the etcd member might join the cluster / control plane components might start even before
					// kubelet registers the node to the API server (e.g. in case kubelet has issues to register itself).
					if machine.Spec.ProviderID == nil {
						kcpMachinesWithInfo.Insert(machine.Name)
						break
					}

					kcpMachinesWithUnknown.Insert(machine.Name)
					m := machineCondition.Message
					if m == "" {
						m = fmt.Sprintf("condition is %s", machineCondition.Status)
					}
					machineMessages = append(machineMessages, fmt.Sprintf("  * %s: %s", machineCondition.Type, m))
				}
			}
		}

		if len(machineMessages) > 0 {
			if conditionCount > 1 && len(conditionMessages) == 1 {
				message := fmt.Sprintf("  * Control plane components: %s", conditionMessages.UnsortedList()[0])
				messageMap[message] = append(messageMap[message], machine.Name)
				continue
			}

			message := strings.Join(machineMessages, "\n")
			messageMap[message] = append(messageMap[message], machine.Name)
		}
	}

	// compute the order of messages according to the number of machines reporting the same message.
	// Note: The list of object names is used as a secondary criteria to sort messages with the same number of objects.
	messageIndex := make([]string, 0, len(messageMap))
	for m := range messageMap {
		messageIndex = append(messageIndex, m)
	}

	sort.SliceStable(messageIndex, func(i, j int) bool {
		return len(messageMap[messageIndex[i]]) > len(messageMap[messageIndex[j]]) ||
			(len(messageMap[messageIndex[i]]) == len(messageMap[messageIndex[j]]) && strings.Join(messageMap[messageIndex[i]], ",") < strings.Join(messageMap[messageIndex[j]], ","))
	})

	// Build the message
	messages := []string{}
	for _, message := range messageIndex {
		machines := messageMap[message]
		machinesMessage := "Machine"
		if len(messageMap[message]) > 1 {
			machinesMessage += "s"
		}

		sort.Strings(machines)
		machinesMessage += " " + clog.ListToString(machines, func(s string) string { return s }, 3)

		messages = append(messages, fmt.Sprintf("* %s:\n%s", machinesMessage, message))
	}

	// Append messages impacting KCP as a whole, if any
	if len(input.kcpErrors) > 0 {
		for _, message := range input.kcpErrors {
			messages = append(messages, fmt.Sprintf("* %s", message))
		}
	}
	message := strings.Join(messages, "\n")

	// In case of at least one machine with errors or KCP level errors (nodes without machines), report false.
	if len(input.kcpErrors) > 0 || len(kcpMachinesWithErrors) > 0 {
		v1beta2conditions.Set(input.controlPlane.KCP, metav1.Condition{
			Type:    input.condition,
			Status:  metav1.ConditionFalse,
			Reason:  input.falseReason,
			Message: message,
		})
		return
	}

	// Otherwise, if there is at least one machine with unknown, report unknown.
	if len(kcpMachinesWithUnknown) > 0 {
		v1beta2conditions.Set(input.controlPlane.KCP, metav1.Condition{
			Type:    input.condition,
			Status:  metav1.ConditionUnknown,
			Reason:  input.unknownReason,
			Message: message,
		})
		return
	}

	// In case of no errors, no unknown, and at least one machine with info, report true.
	if len(kcpMachinesWithInfo) > 0 {
		v1beta2conditions.Set(input.controlPlane.KCP, metav1.Condition{
			Type:   input.condition,
			Status: metav1.ConditionTrue,
			Reason: input.trueReason,
		})
		return
	}

	// This last case should happen only if there are no provisioned machines.
	v1beta2conditions.Set(input.controlPlane.KCP, metav1.Condition{
		Type:    input.condition,
		Status:  metav1.ConditionUnknown,
		Reason:  input.unknownReason,
		Message: fmt.Sprintf("No Machines reporting %s status", input.note),
	})
}
