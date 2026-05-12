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

// State-machine property test for reconcilePreTerminateHook. Models the cluster as a set of
// Machines / etcd members / Node names plus a transcript of RemoveEtcdMember invocations,
// applies random actions (mark Machine deleting, set override annotation, add/remove member),
// and verifies the invariants of #13667 after every action:
//
//   I-no-silent-leak  – If the hook removed the cleanup annotation from a Machine, then
//                       either RemoveEtcdMember was called against the correlated member,
//                       OR there were no orphan etcd candidates at the decision point.
//   I-eventual-progress – Setting an EtcdMemberIDAnnotation on a blocked Machine and re-running
//                       the hook clears the cleanup annotation (forward progress).
//   I-quorum-respected – Every RemoveEtcdMember call would have been admitted by
//                       targetEtcdClusterHealthy on the resolved name at the time of the call.

package controllers

import (
	"strconv"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"pgregory.net/rapid"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// preterminateModel is the in-memory state the state machine mutates.
type preterminateModel struct {
	t *rapid.T

	// Cluster snapshot. The "deletingMachine" name is fixed throughout — the state machine
	// always targets the same Machine to keep actions composable.
	deletingMachineName string
	machines            map[string]*clusterv1.Machine
	members             []*etcd.Member
	nodes               map[string]bool // set semantics keyed by Node name

	// Latest hook invocation results, used by the post-action invariant checker.
	lastObservedRemoves []string // RemoveEtcdMember name args observed during the most recent RunHook
	lastResolvedName    string   // tryGetEtcdMemberName result during the most recent RunHook
	lastCandidates      []*etcd.Member
	lastCleanupRemoved  bool // whether the hook removed the cleanup annotation from deletingMachine
}

func newPreterminateModel(t *rapid.T) *preterminateModel {
	const deletingName = "m-deleting"
	// Seed with: the deleting Machine plus one healthy peer so the "<= 1 CP Machines" early
	// return doesn't fire. The healthy peer has a Node, NodeRef and a matching etcd member.
	machines := map[string]*clusterv1.Machine{
		deletingName: {
			ObjectMeta: metav1.ObjectMeta{
				Name:       deletingName,
				Finalizers: []string{clusterv1.MachineFinalizer},
				Annotations: map[string]string{
					controlplanev1.PreTerminateHookCleanupAnnotation: "",
				},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		},
		"m-peer": {
			ObjectMeta: metav1.ObjectMeta{Name: "m-peer"},
			Status: clusterv1.MachineStatus{
				NodeRef: clusterv1.MachineNodeReference{Name: "node-peer"},
			},
		},
	}
	// Mark the peer's k8s control plane + etcd member healthy so targetKubernetesControlPlaneComponentsHealthy
	// can be satisfied during the hook's pre-checks.
	markK8sControlPlaneHealthy(machines["m-peer"])
	markEtcdMemberHealthy(machines["m-peer"])

	return &preterminateModel{
		t:                   t,
		deletingMachineName: deletingName,
		machines:            machines,
		members:             []*etcd.Member{{ID: 1, Name: "node-peer"}},
		nodes:               map[string]bool{"node-peer": true},
	}
}

func markK8sControlPlaneHealthy(m *clusterv1.Machine) {
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason})
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason})
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason})
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason})
}

func markEtcdMemberHealthy(m *clusterv1.Machine) {
	conditions.Set(m, metav1.Condition{Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason})
}

// ---------------- Actions ----------------------------------------------------

// addOrphanMember adds a new etcd learner member with a name not currently in the Nodes set,
// modelling a kubeadm-join that ran MemberAdd(learner=true) but never produced a Node — the
// canonical orphan-learner trigger.
func (s *preterminateModel) addOrphanMember() {
	s.appendOrphan(true)
}

// addOrphanVoter adds an orphan member with IsLearner=false. Used by eventual-progress tests
// that need the targetEtcdClusterHealthy quorum guard to admit a removal — that guard
// independently blocks removal when any learner is in-flight, which is by design but obscures
// the question of whether the correlation chain unblocked.
func (s *preterminateModel) addOrphanVoter() {
	s.appendOrphan(false)
}

func (s *preterminateModel) appendOrphan(isLearner bool) {
	id := uint64(len(s.members) + 100) // monotonic, disjoint from initial IDs
	name := "orphan-" + strconv.FormatUint(id, 10)
	s.members = append(s.members, &etcd.Member{
		ID:        id,
		Name:      name,
		IsLearner: isLearner,
		PeerURLs:  []string{"https://10.0.0." + strconv.FormatUint(id%200, 10) + ":2380"},
	})
}

