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

package machinedeployment

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
)

func TestCalculateV1Beta1Status(t *testing.T) {
	var tests = map[string]struct {
		machineSets    []*clusterv1.MachineSet
		newMachineSet  *clusterv1.MachineSet
		deployment     *clusterv1.MachineDeployment
		expectedStatus clusterv1.MachineDeploymentStatus
	}{
		"all machines are running": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 2,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 2,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				Deprecated: &clusterv1.MachineDeploymentDeprecatedStatus{
					V1Beta1: &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{
						UpdatedReplicas:     2,
						ReadyReplicas:       2,
						AvailableReplicas:   2,
						UnavailableReplicas: 0,
					},
				},
			},
		},
		"scaling up": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 1,
							ReadyReplicas:     1,
						},
					},
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 1,
							ReadyReplicas:     1,
						},
					},
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				Deprecated: &clusterv1.MachineDeploymentDeprecatedStatus{
					V1Beta1: &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{
						UpdatedReplicas:     2,
						ReadyReplicas:       1,
						AvailableReplicas:   1,
						UnavailableReplicas: 1,
					},
				},
			},
		},
		"scaling down": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 3,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			}},
			newMachineSet: &clusterv1.MachineSet{
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector: "",
					Replicas: ptr.To[int32](2),
					Deprecated: &clusterv1.MachineSetDeprecatedStatus{
						V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
							AvailableReplicas: 3,
							ReadyReplicas:     2,
						},
					},
					ObservedGeneration: 1,
				},
			},
			deployment: &clusterv1.MachineDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
				},
				Spec: clusterv1.MachineDeploymentSpec{
					Replicas: ptr.To[int32](2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				Deprecated: &clusterv1.MachineDeploymentDeprecatedStatus{
					V1Beta1: &clusterv1.MachineDeploymentV1Beta1DeprecatedStatus{
						UpdatedReplicas:     2,
						ReadyReplicas:       2,
						AvailableReplicas:   3,
						UnavailableReplicas: 0,
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			calculateV1Beta1Status(test.machineSets, test.newMachineSet, test.deployment)
			g.Expect(test.deployment.Status).To(BeComparableTo(test.expectedStatus))
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
					Replicas: ptr.To[int32](2),
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
					Replicas: ptr.To[int32](2),
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
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](0),
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
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](4),
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
			machineSet: &clusterv1.MachineSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
				Spec: clusterv1.MachineSetSpec{
					Replicas: ptr.To[int32](2),
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

			r := &Reconciler{
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
		})
	}
}

func newTestMachineDeployment(replicas, statusReplicas, upToDateReplicas, availableReplicas int32) *clusterv1.MachineDeployment {
	d := &clusterv1.MachineDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "progress-test",
		},
		Spec: clusterv1.MachineDeploymentSpec{
			Replicas: &replicas,
			Rollout: clusterv1.MachineDeploymentRolloutSpec{
				Strategy: clusterv1.MachineDeploymentRolloutStrategy{
					Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
					RollingUpdate: clusterv1.MachineDeploymentRolloutStrategyRollingUpdate{
						MaxUnavailable: intOrStrPtr(0),
						MaxSurge:       intOrStrPtr(1),
					},
				},
			},
			Deletion: clusterv1.MachineDeploymentDeletionSpec{
				Order: clusterv1.OldestMachineSetDeletionOrder,
			},
		},
		Status: clusterv1.MachineDeploymentStatus{
			Replicas:          ptr.To[int32](statusReplicas),
			UpToDateReplicas:  ptr.To[int32](upToDateReplicas),
			AvailableReplicas: ptr.To[int32](availableReplicas),
		},
	}
	return d
}

// helper to create MS with given availableReplicas.
func newTestMachinesetWithReplicas(name string, specReplicas, statusReplicas, availableReplicas int32, v1Beta1Conditions clusterv1.Conditions) *clusterv1.MachineSet {
	return &clusterv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Time{},
			Namespace:         metav1.NamespaceDefault,
		},
		Spec: clusterv1.MachineSetSpec{
			Replicas: ptr.To[int32](specReplicas),
		},
		Status: clusterv1.MachineSetStatus{
			Replicas:          ptr.To(statusReplicas),
			AvailableReplicas: ptr.To[int32](availableReplicas),
			Deprecated: &clusterv1.MachineSetDeprecatedStatus{
				V1Beta1: &clusterv1.MachineSetV1Beta1DeprecatedStatus{
					Conditions: v1Beta1Conditions,
				},
			},
		},
	}
}

