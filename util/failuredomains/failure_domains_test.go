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
	"sort"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/collections"
)

var (
	ctx = ctrl.SetupSignalHandler()
)

func TestNewFailureDomainPicker(t *testing.T) {
	a := ptr.To("us-west-1a")
	b := ptr.To("us-west-1b")

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

			fd := PickFewest(ctx, tc.fds, tc.machines, nil)
			if tc.expected == nil {
				g.Expect(fd).To(BeNil())
			} else {
				g.Expect(fd).To(BeElementOf(tc.expected))
			}
		})
	}
}

func TestPickMost(t *testing.T) {
	a := ptr.To("us-west-1a")
	b := ptr.To("us-west-1b")

	fds := clusterv1.FailureDomains{
		*a: clusterv1.FailureDomainSpec{ControlPlane: true},
		*b: clusterv1.FailureDomainSpec{ControlPlane: true},
	}
	machinea := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: a}}
	machineb := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: b}}
	machinenil := &clusterv1.Machine{Spec: clusterv1.MachineSpec{FailureDomain: nil}}

	testcases := []struct {
		name             string
		fds              clusterv1.FailureDomains
		allMachines      collections.Machines
		eligibleMachines collections.Machines
		expected         *string
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
			name:             "one machine in a failure domain",
			fds:              fds,
			allMachines:      collections.FromMachines(machinea.DeepCopy()),
			eligibleMachines: collections.FromMachines(machinea.DeepCopy()),
			expected:         a,
		},
		{
			name: "no failure domain specified on machine",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{ControlPlane: true},
			},
			allMachines:      collections.FromMachines(machinenil.DeepCopy()),
			eligibleMachines: collections.FromMachines(machinenil.DeepCopy()),
			expected:         nil,
		},
		{
			name: "mismatched failure domain on machine should return nil",
			fds: clusterv1.FailureDomains{
				*a: clusterv1.FailureDomainSpec{ControlPlane: true},
			},
			allMachines:      collections.FromMachines(machineb.DeepCopy()),
			eligibleMachines: collections.FromMachines(machineb.DeepCopy()),
			expected:         nil,
		},
		{
			name:     "failure domains and no machines should return nil",
			fds:      fds,
			expected: nil,
		},
		{
			name:             "nil failure domains with machines",
			allMachines:      collections.FromMachines(machineb.DeepCopy()),
			eligibleMachines: collections.FromMachines(machineb.DeepCopy()),
			expected:         nil,
		},
		{
			name:     "nil failure domains with no machines",
			expected: nil,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			fd := PickMost(ctx, tc.fds, tc.allMachines, tc.eligibleMachines)
			if tc.expected == nil {
				g.Expect(fd).To(BeNil())
			} else {
				g.Expect(fd).To(Equal(tc.expected))
			}
		})
	}
}

