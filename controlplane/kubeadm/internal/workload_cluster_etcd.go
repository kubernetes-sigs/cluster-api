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

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	etcdutil "sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd/util"
)

type etcdClientFor interface {
	forFirstAvailableNode(ctx context.Context, nodeNames []string) (*etcd.Client, error)
	forLeader(ctx context.Context, nodeNames []string) (*etcd.Client, error)
}

// UpdateEtcdLocalInKubeadmConfigMap sets etcd local configuration in the kubeadm config map.
func (w *Workload) UpdateEtcdLocalInKubeadmConfigMap(etcdLocal bootstrapv1.LocalEtcd) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		// Note: Etcd local and external are mutually exclusive and they cannot be switched, once set.
		c.Etcd.Local = etcdLocal
		c.Etcd.External = bootstrapv1.ExternalEtcd{}
	}
}

// UpdateEtcdExternalInKubeadmConfigMap sets etcd external configuration in the kubeadm config map.
func (w *Workload) UpdateEtcdExternalInKubeadmConfigMap(etcdExternal bootstrapv1.ExternalEtcd) func(*bootstrapv1.ClusterConfiguration) {
	return func(c *bootstrapv1.ClusterConfiguration) {
		// Note: Etcd local and external are mutually exclusive and they cannot be switched, once set.
		c.Etcd.Local = bootstrapv1.LocalEtcd{}
		c.Etcd.External = etcdExternal
	}
}

// RemoveEtcdMember removes the etcd member from the target cluster's etcd cluster.
// Removing the last remaining member of the cluster is not supported.
// Note: It is a responsibility of the caller to check if this operation doesn't lead to quorum loss.
func (w *Workload) RemoveEtcdMember(ctx context.Context, name string) error {
	controlPlaneNodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return err
	}

	// Exclude node being removed from etcd client node list
	var remainingNodes []string
	for _, n := range controlPlaneNodes.Items {
		if n.Name != name {
			remainingNodes = append(remainingNodes, n.Name)
		}
	}
	etcdClient, err := w.etcdClientGenerator.forFirstAvailableNode(ctx, remainingNodes)
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

	// This is a safeguard preventing from removing the last member in an etcd-cluster.
	if len(members) == 1 {
		return errors.New("cannot remove the last etcd member in the cluster")
	}

	if err := etcdClient.RemoveMember(ctx, member.ID); err != nil {
		return errors.Wrap(err, "failed to remove member from etcd")
	}

	return nil
}

// ForwardEtcdLeadership forwards etcd leadership to the first follower.
func (w *Workload) ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error {
	if machine == nil || !machine.Status.NodeRef.IsDefined() {
		return nil
	}
	if leaderCandidate == nil {
		return errors.New("leader candidate cannot be nil")
	}
	if !leaderCandidate.Status.NodeRef.IsDefined() {
		return errors.New("leader candidate has no node reference")
	}

	nodes, err := w.getControlPlaneNodes(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list control plane nodes")
	}
	nodeNames := make([]string, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeNames = append(nodeNames, node.Name)
	}
	etcdClient, err := w.etcdClientGenerator.forLeader(ctx, nodeNames)
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

// EtcdMemberStatus contains status information for a single etcd member.
type EtcdMemberStatus struct {
	Name       string
	Responsive bool
}
