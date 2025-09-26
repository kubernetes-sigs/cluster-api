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
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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
			err := planner.reconcileNewMachineSet(ctx, tc.machineDeployment, tc.newMachineSet, tc.oldMachineSets)
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

func TestReconcileOldMachineSets(t *testing.T) {
	testCases := []struct {
		name                           string
		machineDeployment              *clusterv1.MachineDeployment
		newMachineSet                  *clusterv1.MachineSet
		oldMachineSets                 []*clusterv1.MachineSet
		expectedOldMachineSetsReplicas int
		error                          error
	}{
		{
			name: "RollingUpdate strategy: Scale down old MachineSets when all new replicas are available",
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
								MaxUnavailable: intOrStrPtr(1),
								MaxSurge:       intOrStrPtr(3),
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
				Status: clusterv1.MachineSetStatus{
					AvailableReplicas: ptr.To[int32](2),
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "2replicas",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](2),
					},
					Status: clusterv1.MachineSetStatus{
						AvailableReplicas: ptr.To[int32](2),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "1replicas",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](1),
					},
					Status: clusterv1.MachineSetStatus{
						AvailableReplicas: ptr.To[int32](1),
					},
				},
			},
			expectedOldMachineSetsReplicas: 0,
		},
		{
			name: "RollingUpdate strategy: It does not scale down old MachineSets when above maxUnavailable",
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
								MaxUnavailable: intOrStrPtr(2),
								MaxSurge:       intOrStrPtr(3),
							},
						},
					},
					Replicas: ptr.To[int32](10),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](5),
				},
				Status: clusterv1.MachineSetStatus{
					Replicas: ptr.To[int32](5),
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "8replicas",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: ptr.To[int32](8),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas:          ptr.To[int32](10),
						ReadyReplicas:     ptr.To[int32](8),
						AvailableReplicas: ptr.To[int32](8),
					},
				},
			},
			expectedOldMachineSetsReplicas: 8,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			planner := newRolloutPlanner()
			err := planner.reconcileOldMachineSets(ctx, tc.machineDeployment, tc.newMachineSet, tc.oldMachineSets)
			if tc.error != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(BeEquivalentTo(tc.error.Error()))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			for i := range tc.oldMachineSets {
				scaleIntent := ptr.Deref(tc.oldMachineSets[i].Spec.Replicas, 0)
				if v, ok := planner.scaleIntents[tc.oldMachineSets[i].Name]; ok {
					scaleIntent = v
				}
				g.Expect(scaleIntent).To(BeEquivalentTo(tc.expectedOldMachineSetsReplicas))
			}
		})
	}
}
