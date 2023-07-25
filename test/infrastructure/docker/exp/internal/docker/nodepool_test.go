/*
Copyright 2023 The Kubernetes Authors.

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

// Package docker implements docker functionality.
package docker

import (
	"fmt"
	"sort"
	"testing"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/test/infrastructure/docker/internal/docker"
)

func getMachines(length int) []*docker.Machine {
	machines := []*docker.Machine{}
	for i := 0; i < length; i++ {
		machine := &docker.Machine{}
		machine.SetName(fmt.Sprintf("instance-%d", i+1))
		machines = append(machines, machine)
	}

	return machines
}

func TestMachineDeleteOrder(t *testing.T) {
	testcases := []struct {
		name             string
		nodePoolMachines NodePoolMachines
	}{
		{
			name: "no prioritized instances",
			nodePoolMachines: NodePoolMachines{
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-1",
						PrioritizeDelete: false,
					},
				},
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-2",
						PrioritizeDelete: false,
					},
				},
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-3",
						PrioritizeDelete: false,
					},
				},
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-4",
						PrioritizeDelete: false,
					},
				},
			},
		},
		{
			name: "sort prioritized instances to front",
			nodePoolMachines: NodePoolMachines{
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-1",
						PrioritizeDelete: false,
					},
				},
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-2",
						PrioritizeDelete: false,
					},
				},
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-3",
						PrioritizeDelete: true,
					},
				},
				{
					Status: &NodePoolMachineStatus{
						Name:             "instance-4",
						PrioritizeDelete: true,
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			sort.Sort(tc.nodePoolMachines)

			var prevMachine NodePoolMachine
			for i, nodePoolMachine := range tc.nodePoolMachines {
				if nodePoolMachine.Status.PrioritizeDelete && i > 0 {
					g.Expect(prevMachine.Status.PrioritizeDelete).To(BeTrue())
				}

			}
		})
	}
}
