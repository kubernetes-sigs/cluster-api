/*
Copyright 2018 The Kubernetes Authors.

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

package machineset

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

func TestMachineToDelete(t *testing.T) {
	msg := "something wrong with the machine"
	now := metav1.Now()
	nodeRef := &corev1.ObjectReference{Name: "some-node"}
	healthyMachine := &clusterv1.Machine{Status: clusterv1.MachineStatus{NodeRef: nodeRef}}
	mustDeleteMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	betterDeleteMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{FailureMessage: &msg, NodeRef: nodeRef},
	}
	deleteMachineWithMachineAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.DeleteMachineAnnotation: ""}},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	deleteMachineWithoutNodeRef := &clusterv1.Machine{}
	nodeHealthyConditionFalseMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: clusterv1.Conditions{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	nodeHealthyConditionUnknownMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: clusterv1.Conditions{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: corev1.ConditionUnknown,
				},
			},
		},
	}

	tests := []struct {
		desc     string
		machines []*clusterv1.Machine
		diff     int
		expect   []*clusterv1.Machine
	}{
		{
			desc: "func=randomDeletePolicy, diff=0",
			diff: 0,
			machines: []*clusterv1.Machine{
				healthyMachine,
			},
			expect: []*clusterv1.Machine{},
		},
		{
			desc: "func=randomDeletePolicy, diff>len(machines)",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				healthyMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff>betterDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
				betterDeleteMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				betterDeleteMachine,
				healthyMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<betterDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
				betterDeleteMachine,
				betterDeleteMachine,
				betterDeleteMachine,
			},
			expect: []*clusterv1.Machine{
				betterDeleteMachine,
				betterDeleteMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<=mustDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
				mustDeleteMachine,
				betterDeleteMachine,
				mustDeleteMachine,
			},
			expect: []*clusterv1.Machine{
				mustDeleteMachine,
				mustDeleteMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<=mustDelete+betterDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
				mustDeleteMachine,
				healthyMachine,
				betterDeleteMachine,
			},
			expect: []*clusterv1.Machine{
				mustDeleteMachine,
				betterDeleteMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<=mustDelete+betterDelete+couldDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
				mustDeleteMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				mustDeleteMachine,
				healthyMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff>betterDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
				betterDeleteMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				betterDeleteMachine,
				healthyMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, DeleteMachineAnnotation, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				healthyMachine,
				deleteMachineWithMachineAnnotation,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				deleteMachineWithMachineAnnotation,
			},
		},
		{
			desc: "func=randomDeletePolicy, MachineWithNoNodeRef, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				healthyMachine,
				deleteMachineWithoutNodeRef,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				deleteMachineWithoutNodeRef,
			},
		},
		{
			desc: "func=randomDeletePolicy, NodeHealthyConditionFalseMachine, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				healthyMachine,
				nodeHealthyConditionFalseMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				nodeHealthyConditionFalseMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, NodeHealthyConditionUnknownMachine, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				healthyMachine,
				nodeHealthyConditionUnknownMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				nodeHealthyConditionUnknownMachine,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			result := getMachinesToDeletePrioritized(test.machines, test.diff, randomDeletePolicy)
			g.Expect(result).To(Equal(test.expect))
		})
	}
}

func TestMachineNewestDelete(t *testing.T) {
	currentTime := metav1.Now()
	statusError := capierrors.MachineStatusError("I'm unhealthy!")
	nodeRef := &corev1.ObjectReference{Name: "some-node"}
	mustDeleteMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &currentTime},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	newest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -1))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondNewest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondOldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	oldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	deleteMachineWithMachineAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.DeleteMachineAnnotation: ""}, CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	unhealthyMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{FailureReason: &statusError, NodeRef: nodeRef},
	}
	deleteMachineWithoutNodeRef := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -1))},
	}
	nodeHealthyConditionFalseMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: clusterv1.Conditions{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	nodeHealthyConditionUnknownMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: clusterv1.Conditions{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: corev1.ConditionUnknown,
				},
			},
		},
	}

	tests := []struct {
		desc     string
		machines []*clusterv1.Machine
		diff     int
		expect   []*clusterv1.Machine
	}{
		{
			desc: "func=newestDeletePriority, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, mustDeleteMachine, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine},
		},
		{
			desc: "func=newestDeletePriority, diff=2",
			diff: 2,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, mustDeleteMachine, secondOldest, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine, newest},
		},
		{
			desc: "func=newestDeletePriority, diff=3",
			diff: 3,
			machines: []*clusterv1.Machine{
				secondNewest, mustDeleteMachine, oldest, secondOldest, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine, newest, secondNewest},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (DeleteMachineAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, deleteMachineWithMachineAnnotation,
			},
			expect: []*clusterv1.Machine{deleteMachineWithMachineAnnotation},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (deleteMachineWithoutNodeRef)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, deleteMachineWithoutNodeRef,
			},
			expect: []*clusterv1.Machine{deleteMachineWithoutNodeRef},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (unhealthy)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, unhealthyMachine,
			},
			expect: []*clusterv1.Machine{unhealthyMachine},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (nodeHealthyConditionFalseMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, nodeHealthyConditionFalseMachine,
			},
			expect: []*clusterv1.Machine{nodeHealthyConditionFalseMachine},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (nodeHealthyConditionUnknownMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, nodeHealthyConditionUnknownMachine,
			},
			expect: []*clusterv1.Machine{nodeHealthyConditionUnknownMachine},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			result := getMachinesToDeletePrioritized(test.machines, test.diff, newestDeletePriority)
			g.Expect(result).To(Equal(test.expect))
		})
	}
}

func TestMachineOldestDelete(t *testing.T) {
	currentTime := metav1.Now()
	statusError := capierrors.MachineStatusError("I'm unhealthy!")
	nodeRef := &corev1.ObjectReference{Name: "some-node"}
	empty := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	newest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -1))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondNewest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondOldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	oldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	deleteMachineWithMachineAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.DeleteMachineAnnotation: ""}, CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	unhealthyMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{FailureReason: &statusError, NodeRef: nodeRef},
	}
	deleteMachineWithoutNodeRef := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
	}
	nodeHealthyConditionFalseMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: clusterv1.Conditions{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}
	nodeHealthyConditionUnknownMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: clusterv1.Conditions{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: corev1.ConditionUnknown,
				},
			},
		},
	}

	tests := []struct {
		desc     string
		machines []*clusterv1.Machine
		diff     int
		expect   []*clusterv1.Machine
	}{
		{
			desc: "func=oldestDeletePriority, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest,
			},
			expect: []*clusterv1.Machine{oldest},
		},
		{
			desc: "func=oldestDeletePriority, diff=2",
			diff: 2,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, secondOldest},
		},
		{
			desc: "func=oldestDeletePriority, diff=3",
			diff: 3,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, secondOldest, secondNewest},
		},
		{
			desc: "func=oldestDeletePriority, diff=4",
			diff: 4,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, secondOldest, secondNewest, newest},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (DeleteMachineAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, deleteMachineWithMachineAnnotation,
			},
			expect: []*clusterv1.Machine{deleteMachineWithMachineAnnotation},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (deleteMachineWithoutNodeRef)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, deleteMachineWithoutNodeRef,
			},
			expect: []*clusterv1.Machine{deleteMachineWithoutNodeRef},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (unhealthy)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, unhealthyMachine,
			},
			expect: []*clusterv1.Machine{unhealthyMachine},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (nodeHealthyConditionFalseMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, nodeHealthyConditionFalseMachine,
			},
			expect: []*clusterv1.Machine{nodeHealthyConditionFalseMachine},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (nodeHealthyConditionUnknownMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, nodeHealthyConditionUnknownMachine,
			},
			expect: []*clusterv1.Machine{nodeHealthyConditionUnknownMachine},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			result := getMachinesToDeletePrioritized(test.machines, test.diff, oldestDeletePriority)
			g.Expect(result).To(Equal(test.expect))
		})
	}
}

func TestMachineDeleteMultipleSamePriority(t *testing.T) {
	machines := make([]*clusterv1.Machine, 0, 10)
	// All of these machines will have the same delete priority because they all have the "must delete" annotation.
	for i := 0; i < 10; i++ {
		machines = append(machines, &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("machine-%d", i), Annotations: map[string]string{clusterv1.DeleteMachineAnnotation: "true"}},
		})
	}

	tests := []struct {
		desc           string
		diff           int
		deletePriority deletePriorityFunc
	}{
		{
			desc:           "multiple with same priority, func=oldestDeletePriority, diff=1",
			diff:           1,
			deletePriority: oldestDeletePriority,
		},
		{
			desc:           "multiple with same priority, func=oldestDeletePriority, diff=5",
			diff:           5,
			deletePriority: oldestDeletePriority,
		},
		{
			desc:           "multiple with same priority, func=oldestDeletePriority, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: oldestDeletePriority,
		},
		{
			desc:           "multiple with same priority, func=randomDeletePolicy, diff=1",
			diff:           1,
			deletePriority: randomDeletePolicy,
		},
		{
			desc:           "multiple with same priority, func=randomDeletePolicy, diff=5",
			diff:           5,
			deletePriority: randomDeletePolicy,
		},
		{
			desc:           "multiple with same priority, func=randomDeletePolicy, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: randomDeletePolicy,
		},
		{
			desc:           "multiple with same priority, func=newestDeletePriority, diff=1",
			diff:           1,
			deletePriority: newestDeletePriority,
		},
		{
			desc:           "multiple with same priority, func=newestDeletePriority, diff=5",
			diff:           5,
			deletePriority: newestDeletePriority,
		},
		{
			desc:           "multiple with same priority, func=newestDeletePriority, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: newestDeletePriority,
		},
		{
			desc:           "multiple with same priority, func=randomDeletePolicy, diff=1",
			diff:           1,
			deletePriority: randomDeletePolicy,
		},
		{
			desc:           "multiple with same priority, func=randomDeletePolicy, diff=5",
			diff:           5,
			deletePriority: randomDeletePolicy,
		},
		{
			desc:           "multiple with same priority, func=randomDeletePolicy, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: randomDeletePolicy,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			order := rand.Perm(len(machines))
			shuffledMachines := make([]*clusterv1.Machine, len(machines))
			for i, j := range order {
				shuffledMachines[i] = machines[j]
			}

			result := getMachinesToDeletePrioritized(shuffledMachines, test.diff, test.deletePriority)
			g.Expect(result).To(Equal(machines[:test.diff]))
		})
	}
}

func TestIsMachineHealthy(t *testing.T) {
	nodeRef := &corev1.ObjectReference{Name: "some-node"}
	statusError := capierrors.MachineStatusError("I'm unhealthy!")
	msg := "something wrong with the machine"

	tests := []struct {
		desc    string
		machine *clusterv1.Machine
		expect  bool
	}{
		{
			desc:    "when it has no NodeRef",
			machine: &clusterv1.Machine{},
			expect:  false,
		},
		{
			desc: "when it has a FailureReason",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{FailureReason: &statusError, NodeRef: nodeRef},
			},
			expect: false,
		},
		{
			desc: "when it has a FailureMessage",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{FailureMessage: &msg, NodeRef: nodeRef},
			},
			expect: false,
		},
		{
			desc: "when nodeHealthyCondition is false",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: nodeRef,
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			expect: false,
		},
		{
			desc: "when nodeHealthyCondition is unknown",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: nodeRef,
					Conditions: clusterv1.Conditions{
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			expect: false,
		},
		{
			desc: "when all requirements are met for node to be healthy",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{NodeRef: nodeRef},
			},
			expect: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			result := isMachineHealthy(test.machine)
			g.Expect(result).To(Equal(test.expect))
		})
	}
}