func TestPickFewestNew(t *testing.T) {
	a := "us-west-1a"
	b := "us-west-1b"
	c := "us-west-1c"

	fds3 := clusterv1.FailureDomains{
		a: clusterv1.FailureDomainSpec{ControlPlane: true},
		b: clusterv1.FailureDomainSpec{ControlPlane: true},
		c: clusterv1.FailureDomainSpec{ControlPlane: true},
	}

	fds2 := clusterv1.FailureDomains{
		a: clusterv1.FailureDomainSpec{ControlPlane: true},
		b: clusterv1.FailureDomainSpec{ControlPlane: true},
	}

	fds0 := clusterv1.FailureDomains{}

	machineA1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineA2 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a2"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineB1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineC1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "c1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(c)}}
	machineA1Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a1-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineA2Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a2-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineB1Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b1-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineB2Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b2-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineC1Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "c1-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(c)}}
	machineA1New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a1-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineA2New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a2-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineB1New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b1-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineC1New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "c1-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(c)}}

	testcases := []struct {
		name             string
		fds              clusterv1.FailureDomains
		allMachines      collections.Machines
		upToDateMachines collections.Machines
		expected         []string
	}{
		// Use case A: 3 failure domains, 3 control plane machines

		// scenario A.1: scale up to 3
		{
			name:             "3 failure domains, 0 machine, scale up from 0 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(),
			upToDateMachines: collections.FromMachines(),
			expected:         []string{a, b, c}, // select fd a, b or c because they all have no up-to-date machines
		},
		{
			name:             "3 failure domains, 1 machine, scale up from 1 to 2",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1),
			upToDateMachines: collections.FromMachines(machineA1),
			expected:         []string{b, c}, // select fd b or c because they all have no up-to-date machines
		},
		{
			name:             "3 failure domains, 2 machines, scale up from 2 to 3",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1),
			upToDateMachines: collections.FromMachines(machineA1, machineB1),
			expected:         []string{c}, // select fd c because it has no up-to-date machines
		},

		// scenario A.2: scale up during a rollout
		{
			name:             "3 failure domains, 3 outdated machines, scale up to 4",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineB1Old, machineC1Old),
			upToDateMachines: collections.FromMachines(),
			expected:         []string{a, b, c}, // select fd a, b or c because they all have no up-to-date machines, 1 machine overall
		},
		{
			name:             "3 failure domains, 2 outdated machines, 1 new machine, scale up to 4",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineB1Old, machineC1Old),
			upToDateMachines: collections.FromMachines(machineA1New),
			expected:         []string{b, c}, // select fd b or c because they all have no up-to-date machines (less than a)
		},
		{
			name:             "3 failure domains, 1 outdated machine, 2 new machines, scale up to 4",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineB1New, machineC1Old),
			upToDateMachines: collections.FromMachines(machineA1New, machineB1New),
			expected:         []string{c}, // select fd c because it has no up-to-date machines (less than a, b)
		},

		// scenario A.3: scale up after machines has been remediated or forcefully deleted
		{
			name:             "3 failure domains, 3 machines, 1 new machine, scale up to 4",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineC1Old, machineC1New),
			upToDateMachines: collections.FromMachines(machineC1New),
			expected:         []string{b}, // select fd b because it has no up-to-date machines (like a), 0 machine overall (less than a)
		},
		{
			name:             "3 failure domains, 3 machines, 2 new machines, scale up to 4",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineA1New, machineC1New),
			upToDateMachines: collections.FromMachines(machineA1New, machineC1New),
			expected:         []string{b}, // select fd b because it has no up-to-date machines (less than a, c)
		},

		// Use case B: 2 failure domains, 3 control plane machines

		// scenario B.1: scale up to 3
		{
			name:             "2 failure domains, 0 machine, scale up from 0 to 1",
			fds:              fds2,
			allMachines:      collections.FromMachines(),
			upToDateMachines: collections.FromMachines(),
			expected:         []string{a, b}, // select fd a, b because they all have no up-to-date machines
		},
		{
			name:             "2 failure domains, 1 machine, scale up from 1 to 2",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1),
			upToDateMachines: collections.FromMachines(machineA1),
			expected:         []string{b}, // select fd b because it has no up-to-date machines
		},
		{
			name:             "2 failure domains, 2 machines, scale up from 2 to 3",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1, machineB1),
			upToDateMachines: collections.FromMachines(machineA1, machineB1),
			expected:         []string{a, b}, // select fd a, b because they all have 1 up-to-date machine
		},

		// scenario B.2: scale up during a rollout
		{
			name:             "2 failure domains, 3 outdated machines, scale up to 4",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1Old, machineA2Old, machineB1Old),
			upToDateMachines: collections.FromMachines(),
			expected:         []string{b}, // select fd b because it has no up-to-date machines (like a), 1 machine overall (less than a)
		},
		{
			name:             "2 failure domains, 2 outdated machines, 1 new machine, scale up to 4",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1Old, machineB1Old, machineB1New),
			upToDateMachines: collections.FromMachines(machineB1New),
			expected:         []string{a}, // select fd a because it has no up-to-date machines (less than b)
		},
		{
			name:             "2 failure domains, 1 outdated machine, 2 new machines, scale up to 4",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1New, machineB1Old, machineB1New),
			upToDateMachines: collections.FromMachines(machineA1New, machineB1New),
			expected:         []string{a}, // select fd b because it has 1 up-to-date machine (like b), 1 machine overall (less than b)
		},

		// Use case C: 3 failure domains, 5 control plane machines

		// scenario C.1: scale up to 5
		{
			name:             "3 failure domains, 0 machine, scale up from 0 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(),
			upToDateMachines: collections.FromMachines(),
			expected:         []string{a, b, c}, // select fd a, b or c because they all have no up-to-date machines
		},
		{
			name:             "3 failure domains, 1 machine, scale up from 1 to 2",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1),
			upToDateMachines: collections.FromMachines(machineA1),
			expected:         []string{b, c}, // select fd b or c because they all have no up-to-date machines (less than a)
		},
		{
			name:             "3 failure domains, 2 machines, scale up from 2 to 3",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1),
			upToDateMachines: collections.FromMachines(machineA1, machineB1),
			expected:         []string{c}, // select fd c because it has no up-to-date machines (less than a, b)
		},
		{
			name:             "3 failure domains, 3 machines, scale up from 3 to 4",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineC1),
			upToDateMachines: collections.FromMachines(machineA1, machineB1, machineC1),
			expected:         []string{a, b, c}, // select fd a, b or c because they all have 1 up-to-date machines
		},
		{
			name:             "3 failure domains, 4 machines, scale up from 4 to 5",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineA2, machineB1, machineC1),
			upToDateMachines: collections.FromMachines(machineA1, machineA2, machineB1, machineC1),
			expected:         []string{b, c}, // select fd b or c because they all have no up-to-date machines (less than a)
		},

		// scenario C.2: scale up during a rollout
		{
			name:             "3 failure domains, 5 outdated machines, scale up to 6",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineA2Old, machineB1Old, machineB2Old, machineC1Old),
			upToDateMachines: collections.FromMachines(),
			expected:         []string{c}, // select fd c because it has no up-to-date machines (like a, b), 1 machine overall (less than a, b)
		},
		{
			name:             "3 failure domains, 4 outdated machines, 1 new machine, scale up to 6",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineB1Old, machineB2Old, machineC1Old, machineC1New),
			upToDateMachines: collections.FromMachines(machineC1New),
			expected:         []string{a}, // select fd a because it has no up-to-date machines (like b), 1 machine overall (less than b)
		},
		{
			name:             "3 failure domains, 3 outdated machines, 2 new machines, scale up to 6",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineA1New, machineB1Old, machineC1Old, machineC1New),
			upToDateMachines: collections.FromMachines(machineA1New, machineC1New),
			expected:         []string{b}, // select fd b because it has no up-to-date machines
		},
		{
			name:             "3 failure domains, 2 outdated machines, 3 new machines, scale up to 6",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineB1Old, machineB1New, machineC1Old, machineC1New),
			upToDateMachines: collections.FromMachines(machineA1New, machineB1New, machineC1New),
			expected:         []string{a}, // select fd a because it has 1 up-to-date machines (like b,c) 1 machine overall (less than b,c)
		},
		{
			name:             "3 failure domains, 1 outdated machine, 4 new machines, scale up to 6",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineA2New, machineB1New, machineC1Old, machineC1New),
			upToDateMachines: collections.FromMachines(machineA1New, machineA2New, machineB1New, machineC1New),
			expected:         []string{b}, // select fd b because it has 1 up-to-date machines (less than a, like c) 1 machine overall (less than c)
		},

		// Use case D: no failure domains, 3 control plane machines

		// scenario D.1: scale up to 3
		{
			name:             "3 failure domains, 0 machine, scale up from 0 to 1",
			fds:              fds0,
			allMachines:      collections.FromMachines(),
			upToDateMachines: collections.FromMachines(),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 1 machine, scale up from 1 to 2",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1),
			upToDateMachines: collections.FromMachines(machineA1),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 2 machines, scale up from 2 to 3",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1, machineB1),
			upToDateMachines: collections.FromMachines(machineA1, machineB1),
			expected:         nil, // no fd
		},

		// scenario D.2: scale up during a rollout
		{
			name:             "3 failure domains, 3 outdated machines, scale up to 4",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1Old, machineB1Old, machineC1Old),
			upToDateMachines: collections.FromMachines(),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 2 outdated machines, 1 new machine, scale up to 4",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1New, machineB1Old, machineC1Old),
			upToDateMachines: collections.FromMachines(machineA1New),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 1 outdated machine, 2 new machines, scale up to 4",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1New, machineB1New, machineC1Old),
			upToDateMachines: collections.FromMachines(machineA1New, machineB1New),
			expected:         nil, // no fd
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			fd := PickFewest(ctx, tc.fds, tc.allMachines, tc.upToDateMachines)
			if tc.expected == nil {
				g.Expect(fd).To(BeNil())
			} else {
				g.Expect(fd).ToNot(BeNil())
				g.Expect(tc.expected).To(ContainElement(*fd))
			}
		})
	}
}

