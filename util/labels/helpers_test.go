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

package labels

import (
	"regexp"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

func TestHasWatchLabel(t *testing.T) {
	g := NewWithT(t)

	var testcases = []struct {
		name     string
		obj      metav1.Object
		input    string
		expected bool
	}{
		{
			name: "should handle empty input",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input:    "",
			expected: false,
		},
		{
			name: "should return false if no input string is give",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.WatchLabel: "bar",
					},
				},
			},
			input:    "",
			expected: false,
		},
		{
			name: "should return true if label matches",
			obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.WatchLabel: "bar",
					},
				},
				Spec:   corev1.NodeSpec{},
				Status: corev1.NodeStatus{},
			},
			input:    "bar",
			expected: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(*testing.T) {
			res := HasWatchLabel(tc.obj, tc.input)
			g.Expect(res).To(Equal(tc.expected))
		})
	}
}

func TestIsMachinePoolOwned(t *testing.T) {
	tests := []struct {
		name     string
		object   metav1.Object
		expected bool
	}{
		{
			name: "machine is a MachinePoolMachine",
			object: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.MachinePoolNameLabel: "foo",
					},
				},
			},
			expected: true,
		},
		{
			name: "random type is machinepool owned",
			object: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						clusterv1.MachinePoolNameLabel: "foo",
					},
				},
			},
			expected: true,
		},
		{
			name: "machine not a MachinePoolMachine with random labels",
			object: &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expected: false,
		},
		{
			name:     "machine is not a MachinePoolMachine with no labels",
			object:   &clusterv1.Machine{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result := IsMachinePoolOwned(tt.object)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestGetManagedLabels(t *testing.T) {
	defaultLabels := map[string]string{
		clusterv1.NodeRoleLabelPrefix + "/anyRole": "",

		clusterv1.ManagedNodeLabelDomain:                                  "",
		"custom-prefix." + clusterv1.ManagedNodeLabelDomain:               "",
		clusterv1.ManagedNodeLabelDomain + "/anything":                    "",
		"custom-prefix." + clusterv1.ManagedNodeLabelDomain + "/anything": "",

		clusterv1.NodeRestrictionLabelDomain:                                  "",
		"custom-prefix." + clusterv1.NodeRestrictionLabelDomain:               "",
		clusterv1.NodeRestrictionLabelDomain + "/anything":                    "",
		"custom-prefix." + clusterv1.NodeRestrictionLabelDomain + "/anything": "",
	}

	additionalLabels := map[string]string{
		"foo":                               "bar",
		"bar":                               "baz",
		"company.xyz/node.cluster.x-k8s.io": "not-managed",
		"gpu-node.cluster.x-k8s.io":         "not-managed",
		"company.xyz/node-restriction.kubernetes.io": "not-managed",
		"gpu-node-restriction.kubernetes.io":         "not-managed",
		"wrong.test.foo.com":                         "",
	}

	exampleRegex := regexp.MustCompile(`foo`)
	defaultAndRegexLabels := map[string]string{}
	for k, v := range defaultLabels {
		defaultAndRegexLabels[k] = v
	}
	defaultAndRegexLabels["foo"] = "bar"
	defaultAndRegexLabels["wrong.test.foo.com"] = ""

	allLabels := map[string]string{}
	for k, v := range defaultLabels {
		allLabels[k] = v
	}
	for k, v := range additionalLabels {
		allLabels[k] = v
	}

	tests := []struct {
		name                        string
		additionalSyncMachineLabels []*regexp.Regexp
		allLabels                   map[string]string
		managedLabels               map[string]string
	}{
		{
			name:                        "always sync default labels",
			additionalSyncMachineLabels: nil,
			allLabels:                   allLabels,
			managedLabels:               defaultLabels,
		},
		{
			name: "sync additional defined labels",
			additionalSyncMachineLabels: []*regexp.Regexp{
				exampleRegex,
			},
			allLabels:     allLabels,
			managedLabels: defaultAndRegexLabels,
		},
		{
			name: "sync all labels",
			additionalSyncMachineLabels: []*regexp.Regexp{
				regexp.MustCompile(`.*`),
			},
			allLabels:     allLabels,
			managedLabels: allLabels,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := GetManagedLabels(tt.allLabels, tt.additionalSyncMachineLabels...)
			g.Expect(got).To(BeEquivalentTo(tt.managedLabels))
		})
	}
}
