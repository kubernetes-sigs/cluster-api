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

package controllers

import (
	"strconv"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/internal/mdutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			name: "It fails when machineDeployment has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			error: errors.Errorf("spec.replicas for MachineDeployment foo/bar is nil, this is unexpected"),
		},
		{
			name: "It fails when new machineSet has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
			},
			error: errors.Errorf("spec.replicas for MachineSet foo/bar is nil, this is unexpected"),
		},
		{
			name: "RollingUpdate strategy: Scale up: 0 -> 2",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(2),
						},
					},
					Replicas: pointer.Int32Ptr(2),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(0),
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(2),
						},
					},
					Replicas: pointer.Int32Ptr(0),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(2),
						},
					},
					Replicas: pointer.Int32Ptr(3),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(1),
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
						Replicas: pointer.Int32Ptr(3),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas: 3,
					},
				},
			},
			error: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			resources := []client.Object{
				tc.machineDeployment,
			}

			allMachineSets := append(tc.oldMachineSets, tc.newMachineSet)
			for key := range allMachineSets {
				resources = append(resources, allMachineSets[key])
			}

			r := &MachineDeploymentReconciler{
				Client:   fake.NewClientBuilder().WithObjects(resources...).Build(),
				recorder: record.NewFakeRecorder(32),
			}

			err := r.reconcileNewMachineSet(ctx, allMachineSets, tc.newMachineSet, tc.machineDeployment)
			if tc.error != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(BeEquivalentTo(tc.error.Error()))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			freshNewMachineSet := &clusterv1.MachineSet{}
			err = r.Client.Get(ctx, client.ObjectKeyFromObject(tc.newMachineSet), freshNewMachineSet)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(*freshNewMachineSet.Spec.Replicas).To(BeEquivalentTo(tc.expectedNewMachineSetReplicas))

			desiredReplicasAnnotation, ok := freshNewMachineSet.GetAnnotations()[clusterv1.DesiredReplicasAnnotation]
			g.Expect(ok).To(BeTrue())
			g.Expect(strconv.Atoi(desiredReplicasAnnotation)).To(BeEquivalentTo(*tc.machineDeployment.Spec.Replicas))

			maxReplicasAnnotation, ok := freshNewMachineSet.GetAnnotations()[clusterv1.MaxReplicasAnnotation]
			g.Expect(ok).To(BeTrue())
			g.Expect(strconv.Atoi(maxReplicasAnnotation)).To(BeEquivalentTo(*tc.machineDeployment.Spec.Replicas + mdutil.MaxSurge(*tc.machineDeployment)))
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
			name: "It fails when machineDeployment has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			error: errors.Errorf("spec.replicas for MachineDeployment foo/bar is nil, this is unexpected"),
		},
		{
			name: "It fails when new machineSet has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
			},
			error: errors.Errorf("spec.replicas for MachineSet foo/bar is nil, this is unexpected"),
		},
		{
			name: "RollingUpdate strategy: Scale down old MachineSets when all new replicas are available",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(1),
							MaxSurge:       intOrStrPtr(3),
						},
					},
					Replicas: pointer.Int32Ptr(2),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(0),
				},
				Status: clusterv1.MachineSetStatus{
					AvailableReplicas: 2,
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "2replicas",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: pointer.Int32Ptr(2),
					},
					Status: clusterv1.MachineSetStatus{
						AvailableReplicas: 2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "1replicas",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: pointer.Int32Ptr(1),
					},
					Status: clusterv1.MachineSetStatus{
						AvailableReplicas: 1,
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(2),
							MaxSurge:       intOrStrPtr(3),
						},
					},
					Replicas: pointer.Int32Ptr(10),
				},
			},
			newMachineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(5),
				},
				Status: clusterv1.MachineSetStatus{
					Replicas:          5,
					ReadyReplicas:     0,
					AvailableReplicas: 0,
				},
			},
			oldMachineSets: []*clusterv1.MachineSet{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "foo",
						Name:      "8replicas",
					},
					Spec: clusterv1.MachineSetSpec{
						Replicas: pointer.Int32Ptr(8),
					},
					Status: clusterv1.MachineSetStatus{
						Replicas:          10,
						ReadyReplicas:     8,
						AvailableReplicas: 8,
					},
				},
			},
			expectedOldMachineSetsReplicas: 8,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			resources := []client.Object{
				tc.machineDeployment,
			}

			allMachineSets := append(tc.oldMachineSets, tc.newMachineSet)
			for key := range allMachineSets {
				resources = append(resources, allMachineSets[key])
			}

			r := &MachineDeploymentReconciler{
				Client:   fake.NewClientBuilder().WithObjects(resources...).Build(),
				recorder: record.NewFakeRecorder(32),
			}

			err := r.reconcileOldMachineSets(ctx, allMachineSets, tc.oldMachineSets, tc.newMachineSet, tc.machineDeployment)
			if tc.error != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(BeEquivalentTo(tc.error.Error()))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			for key := range tc.oldMachineSets {
				freshOldMachineSet := &clusterv1.MachineSet{}
				err = r.Client.Get(ctx, client.ObjectKeyFromObject(tc.oldMachineSets[key]), freshOldMachineSet)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(*freshOldMachineSet.Spec.Replicas).To(BeEquivalentTo(tc.expectedOldMachineSetsReplicas))
			}
		})
	}
}
