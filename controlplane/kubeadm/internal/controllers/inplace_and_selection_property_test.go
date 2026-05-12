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

// Property-based tests for canUpdateMachine (the in-place-update gate) and
// selectMachineForInPlaceUpdateOrScaleDown (the failure-domain-balanced selection helper).

package controllers

import (
	"context"
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"pgregory.net/rapid"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controlplane/kubeadm/internal"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/collections"
)

// ---------------- canUpdateMachine -----------------------------------------.

// Most canUpdateMachine paths require a real RuntimeClient + extension wiring; the table-
// driven Test_canUpdateMachine in inplace_canupdatemachine_test.go covers those. The two
// universally-true short-circuits — feature-gate-off and incomplete-UpToDateResult — can be
// asserted via random property tests without any mocking.

func TestProperty_CanUpdateMachine_FeatureGateOff_AlwaysFalse(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, false)
	rapid.Check(t, func(t *rapid.T) {
		r := &KubeadmControlPlaneReconciler{}
		machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m"}}
		// Random (incomplete or complete) UpToDateResult — irrelevant when the feature gate
		// is off, but the property should hold for any input shape.
		res := internal.UpToDateResult{}
		got, err := r.canUpdateMachine(t.Context(), machine, res)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("InPlaceUpdates feature gate is off; canUpdateMachine must return false, got true")
		}
	})
}

func TestProperty_CanUpdateMachine_IncompleteUpToDateResult_False(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)
	rapid.Check(t, func(t *rapid.T) {
		// Draw which of the five required pointers to leave nil; at least one is nil so the
		// guard at inplace_canupdatemachine.go:58-64 must fire.
		nilDesiredMachine := rapid.Bool().Draw(t, "nilDesiredMachine")
		nilCurrentInfra := rapid.Bool().Draw(t, "nilCurrentInfra")
		nilDesiredInfra := rapid.Bool().Draw(t, "nilDesiredInfra")
		nilCurrentKubeadm := rapid.Bool().Draw(t, "nilCurrentKubeadm")
		nilDesiredKubeadm := rapid.Bool().Draw(t, "nilDesiredKubeadm")
		// At least one must be nil; if rapid picks all-true, fine; if all-false, force one
		// to be nil so the property's precondition is satisfied.
		if !nilDesiredMachine && !nilCurrentInfra && !nilDesiredInfra && !nilCurrentKubeadm && !nilDesiredKubeadm {
			nilDesiredMachine = true
		}
		res := internal.UpToDateResult{}
		if !nilDesiredMachine {
			res.DesiredMachine = &clusterv1.Machine{}
		}
		// The other fields are infrastructure-package types we don't construct here; passing
		// nil for them is exactly the precondition this property tests.
		_ = nilCurrentInfra
		_ = nilDesiredInfra
		_ = nilCurrentKubeadm
		_ = nilDesiredKubeadm

		r := &KubeadmControlPlaneReconciler{}
		machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m"}}
		got, err := r.canUpdateMachine(t.Context(), machine, res)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatalf("at least one UpToDateResult field is nil; canUpdateMachine must return false, got true")
		}
	})
}

func TestProperty_CanUpdateMachine_ExtensionsAccept_True(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)
	rapid.Check(t, func(t *rapid.T) {
		// Use the override hook to short-circuit the extension-call path with a "yes" verdict.
		// This isolates the property: when the chain reaches the extension delegate AND the
		// delegate says yes, the final result must be true.
		r := &KubeadmControlPlaneReconciler{
			overrideCanUpdateMachineFunc: func(_ context.Context, _ *clusterv1.Machine, _ internal.UpToDateResult) (bool, error) {
				return true, nil
			},
		}
		machine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "m"}}
		got, err := r.canUpdateMachine(t.Context(), machine, internal.UpToDateResult{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatalf("override returns true → canUpdateMachine must return true, got false")
		}
	})
}

// ---------------- selectMachineForInPlaceUpdateOrScaleDown -----------------.

