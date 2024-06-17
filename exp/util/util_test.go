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

// Package util implements utility functions.
package util

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/labels/format"
)

func TestGetMachinePoolByLabels(t *testing.T) {
	g := NewWithT(t)

	longMachinePoolName := "this-is-a-very-long-machinepool-name-that-will-turned-into-a-hash-because-it-is-longer-than-63-characters"
	namespace := "default"

	testcases := []struct {
		name                    string
		labels                  map[string]string
		machinePools            []client.Object
		expectedMachinePoolName string
		expectedError           string
	}{
		{
			name: "returns a MachinePool with matching labels",
			labels: map[string]string{
				clusterv1.MachinePoolNameLabel: "test-pool",
			},
			machinePools: []client.Object{
				&expv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pool",
						Namespace: "default",
					},
				},
				&expv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-pool",
						Namespace: "default",
					},
				},
			},
			expectedMachinePoolName: "test-pool",
		},
		{
			name: "returns a MachinePool with matching labels and cluster name is included",
			labels: map[string]string{
				clusterv1.MachinePoolNameLabel: "test-pool",
				clusterv1.ClusterNameLabel:     "test-cluster",
			},
			machinePools: []client.Object{
				&expv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pool",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: "test-cluster",
						},
					},
				},
				&expv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-pool",
						Namespace: "default",
						Labels: map[string]string{
							clusterv1.ClusterNameLabel: "test-cluster",
						},
					},
				},
			},
			expectedMachinePoolName: "test-pool",
		},
		{
			name: "returns a MachinePool where label is a hash",
			labels: map[string]string{
				clusterv1.MachinePoolNameLabel: format.MustFormatValue(longMachinePoolName),
			},
			machinePools: []client.Object{
				&expv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      longMachinePoolName,
						Namespace: "default",
					},
				},
				&expv1.MachinePool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-pool",
						Namespace: "default",
					},
				},
			},
			expectedMachinePoolName: longMachinePoolName,
		},
		{
			name:          "missing required key",
			labels:        map[string]string{},
			expectedError: fmt.Sprintf("labels missing required key `%s`", clusterv1.MachinePoolNameLabel),
		},
		{
			name: "returns nil when no machine pool matches",
			labels: map[string]string{
				clusterv1.MachinePoolNameLabel: "test-pool",
			},
			machinePools:            []client.Object{},
			expectedMachinePoolName: "",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(*testing.T) {
			clientFake := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(
					tc.machinePools...,
				).Build()

			mp, err := GetMachinePoolByLabels(ctx, clientFake, namespace, tc.labels)
			if tc.expectedError != "" {
				g.Expect(err).To(MatchError(tc.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				if tc.expectedMachinePoolName != "" {
					g.Expect(mp).ToNot(BeNil())
					g.Expect(mp.Name).To(Equal(tc.expectedMachinePoolName))
				} else {
					g.Expect(mp).To(BeNil())
				}
			}
		})
	}
}
