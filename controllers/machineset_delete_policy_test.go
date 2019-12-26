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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capierrors "sigs.k8s.io/cluster-api/errors"
)

func TestMachineToDelete(t *testing.T) {
	msg := "something wrong with the machine"
	now := metav1.Now()
	mustDeleteMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}
	betterDeleteMachine := &clusterv1.Machine{Status: clusterv1.MachineStatus{FailureMessage: &msg}}
	deleteMeMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{DeleteNodeAnnotation: "yes"}}}

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
				{},
			},
			expect: []*clusterv1.Machine{},
		},
		{
			desc: "func=randomDeletePolicy, diff>len(machines)",
			diff: 2,
			machines: []*clusterv1.Machine{
				{},
			},
			expect: []*clusterv1.Machine{
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, diff>betterDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				{},
				betterDeleteMachine,
				{},
			},
			expect: []*clusterv1.Machine{
				betterDeleteMachine,
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<betterDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				{},
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
				{},
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
				{},
				mustDeleteMachine,
				{},
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
				{},
				mustDeleteMachine,
				{},
			},
			expect: []*clusterv1.Machine{
				mustDeleteMachine,
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, diff>betterDelete",
			diff: 2,
			machines: []*clusterv1.Machine{
				{},
				betterDeleteMachine,
				{},
			},
			expect: []*clusterv1.Machine{
				betterDeleteMachine,
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, annotated, diff=1",
			diff: 1,
			machines: []*clusterv1.Machine{
				{},
				deleteMeMachine,
				{},
			},
			expect: []*clusterv1.Machine{
				deleteMeMachine,
			},
		}}

	for _, test := range tests {
		g := NewWithT(t)

		result := getMachinesToDeletePrioritized(test.machines, test.diff, randomDeletePolicy)
		g.Expect(result).To(Equal(test.expect))
	}
}

func TestMachineNewestDelete(t *testing.T) {

	currentTime := metav1.Now()
	statusError := capierrors.MachineStatusError("I'm unhealthy!")
	mustDeleteMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &currentTime}}
	newest := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -1))}}
	new := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))}}
	old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	oldest := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	annotatedMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{DeleteNodeAnnotation: "yes"}, CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	unhealthyMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}, Status: clusterv1.MachineStatus{FailureReason: &statusError}}

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
			desc: "func=newestDeletePriority, diff=1 (annotated)",
			diff: 1,
			machines: []*clusterv1.Machine{
				new, oldest, old, newest, annotatedMachine,
			},
			expect: []*clusterv1.Machine{annotatedMachine},
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
		g := NewWithT(t)

		result := getMachinesToDeletePrioritized(test.machines, test.diff, newestDeletePriority)
		g.Expect(result).To(Equal(test.expect))
	}
}

func TestMachineOldestDelete(t *testing.T) {

	currentTime := metav1.Now()
	statusError := capierrors.MachineStatusError("I'm unhealthy!")
	empty := &clusterv1.Machine{}
	newest := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -1))}}
	new := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))}}
	old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	oldest := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	annotatedMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{DeleteNodeAnnotation: "yes"}, CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	unhealthyMachine := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}, Status: clusterv1.MachineStatus{FailureReason: &statusError}}

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
			desc: "func=oldestDeletePriority, diff=1 (annotated)",
			diff: 1,
			machines: []*clusterv1.Machine{
				empty, new, oldest, old, newest, annotatedMachine,
			},
			expect: []*clusterv1.Machine{annotatedMachine},
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
		g := NewWithT(t)

		result := getMachinesToDeletePrioritized(test.machines, test.diff, oldestDeletePriority)
		g.Expect(result).To(Equal(test.expect))
	}
}
