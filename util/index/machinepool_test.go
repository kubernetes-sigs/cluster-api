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
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestIndexMachinePoolByNodeName(t *testing.T) {
	testCases := []struct {
		name     string
		object   client.Object
		expected []string
	}{
		{
			name:     "when the machinepool has no NodeRef",
			object:   &clusterv1.MachinePool{},
			expected: []string{},
		},
		{
			name: "when the machinepool has valid NodeRefs",
			object: &clusterv1.MachinePool{
				Status: clusterv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{
						{
							Name: "node1",
						},
						{
							Name: "node2",
						},
					},
				},
			},
			expected: []string{"node1", "node2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := MachinePoolByNodeName(tc.object)
			g.Expect(got).To(ConsistOf(tc.expected))
		})
	}
}

func TestIndexMachinePoolByProviderID(t *testing.T) {
	validProviderID := "aws://region/zone/1"
	otherValidProviderID := "aws://region/zone/2"

	testCases := []struct {
		name     string
		object   client.Object
		expected []string
	}{
		{
			name:     "MachinePool has no providerID",
			object:   &clusterv1.MachinePool{},
			expected: nil,
		},
		{
			name: "MachinePool has invalid providerID",
			object: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					ProviderIDList: []string{""},
				},
			},
			expected: []string{},
		},
		{
			name: "MachinePool has valid providerIDs",
			object: &clusterv1.MachinePool{
				Spec: clusterv1.MachinePoolSpec{
					ProviderIDList: []string{validProviderID, otherValidProviderID},
				},
			},
			expected: []string{validProviderID, otherValidProviderID},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := machinePoolByProviderID(tc.object)
			g.Expect(got).To(BeEquivalentTo(tc.expected))
		})
	}
}
