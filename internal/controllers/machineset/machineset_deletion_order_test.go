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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestMachineRandomDelete(t *testing.T) {
	now := metav1.Now()
	nodeRef := clusterv1.MachineNodeReference{Name: "some-node"}
	healthyMachine := &clusterv1.Machine{Status: clusterv1.MachineStatus{NodeRef: nodeRef}}
	mustDeleteMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	betterDeleteMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
			NodeRef: nodeRef,
		},
	}
	deleteMachineWithMachineAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.DeleteMachineAnnotation: ""}},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	machineWithUpdateInProgressAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.UpdateInProgressAnnotation: ""}},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	deleteMachineWithoutNodeRef := &clusterv1.Machine{}
	nodeHealthyConditionFalseMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}
	nodeHealthyConditionUnknownMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionUnknown,
				},
			},
		},
	}
	healthCheckSucceededConditionFalseMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckSucceededCondition,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}
	healthCheckSucceededConditionUnknownMachine := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineHealthCheckSucceededCondition,
					Status: metav1.ConditionUnknown,
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
			desc: "func=randomDeletionOrder, diff=0",
			diff: 0,
			machines: []*clusterv1.Machine{
				healthyMachine,
			},
			expect: []*clusterv1.Machine{},
		},
		{
			desc: "func=randomDeletionOrder, diff>len(machines)",
			diff: 2,
			machines: []*clusterv1.Machine{
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				healthyMachine,
			},
		},
		{
			desc: "func=randomDeletionOrder, diff>betterDelete",
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
			desc: "func=randomDeletionOrder, diff<betterDelete",
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
			desc: "func=randomDeletionOrder, diff<=mustDelete",
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
			desc: "func=randomDeletionOrder, diff<=mustDelete+betterDelete",
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
			desc: "func=randomDeletionOrder, diff<=mustDelete+betterDelete+couldDelete",
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
			desc: "func=randomDeletionOrder, diff>betterDelete",
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
			desc: "func=randomDeletionOrder, DeleteMachineAnnotation, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				betterDeleteMachine,
				deleteMachineWithMachineAnnotation,
				betterDeleteMachine,
				machineWithUpdateInProgressAnnotation,
			},
			expect: []*clusterv1.Machine{
				deleteMachineWithMachineAnnotation,
			},
		},
		{
			desc: "func=randomDeletionOrder, DeleteMachineAnnotation, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				betterDeleteMachine,
				machineWithUpdateInProgressAnnotation,
				betterDeleteMachine,
			},
			expect: []*clusterv1.Machine{
				machineWithUpdateInProgressAnnotation,
			},
		},
		{
			desc: "func=randomDeletionOrder, MachineWithNoNodeRef, diff=1",
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
			desc: "func=randomDeletionOrder, NodeHealthyConditionFalseMachine, diff=1",
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
			desc: "func=randomDeletionOrder, NodeHealthyConditionUnknownMachine, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				healthyMachine,
				nodeHealthyConditionUnknownMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				healthyMachine,
			},
		},
		{
			desc: "func=randomDeletionOrder, HealthCheckSucceededConditionFalseMachine, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				healthyMachine,
				healthCheckSucceededConditionFalseMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				healthCheckSucceededConditionFalseMachine,
			},
		},
		{
			desc: "func=randomDeletionOrder, HealthCheckSucceededConditionUnknownMachine, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				healthyMachine,
				healthCheckSucceededConditionUnknownMachine,
				healthyMachine,
			},
			expect: []*clusterv1.Machine{
				healthyMachine,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			result := getMachinesToDeletePrioritized(test.machines, test.diff, randomDeletionOrder)
			g.Expect(result).To(BeComparableTo(test.expect))
		})
	}
}

