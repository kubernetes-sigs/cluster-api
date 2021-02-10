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
	"testing"
	"time"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newDeploymentStatus(replicas, updatedReplicas, availableReplicas int32) clusterv1.MachineDeploymentStatus {
	return clusterv1.MachineDeploymentStatus{
		Replicas:          replicas,
		UpdatedReplicas:   updatedReplicas,
		AvailableReplicas: availableReplicas,
	}
}

// assumes the retuned deployment is always observed - not needed to be tested here.
func currentDeployment(pds *int32, replicas, statusReplicas, updatedReplicas, availableReplicas int32, conditions clusterv1.Conditions) *clusterv1.MachineDeployment {
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
		Status: newDeploymentStatus(statusReplicas, updatedReplicas, availableReplicas),
	}
	d.Status.Conditions = conditions
	return d
}

// helper to create MS with given availableReplicas
func newMSWithAvailable(name string, specReplicas, statusReplicas, availableReplicas int32) *clusterv1.MachineSet {
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
			Selector:          "",
			AvailableReplicas: int32(availableReplicas),
			Replicas:          int32(statusReplicas),
		},
	}
}

func TestMachineDeploymentSyncDeploymentStatus(t *testing.T) {
	pds := int32(60)
	testTime := metav1.Date(2017, 2, 15, 18, 49, 00, 00, time.UTC)
	failedTimedOut := clusterv1.Condition{
		Type:   clusterv1.MachineDeploymentProgressing,
		Status: corev1.ConditionFalse,
		Reason: clusterv1.TimedOutReason,
	}
	newMSAvailable := clusterv1.Condition{
		Type:               clusterv1.MachineDeploymentProgressing,
		Status:             corev1.ConditionTrue,
		Reason:             clusterv1.NewMSAvailableReason,
		LastTransitionTime: testTime,
	}
	machineSetUpdated := clusterv1.Condition{
		Type:               clusterv1.MachineDeploymentProgressing,
		Status:             corev1.ConditionTrue,
		Reason:             clusterv1.MachineSetUpdatedReason,
		LastTransitionTime: testTime,
	}
	tests := []struct {
		name                           string
		d                              *clusterv1.MachineDeployment
		allMSs                         []*clusterv1.MachineSet
		newMS                          *clusterv1.MachineSet
		expectCond                     bool
		expectedCondType               clusterv1.ConditionType
		expectedCondStatus             corev1.ConditionStatus
		expectedCondReason             string
		expectedCondLastTransitionTime metav1.Time
	}{
		{
			name:             "General: remove Progressing condition and do not estimate progress if machinedeployment has no Progress Deadline",
			d:                currentDeployment(nil, 3, 2, 2, 2, clusterv1.Conditions{machineSetUpdated}),
			allMSs:           []*clusterv1.MachineSet{newMSWithAvailable("bar", 0, 1, 1)},
			newMS:            newMSWithAvailable("foo", 3, 2, 2),
			expectCond:       false,
			expectedCondType: clusterv1.MachineDeploymentProgressing,
		},
		{
			name:                           "General: do not estimate progress of machinedeployment with only one active machineset",
			d:                              currentDeployment(&pds, 3, 3, 3, 3, clusterv1.Conditions{newMSAvailable}),
			allMSs:                         []*clusterv1.MachineSet{newMSWithAvailable("bar", 3, 3, 3)},
			expectCond:                     true,
			expectedCondType:               clusterv1.MachineDeploymentProgressing,
			expectedCondStatus:             corev1.ConditionTrue,
			expectedCondReason:             clusterv1.NewMSAvailableReason,
			expectedCondLastTransitionTime: testTime,
		},
		{
			name:               "DeploymentProgressing: create Progressing condition if it does not exist",
			d:                  currentDeployment(&pds, 3, 2, 2, 2, clusterv1.Conditions{}),
			allMSs:             []*clusterv1.MachineSet{newMSWithAvailable("bar", 0, 1, 1)},
			newMS:              newMSWithAvailable("foo", 3, 2, 2),
			expectCond:         true,
			expectedCondType:   clusterv1.MachineDeploymentProgressing,
			expectedCondStatus: corev1.ConditionTrue,
			expectedCondReason: clusterv1.MachineSetUpdatedReason,
		},
		{
			name:               "DeploymentComplete: create Progressing condition if it does not exist",
			d:                  currentDeployment(&pds, 3, 3, 3, 3, clusterv1.Conditions{}),
			allMSs:             []*clusterv1.MachineSet{},
			newMS:              newMSWithAvailable("foo", 3, 3, 3),
			expectCond:         true,
			expectedCondType:   clusterv1.MachineDeploymentProgressing,
			expectedCondStatus: corev1.ConditionTrue,
			expectedCondReason: clusterv1.NewMSAvailableReason,
		},
		{
			name:               "DeploymentTimedOut: update status if rollout exceeds Progress Deadline",
			d:                  currentDeployment(&pds, 3, 2, 2, 2, clusterv1.Conditions{machineSetUpdated}),
			allMSs:             []*clusterv1.MachineSet{},
			newMS:              newMSWithAvailable("foo", 3, 2, 2),
			expectCond:         true,
			expectedCondType:   clusterv1.MachineDeploymentProgressing,
			expectedCondStatus: corev1.ConditionFalse,
			expectedCondReason: clusterv1.TimedOutReason,
		},
		{
			name:                           "DeploymentTimedOut: do not update status if deployment has existing timedOut condition",
			d:                              currentDeployment(&pds, 3, 2, 2, 2, clusterv1.Conditions{failedTimedOut}),
			allMSs:                         []*clusterv1.MachineSet{},
			newMS:                          newMSWithAvailable("foo", 3, 2, 2),
			expectCond:                     true,
			expectedCondType:               clusterv1.MachineDeploymentProgressing,
			expectedCondStatus:             corev1.ConditionFalse,
			expectedCondReason:             clusterv1.TimedOutReason,
			expectedCondLastTransitionTime: testTime,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &MachineDeploymentReconciler{
				Client:   fake.NewClientBuilder().Build(),
				recorder: record.NewFakeRecorder(32),
			}

			if test.newMS != nil {
				test.allMSs = append(test.allMSs, test.newMS)
			}

			err := r.syncDeploymentStatus(context.Background(), test.allMSs, test.newMS, test.d)
			g.Expect(err).ToNot(HaveOccurred())

			newCond := conditions.Get(test.d, test.expectedCondType)
			if !test.expectCond {
				g.Expect(newCond).To(BeNil())
			} else {
				g.Expect(newCond.Type).To(Equal(test.expectedCondType))
				g.Expect(newCond.Status).To(Equal(test.expectedCondStatus))
				g.Expect(newCond.Reason).To(Equal(test.expectedCondReason))
				if !test.expectedCondLastTransitionTime.IsZero() && test.expectedCondLastTransitionTime != testTime {
					g.Expect(newCond.LastTransitionTime).To(Equal(test.expectedCondLastTransitionTime))
				}
			}
		})
	}
}