func TestPickMostNew(t *testing.T) {
	a := "us-west-1a"
	b := "us-west-1b"
	c := "us-west-1c"

	fds3 := clusterv1.FailureDomains{
		a: clusterv1.FailureDomainSpec{ControlPlane: true},
		b: clusterv1.FailureDomainSpec{ControlPlane: true},
		c: clusterv1.FailureDomainSpec{ControlPlane: true},
	}

	fds2 := clusterv1.FailureDomains{
		a: clusterv1.FailureDomainSpec{ControlPlane: true},
		b: clusterv1.FailureDomainSpec{ControlPlane: true},
	}

	fds0 := clusterv1.FailureDomains{}

	machineA1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineA2 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a2"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineB1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineB2 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b2"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineC1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "c1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(c)}}
	machineA1Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a1-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineA2Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a2-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineB1Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b1-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineB2Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b2-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineC1Old := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "c1-old"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(c)}}
	machineA1New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a1-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineA2New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a2-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineB1New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b1-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineB2New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b2-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machineC1New := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "c1-new"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(c)}}

	testcases := []struct {
		name             string
		fds              clusterv1.FailureDomains
		allMachines      collections.Machines
		eligibleMachines collections.Machines
		expected         []string
	}{
		// Use case A: 3 failure domains, 3 control plane machines

		// scenario A.1: scale down during a rollout
		{
			name:             "3 failure domains, 3 outdated machines, 1 new machine, scale down from 4 to 3",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineA1New, machineB1Old, machineC1Old),
			eligibleMachines: collections.FromMachines(machineA1Old, machineB1Old, machineC1Old),
			expected:         []string{a}, // select fd a because it has 1 old machine (like b and c) but 2 machines overall (more than b and c)
		},
		{
			name:             "3 failure domains, 2 outdated machines, 2 new machines, scale down from 4 to 3",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineB1Old, machineB1New, machineC1Old),
			eligibleMachines: collections.FromMachines(machineB1Old, machineC1Old),
			expected:         []string{b}, // select fd b because it has 1 old machine (like c) but 2 machines overall (more c); fd a is discarded because it doesn't have any eligible machine
		},
		{
			name:             "3 failure domains, 1 outdated machine, 3 new machines, scale down from 4 to 3",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineB1New, machineC1Old, machineC1New),
			eligibleMachines: collections.FromMachines(machineC1Old),
			expected:         []string{c}, // select fd c because it has 1 old machine; fd a and b are discarded because they don't have any eligible machine
		},

		// scenario A.2: scale down to 0
		{
			name:             "3 failure domains, 3 machines, scale down from 3 to 2",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineA1, machineB1, machineC1),
			expected:         []string{a, b, c}, // select fd a, b or c because they have 1 eligible machine
		},
		{
			name:             "3 failure domains, 2 machines, scale down from 2 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineB1, machineC1),
			expected:         []string{b, c}, // select fd b or c because they have 1 eligible machine
		},
		{
			name:             "3 failure domains, 1 machine, scale down from 1 to 0",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineC1),
			eligibleMachines: collections.FromMachines(machineC1),
			expected:         []string{c}, // select fd c because it has 1 eligible machine
		},

		// scenario A.3: scale down or rollout when a machine with delete annotation or an un-healthy machine is prioritized for deletion
		{
			name:             "3 failure domains, 3 machines, 1 eligible machine",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineA1),
			expected:         []string{a}, // select fd a because it has 1 eligible machine
		},
		{
			name:             "3 failure domains, 3 machines, 2 eligible machines",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineA1, machineB1),
			expected:         []string{a, b}, // select fd a or b because they have 1 eligible machine
		},

		// Use case B: 2 failure domains, 3 control plane machines

		// scenario B.1: scale down during a rollout
		{
			name:             "2 failure domains, 3 outdated machines, 1 new machine, scale down from 4 to 3",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1Old, machineA2Old, machineB1Old, machineB2New),
			eligibleMachines: collections.FromMachines(machineA1Old, machineA2Old, machineB1Old),
			expected:         []string{a}, // select fd a because it has 2 old machines (more than b)
		},
		{
			name:             "2 failure domains, 2 outdated machines, 2 new machines, scale down from 4 to 3",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1New, machineA2Old, machineB1Old, machineB2New),
			eligibleMachines: collections.FromMachines(machineA2Old, machineB1Old),
			expected:         []string{a, b}, // select fd a or b because both have 1 old machine and 2 machines overall
		},
		{
			name:             "2 failure domains, 1 outdated machine, 3 new machines, scale down from 4 to 3",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1New, machineA2New, machineB1Old, machineB2New),
			eligibleMachines: collections.FromMachines(machineB1Old),
			expected:         []string{b}, // select fd b because it has 1 old machine (more than a)
		},

		// scenario B.2: scale down to 0
		{
			name:             "2 failure domains, 3 machines, scale down from 3 to 2",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1, machineA2, machineB1),
			eligibleMachines: collections.FromMachines(machineA1, machineA2, machineB1),
			expected:         []string{a}, // select fd a because it has 2 eligible machine (more than b)
		},
		{
			name:             "2 failure domains, 2 machines, scale down from 2 to 1",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1, machineB1),
			eligibleMachines: collections.FromMachines(machineA1, machineB1),
			expected:         []string{a, b}, // select fd a or b because they have 1 eligible machine
		},
		{
			name:             "2 failure domains, 1 machine, scale down from 1 to 0",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineB1),
			eligibleMachines: collections.FromMachines(machineB1),
			expected:         []string{b}, // select fd b because it has 1 eligible machine
		},

		// scenario B.3: scale down or rollout when a machine with delete annotation or an un-healthy machine is prioritized for deletion
		{
			name:             "2 failure domains, 3 machines, 1 eligible machine",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1, machineA2, machineB1),
			eligibleMachines: collections.FromMachines(machineB1),
			expected:         []string{b}, // select fd b because it has 1 eligible machine
		},
		{
			name:             "2 failure domains, 3 machines, 2 eligible machines",
			fds:              fds2,
			allMachines:      collections.FromMachines(machineA1, machineA2, machineB1),
			eligibleMachines: collections.FromMachines(machineA2, machineB1),
			expected:         []string{a}, // select fd a because it has 1 eligible machine (like b) and 2 machines overall (more than b)
		},

		// Use case C: 3 failure domains, 5 control plane machines

		// scenario C.1: scale down during a rollout
		{
			name:             "3 failure domains, 5 outdated machines, 1 new machine, scale down from 6 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineA2Old, machineB1Old, machineB2Old, machineC1Old, machineC1New),
			eligibleMachines: collections.FromMachines(machineA1Old, machineA2Old, machineB1Old, machineB2Old, machineC1Old),
			expected:         []string{a, b}, // select fd a or b because they have 2 old machine (more than c)
		},
		{
			name:             "3 failure domains, 4 outdated machines, 2 new machines, scale down from 6 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineA1New, machineB1Old, machineB2Old, machineC1Old, machineC1New),
			eligibleMachines: collections.FromMachines(machineA1Old, machineB1Old, machineB2Old, machineC1Old),
			expected:         []string{b}, // select fd b because it has 2 old machine (more than a and c)
		},
		{
			name:             "3 failure domains, 3 outdated machines, 3 new machines, scale down from 6 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1Old, machineA1New, machineB1Old, machineB1New, machineC1Old, machineC1New),
			eligibleMachines: collections.FromMachines(machineA1Old, machineB1Old, machineC1Old),
			expected:         []string{a, b, c}, // select fd a, b or c because they have 1 old machine
		},
		{
			name:             "3 failure domains, 2 outdated machines, 4 new machines, scale down from 6 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineA2New, machineB1Old, machineB1New, machineC1Old, machineC1New),
			eligibleMachines: collections.FromMachines(machineB1Old, machineC1Old),
			expected:         []string{b, c}, // select fd b or c because they have 1 old machine and 2 machines overall; fd a is discarded because it doesn't have any eligible machine
		},
		{
			name:             "3 failure domains, 1 outdated machine, 5 new machines, scale down from 6 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1New, machineA2New, machineB1New, machineB2New, machineC1Old, machineC1New),
			eligibleMachines: collections.FromMachines(machineC1Old),
			expected:         []string{c}, // select fd c because it has 1 old machine; fd a and b are discarded because they don't have any eligible machine
		},

		// scenario C.2: scale down to 0
		{
			name:             "3 failure domains, 5 machines, scale down from 5 to 4",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineA2, machineB1, machineB2, machineC1),
			eligibleMachines: collections.FromMachines(machineA1, machineA2, machineB1, machineB2, machineC1),
			expected:         []string{a, b}, // select fd a or b because they have 2 eligible machine (more than c)
		},
		{
			name:             "3 failure domains, 4 machines, scale down from 4 to 3",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineB2, machineC1),
			eligibleMachines: collections.FromMachines(machineA1, machineB1, machineB2, machineC1),
			expected:         []string{b}, // select fd b because they have 2 eligible machine (more than a, c)
		},
		{
			name:             "3 failure domains, 3 machines, scale down from 3 to 2",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineA1, machineB1, machineC1),
			expected:         []string{a, b, c}, // select fd a, b or c because they have 1 eligible machine
		},
		{
			name:             "3 failure domains, 2 machines, scale down from 2 to 1",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineB1, machineC1),
			expected:         []string{b, c}, // select fd b or c because they have 1 eligible machine
		},
		{
			name:             "3 failure domains, 1 machine, scale down from 1 to 0",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineC1),
			eligibleMachines: collections.FromMachines(machineC1),
			expected:         []string{c}, // select fd c because it has 1 eligible machine
		},

		// scenario C.3: scale down or rollout when a machine with delete annotation or an un-healthy machine is prioritized for deletion
		{
			name:             "3 failure domains, 5 machines, 1 eligible machine",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineA2, machineB1, machineB2, machineC1),
			eligibleMachines: collections.FromMachines(machineC1),
			expected:         []string{c}, // select fd c because it has 1 eligible machine
		},
		{
			name:             "3 failure domains, 5 machines, 2 eligible machines",
			fds:              fds3,
			allMachines:      collections.FromMachines(machineA1, machineA2, machineB1, machineB2, machineC1),
			eligibleMachines: collections.FromMachines(machineA2, machineC1),
			expected:         []string{a}, // select fd a because it has 1 eligible machine (like c) and 2 machines overall (more than c)
		},

		// Use case D: no failure domains, 3 control plane machines

		// scenario D.1: scale down during a rollout
		{
			name:             "3 failure domains, 3 outdated machines, 1 new machine, scale down from 4 to 3",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1Old, machineA1New, machineB1Old, machineC1Old),
			eligibleMachines: collections.FromMachines(machineA1Old, machineB1Old, machineC1Old),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 2 outdated machines, 2 new machines, scale down from 4 to 3",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1New, machineB1Old, machineB1New, machineC1Old),
			eligibleMachines: collections.FromMachines(machineB1Old, machineC1Old),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 1 outdated machines, 3 new machines, scale down from 4 to 3",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1New, machineB1New, machineC1Old, machineC1New),
			eligibleMachines: collections.FromMachines(machineC1Old),
			expected:         nil, // no fd
		},

		// scenario D.2: scale down to 0
		{
			name:             "3 failure domains, 3 machines, scale down from 3 to 2",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineA1, machineB1, machineC1),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 2 machines, scale down from 2 to 1",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineB1, machineC1),
			eligibleMachines: collections.FromMachines(machineB1, machineC1),
			expected:         nil, // no fd
		},
		{
			name:             "3 failure domains, 1 machine, scale down from 1 to 0",
			fds:              fds0,
			allMachines:      collections.FromMachines(machineC1),
			eligibleMachines: collections.FromMachines(machineC1),
			expected:         nil, // no fd
		},

		// Edge cases

		{
			name:             "No eligible machines", // Note: this should never happen (scale down is called only when there are eligible machines)
			fds:              fds0,
			allMachines:      collections.FromMachines(machineA1, machineB1, machineC1),
			eligibleMachines: collections.FromMachines(),
			expected:         nil,
		},
		{
			name:             "No machines", // Note: this should never happen (scale down is called only when there are machines)
			fds:              fds0,
			allMachines:      collections.FromMachines(),
			eligibleMachines: collections.FromMachines(),
			expected:         nil,
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			fd := PickMost(ctx, tc.fds, tc.allMachines, tc.eligibleMachines)
			if tc.expected == nil {
				g.Expect(fd).To(BeNil())
			} else {
				g.Expect(fd).ToNot(BeNil())
				g.Expect(tc.expected).To(ContainElement(*fd))
			}
		})
	}
}

