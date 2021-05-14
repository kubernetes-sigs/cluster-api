/*
Copyright 2019 The Kubernetes Authors.

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
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/mdutil"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCalculateStatus(t *testing.T) {
	msStatusError := capierrors.MachineSetStatusError("some failure")

	var tests = map[string]struct {
		machineSets    []*clusterv1.MachineSet
		newMachineSet  *clusterv1.MachineSet
		deployment     *clusterv1.MachineDeployment
		expectedStatus clusterv1.MachineDeploymentStatus
	}{
		"all machines are running": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  2,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  2,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   2,
				UnavailableReplicas: 0,
				Phase:               "Running",
			},
		},
		"scaling up": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  1,
					ReadyReplicas:      1,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  1,
					ReadyReplicas:      1,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       1,
				AvailableReplicas:   1,
				UnavailableReplicas: 1,
				Phase:               "ScalingUp",
			},
		},
		"scaling down": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  3,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  3,
					ReadyReplicas:      2,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   3,
				UnavailableReplicas: 0,
				Phase:               "ScalingDown",
			},
		},
		"MachineSet failed": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  0,
					ReadyReplicas:      0,
					Replicas:           2,
					ObservedGeneration: 1,
					FailureReason:      &msStatusError,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  0,
					ReadyReplicas:      0,
					Replicas:           2,
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       0,
				AvailableReplicas:   0,
				UnavailableReplicas: 2,
				Phase:               "Failed",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			actualStatus := calculateStatus(test.machineSets, test.newMachineSet, test.deployment)
			g.Expect(actualStatus).To(Equal(test.expectedStatus))
		})
	}
}

func TestScaleMachineSet(t *testing.T) {
	testCases := []struct {
		name              string
		machineDeployment *clusterv1.MachineDeployment
		machineSet        *clusterv1.MachineSet
		newScale          int32
		error             error
	}{
		{
			name: "It fails when new MachineSet has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
			},
			error: errors.Errorf("spec.replicas for MachineSet foo/bar is nil, this is unexpected"),
		},
		{
			name: "It fails when new MachineDeployment has no replicas",
			machineDeployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineDeploymentSpec{},
			},
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			error: errors.Errorf("spec.replicas for MachineDeployment foo/bar is nil, this is unexpected"),
		},
		{
			name: "Scale up",
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
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(0),
				},
			},
			newScale: 2,
		},
		{
			name: "Scale down",
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
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(4),
				},
			},
			newScale: 2,
		},
		{
			name: "Same replicas does not scale",
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
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
			},
			newScale: 2,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			resources := []client.Object{
				tc.machineDeployment,
				tc.machineSet,
			}

			r := &MachineDeploymentReconciler{
				Client:   fake.NewClientBuilder().WithObjects(resources...).Build(),
				recorder: record.NewFakeRecorder(32),
			}

			err := r.scaleMachineSet(context.Background(), tc.machineSet, tc.newScale, tc.machineDeployment)
			if tc.error != nil {
				g.Expect(err.Error()).To(BeEquivalentTo(tc.error.Error()))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			freshMachineSet := &clusterv1.MachineSet{}
			err = r.Client.Get(ctx, client.ObjectKeyFromObject(tc.machineSet), freshMachineSet)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(*freshMachineSet.Spec.Replicas).To(BeEquivalentTo(tc.newScale))

			expectedMachineSetAnnotations := map[string]string{
				clusterv1.DesiredReplicasAnnotation: fmt.Sprintf("%d", *tc.machineDeployment.Spec.Replicas),
				clusterv1.MaxReplicasAnnotation:     fmt.Sprintf("%d", (*tc.machineDeployment.Spec.Replicas)+mdutil.MaxSurge(*tc.machineDeployment)),
			}
			g.Expect(freshMachineSet.GetAnnotations()).To(BeEquivalentTo(expectedMachineSetAnnotations))
		})
	}
}

func newTestMachineDeployment(pds *int32, replicas, statusReplicas, updatedReplicas, availableReplicas int32, conditions clusterv1.Conditions) *clusterv1.MachineDeployment {
	d := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "progress-test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			ProgressDeadlineSeconds: pds,
			Replicas:                &replicas,
			Strategy: &clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: intOrStrPtr(0),
					MaxSurge:       intOrStrPtr(1),
					DeletePolicy:   pointer.StringPtr("Oldest"),
				},
			},
		},
		Status: clusterv1.MachineDeploymentStatus{
			Replicas:          statusReplicas,
			UpdatedReplicas:   updatedReplicas,
			AvailableReplicas: availableReplicas,
			Conditions:        conditions,
		},
	}
	return d
}

// helper to create MS with given availableReplicas.
func newTestMachinesetWithReplicas(name string, specReplicas, statusReplicas, availableReplicas int32) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Time{},
			Namespace:         metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: pointer.Int32Ptr(specReplicas),
		},
		Status: clusterv1.MachineSetStatus{
			AvailableReplicas: availableReplicas,
			Replicas:          statusReplicas,
		},
	}
}

func TestSyncDeploymentStatus(t *testing.T) {
	pds := int32(60)
	tests := []struct {
		name               string
		d                  *clusterv1.MachineDeployment
		oldMachineSets     []*clusterv1.MachineSet
		newMachineSet      *clusterv1.MachineSet
		expectedConditions []*clusterv1.Condition
	}{
		{
			name:           "Deployment not available: MachineDeploymentAvailableCondition should exist and be false",
			d:              newTestMachineDeployment(&pds, 3, 2, 2, 2, clusterv1.Conditions{}),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 2, 2),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:     clusterv1.MachineDeploymentAvailableCondition,
					Status:   corev1.ConditionFalse,
					Severity: clusterv1.ConditionSeverityWarning,
					Reason:   clusterv1.WaitingForAvailableMachinesReason,
				},
			},
		},
		{
			name:           "Deployment Available: MachineDeploymentAvailableCondition should exist and be true",
			d:              newTestMachineDeployment(&pds, 3, 3, 3, 3, clusterv1.Conditions{}),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 3, 3),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:   clusterv1.MachineDeploymentAvailableCondition,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &MachineDeploymentReconciler{
				Client:   fake.NewClientBuilder().Build(),
				recorder: record.NewFakeRecorder(32),
			}
			allMachineSets := append(test.oldMachineSets, test.newMachineSet)
			err := r.syncDeploymentStatus(allMachineSets, test.newMachineSet, test.d)
			g.Expect(err).ToNot(HaveOccurred())
			assertConditions(t, test.d, test.expectedConditions...)
		})
	}
}
