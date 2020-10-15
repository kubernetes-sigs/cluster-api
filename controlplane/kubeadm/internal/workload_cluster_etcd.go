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
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	controlplanev1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdutil "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type etcdClientFor interface {
	forNodes(ctx context.Context, nodes []corev1.Node) (*etcd.Client, error)
	forLeader(ctx context.Context, nodes []corev1.Node) (*etcd.Client, error)
}

// EtcdIsHealthy runs checks for every etcd member in the cluster to satisfy our definition of healthy.
// This is a best effort check and nodes can become unhealthy after the check is complete. It is not a guarantee.
// It's used a signal for if we should allow a target cluster to scale up, scale down or upgrade.
// It returns a map of nodes checked along with an error for a given node.
func (w *Workload) EtcdIsHealthy(ctx context.Context, controlPlane *ControlPlane) (HealthCheckResult, error) {
	var knownClusterID uint64
	var knownMemberIDSet etcdutil.UInt64Set

	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, err
	}

	expectedMembers := 0
	response := make(map[string]error)
	var owningMachine *clusterv1.Machine

	// Initial assumtion is that etcd cluster is healthy. If otherwise is observed below, it is set to false.
	conditions.MarkTrue(controlPlane.KCP, controlplanev1.EtcdClusterHealthy)

	for _, node := range controlPlaneNodes.Items {
		name := node.Name
		response[name] = nil
		if node.Spec.ProviderID == "" {
			response[name] = errors.New("empty provider ID")
			continue
		}

		for _, m := range controlPlane.Machines {
			if m.Spec.ProviderID != nil && *m.Spec.ProviderID == node.Spec.ProviderID {
				owningMachine = m
				// Only set this condition if the node has an owning machine.
				conditions.MarkTrue(owningMachine, controlplanev1.MachineEtcdMemberHealthyCondition)
				break
			}
		}

		// TODO: If owning machine is nil, should not continue. But this change breaks the logic below.

		// Check etcd pod's health
		etcdPodState, _ := w.reconcilePodStatusCondition(EtcdPodNamePrefix, node, owningMachine, controlplanev1.MachineEtcdPodHealthyCondition)
		// etcdPodState is
		if !etcdPodState.ready {
			// Nothing wrong here, etcd on this node is just not running.
			// If it's a true failure the healthcheck will fail since it won't have checked enough members.
			response[name] = errors.Wrap(err, "etcd pod is not ready")
			continue
		}
		// Only expect a member reports healthy if its pod is ready.
		// This fixes the known state where the control plane has a crash-looping etcd pod that is not part of the
		// etcd cluster.
		expectedMembers++

		// Create the etcd Client for the etcd Pod scheduled on the Node
		etcdClient, err := w.etcdClientGenerator.forNodes(ctx, []corev1.Node{node})
		if err != nil {
			if owningMachine != nil {
				conditions.MarkFalse(owningMachine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd client related failure.")
			}
			response[name] = errors.Wrap(err, "failed to create etcd client")
			continue
		}
		defer etcdClient.Close()

		if err := etcdClient.HealthCheck(ctx); err != nil {
			response[name] = errors.Wrap(err, "etcd member is unhealthy")
			continue
		}
		// List etcd members. This checks that the member is healthy, because the request goes through consensus.
		members, err := etcdClient.Members(ctx)
		if err != nil {
			if owningMachine != nil {
				conditions.MarkFalse(owningMachine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd client related failure.")
			}
			response[name] = errors.Wrap(err, "failed to list etcd members using etcd client")
			continue
		}

		member := etcdutil.MemberForName(members, name)

		// Check that the member reports no alarms.
		if len(member.Alarms) > 0 {
			if owningMachine != nil {
				conditions.MarkFalse(owningMachine, controlplanev1.MachineEtcdMemberHealthyCondition, controlplanev1.EtcdMemberUnhealthyReason, clusterv1.ConditionSeverityError, "etcd member has alarms.")
			}
			response[name] = errors.Errorf("etcd member reports alarms: %v", member.Alarms)
			continue
		}

		// Check that the member belongs to the same cluster as all other members.
		clusterID := member.ClusterID
		if knownClusterID == 0 {
			knownClusterID = clusterID
		} else if knownClusterID != clusterID {
			response[name] = errors.Errorf("etcd member has cluster ID %d, but all previously seen etcd members have cluster ID %d", clusterID, knownClusterID)
			continue
		}

		// Check that the member list is stable.
		memberIDSet := etcdutil.MemberIDSet(members)
		if knownMemberIDSet.Len() == 0 {
			knownMemberIDSet = memberIDSet
		} else {
			unknownMembers := memberIDSet.Difference(knownMemberIDSet)
			if unknownMembers.Len() > 0 {
				conditions.MarkFalse(controlPlane.KCP, controlplanev1.EtcdClusterHealthy, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityWarning, "etcd members do not have the same member-list view")
				response[name] = errors.Errorf("etcd member reports members IDs %v, but all previously seen etcd members reported member IDs %v", memberIDSet.UnsortedList(), knownMemberIDSet.UnsortedList())
			}
			continue
		}
	}

	// TODO: ensure that each pod is owned by a node that we're managing. That would ensure there are no out-of-band etcd members

	// Check that there is exactly one etcd member for every healthy pod.
	// This allows us to handle the expected case where there is a failing pod but it's been removed from the member list.
	if expectedMembers != len(knownMemberIDSet) {
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.EtcdClusterHealthy, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityWarning, "etcd pods does not match with etcd members.")
		return response, errors.Errorf("there are %d healthy etcd pods, but %d etcd members", expectedMembers, len(knownMemberIDSet))
	}

	// Check etcd cluster alarms
	etcdClient, err := w.etcdClientGenerator.forNodes(ctx, controlPlaneNodes.Items)
	if err != nil {
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.EtcdClusterHealthy, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityWarning, "failed to get etcd client.")
		return response, err
	}

	defer etcdClient.Close()
	alarmList, err := etcdClient.Alarms(ctx)
	if len(alarmList) > 0 || err != nil {
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.EtcdClusterHealthy, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityWarning, "etcd cluster has alarms.")
		return response, errors.Errorf("etcd cluster has %d alarms", len(alarmList))
	}

	members, err := etcdClient.Members(ctx)
	if err != nil {
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.EtcdClusterHealthy, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityWarning, "failed to get etcd members.")
		return response, err
	}

	healthyMembers := 0
	for _, m := range members {
		if val, ok := response[m.Name]; ok {
			if val == nil {
				healthyMembers++
			}
		} else {
			// There are members in etcd cluster that is not part of controlplane nodes.
			conditions.MarkFalse(controlPlane.KCP, controlplanev1.EtcdClusterHealthy, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityWarning, "unknown etcd member that is not part of control plane nodes.")
			return response, err
		}
	}
	// TODO: During provisioning, this condition may be set false for a short time until all pods are provisioned, can add additional checks here to prevent this.
	if healthyMembers < (len(members)/2 + 1) {
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.EtcdClusterHealthy, controlplanev1.EtcdClusterUnhealthyReason, clusterv1.ConditionSeverityWarning, "etcd cluster's quorum is lost.")
		return response, errors.Errorf("etcd lost its quorum: there are %d control-plane machines, but %d etcd members", controlPlane.Machines.Len(), len(knownMemberIDSet))
	}

	return response, nil
}

