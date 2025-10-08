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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

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
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, klog.Background())
	klog.SetOutput(ginkgo.GinkgoWriter)

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
					createMS("ms3", "v3", 0),
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
				p.computeDesiredMS = func(_ context.Context, deployment *clusterv1.MachineDeployment, currentNewMS *clusterv1.MachineSet) (*clusterv1.MachineSet, error) {
					desiredNewMS := currentNewMS
					if currentNewMS == nil {
						// uses a predictable MS name when creating newMS, also add the newMS to current.machineSets
						totMS := len(current.machineSets)
						desiredNewMS = createMS(fmt.Sprintf("ms%d", totMS+1), deployment.Spec.Template.Spec.FailureDomain, 0)
						current.machineSets = append(current.machineSets, desiredNewMS)
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
					machineSetControllerMutator(fLogger, ms, current)
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