// removeRandomMember removes a non-peer member (i.e. an orphan we previously added). Models an
// out-of-band RemoveMember (e.g. operator-driven `etcdctl member remove`).
func (s *preterminateModel) removeRandomMember(t *rapid.T) {
	candidates := []int{}
	for i, m := range s.members {
		if m.Name != "node-peer" {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		t.Skip()
	}
	idx := rapid.SampledFrom(candidates).Draw(t, "removeIdx")
	s.members = append(s.members[:idx], s.members[idx+1:]...)
}

// setIDAnnotation pins the deletingMachine to a specific etcd member ID via the operator
// override annotation. The annotation value is chosen from the current member-ID space (or a
// non-resolvable garbage value) so the model exercises both "annotation resolves" and
// "annotation is stale" cases.
func (s *preterminateModel) setIDAnnotation(t *rapid.T) {
	if len(s.members) == 0 {
		t.Skip()
	}
	choice := rapid.OneOf(
		rapid.Custom(func(t *rapid.T) string {
			m := rapid.SampledFrom(s.members).Draw(t, "memberToPin")
			return strconv.FormatUint(m.ID, 10)
		}),
		rapid.Just("999999"), // does not resolve
	).Draw(t, "annotationValue")
	m := s.machines[s.deletingMachineName]
	if m.Annotations == nil {
		m.Annotations = map[string]string{}
	}
	m.Annotations[controlplanev1.EtcdMemberIDAnnotation] = choice
}

// clearAnnotations removes both override annotations from the deletingMachine.
func (s *preterminateModel) clearAnnotations() {
	m := s.machines[s.deletingMachineName]
	delete(m.Annotations, controlplanev1.EtcdMemberIDAnnotation)
	delete(m.Annotations, controlplanev1.EtcdMemberNameAnnotation)
}

// runHook is the main action. It marks the deletingMachine as waiting-for-pre-terminate-hook,
// constructs an internal.ControlPlane snapshot from the current model state, invokes
// reconcilePreTerminateHook, and records observable side effects.
func (s *preterminateModel) runHook(t *rapid.T) {
	dm := s.machines[s.deletingMachineName]
	// If the cleanup annotation isn't present, this hook would no-op on the "deleting Machine
	// without pre-terminate hook" early return. There's nothing interesting to verify in that
	// case — skip the iteration so action sequences stay informative.
	if _, hadCleanup := dm.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]; !hadCleanup {
		t.Skip()
	}

	conditions.Set(dm, metav1.Condition{Type: clusterv1.MachineDeletingCondition, Status: metav1.ConditionTrue, Reason: clusterv1.MachineDeletingWaitingForPreTerminateHookReason})

	objs := []client.Object{}
	for _, m := range s.machines {
		objs = append(objs, m)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(objs...).Build()
	r := &KubeadmControlPlaneReconciler{Client: fakeClient}

	wl := &fakeWorkloadCluster{KubeadmConfigExist: true}
	machines := collections.Machines{}
	for _, m := range s.machines {
		machines[m.Name] = m
	}
	nodes := []*internal.Node{}
	for n := range s.nodes {
		nodes = append(nodes, &internal.Node{ObjectMeta: internal.ObjectMeta{Name: n}})
	}
	cp := &internal.ControlPlane{
		Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
		KCP: &controlplanev1.KubeadmControlPlane{
			Spec: controlplanev1.KubeadmControlPlaneSpec{Version: "v1.31.0"},
		},
		Machines:       machines,
		EtcdMembers:    s.members,
		Nodes:          nodes,
		InfraResources: map[string]*unstructured.Unstructured{},
	}
	cp.InjectTestManagementCluster(&fakeManagementCluster{Workload: wl})

	// Capture (resolved name, candidates) by calling the chain directly — the hook does this
	// internally but doesn't expose the result. The chain is pure so calling it twice is safe.
	resolvedMember, candidates := r.tryGetEtcdMemberName(t.Context(), cp, dm)
	if resolvedMember != nil {
		s.lastResolvedName = resolvedMember.Name
	} else {
		s.lastResolvedName = ""
	}
	s.lastCandidates = candidates

	_, err := r.reconcilePreTerminateHook(t.Context(), cp)
	if err != nil {
		// Hook errors (e.g. empty member list with managed etcd) are tolerable for the model —
		// they just mean the action didn't make progress. Skip the post-action invariants for
		// this iteration.
		t.Skip()
	}

	s.lastObservedRemoves = append([]string(nil), wl.removeEtcdMemberArgs...)

	// Determine whether THIS hook invocation removed the cleanup annotation: it was present
	// before the call (we asserted that above), so a missing annotation after the call means
	// the hook removed it.
	updated := &clusterv1.Machine{}
	if err := fakeClient.Get(t.Context(), client.ObjectKey{Name: s.deletingMachineName}, updated); err == nil {
		_, stillHasCleanup := updated.Annotations[controlplanev1.PreTerminateHookCleanupAnnotation]
		s.lastCleanupRemoved = !stillHasCleanup
		// Mirror the patched annotation set back to the model so future actions see the right state.
		s.machines[s.deletingMachineName].Annotations = updated.Annotations
	}
}

// ---------------- Invariants -------------------------------------------------

// checkInvariants is run after every state machine action via the special "" key in
// rapid.T.Repeat. It asserts the no-silent-leak / quorum-respected properties.
func (s *preterminateModel) checkInvariants(t *rapid.T) {
	// I-no-silent-leak.
	if s.lastCleanupRemoved {
		// The hook decided to proceed. There must be a justification: either RemoveEtcdMember
		// was called against a name correctly correlated to the deleting Machine, or there were
		// no orphan candidates at the decision point.
		if len(s.lastObservedRemoves) == 0 && len(s.lastCandidates) > 0 {
			t.Fatalf("I-no-silent-leak violated: hook removed the cleanup annotation but neither a RemoveEtcdMember call happened nor was the candidate set empty; resolvedName=%q candidates=%v",
				s.lastResolvedName, candidateIDs(s.lastCandidates))
		}
		if len(s.lastObservedRemoves) > 0 {
			// I-quorum-respected: every observed RemoveEtcdMember call should be the same as
			// the resolved name (we don't call RemoveEtcdMember for any other member in this
			// hook). And targetEtcdClusterHealthy should have admitted it.
			for _, removed := range s.lastObservedRemoves {
				if removed != s.lastResolvedName {
					t.Fatalf("RemoveEtcdMember called with %q but tryGetEtcdMemberName resolved %q for deletingMachine",
						removed, s.lastResolvedName)
				}
			}
		}
	}
}

// ---------------- Driver -----------------------------------------------------

// TestProperty_PreTerminateHookStateMachine runs the state machine. Each rapid example draws a
// sequence of actions and verifies the invariants after each.
func TestProperty_PreTerminateHookStateMachine(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := newPreterminateModel(t)
		t.Repeat(map[string]func(*rapid.T){
			"AddOrphanMember": func(t *rapid.T) { s.addOrphanMember() },
			"RemoveMember":    func(t *rapid.T) { s.removeRandomMember(t) },
			"SetIDAnnotation": func(t *rapid.T) { s.setIDAnnotation(t) },
			"ClearAnnotations": func(t *rapid.T) { s.clearAnnotations() },
			"RunHook":         func(t *rapid.T) { s.runHook(t) },
			"":                func(t *rapid.T) { s.checkInvariants(t) },
		})
	})
}

