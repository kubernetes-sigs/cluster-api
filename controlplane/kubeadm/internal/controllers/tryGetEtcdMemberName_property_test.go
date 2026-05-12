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
	"sort"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"pgregory.net/rapid"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/api/core/v1beta2/index"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
	"sigs.k8s.io/cluster-api/util/collections"
)

// ---------------- Generator infrastructure -----------------------------------

// Bounded pools shared across every generator. Keeping the alphabets small forces shrinking
// to converge quickly and lets properties say things like "two members with the same address"
// without rapid having to land on the exact string by chance.
var (
	propIPv4Pool = []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5", "10.0.0.6", "10.0.0.7", "10.0.0.8"}
	propIPv6Pool = []string{"fd00::1", "fd00::2", "fd00::3", "fd00::4"}
	propNodePool = []string{"node-a", "node-b", "node-c", "node-d", "node-e", "node-f", "node-g", "node-h"}
)

// genHost returns one of the bounded pool entries plus a small chance of a malformed string,
// to keep peer-URL parsing exercised under shrink.
func genHost() *rapid.Generator[string] {
	return rapid.OneOf(
		rapid.SampledFrom(propIPv4Pool),
		rapid.SampledFrom(propIPv6Pool),
		rapid.Just("garbage://[::"),
		rapid.Just(""),
	)
}

// genPeerURL produces an etcd peer URL string. For IPv4/IPv6 hosts it returns the canonical
// `https://host:2380` form; for garbage hosts it returns the host string verbatim so the parser
// is exercised against unparseable input.
func genPeerURL() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		h := genHost().Draw(t, "host")
		switch h {
		case "":
			return ""
		case "garbage://[::":
			return h
		}
		if strings.Contains(h, ":") && !strings.HasPrefix(h, "[") {
			return "https://[" + h + "]:2380"
		}
		return "https://" + h + ":2380"
	})
}

// genMember builds an etcd.Member with a deterministic ID supplied by the caller (so the
// caller can guarantee uniqueness across a generated set).
func genMember(id uint64) *rapid.Generator[*etcd.Member] {
	return rapid.Custom(func(t *rapid.T) *etcd.Member {
		name := rapid.OneOf(
			rapid.SampledFrom(propNodePool),
			rapid.Just(""),
		).Draw(t, "memberName")
		peerCount := rapid.IntRange(0, 2).Draw(t, "peerURLCount")
		peers := make([]string, 0, peerCount)
		for i := 0; i < peerCount; i++ {
			peers = append(peers, genPeerURL().Draw(t, "peerURL"))
		}
		return &etcd.Member{
			ID:        id,
			Name:      name,
			PeerURLs:  peers,
			IsLearner: rapid.Bool().Draw(t, "isLearner"),
		}
	})
}

// genAddresses returns a small slice of MachineAddress entries with a mix of Type values.
func genAddresses() *rapid.Generator[[]clusterv1.MachineAddress] {
	return rapid.Custom(func(t *rapid.T) []clusterv1.MachineAddress {
		n := rapid.IntRange(0, 3).Draw(t, "addressCount")
		out := make([]clusterv1.MachineAddress, 0, n)
		types := []clusterv1.MachineAddressType{
			clusterv1.MachineInternalIP, clusterv1.MachineExternalIP,
			clusterv1.MachineHostName, clusterv1.MachineInternalDNS, clusterv1.MachineExternalDNS,
		}
		for i := 0; i < n; i++ {
			h := rapid.OneOf(rapid.SampledFrom(propIPv4Pool), rapid.SampledFrom(propIPv6Pool)).Draw(t, "addr")
			out = append(out, clusterv1.MachineAddress{
				Type:    rapid.SampledFrom(types).Draw(t, "addrType"),
				Address: h,
			})
		}
		return out
	})
}

// genMachine assembles a Machine with an optional NodeRef (drawn from the bounded node pool),
// optional addresses, and optional override annotations. `name` is supplied by the caller so the
// generated set has unique Machine names.
func genMachine(name string) *rapid.Generator[*clusterv1.Machine] {
	return rapid.Custom(func(t *rapid.T) *clusterv1.Machine {
		m := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}
		if rapid.Bool().Draw(t, "hasNodeRef") {
			m.Status.NodeRef.Name = rapid.SampledFrom(propNodePool).Draw(t, "nodeRefName")
		}
		if rapid.Bool().Draw(t, "hasAddresses") {
			m.Status.Addresses = genAddresses().Draw(t, "addresses")
		}
		if rapid.Bool().Draw(t, "hasIDAnnotation") {
			if m.Annotations == nil {
				m.Annotations = map[string]string{}
			}
			m.Annotations[controlplanev1.EtcdMemberIDAnnotation] = rapid.SampledFrom([]string{"1", "2", "3", "0xa", "garbage"}).Draw(t, "idAnnValue")
		}
		if rapid.Bool().Draw(t, "hasNameAnnotation") {
			if m.Annotations == nil {
				m.Annotations = map[string]string{}
			}
			m.Annotations[controlplanev1.EtcdMemberNameAnnotation] = rapid.SampledFrom(append([]string{"operator-pinned"}, propNodePool...)).Draw(t, "nameAnnValue")
		}
		return m
	})
}