func TestMachineNewestDelete(t *testing.T) {
	currentTime := metav1.Now()
	nodeRef := clusterv1.MachineNodeReference{Name: "some-node"}
	mustDeleteMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &currentTime},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	newest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -1))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondNewest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -5))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondOldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	oldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	deleteMachineWithMachineAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.DeleteMachineAnnotation: ""}, CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	machineWithUpdateInProgressAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.UpdateInProgressAnnotation: ""}, CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	unhealthyMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
			NodeRef: nodeRef,
		},
	}
	deleteMachineWithoutNodeRef := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -1))},
	}
	nodeHealthyConditionFalseMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}
	nodeHealthyConditionUnknownMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionUnknown,
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
			desc: "func=newestDeletionOrder, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, mustDeleteMachine, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine},
		},
		{
			desc: "func=newestDeletionOrder, diff=2",
			diff: 2,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, mustDeleteMachine, secondOldest, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine, newest},
		},
		{
			desc: "func=newestDeletionOrder, diff=3",
			diff: 3,
			machines: []*clusterv1.Machine{
				secondNewest, mustDeleteMachine, oldest, secondOldest, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine, newest, secondNewest},
		},
		{
			desc: "func=newestDeletionOrder, diff=1 (DeleteMachineAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, deleteMachineWithMachineAnnotation, machineWithUpdateInProgressAnnotation,
			},
			expect: []*clusterv1.Machine{deleteMachineWithMachineAnnotation},
		},
		{
			desc: "func=newestDeletionOrder, diff=1 (UpdateInProgressAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, machineWithUpdateInProgressAnnotation,
			},
			expect: []*clusterv1.Machine{machineWithUpdateInProgressAnnotation},
		},
		{
			desc: "func=newestDeletionOrder, diff=1 (deleteMachineWithoutNodeRef)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, deleteMachineWithoutNodeRef,
			},
			expect: []*clusterv1.Machine{deleteMachineWithoutNodeRef},
		},
		{
			desc: "func=newestDeletionOrder, diff=1 (unhealthy)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, unhealthyMachine,
			},
			expect: []*clusterv1.Machine{unhealthyMachine},
		},
		{
			desc: "func=newestDeletionOrder, diff=1 (nodeHealthyConditionFalseMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, nodeHealthyConditionFalseMachine,
			},
			expect: []*clusterv1.Machine{nodeHealthyConditionFalseMachine},
		},
		{
			desc: "func=newestDeletionOrder, diff=1 (nodeHealthyConditionUnknownMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				// nodeHealthyConditionUnknownMachine is not considered unhealthy with unknown condition.
				secondNewest, oldest, secondOldest, newest, nodeHealthyConditionUnknownMachine,
			},
			expect: []*clusterv1.Machine{newest},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			result := getMachinesToDeletePrioritized(test.machines, test.diff, newestDeletionOrder)
			g.Expect(result).To(BeComparableTo(test.expect))
		})
	}
}

