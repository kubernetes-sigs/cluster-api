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

// Property-based tests for compareMachinesAndMembers — the bidirectional Machine ↔ etcd
// member accuracy check that powers the KubeadmControlPlane EtcdClusterHealthy condition and
// the "Etcd member X does not have a corresponding Machine" diagnostic. The function carries
// three subtle invariants (provisioning-grace, two-minute new-Node grace, empty-name
// rendering) that this file locks in with shrinkable random inputs.

package internal

import (
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"pgregory.net/rapid"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// ---------------- Generators ---------------------------------------------------

// genCompareCP draws a random ControlPlane shaped for compareMachinesAndMembers:
//   - 0..5 Machines, each either has a NodeRef (matched name from the bounded pool) or doesn't.
//   - 0..5 etcd members, each with a Name drawn from the same pool (or empty).
//   - 0..5 workload-cluster Nodes (corev1.Node objects), each with a CreationTimestamp drawn
//     as either "old" (>2 minutes) or "young" (<30 seconds) so the 2-minute grace path is
//     exercised.
//
// The control plane is constructed directly (no fake-client) — compareMachinesAndMembers is
// pure over its `*ControlPlane` argument.
func genCompareCP() *rapid.Generator[*ControlPlane] {
	return rapid.Custom(func(t *rapid.T) *ControlPlane {
		nodePool := []string{"node-a", "node-b", "node-c", "node-d", "node-e"}
		nMachines := rapid.IntRange(0, 5).Draw(t, "nMachines")
		machines := []*clusterv1.Machine{}
		for i := 0; i < nMachines; i++ {
			m := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("m-%d", i)}}
			if rapid.Bool().Draw(t, fmt.Sprintf("m%d-hasNodeRef", i)) {
				m.Status.NodeRef.Name = rapid.SampledFrom(nodePool).Draw(t, fmt.Sprintf("m%d-nodeRef", i))
			}
			machines = append(machines, m)
		}
		nMembers := rapid.IntRange(0, 5).Draw(t, "nMembers")
		members := []*etcd.Member{}
		for i := 0; i < nMembers; i++ {
			name := rapid.OneOf(
				rapid.SampledFrom(nodePool),
				rapid.Just(""),
			).Draw(t, fmt.Sprintf("m%d-name", i))
			members = append(members, &etcd.Member{ID: uint64(i + 1), Name: name})
		}
		// Some Node entries (with controllable age) so the 2-minute grace path is reachable.
		nNodes := rapid.IntRange(0, 5).Draw(t, "nNodes")
		nodes := []*Node{}
		for i := 0; i < nNodes; i++ {
			young := rapid.Bool().Draw(t, fmt.Sprintf("n%d-young", i))
			var ts metav1.Time
			if young {
				ts = metav1.NewTime(time.Now().Add(-30 * time.Second))
			} else {
				ts = metav1.NewTime(time.Now().Add(-5 * time.Minute))
			}
			nodes = append(nodes, &Node{
				ObjectMeta: ObjectMeta{
					Name:              rapid.SampledFrom(nodePool).Draw(t, fmt.Sprintf("n%d-name", i)),
					CreationTimestamp: ts,
				},
			})
		}
		return &ControlPlane{
			Cluster:     &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
			KCP:         &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines:    collections.FromMachines(machines...),
			EtcdMembers: members,
			Nodes:       nodes,
		}
	})
}

// hasMachineWithNode returns true if at least one Machine carries a NodeRef.
func hasMachineWithNode(cp *ControlPlane) bool {
	for _, m := range cp.Machines {
		if m.Status.NodeRef.IsDefined() {
			return true
		}
	}
	return false
}

// ---------------- Properties ---------------------------------------------------

func TestProperty_CompareMachinesAndMembers_BijectionAccepted(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Construct a perfect bijection: N Machines, each with a unique NodeRef, and N members
		// with matching Names. Add an "old" Node entry per Machine so 2-minute-grace paths
		// don't fire.
		n := rapid.IntRange(1, 5).Draw(t, "n")
		machines := []*clusterv1.Machine{}
		members := []*etcd.Member{}
		nodes := []*Node{}
		for i := 0; i < n; i++ {
			nodeName := fmt.Sprintf("node-%d", i)
			machines = append(machines, &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("m-%d", i)},
				Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: nodeName}},
			})
			members = append(members, &etcd.Member{ID: uint64(i + 1), Name: nodeName})
			nodes = append(nodes, &Node{ObjectMeta: ObjectMeta{
				Name: nodeName, CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			}})
		}
		cp := &ControlPlane{
			Cluster:     &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
			KCP:         &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines:    collections.FromMachines(machines...),
			EtcdMembers: members,
			Nodes:       nodes,
		}
		matching, kcpErrors := compareMachinesAndMembers(cp)
		if !matching {
			t.Fatalf("bijection should produce matching=true; got false. errors=%v", kcpErrors)
		}
		if len(kcpErrors) != 0 {
			t.Fatalf("bijection should produce no kcp errors; got %v", kcpErrors)
		}
	})
}

func TestProperty_CompareMachinesAndMembers_MachineWithoutMember_SetsCondition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Construct a Machine with NodeRef but no matching member.
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m-no-member"},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-no-member"}},
		}
		// Add 0+ other unrelated members so the function still iterates.
		members := []*etcd.Member{{ID: 1, Name: "other-node"}}
		// Use a hopefully-old Node so the 2-minute grace doesn't fire.
		nodes := []*Node{{ObjectMeta: ObjectMeta{
			Name: "node-no-member", CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		}}}
		cp := &ControlPlane{
			Cluster:     &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
			KCP:         &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines:    collections.FromMachines(machine),
			EtcdMembers: members,
			Nodes:       nodes,
		}
		_, _ = compareMachinesAndMembers(cp)
		c := conditions.Get(machine, controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition)
		if c == nil || c.Status != metav1.ConditionFalse || c.Reason != controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyReason {
			t.Fatalf("Machine with NodeRef but no matching member should get EtcdMemberHealthy=False/NotHealthy; got %+v", c)
		}
	})
}

