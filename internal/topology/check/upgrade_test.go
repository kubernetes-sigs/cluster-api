/*
Copyright 2024 The Kubernetes Authors.

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

package check

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestIsMachineDeploymentUpgrading(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name     string
		md       *clusterv1.MachineDeployment
		machines []*clusterv1.Machine
		want     bool
		wantErr  bool
	}{
		{
			name: "should return false if all the machines of MachineDeployment have the same version as the MachineDeployment",
			md: builder.MachineDeployment("ns", "md1").
				WithClusterName("cluster1").
				WithVersion("v1.2.3").
				Build(),
			machines: []*clusterv1.Machine{
				builder.Machine("ns", "machine1").
					WithClusterName("cluster1").
					WithVersion("v1.2.3").
					Build(),
				builder.Machine("ns", "machine2").
					WithClusterName("cluster1").
					WithVersion("v1.2.3").
					Build(),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "should return true if at least one of the machines of MachineDeployment has a different version",
			md: builder.MachineDeployment("ns", "md1").
				WithClusterName("cluster1").
				WithVersion("v1.2.3").
				Build(),
			machines: []*clusterv1.Machine{
				builder.Machine("ns", "machine1").
					WithClusterName("cluster1").
					WithVersion("v1.2.3").
					Build(),
				builder.Machine("ns", "machine2").
					WithClusterName("cluster1").
					WithVersion("v1.2.2").
					Build(),
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "should return false if the MachineDeployment has no machines (creation phase)",
			md: builder.MachineDeployment("ns", "md1").
				WithClusterName("cluster1").
				WithVersion("v1.2.3").
				Build(),
			machines: []*clusterv1.Machine{},
			want:     false,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			objs := []client.Object{}
			objs = append(objs, tt.md)
			for _, m := range tt.machines {
				objs = append(objs, m)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			got, err := IsMachineDeploymentUpgrading(ctx, fakeClient, tt.md)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestIsMachinePoolUpgrading(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name    string
		mp      *expv1.MachinePool
		nodes   []*corev1.Node
		want    bool
		wantErr bool
	}{
		{
			name: "should return false if all the nodes of MachinePool have the same version as the MachinePool",
			mp: builder.MachinePool("ns", "mp1").
				WithClusterName("cluster1").
				WithVersion("v1.2.3").
				WithStatus(expv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{
						{Name: "node1"},
						{Name: "node2"},
					},
				}).
				Build(),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.2.3"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node2"},
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.2.3"},
					},
				},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "should return true if at least one of the nodes of MachinePool has a different version",
			mp: builder.MachinePool("ns", "mp1").
				WithClusterName("cluster1").
				WithVersion("v1.2.3").
				WithStatus(expv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{
						{Name: "node1"},
						{Name: "node2"},
					},
				}).
				Build(),
			nodes: []*corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node1"},
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.2.3"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node2"},
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.2.2"},
					},
				},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "should return false if the MachinePool has no nodes (creation phase)",
			mp: builder.MachinePool("ns", "mp1").
				WithClusterName("cluster1").
				WithVersion("v1.2.3").
				WithStatus(expv1.MachinePoolStatus{
					NodeRefs: []corev1.ObjectReference{},
				}).
				Build(),
			nodes:   []*corev1.Node{},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			objs := []client.Object{}
			for _, m := range tt.nodes {
				objs = append(objs, m)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			got, err := IsMachinePoolUpgrading(ctx, fakeClient, tt.mp)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}
