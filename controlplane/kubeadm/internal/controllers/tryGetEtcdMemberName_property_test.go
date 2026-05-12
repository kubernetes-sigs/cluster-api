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

// Property-based tests for the monotonic fallback chain in tryGetEtcdMemberName, covering
// the ten properties (P1–P10) documented at kubernetes-sigs/cluster-api#13667. Uses
// pgregory.net/rapid (https://github.com/flyingmutant/rapid) for generators + shrinking; run
// with `go test -run Property -rapid.checks=<N>` to crank the example count for nightly CI.

package controllers

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"pgregory.net/rapid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
)

// Shared generators (propControlPlane, genMember, genMachine, etc.) live in
// property_generators_test.go.

// nameOf returns the Name field of a non-nil *etcd.Member, or "" if member is nil. Used by
// properties that assert on Name semantics.
func nameOf(m *etcd.Member) string {
	if m == nil {
		return ""
	}
	return m.Name
}

// runChain invokes tryGetEtcdMemberName against a propControlPlane. The remote-client mock
// always returns no Nodes — step 3 (ProviderID → Node lookup) is exercised in the table-driven
// test; the properties focus on steps 1, 2, 4, 5, 6 which are pure functions of the input.
func runChain(t *rapid.T, cp propControlPlane) (*etcd.Member, []*etcd.Member) {
	testCluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: metav1.NamespaceDefault, Name: "test-cluster"}}
	controlPlane := &internal.ControlPlane{
		Cluster:        testCluster,
		Machines:       collections.FromMachines(cp.machines...),
		EtcdMembers:    cp.etcdMembers,
		Nodes:          cp.nodes,
		InfraResources: map[string]*unstructured.Unstructured{},
	}
	remoteClient := fake.NewClientBuilder().
		WithIndex(&corev1.Node{}, index.MachineProviderIDField, index.NodeByProviderID).
		Build()
	r := &KubeadmControlPlaneReconciler{
		ClusterCache: clustercache.NewFakeClusterCache(remoteClient, client.ObjectKeyFromObject(testCluster)),
	}
	return r.tryGetEtcdMemberName(t.Context(), controlPlane, cp.machines[0])
}

// ---------------- Property tests ---------------------------------------------.

// P3 Determinism: tryGetEtcdMemberName is a pure function — repeated calls on the same input
// return equal results.
func TestProperty_Determinism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genControlPlane().Draw(t, "cp")
		m1, c1 := runChain(t, cp)
		n1 := nameOf(m1)
		m2, c2 := runChain(t, cp)
		n2 := nameOf(m2)
		_ = m1
		_ = m2
		if n1 != n2 {
			t.Fatalf("name not deterministic: %q vs %q", n1, n2)
		}
		if !reflect.DeepEqual(memberIDSet(c1), memberIDSet(c2)) {
			t.Fatalf("candidates not deterministic: %v vs %v", memberIDs(c1), memberIDs(c2))
		}
	})
}

// P9 peerURLHosts total: any string input returns a (possibly-empty) host slice and never
// panics. Includes garbage, unicode, IPv6 with/without brackets, ports without schemes.
func TestProperty_PeerURLHostsIsTotal(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.String().Draw(t, "rawPeerURL")
		hosts := peerURLHosts(&etcd.Member{PeerURLs: []string{raw}})
		for _, h := range hosts {
			if h == "" {
				t.Fatalf("peerURLHosts returned an empty host entry for %q", raw)
			}
		}
	})
}

// P4 No spurious name: if tryGetEtcdMemberName returns a non-empty name, that name must
// either be the Name of some member in cp.EtcdMembers, the value of the EtcdMemberNameAnnotation,
// or the deletingMachine's NodeRef.Name. Catches transcription bugs in the ID-lookup → Name path.
func TestProperty_NoSpuriousName(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genControlPlane().Draw(t, "cp")
		member, _ := runChain(t, cp)
		name := nameOf(member)
		if name == "" {
			return
		}
		annName := cp.machines[0].GetAnnotations()[controlplanev1.EtcdMemberNameAnnotation]
		if name == annName {
			return
		}
		if cp.machines[0].Status.NodeRef.Name != "" && name == cp.machines[0].Status.NodeRef.Name {
			return
		}
		for _, m := range cp.etcdMembers {
			if m.Name == name {
				return
			}
		}
		t.Fatalf("name=%q is not in member list %v and is not the override-name annotation %q or NodeRef", name, memberNames(cp.etcdMembers), annName)
	})
}