func TestProperty_CompareMachinesAndMembers_MemberWithoutMachine_AddsKCPError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		memberName := rapid.OneOf(rapid.Just("orphan-node"), rapid.Just("")).Draw(t, "memberName")
		// Machine set has one Machine with a different NodeRef, so the orphan member has no
		// matching Machine.
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m-known"},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-known"}},
		}
		orphan := &etcd.Member{ID: 999, Name: memberName}
		cp := &ControlPlane{
			Cluster:     &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
			KCP:         &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines:    collections.FromMachines(machine),
			EtcdMembers: []*etcd.Member{{ID: 1, Name: "node-known"}, orphan},
			Nodes:       []*Node{{ObjectMeta: ObjectMeta{Name: "node-known", CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute))}}},
		}
		_, kcpErrors := compareMachinesAndMembers(cp)
		// One kcpError must mention the orphan member. Empty-Name members render as
		// "<ID> (Name not yet assigned)"; non-empty render verbatim.
		wantSubstring := fmt.Sprintf("%d (Name not yet assigned)", orphan.ID)
		if memberName != "" {
			wantSubstring = memberName
		}
		found := false
		for _, e := range kcpErrors {
			if strings.Contains(e, wantSubstring) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected a kcpError mentioning %q; got %v", wantSubstring, kcpErrors)
		}
	})
}

func TestProperty_CompareMachinesAndMembers_ProvisioningSuppressesMismatch(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Setup: a Machine without a NodeRef (provisioning) AND an etcd member with no
		// matching Machine. Function must return matching=true (provisioning grace).
		provisioning := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m-provisioning"}}
		orphan := &etcd.Member{ID: 42, Name: "orphan-node"}
		cp := &ControlPlane{
			Cluster:     &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
			KCP:         &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines:    collections.FromMachines(provisioning),
			EtcdMembers: []*etcd.Member{orphan},
		}
		matching, _ := compareMachinesAndMembers(cp)
		if !matching {
			t.Fatalf("provisioning Machine + orphan member should yield matching=true (grace); got false")
		}
	})
}

func TestProperty_CompareMachinesAndMembers_TwoMinuteGraceForNewNodes(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Setup: a Machine with NodeRef pointing to a YOUNG (< 2 min) Node, and an etcd member
		// list that does NOT contain a member matching that NodeRef. The function should not
		// flip matching=false on the Machine-side loop (the 2-minute grace).
		//
		// We also add a second Machine without a NodeRef so the SYMMETRIC member-side loop
		// sees hasProvisioningMachine=true and doesn't flip matching=false either — the
		// property under test is specifically the Machine-side grace, not the member-side
		// path.
		young := metav1.NewTime(time.Now().Add(-30 * time.Second))
		newMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m-new"},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-new"}},
		}
		provisioning := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m-provisioning"}}
		cp := &ControlPlane{
			Cluster:     &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
			KCP:         &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines:    collections.FromMachines(newMachine, provisioning),
			EtcdMembers: []*etcd.Member{{ID: 1, Name: "other-existing"}},
			Nodes:       []*Node{{ObjectMeta: ObjectMeta{Name: "node-new", CreationTimestamp: young}}},
		}
		matching, _ := compareMachinesAndMembers(cp)
		if !matching {
			t.Fatalf("young Node (< 2 minutes) should suppress Machine-side matching=false; got false")
		}

		// Counterpart: with the SAME setup but an OLD Node, matching should now be false.
		old := metav1.NewTime(time.Now().Add(-5 * time.Minute))
		cp.Nodes[0].CreationTimestamp = old
		// Reset Machine conditions because compareMachinesAndMembers mutates them.
		newMachine.Status = clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-new"}}
		matchingOld, _ := compareMachinesAndMembers(cp)
		if matchingOld {
			t.Fatalf("old Node (> 2 minutes) should not suppress matching=false; got true")
		}
	})
}

func TestProperty_CompareMachinesAndMembers_NilMembers_NoNodeMachines_True(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genCompareCP().Draw(t, "cp")
		// Force the nil-members + no-Node-Machines precondition.
		cp.EtcdMembers = nil
		for _, m := range cp.Machines {
			m.Status.NodeRef = clusterv1.MachineNodeReference{}
		}
		if hasMachineWithNode(cp) {
			t.Fatalf("test setup error: still has Machine with Node")
		}
		matching, kcpErrors := compareMachinesAndMembers(cp)
		if !matching {
			t.Fatalf("nil members + no-Node-Machines should be matching=true; got false. errors=%v", kcpErrors)
		}
	})
}

func TestProperty_CompareMachinesAndMembers_NilMembers_WithNodeMachines_False(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Construct: ≥1 Machine with NodeRef and nil EtcdMembers.
		machine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m"},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-something"}},
		}
		cp := &ControlPlane{
			Cluster:     &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
			KCP:         &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines:    collections.FromMachines(machine),
			EtcdMembers: nil,
		}
		matching, _ := compareMachinesAndMembers(cp)
		if matching {
			t.Fatalf("nil members + Machine-with-Node should be matching=false; got true")
		}
	})
}

// silence unused-import linters when the file is built standalone.
var _ = corev1.Node{}
