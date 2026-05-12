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

// Shared rapid generators for KCP property-based tests. Every property test in this package
// reuses the same bounded alphabets, member/machine shapes, and helpers — keeping one set
// here means a fix to one generator improves shrinking everywhere and the cross-property
// counter-examples stay legible.

package controllers

import (
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"pgregory.net/rapid"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal/etcd"
)

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

// propControlPlane is a self-contained snapshot of inputs the property tests feed into the
// functions under test. The deletingMachine / target machine is conventionally the first
// element of `machines` so shrinking keeps it stable while varying everything else.
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

// ---------------- Utility helpers used by property assertions ----------------

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