// P5 Candidates set exact: when name == "", the returned candidates are exactly the etcd
// members with non-empty Name that don't match any Node name. This is the operator-facing
// orphan set; the property locks its definition in.
//
// We log the input on failure so the shrunk counter-example is fully decodable in CI artifacts.
func TestProperty_CandidatesSetExact(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genControlPlane().Draw(t, "cp")
		member, candidates := runChain(t, cp)
		// The property is "when no member is resolved, candidates is the exact orphan set".
		// The signal for "no resolution" is member==nil — name=="" alone is no longer enough
		// because the chain now resolves empty-Name members too.
		if member != nil {
			return
		}
		nodeSet := map[string]bool{}
		for _, n := range cp.nodes {
			nodeSet[n.Name] = true
		}
		expectedIDs := map[uint64]bool{}
		for _, m := range cp.etcdMembers {
			// Empty-Name members ARE included in the candidate set — etcd v3 reports Name=""
			// from MemberAdd until the new peer publishes its name, so they need to flow
			// through the peer-URL/strict-pairing path the same way named orphans do.
			if m.Name != "" && nodeSet[m.Name] {
				continue
			}
			expectedIDs[m.ID] = true
		}
		got := memberIDSet(candidates)
		if !reflect.DeepEqual(got, expectedIDs) {
			t.Logf("DEBUG deletingMachine: name=%q nodeRef=%q providerID=%q addresses=%+v annotations=%+v",
				cp.machines[0].Name, cp.machines[0].Status.NodeRef.Name, cp.machines[0].Spec.ProviderID,
				cp.machines[0].Status.Addresses, cp.machines[0].Annotations)
			for i, m := range cp.etcdMembers {
				t.Logf("DEBUG member[%d]: ID=%d name=%q peerURLs=%v", i, m.ID, m.Name, m.PeerURLs)
			}
			for i, n := range cp.nodes {
				t.Logf("DEBUG cpNode[%d]: name=%q", i, n.Name)
			}
			t.Fatalf("orphan candidate set mismatch: got=%v want=%v", got, expectedIDs)
		}
	})
}

// P8 Order invariance: permuting Machines and EtcdMembers does not change the result. Catches
// accidental dependence on slice order in the fallback chain.
func TestProperty_OrderInvariance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genControlPlane().Draw(t, "cp")
		baseMember, baseCandidates := runChain(t, cp)
		baseName := nameOf(baseMember)

		permMachines := append([]*clusterv1.Machine{cp.machines[0]}, reverseMachines(cp.machines[1:])...)
		permMembers := reverseMembers(cp.etcdMembers)
		permuted := propControlPlane{machines: permMachines, etcdMembers: permMembers, nodes: cp.nodes}
		permMember, permCandidates := runChain(t, permuted)
		permName := nameOf(permMember)

		if permName != baseName {
			t.Logf("DEBUG base machines[0]: name=%q nodeRef=%q providerID=%q addrs=%+v annotations=%+v",
				cp.machines[0].Name, cp.machines[0].Status.NodeRef.Name, cp.machines[0].Spec.ProviderID,
				cp.machines[0].Status.Addresses, cp.machines[0].Annotations)
			for i, m := range cp.machines {
				t.Logf("DEBUG base machine[%d]: name=%q nodeRef=%q addrs=%+v annotations=%+v", i, m.Name, m.Status.NodeRef.Name, m.Status.Addresses, m.Annotations)
			}
			for i, m := range cp.etcdMembers {
				t.Logf("DEBUG base member[%d]: ID=%d name=%q peerURLs=%v isLearner=%v", i, m.ID, m.Name, m.PeerURLs, m.IsLearner)
			}
			for i, n := range cp.nodes {
				t.Logf("DEBUG cpNode[%d]: name=%q", i, n.Name)
			}
			t.Fatalf("name changed under permutation: base=%q perm=%q", baseName, permName)
		}
		if !equalUint64Slices(sortedIDs(memberIDs(baseCandidates)), sortedIDs(memberIDs(permCandidates))) {
			t.Fatalf("candidate set changed under permutation: base=%v perm=%v",
				sortedIDs(memberIDs(baseCandidates)), sortedIDs(memberIDs(permCandidates)))
		}
	})
}