func TestMachineDeploymentCalculateStatus(t *testing.T) {
	msStatusError := capierrors.MachineSetStatusError("some failure")
	testDeployment := clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 2,
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Replicas: pointer.Int32Ptr(2),
			Strategy: &clusterv1.MachineDeploymentStrategy{
				Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
				RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
					MaxUnavailable: intOrStrPtr(0),
					MaxSurge:       intOrStrPtr(1),
					DeletePolicy:   pointer.StringPtr("Oldest"),
				},
			},
		},
	}

	var tests = map[string]struct {
		machineSets    []*clusterv1.MachineSet
		newMachineSet  *clusterv1.MachineSet
		deployment     clusterv1.MachineDeployment
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
			deployment: testDeployment,
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   2,
				UnavailableReplicas: 0,
				Phase:               "Running",
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:    "MachineDeploymentAvailable",
						Status:  "True",
						Reason:  "MinimumMachinesAvailable",
						Message: "MachineDeployment has minimum availability",
					},
				},
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
			deployment: testDeployment,
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       1,
				AvailableReplicas:   1,
				UnavailableReplicas: 1,
				Phase:               "ScalingUp",
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:    "MachineDeploymentAvailable",
						Status:  "False",
						Reason:  "MinimumMachinesUnavailable",
						Message: "MachineDeployment does not have minimum availability",
					},
				},
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
			deployment: testDeployment,
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   3,
				UnavailableReplicas: 0,
				Phase:               "ScalingDown",
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:    "MachineDeploymentAvailable",
						Status:  "True",
						Reason:  "MinimumMachinesAvailable",
						Message: "MachineDeployment has minimum availability",
					},
				},
			},
		},
		"machine set failed": {
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
			deployment: testDeployment,
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       0,
				AvailableReplicas:   0,
				UnavailableReplicas: 2,
				Phase:               "Failed",
				Conditions: clusterv1.Conditions{
					clusterv1.Condition{
						Type:    "MachineDeploymentAvailable",
						Status:  "False",
						Reason:  "MinimumMachinesUnavailable",
						Message: "MachineDeployment does not have minimum availability",
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			actualStatus := calculateStatus(test.machineSets, test.newMachineSet, &test.deployment)
			g.Expect(actualStatus).To(Equal(test.expectedStatus))
		})
	}
}