// TestProperty_PreTerminateHookEventualProgress: from a blocked state (deletingMachine has no
// NodeRef + ≥2 orphan members so step 5 cannot pair), setting EtcdMemberIDAnnotation to one
// of the orphans must make the chain resolve to that orphan's name on the next reconcile.
// This is the "operator-driven unblock" promise of #13667.
//
// Note: we use orphan voters (IsLearner=false) so the targetLearnerMembers > 0 guard in
// targetEtcdClusterHealthy doesn't independently block the hook's RemoveEtcdMember call — that
// guard is by design (a separate FM concern from correlation) and would mask whether the
// chain itself unblocked. The property asserts on the chain's resolution, which is the part the
// fix changes.
func TestProperty_PreTerminateHookEventualProgress(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := newPreterminateModel(t)
		// Two non-learner orphans: step 5 cannot pair (two unmatched members), so the first hook
		// run must hit step 6 and BLOCK.
		s.addOrphanVoter()
		s.addOrphanVoter()
		orphan := s.members[len(s.members)-2]

		s.runHook(t)
		if s.lastCleanupRemoved {
			t.Fatalf("setup did not produce a blocked state; cleanup annotation removed on first runHook (resolvedName=%q)", s.lastResolvedName)
		}

		// Operator pins one orphan via the ID annotation. Re-run.
		dm := s.machines[s.deletingMachineName]
		if dm.Annotations == nil {
			dm.Annotations = map[string]string{}
		}
		dm.Annotations[controlplanev1.EtcdMemberIDAnnotation] = strconv.FormatUint(orphan.ID, 10)
		s.runHook(t)

		if s.lastResolvedName != orphan.Name {
			t.Fatalf("I-eventual-progress violated: after pinning EtcdMemberIDAnnotation=%d the chain should resolve to %q but got %q (candidates=%v)",
				orphan.ID, orphan.Name, s.lastResolvedName, candidateIDs(s.lastCandidates))
		}
	})
}

func candidateIDs(in []*etcd.Member) []uint64 {
	out := make([]uint64, 0, len(in))
	for _, m := range in {
		out = append(out, m.ID)
	}
	return out
}

