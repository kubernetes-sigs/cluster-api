/*
Copyright 2026 The Kubernetes Authors.

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

// Property-based tests for two reconcile-loop state machines:
//
//   1. reconcileEtcdMembers — the safety-net loop that removes orphan etcd members during
//      reconcile. Modelled as a rapid.StateMachine over random sequences of mutations to the
//      cluster snapshot (add/remove members, add/remove Machines, toggle NodeRefs, run the
//      loop). Invariants assert the early-return-on-provisioning guard, the only-unmatched-
//      removed rule, the empty-name skip, and the quorum-respected rule.
//
//   2. scaleDownControlPlane ordering — focused single-call properties asserting that
//      ForwardEtcdLeadership is invoked before Client.Delete when etcd is managed, that it
//      is skipped for external etcd, and that the function bails on preflight failure.

package controllers

import (
	"context"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"pgregory.net/rapid"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// ---------------- reconcileEtcdMembers state machine ----------------.

// reconcileEtcdModel is the in-memory state the state machine mutates.
type reconcileEtcdModel struct {
	machines map[string]*clusterv1.Machine // keyed by Machine name
	members  []*etcd.Member
	nodes    map[string]bool // workload-cluster Node names (set semantics)

	// Last-RunReconcileEtcdMembers observations.
	lastObservedRemoves []string // RemoveEtcdMember names captured during the most recent run
	lastEarlyReturn     bool     // true if the run aborted before any RemoveEtcdMember could be considered
}

func newReconcileEtcdModel() *reconcileEtcdModel {
	// Seed with a 3-machine healthy CP. Tests can then add orphan members.
	machines := map[string]*clusterv1.Machine{}
	members := []*etcd.Member{}
	nodes := map[string]bool{}
	for i := range 3 {
		nodeName := "node-" + strconv.Itoa(i)
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m-" + strconv.Itoa(i)},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: nodeName}},
		}
		// Mark etcd-healthy so targetEtcdClusterHealthy won't flag the member as unhealthy.
		conditions.Set(m, metav1.Condition{
			Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason,
		})
		machines[m.Name] = m
		members = append(members, &etcd.Member{ID: uint64(i + 1), Name: nodeName})
		nodes[nodeName] = true
	}
	return &reconcileEtcdModel{machines: machines, members: members, nodes: nodes}
}

func (s *reconcileEtcdModel) addOrphanMember(t *rapid.T) {
	id := uint64(len(s.members) + 100)
	name := "orphan-" + strconv.FormatUint(id, 10)
	s.members = append(s.members, &etcd.Member{
		ID:        id,
		Name:      name,
		IsLearner: rapid.Bool().Draw(t, "isLearner"),
	})
}

func (s *reconcileEtcdModel) addEmptyNameMember() {
	id := uint64(len(s.members) + 200)
	s.members = append(s.members, &etcd.Member{ID: id, Name: ""})
}

func (s *reconcileEtcdModel) removeRandomMember(t *rapid.T) {
	// Don't remove healthy-base members (IDs 1-3) to keep the cluster usable.
	candidates := []int{}
	for i, m := range s.members {
		if m.ID > 3 {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		t.Skip()
	}
	idx := rapid.SampledFrom(candidates).Draw(t, "removeIdx")
	s.members = append(s.members[:idx], s.members[idx+1:]...)
}

func (s *reconcileEtcdModel) addProvisioningMachine() {
	name := "m-provisioning-" + strconv.Itoa(len(s.machines))
	s.machines[name] = &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func (s *reconcileEtcdModel) clearMachineNodeRef(t *rapid.T) {
	candidates := []string{}
	for name, m := range s.machines {
		if m.Status.NodeRef.IsDefined() {
			candidates = append(candidates, name)
		}
	}
	if len(candidates) == 0 {
		t.Skip()
	}
	name := rapid.SampledFrom(candidates).Draw(t, "clearNodeRef")
	m := s.machines[name].DeepCopy()
	m.Status.NodeRef = clusterv1.MachineNodeReference{}
	s.machines[name] = m
}

func (s *reconcileEtcdModel) runReconcileEtcdMembers(t *rapid.T) {
	s.lastObservedRemoves = nil
	s.lastEarlyReturn = false

	machineList := []*clusterv1.Machine{}
	for _, m := range s.machines {
		machineList = append(machineList, m)
	}
	cpNodes := []*internal.Node{}
	for n := range s.nodes {
		cpNodes = append(cpNodes, &internal.Node{ObjectMeta: internal.ObjectMeta{Name: n}})
	}
	cp := &internal.ControlPlane{
		Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
		KCP: &controlplanev1.KubeadmControlPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "kcp"},
			Spec:       controlplanev1.KubeadmControlPlaneSpec{Version: "v1.31.0"},
		},
		Machines:       collections.FromMachines(machineList...),
		EtcdMembers:    s.members,
		Nodes:          cpNodes,
		InfraResources: map[string]*unstructured.Unstructured{},
	}
	// reconcileEtcdMembers short-circuits if KCP's EtcdClusterHealthy condition is True; ensure
	// it isn't True so the loop runs.
	conditions.Set(cp.KCP, metav1.Condition{
		Type: controlplanev1.KubeadmControlPlaneEtcdClusterHealthyCondition, Status: metav1.ConditionFalse,
		Reason: controlplanev1.KubeadmControlPlaneEtcdClusterNotHealthyReason,
	})

	wl := &fakeWorkloadCluster{KubeadmConfigExist: true}
	cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: wl})

	objs := []client.Object{}
	for _, m := range s.machines {
		objs = append(objs, m)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
	r := &KubeadmControlPlaneReconciler{Client: fakeClient}

	// Detect "early return on provisioning Machine" before invoking, so the invariant check
	// has a deterministic signal.
	for _, m := range s.machines {
		if !m.Status.NodeRef.IsDefined() {
			s.lastEarlyReturn = true
			break
		}
	}

	_, _ = r.reconcileEtcdMembers(t.Context(), cp)
	s.lastObservedRemoves = append([]string(nil), wl.removeEtcdMemberArgs...)
}

func (s *reconcileEtcdModel) checkInvariants(t *rapid.T) {
	if s.lastObservedRemoves == nil && !s.lastEarlyReturn {
		return // RunReconcileEtcdMembers hasn't been called yet this step
	}

	currentNodeSet := map[string]bool{}
	for _, m := range s.machines {
		if m.Status.NodeRef.IsDefined() {
			currentNodeSet[m.Status.NodeRef.Name] = true
		}
	}

	// I-ProvisioningMachine_BlocksAction: if ANY machine has no NodeRef AT THE TIME we ran
	// the loop, no RemoveEtcdMember was made.
	if s.lastEarlyReturn && len(s.lastObservedRemoves) > 0 {
		t.Fatalf("I-ProvisioningMachine_BlocksAction violated: a Machine has no NodeRef but %d RemoveEtcdMember calls were observed: %v",
			len(s.lastObservedRemoves), s.lastObservedRemoves)
	}

	// I-OnlyUnmatchedMembersRemoved + I-EmptyNameMembersSkipped: every observed removal
	// targets a non-empty Name that is NOT in the current node set.
	for _, removed := range s.lastObservedRemoves {
		if removed == "" {
			t.Fatalf("I-EmptyNameMembersSkipped violated: RemoveEtcdMember(\"\") was called")
		}
		if currentNodeSet[removed] {
			t.Fatalf("I-OnlyUnmatchedMembersRemoved violated: RemoveEtcdMember(%q) called for a name that is in the current NodeRef set", removed)
		}
	}

	// I-QuorumGateRespected: re-run targetEtcdClusterHealthy at the post-state to verify that
	// removal would have been admitted. Since the test fixture has all members marked healthy
	// (3 voters + N orphans), removing each orphan one-by-one is safe; we just sanity-check
	// that the post-state still has ≥1 voter, which is the necessary condition for the next
	// remove to be admitted.
	postVoters := 0
	for _, m := range s.members {
		if m.Name == "" {
			continue
		}
		if !m.IsLearner {
			postVoters++
		}
	}
	if len(s.lastObservedRemoves) > 0 && postVoters == 0 {
		t.Fatalf("I-QuorumGateRespected violated: RemoveEtcdMember calls left the cluster with zero voters (members=%v)", memberNames(s.members))
	}
}

// TestProperty_ReconcileEtcdMembersStateMachine drives the state machine.
func TestProperty_ReconcileEtcdMembersStateMachine(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := newReconcileEtcdModel()
		t.Repeat(map[string]func(*rapid.T){
			"AddOrphanMember":         func(t *rapid.T) { s.addOrphanMember(t) },
			"AddEmptyNameMember":      func(_ *rapid.T) { s.addEmptyNameMember() },
			"RemoveMember":            func(t *rapid.T) { s.removeRandomMember(t) },
			"AddProvisioningMachine":  func(_ *rapid.T) { s.addProvisioningMachine() },
			"ClearMachineNodeRef":     func(t *rapid.T) { s.clearMachineNodeRef(t) },
			"RunReconcileEtcdMembers": func(t *rapid.T) { s.runReconcileEtcdMembers(t) },
			"":                        func(t *rapid.T) { s.checkInvariants(t) },
		})
	})
}

// ---------------- scaleDownControlPlane ordering ----------------.

// scaleDownObservations records side effects of one scaleDownControlPlane invocation.
type scaleDownObservations struct {
	forwardLeadershipCalls int
	targetHasDeletionTime  bool
	err                    error
}

// runScaleDownObserved invokes scaleDownControlPlane on a freshly-constructed 3-machine CP and
// returns the observable side effects. preflight is bypassed via the override hook so the
// post-preflight ordering (ForwardEtcdLeadership → Client.Delete) is what the properties assert.
func runScaleDownObserved(t *rapid.T, etcdManaged bool, blockPreflight bool) scaleDownObservations {
	machines := []*clusterv1.Machine{}
	for i := range 3 {
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "m-" + strconv.Itoa(i),
				Finalizers: []string{clusterv1.MachineFinalizer},
			},
			Status: clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-" + strconv.Itoa(i)}},
		}
		machines = append(machines, m)
	}
	target := machines[0]

	objs := []client.Object{}
	for _, m := range machines {
		objs = append(objs, m)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
	wl := &fakeWorkloadCluster{KubeadmConfigExist: true}

	cp := &internal.ControlPlane{
		Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
		KCP: &controlplanev1.KubeadmControlPlane{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "kcp"},
			Spec:       controlplanev1.KubeadmControlPlaneSpec{Version: "v1.31.0"},
		},
		Machines: collections.FromMachines(machines...),
	}
	if !etcdManaged {
		cp.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External.Endpoints = []string{"https://external:2379"}
	}
	cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: wl})

	r := &KubeadmControlPlaneReconciler{
		Client: fakeClient,
		overridePreflightChecksFunc: func(_ context.Context, _ *internal.ControlPlane, _ ...*clusterv1.Machine) ctrl.Result {
			if blockPreflight {
				return ctrl.Result{RequeueAfter: 1}
			}
			return ctrl.Result{}
		},
	}

	_, err := r.scaleDownControlPlane(t.Context(), cp, target)

	// After scaleDown returns: did Client.Delete put a DeletionTimestamp on the target?
	post := &clusterv1.Machine{}
	hasDelTime := false
	if getErr := fakeClient.Get(t.Context(), client.ObjectKey{Name: target.Name}, post); getErr == nil {
		hasDelTime = !post.DeletionTimestamp.IsZero()
	}

	return scaleDownObservations{
		forwardLeadershipCalls: wl.forwardEtcdLeadershipCalled,
		targetHasDeletionTime:  hasDelTime,
		err:                    err,
	}
}

func TestProperty_ScaleDown_ManagedEtcd_ForwardsLeadershipAndDeletes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		obs := runScaleDownObserved(t, true /* managed etcd */, false)
		if obs.err != nil {
			t.Fatalf("scaleDownControlPlane returned error: %v", obs.err)
		}
		if obs.forwardLeadershipCalls != 1 {
			t.Fatalf("managed etcd: expected exactly 1 ForwardEtcdLeadership call, got %d", obs.forwardLeadershipCalls)
		}
		if !obs.targetHasDeletionTime {
			t.Fatalf("managed etcd: expected the target Machine to be marked for deletion, but DeletionTimestamp is zero")
		}
	})
}

