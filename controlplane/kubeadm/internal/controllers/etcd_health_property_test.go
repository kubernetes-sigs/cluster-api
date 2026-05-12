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

// Property-based tests for the two `target*Healthy` predicates that decide whether KCP can
// safely transition the etcd / Kubernetes-CP target state. Both functions are pure over their
// `internal.ControlPlane` input and are gates for scale-down, scale-up, remediation, and the
// pre-terminate hook — so encoding their invariants as shrinkable properties is high-leverage.

package controllers

import (
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"pgregory.net/rapid"

	bootstrapv1 "sigs.k8s.io/cluster-api/api/bootstrap/kubeadm/v1beta2"
	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// ---------------- Helpers for these properties --------------------------------.

// genEtcdHealthCP generates a control plane geared for the etcd-health predicate: 1-5
// Machines (all with NodeRef), 0-5 etcd members where each member's IsLearner and per-Machine
// EtcdMemberHealthy condition are independently drawn. Names line up by index so the
// member↔Machine correlation is unambiguous.
func genEtcdHealthCP() *rapid.Generator[propControlPlane] {
	return rapid.Custom(func(t *rapid.T) propControlPlane {
		nMachines := rapid.IntRange(1, 5).Draw(t, "machineCount")
		machines := make([]*clusterv1.Machine, 0, nMachines)
		members := make([]*etcd.Member, 0, nMachines)
		for i := range nMachines {
			name := fmt.Sprintf("m-%d", i)
			nodeName := fmt.Sprintf("node-%d", i)
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Status: clusterv1.MachineStatus{
					NodeRef: clusterv1.MachineNodeReference{Name: nodeName},
				},
			}
			etcdHealthy := rapid.Bool().Draw(t, fmt.Sprintf("m%d-etcdHealthy", i))
			if etcdHealthy {
				conditions.Set(m, metav1.Condition{
					Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue,
					Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason,
				})
			} else {
				conditions.Set(m, metav1.Condition{
					Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionFalse,
					Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyReason,
				})
			}
			machines = append(machines, m)
			isLearner := rapid.Bool().Draw(t, fmt.Sprintf("m%d-isLearner", i))
			members = append(members, &etcd.Member{
				ID:        uint64(i + 1),
				Name:      nodeName,
				IsLearner: isLearner,
			})
		}
		return propControlPlane{machines: machines, etcdMembers: members}
	})
}

// runTargetEtcdHealthy invokes the predicate against the inputs. addEtcdMember=false is the
// common case (covers scale-down, remediation, pre-terminate hook); addEtcdMember=true covers
// scale-up. The target name is resolved against cp.etcdMembers by Name; if no member has
// that name, a synthetic *etcd.Member{Name: …} is constructed (this is how the production
// code passes name-only resolutions through to targetEtcdClusterHealthy).
func runTargetEtcdHealthy(t *rapid.T, cp propControlPlane, addEtcdMember bool, etcdMemberToBeDeleted string) bool {
	controlPlane := &internal.ControlPlane{
		Machines:    collections.FromMachines(cp.machines...),
		EtcdMembers: cp.etcdMembers,
	}
	var target *etcd.Member
	if etcdMemberToBeDeleted != "" {
		target = &etcd.Member{Name: etcdMemberToBeDeleted}
		for _, m := range cp.etcdMembers {
			if m.Name == etcdMemberToBeDeleted {
				target = m
				break
			}
		}
	}
	r := &KubeadmControlPlaneReconciler{}
	return r.targetEtcdClusterHealthy(t.Context(), controlPlane, addEtcdMember, target)
}

// expectedEtcdHealthy re-derives the predicate from the same inputs the function consumes, so
// the property test is checking the result against a parallel implementation rather than just
// asserting consistency with itself. Matches the `targetLearnerMembers > 0` guard, the
// "skip target", and the new-member optimism/pessimism rules.
func expectedEtcdHealthy(cp propControlPlane, addEtcdMember bool, etcdMemberToBeDeleted string) bool {
	targetTotal := 0
	targetVoters := 0
	targetLearners := 0
	unhealthy := 0

	if addEtcdMember {
		targetTotal = 1
		targetVoters = 1
		if len(cp.etcdMembers) > 1 {
			unhealthy++
		}
	}
	for _, m := range cp.etcdMembers {
		if m.Name == "" {
			targetTotal++
			targetLearners++
			unhealthy++
			continue
		}
		if etcdMemberToBeDeleted == m.Name {
			continue
		}
		targetTotal++
		if m.IsLearner {
			targetLearners++
		} else {
			targetVoters++
		}
		// Find the corresponding Machine via NodeRef.
		var machine *clusterv1.Machine
		for _, mach := range cp.machines {
			if mach.Status.NodeRef.IsDefined() && mach.Status.NodeRef.Name == m.Name {
				machine = mach
				break
			}
		}
		if machine == nil {
			// No alarms in our generator, so the function would treat this as healthy.
			continue
		}
		if !conditions.IsTrue(machine, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition) {
			unhealthy++
		}
	}
	targetQuorum := targetVoters/2 + 1
	ok := targetVoters-unhealthy >= targetQuorum
	if targetLearners > 0 {
		ok = false
	}
	return ok
}

