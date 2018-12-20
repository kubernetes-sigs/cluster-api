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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func TestHasMatchingLabels(t *testing.T) {
	testCases := []struct {
		machineSet v1alpha1.MachineSet
		machine    v1alpha1.Machine
		expected   bool
	}{
		{
			machineSet: v1alpha1.MachineSet{
				Spec: v1alpha1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "matchSelector",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: true,
		},
		{
			machineSet: v1alpha1.MachineSet{
				Spec: v1alpha1.MachineSetSpec{
					Selector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
			machine: v1alpha1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Name: "doesNotMatchSelector",
					Labels: map[string]string{
						"no": "match",
					},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		got := hasMatchingLabels(&tc.machineSet, &tc.machine)
		if tc.expected != got {
			t.Errorf("Case %s. Got: %v, expected %v", tc.machine.Name, got, tc.expected)
		}
	}
}