// P10 Address-type-agnostic: changing every address Type on the deletingMachine (e.g. all to
// ExternalIP, all to Hostname) must not change the returned name.
func TestProperty_AddressTypeAgnostic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genControlPlane().Draw(t, "cp")
		baseMemberAT, _ := runChain(t, cp)
		baseName := nameOf(baseMemberAT)

		types := []clusterv1.MachineAddressType{
			clusterv1.MachineInternalIP, clusterv1.MachineExternalIP,
			clusterv1.MachineHostName, clusterv1.MachineInternalDNS, clusterv1.MachineExternalDNS,
		}
		newType := rapid.SampledFrom(types).Draw(t, "newAddrType")

		mutated := cp.machines[0].DeepCopy()
		for i := range mutated.Status.Addresses {
			mutated.Status.Addresses[i].Type = newType
		}
		mutatedMachines := append([]*clusterv1.Machine{mutated}, cp.machines[1:]...)
		mutatedCp := propControlPlane{machines: mutatedMachines, etcdMembers: cp.etcdMembers, nodes: cp.nodes}
		mutMemberAT, _ := runChain(t, mutatedCp)
		mutName := nameOf(mutMemberAT)

		if mutName != baseName {
			t.Fatalf("name changed after rewriting all address types to %q: base=%q mut=%q", newType, baseName, mutName)
		}
	})
}

// P2 ID beats Name: when both EtcdMemberIDAnnotation and EtcdMemberNameAnnotation are set
// and resolve to *different* members, the ID's member name wins.
func TestProperty_IDBeatsName(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		idTarget := rapid.SampledFrom(propNodePool[:4]).Draw(t, "idTarget")
		nameTarget := rapid.SampledFrom(propNodePool[4:]).Draw(t, "nameTarget")
		idMemberID := rapid.Uint64Range(1, 100).Draw(t, "idMemberID")
		nameMemberID := rapid.Uint64Range(101, 200).Draw(t, "nameMemberID")

		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name: "m-deleting",
				Annotations: map[string]string{
					controlplanev1.EtcdMemberIDAnnotation:   strconv.FormatUint(idMemberID, 10),
					controlplanev1.EtcdMemberNameAnnotation: nameTarget,
				},
			},
		}
		cp := propControlPlane{
			machines: []*clusterv1.Machine{m},
			etcdMembers: []*etcd.Member{
				{ID: idMemberID, Name: idTarget},
				{ID: nameMemberID, Name: nameTarget},
			},
		}
		member, _ := runChain(t, cp)
		name := nameOf(member)
		if name != idTarget {
			t.Fatalf("ID annotation should beat Name annotation; id=%d→%q nameAnn=%q got=%q", idMemberID, idTarget, nameTarget, name)
		}
	})
}

