/*
Copyright 2025 The Kubernetes Authors.

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

package index

import (
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestIndexMachineByNodeName(t *testing.T) {
	testCases := []struct {
		name     string
		object   client.Object
		expected []string
	}{
		{
			name:     "when the machine has no NodeRef",
			object:   &clusterv1.Machine{},
			expected: []string{},
		},
		{
			name: "when the machine has valid a NodeRef",
			object: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					NodeRef: clusterv1.MachineNodeReference{
						Name: "node1",
					},
				},
			},
			expected: []string{"node1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := MachineByNodeName(tc.object)
			g.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestIndexMachineByProviderID(t *testing.T) {
	validProviderID := "aws://region/zone/id"

	testCases := []struct {
		name     string
		object   client.Object
		expected []string
	}{
		{
			name:     "Machine has no providerID",
			object:   &clusterv1.Machine{},
			expected: nil,
		},
		{
			name: "Machine has invalid providerID",
			object: &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					ProviderID: "",
				},
			},
			expected: nil,
		},
		{
			name: "Machine has valid providerID",
			object: &clusterv1.Machine{
				Spec: clusterv1.MachineSpec{
					ProviderID: validProviderID,
				},
			},
			expected: []string{validProviderID},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := machineByProviderID(tc.object)
			g.Expect(got).To(BeEquivalentTo(tc.expected))
		})
	}
}