// ReconcileEtcdMembers iterates over all etcd members and finds members that do not have corresponding nodes.
// If there are any such members, it deletes them from etcd and removes their nodes from the kubeadm configmap so that kubeadm does not run etcd health checks on them.
func (w *Workload) ReconcileEtcdMembers(ctx context.Context) error {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return err
	}

	errs := []error{}
	for _, node := range controlPlaneNodes.Items {
		// Create the etcd Client for the etcd Pod scheduled on the Node
		etcdClient, err := w.etcdClientGenerator.forNodes(ctx, []corev1.Node{node})
		if err != nil {
			continue
		}
		defer etcdClient.Close()

		members, err := etcdClient.Members(ctx)
		if err != nil {
			continue
		}
		// Check if any member's node is missing from workload cluster
		// If any, delete it with best effort
		for _, member := range members {
			isFound := false
			for _, node := range controlPlaneNodes.Items {
				if member.Name == node.Name {
					isFound = true
					break
				}
			}
			// Stop here if we found the member to be in the list of control plane nodes.
			if isFound {
				continue
			}
			if err := w.removeMemberForNode(ctx, member.Name); err != nil {
				errs = append(errs, err)
			}

			if err := w.RemoveNodeFromKubeadmConfigMap(ctx, member.Name); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return kerrors.NewAggregate(errs)
}

// UpdateEtcdVersionInKubeadmConfigMap sets the imageRepository or the imageTag or both in the kubeadm config map.
func (w *Workload) UpdateEtcdVersionInKubeadmConfigMap(ctx context.Context, imageRepository, imageTag string) error {
	configMapKey := ctrlclient.ObjectKey{Name: kubeadmConfigKey, Namespace: metav1.NamespaceSystem}
	kubeadmConfigMap, err := w.getConfigMap(ctx, configMapKey)
	if err != nil {
		return err
	}
	config := &kubeadmConfig{ConfigMap: kubeadmConfigMap}
	changed, err := config.UpdateEtcdMeta(imageRepository, imageTag)
	if err != nil || !changed {
		return err
	}
	if err := w.Client.Update(ctx, config.ConfigMap); err != nil {
		return errors.Wrap(err, "error updating kubeadm ConfigMap")
	}
	return nil
}

// RemoveEtcdMemberForMachine removes the etcd member from the target cluster's etcd cluster.
// Removing the last remaining member of the cluster is not supported.
func (w *Workload) RemoveEtcdMemberForMachine(ctx context.Context, machine *clusterv1.Machine) error {
	if machine == nil || machine.Status.NodeRef == nil {
		// Nothing to do, no node for Machine
		return nil
	}
	return w.removeMemberForNode(ctx, machine.Status.NodeRef.Name)
}

func (w *Workload) removeMemberForNode(ctx context.Context, name string) error {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return err
	}
	if len(controlPlaneNodes.Items) < 2 {
		return ErrControlPlaneMinNodes
	}

	// Exclude node being removed from etcd client node list
	var remainingNodes []corev1.Node
	for _, n := range controlPlaneNodes.Items {
		if n.Name != name {
			remainingNodes = append(remainingNodes, n)
		}
	}
	etcdClient, err := w.etcdClientGenerator.forNodes(ctx, remainingNodes)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	// List etcd members. This checks that the member is healthy, because the request goes through consensus.
	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}
	member := etcdutil.MemberForName(members, name)

	// The member has already been removed, return immediately
	if member == nil {
		return nil
	}

	if err := etcdClient.RemoveMember(ctx, member.ID); err != nil {
		return errors.Wrap(err, "failed to remove member from etcd")
	}

	return nil
}

// ForwardEtcdLeadership forwards etcd leadership to the first follower
func (w *Workload) ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error {
	if machine == nil || machine.Status.NodeRef == nil {
		return nil
	}
	if leaderCandidate == nil {
		return errors.New("leader candidate cannot be nil")
	}
	if leaderCandidate.Status.NodeRef == nil {
		return errors.New("leader has no node reference")
	}

	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list control plane nodes")
	}

	etcdClient, err := w.etcdClientGenerator.forLeader(ctx, nodes.Items)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}

	currentMember := etcdutil.MemberForName(members, machine.Status.NodeRef.Name)
	if currentMember == nil || currentMember.ID != etcdClient.LeaderID {
		// nothing to do, this is not the etcd leader
		return nil
	}

	// Move the leader to the provided candidate.
	nextLeader := etcdutil.MemberForName(members, leaderCandidate.Status.NodeRef.Name)
	if nextLeader == nil {
		return errors.Errorf("failed to get etcd member from node %q", leaderCandidate.Status.NodeRef.Name)
	}
	if err := etcdClient.MoveLeader(ctx, nextLeader.ID); err != nil {
		return errors.Wrapf(err, "failed to move leader")
	}
	return nil
}
