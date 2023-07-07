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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestIsUpgrading(t *testing.T) {
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
			mdState := &MachineDeploymentState{
				Object: tt.md,
			}
			got, err := mdState.IsUpgrading(ctx, fakeClient)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestUpgrading(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())

	ctx := context.Background()

	t.Run("should return the names of the upgrading MachineDeployments", func(t *testing.T) {
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
