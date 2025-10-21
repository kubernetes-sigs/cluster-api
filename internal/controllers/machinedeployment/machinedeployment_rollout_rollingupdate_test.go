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
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
)

func TestReconcileReplicasPendingAcknowledgeMove(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)

	testCases := []struct {
		name                              string
		md                                *clusterv1.MachineDeployment
		originalNewMS                     *clusterv1.MachineSet
		newMS                             *clusterv1.MachineSet
		machines                          []*clusterv1.Machine
		expectedReplicas                  int32
		expectedAcknowledgeMoveAnnotation *string
	}{
		{
			name:                              "Should not scale up when there are no machines",
			md:                                createMD("v1", 3),
			newMS:                             createMS("ms1", "v1", 1),
			machines:                          nil,
			expectedReplicas:                  1,
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
			expectedReplicas:                  1,
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
			expectedReplicas:                  2, // up by one
			expectedAcknowledgeMoveAnnotation: ptr.To("m2"),
		},
		{
			name:          "Should scale up when there are machines recently moved to newMS and not yet acknowledged and there are other machines already acknowledged",
			md:            createMD("v1", 3),
			originalNewMS: createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m1")), // another machine already acknowledged
			newMS:         createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas:                  2, // up by one
			expectedAcknowledgeMoveAnnotation: ptr.To("m1,m2"),
		},
		{
			name:          "Should not scale up when there machines with the pendingAcknowledgeMove annotation but they are already acknowledged",
			md:            createMD("v1", 3),
			originalNewMS: createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m2")), // moved machine already acknowledged
			newMS:         createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1", withMAnnotation(clusterv1.PendingAcknowledgeMoveAnnotation, "")),
			},
			expectedReplicas:                  1,
			expectedAcknowledgeMoveAnnotation: ptr.To("m2"),
		},
		{
			name:          "Should drop machines from acknowledged movr annotation when they not anymore reporting pendingAcknowledge",
			md:            createMD("v1", 3),
			originalNewMS: createMS("ms1", "v1", 1, withMSAnnotation(clusterv1.AcknowledgedMoveAnnotation, "m2")), // moved machine already acknowledged
			newMS:         createMS("ms1", "v1", 1),
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
			},
			expectedReplicas: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			planner := newRolloutPlanner()
			planner.md = tc.md
			planner.newMS = tc.newMS
			if tc.originalNewMS != nil {
				planner.originalMS = make(map[string]*clusterv1.MachineSet)
				planner.originalMS[tc.newMS.Name] = tc.originalNewMS
			}
			planner.machines = tc.machines

			planner.reconcileReplicasPendingAcknowledgeMove(ctx)
			g.Expect(ptr.Deref(tc.newMS.Spec.Replicas, 0)).To(Equal(tc.expectedReplicas))
			if tc.expectedAcknowledgeMoveAnnotation != nil {
				g.Expect(planner.newMS.Annotations).To(HaveKeyWithValue(clusterv1.AcknowledgedMoveAnnotation, *tc.expectedAcknowledgeMoveAnnotation))
			} else {
				g.Expect(planner.newMS.Annotations).ToNot(HaveKey(clusterv1.AcknowledgedMoveAnnotation))
			}
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
		error                         error
	}{
		{
			name: "RollingUpdate strategy: Scale up: 0 -> 2",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(2),
							},
						},
					},
					Replicas: ptr.To[int32](2),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](0),
				},
			},
			expectedNewMachineSetReplicas: 2,
		},
		{
			name: "RollingUpdate strategy: Scale down: 2 -> 0",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(2),
							},
						},
					},
					Replicas: ptr.To[int32](0),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedNewMachineSetReplicas: 0,
		},
		{
			name: "RollingUpdate strategy: Scale up does not go above maxSurge (3+2)",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(2),
							},
						},
					},
					Replicas: ptr.To[int32](3),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](1),
				},
			},
			expectedNewMachineSetReplicas: 2,
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "3replicas",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](3),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](3),
					},
				},
			},
			error: nil,
		},
		{
			name: "RollingUpdate strategy: Scale up accounts for deleting Machines to honour maxSurge",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(0),
							},
						},
					},
					Replicas: ptr.To[int32](1),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](0),
				},
			},
			expectedNewMachineSetReplicas: 0,
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "machine-not-yet-deleted",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](0),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: ptr.To[int32](1),
					},
				},
			},
			error: nil,
		},
		{
			name: "Rolling Updated Cleanup disable machine create annotation",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Rollout: clusterv1.MachineDeploymentRolloutSpec{
						Strategy: clusterv1.MachineDeploymentRolloutStrategy{
							Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
							RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
								MaxUnavailable: intOrStrPtr(0),
								MaxSurge:       intOrStrPtr(0),
							},
						},
					},
					Replicas: ptr.To[int32](2),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
					Annotations: map[string]string{
						clusterv1.DisableMachineCreateAnnotation: "true",
						clusterv1.DesiredReplicasAnnotation:      "2",
						clusterv1.MaxReplicasAnnotation:          "2",
					},
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedNewMachineSetReplicas: 2,
			error:                         nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			planner := newRolloutPlanner()
			planner.md = tc.machineDeployment
			planner.newMS = tc.newMachineSet
			planner.oldMSs = tc.oldMachineSets

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
		expectScaleIntent          map[string]int32
		skipMaxUnavailabilityCheck bool
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &rolloutPlanner{
				md:           tt.md,
				newMS:        tt.newMS,
				oldMSs:       tt.oldMSs,
				scaleIntents: tt.scaleIntent,
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
					oldMS.Status.AvailableReplicas = ptr.To(max(ptr.Deref(oldMS.Status.AvailableReplicas, 0)-machineScaleDown, 0))
				}
			}
			minAvailableReplicas := ptr.Deref(tt.md.Spec.Replicas, 0) - mdutil.MaxUnavailable(*tt.md)
			totAvailableReplicas := ptr.Deref(mdutil.GetAvailableReplicaCountForMachineSets(append(tt.oldMSs, tt.newMS)), 0)
			if !tt.skipMaxUnavailabilityCheck {
				g.Expect(totAvailableReplicas).To(BeNumerically(">=", minAvailableReplicas), "totAvailable machines is less than minAvailable")
			} else {
				t.Logf("skipping MaxUnavailability check (totAvailableReplicas: %d, minAvailableReplicas: %d)", totAvailableReplicas, minAvailableReplicas)
			}
		})
	}
}

