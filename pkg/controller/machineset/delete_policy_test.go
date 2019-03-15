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
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/common"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestMachineToDelete(t *testing.T) {
	msg := "something wrong with the machine"
	now := metav1.Now()
	mustDeleteMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}
	betterDeleteMachine := &v1alpha1.Machine{Status: v1alpha1.MachineStatus{ErrorMessage: &msg}}
	deleteMeMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{DeleteNodeAnnotation: "yes"}}}

	tests := []struct {
		desc     string
		machines []*v1alpha1.Machine
		diff     int
		expect   []*v1alpha1.Machine
	}{
		{
			desc: "func=randomDeletePolicy, diff=0",
			diff: 0,
			machines: []*v1alpha1.Machine{
				{},
			},
			expect: []*v1alpha1.Machine{},
		},
		{
			desc: "func=randomDeletePolicy, diff>len(machines)",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
			},
			expect: []*v1alpha1.Machine{
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, diff>betterDelete",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
				betterDeleteMachine,
				{},
			},
			expect: []*v1alpha1.Machine{
				betterDeleteMachine,
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<betterDelete",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
				betterDeleteMachine,
				betterDeleteMachine,
				betterDeleteMachine,
			},
			expect: []*v1alpha1.Machine{
				betterDeleteMachine,
				betterDeleteMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<=mustDelete",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
				mustDeleteMachine,
				betterDeleteMachine,
				mustDeleteMachine,
			},
			expect: []*v1alpha1.Machine{
				mustDeleteMachine,
				mustDeleteMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<=mustDelete+betterDelete",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
				mustDeleteMachine,
				{},
				betterDeleteMachine,
			},
			expect: []*v1alpha1.Machine{
				mustDeleteMachine,
				betterDeleteMachine,
			},
		},
		{
			desc: "func=randomDeletePolicy, diff<=mustDelete+betterDelete+couldDelete",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
				mustDeleteMachine,
				{},
			},
			expect: []*v1alpha1.Machine{
				mustDeleteMachine,
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, diff>betterDelete",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
				betterDeleteMachine,
				{},
			},
			expect: []*v1alpha1.Machine{
				betterDeleteMachine,
				{},
			},
		},
		{
			desc: "func=randomDeletePolicy, annotated, diff=1",
			diff: 1,
			machines: []*v1alpha1.Machine{
				{},
				deleteMeMachine,
				{},
			},
			expect: []*v1alpha1.Machine{
				deleteMeMachine,
			},
		}}

	for _, test := range tests {
		result := getMachinesToDeletePrioritized(test.machines, test.diff, randomDeletePolicy)
		if !reflect.DeepEqual(result, test.expect) {
			t.Errorf("[case %s]", test.desc)
		}
	}
}

func TestMachineNewestDelete(t *testing.T) {

	currentTime := metav1.Now()
	statusError := common.MachineStatusError("I'm unhealthy!")
	mustDeleteMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &currentTime}}
	newest := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -1))}}
	new := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))}}
	old := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	oldest := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	annotatedMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{DeleteNodeAnnotation: "yes"}, CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	unhealthyMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}, Status: v1alpha1.MachineStatus{ErrorReason: &statusError}}

	tests := []struct {
		desc     string
		machines []*v1alpha1.Machine
		diff     int
		expect   []*v1alpha1.Machine
	}{
		{
			desc: "func=newestDeletePriority, diff=1",
			diff: 1,
			machines: []*v1alpha1.Machine{
				new, oldest, old, mustDeleteMachine, newest,
			},
			expect: []*v1alpha1.Machine{mustDeleteMachine},
		},
		{
			desc: "func=newestDeletePriority, diff=2",
			diff: 2,
			machines: []*v1alpha1.Machine{
				new, oldest, mustDeleteMachine, old, newest,
			},
			expect: []*v1alpha1.Machine{mustDeleteMachine, newest},
		},
		{
			desc: "func=newestDeletePriority, diff=3",
			diff: 3,
			machines: []*v1alpha1.Machine{
				new, mustDeleteMachine, oldest, old, newest,
			},
			expect: []*v1alpha1.Machine{mustDeleteMachine, newest, new},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (annotated)",
			diff: 1,
			machines: []*v1alpha1.Machine{
				new, oldest, old, newest, annotatedMachine,
			},
			expect: []*v1alpha1.Machine{annotatedMachine},
		},
		{
			desc: "func=newestDeletePriority, diff=1 (unhealthy)",
			diff: 1,
			machines: []*v1alpha1.Machine{
				new, oldest, old, newest, unhealthyMachine,
			},
			expect: []*v1alpha1.Machine{unhealthyMachine},
		},
	}

	for _, test := range tests {
		result := getMachinesToDeletePrioritized(test.machines, test.diff, newestDeletePriority)
		if !reflect.DeepEqual(result, test.expect) {
			t.Errorf("[case %s]", test.desc)
		}
	}
}

func TestMachineOldestDelete(t *testing.T) {

	currentTime := metav1.Now()
	statusError := common.MachineStatusError("I'm unhealthy!")
	empty := &v1alpha1.Machine{}
	newest := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -1))}}
	new := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -5))}}
	old := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	oldest := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	annotatedMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{DeleteNodeAnnotation: "yes"}, CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}}
	unhealthyMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(currentTime.Time.AddDate(0, 0, -10))}, Status: v1alpha1.MachineStatus{ErrorReason: &statusError}}

	tests := []struct {
		desc     string
		machines []*v1alpha1.Machine
		diff     int
		expect   []*v1alpha1.Machine
	}{
		{
			desc: "func=oldestDeletePriority, diff=1",
			diff: 1,
			machines: []*v1alpha1.Machine{
				empty, new, oldest, old, newest,
			},
			expect: []*v1alpha1.Machine{oldest},
		},
		{
			desc: "func=oldestDeletePriority, diff=2",
			diff: 2,
			machines: []*v1alpha1.Machine{
				new, oldest, old, newest, empty,
			},
			expect: []*v1alpha1.Machine{oldest, old},
		},
		{
			desc: "func=oldestDeletePriority, diff=3",
			diff: 3,
			machines: []*v1alpha1.Machine{
				new, oldest, old, newest, empty,
			},
			expect: []*v1alpha1.Machine{oldest, old, new},
		},
		{
			desc: "func=oldestDeletePriority, diff=4",
			diff: 4,
			machines: []*v1alpha1.Machine{
				new, oldest, old, newest, empty,
			},
			expect: []*v1alpha1.Machine{oldest, old, new, newest},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (annotated)",
			diff: 1,
			machines: []*v1alpha1.Machine{
				empty, new, oldest, old, newest, annotatedMachine,
			},
			expect: []*v1alpha1.Machine{annotatedMachine},
		},
		{
			desc: "func=oldestDeletePriority, diff=1 (unhealthy)",
			diff: 1,
			machines: []*v1alpha1.Machine{
				empty, new, oldest, old, newest, unhealthyMachine,
			},
			expect: []*v1alpha1.Machine{unhealthyMachine},
		},
	}

	for _, test := range tests {
		result := getMachinesToDeletePrioritized(test.machines, test.diff, oldestDeletePriority)
		if !reflect.DeepEqual(result, test.expect) {
			t.Errorf("[case %s]", test.desc)
		}
	}
}
