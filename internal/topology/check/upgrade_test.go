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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
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

func TestIsMachineDeploymentRollingOut(t *testing.T) {
	newMD := func(generation, observedGeneration int64, rollingOut metav1.ConditionStatus) *clusterv1.MachineDeployment {
		md := builder.MachineDeployment("ns", "md1").
			WithClusterName("cluster1").
			WithVersion("v1.2.3").
			WithGeneration(generation).
			WithStatus(clusterv1.MachineDeploymentStatus{
				ObservedGeneration: observedGeneration,
				Conditions: []metav1.Condition{
					{
						Type:   clusterv1.MachineDeploymentRollingOutCondition,
						Status: rollingOut,
					},
				},
			}).
			Build()
		md.Spec.Selector.MatchLabels[clusterv1.ClusterTopologyMachineDeploymentNameLabel] = "md1-topology"
		md.Spec.Template.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel] = "md1-topology"
		return md
	}
	newMS := func(name, version string, replicas int32) *clusterv1.MachineSet {
		ms := builder.MachineSet("ns", name).
			WithClusterName("cluster1").
			WithLabels(map[string]string{
				clusterv1.ClusterNameLabel:                          "cluster1",
				clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md1-topology",
			}).
			WithReplicas(ptr.To(replicas)).
			Build()
		ms.Spec.Template.Spec.Version = version
		return ms
	}
	otherMS := newMS("other-ms-old", "v1.2.2", 1)
	otherMS.Labels[clusterv1.ClusterTopologyMachineDeploymentNameLabel] = "other-topology"

	tests := []struct {
		name        string
		md          *clusterv1.MachineDeployment
		machineSets []*clusterv1.MachineSet
		want        bool
	}{
		{
			name: "should return true if the RollingOut condition is true",
			md:   newMD(1, 1, metav1.ConditionTrue),
			machineSets: []*clusterv1.MachineSet{
				newMS("ms-new", "v1.2.3", 1),
			},
			want: true,
		},
		{
			name: "should return true if the controller has not observed the latest generation",
			md:   newMD(2, 1, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{
				newMS("ms-new", "v1.2.3", 1),
			},
			want: true,
		},
		{
			name: "should return true if a MachineSet not matching the MachineDeployment template still has replicas",
			// Regression test for the window in which the MachineDeployment controller already observed a template
			// change but the Machines' UpToDate conditions, and thus the RollingOut condition, are not updated yet.
			md: newMD(2, 2, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{
				newMS("ms-new", "v1.2.3", 1),
				newMS("ms-old", "v1.2.2", 1),
			},
			want: true,
		},
		{
			name: "should return false if the only MachineSet matches the MachineDeployment template",
			md:   newMD(2, 2, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{
				newMS("ms-new", "v1.2.3", 1),
			},
			want: false,
		},
		{
			name: "should ignore MachineSets not selected by the MachineDeployment",
			md:   newMD(2, 2, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{
				newMS("ms-new", "v1.2.3", 1),
				otherMS,
			},
			want: false,
		},
		{
			name: "should return false if MachineSets not matching the MachineDeployment template are fully scaled down",
			md:   newMD(2, 2, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{
				newMS("ms-new", "v1.2.3", 1),
				newMS("ms-old", "v1.2.2", 0),
			},
			want: false,
		},
		{
			name: "should return true if no MachineSet matches the MachineDeployment template",
			md:   newMD(2, 2, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{
				newMS("ms-old", "v1.2.2", 0),
			},
			want: true,
		},
		{
			name:        "should return true if the MachineDeployment has no MachineSets yet",
			md:          newMD(1, 1, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{},
			want:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			machineSets := make([]clusterv1.MachineSet, 0, len(tt.machineSets))
			for _, ms := range tt.machineSets {
				machineSets = append(machineSets, *ms)
			}

			got, err := IsMachineDeploymentRollingOut(tt.md, machineSets)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestIsMachinePoolUpgrading(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name    string
		mp      *clusterv1.MachinePool
		nodes   []*corev1.Node
		want    bool
		wantErr bool
	}{
		{
			name: "should return false if all the nodes of MachinePool have the same version as the MachinePool",
			mp: builder.MachinePool("ns", "mp1").
				WithClusterName("cluster1").
				WithVersion("v1.2.3").
				WithStatus(clusterv1.MachinePoolStatus{
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
				WithStatus(clusterv1.MachinePoolStatus{
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
				WithStatus(clusterv1.MachinePoolStatus{
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
