/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func Test_getActiveMachinesInCluster(t *testing.T) {
	ns1Cluster1 := clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns1cluster1",
			Namespace: "test-ns-1",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster-1",
			},
		},
	}
	ns1Cluster2 := clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns1cluster2",
			Namespace: "test-ns-1",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster-2",
			},
		},
	}
	time := metav1.Now()
	ns1Cluster1Deleted := clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns1cluster1deleted",
			Namespace: "test-ns-1",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster-2",
			},
			DeletionTimestamp: &time,
		},
	}
	ns2Cluster2 := clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind: "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns2cluster2",
			Namespace: "test-ns-2",
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "test-cluster-2",
			},
		},
	}

	type args struct {
		namespace string
		name      string
	}
	tests := []struct {
		name    string
		args    args
		want    []*clusterv1.Machine
		wantErr bool
	}{
		{
			name: "ns1 cluster1",
			args: args{
				namespace: "test-ns-1",
				name:      "test-cluster-1",
			},
			want:    []*clusterv1.Machine{&ns1Cluster1},
			wantErr: false,
		},
		{
			name: "ns2 cluster2",
			args: args{
				namespace: "test-ns-2",
				name:      "test-cluster-2",
			},
			want:    []*clusterv1.Machine{&ns2Cluster2},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(clusterv1.AddToScheme(scheme.Scheme)).To(Succeed())

			c := fake.NewFakeClientWithScheme(scheme.Scheme, &ns1Cluster1, &ns1Cluster2, &ns1Cluster1Deleted, &ns2Cluster2)
			got, err := getActiveMachinesInCluster(context.TODO(), c, tt.args.namespace, tt.args.name)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestMachineHealthCheckHasMatchingLabels(t *testing.T) {
	testCases := []struct {
		name     string
		selector metav1.LabelSelector
		labels   map[string]string
		expected bool
	}{
		{
			name: "selector matches labels",

			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},

			labels: map[string]string{
				"foo": "bar",
			},

			expected: true,
		},
		{
			name: "selector does not match labels",

			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},

			labels: map[string]string{
				"no": "match",
			},
			expected: false,
		},
		{
			name:     "selector is empty",
			selector: metav1.LabelSelector{},
			labels:   map[string]string{},
			expected: false,
		},
		{
			name: "seelctor is invalid",
			selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Operator: "bad-operator",
					},
				},
			},
			labels: map[string]string{
				"foo": "bar",
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			got := hasMatchingLabels(tc.selector, tc.labels)
			g.Expect(got).To(Equal(tc.expected))
		})
	}
}
