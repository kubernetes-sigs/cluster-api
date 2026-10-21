/*
Copyright 2021 The Kubernetes Authors.

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

package machinedeployment

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/core/reconcilers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/util/inplace"
)

func TestReconcileReplicasPendingAcknowledgeMove(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)

	testCases := []struct {
		name                              string
		md                                *clusterv1.MachineDeployment
		originalMSs                       map[string]*clusterv1.MachineSet
		newMS                             *clusterv1.MachineSet
		oldMSs                            []*clusterv1.MachineSet
		machines                          []*clusterv1.Machine
		expectedReplicas                  map[string]int32
		expectedAcknowledgeMoveAnnotation map[string]*string
		expectedNotes                     map[string][]string
	}{
		// AcknowledgeMove to the new MS
		{
			name:                              "Should not scale up newMS when there are no machines",
			md:                                createMD("v1", 3),
			newMS:                             createMS("ms1", "v1", 1),
			machines:                          nil,
			expectedReplicas:                  map[string]int32{"ms1": 1},
			expectedAcknowledgeMoveAnnotation: nil,
		},
		{
			name:  "Should not scale up when there are no machines recently moved to newMS",
			md:    createMD("v1", 3),
			newMS: createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
			},
			expectedReplicas:                  map[string]int32{"ms1": 1},
			expectedAcknowledgeMoveAnnotation: nil,
		},
		{
			name:  "Should scale up when there are machines recently moved to newMS and not yet acknowledged",
			md:    createMD("v1", 3),
			newMS: createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas:                  map[string]int32{"ms1": 2},           // up by one
			expectedAcknowledgeMoveAnnotation: map[string]*string{"ms1": new("m2")}, // One machine acknowledged
			expectedNotes:                     map[string][]string{"ms1": {"acknowledge Machines m2 moved from an old MachineSet"}},
		},
		{
			name:        "Should scale up when there are machines recently moved to newMS and not yet acknowledged and there are other machines already acknowledged",
			md:          createMD("v1", 3),
			originalMSs: map[string]*clusterv1.MachineSet{"ms1": createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m1"))}, // another machine already acknowledged
			newMS:       createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas:                  map[string]int32{"ms1": 2},              // up by one
			expectedAcknowledgeMoveAnnotation: map[string]*string{"ms1": new("m1,m2")}, // One machine acknowledged, one already acknowledged (keep this entry until the MS controller removes PendingAcknowledgeMoveAnnotation)
			expectedNotes:                     map[string][]string{"ms1": {"acknowledge Machines m1,m2 moved from an old MachineSet"}},
		},
		{
			name:        "Should not scale up newMS when there machines with the pendingAcknowledgeMove annotation but they are already acknowledged",
			md:          createMD("v1", 3),
			originalMSs: map[string]*clusterv1.MachineSet{"ms1": createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m2"))}, // moved machine already acknowledged
			newMS:       createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas:                  map[string]int32{"ms1": 1},
			expectedAcknowledgeMoveAnnotation: map[string]*string{"ms1": new("m2")}, // One machine already acknowledged (keep this entry until the MS controller removes PendingAcknowledgeMoveAnnotation)
		},
		{
			name:        "Should drop machines from acknowledged move annotation on newMS when they not anymore reporting pendingAcknowledge",
			md:          createMD("v1", 3),
			originalMSs: map[string]*clusterv1.MachineSet{"ms1": createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m2"))}, // moved machine already acknowledged
			newMS:       createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
			},
			expectedReplicas: map[string]int32{"ms1": 1},
		},

		// AcknowledgeMove to the old MS
		{
			name:  "Should not scale up oldMS when there are no machines",
			md:    createMD("v1", 3),
			newMS: createMS("ms2", "v2", 0),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: nil,
			expectedReplicas: map[string]int32{
				"ms2": 0,
				"ms1": 1,
			},
			expectedAcknowledgeMoveAnnotation: nil,
		},
		{
			name:  "Should not scale up when there are no machines recently moved to oldMS",
			md:    createMD("v1", 3),
			newMS: createMS("ms2", "v2", 0),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
			},
			expectedReplicas: map[string]int32{
				"ms2": 0,
				"ms1": 1,
			},
			expectedAcknowledgeMoveAnnotation: nil,
		},
		{
			name:  "Should not scale up when there are machines recently moved to oldMS and not yet acknowledged",
			md:    createMD("v1", 3),
			newMS: createMS("ms2", "v2", 0),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas: map[string]int32{
				"ms2": 0,
				"ms1": 1,
			},
			expectedAcknowledgeMoveAnnotation: map[string]*string{"ms1": new("m2")}, // One machine acknowledged
			expectedNotes:                     map[string][]string{"ms1": {"acknowledge Machines m2 moved from an old MachineSet"}},
		},
		{
			name:        "Should not scale up when there are machines recently moved to oldMS and not yet acknowledged and there are other machines already acknowledged",
			md:          createMD("v1", 3),
			originalMSs: map[string]*clusterv1.MachineSet{"ms1": createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m1"))}, // another machine already acknowledged
			newMS:       createMS("ms2", "v2", 0),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas: map[string]int32{
				"ms2": 0,
				"ms1": 1,
			},
			expectedAcknowledgeMoveAnnotation: map[string]*string{"ms1": new("m1,m2")}, // One machine acknowledged, one already acknowledged (keep this entry until the MS controller removes PendingAcknowledgeMoveAnnotation)
			expectedNotes:                     map[string][]string{"ms1": {"acknowledge Machines m1,m2 moved from an old MachineSet"}},
		},
		{
			name:        "Should not scale up oldMS when there machines with the pendingAcknowledgeMove annotation but they are already acknowledged",
			md:          createMD("v1", 3),
			originalMSs: map[string]*clusterv1.MachineSet{"ms1": createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m2"))}, // moved machine already acknowledged
			newMS:       createMS("ms2", "v2", 0),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas: map[string]int32{
				"ms2": 0,
				"ms1": 1,
			},
			expectedAcknowledgeMoveAnnotation: map[string]*string{"ms1": new("m2")}, // One machine already acknowledged (keep this entry until the MS controller removes PendingAcknowledgeMoveAnnotation)
		},
		{
			name:        "Should drop machines from acknowledged move annotation on oldMS when they not anymore reporting pendingAcknowledge",
			md:          createMD("v1", 3),
			originalMSs: map[string]*clusterv1.MachineSet{"ms1": createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m2"))}, // moved machine already acknowledged
			newMS:       createMS("ms2", "v2", 0),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
			},
			expectedReplicas: map[string]int32{
				"ms2": 0,
				"ms1": 1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			planner := newRolloutPlanner(nil, nil, nil)
			planner.md = tc.md
			planner.newMS = tc.newMS
			planner.oldMSs = tc.oldMSs
			planner.originalMSs = tc.originalMSs
			planner.machines = tc.machines

			planner.reconcileReplicasPendingAcknowledgeMove(ctx)

			for _, ms := range append(planner.oldMSs, planner.newMS) {
				g.Expect(ptr.Deref(ms.Spec.Replicas, 0)).To(Equal(tc.expectedReplicas[ms.Name]))
				if tc.expectedAcknowledgeMoveAnnotation[ms.Name] != nil {
					g.Expect(ms.Annotations).To(HaveKeyWithValue(clusterv1.AcknowledgedMoveAnnotation, *tc.expectedAcknowledgeMoveAnnotation[ms.Name]))
				} else {
					g.Expect(ms.Annotations).ToNot(HaveKey(clusterv1.AcknowledgedMoveAnnotation))
				}
				g.Expect(planner.notes[ms.Name]).To(Equal(tc.expectedNotes[ms.Name]), "unexpected notes")
			}
		})
	}
}

func Test_canUpdateMachineSetInPlaceDecisions(t *testing.T) {
	var ctx = context.Background()
	now := time.Now()

	inPlaceUpdateNotAllowed := new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationReplace})
	replaceNotAllowed := new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace})
	nothingAllowed := new([]runtimehooksv1.MachineUpdateOperation{})

	tests := []struct {
		name                                                  string
		inPlaceUpdatesOff                                     bool
		md                                                    *clusterv1.MachineDeployment
		newMS                                                 *clusterv1.MachineSet
		oldMSs                                                []*clusterv1.MachineSet
		upToDateResults                                       map[string]mdutil.UpToDateResult
		canUpdateResults                                      map[string]canUpdateMachineSetResult
		expectNoSetsInitted                                   bool
		expectPendingMSs                                      []string
		expectBlockedMSs                                      []string
		expectReplaceCandidates                               []string
		expectInPlaceUpdateCandidates                         []string
		expectInPlaceUpdateCandidatesNotAffectingAvailability []string
		expectPendingRetryAfter                               time.Duration
		expectBlockedRetryAfter                               time.Duration
		expectedNotes                                         map[string][]string
	}{
		{
			name:                "no op if the InPlaceUpdates feature gate is off",
			inPlaceUpdatesOff:   true,
			md:                  createMD("v2", 3),
			newMS:               createMS("ms-new", "v2", 1),
			oldMSs:              []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults:     map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			expectNoSetsInitted: true,
			expectedNotes:       map[string][]string{},
		},
		{
			name:                "no op if the newMS already has all the desired replicas",
			md:                  createMD("v2", 3),
			newMS:               createMS("ms-new", "v2", 3),
			oldMSs:              []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults:     map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			expectNoSetsInitted: true,
			expectedNotes:       map[string][]string{},
		},
		{
			name:                    "oldMS without replicas is skipped",
			md:                      createMD("v2", 3),
			newMS:                   createMS("ms-new", "v2", 1),
			oldMSs:                  []*clusterv1.MachineSet{createMS("ms1", "v1", 0)},
			upToDateResults:         map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			expectReplaceCandidates: []string{},
			expectedNotes:           map[string][]string{},
		},
		{
			name:                    "oldMS not eligible for in-place update is a replace candidate, the extension is not called",
			md:                      createMD("v2", 3),
			newMS:                   createMS("ms-new", "v2", 1),
			oldMSs:                  []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults:         map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: false}},
			expectReplaceCandidates: []string{"ms1"},
			expectedNotes:           map[string][]string{},
		},
		{
			name:            "oldMS waiting for a decision from the extension is pending",
			md:              createMD("v2", 3),
			newMS:           createMS("ms-new", "v2", 1),
			oldMSs:          []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults: map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms1": {retryAfter: now.Add(2 * time.Minute), retryAfterFromExtension: true},
			},
			expectPendingMSs:        []string{"ms1"},
			expectPendingRetryAfter: 2 * time.Minute,
			expectedNotes: map[string][]string{
				"ms1": {"rollout paused, waiting for the extension to take a decision about how to roll out changes (retry after 2m0s)"},
			},
		},
		{
			name:            "oldMS for which no operation is allowed is blocked",
			md:              createMD("v2", 3),
			newMS:           createMS("ms-new", "v2", 1),
			oldMSs:          []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults: map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms1": {canUpdateMachineSet: true, allowedOperations: nothingAllowed, retryAfter: now.Add(5 * time.Minute), messages: []string{"maintenance window is closed"}},
			},
			expectBlockedMSs:        []string{"ms1"},
			expectBlockedRetryAfter: 5 * time.Minute,
			expectedNotes: map[string][]string{
				"ms1": {"rollout blocked, neither replace nor in-place update operations are allowed: maintenance window is closed"},
			},
		},
		{
			name:            "oldMS for which replace is not allowed and not all changes can be updated in-place is blocked",
			md:              createMD("v2", 3),
			newMS:           createMS("ms-new", "v2", 1),
			oldMSs:          []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults: map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms1": {canUpdateMachineSet: false, allowedOperations: replaceNotAllowed, retryAfter: now.Add(5 * time.Minute)},
			},
			expectBlockedMSs:        []string{"ms1"},
			expectBlockedRetryAfter: 5 * time.Minute,
			expectedNotes: map[string][]string{
				"ms1": {"rollout blocked, replace operation is not allowed and not all changes can be updated in-place"},
			},
		},
		{
			name:            "oldMS where in-place update is possible but not allowed is a replace candidate",
			md:              createMD("v2", 3),
			newMS:           createMS("ms-new", "v2", 1),
			oldMSs:          []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults: map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms1": {canUpdateMachineSet: true, affectsAvailability: true, allowedOperations: inPlaceUpdateNotAllowed},
			},
			expectReplaceCandidates: []string{"ms1"},
			expectedNotes: map[string][]string{
				"ms1": {"in-place update is possible but not allowed, falling back to replace machines"},
			},
		},
		{
			name:            "oldMS where in-place update is not possible is a replace candidate",
			md:              createMD("v2", 3),
			newMS:           createMS("ms-new", "v2", 1),
			oldMSs:          []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults: map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms1": {canUpdateMachineSet: false},
			},
			expectReplaceCandidates: []string{"ms1"},
			expectedNotes:           map[string][]string{},
		},
		{
			name:            "oldMS updated in-place affecting availability",
			md:              createMD("v2", 3),
			newMS:           createMS("ms-new", "v2", 1),
			oldMSs:          []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults: map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms1": {canUpdateMachineSet: true, affectsAvailability: true},
			},
			expectInPlaceUpdateCandidates: []string{"ms1"},
			expectedNotes: map[string][]string{
				"ms1": {"should scale down by moving Machines to MachineSet ms-new (affects availability)"},
			},
		},
		{
			name:            "oldMS updated in-place not affecting availability",
			md:              createMD("v2", 3),
			newMS:           createMS("ms-new", "v2", 1),
			oldMSs:          []*clusterv1.MachineSet{createMS("ms1", "v1", 2)},
			upToDateResults: map[string]mdutil.UpToDateResult{"ms1": {EligibleForInPlaceUpdate: true}},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms1": {canUpdateMachineSet: true, affectsAvailability: false, allowedOperations: replaceNotAllowed},
			},
			expectInPlaceUpdateCandidates:                         []string{"ms1"},
			expectInPlaceUpdateCandidatesNotAffectingAvailability: []string{"ms1"},
			expectedNotes: map[string][]string{
				"ms1": {"should scale down by moving Machines to MachineSet ms-new (does not affect availability)"},
			},
		},
		{
			name:  "multiple oldMSs, mixed use cases",
			md:    createMD("v2", 10),
			newMS: createMS("ms-new", "v2", 1),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1), // not eligible
				createMS("ms2", "v1", 1), // pending
				createMS("ms3", "v1", 1), // blocked
				createMS("ms4", "v1", 1), // replace, in-place update not allowed
				createMS("ms5", "v1", 1), // in-place, affects availability
				createMS("ms6", "v1", 1), // in-place, does not affect availability
				createMS("ms7", "v1", 0), // no replicas
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: false},
				"ms2": {EligibleForInPlaceUpdate: true},
				"ms3": {EligibleForInPlaceUpdate: true},
				"ms4": {EligibleForInPlaceUpdate: true},
				"ms5": {EligibleForInPlaceUpdate: true},
				"ms6": {EligibleForInPlaceUpdate: true},
				"ms7": {EligibleForInPlaceUpdate: true},
			},
			canUpdateResults: map[string]canUpdateMachineSetResult{
				"ms2": {retryAfter: now.Add(1 * time.Minute), retryAfterFromExtension: true},
				"ms3": {allowedOperations: nothingAllowed, retryAfter: now.Add(5 * time.Minute)},
				"ms4": {canUpdateMachineSet: true, allowedOperations: inPlaceUpdateNotAllowed},
				"ms5": {canUpdateMachineSet: true, affectsAvailability: true},
				"ms6": {canUpdateMachineSet: true, affectsAvailability: false},
			},
			expectPendingMSs:              []string{"ms2"},
			expectBlockedMSs:              []string{"ms3"},
			expectReplaceCandidates:       []string{"ms1", "ms4"},
			expectInPlaceUpdateCandidates: []string{"ms5", "ms6"},
			expectInPlaceUpdateCandidatesNotAffectingAvailability: []string{"ms6"},
			expectPendingRetryAfter:                               1 * time.Minute,
			expectBlockedRetryAfter:                               5 * time.Minute,
			expectedNotes: map[string][]string{
				"ms2": {"rollout paused, waiting for the extension to take a decision about how to roll out changes (retry after 1m0s)"},
				"ms3": {"rollout blocked, neither replace nor in-place update operations are allowed"},
				"ms4": {"in-place update is possible but not allowed, falling back to replace machines"},
				"ms5": {"should scale down by moving Machines to MachineSet ms-new (affects availability)"},
				"ms6": {"should scale down by moving Machines to MachineSet ms-new (does not affect availability)"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, !tt.inPlaceUpdatesOff)

			p := &rolloutPlanner{
				md:              tt.md,
				newMS:           tt.newMS,
				oldMSs:          tt.oldMSs,
				upToDateResults: tt.upToDateResults,
				scaleIntents:    map[string]int32{},
				notes:           map[string][]string{},
				overrideNowFunc: func() time.Time { return now },
				overrideCanUpdateMachineSetInPlace: func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					res, ok := tt.canUpdateResults[oldMS.Name]
					if !ok {
						return canUpdateMachineSetResult{}, fmt.Errorf("unexpected call to CanUpdateMachineSet for MachineSet %s", oldMS.Name)
					}
					return res, nil
				},
			}
			g.Expect(p.canUpdateMachineSetInPlaceDecisions(ctx)).To(Succeed())

			if tt.expectNoSetsInitted {
				g.Expect(p.pendingMSs).To(BeNil())
				g.Expect(p.blockedMSs).To(BeNil())
				g.Expect(p.replaceCandidates).To(BeNil())
				g.Expect(p.inPlaceUpdateCandidates).To(BeNil())
				g.Expect(p.inPlaceUpdateCandidatesNotAffectingAvailability).To(BeNil())
			} else {
				g.Expect(p.pendingMSs).ToNot(BeNil())
				g.Expect(p.blockedMSs).ToNot(BeNil())
				g.Expect(p.replaceCandidates).ToNot(BeNil())
				g.Expect(p.inPlaceUpdateCandidates).ToNot(BeNil())
				g.Expect(p.inPlaceUpdateCandidatesNotAffectingAvailability).ToNot(BeNil())
			}
			g.Expect(sets.List(p.pendingMSs)).To(ConsistOf(tt.expectPendingMSs))
			g.Expect(sets.List(p.blockedMSs)).To(ConsistOf(tt.expectBlockedMSs))
			g.Expect(sets.List(p.replaceCandidates)).To(ConsistOf(tt.expectReplaceCandidates))
			g.Expect(sets.List(p.inPlaceUpdateCandidates)).To(ConsistOf(tt.expectInPlaceUpdateCandidates))
			g.Expect(sets.List(p.inPlaceUpdateCandidatesNotAffectingAvailability)).To(ConsistOf(tt.expectInPlaceUpdateCandidatesNotAffectingAvailability))

			if tt.expectPendingRetryAfter == 0 {
				g.Expect(p.pendingRetryAfter.IsZero()).To(BeTrue())
			} else {
				g.Expect(p.pendingRetryAfter).To(Equal(now.Add(tt.expectPendingRetryAfter)))
			}
			if tt.expectBlockedRetryAfter == 0 {
				g.Expect(p.blockedRetryAfter.IsZero()).To(BeTrue())
			} else {
				g.Expect(p.blockedRetryAfter).To(Equal(now.Add(tt.expectBlockedRetryAfter)))
			}

			g.Expect(p.notes).To(Equal(tt.expectedNotes))
		})
	}
}

func TestReconcileNewMachineSet(t *testing.T) {
	testCases := []struct {
		name                          string
		machineDeployment             *clusterv1.MachineDeployment
		newMachineSet                 *clusterv1.MachineSet
		oldMachineSets                []*clusterv1.MachineSet
		expectedNewMachineSetReplicas int
		expectedNotes                 []string
		// pendingMSs, blockedMSs, replaceCandidates, inPlaceUpdateCandidates and inPlaceUpdateCandidatesNotAffectingAvailability
		// are the outcome of canUpdateMachineSetInPlaceDecisions, and determine if maxSurge can be used.
		pendingMSs                                      sets.Set[string]
		blockedMSs                                      sets.Set[string]
		replaceCandidates                               sets.Set[string]
		inPlaceUpdateCandidates                         sets.Set[string]
		inPlaceUpdateCandidatesNotAffectingAvailability sets.Set[string]
		error                                           error
	}{
		{
			name:                          "Scale up: 0 -> 2",
			machineDeployment:             createMD("v2", 2, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms2", "v2", 0),
			expectedNewMachineSetReplicas: 2,
			expectedNotes:                 []string{"0 current Machines < 2 MachineDeployment spec.replicas + 2 maxSurge"},
		},
		{
			name:                          "Scale down: 2 -> 0",
			machineDeployment:             createMD("v2", 0, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms2", "v2", 2),
			expectedNewMachineSetReplicas: 0,
			expectedNotes:                 []string{"scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas"},
		},
		{
			name:                          "Scale up does not go above maxSurge (3+2)",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms2", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 3)},
			expectedNewMachineSetReplicas: 2,
			expectedNotes:                 []string{"4 current Machines < 3 MachineDeployment spec.replicas + 2 maxSurge"},
		},
		{
			name:              "Scale up accounts for deleting Machines to honour maxSurge",
			machineDeployment: createMD("v2", 1, withRollingUpdateStrategy(0, 0)),
			newMachineSet:     createMS("ms2", "v2", 0),
			// Note: The Machine on ms1 is not yet deleted.
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 0, withStatusReplicas(1))},
			expectedNewMachineSetReplicas: 0,
		},
		{
			name:              "Rolling Updated Cleanup disable machine create annotation",
			machineDeployment: createMD("v2", 2, withRollingUpdateStrategy(0, 0)),
			newMachineSet: createMS("ms2", "v2", 2,
				withMSAnnotation(clusterv1.DisableMachineCreateAnnotation, "true"),
				withMSAnnotation(clusterv1.DesiredReplicasAnnotation, "2"),
				withMSAnnotation(clusterv1.MaxReplicasAnnotation, "2"),
			),
			expectedNewMachineSetReplicas: 2,
		},
		{
			name:                          "Scale down when the rollout is paused waiting for a decision from the extension: 3 -> 2",
			machineDeployment:             createMD("v2", 2, withRollingUpdateStrategy(1, 0)),
			newMachineSet:                 createMS("ms2", "v2", 3, withStatusReplicas(3)),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1, withStatusReplicas(1))},
			pendingMSs:                    sets.New("ms1"),
			expectedNewMachineSetReplicas: 2,
			expectedNotes:                 []string{"scale down to align MachineSet spec.replicas to MachineDeployment spec.replicas"},
		},
		{
			name:                          "Do not use maxSurge to scale up when the rollout is paused waiting for a decision from the extension",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMachineSet:                 createMS("ms2", "v2", 1, withStatusReplicas(1)),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 2, withStatusReplicas(2))},
			pendingMSs:                    sets.New("ms1"),
			expectedNewMachineSetReplicas: 1,
			expectedNotes:                 []string{"Rollout paused for MachineSet ms1, maxSurge cannot be used for ms2"},
		},
		{
			name:                          "Scale up to align to spec.replicas, without using maxSurge, when the rollout is paused waiting for a decision from the extension: 1 -> 3",
			machineDeployment:             createMD("v2", 5, withRollingUpdateStrategy(1, 0)),
			newMachineSet:                 createMS("ms2", "v2", 1, withStatusReplicas(1)),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 2, withStatusReplicas(2))},
			pendingMSs:                    sets.New("ms1"),
			expectedNewMachineSetReplicas: 3,
			expectedNotes: []string{
				"Rollout paused for MachineSet ms1, maxSurge cannot be used for ms2",
				"3 current Machines < 5 MachineDeployment spec.replicas + 0 maxSurge",
			},
		},
		{
			name:                          "maxSurge limit: rollout paused wins over in-place candidates and blocked oldMSs",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			pendingMSs:                    sets.New("ms1"),
			blockedMSs:                    sets.New("ms2"),
			inPlaceUpdateCandidates:       sets.New("ms2"),
			expectedNewMachineSetReplicas: 1,
			expectedNotes:                 []string{"Rollout paused for MachineSet ms1, maxSurge cannot be used for ms3"},
		},
		{
			name:                          "maxSurge limit: all the oldMSs are blocked, maxSurge cannot be used",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			blockedMSs:                    sets.New("ms1", "ms2"),
			expectedNewMachineSetReplicas: 1,
			expectedNotes:                 []string{"Rollout blocked for MachineSet ms1,ms2, maxSurge cannot be used for ms3"},
		},
		{
			name:                          "maxSurge limit: not all the oldMSs are blocked (there are replace candidates), maxSurge can be used",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			blockedMSs:                    sets.New("ms1"),
			replaceCandidates:             sets.New("ms2"),
			expectedNewMachineSetReplicas: 3,
			expectedNotes:                 []string{"3 current Machines < 3 MachineDeployment spec.replicas + 2 maxSurge"},
		},
		{
			name:                          "maxSurge limit: not all the oldMSs are blocked (there are in-place candidates), maxSurge is limited by the in-place candidates",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 1)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			blockedMSs:                    sets.New("ms1"),
			inPlaceUpdateCandidates:       sets.New("ms2"),
			expectedNewMachineSetReplicas: 1,
			expectedNotes:                 []string{"MachineSet ms2 can rollout in place affecting availability, maxUnavailable is greater than zero, maxSurge is not required for ms3"},
		},
		{
			name:                    "maxSurge limit: in-place candidates not affecting availability, maxSurge is not required",
			machineDeployment:       createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:           createMS("ms3", "v2", 1),
			oldMachineSets:          []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			inPlaceUpdateCandidates: sets.New("ms1", "ms2"),
			inPlaceUpdateCandidatesNotAffectingAvailability: sets.New("ms1"),
			expectedNewMachineSetReplicas:                   1,
			expectedNotes:                                   []string{"MachineSet ms1 can rollout in place without affecting availability, maxSurge is not required for ms3"},
		},
		{
			name:                          "maxSurge limit: in-place candidates affecting availability and maxUnavailable greater than zero, maxSurge is not required",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 1)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			inPlaceUpdateCandidates:       sets.New("ms1", "ms2"),
			expectedNewMachineSetReplicas: 1,
			expectedNotes:                 []string{"MachineSet ms1,ms2 can rollout in place affecting availability, maxUnavailable is greater than zero, maxSurge is not required for ms3"},
		},
		{
			name:                          "maxSurge limit: in-place candidates affecting availability and maxUnavailable zero, maxSurge is limited to 1",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			inPlaceUpdateCandidates:       sets.New("ms1", "ms2"),
			expectedNewMachineSetReplicas: 2,
			expectedNotes: []string{
				"MachineSet ms1,ms2 can rollout in place affecting availability, maxSurge limit set to 1 for ms3",
				"3 current Machines < 3 MachineDeployment spec.replicas + 1 maxSurge",
			},
		},
		{
			name:                          "maxSurge limit: only replace candidates, maxSurge can be used in full",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			replaceCandidates:             sets.New("ms1", "ms2"),
			expectedNewMachineSetReplicas: 3,
			expectedNotes:                 []string{"3 current Machines < 3 MachineDeployment spec.replicas + 2 maxSurge"},
		},
		{
			name:                          "maxSurge limit: no decisions (e.g. InPlaceUpdates feature gate off), maxSurge can be used in full",
			machineDeployment:             createMD("v2", 3, withRollingUpdateStrategy(2, 0)),
			newMachineSet:                 createMS("ms3", "v2", 1),
			oldMachineSets:                []*clusterv1.MachineSet{createMS("ms1", "v1", 1), createMS("ms2", "v1", 1)},
			expectedNewMachineSetReplicas: 3,
			expectedNotes:                 []string{"3 current Machines < 3 MachineDeployment spec.replicas + 2 maxSurge"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			planner := newRolloutPlanner(nil, nil, nil)
			planner.md = tc.machineDeployment
			planner.newMS = tc.newMachineSet
			planner.oldMSs = tc.oldMachineSets
			planner.pendingMSs = tc.pendingMSs
			planner.blockedMSs = tc.blockedMSs
			planner.replaceCandidates = tc.replaceCandidates
			planner.inPlaceUpdateCandidates = tc.inPlaceUpdateCandidates
			planner.inPlaceUpdateCandidatesNotAffectingAvailability = tc.inPlaceUpdateCandidatesNotAffectingAvailability

			err := planner.reconcileNewMachineSet(ctx)
			if tc.error != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(BeEquivalentTo(tc.error.Error()))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			scaleIntent := ptr.Deref(tc.newMachineSet.Spec.Replicas, 0)
			if v, ok := planner.scaleIntents[tc.newMachineSet.Name]; ok {
				scaleIntent = v
			}
			g.Expect(scaleIntent).To(BeEquivalentTo(tc.expectedNewMachineSetReplicas))
			g.Expect(planner.notes[tc.newMachineSet.Name]).To(Equal(tc.expectedNotes), "unexpected notes")
		})
	}
}

func Test_reconcileOldMachineSetsRollingUpdate(t *testing.T) {
	var ctx = context.Background()

	tests := []struct {
		name                       string
		md                         *clusterv1.MachineDeployment
		scaleIntent                map[string]int32
		newMS                      *clusterv1.MachineSet
		oldMSs                     []*clusterv1.MachineSet
		machines                   []*clusterv1.Machine
		expectScaleIntent          map[string]int32
		expectedNotes              map[string][]string
		skipMaxUnavailabilityCheck bool
		// pendingMSs, blockedMSs, inPlaceUpdateCandidates and inPlaceUpdateCandidatesNotAffectingAvailability are the outcome of canUpdateMachineSetInPlaceDecisions.
		pendingMSs                                      sets.Set[string]
		blockedMSs                                      sets.Set[string]
		inPlaceUpdateCandidates                         sets.Set[string]
		inPlaceUpdateCandidatesNotAffectingAvailability sets.Set[string]
		// expectMoveToNewMS contains the oldMSs that must get the move annotation, with the expected affectsAvailability value.
		expectMoveToNewMS    map[string]bool
		expectReceiveFromMSs string
	}{
		{
			name:        "no op if there are no replicas on old machinesets",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 10, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 10, withStatusReplicas(10), withStatusAvailableReplicas(10)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 0, withStatusReplicas(0), withStatusAvailableReplicas(0)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "do not scale down if replicas is equal to minAvailable replicas",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 2 available replicas from ms1 + 1 available replica from ms2 = 3 available replicas == minAvailability, we cannot scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "do not scale down if replicas is less then minAvailable replicas",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(1)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 1 available replicas from ms1 + 1 available replica from ms2 = 2 available replicas < minAvailability, we cannot scale down
			},
			skipMaxUnavailabilityCheck: true,
			expectedNotes:              map[string][]string{},
		},
		{
			name:        "do not scale down if there are more replicas than minAvailable replicas, but scale down from a previous reconcile already takes the availability buffer",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(3), withStatusAvailableReplicas(3)), // OldMS is scaling down from a previous reconcile
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 3 available replicas from ms1 - 1 replica already scaling down from ms1 + 1 available replica from ms2 = 3 available replicas == minAvailability, we cannot further scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "do not scale down if there are more replicas than minAvailable replicas, and some of them is updating in-place without affecting availability", // The system should slow down the rollout in this case
			scaleIntent: map[string]int32{},
			md:          createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms3", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
				createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)), // MS with a machine updating in place (does not affect availability)
			},
			machines: []*clusterv1.Machine{ // in this test it is required to define machines because the test is checking a code path validating UpdateInProgressAnnotation on Machine
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
				createM("m3", "ms2", "v2", withMAnnotation(clusterv1.UpdateInProgressAnnotation, "{\"affectsAvailability\":false}")),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1,  ms2):
				// 3 available replicas, one of them updating in place (does not affect availability), we cannot further scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "do not scale down if there are more replicas than minAvailable replicas, but scale down from a previous reconcile already takes the availability buffer, scale down from a previous reconcile on another MS",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 6, withRollingUpdateStrategy(3, 1)),
			newMS:       createMS("ms3", "v2", 3, withStatusReplicas(0), withStatusAvailableReplicas(0)), // NewMS is scaling up from previous reconcile, but replicas do not exist yet
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(3), withStatusAvailableReplicas(3)), // OldMS is scaling down from a previous reconcile
				createMS("ms2", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)), // OldMS
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 3 available replicas from ms1 - 1 replica already scaling down from ms1 + 3 available replica from ms2 = 5 available replicas == minAvailability, we cannot further scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "do not scale down if there are unavailable replicas on the new MachineSet (maxSurge 1, maxUnavailable 0), all  machines unavailable",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // one machine created on the new NewMS due to maxSurge, but it is not reaching the available state,
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(0)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1): unavailability on new MS
			},
			expectedNotes:              map[string][]string{},
			skipMaxUnavailabilityCheck: true, // The test case is simulating all machines not unavailable, so this check will fail
		},
		{
			name:        "do not scale down if there are unavailable replicas on the new MachineSet (maxSurge 0, maxUnavailable 1), all  machines unavailable",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(0, 1)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // one machine created on the new NewMS after scale down on the old MS, but it is not reaching the available state,
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(0)), // OldMS scaled down due to maxUnavailable 1
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1): unavailability on new MS
			},
			expectedNotes:              map[string][]string{},
			skipMaxUnavailabilityCheck: true, // The test case is simulating all machines not unavailable, so this check will fail
		},
		{
			name: "do not scale down if there are more replicas than minAvailable replicas, but scale down from current reconcile already takes the availability buffer (newMS is scaling down)",
			scaleIntent: map[string]int32{
				"ms2": 1, // newMS (ms2) has a scaling down intent from current reconcile
			},
			md:    createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS: createMS("ms2", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			expectScaleIntent: map[string]int32{
				"ms2": 1,
				// no new scale down intent for oldMSs (ms1):
				// 2 available replicas from ms1 + 2 available replicas from ms2 - 1 replica already scaling down from ms2 = 3 available replicas == minAvailability, we cannot further scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name: "do not scale down if there are more replicas than minAvailable replicas, but scale down from current reconcile already takes the availability buffer (oldMS is scaling down)",
			scaleIntent: map[string]int32{
				"ms1": 1, // oldMS (ms1) has a scaling down intent from current reconcile
			},
			md:    createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS: createMS("ms2", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs:
				"ms1": 1,
				// 2 available replicas from ms1 - 1 replica already scaling down from ms1 + 2 available replicas from ms2 = 3 available replicas == minAvailability, we cannot further scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "do not scale down replicas when there are more replicas than minAvailable replicas, but not all the replicas are available (unavailability on newMS)",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 3 available replicas from ms1 + 0 available replicas from ms2 = 3 available replicas == minAvailability, we cannot further scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "do not scale down replicas when there are more replicas than minAvailable replicas, but not all the replicas are available (unavailability on oldMS)",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(2)), // only 2 replicas are available
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 2 available replicas from ms1 + 1 available replicas from ms2 = 3 available replicas == minAvailability, we cannot further scale down
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, all replicas are available",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs (ms1):
				"ms1": 2, // 3 available replicas from ms1 + 1 available replicas from ms2 = 4 available replicas > minAvailability, scale down to 2 replicas (-1)
			},
			expectedNotes: map[string][]string{
				"ms1": {"4 available replicas > 3 minimum available replicas"},
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
				createMS("ms2", "v1", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available
				createMS("ms3", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(0)), // no replicas are available
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				// ms1 skipped in the first iteration because it does not have any unavailable replica
				"ms2": 0, // 0 available replicas from ms2, it can be scaled down without impact on availability (-1)
				"ms3": 0, // 0 available replicas from ms3, it can be scaled down without impact on availability (-2)
				// no need to further scale down.
			},
			expectedNotes: map[string][]string{
				"ms2": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
				"ms3": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, available replicas are scaled down when unavailable replicas are gone",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
				createMS("ms2", "v1", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available
				createMS("ms3", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(0)), // no replicas are available
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				// ms1 skipped in the first iteration because it does not have any unavailable replica
				"ms2": 0, // 0 available replicas from ms2, it can be scaled down to 0 without any impact on availability (-1)
				"ms3": 0, // 0 available replicas from ms3, it can be scaled down to 0 without any impact on availability (-2)
				"ms1": 2, // 3 available replicas from ms1 + 1 available replica from ms4 = 4 available replicas > minAvailability, scale down to 2 replicas (-1)
			},
			expectedNotes: map[string][]string{
				"ms1": {"4 available replicas > 3 minimum available replicas"},
				"ms2": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
				"ms3": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, available replicas are scaled down when unavailable replicas are gone is not affected by replicas without machines",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 4, withStatusReplicas(3), withStatusAvailableReplicas(3)), // 1 replica without machine
				createMS("ms2", "v1", 2, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available, 1 replica without machine
				createMS("ms3", "v2", 5, withStatusReplicas(2), withStatusAvailableReplicas(0)), // no replicas are available, 3 replicas without machines
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				// ms1 skipped in the first iteration because it does not have any unavailable replica
				"ms2": 0, // 0 available replicas from ms2, it can be scaled down to 0 without any impact on availability (-2)
				"ms3": 0, // 0 available replicas from ms3, it can be scaled down to 0 without any impact on availability (-5)
				"ms1": 2, // 3 available replicas from ms1 + 1 available replica from ms4 = 4 available replicas > minAvailability, scale down to 2 replicas, also drop the replica without machine (-2)
			},
			expectedNotes: map[string][]string{
				"ms1": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)", "4 available replicas > 3 minimum available replicas"},
				"ms2": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
				"ms3": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, scale down stops before breaching minAvailable replicas",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
				createMS("ms2", "v1", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available
				createMS("ms3", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(1)), // only 1 replica is available
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				"ms2": 0, // 0 available replicas from ms2, it can be scaled down to 0 without any impact on availability (-1)
				// even if there is still room to scale down, we cannot scale down ms3: 1 available replica from ms1 + 1 available replica from ms4 = 2 available replicas < minAvailability
				// does not make sense to continue scale down as there is no guarantee that MS3 would remove the unavailable replica
			},
			expectedNotes: map[string][]string{
				"ms2": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, scale down stops before breaching minAvailable replicas is not affected by replicas without machines",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 2, withStatusReplicas(1), withStatusAvailableReplicas(1)), // 1 replica without machine
				createMS("ms2", "v1", 3, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available, 2 replica without machines
				createMS("ms3", "v2", 3, withStatusReplicas(2), withStatusAvailableReplicas(1)), // only 1 replica is available, 1 replica without machine
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				"ms1": 1, // 1 replica without machine, it can be scaled down to 1 without any impact on availability (-1)
				"ms2": 0, // 2 replica without machine, 0 available replicas from ms2, it can be scaled down to 0 without any impact on availability (-3)
				"ms3": 2, // 1 replica without machine, it can be scaled down to 2 without any impact on availability (-1); even if there is still room to scale down, we cannot further scale down ms3: 1 available replica from ms1 + 1 available replica from ms4 = 2 available replicas < minAvailability
				// does not make sense to continue scale down.
			},
			expectedNotes: map[string][]string{
				"ms1": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
				"ms2": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
				"ms3": {"scale down to align to existing Machines or scale down by removing unavailable replicas (and no available replicas exist on the MachineSet)"},
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, scale down keeps into account scale downs from a previous reconcile",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v3", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 3, withStatusReplicas(4), withStatusAvailableReplicas(4)), // OldMS is scaling down from a previous reconcile
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				"ms1": 2, // 4 available replicas from ms1 - 1 replica already scaling down from ms1 + 2 available replicas from ms2 = 5 available replicas > minAvailability, scale down to 2 (-1)
			},
			expectedNotes: map[string][]string{
				"ms1": {"6 available replicas > 3 minimum available replicas"},
			},
		},
		{
			name: "scale down replicas when there are more replicas than minAvailable replicas, scale down keeps into account scale downs from the current reconcile (newMS is scaling down)",
			scaleIntent: map[string]int32{
				"ms2": 1, // newMS (ms2) has a scaling down intent from current reconcile
			},
			md:    createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS: createMS("ms2", "v3", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
			},
			expectScaleIntent: map[string]int32{
				"ms2": 1,
				// new scale down intent for oldMSs:
				"ms1": 2, // 3 available replicas from ms1 + 2 available replicas from ms2 - 1 replica already scaling down from ms2 = 4 available replicas > minAvailability, scale down to 2 (-1)
			},
			expectedNotes: map[string][]string{
				"ms1": {"5 available replicas > 3 minimum available replicas"},
			},
		},
		{
			name: "scale down replicas when there are more replicas than minAvailable replicas, scale down keeps into account scale downs from the current reconcile (oldMS is scaling down)",
			scaleIntent: map[string]int32{
				"ms1": 2, // oldMS (ms1) has a scaling down intent from current reconcile
			},
			md:    createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS: createMS("ms2", "v3", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				"ms1": 1, // 3 available replicas from ms1 - 1 replica already scaling down from ms1 + 2 available replicas from ms2 = 4 available replicas > minAvailability, scale down to 1 (-1)
			},
			expectedNotes: map[string][]string{
				"ms1": {"5 available replicas > 3 minimum available replicas"},
			},
		},
		{
			name:        "set the move annotations for oldMSs updated in-place affecting availability",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			inPlaceUpdateCandidates: sets.New("ms1"),
			expectScaleIntent:       map[string]int32{},
			expectMoveToNewMS:       map[string]bool{"ms1": true},
			expectReceiveFromMSs:    "ms1",
			expectedNotes:           map[string][]string{},
		},
		{
			name:        "set the move annotations for oldMSs updated in-place not affecting availability",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			inPlaceUpdateCandidates:                         sets.New("ms1"),
			inPlaceUpdateCandidatesNotAffectingAvailability: sets.New("ms1"),
			expectScaleIntent:                               map[string]int32{"ms1": 1},
			expectMoveToNewMS:                               map[string]bool{"ms1": false},
			expectReceiveFromMSs:                            "ms1",
			expectedNotes: map[string][]string{
				"ms1": {"scale down for MachineSet ms1 to 1 replicas (-1) allowed for in place update not affecting availability"},
			},
			skipMaxUnavailabilityCheck: true, // The Machine is moved to be updated in-place without affecting availability, even if there is no availability buffer.
		},
		{
			name:        "set the move annotations also if the oldMS spec.replicas is already 0 but Machines still have to be moved",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 0, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			},
			inPlaceUpdateCandidates: sets.New("ms1"),
			expectScaleIntent:       map[string]int32{},
			expectMoveToNewMS:       map[string]bool{"ms1": true},
			expectReceiveFromMSs:    "ms1",
			expectedNotes:           map[string][]string{},
		},
		{
			name:        "do not set the move annotations if the rollout is paused waiting for a decision from the extension",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms3", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
				createMS("ms2", "v1", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			},
			pendingMSs:              sets.New("ms2"),
			inPlaceUpdateCandidates: sets.New("ms1"),
			expectScaleIntent:       map[string]int32{},
			expectedNotes:           map[string][]string{},
		},
		{
			name:        "scale down excess replicas, oldest MachineSet first, if the rollout is paused waiting for a decision from the extension",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)), // e.g. spec.replicas lowered from 6 to 3
			newMS:       createMS("ms3", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
				createMS("ms2", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			pendingMSs: sets.New("ms2"),
			expectScaleIntent: map[string]int32{
				"ms1": 1, // 1+3+2=6 replicas > 3 spec.replicas + 1 maxSurge, scale down by 2
			},
			expectedNotes: map[string][]string{
				"ms1": {"6 current Machines > 3 MachineDeployment spec.replicas + 1 maxSurge"},
			},
		},
		{
			name:        "scale down excess replicas also on pending and blocked oldMSs, taking into account scale down intents already set, if the rollout is paused",
			scaleIntent: map[string]int32{"ms3": 0},
			md:          createMD("v2", 2, withRollingUpdateStrategy(0, 1)),
			newMS:       createMS("ms3", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
				createMS("ms2", "v1", 4, withStatusReplicas(4), withStatusAvailableReplicas(4)),
			},
			pendingMSs: sets.New("ms2"),
			blockedMSs: sets.New("ms1"),
			expectScaleIntent: map[string]int32{
				"ms3": 0,
				"ms1": 0, // 0+2+4=6 replicas > 2 spec.replicas + 0 maxSurge, scale down by 4
				"ms2": 2,
			},
			expectedNotes: map[string][]string{
				"ms1": {"6 current Machines > 2 MachineDeployment spec.replicas + 0 maxSurge"},
				"ms2": {"6 current Machines > 2 MachineDeployment spec.replicas + 0 maxSurge"},
			},
			skipMaxUnavailabilityCheck: true, // The scale down is requested by the user, minAvailable is not considered.
		},
		{
			name:        "do not scale down if the rollout is paused waiting for a decision from the extension and replicas do not exceed spec.replicas + maxSurge",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
			},
			pendingMSs:        sets.New("ms1"),
			expectScaleIntent: map[string]int32{},
			expectedNotes:     map[string][]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &rolloutPlanner{
				md:                      tt.md,
				newMS:                   tt.newMS,
				oldMSs:                  tt.oldMSs,
				machines:                tt.machines,
				scaleIntents:            tt.scaleIntent,
				notes:                   make(map[string][]string),
				pendingMSs:              tt.pendingMSs,
				blockedMSs:              tt.blockedMSs,
				inPlaceUpdateCandidates: tt.inPlaceUpdateCandidates,
				inPlaceUpdateCandidatesNotAffectingAvailability: tt.inPlaceUpdateCandidatesNotAffectingAvailability,
			}
			err := p.reconcileOldMachineSetsRollingUpdate(ctx)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(p.scaleIntents).To(Equal(tt.expectScaleIntent), "unexpected scaleIntents")

			// Check we are not breaching minAvailability by simulating what will happen by applying intent + worst scenario when a machine deletion is always an available machine deletion.
			for _, oldMS := range tt.oldMSs {
				scaleIntent, ok := p.scaleIntents[oldMS.Name]
				if !ok {
					continue
				}
				machineScaleDown := max(ptr.Deref(oldMS.Status.Replicas, 0)-scaleIntent, 0)
				if machineScaleDown > 0 {
					oldMS.Status.AvailableReplicas = new(max(ptr.Deref(oldMS.Status.AvailableReplicas, 0)-machineScaleDown, 0))
				}
			}
			minAvailableReplicas := ptr.Deref(tt.md.Spec.Replicas, 0) - mdutil.MaxUnavailable(*tt.md)
			totAvailableReplicas := ptr.Deref(mdutil.GetAvailableReplicaCountForMachineSets(append(tt.oldMSs, tt.newMS)), 0)
			if !tt.skipMaxUnavailabilityCheck {
				g.Expect(totAvailableReplicas).To(BeNumerically(">=", minAvailableReplicas), "totAvailable machines is less than minAvailable")
			} else {
				t.Logf("skipping MaxUnavailability check (totAvailableReplicas: %d, minAvailableReplicas: %d)", totAvailableReplicas, minAvailableReplicas)
			}
			g.Expect(p.notes).To(Equal(tt.expectedNotes), "unexpected notes")

			for _, oldMS := range tt.oldMSs {
				affectsAvailability, ok := tt.expectMoveToNewMS[oldMS.Name]
				if !ok {
					g.Expect(oldMS.Annotations).ToNot(HaveKey(clusterv1.MachineSetMoveMachinesToMachineSetAnnotation))
					continue
				}
				expectedData, err := json.Marshal(clusterv1.MachineSetMoveMachinesToMachineSetAnnotationData{
					Name:                tt.newMS.Name,
					AffectsAvailability: new(affectsAvailability),
				})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(oldMS.Annotations).To(HaveKeyWithValue(clusterv1.MachineSetMoveMachinesToMachineSetAnnotation, string(expectedData)))
			}
			if tt.expectReceiveFromMSs == "" {
				g.Expect(tt.newMS.Annotations).ToNot(HaveKey(clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation))
			} else {
				g.Expect(tt.newMS.Annotations).To(HaveKeyWithValue(clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation, tt.expectReceiveFromMSs))
			}
		})
	}
}

func Test_reconcileDeadlockBreaker(t *testing.T) {
	var ctx = context.Background()

	tests := []struct {
		name                       string
		scaleIntent                map[string]int32
		newMS                      *clusterv1.MachineSet
		oldMSs                     []*clusterv1.MachineSet
		expectScaleIntent          map[string]int32
		expectedNotes              map[string][]string
		skipMaxUnavailabilityCheck bool

		pendingMSs sets.Set[string]
		blockedMSs sets.Set[string]
	}{
		{
			name:        "no op if there are no replicas on old machinesets",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms2", "v2", 10, withStatusReplicas(10), withStatusAvailableReplicas(10)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 0, withStatusReplicas(0), withStatusAvailableReplicas(0)), // there no replicas, not a deadlock, we are actually at desired state
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "no op if all the replicas on OldMS are available",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)), // there no unavailable replicas, not a deadlock (rollout will continue as usual)
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "no op if there are scale operation still in progress from a previous reconcile on newMS",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms2", "v2", 6, withStatusReplicas(5), withStatusAvailableReplicas(5)), // scale up from a previous reconcile
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // there is at least one unavailable replica, not yet considered deadlock because scale operation still in progress
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "no op if there are scale operation still in progress from a previous reconcile on oldMS",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 4, withStatusReplicas(5), withStatusAvailableReplicas(4)), // there is at least one unavailable replica, not yet considered deadlock because scale operation still in progress
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
			},
			expectedNotes: map[string][]string{},
		},
		{
			name: "no op if there are scale operation from the current reconcile on newMS",
			scaleIntent: map[string]int32{
				"ms2": 6, // scale up intent for newMS
			},
			newMS: createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // there is at least one unavailable replica, not yet considered deadlock because scale operation still in progress
			},
			expectScaleIntent: map[string]int32{
				"ms2": 6,
				// no new scale down intent for oldMSs (ms1):
			},
			expectedNotes: map[string][]string{},
		},
		{
			name: "no op if there are scale operation from the current reconcile on oldMS",
			scaleIntent: map[string]int32{
				"ms1": 4, // scale up intent for oldMS
			},
			newMS: createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // there is at least one unavailable replica, not yet considered deadlock because scale operation still in progress
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				"ms1": 4,
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "wait for unavailable replicas on the newMS if any",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(3)), // one unavailable replica
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // there is at least one unavailable replica, no scale operations in progress, potential deadlock, but the system must wait until all replica on newMS are available before unblocking further deletion on oldMS.
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
			},
			expectedNotes: map[string][]string{},
		},
		{
			name:        "unblock a deadlock when necessary",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(3)), // there is at least one unavailable replica, all replicas on newMS available, no scale operations in progress, deadlock
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs (ms1):
				"ms1": 4,
			},
			expectedNotes: map[string][]string{
				"ms2": {"scaling down by 1 to unblock rollout stuck due to unavailable Machine on oldMS"},
			},
		},
		{
			name:        "no op if the rollout is paused waiting for a decision from the extension, even if there is a deadlock",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(3)), // same as "unblock a deadlock when necessary", but the rollout is paused
			},
			pendingMSs:        sets.New("ms1"),
			expectScaleIntent: map[string]int32{},
			expectedNotes:     map[string][]string{},
		},
		{
			name:        "no op if at least one oldMS is blocked, even if another oldMS has unavailable replicas",
			scaleIntent: map[string]int32{},
			newMS:       createMS("ms3", "v3", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(3)), // same as "unblock a deadlock when necessary"
				createMS("ms2", "v2", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)), // blocked
			},
			blockedMSs:        sets.New("ms2"),
			expectScaleIntent: map[string]int32{},
			expectedNotes:     map[string][]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &rolloutPlanner{
				newMS:        tt.newMS,
				oldMSs:       tt.oldMSs,
				scaleIntents: tt.scaleIntent,
				notes:        make(map[string][]string),

				pendingMSs: tt.pendingMSs,
				blockedMSs: tt.blockedMSs,
			}
			p.reconcileDeadlockBreaker(ctx)
			g.Expect(p.scaleIntents).To(Equal(tt.expectScaleIntent), "unexpected scaleIntents")
			g.Expect(p.notes).To(Equal(tt.expectedNotes), "unexpected notes")
		})
	}
}

func TestRequeueAfter(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name              string
		pendingRetryAfter time.Time
		blockedRetryAfter time.Time
		want              time.Duration
	}{
		{
			name: "Do not requeue if the rollout is neither pending nor blocked",
			want: 0,
		},
		{
			name:              "Requeue when the pending cached result expires",
			pendingRetryAfter: now.Add(3 * time.Minute),
			want:              3 * time.Minute,
		},
		{
			name:              "Requeue when the blocked cached result expires",
			blockedRetryAfter: now.Add(2 * time.Minute),
			want:              2 * time.Minute,
		},
		{
			name:              "Requeue after the pending retryAfter if the rollout is pending and blocked (blocked is re-evaluated when the rollout is unpaused)",
			pendingRetryAfter: now.Add(3 * time.Minute),
			blockedRetryAfter: now.Add(1 * time.Minute),
			want:              3 * time.Minute,
		},
		{
			name:              "Requeue after at least 1s if the cached result already expired",
			pendingRetryAfter: now.Add(-1 * time.Minute),
			want:              time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &rolloutPlanner{
				pendingRetryAfter: tt.pendingRetryAfter,
				blockedRetryAfter: tt.blockedRetryAfter,
				overrideNowFunc:   func() time.Time { return now },
			}
			g.Expect(p.requeueAfter()).To(Equal(tt.want))
		})
	}
}

type rollingUpdateSequenceTestCase struct {
	name           string
	maxSurge       int32
	maxUnavailable int32

	// currentMachineNames is the list of machines before the rollout, and provides a simplified alternative to currentScope.
	// all the machines in this list are initialized as upToDate and owned by the new MS before the rollout (which is different from the new MS after the rollout).
	// Please name machines as "mX" where X is a progressive number starting from 1 (do not skip numbers),
	// e.g. "m1","m2","m3"
	currentMachineNames []string

	// currentScope defines the current state at the beginning of the test case.
	// When the test case start from a stable state (there are no previous rollout in progress), use  currentMachineNames instead.
	// Please name machines as "mX" where X is a progressive number starting from 1 (do not skip numbers),
	// e.g. "m1","m2","m3"
	// machineUID must be set to the last used number.
	currentScope *rolloutScope

	// maxUnavailableBreachToleration can be used to temporarily silence MaxUnavailable breaches
	//
	// maxUnavailableBreachToleration: func(log *logger, i int, scope *rolloutScope, minAvailableReplicas, totAvailableReplicas int32) bool {
	// 		if i == 5 {
	// 			t.Log("[Toleration] tolerate minAvailable breach after scale up")
	// 			return true
	// 		}
	// 		return false
	// 	},
	maxUnavailableBreachToleration func(log *fileLogger, i int, scope *rolloutScope, minAvailableReplicas, totAvailableReplicas int32) bool

	// maxSurgeBreachToleration can be used to temporarily silence MaxSurge breaches
	// (see maxUnavailableBreachToleration example)
	maxSurgeBreachToleration func(log *fileLogger, i int, scope *rolloutScope, maxAllowedReplicas, totReplicas int32) bool

	// desiredMachineNames is the list of machines at the end of the rollout.
	// all the machines in this list are expected to be upToDate and owned by the new MS after the rollout (which is different from the new MS before the rollout).
	// if this list contains old machines names (machine names already in currentMachineNames), it implies those machine have been upgraded in places.
	// if this list contains new machines names (machine names not in currentMachineNames), it implies those machines have been created during a rollout;
	// please name new machines names as "mX" where X is a progressive number starting after the max number in currentMachineNames (do not skip numbers),
	// e.g. desiredMachineNames "m4","m5","m6" (desired machine names after a regular rollout of a MD with currentMachineNames "m1","m2","m3")
	// e.g. desiredMachineNames "m1","m2","m3" (desired machine names after rollout performed using in-place update for an MD with currentMachineNames "m1","m2","m3")
	desiredMachineNames []string

	// overrideCanUpdateMachineSetInPlaceFunc allows to inject a function that will be used to perform the canUpdateMachineSetInPlace decision
	overrideCanUpdateMachineSetInPlaceFunc func(i int) func(ctx context.Context, oldMS, newMS *clusterv1.MachineSet) (canUpdateMachineSetResult, error)

	// expectedMachinesCreated, expectedMachinesDeleted and expectedMachinesUpdatedInPlace assert the
	// total number of machine create, delete and in-place update operations performed over the whole rollout.
	expectedMachinesCreated        int
	expectedMachinesDeleted        int
	expectedMachinesUpdatedInPlace int

	// skipLogToFileAndGoldenFileCheck allows to skip storing the log to file and golden file Check.
	// NOTE: this field is controlled by the test itself.
	skipLogToFileAndGoldenFileCheck bool

	// name of the log to file and the golden file.
	// NOTE: this field is controlled by the test itself.
	logAndGoldenFileName string

	// randomControllerOrder force the tests to run controllers in random order, mimicking what happens in production.
	// NOTE. We are using a pseudo randomizer, so the random order remains consistent across runs of the same groups of tests.
	// NOTE: this field is controlled by the test itself.
	randomControllerOrder bool

	// maxIterations defines the max number of iterations the system must attempt before assuming the logic has an issue
	// in reaching the desired state.
	// When the test is using default controller order, an iteration implies reconcile MD + reconcile all MS in a predictable order;
	// while using randomControllerOrder the concept of iteration is less defined, but it can still be used to prevent
	// the test from running indefinitely.
	// NOTE: this field is controlled by the test itself.
	maxIterations int

	// seed value to initialize the generator.
	// NOTE: this field is controlled by the test itself.
	seed int64
}

func Test_RollingUpdateSequences(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)

	tests := []rollingUpdateSequenceTestCase{
		// Regular rollout (no in-place)

		{ // scale out by 1
			name:                    "Regular rollout, 3 Replicas, maxSurge 1, maxUnavailable 0",
			maxSurge:                1,
			maxUnavailable:          0,
			currentMachineNames:     []string{"m1", "m2", "m3"},
			desiredMachineNames:     []string{"m4", "m5", "m6"},
			expectedMachinesCreated: 3,
			expectedMachinesDeleted: 3,
		},
		{ // scale in by 1
			name:                    "Regular rollout, 3 Replicas, maxSurge 0, maxUnavailable 1",
			maxSurge:                0,
			maxUnavailable:          1,
			currentMachineNames:     []string{"m1", "m2", "m3"},
			desiredMachineNames:     []string{"m4", "m5", "m6"},
			expectedMachinesCreated: 3,
			expectedMachinesDeleted: 3,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable)
			name:                    "Regular rollout, 6 Replicas, maxSurge 3, maxUnavailable 1",
			maxSurge:                3,
			maxUnavailable:          1,
			currentMachineNames:     []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:     []string{"m7", "m8", "m9", "m10", "m11", "m12"},
			expectedMachinesCreated: 6,
			expectedMachinesDeleted: 6,
		},
		{ // scale out by 1, scale in by 3 (maxSurge < maxUnavailable)
			name:                    "Regular rollout, 6 Replicas, maxSurge 1, maxUnavailable 3",
			maxSurge:                1,
			maxUnavailable:          3,
			currentMachineNames:     []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:     []string{"m7", "m8", "m9", "m10", "m11", "m12"},
			expectedMachinesCreated: 6,
			expectedMachinesDeleted: 6,
		},
		{ // scale out by 10 (maxSurge >= replicas)
			name:                    "Regular rollout, 6 Replicas, maxSurge 10, maxUnavailable 0",
			maxSurge:                10,
			maxUnavailable:          0,
			currentMachineNames:     []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:     []string{"m7", "m8", "m9", "m10", "m11", "m12"},
			expectedMachinesCreated: 6,
			expectedMachinesDeleted: 6,
		},
		{ // scale in by 10 (maxUnavailable >= replicas)
			name:                    "Regular rollout, 6 Replicas, maxSurge 0, maxUnavailable 10",
			maxSurge:                0,
			maxUnavailable:          10,
			currentMachineNames:     []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:     []string{"m7", "m8", "m9", "m10", "m11", "m12"},
			expectedMachinesCreated: 6,
			expectedMachinesDeleted: 6,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale up machine deployment in the middle
			name:           "Regular rollout, 6 Replicas, maxSurge 3, maxUnavailable 1, scale up to 12",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 6 replica in the middle of a rollout, with 3 machines already created in the newMS and 3 still on the oldMS, and then MD scaled up to 12.
				machineDeployment: createMD("v2", 12, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already deleted
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						createM("m7", "ms2", "v2"),
						createM("m8", "ms2", "v2"),
						createM("m9", "ms2", "v2"),
					},
				},
				machineUID: 9,
			},
			desiredMachineNames:            []string{"m7", "m8", "m9", "m10", "m11", "m12", "m13", "m14", "m15", "m16", "m17", "m18"},
			maxUnavailableBreachToleration: maxUnavailableBreachToleration(), // after scale up it is expected to have less available machine than expected
			expectedMachinesCreated:        9,
			expectedMachinesDeleted:        3,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale down machine deployment in the middle
			name:           "Regular rollout, 12 Replicas, maxSurge 3, maxUnavailable 1, scale down to 6",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 12 replica in the middle of a rollout, with 3 machines already created in the newMS and 9 still on the oldMS, and then MD scaled down to 6.
				machineDeployment: createMD("v2", 6, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 9),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already deleted
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
						createM("m7", "ms1", "v1"),
						createM("m8", "ms1", "v1"),
						createM("m9", "ms1", "v1"),
						createM("m10", "ms1", "v1"),
						createM("m11", "ms1", "v1"),
						createM("m12", "ms1", "v1"),
					},
					"ms2": {
						createM("m13", "ms2", "v2"),
						createM("m14", "ms2", "v2"),
						createM("m15", "ms2", "v2"),
					},
				},
				machineUID: 15,
			},
			desiredMachineNames:      []string{"m13", "m14", "m15", "m16", "m17", "m18"},
			maxSurgeBreachToleration: maxSurgeToleration(), // after scale down it is expected to temporarily have more machine than expected
			expectedMachinesCreated:  3,
			expectedMachinesDeleted:  9,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + change spec in the middle
			name:           "Regular rollout, 6 Replicas, maxSurge 3, maxUnavailable 1, change spec",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD with 6 replica in the middle of a rollout, with 3 machines already created in the newMS and 3 still on the oldMS, and then MD spec is changed.
				machineDeployment: createMD("v3", 6, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already deleted
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						createM("m7", "ms2", "v2"),
						createM("m8", "ms2", "v2"),
						createM("m9", "ms2", "v2"),
					},
				},
				machineUID: 9,
			},
			desiredMachineNames:     []string{"m10", "m11", "m12", "m13", "m14", "m15"}, // NOTE: Machines created before the spec change are deleted
			expectedMachinesCreated: 6,
			expectedMachinesDeleted: 6,
		},

		// Rollout with In-place updates (affecting availability)

		{ // scale out by 1
			name:                                   "In-place rollout, 3 Replicas, maxSurge 1, MaxUnavailable 0",
			maxSurge:                               1,
			maxUnavailable:                         0,
			currentMachineNames:                    []string{"m1", "m2", "m3"},
			desiredMachineNames:                    []string{"m1", "m2", "m4"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesCreated:                1,
			expectedMachinesDeleted:                1,
			expectedMachinesUpdatedInPlace:         2,
		},
		{ // scale in by 1
			name:                                   "In-place rollout, 3 Replicas, maxSurge 0, MaxUnavailable 1",
			maxSurge:                               0,
			maxUnavailable:                         1,
			currentMachineNames:                    []string{"m1", "m2", "m3"},
			desiredMachineNames:                    []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesUpdatedInPlace:         3,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 3, MaxUnavailable 1",
			maxSurge:                               3,
			maxUnavailable:                         1,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesUpdatedInPlace:         6,
		},
		{ // scale out by 1, scale in by 3 (maxSurge < maxUnavailable)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 1, MaxUnavailable 3",
			maxSurge:                               1,
			maxUnavailable:                         3,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesUpdatedInPlace:         6,
		},
		{ // scale out by 10 (maxSurge >= replicas)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 10, MaxUnavailable 0",
			maxSurge:                               10,
			maxUnavailable:                         0,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m7"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesCreated:                1,
			expectedMachinesDeleted:                1,
			expectedMachinesUpdatedInPlace:         5,
		},
		{ // scale in by 10 (maxUnavailable >= replicas)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 0, MaxUnavailable 10",
			maxSurge:                               0,
			maxUnavailable:                         10,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesUpdatedInPlace:         6,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale up machine deployment in the middle
			name:           "In-place rollout, 6 Replicas, maxSurge 3, MaxUnavailable 1, scale up to 12",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 6 replica in the middle of a rollout, with 3 machines already move to the newMS and 3 still on the oldMS, and then MD scaled up to 12.
				machineDeployment: createMD("v2", 12, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already moved to ms2
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						// "m1", "m2", "m3" already updated in place
						createM("m1", "ms2", "v2"),
						createM("m2", "ms2", "v2"),
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 6,
			},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m10", "m11", "m12"},
			maxUnavailableBreachToleration:         maxUnavailableBreachToleration(), // after scale up it is expected to have less available machine than expected
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesCreated:                6,
			expectedMachinesUpdatedInPlace:         3,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale down machine deployment in the middle
			name:           "In-place rollout, 12 Replicas, maxSurge 3, MaxUnavailable 1, scale down to 6",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 12 replica in the middle of a rollout, with 3 machines already move to the newMS and 9 still on the oldMS, and then MD scaled down to 6.
				machineDeployment: createMD("v2", 6, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 9),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already moved to ms2
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
						createM("m7", "ms1", "v1"),
						createM("m8", "ms1", "v1"),
						createM("m9", "ms1", "v1"),
						createM("m10", "ms1", "v1"),
						createM("m11", "ms1", "v1"),
						createM("m12", "ms1", "v1"),
					},
					"ms2": {
						// "m1", "m2", "m3" already updated in place
						createM("m1", "ms2", "v2"),
						createM("m2", "ms2", "v2"),
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 12,
			},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			maxSurgeBreachToleration:               maxSurgeToleration(), // after scale down it is expected to temporarily have more machine than expected
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesDeleted:                6,
			expectedMachinesUpdatedInPlace:         7, // TODO: rollout planner does not handle in a optimal way in-place rollout + scale down (it moves more machines than desired spec replicas, update in place exceeding machines, then delete exceeding machines)
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + change spec in the middle
			name:           "In-place rollout, 6 Replicas, maxSurge 3, MaxUnavailable 1, change spec",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 6 replica in the middle of a rollout, with 3 machines already move to the newMS and 3 still on the oldMS, and then MD spec is changed.
				machineDeployment: createMD("v3", 6, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already moved to ms2
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						// "m1", "m2", "m3" already updated in place
						createM("m1", "ms2", "v2"),
						createM("m2", "ms2", "v2"),
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 6,
			},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
			expectedMachinesUpdatedInPlace:         6,
		},

		// Rollout with In-place updates (not affecting availability)

		{ // scale out by 1
			name:                                   "In-place rollout, 3 Replicas, maxSurge 1, MaxUnavailable 0 - not affecting availability",
			maxSurge:                               1,
			maxUnavailable:                         0,
			currentMachineNames:                    []string{"m1", "m2", "m3"},
			desiredMachineNames:                    []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesUpdatedInPlace:         3,
		},
		{ // scale in by 1
			name:                                   "In-place rollout, 3 Replicas, maxSurge 0, MaxUnavailable 1 - not affecting availability",
			maxSurge:                               0,
			maxUnavailable:                         1,
			currentMachineNames:                    []string{"m1", "m2", "m3"},
			desiredMachineNames:                    []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesUpdatedInPlace:         3,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 3, MaxUnavailable 1 - not affecting availability",
			maxSurge:                               3,
			maxUnavailable:                         1,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesUpdatedInPlace:         6,
		},
		{ // scale out by 1, scale in by 3 (maxSurge < maxUnavailable)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 1, MaxUnavailable 3 - not affecting availability",
			maxSurge:                               1,
			maxUnavailable:                         3,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesUpdatedInPlace:         6,
		},
		{ // scale out by 10 (maxSurge >= replicas)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 10, MaxUnavailable 0 - not affecting availability",
			maxSurge:                               10,
			maxUnavailable:                         0,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesUpdatedInPlace:         6,
		},
		{ // scale in by 10 (maxUnavailable >= replicas)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 0, MaxUnavailable 10 - not affecting availability",
			maxSurge:                               0,
			maxUnavailable:                         10,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesUpdatedInPlace:         6,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale up machine deployment in the middle
			name:           "In-place rollout, 6 Replicas, maxSurge 3, MaxUnavailable 1, scale up to 12 - not affecting availability",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 6 replica in the middle of a rollout, with 3 machines already move to the newMS and 3 still on the oldMS, and then MD scaled up to 12.
				machineDeployment: createMD("v2", 12, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already moved to ms2
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						// "m1", "m2", "m3" already updated in place
						createM("m1", "ms2", "v2"),
						createM("m2", "ms2", "v2"),
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 6,
			},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m10", "m11", "m12"},
			maxUnavailableBreachToleration:         maxUnavailableBreachToleration(), // after scale up it is expected to have less available machine then expected
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesCreated:                6,
			expectedMachinesUpdatedInPlace:         3,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + scale down machine deployment in the middle
			name:           "In-place rollout, 12 Replicas, maxSurge 3, MaxUnavailable 1, scale down to 6 - not affecting availability",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 12 replica in the middle of a rollout, with 3 machines already move to the newMS and 9 still on the oldMS, and then MD scaled down to 6.
				machineDeployment: createMD("v2", 6, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 9),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already moved to ms2
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
						createM("m7", "ms1", "v1"),
						createM("m8", "ms1", "v1"),
						createM("m9", "ms1", "v1"),
						createM("m10", "ms1", "v1"),
						createM("m11", "ms1", "v1"),
						createM("m12", "ms1", "v1"),
					},
					"ms2": {
						// "m1", "m2", "m3" already updated in place
						createM("m1", "ms2", "v2"),
						createM("m2", "ms2", "v2"),
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 12,
			},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			maxSurgeBreachToleration:               maxSurgeToleration(), // after scale down it is expected to temporarily have more machine than expected
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesDeleted:                6,
			expectedMachinesUpdatedInPlace:         7, // TODO: rollout planner does not handle in a optimal way in-place rollout + scale down (it moves more machines than desired spec replicas, update in place exceeding machines, then delete exceeding machines)
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable) + change spec in the middle
			name:           "In-place rollout, 6 Replicas, maxSurge 3, MaxUnavailable 1, change spec - not affecting availability",
			maxSurge:       3,
			maxUnavailable: 1,
			currentScope: &rolloutScope{ // Manually providing a scope simulating a MD originally with 6 replica in the middle of a rollout, with 3 machines already move to the newMS and 3 still on the oldMS, and then MD spec is changed.
				machineDeployment: createMD("v3", 6, withRollingUpdateStrategy(3, 1)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						// "m1", "m2", "m3" already moved to ms2
						createM("m4", "ms1", "v1"),
						createM("m5", "ms1", "v1"),
						createM("m6", "ms1", "v1"),
					},
					"ms2": {
						// "m1", "m2", "m3" already updated in place
						createM("m1", "ms2", "v2"),
						createM("m2", "ms2", "v2"),
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 6,
			},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability,
			expectedMachinesUpdatedInPlace:         6,
		},

		// Rollout with In-place updates (mixed affecting availability cases)

		{ // rollout with some machines that can be updated in-place affecting availability,
			name:           "Mixed case in-place affecting and not affecting availability",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 2),
					createMS("ms2", "v2", 1),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
					},
					"ms2": {
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 3,
			},
			// m3 should be moved first because it is possible to move it without affecting availability
			// m4 is created because m1 and m2 can be updated in place but the operation affects availability
			// m1 is updated in place
			// m2 is deleted to restore the desired number of replicas
			desiredMachineNames: []string{"m1", "m3", "m4"},
			overrideCanUpdateMachineSetInPlaceFunc: func(_ int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					switch oldMS.Name {
					case "ms1":
						return canUpdateMachineSetResult{
							canUpdateMachineSet: true,
							affectsAvailability: true,
						}, nil
					case "ms2":
						return canUpdateMachineSetResult{
							canUpdateMachineSet: true,
							affectsAvailability: false,
						}, nil
					default:
						return canUpdateMachineSetResult{}, fmt.Errorf("invalid MachineSet name %s", oldMS.Name)
					}
				}
			},
			expectedMachinesCreated:        1,
			expectedMachinesDeleted:        1,
			expectedMachinesUpdatedInPlace: 2,
		},

		// Rollout with In-place updates retry after

		{ // scale out by 1
			name:                "In-place rollout, 3 Replicas, maxSurge 1, MaxUnavailable 0 - retry after",
			maxSurge:            1,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					if i < 10 {
						return canUpdateMachineSetResult{
							retryAfter:              time.Now().Add(2 * time.Second),
							retryAfterFromExtension: true,
						}, nil
					}
					return canUpdateMachineSetResult{
						canUpdateMachineSet: true,
						affectsAvailability: false,
					}, nil
				}
			},
			expectedMachinesUpdatedInPlace: 3,
		},

		// Rollout with In-place updates fallback to rollout

		{ // scale out by 1
			name:                "In-place rollout, 3 Replicas, maxSurge 1, MaxUnavailable 0 - fallback to rollout",
			maxSurge:            1,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: func(_ int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					return canUpdateMachineSetResult{
						canUpdateMachineSet: true,
						affectsAvailability: false,
						allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationReplace}),
					}, nil
				}
			},
			expectedMachinesCreated: 3,
			expectedMachinesDeleted: 3,
		},

		// Rollout with In-place updates block rollout

		{ // scale out by 1
			name:                "In-place rollout, 3 Replicas, maxSurge 1, MaxUnavailable 0 - block all operations",
			maxSurge:            1,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					if i < 10 {
						return canUpdateMachineSetResult{
							canUpdateMachineSet: true,
							affectsAvailability: false,
							allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{}),
						}, nil
					}
					return canUpdateMachineSetResult{
						canUpdateMachineSet: true,
						affectsAvailability: false,
					}, nil
				}
			},
			expectedMachinesUpdatedInPlace: 3,
		},

		{ // scale out by 1
			name:                "In-place rollout, 3 Replicas, maxSurge 1, MaxUnavailable 0 - only in-place",
			maxSurge:            1,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					if i < 10 {
						return canUpdateMachineSetResult{
							canUpdateMachineSet: false,
							affectsAvailability: false,
							allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}),
						}, nil
					}
					return canUpdateMachineSetResult{
						canUpdateMachineSet: true,
						affectsAvailability: false,
						allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}),
					}, nil
				}
			},
			expectedMachinesUpdatedInPlace: 3,
		},

		// Rollout with In-place updates, multiple old MS, retry after, fallback to rollout and block rollout

		{ // ms1 in-place, ms2 in-place possible but only rolling update allowed
			name:           "In-place rollout, multiple old MS, MaxSurge 1, MaxUnavailable 0 - one MS in-place, one MS fallback to rollout",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 2),
					createMS("ms2", "v2", 1),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
					},
					"ms2": {
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 3,
			},
			desiredMachineNames: []string{"m1", "m2", "m4"},
			overrideCanUpdateMachineSetInPlaceFunc: func(_ int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					switch oldMS.Name {
					case "ms1":
						return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true}, nil
					case "ms2":
						return canUpdateMachineSetResult{
							canUpdateMachineSet: true,
							affectsAvailability: false,
							allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationReplace}),
						}, nil
					default:
						return canUpdateMachineSetResult{}, fmt.Errorf("invalid MachineSet name %s", oldMS.Name)
					}
				}
			},
			expectedMachinesCreated:        1,
			expectedMachinesDeleted:        1,
			expectedMachinesUpdatedInPlace: 2,
		},
		{ // ms1 in-place, ms2 blocked until iteration 10
			name:           "In-place rollout, multiple old MS, MaxSurge 1, MaxUnavailable 0 - one MS in-place, one MS blocked",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 2),
					createMS("ms2", "v2", 1),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
					},
					"ms2": {
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 3,
			},
			desiredMachineNames: []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					switch oldMS.Name {
					case "ms1":
						return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false}, nil
					case "ms2":
						if i < 10 {
							return canUpdateMachineSetResult{
								canUpdateMachineSet: false,
								allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}),
							}, nil
						}
						return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false}, nil
					default:
						return canUpdateMachineSetResult{}, fmt.Errorf("invalid MachineSet name %s", oldMS.Name)
					}
				}
			},
			expectedMachinesCreated:        0,
			expectedMachinesDeleted:        0,
			expectedMachinesUpdatedInPlace: 3,
		},
		{ // ms1 rolling update, ms2 blocked until iteration 10
			name:           "In-place rollout, multiple old MS, MaxSurge 1, MaxUnavailable 0 - one MS rolling update, one MS blocked",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 2),
					createMS("ms2", "v2", 1),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
					},
					"ms2": {
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 3,
			},
			desiredMachineNames: []string{"m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					switch oldMS.Name {
					case "ms1":
						return canUpdateMachineSetResult{canUpdateMachineSet: false}, nil
					case "ms2":
						if i < 10 {
							return canUpdateMachineSetResult{
								canUpdateMachineSet: false,
								allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{runtimehooksv1.MachineUpdateOperationUpdateInPlace}),
							}, nil
						}
						return canUpdateMachineSetResult{canUpdateMachineSet: false}, nil
					default:
						return canUpdateMachineSetResult{}, fmt.Errorf("invalid MachineSet name %s", oldMS.Name)
					}
				}
			},
			expectedMachinesCreated:        3,
			expectedMachinesDeleted:        3,
			expectedMachinesUpdatedInPlace: 0,
		},
		{ // ms1 and ms2 blocked until iteration 10
			name:           "In-place rollout, multiple old MS, MaxSurge 1, MaxUnavailable 0 - all MS blocked",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 2),
					createMS("ms2", "v2", 1),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
					},
					"ms2": {
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 3,
			},
			desiredMachineNames: []string{"m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					if i < 10 {
						return canUpdateMachineSetResult{
							canUpdateMachineSet: false,
							allowedOperations:   new([]runtimehooksv1.MachineUpdateOperation{}),
						}, nil
					}
					return canUpdateMachineSetResult{canUpdateMachineSet: false}, nil
				}
			},
			expectedMachinesCreated:        3,
			expectedMachinesDeleted:        3,
			expectedMachinesUpdatedInPlace: 0,
		},
		{ // ms1 rolling update, ms2 retry after until iteration 10 (the whole rollout is paused)
			name:           "In-place rollout, multiple old MS, MaxSurge 1, MaxUnavailable 0 - one MS rolling update, one MS retry after",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 2),
					createMS("ms2", "v2", 1),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
					},
					"ms2": {
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 3,
			},
			desiredMachineNames: []string{"m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					switch oldMS.Name {
					case "ms1":
						return canUpdateMachineSetResult{canUpdateMachineSet: false}, nil
					case "ms2":
						if i < 10 {
							return canUpdateMachineSetResult{retryAfter: time.Now().Add(2 * time.Second), retryAfterFromExtension: true}, nil
						}
						return canUpdateMachineSetResult{canUpdateMachineSet: false}, nil
					default:
						return canUpdateMachineSetResult{}, fmt.Errorf("invalid MachineSet name %s", oldMS.Name)
					}
				}
			},
			expectedMachinesCreated:        3,
			expectedMachinesDeleted:        3,
			expectedMachinesUpdatedInPlace: 0,
		},
		{ // ms1 in-place, ms2 retry after until iteration 10 (the whole rollout is paused)
			name:           "In-place rollout, multiple old MS, MaxSurge 1, MaxUnavailable 0 - one MS in-place, one MS retry after",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 2),
					createMS("ms2", "v2", 1),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
					},
					"ms2": {
						createM("m3", "ms2", "v2"),
					},
				},
				machineUID: 3,
			},
			desiredMachineNames: []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					switch oldMS.Name {
					case "ms1":
						return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false}, nil
					case "ms2":
						if i < 10 {
							return canUpdateMachineSetResult{retryAfter: time.Now().Add(2 * time.Second), retryAfterFromExtension: true}, nil
						}
						return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: false}, nil
					default:
						return canUpdateMachineSetResult{}, fmt.Errorf("invalid MachineSet name %s", oldMS.Name)
					}
				}
			},
			expectedMachinesCreated:        0,
			expectedMachinesDeleted:        0,
			expectedMachinesUpdatedInPlace: 3,
		},
		{ // md scaled down while ms2 is waiting for a decision, excess machines on ms1 must be deleted, not moved
			name:           "In-place rollout, multiple old MS, MaxSurge 1, MaxUnavailable 0 - scale down while one MS retry after",
			maxSurge:       1,
			maxUnavailable: 0,
			currentScope: &rolloutScope{ // Manually providing a scope simulating multiple old MS, with the MD scaled down to 3 replicas
				machineDeployment: createMD("v3", 3, withRollingUpdateStrategy(1, 0)),
				machineSets: []*clusterv1.MachineSet{
					createMS("ms1", "v1", 3),
					createMS("ms2", "v2", 3),
				},
				machineSetMachines: map[string][]*clusterv1.Machine{
					"ms1": {
						createM("m1", "ms1", "v1"),
						createM("m2", "ms1", "v1"),
						createM("m3", "ms1", "v1"),
					},
					"ms2": {
						createM("m4", "ms2", "v2"),
						createM("m5", "ms2", "v2"),
						createM("m6", "ms2", "v2"),
					},
				},
				machineUID: 6,
			},
			desiredMachineNames:      []string{"m3", "m4", "m5"},
			maxSurgeBreachToleration: maxSurgeToleration(), // after scale down it is expected to temporarily have more machine than expected
			overrideCanUpdateMachineSetInPlaceFunc: func(i int) func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
				return func(_ context.Context, oldMS, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					switch oldMS.Name {
					case "ms1":
						return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true}, nil
					case "ms2":
						if i < 10 {
							return canUpdateMachineSetResult{retryAfter: time.Now().Add(2 * time.Second), retryAfterFromExtension: true}, nil
						}
						return canUpdateMachineSetResult{canUpdateMachineSet: true, affectsAvailability: true}, nil
					default:
						return canUpdateMachineSetResult{}, fmt.Errorf("invalid MachineSet name %s", oldMS.Name)
					}
				}
			},
			expectedMachinesCreated:        0,
			expectedMachinesDeleted:        3, // m1 and m2 are deleted while ms2 is waiting for a decision (no move), m6 is deleted after the decision to align to spec.replicas
			expectedMachinesUpdatedInPlace: 3,
		},
	}

	testWithPredictableReconcileOrder := true
	testWithRandomReconcileOrderFromConstantSeed := true
	testWithRandomReconcileOrderFromRandomSeed := true

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.name

			if testWithPredictableReconcileOrder {
				tt.maxIterations = 50
				tt.randomControllerOrder = false
				tt.logAndGoldenFileName = strings.ToLower(tt.name)
				t.Run("default", func(t *testing.T) {
					runRollingUpdateTestCase(ctx, t, tt)
				})
			}

			if testWithRandomReconcileOrderFromConstantSeed {
				tt.maxIterations = 70
				tt.name = fmt.Sprintf("%s, random(0)", name)
				tt.randomControllerOrder = true
				tt.seed = 0
				tt.logAndGoldenFileName = strings.ToLower(tt.name)
				t.Run("random(0)", func(t *testing.T) {
					runRollingUpdateTestCase(ctx, t, tt)
				})
			}

			if testWithRandomReconcileOrderFromRandomSeed {
				for range 100 {
					tt.maxIterations = 150
					tt.seed = time.Now().UnixNano()
					tt.name = fmt.Sprintf("%s, random(%d)", name, tt.seed)
					tt.randomControllerOrder = true
					tt.skipLogToFileAndGoldenFileCheck = true
					t.Run(fmt.Sprintf("random(%d)", tt.seed), func(t *testing.T) {
						runRollingUpdateTestCase(ctx, t, tt)
					})
				}
			}
		})
	}
}

func runRollingUpdateTestCase(ctx context.Context, t *testing.T, tt rollingUpdateSequenceTestCase) {
	t.Helper()
	g := NewWithT(t)

	rng := rand.New(rand.NewSource(tt.seed)) //nolint:gosec // it is ok to use a weak randomizer here
	fLogger := newFileLogger(t, tt.name, fmt.Sprintf("testdata/rollingupdate/%s", tt.logAndGoldenFileName))
	// uncomment this line to automatically generate/update golden files:
	// uncomment this line to automatically generate/update golden files: fLogger.writeGoldenFile = true

	// Init current and desired state from test case
	current := tt.currentScope.Clone()
	if current == nil {
		current = initCurrentRolloutScope(tt.currentMachineNames, withRollingUpdateStrategy(tt.maxSurge, tt.maxUnavailable))
	}
	desired := computeDesiredRolloutScope(current, tt.desiredMachineNames)

	// Add readme to the generated log file.
	fLogger.Logf("[Test] Legend:\n" +
		"    - X/Y replicas is status.replicas/spec.replicas\n" +
		"    - After each reconcile, the resulting status is reported with notes about relevant changes, if any\n" +
		"    - Machine list icons\n" +
		"      - ✋ Machine pending acknowledge move\n" +
		"      - 🟡 Machine with in place update in progress - affects availability\n" +
		"      - 🟢 Machine with in place update in progress - does not affect availability\n\n")

	// Log initial state
	fLogger.Logf("[Test] Initial state\n%s\n", current.summary())
	fLogger.Logf("[Test] Strategy=RollingUpdate, MaxSurge=%d, MaxUnavailable=%d\n\n", tt.maxSurge, tt.maxUnavailable)
	if tt.randomControllerOrder {
		fLogger.Logf("[Test] Random reconcile sequence, seed=%d\n\n", tt.seed)
	}
	i := 1
	maxIterations := tt.maxIterations
	for {
		// Uncomment this to surface iterations fLogger.Logf("[Test] Iteration %d", i)

		taskList := getTaskListRollingUpdate(current)
		taskCount := len(taskList)
		taskOrder := defaultTaskOrder(taskCount)
		if tt.randomControllerOrder {
			taskOrder = randomTaskOrder(taskCount, rng)
		}
		for _, taskID := range taskOrder {
			task := taskList[taskID]
			if task == "md" {
				// Running a small subset of MD reconcile (the rollout logic and a bit of setReplicas)
				p := newRolloutPlanner(nil, nil, nil)
				p.overrideCanUpdateMachineSetInPlace = func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
					return canUpdateMachineSetResult{
						canUpdateMachineSet: false,
					}, nil
				}
				if tt.overrideCanUpdateMachineSetInPlaceFunc != nil {
					p.overrideCanUpdateMachineSetInPlace = tt.overrideCanUpdateMachineSetInPlaceFunc(i)
				}
				p.overrideComputeDesiredMS = func(ctx context.Context, deployment *clusterv1.MachineDeployment, currentNewMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
					log := ctrl.LoggerFrom(ctx)
					desiredNewMS := currentNewMS
					if currentNewMS == nil {
						// uses a predictable MS name when creating newMS, also add the newMS to current.machineSets
						totMS := len(current.machineSets)
						desiredNewMS = createMS(fmt.Sprintf("ms%d", totMS+1), deployment.Spec.Template.Spec.FailureDomain, 0)
						current.machineSets = append(current.machineSets, desiredNewMS)
						log.V(5).Info(fmt.Sprintf("Computing new MachineSet %s with %d replicas", klog.KObj(desiredNewMS), ptr.Deref(desiredNewMS.Spec.Replicas, 0)), "MachineSet", klog.KObj(desiredNewMS))
					}

					// The annotations set by the rollout planner are not part of the output of ComputeDesiredMS, so drop them to mimic the same behavior.
					delete(desiredNewMS.Annotations, clusterv1.AcknowledgedMoveAnnotation)
					delete(desiredNewMS.Annotations, clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation)
					delete(desiredNewMS.Annotations, clusterv1.MachineSetMoveMachinesToMachineSetAnnotation)
					return desiredNewMS, nil
				}

				// init the rollout planner and plan next step for a rollout.
				err := p.init(ctx, current.machineDeployment, current.machineSets, current.machines(), true, true)
				g.Expect(err).ToNot(HaveOccurred())

				err = p.planRollingUpdate(ctx)
				g.Expect(err).ToNot(HaveOccurred())

				// Apply changes.
				for _, ms := range current.machineSets {
					if scaleIntent, ok := p.scaleIntents[ms.Name]; ok {
						ms.Spec.Replicas = new(scaleIntent)
					}
				}

				// Running a small subset of setReplicas (we don't want to run the full func to avoid unnecessary noise on the test)
				current.machineDeployment.Status.Replicas = mdutil.GetActualReplicaCountForMachineSets(current.machineSets)
				current.machineDeployment.Status.AvailableReplicas = mdutil.GetAvailableReplicaCountForMachineSets(current.machineSets)
				current.machineDeployment.Status.UpToDateReplicas = mdutil.GetUptoDateReplicaCountForMachineSets(current.machineSets)

				// Log state after this reconcile
				fLogger.Logf("[MD controller] Reconcile %s\n  %s", current.machineDeployment.Name, current.rolloutPlannerResultSummary(p))

				// Check that the rollout planner is not breaching rollout constraints
				// Note: After a machine is moved from a MachineSet to another, the replica counter of the target MachineSet
				// are temporarily outdated, until the target MachineSet is reconciled.
				// In order to avoid false negatives when reconcile target MachineSet does not happen in one iteration
				// the code below is computing totAvailableReplicas and totReplicas starting from the list of machines instead
				// of using the replica counters on MachinesSets.
				minAvailableReplicas := ptr.Deref(current.machineDeployment.Spec.Replicas, 0) - mdutil.MaxUnavailable(*current.machineDeployment)
				totAvailableReplicas := int32(0)
				for _, m := range current.machines() {
					if inplace.IsUpdateInProgress(m) {
						// Machines updating in-place should be considered available only if the operation does not affect availability.
						if inplace.IsUpdateInProgressNotAffectingAvailability(m) {
							totAvailableReplicas++
						}
						continue
					}
					totAvailableReplicas++
				}
				if totAvailableReplicas < minAvailableReplicas {
					tolerateBreach := false
					if tt.maxUnavailableBreachToleration != nil {
						tolerateBreach = tt.maxUnavailableBreachToleration(fLogger, i, current, minAvailableReplicas, totAvailableReplicas)
					}
					if !tolerateBreach {
						g.Expect(totAvailableReplicas).To(BeNumerically(">=", minAvailableReplicas), "totAvailable machines is less than md.spec.replicas - maxUnavailable")
					}
				}

				maxAllowedReplicas := ptr.Deref(current.machineDeployment.Spec.Replicas, 0) + mdutil.MaxSurge(*current.machineDeployment)
				totReplicas := int32(len(current.machines()))
				if totReplicas > maxAllowedReplicas {
					tolerateBreach := false
					if tt.maxSurgeBreachToleration != nil {
						tolerateBreach = tt.maxSurgeBreachToleration(fLogger, i, current, maxAllowedReplicas, totReplicas)
					}
					if !tolerateBreach {
						g.Expect(totReplicas).To(BeNumerically("<=", maxAllowedReplicas), "totReplicas machines is greater than md.spec.replicas + maxSurge")
					}
				}
			}

			// Run mutators faking other controllers
			for _, ms := range current.machineSets {
				if ms.Name == task {
					fLogger.Logf("[MS controller] Reconcile %s\n", ms.Name)
					err := machineSetControllerMutator(ctx, ms, current, fLogger)
					g.Expect(err).ToNot(HaveOccurred())
					break
				}
			}
		}

		// Check if we are at the desired state
		inPlaceInProgress := false
		for _, m := range current.machines() {
			if _, ok := m.Annotations[clusterv1.UpdateInProgressAnnotation]; ok {
				inPlaceInProgress = true
				break
			}
		}
		if current.Equal(desired) && !inPlaceInProgress {
			fLogger.Logf("\n[Test] Final state\n%s", current.summary())
			break
		}

		// Safeguard for infinite reconcile
		i++
		if i > maxIterations {
			// NOTE: the following can be used to set a breakpoint for debugging why the system is not reaching desired state after maxIterations (to check what is not yet equal)
			current.Equal(desired)
			// Log desired state we never reached
			fLogger.Logf("\n[Test] Desired state\n%s", desired.summary())
			g.Fail(fmt.Sprintf("Failed to reach desired state in %d iterations", maxIterations))
		}
	}

	g.Expect(current.machinesCreated).To(Equal(tt.expectedMachinesCreated), "unexpected number of machines created")
	g.Expect(current.machinesDeleted).To(Equal(tt.expectedMachinesDeleted), "unexpected number of machines deleted")
	g.Expect(current.machinesUpdatedInPlace).To(Equal(tt.expectedMachinesUpdatedInPlace), "unexpected number of machines updated in-place")

	if !tt.skipLogToFileAndGoldenFileCheck {
		currentLog, goldenLog, err := fLogger.WriteLogAndCompareWithGoldenFile()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentLog).To(Equal(goldenLog), "current test case log and golden test case log are different\n%s", cmp.Diff(currentLog, goldenLog))
	}
}

func oldMSCanAlwaysUpdateInPlace(_ int) func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
	return func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
		return canUpdateMachineSetResult{
			canUpdateMachineSet: true,
			affectsAvailability: true,
		}, nil
	}
}

func oldMSCanAlwaysUpdateInPlaceWithoutAffectingAvailability(_ int) func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
	return func(_ context.Context, _, _ *clusterv1.MachineSet) (canUpdateMachineSetResult, error) {
		return canUpdateMachineSetResult{
			canUpdateMachineSet: true,
			affectsAvailability: false,
		}, nil
	}
}

func maxUnavailableBreachToleration() func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
	return func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
		log.Logf("[Toleration] tolerate maxUnavailable breach")
		return true
	}
}

func maxSurgeToleration() func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
	return func(log *fileLogger, _ int, _ *rolloutScope, _, _ int32) bool {
		log.Logf("[Toleration] tolerate maxSurge breach")
		return true
	}
}

func getTaskListRollingUpdate(current *rolloutScope) []string {
	taskList := make([]string, 0)
	taskList = append(taskList, "md")
	for _, ms := range current.machineSets {
		taskList = append(taskList, ms.Name)
	}
	taskList = append(taskList, fmt.Sprintf("ms%d", len(current.machineSets)+1)) // r the MachineSet that might be created when reconciling md
	return taskList
}
