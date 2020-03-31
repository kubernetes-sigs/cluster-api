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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdutil "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/util"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type etcdClientFor interface {
	forNode(ctx context.Context, name string) (*etcd.Client, error)
}

// EtcdIsHealthy runs checks for every etcd member in the cluster to satisfy our definition of healthy.
// This is a best effort check and nodes can become unhealthy after the check is complete. It is not a guarantee.
// It's used a signal for if we should allow a target cluster to scale up, scale down or upgrade.
// It returns a map of nodes checked along with an error for a given node.
func (w *Workload) EtcdIsHealthy(ctx context.Context) (HealthCheckResult, error) {
	var knownClusterID uint64
	var knownMemberIDSet etcdutil.UInt64Set

	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return nil, err
	}

	expectedMembers := 0
	response := make(map[string]error)
	for _, node := range controlPlaneNodes.Items {
		name := node.Name
		response[name] = nil
		if node.Spec.ProviderID == "" {
			response[name] = errors.New("empty provider ID")
			continue
		}

		// Check to see if the pod is ready
		etcdPodKey := ctrlclient.ObjectKey{
			Namespace: metav1.NamespaceSystem,
			Name:      staticPodName("etcd", name),
		}
		pod := corev1.Pod{}
		if err := w.Client.Get(ctx, etcdPodKey, &pod); err != nil {
			response[name] = errors.Wrap(err, "failed to get etcd pod")
			continue
		}
		if err := checkStaticPodReadyCondition(pod); err != nil {
			// Nothing wrong here, etcd on this node is just not running.
			// If it's a true failure the healthcheck will fail since it won't have checked enough members.
			continue
		}
		// Only expect a member reports healthy if its pod is ready.
		// This fixes the known state where the control plane has a crash-looping etcd pod that is not part of the
		// etcd cluster.
		expectedMembers++

		// Create the etcd Client for the etcd Pod scheduled on the Node
		etcdClient, err := w.etcdClientGenerator.forNode(ctx, name)
		if err != nil {
			response[name] = errors.Wrap(err, "failed to create etcd client")
			continue
		}

		// List etcd members. This checks that the member is healthy, because the request goes through consensus.
		members, err := etcdClient.Members(ctx)
		if err != nil {
			response[name] = errors.Wrap(err, "failed to list etcd members using etcd client")
			continue
		}
		member := etcdutil.MemberForName(members, name)

		// Check that the member reports no alarms.
		if len(member.Alarms) > 0 {
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
				response[name] = errors.Errorf("etcd member reports members IDs %v, but all previously seen etcd members reported member IDs %v", memberIDSet.UnsortedList(), knownMemberIDSet.UnsortedList())
			}
			continue
		}
	}

	// TODO: ensure that each pod is owned by a node that we're managing. That would ensure there are no out-of-band etcd members

	// Check that there is exactly one etcd member for every healthy pod.
	// This allows us to handle the expected case where there is a failing pod but it's been removed from the member list.
	if expectedMembers != len(knownMemberIDSet) {
		return response, errors.Errorf("there are %d healthy etcd pods, but %d etcd members", expectedMembers, len(knownMemberIDSet))
	}

	return response, nil
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

	// Pick a different node to talk to etcd
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return err
	}
	if len(controlPlaneNodes.Items) < 2 {
		return ErrControlPlaneMinNodes
	}
	anotherNode := firstNodeNotMatchingName(machine.Status.NodeRef.Name, controlPlaneNodes.Items)
	if anotherNode == nil {
		return errors.Errorf("failed to find a control plane node whose name is not %s", machine.Status.NodeRef.Name)
	}
	etcdClient, err := w.etcdClientGenerator.forNode(ctx, anotherNode.Name)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}

	// List etcd members. This checks that the member is healthy, because the request goes through consensus.
	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}
	member := etcdutil.MemberForName(members, machine.Status.NodeRef.Name)

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
		// Nothing to do, no node for Machine
		return nil
	}

	// TODO we'd probably prefer to pass in all the known nodes and let grpc handle retrying connections across them
	clientMachineName := machine.Status.NodeRef.Name
	if leaderCandidate != nil && leaderCandidate.Status.NodeRef != nil {
		// connect to the new leader candidate, in case machine's etcd membership has already been removed
		clientMachineName = leaderCandidate.Status.NodeRef.Name
	}

	etcdClient, err := w.etcdClientGenerator.forNode(ctx, clientMachineName)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd Client")
	}

	// List etcd members. This checks that the member is healthy, because the request goes through consensus.
	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}

	currentMember := etcdutil.MemberForName(members, machine.Status.NodeRef.Name)
	if currentMember == nil || currentMember.ID != etcdClient.LeaderID {
		return nil
	}

	// Move the etcd client to the current leader, which in this case is the machine we're about to delete.
	etcdClient, err = w.etcdClientGenerator.forNode(ctx, machine.Status.NodeRef.Name)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd Client")
	}

	// If we don't have a leader candidate, move the leader to the next available machine.
	if leaderCandidate == nil || leaderCandidate.Status.NodeRef == nil {
		for _, member := range members {
			if member.ID != currentMember.ID {
				if err := etcdClient.MoveLeader(ctx, member.ID); err != nil {
					return errors.Wrapf(err, "failed to move leader")
				}
				break
			}
		}
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