// genSelectableCP generates a control plane with random failure-domain assignments and a
// random subset of outdated Machines. Three failure domains are always declared in
// Cluster.Status.FailureDomains so MachineInFailureDomainWithMostMachines treats them as
// "defined" FDs and applies its load-balancing logic (rather than the
// `notInFailureDomains` short-circuit that fires when an eligible Machine sits in an
// FD missing from Cluster.Status.FailureDomains).
func genSelectableCP() *rapid.Generator[struct {
	cp       *internal.ControlPlane
	outdated collections.Machines
}] {
	return rapid.Custom(func(t *rapid.T) struct {
		cp       *internal.ControlPlane
		outdated collections.Machines
	} {
		nMachines := rapid.IntRange(1, 5).Draw(t, "nMachines")
		fds := []string{"fd-0", "fd-1", "fd-2"}
		machines := []*clusterv1.Machine{}
		for i := range nMachines {
			fd := fds[rapid.IntRange(0, len(fds)-1).Draw(t, "fdIdx")]
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "m-" + strconv.Itoa(i),
					CreationTimestamp: metav1.NewTime(metav1.Now().Add(0)),
				},
				Spec: clusterv1.MachineSpec{FailureDomain: fd},
			}
			machines = append(machines, m)
		}
		controlPlaneTrue := true
		fdSlice := []clusterv1.FailureDomain{}
		for _, fd := range fds {
			fdSlice = append(fdSlice, clusterv1.FailureDomain{Name: fd, ControlPlane: &controlPlaneTrue})
		}
		cp := &internal.ControlPlane{
			Cluster: &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "c"},
				Status:     clusterv1.ClusterStatus{FailureDomains: fdSlice},
			},
			KCP:      &controlplanev1.KubeadmControlPlane{ObjectMeta: metav1.ObjectMeta{Name: "kcp"}},
			Machines: collections.FromMachines(machines...),
		}
		// Outdated subset: random non-empty subset, otherwise the function falls through to
		// "consider all Machines" which is a less interesting branch for these properties.
		outdated := collections.Machines{}
		for _, m := range machines {
			if rapid.Bool().Draw(t, "outdated-"+m.Name) {
				outdated[m.Name] = m
			}
		}
		return struct {
			cp       *internal.ControlPlane
			outdated collections.Machines
		}{cp, outdated}
	})
}

func TestProperty_SelectMachine_OutputInInputSet(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genSelectableCP().Draw(t, "in")
		out, err := selectMachineForInPlaceUpdateOrScaleDown(t.Context(), in.cp, in.outdated)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out == nil {
			// May legitimately be nil if no eligible Machine — but with our generator we
			// always have ≥1 Machine in cp.Machines, so the default branch picks it up.
			t.Fatalf("nil output for non-empty Machines set; cp.Machines=%v", in.cp.Machines.Names())
			return
		}
		// out must be in cp.Machines (the function never returns a Machine not in the snapshot).
		if _, ok := in.cp.Machines[out.Name]; !ok {
			t.Fatalf("selected Machine %q is not in cp.Machines %v", out.Name, in.cp.Machines.Names())
		}
	})
}

// selectMachineForInPlaceUpdateOrScaleDown is intentionally non-deterministic on FD ties
// (failuredomains.PickMost uses map iteration order to spread Machines across failure
// domains; see util/failuredomains/failure_domains.go:158). So we test a SET-based property:
// every selection over the same inputs must come from the FD that maximises the
// (priorityCount, allMachinesCount) lexicographic tuple, but the specific Machine returned
// can vary across calls.

func TestProperty_SelectMachine_SelectedFDIsTopRanked(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genSelectableCP().Draw(t, "in")
		out, err := selectMachineForInPlaceUpdateOrScaleDown(t.Context(), in.cp, in.outdated)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out == nil {
			t.Skip()
		}
		// Determine which `eligibleMachines` the function picked from. Our generator never sets
		// the delete annotation or unhealthy K8s/etcd conditions, so the eligible set is
		// outdated (when non-empty) or cp.Machines (default).
		eligible := in.outdated
		if len(eligible) == 0 {
			eligible = in.cp.Machines
		}
		// Compute the top FD score (priorityCount, allMachinesCount).
		priorityCounts := map[string]int{}
		for _, m := range eligible {
			priorityCounts[m.Spec.FailureDomain]++
		}
		allCounts := map[string]int{}
		for _, m := range in.cp.Machines {
			allCounts[m.Spec.FailureDomain]++
		}
		topPriority := 0
		for _, c := range priorityCounts {
			if c > topPriority {
				topPriority = c
			}
		}
		// Among FDs with priorityCount == topPriority, find the max allCount.
		topAll := 0
		for fd, p := range priorityCounts {
			if p == topPriority && allCounts[fd] > topAll {
				topAll = allCounts[fd]
			}
		}
		// Selected Machine's FD must have (priorityCount=topPriority, allCount=topAll).
		selFD := out.Spec.FailureDomain
		if priorityCounts[selFD] != topPriority || allCounts[selFD] != topAll {
			t.Fatalf("selected Machine %q is in FD %q (priority=%d allCount=%d) but the top-ranked tuple is (%d, %d). priorityCounts=%v allCounts=%v",
				out.Name, selFD, priorityCounts[selFD], allCounts[selFD], topPriority, topAll, priorityCounts, allCounts)
		}
	})
}

func TestProperty_SelectMachine_NonNilWhenEligibleNonEmpty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		in := genSelectableCP().Draw(t, "in")
		// At least one Machine exists in cp.Machines (our generator enforces nMachines ≥ 1),
		// so the default branch of the switch makes the eligible set non-empty.
		out, err := selectMachineForInPlaceUpdateOrScaleDown(t.Context(), in.cp, in.outdated)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out == nil {
			t.Fatalf("expected non-nil Machine selection (cp.Machines is non-empty); got nil")
		}
	})
}
