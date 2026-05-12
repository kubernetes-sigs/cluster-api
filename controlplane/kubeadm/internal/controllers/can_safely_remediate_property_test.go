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

// Property-based tests for canSafelyRemediateMachine. The function composes
// targetKubernetesControlPlaneComponentsHealthy, the tryGetEtcdMemberName fallback chain, and
// targetEtcdClusterHealthy — so properties here lock in the composition rules (which gates
// short-circuit, which side-effects fire on the deleting Machine, when the empty-correlation
// path proceeds vs blocks).

package controllers

import (
	"context"
	"fmt"
	"strings"
	"testing"

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

// canSafelyRemediateFixture is the assembled input to canSafelyRemediateMachine plus the
// flags the test wants to assert against (etcdManaged is read out of the controlPlane post-hoc).
type canSafelyRemediateFixture struct {
	controlPlane *internal.ControlPlane
	target       *clusterv1.Machine
	etcdManaged  bool
}

// genCanSafelyRemediateFixture builds a control plane targetable by canSafelyRemediateMachine.
// Knobs: etcd managed/external, per-Machine K8s-CP health (4 conditions), per-Machine etcd
// member presence + IsLearner. The "target" machine is one of cp.Machines — selected randomly
// from the set so the property test exercises both "target is in cp" and "target is healthy /
// unhealthy" paths.
func genCanSafelyRemediateFixture() *rapid.Generator[canSafelyRemediateFixture] {
	return rapid.Custom(func(t *rapid.T) canSafelyRemediateFixture {
		etcdManaged := rapid.Bool().Draw(t, "etcdManaged")
		nMachines := rapid.IntRange(1, 5).Draw(t, "machineCount")
		machines := make([]*clusterv1.Machine, 0, nMachines)
		members := make([]*etcd.Member, 0, nMachines)
		for i := range nMachines {
			name := fmt.Sprintf("m-%d", i)
			nodeName := fmt.Sprintf("node-%d", i)
			m := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: name}}
			if rapid.Bool().Draw(t, fmt.Sprintf("m%d-hasNodeRef", i)) {
				m.Status.NodeRef.Name = nodeName
			}
			// K8s CP component conditions.
			for _, c := range []string{
				controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
				controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
				controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
				controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
			} {
				status := metav1.ConditionFalse
				reason := "NotRunning"
				if rapid.Bool().Draw(t, fmt.Sprintf("m%d-%s", i, c)) {
					status = metav1.ConditionTrue
					reason = controlplanev1.KubeadmControlPlaneMachinePodRunningReason
				}
				conditions.Set(m, metav1.Condition{Type: c, Status: status, Reason: reason})
			}
			// Etcd member health.
			etcdH := metav1.ConditionFalse
			etcdR := controlplanev1.KubeadmControlPlaneMachineEtcdMemberNotHealthyReason
			if rapid.Bool().Draw(t, fmt.Sprintf("m%d-etcdHealthy", i)) {
				etcdH = metav1.ConditionTrue
				etcdR = controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason
			}
			conditions.Set(m, metav1.Condition{
				Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: etcdH, Reason: etcdR,
			})
			machines = append(machines, m)
			// Member exists if NodeRef was set (most realistic shape) — with some probability
			// of an orphan to exercise the empty-correlation branch.
			if m.Status.NodeRef.IsDefined() {
				members = append(members, &etcd.Member{
					ID:        uint64(i + 1),
					Name:      nodeName,
					IsLearner: rapid.Bool().Draw(t, fmt.Sprintf("m%d-isLearner", i)),
				})
			}
		}
		// Optional orphan etcd member with no Machine.
		if rapid.Bool().Draw(t, "addOrphanMember") {
			members = append(members, &etcd.Member{
				ID:        9999,
				Name:      "orphan-node",
				IsLearner: rapid.Bool().Draw(t, "orphanIsLearner"),
			})
		}

		cp := &internal.ControlPlane{
			Cluster: &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cluster"}},
			KCP: &controlplanev1.KubeadmControlPlane{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "kcp"},
			},
			Machines:       collections.FromMachines(machines...),
			EtcdMembers:    members,
			InfraResources: map[string]*unstructured.Unstructured{},
		}
		if !etcdManaged {
			cp.KCP.Spec.KubeadmConfigSpec.ClusterConfiguration.Etcd.External.Endpoints = []string{"https://external:2379"}
		}

		target := machines[rapid.IntRange(0, len(machines)-1).Draw(t, "targetIdx")]
		return canSafelyRemediateFixture{controlPlane: cp, target: target, etcdManaged: etcdManaged}
	})
}

// runCanSafelyRemediate invokes the function with a fresh reconciler (the ClusterCache call is
// only exercised when ProviderID is set on the target; our generator doesn't set ProviderID, so
// the remote-client mock can return no Nodes).
func runCanSafelyRemediate(t *rapid.T, fx canSafelyRemediateFixture) bool {
	remoteClient := fake.NewClientBuilder().
		WithIndex(&corev1.Node{}, index.MachineProviderIDField, index.NodeByProviderID).
		Build()
	r := &KubeadmControlPlaneReconciler{
		ClusterCache: clustercache.NewFakeClusterCache(remoteClient, client.ObjectKeyFromObject(fx.controlPlane.Cluster)),
	}
	return r.canSafelyRemediateMachine(t.Context(), fx.controlPlane, fx.target)
}