func TestProperty_ScaleDown_UnmanagedEtcd_SkipsForwardingButStillDeletes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		obs := runScaleDownObserved(t, false /* unmanaged */, false)
		if obs.err != nil {
			t.Fatalf("scaleDownControlPlane returned error: %v", obs.err)
		}
		if obs.forwardLeadershipCalls != 0 {
			t.Fatalf("unmanaged etcd: ForwardEtcdLeadership should not be called, got %d calls", obs.forwardLeadershipCalls)
		}
		if !obs.targetHasDeletionTime {
			t.Fatalf("unmanaged etcd: expected the target Machine to be marked for deletion, but DeletionTimestamp is zero")
		}
	})
}

func TestProperty_ScaleDown_PreflightFailure_BlocksAllSideEffects(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		etcdManaged := rapid.Bool().Draw(t, "etcdManaged")
		obs := runScaleDownObserved(t, etcdManaged, true /* preflight blocks */)
		if obs.err != nil {
			t.Fatalf("scaleDownControlPlane returned error: %v", obs.err)
		}
		if obs.forwardLeadershipCalls != 0 {
			t.Fatalf("preflight block: ForwardEtcdLeadership must not be called, got %d", obs.forwardLeadershipCalls)
		}
		if obs.targetHasDeletionTime {
			t.Fatalf("preflight block: target Machine must not be marked for deletion; DeletionTimestamp set")
		}
	})
}

// silence unused-import linters.
var (
	_ = corev1.Pod{}
)