func TestCountByFailureDomain(t *testing.T) {
	g := NewWithT(t)

	a := "us-west-1a"
	b := "us-west-1b"

	fds := clusterv1.FailureDomains{
		a: clusterv1.FailureDomainSpec{ControlPlane: true},
		b: clusterv1.FailureDomainSpec{ControlPlane: true},
	}
	machinea1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machinea2 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "a2"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(a)}}
	machineb1 := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "b1"}, Spec: clusterv1.MachineSpec{FailureDomain: ptr.To(b)}}
	machinenil := &clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "nil"}, Spec: clusterv1.MachineSpec{FailureDomain: nil}}

	allMachines := collections.FromMachines(machinea1, machinea2, machineb1, machinenil)
	priorityMachines := collections.FromMachines(machinea1)

	aggregations := countByFailureDomain(ctx, fds, allMachines, priorityMachines)

	g.Expect(aggregations).To(HaveLen(2))
	g.Expect(aggregations).To(ConsistOf(failureDomainAggregation{id: a, countPriority: 1, countAll: 2}, failureDomainAggregation{id: b, countPriority: 0, countAll: 1}))
}

func TestFailureDomainAggregationsSort(t *testing.T) {
	g := NewWithT(t)

	aggregations := failureDomainAggregations{
		{id: "fd1", countPriority: 2, countAll: 2},
		{id: "fd2", countPriority: 2, countAll: 3},
		{id: "fd3", countPriority: 2, countAll: 2},
		{id: "fd4", countPriority: 1, countAll: 5},
	}

	sort.Sort(aggregations)

	// the result should be sorted so the fd with less priority machines are first,
	// the number of overall machine should be used if the number of priority machines is equal,
	// position in the array is used to break ties.

	// fd4 is the failure domain with less priority machines, it should go first
	g.Expect(aggregations[0].id).To(Equal("fd4"))

	// fd1, fd2, fd3 all have the same number of priority machines;

	// fd1, fd3 have also the same number of overall machines.
	g.Expect(aggregations[1].id).To(Equal("fd1"))
	g.Expect(aggregations[2].id).To(Equal("fd3"))

	// fd2 has more overall machines, it should go last
	g.Expect(aggregations[3].id).To(Equal("fd2"))
}