func TestMachineOldestDelete(t *testing.T) {
	currentTime := metav1.Now()
	nodeRef := clusterv1.MachineNodeReference{Name: "some-node"}
	empty := &clusterv1.Machine{
		Status: clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	newest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -1))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondNewest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -5))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	secondOldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	oldest := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	deleteMachineWithMachineAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.DeleteMachineAnnotation: ""}, CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	machineWithUpdateInProgressAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{clusterv1.UpdateInProgressAnnotation: ""}, CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	unhealthyMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
			NodeRef: nodeRef,
		},
	}
	mustDeleteMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "b", DeletionTimestamp: &currentTime},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	unhealthyMachineA := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "unhealthyMachineA", CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
			NodeRef: nodeRef,
		},
	}
	unhealthyMachineZ := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{Name: "z", CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
			NodeRef: nodeRef,
		},
	}
	deleteMachineWithoutNodeRef := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
	}
	nodeHealthyConditionFalseMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionFalse,
				},
			},
		},
	}
	nodeHealthyConditionUnknownMachine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.AddDate(0, 0, -10))},
		Status: clusterv1.MachineStatus{
			NodeRef: nodeRef,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.MachineNodeHealthyCondition,
					Status: metav1.ConditionUnknown,
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
			desc: "func=oldestDeletionOrder, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest,
			},
			expect: []*clusterv1.Machine{oldest},
		},
		{
			desc: "func=oldestDeletionOrder, diff=2",
			diff: 2,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, secondOldest},
		},
		{
			desc: "func=oldestDeletionOrder, diff=3",
			diff: 3,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, secondOldest, secondNewest},
		},
		{
			desc: "func=oldestDeletionOrder, diff=4",
			diff: 4,
			machines: []*clusterv1.Machine{
				secondNewest, oldest, secondOldest, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, secondOldest, secondNewest, newest},
		},
		{
			desc: "func=oldestDeletionOrder, diff=1 (DeleteMachineAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, deleteMachineWithMachineAnnotation, machineWithUpdateInProgressAnnotation,
			},
			expect: []*clusterv1.Machine{deleteMachineWithMachineAnnotation},
		},
		{
			desc: "func=oldestDeletionOrder, diff=1 (hUpdateInProgressAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, machineWithUpdateInProgressAnnotation,
			},
			expect: []*clusterv1.Machine{machineWithUpdateInProgressAnnotation},
		},
		{
			desc: "func=oldestDeletionOrder, diff=1 (deleteMachineWithoutNodeRef)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, deleteMachineWithoutNodeRef,
			},
			expect: []*clusterv1.Machine{deleteMachineWithoutNodeRef},
		},
		{
			desc: "func=oldestDeletionOrder, diff=1 (unhealthy)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, unhealthyMachine,
			},
			expect: []*clusterv1.Machine{unhealthyMachine},
		},
		{
			desc: "func=oldestDeletionOrder, diff=1 (nodeHealthyConditionFalseMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, nodeHealthyConditionFalseMachine,
			},
			expect: []*clusterv1.Machine{nodeHealthyConditionFalseMachine},
		},
		{
			desc: "func=oldestDeletionOrder, diff=1 (nodeHealthyConditionUnknownMachine)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, nodeHealthyConditionUnknownMachine,
			},
			// nodeHealthyConditionUnknownMachine is not considered unhealthy with unknown condition.
			expect: []*clusterv1.Machine{oldest},
		},
		// these two cases ensures the mustDeleteMachine is always picked regardless of the machine names.
		{
			desc: "func=oldestDeletionOrder, diff=1 (unhealthyMachineA)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, mustDeleteMachine, unhealthyMachineA,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine},
		},
		{
			desc: "func=oldestDeletionOrder, diff=1 (unhealthyMachineZ)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, secondNewest, oldest, secondOldest, newest, mustDeleteMachine, unhealthyMachineZ,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			g := NewWithT(t)

			result := getMachinesToDeletePrioritized(test.machines, test.diff, oldestDeletionOrder)
			g.Expect(result).To(BeComparableTo(test.expect))
		})
	}
}

func TestMachineDeleteMultipleSamePriority(t *testing.T) {
	machines := make([]*clusterv1.Machine, 0, 10)
	// All of these machines will have the same delete priority because they all have the "must delete" annotation.
	for i := range 10 {
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
			desc:           "multiple with same priority, func=oldestDeletionOrder, diff=1",
			diff:           1,
			deletePriority: oldestDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=oldestDeletionOrder, diff=5",
			diff:           5,
			deletePriority: oldestDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=oldestDeletionOrder, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: oldestDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=randomDeletionOrder, diff=1",
			diff:           1,
			deletePriority: randomDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=randomDeletionOrder, diff=5",
			diff:           5,
			deletePriority: randomDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=randomDeletionOrder, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: randomDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=newestDeletionOrder, diff=1",
			diff:           1,
			deletePriority: newestDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=newestDeletionOrder, diff=5",
			diff:           5,
			deletePriority: newestDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=newestDeletionOrder, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: newestDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=randomDeletionOrder, diff=1",
			diff:           1,
			deletePriority: randomDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=randomDeletionOrder, diff=5",
			diff:           5,
			deletePriority: randomDeletionOrder,
		},
		{
			desc:           "multiple with same priority, func=randomDeletionOrder, diff=rand",
			diff:           rand.Intn(len(machines)),
			deletePriority: randomDeletionOrder,
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
			g.Expect(result).To(BeComparableTo(machines[:test.diff]))
		})
	}
}

func TestIsMachineHealthy(t *testing.T) {
	nodeRef := clusterv1.MachineNodeReference{Name: "some-node"}

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
			desc: "when nodeHealthyCondition is false",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: nodeRef,
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionFalse,
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
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.MachineNodeHealthyCondition,
							Status: metav1.ConditionUnknown,
						},
					},
				},
			},
			expect: true,
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
