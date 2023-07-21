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
		machines         []*docker.Machine
		nodePoolMachines []NodePoolMachine
	}{
		{
			name:     "no prioritized instances",
			machines: getMachines(4),
			nodePoolMachines: []NodePoolMachine{
				{
					Name:             "instance-1",
					PrioritizeDelete: false,
				},
				{
					Name:             "instance-2",
					PrioritizeDelete: false,
				},
				{
					Name:             "instance-3",
					PrioritizeDelete: false,
				},
				{
					Name:             "instance-4",
					PrioritizeDelete: false,
				},
			},
		},
		{
			name:     "sort prioritized instances to front",
			machines: getMachines(4),
			nodePoolMachines: []NodePoolMachine{
				{
					Name:             "instance-1",
					PrioritizeDelete: false,
				},
				{
					Name:             "instance-2",
					PrioritizeDelete: false,
				},
				{
					Name:             "instance-3",
					PrioritizeDelete: true,
				},
				{
					Name:             "instance-4",
					PrioritizeDelete: true,
				},
			},
		},
	}

	for _, tc := range testcases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			nodePool := &NodePool{
				nodePoolMachines: tc.nodePoolMachines,
				machines:         tc.machines,
			}

			sort.Sort(nodePool)

			instanceMap := map[string]NodePoolMachine{}
			for _, instance := range nodePool.nodePoolMachines {
				instanceMap[instance.Name] = instance
			}

			var prevInstance NodePoolMachine
			for i, machine := range nodePool.machines {
				instance, ok := instanceMap[machine.Name()]
				g.Expect(ok).To(BeTrue())
				if instance.PrioritizeDelete && i > 0 {
					g.Expect(prevInstance.PrioritizeDelete).To(BeTrue())
				}

				prevInstance = instanceMap[nodePool.machines[i].Name()]
			}
		})
	}
}
