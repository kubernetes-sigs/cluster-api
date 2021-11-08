/*
Copyright 2020 The Kubernetes Authors.

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

package failuredomains

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

func TestNewFailureDomainPicker(t *testing.T) {
	a := pointer.StringPtr("us-west-1a")
	b := pointer.StringPtr("us-west-1b")

	fds := clusterv1.FailureDomains{
		*a: clusterv1.FailureDomainSpec{},
		*b: clusterv1.FailureDomainSpec{},
	}
	machinea := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: a}}
	machineb := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: b}}
	machinenil := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: nil}}

	testcases := []struct {
		name     string
		fds      clusterv1.FailureDomains
		machines collections.Machines
		expected []*string
	}{
		{
			name:     "simple",
			expected: nil,
		},
		{
			name: "no machines",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{},
			},
			expected: []*string{a},
		},
		{
			name:     "one machine in a failure domain",
			fds:      fds,
			machines: collections.FromMachines(machinea.DeepCopy()),
			expected: []*string{b},
		},
		{
			name: "no failure domain specified on machine",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{},
			},
			machines: collections.FromMachines(machinenil.DeepCopy()),
			expected: []*string{a},
		},
		{
			name: "mismatched failure domain on machine",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{},
			},
			machines: collections.FromMachines(machineb.DeepCopy()),
			expected: []*string{a},
		},
		{
			name:     "failure domains and no machines should return a valid failure domain",
			fds:      fds,
			expected: []*string{a, b},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			fd := PickFewest(tc.fds, tc.machines)
			if tc.expected == nil {
				g.Expect(fd).To(BeNil())
			} else {
				g.Expect(fd).To(BeElementOf(tc.expected))
			}
		})
	}
}

func TestNewFailureDomainPickMost(t *testing.T) {
	a := pointer.StringPtr("us-west-1a")
	b := pointer.StringPtr("us-west-1b")

	fds := clusterv1.FailureDomains{
		*a: clusterv1.FailureDomainSpec{ControlPlane: true},
		*b: clusterv1.FailureDomainSpec{ControlPlane: true},
	}
	machinea := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: a}}
	machineb := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: b}}
	machinenil := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: nil}}

	testcases := []struct {
		name     string
		fds      clusterv1.FailureDomains
		machines collections.Machines
		expected []*string
	}{
		{
			name:     "simple",
			expected: nil,
		},
		{
			name: "no machines should return nil",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{},
			},
			expected: nil,
		},
		{
			name:     "one machine in a failure domain",
			fds:      fds,
			machines: collections.FromMachines(machinea.DeepCopy()),
			expected: []*string{a},
		},
		{
			name: "no failure domain specified on machine",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{ControlPlane: true},
			},
			machines: collections.FromMachines(machinenil.DeepCopy()),
			expected: nil,
		},
		{
			name: "mismatched failure domain on machine should return nil",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{ControlPlane: true},
			},
			machines: collections.FromMachines(machineb.DeepCopy()),
			expected: nil,
		},
		{
			name:     "failure domains and no machines should return nil",
			fds:      fds,
			expected: nil,
		},
		{
			name:     "nil failure domains with machines",
			machines: collections.FromMachines(machineb.DeepCopy()),
			expected: nil,
		},
		{
			name:     "nil failure domains with no machines",
			expected: nil,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			fd := PickMost(tc.fds, tc.machines, tc.machines)
			if tc.expected == nil {
				g.Expect(fd).To(BeNil())
			} else {
				g.Expect(fd).To(BeElementOf(tc.expected))
			}
		})
	}
}