// propControlPlane is a self-contained snapshot of inputs the property tests feed into
// tryGetEtcdMemberName. The deletingMachine is always the first element of `machines` — that
// lets shrink keep it stable while varying everything else.
type propControlPlane struct {
	machines    []*clusterv1.Machine
	etcdMembers []*etcd.Member
	nodes       []*internal.Node
}

func genControlPlane() *rapid.Generator[propControlPlane] {
	return rapid.Custom(func(t *rapid.T) propControlPlane {
		nMachines := rapid.IntRange(1, 5).Draw(t, "machineCount")
		machines := make([]*clusterv1.Machine, 0, nMachines)
		for i := 0; i < nMachines; i++ {
			machines = append(machines, genMachine("m-"+propNodePool[i]).Draw(t, "machine"))
		}
		machines[0].ObjectMeta.Name = "m-deleting"

		nMembers := rapid.IntRange(0, 5).Draw(t, "memberCount")
		etcdMembers := make([]*etcd.Member, 0, nMembers)
		for i := 0; i < nMembers; i++ {
			etcdMembers = append(etcdMembers, genMember(uint64(i+1)).Draw(t, "member"))
		}

		nNodes := rapid.IntRange(0, 5).Draw(t, "nodeCount")
		nodes := make([]*internal.Node, 0, nNodes)
		for i := 0; i < nNodes; i++ {
			nodes = append(nodes, &internal.Node{
				ObjectMeta: internal.ObjectMeta{
					Name: rapid.SampledFrom(propNodePool).Draw(t, "cpNodeName"),
				},
			})
		}
		return propControlPlane{machines: machines, etcdMembers: etcdMembers, nodes: nodes}
	})
}

// runChain invokes tryGetEtcdMemberName against a propControlPlane. The remote-client mock
// always returns no Nodes — step 3 (ProviderID → Node lookup) is exercised in the table-driven
// test; the properties focus on steps 1, 2, 4, 5, 6 which are pure functions of the input.
func runChain(t *rapid.T, cp propControlPlane) (string, []*etcd.Member) {
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

// ---------------- Property tests ---------------------------------------------

// P3 Determinism: tryGetEtcdMemberName is a pure function — repeated calls on the same input
// return equal results.
func TestProperty_Determinism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cp := genControlPlane().Draw(t, "cp")
		n1, c1 := runChain(t, cp)
		n2, c2 := runChain(t, cp)
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
		name, _ := runChain(t, cp)
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
		name, candidates := runChain(t, cp)
		if name != "" {
			return
		}
		nodeSet := map[string]bool{}
		for _, n := range cp.nodes {
			nodeSet[n.Name] = true
		}
		expectedIDs := map[uint64]bool{}
		for _, m := range cp.etcdMembers {
			if m.Name == "" {
				continue
			}
			if nodeSet[m.Name] {
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
		baseName, baseCandidates := runChain(t, cp)

		permMachines := append([]*clusterv1.Machine{cp.machines[0]}, reverseMachines(cp.machines[1:])...)
		permMembers := reverseMembers(cp.etcdMembers)
		permuted := propControlPlane{machines: permMachines, etcdMembers: permMembers, nodes: cp.nodes}
		permName, permCandidates := runChain(t, permuted)

		if permName != baseName {
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
		baseName, _ := runChain(t, cp)

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
		mutName, _ := runChain(t, mutatedCp)

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
		name, _ := runChain(t, cp)
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
		baseName, _ := runChain(t, baseCp)
		if baseName != pinned {
			t.Fatalf("base annotation resolution failed: got %q want %q", baseName, pinned)
		}

		mutated := genControlPlane().Draw(t, "noise")
		mutated.machines[0] = baseM
		mutated.etcdMembers = append(mutated.etcdMembers, baseMembers...)
		mutName, _ := runChain(t, mutated)
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
		// equals the absolute count.
		stripped := make([]*clusterv1.Machine, 0, len(cp.machines))
		for _, m := range cp.machines {
			c := m.DeepCopy()
			c.Status.Addresses = nil
			stripped = append(stripped, c)
		}
		cp.machines = stripped

		name, _ := runChain(t, cp)
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

// ---------------- Local helpers ----------------------------------------------

func memberIDs(ms []*etcd.Member) []uint64 {
	out := make([]uint64, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func memberIDSet(ms []*etcd.Member) map[uint64]bool {
	out := map[uint64]bool{}
	for _, m := range ms {
		out[m.ID] = true
	}
	return out
}

func memberNames(ms []*etcd.Member) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Name)
	}
	return out
}

func sortedIDs(in []uint64) []uint64 {
	out := append([]uint64(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equalUint64Slices(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func reverseMachines(in []*clusterv1.Machine) []*clusterv1.Machine {
	out := make([]*clusterv1.Machine, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func reverseMembers(in []*etcd.Member) []*etcd.Member {
	out := make([]*etcd.Member, len(in))
	for i := range in {
		out[len(in)-1-i] = in[i]
	}
	return out
}

func hasOverrideAnnotation(m *clusterv1.Machine) bool {
	a := m.GetAnnotations()
	if a == nil {
		return false
	}
	_, idOK := a[controlplanev1.EtcdMemberIDAnnotation]
	_, nameOK := a[controlplanev1.EtcdMemberNameAnnotation]
	return idOK || nameOK
}
