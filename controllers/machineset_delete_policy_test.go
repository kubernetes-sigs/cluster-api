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

package controllers

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
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
	new := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	old := &clusterv1.Machine{
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
				new, oldest, old, mustDeleteMachine, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine},
		},
		{
			desc: "func=newestDeletePriority, diff=2",
			diff: 2,
			machines: []*clusterv1.Machine{
				new, oldest, mustDeleteMachine, old, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine, newest},
		},
		{
			desc: "func=newestDeletePriority, diff=3",
			diff: 3,
			machines: []*clusterv1.Machine{
				new, mustDeleteMachine, oldest, old, newest,
			},
			expect: []*clusterv1.Machine{mustDeleteMachine, newest, new},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (DeleteMachineAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				new, oldest, old, newest, deleteMachineWithMachineAnnotation,
			},
			expect: []*clusterv1.Machine{deleteMachineWithMachineAnnotation},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (deleteMachineWithoutNodeRef)",
			diff: 1,
			machines: []*clusterv1.Machine{
				new, oldest, old, newest, deleteMachineWithoutNodeRef,
			},
			expect: []*clusterv1.Machine{deleteMachineWithoutNodeRef},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (unhealthy)",
			diff: 1,
			machines: []*clusterv1.Machine{
				new, oldest, old, newest, unhealthyMachine,
			},
			expect: []*clusterv1.Machine{unhealthyMachine},
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
	new := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))},
		Status:     clusterv1.MachineStatus{NodeRef: nodeRef},
	}
	old := &clusterv1.Machine{
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
				empty, new, oldest, old, newest,
			},
			expect: []*clusterv1.Machine{oldest},
		},
		{
			desc: "func=oldestDeletePriority, diff=2",
			diff: 2,
			machines: []*clusterv1.Machine{
				new, oldest, old, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, old},
		},
		{
			desc: "func=oldestDeletePriority, diff=3",
			diff: 3,
			machines: []*clusterv1.Machine{
				new, oldest, old, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, old, new},
		},
		{
			desc: "func=oldestDeletePriority, diff=4",
			diff: 4,
			machines: []*clusterv1.Machine{
				new, oldest, old, newest, empty,
			},
			expect: []*clusterv1.Machine{oldest, old, new, newest},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (DeleteMachineAnnotation)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, new, oldest, old, newest, deleteMachineWithMachineAnnotation,
			},
			expect: []*clusterv1.Machine{deleteMachineWithMachineAnnotation},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (deleteMachineWithoutNodeRef)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, new, oldest, old, newest, deleteMachineWithoutNodeRef,
			},
			expect: []*clusterv1.Machine{deleteMachineWithoutNodeRef},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (unhealthy)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, new, oldest, old, newest, unhealthyMachine,
			},
			expect: []*clusterv1.Machine{unhealthyMachine},
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
