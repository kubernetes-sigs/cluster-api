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
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestMachineToDelete(t *testing.T) {
	msg := "something wrong with the machine"
	now := metav1.Now()
	mustDeleteMachine := &v1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}}
	betterDeleteMachine := &v1alpha1.Machine{Status: v1alpha1.MachineStatus{ErrorMessage: &msg}}

	tests := []struct {
		desc     string
		machines []*v1alpha1.Machine
		diff     int
		expect   []*v1alpha1.Machine
	}{
		{
			desc: "func=simpleDeletePriority, diff=0",
			diff: 0,
			machines: []*v1alpha1.Machine{
				{},
			},
			expect: []*v1alpha1.Machine{},
		},
		{
			desc: "func=simpleDeletePriority, diff>len(machines)",
			diff: 2,
			machines: []*v1alpha1.Machine{
				{},
			},
			expect: []*v1alpha1.Machine{
				{},
			},
		},
		{
			desc: "func=simpleDeletePriority, diff>betterDelete",
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
			desc: "func=simpleDeletePriority, diff<betterDelete",
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
			desc: "func=simpleDeletePriority, diff<=mustDelete",
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
			desc: "func=simpleDeletePriority, diff<=mustDelete+betterDelete",
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
			desc: "func=simpleDeletePriority, diff<=mustDelete+betterDelete+couldDelete",
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
	}

	for _, test := range tests {
		result := getMachinesToDeletePrioritized(test.machines, test.diff, simpleDeletePriority)
		if !reflect.DeepEqual(result, test.expect) {
			t.Errorf("[case %s]", test.desc)
		}
	}
}