// ---------------- targetEtcdClusterHealthy properties ----------------.

func TestProperty_TargetEtcdHealthy_QuorumFormulaConsistent(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genEtcdHealthCP().Draw(t, "cp")
		addEtcd := rapid.Bool().Draw(t, "addEtcd")
		var target string
		if !addEtcd && len(cp.etcdMembers) > 0 {
			target = rapid.SampledFrom(cp.etcdMembers).Draw(t, "target").Name
		}

		got := runTargetEtcdHealthy(t, cp, addEtcd, target)
		want := expectedEtcdHealthy(cp, addEtcd, target)
		if got != want {
			t.Logf("DEBUG members: %s", formatMemberCandidates(cp.etcdMembers))
			t.Fatalf("targetEtcdClusterHealthy disagreed with the recomputed quorum formula: got=%v want=%v addEtcdMember=%v target=%q",
				got, want, addEtcd, target)
		}
	})
}

func TestProperty_TargetEtcdHealthy_LearnerInFlightBlocksTransition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genEtcdHealthCP().Draw(t, "cp")
		// Force at least one non-target learner.
		if len(cp.etcdMembers) == 0 {
			t.Skip()
		}
		learnerIdx := rapid.IntRange(0, len(cp.etcdMembers)-1).Draw(t, "learnerIdx")
		cp.etcdMembers[learnerIdx].IsLearner = true
		// Target a DIFFERENT member (or empty) so the learner survives target removal.
		var target string
		if len(cp.etcdMembers) > 1 {
			other := (learnerIdx + 1) % len(cp.etcdMembers)
			target = cp.etcdMembers[other].Name
		}
		got := runTargetEtcdHealthy(t, cp, false, target)
		if got {
			t.Fatalf("expected false because a learner is in-flight (%q), got true; target=%q members=%s",
				cp.etcdMembers[learnerIdx].Name, target, formatMemberCandidates(cp.etcdMembers))
		}
	})
}

func TestProperty_TargetEtcdHealthy_RemovingLastVoterRefused(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Construct: 1 voter, 0 or more learners, target = the lone voter.
		nLearners := rapid.IntRange(0, 2).Draw(t, "nLearners")
		voterName := "node-voter"
		voter := &etcd.Member{ID: 1, Name: voterName, IsLearner: false}
		voterMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m-voter"},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: voterName}},
		}
		conditions.Set(voterMachine, metav1.Condition{
			Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason,
		})
		members := []*etcd.Member{voter}
		for i := range nLearners {
			members = append(members, &etcd.Member{
				ID:        uint64(i + 2),
				Name:      fmt.Sprintf("node-learner-%d", i),
				IsLearner: true,
			})
		}
		cp := propControlPlane{machines: []*clusterv1.Machine{voterMachine}, etcdMembers: members}
		got := runTargetEtcdHealthy(t, cp, false, voterName)
		if got {
			t.Fatalf("removing the only voter must return false, got true; members=%s", formatMemberCandidates(members))
		}
	})
}

func TestProperty_TargetEtcdHealthy_NewMemberPessimisticInLargerCluster(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genEtcdHealthCP().Draw(t, "cp")
		// Force >1 existing members and all members healthy + voter → otherwise the property's
		// expectation about pessimism vs optimism could be obscured by other failure paths.
		if len(cp.etcdMembers) <= 1 {
			t.Skip()
		}
		for i, m := range cp.etcdMembers {
			m.IsLearner = false
			conditions.Set(cp.machines[i], metav1.Condition{
				Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason,
			})
		}

		// In a cluster with N healthy voters and a pessimistic new member, the math is:
		// targetVoters = N + 1, unhealthy = 1, quorum = (N+1)/2 + 1. The result is true iff
		// (N+1) - 1 >= (N+1)/2 + 1, i.e. N >= (N+1)/2 + 1. For N=2: 2 >= 2 → true. For N=3:
		// 3 >= 3 → true. For N=4: 4 >= 3 → true. For N=5: 5 >= 4 → true.
		// So the property is: result is true for any N≥2.
		got := runTargetEtcdHealthy(t, cp, true, "")
		if !got {
			t.Fatalf("addEtcdMember=true with %d healthy voters should still permit transition (new member is pessimistic but quorum holds), got false", len(cp.etcdMembers))
		}
	})
}

