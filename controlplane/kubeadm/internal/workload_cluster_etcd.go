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

// EtcdRemovalExpectation pins admission-time assumptions about the etcd cluster state so
// RemoveEtcdMember / RemoveEtcdMemberByID can abort if the live state has diverged between
// admission and commit. The zero value disables the check, preserving the legacy behaviour
// for code paths that don't yet thread admission state through (e.g. the orphan-removal
// safety net in reconcileEtcdMembers).
//
// AdmissionVoterCount is the voter count the caller observed at admission; a drop at commit
// time means another removal landed concurrently and the caller's quorum gate may no longer
// hold. TargetWasLearner records whether the caller's gate considered the target a learner
// (learner removals are free w.r.t. quorum); if the target has since been promoted to voter,
// the removal would breach assumptions and is aborted.
//
// See risks 1 and 4 in #13680's Caveats.
type EtcdRemovalExpectation struct {
	AdmissionVoterCount int
	TargetWasLearner    bool
}

// RemoveEtcdMember removes the etcd member from the target cluster's etcd cluster.
// Removing the last remaining member of the cluster is not supported.
// Note: It is a responsibility of the caller to check if this operation doesn't lead to quorum loss.
func (w *Workload) RemoveEtcdMember(ctx context.Context, name string, nodes []*Node, expect EtcdRemovalExpectation) error {
	// Exclude node being removed from etcd client node list
	// Note: this operation relies on the assumption that node name is equal to the name of the corresponding etcd member.
	var remainingNodes []string
	for _, n := range nodes {
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

	if err := checkRemovalExpectation(members, member, expect); err != nil {
		return err
	}

	if err := etcdClient.RemoveMember(ctx, member.ID); err != nil {
		return errors.Wrap(err, "failed to remove member from etcd")
	}

	return nil
}

// RemoveEtcdMemberByID removes an etcd member identified by its uint64 member ID. This is the
// counterpart to RemoveEtcdMember(name) for the case where the etcd member's Name field is
// empty in the leader's MemberList — which is standard etcd v3 behaviour between
// `MemberAdd(peerURLs)` returning and the new peer joining raft and publishing its name. The
// pre-terminate hook needs to remove such orphan members before CAPD's docker rm -f drops the
// container, so an ID-based path is required.
//
// Unlike RemoveEtcdMember(name) we do not exclude any node from the etcd client list — when the
// member's Name is empty we cannot identify a corresponding Node anyway, and the typical orphan-learner
// scenario (Node never registered) means there is no Node to exclude. The caller is responsible
// for ensuring the operation does not lead to quorum loss.
func (w *Workload) RemoveEtcdMemberByID(ctx context.Context, id uint64, nodes []*Node, expect EtcdRemovalExpectation) error {
	nodeNames := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}
	etcdClient, err := w.etcdClientGenerator.forFirstAvailableNode(ctx, nodeNames)
	if err != nil {
		return errors.Wrap(err, "failed to create etcd client")
	}
	defer etcdClient.Close()

	members, err := etcdClient.Members(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to list etcd members using etcd client")
	}
	var target *etcd.Member
	for _, m := range members {
		if m.ID == id {
			target = m
			break
		}
	}
	if target == nil {
		// The member is already gone — return successfully (idempotent).
		return nil
	}
	if len(members) == 1 {
		return errors.New("cannot remove the last etcd member in the cluster")
	}

	if err := checkRemovalExpectation(members, target, expect); err != nil {
		return err
	}

	if err := etcdClient.RemoveMember(ctx, id); err != nil {
		return errors.Wrapf(err, "failed to remove member %x from etcd", id)
	}
	return nil
}

// checkRemovalExpectation enforces the EtcdRemovalExpectation against the fresh MemberList
// fetched immediately before the RemoveMember RPC. The check is opt-in (zero expectation =
// no check) so callers like reconcileEtcdMembers' orphan-removal safety net — which removes
// members without going through the admission gate — keep their existing semantics.
//
// Two conditions trigger an abort, each mapping to a specific concurrent-state race:
//
//  1. Live voter count < AdmissionVoterCount: another remediation landed between admission
//     and commit (concurrent removal by another reconcile or by an external etcdctl user).
//     The caller's quorum gate was evaluated against a now-stale voter count; the in-flight
//     removal must be re-validated by the caller in the next reconcile.
//
//  2. TargetWasLearner but the live target is now a voter: the learner was promoted to voter
//     between admission and commit. Learner removals are quorum-free; voter removals are
//     not. The caller's gate didn't evaluate the quorum impact and must do so on the next
//     reconcile with the updated state.
//
// See risks 1 and 4 in #13680's Caveats.
func checkRemovalExpectation(members []*etcd.Member, target *etcd.Member, expect EtcdRemovalExpectation) error {
	if expect == (EtcdRemovalExpectation{}) {
		return nil
	}
	if expect.AdmissionVoterCount > 0 {
		liveVoters := 0
		for _, m := range members {
			if !m.IsLearner {
				liveVoters++
			}
		}
		if liveVoters < expect.AdmissionVoterCount {
			return errors.Errorf("aborting removal of member %s (id=%x): live voter count %d is below admission-time count %d; another removal landed concurrently, re-evaluate the quorum gate on the next reconcile",
				target.Name, target.ID, liveVoters, expect.AdmissionVoterCount)
		}
	}
	if expect.TargetWasLearner && !target.IsLearner {
		return errors.Errorf("aborting removal of member %s (id=%x): target was a learner at admission but is now a voter; the admission gate did not evaluate quorum impact for a voter removal, re-evaluate on the next reconcile",
			target.Name, target.ID)
	}
	return nil
}

// ForwardEtcdLeadership forwards etcd leadership to the first follower.
func (w *Workload) ForwardEtcdLeadership(ctx context.Context, machine *clusterv1.Machine, leaderCandidate *clusterv1.Machine, nodes []*Node) error {
	if machine == nil || !machine.Status.NodeRef.IsDefined() {
		return nil
	}
	if leaderCandidate == nil {
		return errors.New("leader candidate cannot be nil")
	}
	if !leaderCandidate.Status.NodeRef.IsDefined() {
		return errors.New("leader candidate has no node reference")
	}

	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
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