func TestReconcileInPlaceUpdateIntent(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.InPlaceUpdates, true)

	testCases := []struct {
		name                   string
		md                     *clusterv1.MachineDeployment
		newMS                  *clusterv1.MachineSet
		oldMS                  []*clusterv1.MachineSet
		machines               []*clusterv1.Machine
		scaleIntents           map[string]int32
		upToDateResults        map[string]mdutil.UpToDateResult
		canUpdateAnswer        map[string]bool
		expectedCanUpdateCalls map[string]bool
		expectMoveFromMS       []string
		expectScaleIntents     map[string]int32
	}{
		// NewMS not scaling up

		{
			name:  "Move replicas from oldMS to newMS",
			md:    createMD("v2", 2),
			newMS: createMS("ms2", "v2", 1),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms2", "v2"),
			},
			scaleIntents: map[string]int32{},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS:   []string{"ms1"},
			expectScaleIntents: map[string]int32{},
		},
		{
			name:  "Do not move replicas from oldMS to newMS when newMS already have all the desired replicas",
			md:    createMD("v1", 3),
			newMS: createMS("ms2", "v2", 3),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms2", "v2"),
				createM("m3", "ms2", "v2"),
				createM("m4", "ms2", "v2"),
			},
			scaleIntents: map[string]int32{},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{},
			canUpdateAnswer:        map[string]bool{},
			expectMoveFromMS:       []string{},
			expectScaleIntents:     map[string]int32{},
		},
		{
			name:  "Do not move replicas from oldMS to newMS when oldMS doesn't have replicas anymore",
			md:    createMD("v2", 3),
			newMS: createMS("ms2", "v2", 1),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 0),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms2", "v2"),
			},
			scaleIntents: map[string]int32{},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{},
			canUpdateAnswer:        map[string]bool{},
			expectMoveFromMS:       []string{},
			expectScaleIntents:     map[string]int32{},
		},
		{
			name:  "Do not move replicas from oldMS to newMS when the system does not know if oldMS is eligible for in-place update", // Note: this should never happen, defensive programming.
			md:    createMD("v2", 2),
			newMS: createMS("ms2", "v2", 1),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms2", "v2"),
			},
			scaleIntents:           map[string]int32{},
			upToDateResults:        map[string]mdutil.UpToDateResult{},
			expectedCanUpdateCalls: map[string]bool{},
			canUpdateAnswer:        map[string]bool{},
			expectMoveFromMS:       []string{},
			expectScaleIntents:     map[string]int32{},
		},
		{
			name:  "Do not move replicas from oldMS to newMS when oldMS is not eligible for in-place update",
			md:    createMD("v2", 2),
			newMS: createMS("ms2", "v2", 1),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms2", "v2"),
			},
			scaleIntents: map[string]int32{},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: false},
			},
			expectedCanUpdateCalls: map[string]bool{},
			canUpdateAnswer:        map[string]bool{},
			expectMoveFromMS:       []string{},
			expectScaleIntents:     map[string]int32{},
		},
		{
			name:  "Do not move replicas from oldMS to newMS when canUpdateDecision for the oldMS is false",
			md:    createMD("v2", 2),
			newMS: createMS("ms2", "v2", 1),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms2", "v2"),
			},
			scaleIntents: map[string]int32{},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": false,
			},
			expectMoveFromMS:   []string{},
			expectScaleIntents: map[string]int32{},
		},
		{
			name:  "Multiple oldMSs, mixed use cases",
			md:    createMD("v6", 6),
			newMS: createMS("ms6", "v6", 1),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1), // eligible for in-place, canUpdateDecision true
				createMS("ms2", "v2", 1), // eligible for in-place, canUpdateDecision false
				createMS("ms3", "v3", 1), // not eligible for in-place
				createMS("ms4", "v4", 1), // unknown if eligible for in-place (should never happen)
				createMS("ms5", "v5", 1), // eligible for in-place, canUpdateDecision true
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms2", "v2"),
				createM("m3", "ms3", "v3"),
				createM("m4", "ms4", "v4"),
				createM("m5", "ms5", "v5"),
				createM("m6", "ms6", "v6"),
			},
			scaleIntents: map[string]int32{},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
				"ms2": {EligibleForInPlaceUpdate: true},
				"ms3": {EligibleForInPlaceUpdate: false},
				"ms5": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
				"ms2": true,
				"ms5": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
				"ms2": false,
				"ms5": true,
			},
			expectMoveFromMS:   []string{"ms1", "ms5"},
			expectScaleIntents: map[string]int32{},
		},

		// NewMS scaling up

		{
			name:  "When moving replicas from oldMS to newMS, preserve newMS scale up intent if it does not use maxSurge",
			md:    createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS: createMS("ms2", "v2", 1),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 1),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms2", "v2"),
			},
			scaleIntents: map[string]int32{
				"ms2": 2, // +1 => MD expect 3 replicas, MD has currently 2 replicas, scale up of +1 replica is not using maxSurge
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS: []string{"ms1"},
			expectScaleIntents: map[string]int32{
				"ms2": 2, // Scale intent not changed
			},
		},
		{
			name:  "When moving replicas from oldMS to newMS, preserve one usage of MaxSurge in the newMS scale up intent when required to start the rollout (maxSurge 1, maxUnavailable 0)",
			md:    createMD("v2", 3, withRollingUpdateStrategy(1, 0)),
			newMS: createMS("ms2", "v2", 0),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
				createM("m3", "ms1", "v1"),
			},
			scaleIntents: map[string]int32{
				"ms2": 1, // +1 => MD expect 3, has currently 3 replicas, +1 replica it is using maxSurge 1
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS: []string{"ms1"},
			expectScaleIntents: map[string]int32{
				// "ms2": 1, +1 replica from maxSurge preserved, the rollout needs to start
				"ms2": 1,
			},
		},
		{
			name:  "When moving replicas from oldMS to newMS, preserve one usage of MaxSurge in the newMS scale up intent when required to start the rollout (maxSurge 3, maxUnavailable 0)",
			md:    createMD("v2", 3, withRollingUpdateStrategy(3, 0)),
			newMS: createMS("ms2", "v2", 0),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
				createM("m3", "ms1", "v1"),
			},
			scaleIntents: map[string]int32{
				"ms2": 3, // +3 => MD expect 3, has currently 3 replicas, +3 replica it is using maxSurge 3
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS: []string{"ms1"},
			expectScaleIntents: map[string]int32{
				// "ms2": 3, +2 replicas from maxSurge dropped, +1 replicas from maxSurge preserved, the rollout needs to start
				"ms2": 1,
			},
		},
		{
			name:  "When moving replicas from oldMS to newMS, drop usage of MaxSurge in the newMS scale up intent when there are oldMS with scale down intent (maxSurge 3, maxUnavailable 1)",
			md:    createMD("v2", 3, withRollingUpdateStrategy(3, 1)),
			newMS: createMS("ms2", "v2", 0),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
				createM("m3", "ms1", "v1"),
			},
			scaleIntents: map[string]int32{
				"ms2": 3, // +3 => MD expect 3, has currently 3 replicas, +3 replica it is using maxSurge 3
				"ms1": 2, // -1 due to maxUnavailable 1
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS: []string{"ms1"},
			expectScaleIntents: map[string]int32{
				// "ms2": 3, +3 replica using maxSurge dropped, oldMS is scaling down
				"ms1": 2, // -1 due to maxUnavailable 1
			},
		},
		{
			name:  "When moving replicas from oldMS to newMS, drop usage of MaxSurge in the newMS scale up intent when there are oldMS with scale down from a previous reconcile (maxSurge 3, maxUnavailable 1)",
			md:    createMD("v2", 3, withRollingUpdateStrategy(3, 1)),
			newMS: createMS("ms2", "v2", 0),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(4)), // scale down from a previous reconcile
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
				createM("m3", "ms1", "v1"),
				createM("m4", "ms1", "v1"),
			},
			scaleIntents: map[string]int32{
				"ms2": 2, // +2 => MD expect 3, has currently 4 replicas, +2 replica it is using maxSurge 3
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS:   []string{"ms1"},
			expectScaleIntents: map[string]int32{
				// "ms2": 3, +3 replica using maxSurge dropped, oldMS is scaling down
			},
		},
		{
			name:  "When moving replicas from oldMS to newMS, drop usage of MaxSurge in the newMS scale up intent when there machines in-place updating (maxSurge 3, maxUnavailable 0)",
			md:    createMD("v2", 3, withRollingUpdateStrategy(3, 0)),
			newMS: createMS("ms2", "v2", 0),
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1", withMAnnotation(clusterv1.UpdateInProgressAnnotation, "")),
				createM("m2", "ms1", "v1"),
				createM("m3", "ms1", "v1"),
			},
			scaleIntents: map[string]int32{
				"ms2": 3, // +3 => MD expect 3, has currently 3 replicas, +3 replica it is using maxSurge 3
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS:   []string{"ms1"},
			expectScaleIntents: map[string]int32{
				// "ms2": 1, +1 replica using maxSurge dropped, there is a machine updating in place
			},
		},
		{
			name:  "When moving replicas from oldMS to newMS, drop usage of MaxSurge in the newMS scale up intent when there newMS is scaling from a previous reconcile (maxSurge 3, maxUnavailable 1)",
			md:    createMD("v6", 6, withRollingUpdateStrategy(3, 1)),
			newMS: createMS("ms2", "v2", 3, withStatusReplicas(2)), // scaling from a previous reconcile
			oldMS: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3),
			},
			machines: []*clusterv1.Machine{
				createM("m1", "ms1", "v1"),
				createM("m2", "ms1", "v1"),
				createM("m3", "ms1", "v1"),
				createM("m4", "ms2", "v2"),
				createM("m5", "ms2", "v2"),
				createM("m6", "ms2", "v2"),
			},
			scaleIntents: map[string]int32{
				"ms2": 6, // +3 => MD expect 6, has currently 6 replicas, +3 replica it is using maxSurge 3
			},
			upToDateResults: map[string]mdutil.UpToDateResult{
				"ms1": {EligibleForInPlaceUpdate: true},
			},
			expectedCanUpdateCalls: map[string]bool{
				"ms1": true,
			},
			canUpdateAnswer: map[string]bool{
				"ms1": true,
			},
			expectMoveFromMS:   []string{"ms1"},
			expectScaleIntents: map[string]int32{
				// "ms2": 6, +3 replicas from maxSurge dropped, newMS is already scaling up
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			canUpdateCalls := make(map[string]bool)

			planner := newRolloutPlanner()
			planner.md = tc.md
			planner.newMS = tc.newMS
			planner.oldMSs = tc.oldMS
			planner.machines = tc.machines
			planner.scaleIntents = tc.scaleIntents
			planner.upToDateResults = tc.upToDateResults
			planner.overrideCanUpdateMachineSetInPlace = func(oldMS *clusterv1.MachineSet) bool {
				canUpdateCalls[oldMS.Name] = true
				return tc.canUpdateAnswer[oldMS.Name]
			}

			err := planner.reconcileInPlaceUpdateIntent(ctx)
			g.Expect(err).ToNot(HaveOccurred(), "Got unexpected error from reconcileInPlaceUpdateIntent")

			g.Expect(planner.scaleIntents).To(Equal(tc.expectScaleIntents), "Unexpected scaleIntents")
			g.Expect(canUpdateCalls).To(Equal(tc.expectedCanUpdateCalls), "Unexpected canUpdateCalls")

			moveFromMS := sets.Set[string]{}.Insert(tc.expectMoveFromMS...)
			if len(moveFromMS) > 0 {
				g.Expect(planner.newMS.Annotations).To(HaveKeyWithValue(clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation, sortAndJoin(moveFromMS.UnsortedList())), "Unexpected annotation on newMS")
			} else {
				g.Expect(planner.newMS.Annotations).ToNot(HaveKey(clusterv1.MachineSetReceiveMachinesFromMachineSetsAnnotation), "Unexpected annotation on newMS")
			}
			for _, oldMS := range tc.oldMS {
				if moveFromMS.Has(oldMS.Name) {
					g.Expect(oldMS.Annotations).To(HaveKeyWithValue(clusterv1.MachineSetMoveMachinesToMachineSetAnnotation, planner.newMS.Name), "Unexpected annotation on oldMS")
				} else {
					g.Expect(oldMS.Annotations).ToNot(HaveKey(clusterv1.MachineSetMoveMachinesToMachineSetAnnotation), "Unexpected annotation on oldMS")
				}
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
		skipMaxUnavailabilityCheck bool
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
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			p := &rolloutPlanner{
				newMS:        tt.newMS,
				oldMSs:       tt.oldMSs,
				scaleIntents: tt.scaleIntent,
			}
			p.reconcileDeadlockBreaker(ctx)
			g.Expect(p.scaleIntents).To(Equal(tt.expectScaleIntent), "unexpected scaleIntents")
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
	// e.g. desiredMachineNames "m1","m2","m3" (desired machine names after rollout performed using in-place upgrade for an MD with currentMachineNames "m1","m2","m3")
	desiredMachineNames []string

	// overrideCanUpdateMachineSetInPlaceFunc allows to inject a function that will be used to perform the canUpdateMachineSetInPlace decision
	overrideCanUpdateMachineSetInPlaceFunc func(oldMS *clusterv1.MachineSet) bool

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

	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(5), textlogger.Output(os.Stdout))))

	tests := []rollingUpdateSequenceTestCase{
		// Regular rollout (no in-place)

		{ // scale out by 1
			name:                "Regular rollout, 3 Replicas, maxSurge 1, maxUnavailable 0",
			maxSurge:            1,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m4", "m5", "m6"},
		},
		{ // scale in by 1
			name:                "Regular rollout, 3 Replicas, maxSurge 0, maxUnavailable 1",
			maxSurge:            0,
			maxUnavailable:      1,
			currentMachineNames: []string{"m1", "m2", "m3"},
			desiredMachineNames: []string{"m4", "m5", "m6"},
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable)
			name:                "Regular rollout, 6 Replicas, maxSurge 3, maxUnavailable 1",
			maxSurge:            3,
			maxUnavailable:      1,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // scale out by 1, scale in by 3 (maxSurge < maxUnavailable)
			name:                "Regular rollout, 6 Replicas, maxSurge 1, maxUnavailable 3",
			maxSurge:            1,
			maxUnavailable:      3,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // scale out by 10 (maxSurge >= replicas)
			name:                "Regular rollout, 6 Replicas, maxSurge 10, maxUnavailable 0",
			maxSurge:            10,
			maxUnavailable:      0,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
		},
		{ // scale in by 10 (maxUnavailable >= replicas)
			name:                "Regular rollout, 6 Replicas, maxSurge 0, maxUnavailable 10",
			maxSurge:            0,
			maxUnavailable:      10,
			currentMachineNames: []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames: []string{"m7", "m8", "m9", "m10", "m11", "m12"},
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
			maxUnavailableBreachToleration: maxUnavailableBreachToleration(),
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
			maxSurgeBreachToleration: maxSurgeToleration(),
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
			desiredMachineNames: []string{"m10", "m11", "m12", "m13", "m14", "m15"}, // NOTE: Machines created before the spec change are deleted
		},

		// Rollout with In-place updates

		{ // scale out by 1
			name:                                   "In-place rollout, 3 Replicas, maxSurge 1, MaxUnavailable 0",
			maxSurge:                               1,
			maxUnavailable:                         0,
			currentMachineNames:                    []string{"m1", "m2", "m3"},
			desiredMachineNames:                    []string{"m1", "m2", "m4"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
		},
		{ // scale in by 1
			name:                                   "In-place rollout, 3 Replicas, maxSurge 0, MaxUnavailable 1",
			maxSurge:                               0,
			maxUnavailable:                         1,
			currentMachineNames:                    []string{"m1", "m2", "m3"},
			desiredMachineNames:                    []string{"m1", "m2", "m3"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
		},
		{ // scale out by 3, scale in by 1 (maxSurge > maxUnavailable)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 3, MaxUnavailable 1",
			maxSurge:                               3,
			maxUnavailable:                         1,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
		},
		{ // scale out by 1, scale in by 3 (maxSurge < maxUnavailable)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 1, MaxUnavailable 3",
			maxSurge:                               1,
			maxUnavailable:                         3,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
		},
		{ // scale out by 10 (maxSurge >= replicas)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 10, MaxUnavailable 0",
			maxSurge:                               10,
			maxUnavailable:                         0,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m7"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
		},
		{ // scale in by 10 (maxUnavailable >= replicas)
			name:                                   "In-place rollout, 6 Replicas, maxSurge 0, MaxUnavailable 10",
			maxSurge:                               0,
			maxUnavailable:                         10,
			currentMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			desiredMachineNames:                    []string{"m1", "m2", "m3", "m4", "m5", "m6"},
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
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
			maxUnavailableBreachToleration:         maxUnavailableBreachToleration(),
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
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
			maxSurgeBreachToleration:               maxSurgeToleration(),
			overrideCanUpdateMachineSetInPlaceFunc: oldMSCanAlwaysUpdateInPlace,
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
	// uncomment this line to automatically generate/update golden files: fLogger.writeGoldenFile = true

	// Init current and desired state from test case
	current := tt.currentScope.Clone()
	if current == nil {
		current = initCurrentRolloutScope(tt.currentMachineNames, withRollingUpdateStrategy(tt.maxSurge, tt.maxUnavailable))
	}
	desired := computeDesiredRolloutScope(current, tt.desiredMachineNames)

	// Log initial state
	fLogger.Logf("[Test] Initial state\n%s", current)
	random := ""
	if tt.randomControllerOrder {
		random = fmt.Sprintf(", random(%d)", tt.seed)
	}
	fLogger.Logf("[Test] Rollout %d replicas, MaxSurge=%d, MaxUnavailable=%d%s\n", len(current.machines()), tt.maxSurge, tt.maxUnavailable, random)
	i := 1
	maxIterations := tt.maxIterations
	for {
		taskList := getTaskListRollingUpdate(current)
		taskCount := len(taskList)
		taskOrder := defaultTaskOrder(taskCount)
		if tt.randomControllerOrder {
			taskOrder = randomTaskOrder(taskCount, rng)
		}
		for _, taskID := range taskOrder {
			task := taskList[taskID]
			if task == "md" {
				fLogger.Logf("[MD controller] Iteration %d, Reconcile md", i)
				fLogger.Logf("[MD controller] - Input to rollout planner\n%s", current)

				// Running a small subset of MD reconcile (the rollout logic and a bit of setReplicas)
				p := newRolloutPlanner()
				p.overrideCanUpdateMachineSetInPlace = tt.overrideCanUpdateMachineSetInPlaceFunc
				p.overrideComputeDesiredMS = func(ctx context.Context, deployment *clusterv1.MachineDeployment, currentNewMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
					log := ctrl.LoggerFrom(ctx)
					desiredNewMS := currentNewMS
					if currentNewMS == nil {
						// uses a predictable MS name when creating newMS, also add the newMS to current.machineSets
						totMS := len(current.machineSets)
						desiredNewMS = createMS(fmt.Sprintf("ms%d", totMS+1), deployment.Spec.Template.Spec.FailureDomain, 0)
						current.machineSets = append(current.machineSets, desiredNewMS)
						log.V(5).Info(fmt.Sprintf("Computing new MachineSet %s with %d replicas", desiredNewMS.Name, ptr.Deref(desiredNewMS.Spec.Replicas, 0)), "MachineSet", klog.KObj(desiredNewMS))
					}
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
						ms.Spec.Replicas = ptr.To(scaleIntent)
					}
				}

				// Running a small subset of setReplicas (we don't want to run the full func to avoid unnecessary noise on the test)
				current.machineDeployment.Status.Replicas = mdutil.GetActualReplicaCountForMachineSets(current.machineSets)
				current.machineDeployment.Status.AvailableReplicas = mdutil.GetAvailableReplicaCountForMachineSets(current.machineSets)

				// Log state after this reconcile
				fLogger.Logf("[MD controller] - Result of rollout planner\n%s", current)

				// Check we are not breaching rollout constraints
				minAvailableReplicas := ptr.Deref(current.machineDeployment.Spec.Replicas, 0) - mdutil.MaxUnavailable(*current.machineDeployment)
				totAvailableReplicas := ptr.Deref(current.machineDeployment.Status.AvailableReplicas, 0)
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
				totReplicas := mdutil.TotalMachineSetsReplicaSum(current.machineSets)
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
					fLogger.Logf("[MS controller] Iteration %d, Reconcile %s, %s", i, ms.Name, msLog(ms, current.machineSetMachines[ms.Name]))
					err := machineSetControllerMutator(fLogger, ms, current)
					g.Expect(err).ToNot(HaveOccurred())
					break
				}
			}
		}

		// Check if we are at the desired state
		if current.Equal(desired) {
			fLogger.Logf("[Test] Final state\n%s", current)
			break
		}

		// Safeguard for infinite reconcile
		i++
		if i > maxIterations {
			// NOTE: the following can be used to set a breakpoint for debugging why the system is not reaching desired state after maxIterations (to check what is not yet equal)
			current.Equal(desired)
			// Log desired state we never reached
			fLogger.Logf("[Test] Desired state\n%s", desired)
			g.Fail(fmt.Sprintf("Failed to reach desired state in %d iterations", maxIterations))
		}
	}

	if !tt.skipLogToFileAndGoldenFileCheck {
		currentLog, goldenLog, err := fLogger.WriteLogAndCompareWithGoldenFile()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(currentLog).To(Equal(goldenLog), "current test case log and golden test case log are different\n%s", cmp.Diff(currentLog, goldenLog))
	}
}

func oldMSCanAlwaysUpdateInPlace(_ *clusterv1.MachineSet) bool {
	return true
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