func TestProperty_TargetEtcdHealthy_NewMemberOptimisticInTinyCluster(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 0 or 1 existing members: the new member is treated as healthy. Specifically test
		// "scale up from 0 to 1" — that path must succeed.
		nMembers := rapid.IntRange(0, 1).Draw(t, "nMembers")
		var members []*etcd.Member
		var machines []*clusterv1.Machine
		if nMembers == 1 {
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: "m-existing"},
				Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-existing"}},
			}
			conditions.Set(m, metav1.Condition{
				Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason,
			})
			machines = []*clusterv1.Machine{m}
			members = []*etcd.Member{{ID: 1, Name: "node-existing"}}
		}
		cp := propControlPlane{machines: machines, etcdMembers: members}
		got := runTargetEtcdHealthy(t, cp, true, "")
		if !got {
			t.Fatalf("addEtcdMember=true with %d existing members should succeed (new member counted optimistically), got false", nMembers)
		}
	})
}

func TestProperty_TargetEtcdHealthy_TargetMemberExcludedFromCounts(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genEtcdHealthCP().Draw(t, "cp")
		if len(cp.etcdMembers) == 0 {
			t.Skip()
		}
		target := cp.etcdMembers[0].Name

		// Calling with target=name should equal calling on a controlPlane that has the target
		// already excluded.
		stripped := propControlPlane{
			machines:    cp.machines[1:],
			etcdMembers: cp.etcdMembers[1:],
		}
		withTarget := runTargetEtcdHealthy(t, cp, false, target)
		withoutTarget := runTargetEtcdHealthy(t, stripped, false, "")
		// The two are equivalent up to a one-machine count difference; both compute target
		// totals over the residual set. Equality holds as long as the residual cluster has
		// the same per-member health and the dropped member was the target.
		if withTarget != withoutTarget {
			t.Logf("DEBUG members=%s", formatMemberCandidates(cp.etcdMembers))
			t.Fatalf("excluding target via removal vs via etcdMemberToBeDeleted disagreed: withTarget=%v withoutTarget=%v target=%q", withTarget, withoutTarget, target)
		}
	})
}

func TestProperty_TargetEtcdHealthy_OrderInvariance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genEtcdHealthCP().Draw(t, "cp")
		addEtcd := rapid.Bool().Draw(t, "addEtcd")
		var target string
		if !addEtcd && len(cp.etcdMembers) > 0 {
			target = rapid.SampledFrom(cp.etcdMembers).Draw(t, "target").Name
		}
		base := runTargetEtcdHealthy(t, cp, addEtcd, target)
		permuted := propControlPlane{
			machines:    reverseMachines(cp.machines),
			etcdMembers: reverseMembers(cp.etcdMembers),
		}
		perm := runTargetEtcdHealthy(t, permuted, addEtcd, target)
		if base != perm {
			t.Fatalf("result changed under permutation: base=%v perm=%v target=%q", base, perm, target)
		}
	})
}

func TestProperty_TargetEtcdHealthy_Determinism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genEtcdHealthCP().Draw(t, "cp")
		addEtcd := rapid.Bool().Draw(t, "addEtcd")
		var target string
		if !addEtcd && len(cp.etcdMembers) > 0 {
			target = rapid.SampledFrom(cp.etcdMembers).Draw(t, "target").Name
		}
		a := runTargetEtcdHealthy(t, cp, addEtcd, target)
		b := runTargetEtcdHealthy(t, cp, addEtcd, target)
		if a != b {
			t.Fatalf("non-deterministic: %v vs %v", a, b)
		}
	})
}

// ---------------- targetKubernetesControlPlaneComponentsHealthy properties ----------------.

// genCPHealthMachine produces a Machine with controllable health for the four CP-component
// conditions. allHealthy=true sets all four True; otherwise at least one is False.
func genCPHealthMachine(name string) *rapid.Generator[*clusterv1.Machine] {
	return rapid.Custom(func(t *rapid.T) *clusterv1.Machine {
		m := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: name}}
		apiHealthy := rapid.Bool().Draw(t, "api")
		cmHealthy := rapid.Bool().Draw(t, "cm")
		schedHealthy := rapid.Bool().Draw(t, "sched")
		etcdHealthy := rapid.Bool().Draw(t, "etcd")
		setBool := func(typ string, healthy bool) {
			status := metav1.ConditionTrue
			reason := controlplanev1.KubeadmControlPlaneMachinePodRunningReason
			if !healthy {
				status = metav1.ConditionFalse
				reason = "NotRunning"
			}
			conditions.Set(m, metav1.Condition{Type: typ, Status: status, Reason: reason})
		}
		setBool(controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition, apiHealthy)
		setBool(controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition, cmHealthy)
		setBool(controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition, schedHealthy)
		setBool(controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition, etcdHealthy)
		return m
	})
}

