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

package scope

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

func TestMDUpgrading(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	ctx := context.Background()

	t.Run("should return the names of the upgrading MachineDeployments", func(*testing.T) {
		stableMD := builder.MachineDeployment("ns", "stableMD").
			WithClusterName("cluster1").
			WithVersion("v1.2.3").
			Build()
		stableMDMachine := builder.Machine("ns", "stableMD-machine1").
			WithClusterName("cluster1").
			WithVersion("v1.2.3").
			Build()

		upgradingMD := builder.MachineDeployment("ns", "upgradingMD").
			WithClusterName("cluster2").
			WithVersion("v1.2.3").
			Build()
		upgradingMDMachine := builder.Machine("ns", "upgradingMD-machine1").
			WithClusterName("cluster2").
			WithVersion("v1.2.2").
			Build()

		objs := []client.Object{stableMD, stableMDMachine, upgradingMD, upgradingMDMachine}
		fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithScheme(scheme).Build()

		mdsStateMap := MachineDeploymentsStateMap{
			"stableMD":    {Object: stableMD},
			"upgradingMD": {Object: upgradingMD},
		}
		want := []string{"upgradingMD"}

		got, err := mdsStateMap.Upgrading(ctx, fakeClient)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(BeComparableTo(want))
	})
}

func TestMDRollingOut(t *testing.T) {
	scheme := runtime.NewScheme()
	NewWithT(t).Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	newMD := func(name string, generation, observedGeneration int64, rollingOut metav1.ConditionStatus) *clusterv1.MachineDeployment {
		return builder.MachineDeployment("ns", name).
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
	}
	newMS := func(name, version string, replicas int32) *clusterv1.MachineSet {
		ms := builder.MachineSet("ns", name).
			WithClusterName("cluster1").
			// Labels must match the MachineDeployment selector defaulted from the cluster name.
			WithLabels(map[string]string{clusterv1.ClusterNameLabel: "cluster1"}).
			WithReplicas(ptr.To(replicas)).
			Build()
		ms.Spec.Template.Spec.Version = version
		return ms
	}

	tests := []struct {
		name        string
		md          *clusterv1.MachineDeployment
		machineSets []*clusterv1.MachineSet
		want        bool
	}{
		{
			name:        "should return true if RollingOut condition is true",
			md:          newMD("md-1", 1, 1, metav1.ConditionTrue),
			machineSets: []*clusterv1.MachineSet{newMS("md-1-ms", "v1.2.3", 1)},
			want:        true,
		},
		{
			name:        "should return false if RollingOut condition is false and the MachineSets match the MachineDeployment template",
			md:          newMD("md-2", 1, 1, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{newMS("md-2-ms", "v1.2.3", 1)},
			want:        false,
		},
		{
			name:        "should return true if observedGeneration is stale",
			md:          newMD("md-3", 2, 1, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{newMS("md-3-ms", "v1.2.3", 1)},
			want:        true,
		},
		{
			name: "should return true if a MachineSet not matching the MachineDeployment template still has replicas even if RollingOut condition is false",
			md:   newMD("md-4", 2, 2, metav1.ConditionFalse),
			machineSets: []*clusterv1.MachineSet{
				newMS("md-4-ms-new", "v1.2.3", 1),
				newMS("md-4-ms-old", "v1.2.2", 1),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			objs := []client.Object{tt.md}
			machineSets := make([]clusterv1.MachineSet, 0, len(tt.machineSets))
			for _, ms := range tt.machineSets {
				objs = append(objs, ms)
				machineSets = append(machineSets, *ms)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			mdsStateMap := MachineDeploymentsStateMap{
				tt.md.Name: {Object: tt.md},
			}

			rollingOut, err := (&MachineDeploymentState{Object: tt.md}).IsRollingOut(machineSets)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(rollingOut).To(Equal(tt.want))

			rollingOutNames, err := mdsStateMap.RollingOut(ctx, fakeClient)
			g.Expect(err).ToNot(HaveOccurred())
			if tt.want {
				g.Expect(rollingOutNames).To(ConsistOf(tt.md.Name))
			} else {
				g.Expect(rollingOutNames).To(BeEmpty())
			}
		})
	}
}

func TestMPUpgrading(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	ctx := context.Background()

	t.Run("should return the names of the upgrading MachinePools", func(*testing.T) {
		stableMP := builder.MachinePool("ns", "stableMP").
			WithClusterName("cluster1").
			WithVersion("v1.2.3").
			WithStatus(clusterv1.MachinePoolStatus{
				NodeRefs: []corev1.ObjectReference{
					{
						Name: "stableMP-node1",
					},
				},
			}).
			Build()
		stableMPNode := builder.Node("stableMP-node1").
			WithStatus(corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion: "v1.2.3",
				},
			}).
			Build()

		upgradingMP := builder.MachinePool("ns", "upgradingMP").
			WithClusterName("cluster2").
			WithVersion("v1.2.3").
			WithStatus(clusterv1.MachinePoolStatus{
				NodeRefs: []corev1.ObjectReference{
					{
						Name: "upgradingMP-node1",
					},
				},
			}).
			Build()
		upgradingMPNode := builder.Node("upgradingMP-node1").
			WithStatus(corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion: "v1.2.2",
				},
			}).
			Build()

		objs := []client.Object{stableMP, stableMPNode, upgradingMP, upgradingMPNode}
		fakeClient := fake.NewClientBuilder().WithObjects(objs...).WithScheme(scheme).Build()

		mpsStateMap := MachinePoolsStateMap{
			"stableMP":    {Object: stableMP},
			"upgradingMP": {Object: upgradingMP},
		}
		want := []string{"upgradingMP"}

		got, err := mpsStateMap.Upgrading(ctx, fakeClient)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(BeComparableTo(want))
	})
}