func TestSyncDeploymentStatus(t *testing.T) {
	tests := []struct {
		name               string
		d                  *clusterv1.MachineDeployment
		oldMachineSets     []*clusterv1.MachineSet
		newMachineSet      *clusterv1.MachineSet
		expectedConditions []*clusterv1.Condition
	}{
		{
			name:           "Deployment not available: MachineDeploymentAvailableCondition should exist and be false",
			d:              newTestMachineDeployment(3, 2, 2, 2),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 2, 2, nil),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:     clusterv1.MachineDeploymentAvailableV1Beta1Condition,
					Status:   corev1.ConditionFalse,
					Severity: clusterv1.ConditionSeverityWarning,
					Reason:   clusterv1.WaitingForAvailableMachinesV1Beta1Reason,
				},
			},
		},
		{
			name:           "Deployment Available: MachineDeploymentAvailableCondition should exist and be true",
			d:              newTestMachineDeployment(3, 3, 3, 3),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  newTestMachinesetWithReplicas("foo", 3, 3, 3, nil),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:   clusterv1.MachineDeploymentAvailableV1Beta1Condition,
					Status: corev1.ConditionTrue,
				},
			},
		},
		{
			name:           "MachineSet exist: MachineSetReadyCondition should exist and mirror MachineSet Ready condition",
			d:              newTestMachineDeployment(3, 3, 3, 3),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet: newTestMachinesetWithReplicas("foo", 3, 3, 3, clusterv1.Conditions{
				{
					Type:    clusterv1.ReadyV1Beta1Condition,
					Status:  corev1.ConditionFalse,
					Reason:  "TestErrorResaon",
					Message: "test error messsage",
				},
			}),
			expectedConditions: []*clusterv1.Condition{
				{
					Type:    clusterv1.MachineSetReadyV1Beta1Condition,
					Status:  corev1.ConditionFalse,
					Reason:  "TestErrorResaon",
					Message: "test error messsage",
				},
			},
		},
		{
			name:           "MachineSet doesn't exist: MachineSetReadyCondition should exist and be false",
			d:              newTestMachineDeployment(3, 3, 3, 3),
			oldMachineSets: []*clusterv1.MachineSet{},
			newMachineSet:  nil,
			expectedConditions: []*clusterv1.Condition{
				{
					Type:     clusterv1.MachineSetReadyV1Beta1Condition,
					Status:   corev1.ConditionFalse,
					Severity: clusterv1.ConditionSeverityInfo,
					Reason:   clusterv1.WaitingForMachineSetFallbackV1Beta1Reason,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			r := &Reconciler{
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

// asserts the conditions set on the Getter object.
// TODO: replace this with util.condition.MatchConditions (or a new matcher in controller runtime komega).
func assertConditions(t *testing.T, from v1beta1conditions.Getter, conditions ...*clusterv1.Condition) {
	t.Helper()

	for _, condition := range conditions {
		assertCondition(t, from, condition)
	}
}

// asserts whether a condition of type is set on the Getter object
// when the condition is true, asserting the reason/severity/message
// for the condition are avoided.
func assertCondition(t *testing.T, from v1beta1conditions.Getter, condition *clusterv1.Condition) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(v1beta1conditions.Has(from, condition.Type)).To(BeTrue())

	if condition.Status == corev1.ConditionTrue {
		v1beta1conditions.IsTrue(from, condition.Type)
	} else {
		conditionToBeAsserted := v1beta1conditions.Get(from, condition.Type)
		g.Expect(conditionToBeAsserted.Status).To(Equal(condition.Status))
		g.Expect(conditionToBeAsserted.Severity).To(Equal(condition.Severity))
		g.Expect(conditionToBeAsserted.Reason).To(Equal(condition.Reason))
		if condition.Message != "" {
			g.Expect(conditionToBeAsserted.Message).To(Equal(condition.Message))
		}
	}
}

func Test_computeNewMachineSetName(t *testing.T) {
	tests := []struct {
		base       string
		wantPrefix string
	}{
		{
			"a",
			"a",
		},
		{
			fmt.Sprintf("%058d", 0),
			fmt.Sprintf("%058d", 0),
		},
		{
			fmt.Sprintf("%059d", 0),
			fmt.Sprintf("%058d", 0),
		},
		{
			fmt.Sprintf("%0100d", 0),
			fmt.Sprintf("%058d", 0),
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("base=%q, wantPrefix=%q", tt.base, tt.wantPrefix), func(t *testing.T) {
			got, gotSuffix := computeNewMachineSetName(tt.base)
			gotPrefix := strings.TrimSuffix(got, gotSuffix)
			if gotPrefix != tt.wantPrefix {
				t.Errorf("computeNewMachineSetName() = (%v, %v) wantPrefix %v", got, gotSuffix, tt.wantPrefix)
			}
			if len(got) > maxNameLength {
				t.Errorf("expected %s to be of max length %d", got, maxNameLength)
			}
		})
	}
}