// P1 First-match-wins: if step 1 (annotation) resolves, mutating later inputs doesn't change
// the result. Pin an ID annotation, then add random NodeRef / addresses / other machines /
// extra members; the resolved name must stay equal.
//
// The pinned ID is chosen from a range disjoint from genControlPlane()'s generated member IDs
// (which use 1..5) so the merged member list has no ID collisions — otherwise the generator
// could land on the same ID with a different Name, and the property would (correctly) fail
// because the first matching member in the slice wins, not the appended one.
func TestProperty_FirstMatchWinsAnnotation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		idMemberID := rapid.Uint64Range(1000, 1016).Draw(t, "id") // disjoint from genControlPlane()'s 1..5 range
		pinned := rapid.SampledFrom(propNodePool).Draw(t, "pinnedName")

		baseMembers := []*etcd.Member{{ID: idMemberID, Name: pinned}}
		baseM := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "m-deleting",
				Annotations: map[string]string{controlplanev1.EtcdMemberIDAnnotation: strconv.FormatUint(idMemberID, 10)},
			},
		}
		baseCp := propControlPlane{machines: []*clusterv1.Machine{baseM}, etcdMembers: baseMembers}
		baseMember, _ := runChain(t, baseCp)
		baseName := nameOf(baseMember)
		if baseName != pinned {
			t.Fatalf("base annotation resolution failed: got %q want %q", baseName, pinned)
		}

		mutated := genControlPlane().Draw(t, "noise")
		mutated.machines[0] = baseM
		mutated.etcdMembers = append(mutated.etcdMembers, baseMembers...)
		mutMember, _ := runChain(t, mutated)
		mutName := nameOf(mutMember)
		if mutName != pinned {
			t.Fatalf("annotation resolution should win over later-stage mutations; got %q want %q", mutName, pinned)
		}
	})
}

// P6 Strict-pairing soundness: if the fallback returns a name via step 5 (i.e. all earlier
// steps were neutralised), then the input had exactly one Machine without a NodeRef and
// exactly one orphan etcd-member candidate.
//
// The test isolates step 5 by stripping inputs that would let earlier steps fire:
//   - no override annotations (skip cases where step 1 could resolve)
//   - no NodeRef on the deletingMachine (skip step 2)
//   - no ProviderID on the deletingMachine (skip step 3)
//   - nil addresses on EVERY Machine, so step 4's peer-URL ↔ address match and step 5's
//     own address-pairing relation are both empty. With no address relations, my code's
//     unmatched-count and the property's absolute unmatched-count are guaranteed to agree.
func TestProperty_StrictPairingSoundness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genControlPlane().Draw(t, "cp")

		dm := cp.machines[0]
		if hasOverrideAnnotation(dm) {
			t.Skip()
		}
		if dm.Status.NodeRef.Name != "" {
			t.Skip()
		}
		if dm.Spec.ProviderID != "" {
			t.Skip()
		}
		// Strip all addresses so address-pairing is empty and step 5's relative-unmatched count
		// equals the absolute count. Also mark the deletingMachine as step5-admissible so the
		// new MHC + startup-grace gate doesn't preempt step 5 — the property is about step 5's
		// pairing soundness when it fires, not the gate's correctness (that lives in
		// TestStep5Admissible).
		stripped := make([]*clusterv1.Machine, 0, len(cp.machines))
		for i, m := range cp.machines {
			c := m.DeepCopy()
			c.Status.Addresses = nil
			if i == 0 {
				c.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * step5StartupGrace))
				conditions.Set(c, metav1.Condition{
					Type:   clusterv1.MachineHealthCheckSucceededCondition,
					Status: metav1.ConditionFalse,
					Reason: "Test",
				})
			}
			stripped = append(stripped, c)
		}
		cp.machines = stripped

		member, _ := runChain(t, cp)
		name := nameOf(member)
		if name == "" {
			return
		}

		// Step 5 fired. Verify the invariant.
		nodeSet := map[string]bool{}
		for _, n := range cp.nodes {
			nodeSet[n.Name] = true
		}
		candidates := []*etcd.Member{}
		for _, m := range cp.etcdMembers {
			if m.Name == "" || nodeSet[m.Name] {
				continue
			}
			candidates = append(candidates, m)
		}
		unmatchedMachines := 0
		for _, m := range cp.machines {
			if !m.Status.NodeRef.IsDefined() {
				unmatchedMachines++
			}
		}
		if unmatchedMachines != 1 {
			t.Fatalf("step 5 fired but %d unmatched Machines exist (expected 1)", unmatchedMachines)
		}
		if len(candidates) != 1 {
			t.Fatalf("step 5 fired but %d orphan candidates exist (expected 1)", len(candidates))
		}
	})
}

// Utility helpers (memberIDs, memberIDSet, memberNames, sortedIDs, equalUint64Slices,
// reverseMachines, reverseMembers, hasOverrideAnnotation) live in property_generators_test.go.
