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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
)

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

			// TODO(in-place): Restore tests on DisableMachineCreateAnnotation and MaxReplicasAnnotation as soon as handling those annotation is moved into the rollout planner
			// 	_, ok := freshNewMachineSet.GetAnnotations()[clusterv1.DisableMachineCreateAnnotation]
			// 	g.Expect(ok).To(BeFalse())
			//
			// 	desiredReplicasAnnotation, ok := freshNewMachineSet.GetAnnotations()[clusterv1.DesiredReplicasAnnotation]
			// 	g.Expect(ok).To(BeTrue())
			// 	g.Expect(strconv.Atoi(desiredReplicasAnnotation)).To(BeEquivalentTo(*tc.machineDeployment.Spec.Replicas))
			//
			// 	maxReplicasAnnotation, ok := freshNewMachineSet.GetAnnotations()[clusterv1.MaxReplicasAnnotation]
			// 	g.Expect(ok).To(BeTrue())
			// 	g.Expect(strconv.Atoi(maxReplicasAnnotation)).To(BeEquivalentTo(*tc.machineDeployment.Spec.Replicas + mdutil.MaxSurge(*tc.machineDeployment)))
		})
	}
}

func Test_reconcileOldMachineSetsRolloutRolling(t *testing.T) {
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
			md:          createMD("v2", 10, withRolloutStrategy(1, 0)),
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
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 2 available replicas from ms1 + 1 available replica from ms22 = 3 available replicas <= minAvailability, we cannot scale down
			},
		},
		{
			name:        "do not scale down if replicas is less then minAvailable replicas",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(1)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 1 available replicas from ms1 + 1 available replica from ms22 = 3 available replicas = minAvailability, we cannot scale down
			},
			skipMaxUnavailabilityCheck: true,
		},
		{
			name:        "do not scale down if there are more replicas than minAvailable replicas, but scale down from a previous reconcile already takes the availability buffer",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(3), withStatusAvailableReplicas(3)), // OldMS is scaling down from a previous reconcile
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 3 available replicas from ms1 - 1 replica already scaling down from ms1 + 1 available replica from ms2 = 3 available replicas = minAvailability, we cannot further scale down
			},
		},
		{
			name:        "do not scale down if there are more replicas than minAvailable replicas, but scale down from a previous reconcile already takes the availability buffer, scale down from a previous reconcile on another MSr",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 6, withRolloutStrategy(3, 1)),
			newMS:       createMS("ms3", "v2", 3, withStatusReplicas(0), withStatusAvailableReplicas(0)), // NewMS is scaling up from previous reconcile, but replicas do not exists yet
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(3), withStatusAvailableReplicas(3)), // OldMS is scaling down from a previous reconcile
				createMS("ms2", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)), // OldMS
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 3 available replicas from ms1 - 1 replica already scaling down from ms1 + 3 available replica from ms2 = 5 available replicas = minAvailability, we cannot further scale down
			},
		},
		{
			name: "do not scale down if there are more replicas than minAvailable replicas, but scale down from current reconcile already takes the availability buffer",
			scaleIntent: map[string]int32{
				"ms2": 1, // newMS (ms2) has a scaling down intent from current reconcile
			},
			md:    createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS: createMS("ms2", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			},
			expectScaleIntent: map[string]int32{
				"ms2": 1,
				// no new scale down intent for oldMSs (ms1):
				// 2 available replicas from ms1 + 2 available replicas from ms2 - 1 replica already scaling down from ms2 = 3 available replicas = minAvailability, we cannot further scale down
			},
		},
		{
			name:        "do not scale down replicas when there are more replicas than minAvailable replicas, but not all the replicas are available (unavailability on newMS)",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(3)),
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 3 available replicas from ms1 + 0 available replicas from ms2 = 3 available replicas = minAvailability, we cannot further scale down
			},
		},
		{
			name:        "do not scale down replicas when there are more replicas than minAvailable replicas, but not all the replicas are available (unavailability on oldMS)",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms2", "v2", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v1", 3, withStatusReplicas(3), withStatusAvailableReplicas(2)), // only 2 replicas are available
			},
			expectScaleIntent: map[string]int32{
				// no new scale down intent for oldMSs (ms1):
				// 2 available replicas from ms1 + 1 available replicas from ms2 = 3 available replicas = minAvailability, we cannot further scale down
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, all replicas are available",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
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
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
				createMS("ms2", "v1", 1, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available
				createMS("ms3", "v2", 2, withStatusReplicas(2), withStatusAvailableReplicas(0)), // no replicas are available
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				// ms1 skipped in the first iteration because it does not have any unavailable replica
				"ms2": 0, // 0 available replicas from ms2, it can be scaled down with impact on availability (-1)
				"ms3": 0, // 0 available replicas from ms3, it can be scaled down with impact on availability (-2)
				// no need to further scale down.
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, available replicas are scaled down when unavailable replicas are gone",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
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
				"ms3": 0, // 0 available replicas from ms3, it can be scaled down to 0 without any impact on availability (-1)
				"ms1": 2, // 3 available replicas from ms2 + 1 available replica from ms4 = 4 available replicas > minAvailability, scale down to 2 replicas (-1)
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, available replicas are scaled down when unavailable replicas are gone is not affected by replicas without machines",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 4, withStatusReplicas(3), withStatusAvailableReplicas(3)), // 1 replica without machine
				createMS("ms2", "v1", 2, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available, 1 replica without machine
				createMS("ms3", "v2", 5, withStatusReplicas(2), withStatusAvailableReplicas(0)), // no replicas are available, 2 replicas without machines
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				// ms1 skipped in the first iteration because it does not have any unavailable replica
				"ms2": 0, // 0 available replicas from ms2, it can be scaled down to 0 without any impact on availability (-1)
				"ms3": 0, // 0 available replicas from ms3, it can be scaled down to 0 without any impact on availability (-5)
				"ms1": 2, // 3 available replicas from ms2 + 1 available replica from ms4 = 4 available replicas > minAvailability, scale down to 2 replicas, also drop the replica without machines (-2)
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, scale down stops before breaching minAvailable replicas",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
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
				// does not make sense to continue scale down.
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, unavailable replicas are scaled down first, scale down stops before breaching minAvailable replicas is not affected by replicas without machines",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms4", "v3", 1, withStatusReplicas(1), withStatusAvailableReplicas(1)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 2, withStatusReplicas(1), withStatusAvailableReplicas(1)), // 1 replica without machine
				createMS("ms2", "v1", 3, withStatusReplicas(1), withStatusAvailableReplicas(0)), // no replicas are available, 2 replica without machines
				createMS("ms3", "v2", 3, withStatusReplicas(2), withStatusAvailableReplicas(1)), // only 1 replica is available, 1 replica without machine
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				"ms1": 1, // 1 replica without machine, it can be scaled down to 1 without any impact on availability (-1)
				"ms2": 0, // 1 replica without machine, 0 available replicas from ms2, it can be scaled down to 0 without any impact on availability (-2)
				"ms3": 2, // 1 replica without machine, it can be scaled down to 2 without any impact on availability (-1); even if there is still room to scale down, we cannot further scale down ms3: 1 available replica from ms1 + 1 available replica from ms4 = 2 available replicas < minAvailability
				// does not make sense to continue scale down.
			},
		},
		{
			name:        "scale down replicas when there are more replicas than minAvailable replicas, scale down keeps into account scale downs from a previous reconcile",
			scaleIntent: map[string]int32{},
			md:          createMD("v2", 3, withRolloutStrategy(1, 0)),
			newMS:       createMS("ms2", "v3", 2, withStatusReplicas(2), withStatusAvailableReplicas(2)),
			oldMSs: []*clusterv1.MachineSet{
				createMS("ms1", "v0", 3, withStatusReplicas(4), withStatusAvailableReplicas(4)), // OldMS is scaling down from a previous reconcile
			},
			expectScaleIntent: map[string]int32{
				// new scale down intent for oldMSs:
				"ms1": 2, // 4 available replicas from ms1 - 1 replica already scaling down from ms1 + 2 available replicas from ms2 = 4 available replicas > minAvailability, scale down to 2 (-1)
			},
		},
		{
			name: "scale down replicas when there are more replicas than minAvailable replicas, scale down keeps into account scale downs from the current reconcile",
			scaleIntent: map[string]int32{
				"ms2": 1, // newMS (ms2) has a scaling down intent from current reconcile
			},
			md:    createMD("v2", 3, withRolloutStrategy(1, 0)),
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
			err := p.reconcileOldMachineSetsRolloutRolling(ctx)
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
				g.Expect(totAvailableReplicas).To(BeNumerically(">=", minAvailableReplicas), "totAvailable machines is less than MinUnavailable")
			} else {
				t.Logf("skipping MaxUnavailability check (totAvailableReplicas: %d, minAvailableReplicas: %d)", totAvailableReplicas, minAvailableReplicas)
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
				createMS("ms1", "v1", 0, withStatusReplicas(0), withStatusAvailableReplicas(0)),
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
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(5)),
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
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // one unavailable replica, potential deadlock
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
				createMS("ms1", "v1", 4, withStatusReplicas(5), withStatusAvailableReplicas(4)), // one unavailable replica, potential deadlock, scale down from a previous reconcile
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
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // one unavailable replica, potential deadlock
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
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // one unavailable replica, potential deadlock
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
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(4)), // one unavailable replica, potential deadlock
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
				createMS("ms1", "v1", 5, withStatusReplicas(5), withStatusAvailableReplicas(3)), // one unavailable replica, potential deadlock
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