// genCPHealthControlPlane returns a control plane with N machines (1-5), each independently
// health-toggleable. Pretty-prints machine health for failure diagnostics via the Machine
// name suffix.
func genCPHealthControlPlane() *rapid.Generator[propControlPlane] {
	return rapid.Custom(func(t *rapid.T) propControlPlane {
		nMachines := rapid.IntRange(1, 5).Draw(t, "machineCount")
		machines := make([]*clusterv1.Machine, 0, nMachines)
		for i := range nMachines {
			machines = append(machines, genCPHealthMachine(fmt.Sprintf("m-%d", i)).Draw(t, "machine"))
		}
		return propControlPlane{machines: machines}
	})
}

// runTargetK8sCPHealthy wraps the predicate. We toggle EtcdManagedCluster via an external etcd
// configuration so the IsEtcdManaged() path picks up the right code path; that flag is set on
// controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.
func runTargetK8sCPHealthy(t *rapid.T, cp propControlPlane, addCP bool, target string, etcdManaged bool) bool {
	controlPlane := &internal.ControlPlane{
		Machines: collections.FromMachines(cp.machines...),
		KCP: &controlplanev1.KubeadmControlPlane{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "kcp"},
		},
	}
	if !etcdManaged {
		controlPlane.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External.Endpoints = []string{"https://external-etcd:2379"}
	}
	_ = bootstrapv1.ExternalEtcd{} // pin import even if etcdManaged=true skips the assignment
	r := &KubeadmControlPlaneReconciler{}
	return r.targetKubernetesControlPlaneComponentsHealthy(t.Context(), controlPlane, addCP, target)
}

// isFullyHealthyCP returns true iff a Machine has all four CP-component conditions True. When
// etcdManaged=false the EtcdPodHealthy condition is ignored, matching the function under test.
func isFullyHealthyCP(m *clusterv1.Machine, etcdManaged bool) bool {
	if !conditions.IsTrue(m, controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition) ||
		!conditions.IsTrue(m, controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition) ||
		!conditions.IsTrue(m, controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition) {
		return false
	}
	if etcdManaged && !conditions.IsTrue(m, controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition) {
		return false
	}
	return true
}

func TestProperty_TargetK8sCPHealthy_EmptyMachineList_IsFalse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		etcdManaged := rapid.Bool().Draw(t, "etcdManaged")
		cp := propControlPlane{} // no Machines
		got := runTargetK8sCPHealthy(t, cp, false, "", etcdManaged)
		if got {
			t.Fatalf("empty Machine list should return false (nothing to retain); got true")
		}
	})
}

func TestProperty_TargetK8sCPHealthy_RequiresOneFullyHealthyCP(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genCPHealthControlPlane().Draw(t, "cp")
		etcdManaged := rapid.Bool().Draw(t, "etcdManaged")
		target := ""
		got := runTargetK8sCPHealthy(t, cp, false, target, etcdManaged)
		// Recompute: result is true iff ≥1 non-target Machine is fully healthy.
		want := false
		for _, m := range cp.machines {
			if m.Name == target {
				continue
			}
			if isFullyHealthyCP(m, etcdManaged) {
				want = true
				break
			}
		}
		if got != want {
			names := make([]string, 0, len(cp.machines))
			for _, m := range cp.machines {
				names = append(names, m.Name)
			}
			t.Fatalf("predicate disagreed with recomputed health check: got=%v want=%v machines=%v etcdManaged=%v",
				got, want, strings.Join(names, ","), etcdManaged)
		}
	})
}

func TestProperty_TargetK8sCPHealthy_TargetMachineExcluded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genCPHealthControlPlane().Draw(t, "cp")
		if len(cp.machines) < 2 {
			t.Skip()
		}
		etcdManaged := rapid.Bool().Draw(t, "etcdManaged")
		target := cp.machines[0].Name
		withTarget := runTargetK8sCPHealthy(t, cp, false, target, etcdManaged)
		stripped := propControlPlane{machines: cp.machines[1:]}
		withoutTarget := runTargetK8sCPHealthy(t, stripped, false, "", etcdManaged)
		if withTarget != withoutTarget {
			t.Fatalf("excluding target via removal vs etcdMemberToBeDeleted disagreed: withTarget=%v withoutTarget=%v", withTarget, withoutTarget)
		}
	})
}

func TestProperty_TargetK8sCPHealthy_OrderInvariance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genCPHealthControlPlane().Draw(t, "cp")
		etcdManaged := rapid.Bool().Draw(t, "etcdManaged")
		base := runTargetK8sCPHealthy(t, cp, false, "", etcdManaged)
		permuted := propControlPlane{machines: reverseMachines(cp.machines)}
		perm := runTargetK8sCPHealthy(t, permuted, false, "", etcdManaged)
		if base != perm {
			t.Fatalf("result changed under permutation: base=%v perm=%v", base, perm)
		}
	})
}