func TestProperty_CanSafelyRemediate_UnmanagedEtcd_SkipsEtcdChecks(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fx := genCanSafelyRemediateFixture().Draw(t, "fx")
		if fx.etcdManaged {
			t.Skip()
		}
		// With unmanaged etcd the function returns based solely on targetK8sCPHealthy.
		got := runCanSafelyRemediate(t, fx)
		r := &KubeadmControlPlaneReconciler{}
		want := r.targetKubernetesControlPlaneComponentsHealthy(t.Context(), fx.controlPlane, false, fx.target.Name)
		if got != want {
			t.Fatalf("unmanaged etcd: result must equal targetK8sCPHealthy; got=%v want=%v target=%q", got, want, fx.target.Name)
		}
	})
}

func TestProperty_CanSafelyRemediate_K8sCPUnhealthy_HardBlocks(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fx := genCanSafelyRemediateFixture().Draw(t, "fx")
		r := &KubeadmControlPlaneReconciler{}
		k8sHealthy := r.targetKubernetesControlPlaneComponentsHealthy(t.Context(), fx.controlPlane, false, fx.target.Name)
		if k8sHealthy {
			t.Skip()
		}
		got := runCanSafelyRemediate(t, fx)
		if got {
			t.Fatalf("K8s CP unhealthy must hard-block remediation; got true. target=%q etcdManaged=%v", fx.target.Name, fx.etcdManaged)
		}
	})
}

func TestProperty_CanSafelyRemediate_EmptyEtcdMembers_Blocks(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fx := genCanSafelyRemediateFixture().Draw(t, "fx")
		if !fx.etcdManaged {
			t.Skip()
		}
		// Force K8s CP healthy on at least one non-target Machine so the K8s-CP gate doesn't
		// short-circuit before we reach the empty-etcd branch.
		setAllMachinesHealthy(fx.controlPlane, fx.target.Name)
		// Force the target to have a NodeRef so tryGetEtcdMemberName resolves via step 2 (gets
		// a name) and the function then evaluates len(EtcdMembers).
		fx.target.Status.NodeRef.Name = "node-target"
		// Empty the etcd member list.
		fx.controlPlane.EtcdMembers = nil
		got := runCanSafelyRemediate(t, fx)
		if got {
			t.Fatalf("managed etcd with empty EtcdMembers and resolved name must block; got true")
		}
	})
}

func TestProperty_CanSafelyRemediate_NoCorrelation_OrphanCandidates_ProceedsWithCondition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fx := genCanSafelyRemediateFixture().Draw(t, "fx")
		if !fx.etcdManaged {
			t.Skip()
		}
		// Force the chain to hit step 6: clear the target's NodeRef + ProviderID + annotations,
		// strip its address set, and ensure there is ≥1 orphan member that doesn't match any
		// other Machine and there are ≥2 unmatched Machines (so step 5 cannot pair).
		fx.target.Status.NodeRef.Name = ""
		fx.target.Spec.ProviderID = ""
		fx.target.Annotations = nil
		fx.target.Status.Addresses = nil
		// Add a guaranteed orphan + a second un-noderef Machine to defeat step 5.
		fx.controlPlane.EtcdMembers = append(fx.controlPlane.EtcdMembers, &etcd.Member{
			ID: 88888, Name: "orphan-step6", IsLearner: true,
		})
		extra := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m-extra-orphan-machine"}}
		fx.controlPlane.Machines[extra.Name] = extra
		// Add a guaranteed-healthy peer Machine with a NodeRef so targetK8sCPHealthy can resolve
		// to true regardless of how the random fixture seeded the original Machines.
		peer := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m-healthy-peer"},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-healthy-peer"}},
		}
		fx.controlPlane.Machines[peer.Name] = peer
		setAllMachinesHealthy(fx.controlPlane, fx.target.Name)
		// Mark every non-target Machine's etcd member condition True so the recomputed
		// expectation doesn't fight the property.
		for _, m := range fx.controlPlane.Machines {
			if m.Name == fx.target.Name {
				continue
			}
			conditions.Set(m, metav1.Condition{
				Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue,
				Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason,
			})
		}

		got := runCanSafelyRemediate(t, fx)
		if !got {
			t.Fatalf("empty correlation with orphan candidates must proceed (returns true); got false. " +
				"K8s CP gate was forced healthy and etcd member list is non-empty.")
		}
		// The condition must be set to True/CorrelationFailed on the target.
		c := conditions.Get(fx.target, controlplanev1.KubeadmControlPlaneMachineEtcdMemberOrphanCorrelationFailedCondition)
		if c == nil || c.Status != metav1.ConditionTrue || c.Reason != controlplanev1.KubeadmControlPlaneMachineEtcdMemberOrphanCorrelationFailedReason {
			t.Fatalf("EtcdMemberOrphanCorrelationFailed condition not set to True/CorrelationFailed on the target; got %+v", c)
		}
	})
}

