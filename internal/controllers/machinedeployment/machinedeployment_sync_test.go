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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
	"sigs.k8s.io/cluster-api/internal/controllers/machinedeployment/mdutil"
	"sigs.k8s.io/cluster-api/util/conditions"
)

func TestCalculateStatus(t *testing.T) {
	msStatusError := capierrors.MachineSetStatusError("some failure")

	var tests = map[string]struct {
		machineSets    []*clusterv1.MachineSet
		newMachineSet  *clusterv1.MachineSet
		deployment     *clusterv1.MachineDeployment
		expectedStatus clusterv1.MachineDeploymentStatus
	}{
		"When availableReplicas < minDesiredReplicas (desiredReplicas - mdutil.MaxUnavailable(*deployment)) then MachineDeploymentAvailableCondition is false ": {
			machineSets: []*clusterv1.MachineSet{{
				Spec: clusterv1.MachineSetSpec{
					Replicas: pointer.Int32Ptr(2),
				},
				Status: clusterv1.MachineSetStatus{
					Selector:           "",
					AvailableReplicas:  1,
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
					AvailableReplicas:  1,
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
					Strategy: &clusterv1.MachineDeploymentStrategy{
						Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
						RollingUpdate: &clusterv1.MachineRollingUpdateDeployment{
							MaxUnavailable: intOrStrPtr(0),
							MaxSurge:       intOrStrPtr(1),
							DeletePolicy:   pointer.StringPtr("Oldest"),
						},
					},
					Replicas: pointer.Int32Ptr(2),
				},
			},
			expectedStatus: clusterv1.MachineDeploymentStatus{
				ObservedGeneration:  2,
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   1,
				UnavailableReplicas: 1,
				Phase:               "Running",
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineDeploymentAvailableCondition,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   clusterv1.ResizedCondition,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		"When the new MachineSet readyReplicas = MachineDeployment.spec.replicas then phase=Running": {
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
					Strategy: &clusterv1.MachineDeploymentStrategy{},
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
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineDeploymentAvailableCondition,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   clusterv1.ResizedCondition,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
		"When the new MachineSet readyReplicas < MachineDeployment.spec.replicas then phase=scalingUp": {
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
					Strategy: &clusterv1.MachineDeploymentStrategy{},
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
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineDeploymentAvailableCondition,
						Status: corev1.ConditionTrue,
					},
					{
						Type:     clusterv1.ResizedCondition,
						Status:   corev1.ConditionFalse,
						Reason:   clusterv1.ScalingUpReason,
						Severity: clusterv1.ConditionSeverityWarning,
					},
				},
			},
		},
		"When the new MachineSet availableReplicas > MachineDeployment.spec.replicas then phase=ScalingDown": {
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
					Strategy: &clusterv1.MachineDeploymentStrategy{},
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
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineDeploymentAvailableCondition,
						Status: corev1.ConditionTrue,
					},
					{
						Type:     clusterv1.ResizedCondition,
						Status:   corev1.ConditionFalse,
						Reason:   clusterv1.ScalingDownReason,
						Severity: clusterv1.ConditionSeverityWarning,
					},
				},
			},
		},
		"When the new MachineSet has a FailureReason then Phase=Failed": {
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
					Strategy: &clusterv1.MachineDeploymentStrategy{},
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
				Conditions: []clusterv1.Condition{
					{
						Type:   clusterv1.MachineDeploymentAvailableCondition,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   clusterv1.ResizedCondition,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)

			err := updateMachineDeploymentStatus(test.machineSets, test.newMachineSet, test.deployment)
			g.Expect(err).ToNot(HaveOccurred())
			assertConditions(t, test.deployment, test.expectedStatus.Conditions...)

			// Remove conditions to compare status, otherwise LastTransitionTime will make the comparison fail.
			test.deployment.Status.Conditions = nil
			test.expectedStatus.Conditions = nil
			g.Expect(test.deployment.Status).To(Equal(test.expectedStatus))
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

			expectedMachineSetAnnotations := map[string]string{
				clusterv1.DesiredReplicasAnnotation: fmt.Sprintf("%d", *tc.machineDeployment.Spec.Replicas),
				clusterv1.MaxReplicasAnnotation:     fmt.Sprintf("%d", (*tc.machineDeployment.Spec.Replicas)+mdutil.MaxSurge(*tc.machineDeployment)),
			}
			g.Expect(freshMachineSet.GetAnnotations()).To(BeEquivalentTo(expectedMachineSetAnnotations))
		})
	}
}

// asserts the conditions set on the Getter object.
// TODO: replace this with util.condition.MatchConditions (or a new matcher in internal/matchers).
func assertConditions(t *testing.T, from conditions.Getter, conditions ...clusterv1.Condition) {
	t.Helper()

	for i := range conditions {
		assertCondition(t, from, &conditions[i])
	}
}

// asserts whether a condition of type is set on the Getter object
// when the condition is true, asserting the reason/severity/message
// for the condition are avoided.
func assertCondition(t *testing.T, from conditions.Getter, condition *clusterv1.Condition) {
	t.Helper()

	g := NewWithT(t)
	g.Expect(conditions.Has(from, condition.Type)).To(BeTrue())

	if condition.Status == corev1.ConditionTrue {
		conditions.IsTrue(from, condition.Type)
	} else {
		conditionToBeAsserted := conditions.Get(from, condition.Type)
		g.Expect(conditionToBeAsserted.Status).To(Equal(condition.Status))
		g.Expect(conditionToBeAsserted.Severity).To(Equal(condition.Severity))
		g.Expect(conditionToBeAsserted.Reason).To(Equal(condition.Reason))
		if condition.Message != "" {
			g.Expect(conditionToBeAsserted.Message).To(Equal(condition.Message))
		}
	}
}