func TestProperty_CanSafelyRemediate_NoCorrelation_NoCandidates_ClearsCondition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fx := genCanSafelyRemediateFixture().Draw(t, "fx")
		if !fx.etcdManaged {
			t.Skip()
		}
		fx.target.Status.NodeRef.Name = ""
		fx.target.Spec.ProviderID = ""
		fx.target.Annotations = nil
		fx.target.Status.Addresses = nil
		// Drop all etcd members so candidates is empty.
		fx.controlPlane.EtcdMembers = nil
		// Add a guaranteed-healthy peer Machine so targetK8sCPHealthy passes regardless of the
		// random fixture; without this the property is conditional on the fixture's health
		// draws and rapid would spend its budget on filtered-out runs.
		peer := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: "m-healthy-peer"},
			Status:     clusterv1.MachineStatus{NodeRef: clusterv1.MachineNodeReference{Name: "node-healthy-peer"}},
		}
		fx.controlPlane.Machines[peer.Name] = peer
		setAllMachinesHealthy(fx.controlPlane, fx.target.Name)

		got := runCanSafelyRemediate(t, fx)
		if !got {
			t.Fatalf("empty correlation with no candidates and no members must return true; got false")
		}
		c := conditions.Get(fx.target, controlplanev1.KubeadmControlPlaneMachineEtcdMemberOrphanCorrelationFailedCondition)
		if c == nil || c.Status != metav1.ConditionFalse || c.Reason != controlplanev1.KubeadmControlPlaneMachineEtcdMemberOrphanCorrelationNoCandidatesReason {
			t.Fatalf("EtcdMemberOrphanCorrelationFailed condition not set to False/NoOrphanCandidates; got %+v", c)
		}
	})
}

func TestProperty_CanSafelyRemediate_NameResolved_AgreesWithQuorumGate(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fx := genCanSafelyRemediateFixture().Draw(t, "fx")
		if !fx.etcdManaged {
			t.Skip()
		}
		// Force the target to have a NodeRef and a corresponding member so the chain resolves
		// via step 2 unambiguously.
		fx.target.Status.NodeRef.Name = "node-target"
		fx.target.Annotations = nil
		fx.controlPlane.EtcdMembers = append(fx.controlPlane.EtcdMembers, &etcd.Member{
			ID: 77777, Name: "node-target",
		})
		setAllMachinesHealthy(fx.controlPlane, "")
		// Mark the target's etcd member as healthy too so the recomputed expectation matches.
		conditions.Set(fx.target, metav1.Condition{
			Type: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyCondition, Status: metav1.ConditionTrue,
			Reason: controlplanev1.KubeadmControlPlaneMachineEtcdMemberHealthyReason,
		})

		got := runCanSafelyRemediate(t, fx)

		// Expected: targetK8sCPHealthy AND targetEtcdClusterHealthy. K8s CP gate is forced
		// healthy above, so the result is whatever targetEtcdClusterHealthy says.
		r := &KubeadmControlPlaneReconciler{}
		k8sHealthy := r.targetKubernetesControlPlaneComponentsHealthy(t.Context(), fx.controlPlane, false, fx.target.Name)
		etcdHealthy := r.targetEtcdClusterHealthy(t.Context(), fx.controlPlane, false, &etcd.Member{Name: "node-target"})
		want := k8sHealthy && etcdHealthy
		if got != want {
			t.Fatalf("name-resolved path disagreed with quorum gate composition: got=%v want=%v (k8sHealthy=%v etcdHealthy=%v)",
				got, want, k8sHealthy, etcdHealthy)
		}
	})
}

func TestProperty_CanSafelyRemediate_Determinism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		fx := genCanSafelyRemediateFixture().Draw(t, "fx")
		a := runCanSafelyRemediate(t, fx)
		b := runCanSafelyRemediate(t, fx)
		if a != b {
			t.Fatalf("non-deterministic: %v vs %v", a, b)
		}
	})
}

// setAllMachinesHealthy forces every Machine except optionally one (`exceptName`) to be CP-
// healthy. Used by properties that want to neutralise the targetK8sCPHealthy gate so the
// etcd-side behaviour is the only thing varying.
func setAllMachinesHealthy(cp *internal.ControlPlane, exceptName string) {
	for _, m := range cp.Machines {
		if m.Name == exceptName {
			continue
		}
		for _, c := range []string{
			controlplanev1.KubeadmControlPlaneMachineAPIServerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineControllerManagerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineSchedulerPodHealthyCondition,
			controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
		} {
			conditions.Set(m, metav1.Condition{Type: c, Status: metav1.ConditionTrue, Reason: controlplanev1.KubeadmControlPlaneMachinePodRunningReason})
		}
	}
}

// silence the unused-import check when this file is the only one in the package needing them.
var (
	_ = context.Background
	_ = strings.Join
)
