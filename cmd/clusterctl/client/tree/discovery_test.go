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

package tree

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
)

func clusterObjectsWithResourceSet() []client.Object {
	namespace := "ns1"
	clusterObjs := test.NewFakeCluster(namespace, "cluster1").
		WithControlPlane(
			test.NewFakeControlPlane("cp").
				WithMachines(
					test.NewFakeMachine("cp1"),
				),
		).
		WithMachineDeployments(
			test.NewFakeMachineDeployment("md1").
				WithMachineSets(
					test.NewFakeMachineSet("ms1").
						WithMachines(
							test.NewFakeMachine("m1"),
							test.NewFakeMachine("m2"),
						),
				),
		).
		Objs()

	var cluster *clusterv1.Cluster
	for _, obj := range clusterObjs {
		if obj.GetObjectKind().GroupVersionKind().Kind == "Cluster" {
			cluster = obj.(*clusterv1.Cluster)
		}
	}
	resourceSet := test.NewFakeClusterResourceSet(namespace, "crs1")
	resourceSetObjs := resourceSet.ApplyToCluster(cluster).Objs()

	return append(clusterObjs, resourceSetObjs...)
}

func Test_Discovery(t *testing.T) {
	type nodeCheck func(*WithT, client.Object)
	type args struct {
		objs            []client.Object
		discoverOptions DiscoverOptions
	}
	tests := []struct {
		name          string
		args          args
		wantTree      map[string][]string
		wantNodeCheck map[string]nodeCheck
	}{
		{
			name: "Discovery with default discovery settings",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping: true,
				},
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp").
							WithMachines(
								test.NewFakeMachine("cp1"),
							),
					).
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md1").
							WithMachineSets(
								test.NewFakeMachineSet("ms1").
									WithMachines(
										test.NewFakeMachine("m1"),
										test.NewFakeMachine("m2"),
									),
							),
					).
					Objs(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, and WorkerNodes
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
				},
				// Machine should be leaf (no echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {},
				// Workers should have a machine deployment
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
				},
				// Machine deployment should have a group of machines (grouping)
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					"virtual.cluster.x-k8s.io/v1beta1, Kind=MachineGroup, ns1/zzz_",
				},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine deployment should be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
			},
		},
		{
			name: "Discovery with grouping disabled",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping: false,
				},
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp").
							WithMachines(
								test.NewFakeMachine("cp1"),
							),
					).
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md1").
							WithMachineSets(
								test.NewFakeMachineSet("ms1").
									WithMachines(
										test.NewFakeMachine("m1"),
										test.NewFakeMachine("m2"),
									),
							),
					).
					Objs(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, and WorkerNodes
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
				},
				// Workers should have a machine deployment
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
				},
				// Machine deployment should have a group of machines
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m2",
				},
				// Machine should be leaf (no echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {},
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1":  {},
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m2":  {},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, should NOT be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeFalse())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine deployment should NOT be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeFalse())
				},
			},
		},
		{
			name: "Discovery with MachinePool Machines with echo enabled",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping: false,
					Echo:     true,
				},
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp").
							WithMachines(
								test.NewFakeMachine("cp1"),
							),
					).
					WithMachinePools(
						test.NewFakeMachinePool("mp1").
							WithMachines(
								test.NewFakeMachine("mp1m1"),
								test.NewFakeMachine("mp1m2"),
							),
					).
					Objs(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, and WorkerNodes
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
				},
				// Workers should have a machine deployment
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachinePool, ns1/mp1",
				},
				// Machine Pool should have a group of machines
				"cluster.x-k8s.io/v1beta1, Kind=MachinePool, ns1/mp1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/mp1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/mp1",
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/mp1m1",
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/mp1m2",
				},
				// Machine should have infra machine and bootstrap (echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cp1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cp1",
				},
				// MachinePool Machine should only have infra machine
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/mp1m1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/mp1m1",
				},
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/mp1m2": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/mp1m2",
				},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, should NOT be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeFalse())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine pool should NOT be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachinePool, ns1/mp1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeFalse())
				},
			},
		},
		{
			name: "Discovery with grouping and no-echo disabled",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping: false,
					Echo:     true,
				},
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp").
							WithMachines(
								test.NewFakeMachine("cp1"),
							),
					).
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md1").
							WithMachineSets(
								test.NewFakeMachineSet("ms1").
									WithMachines(
										test.NewFakeMachine("m1"),
									),
							),
					).
					Objs(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, and WorkerNodes
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
				},
				// Machine should have infra machine and bootstrap (echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cp1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cp1",
				},
				// Workers should have a machine deployment
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
				},
				// Machine deployment should have a group of machines
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
				},
				// Machine should have infra machine and bootstrap (echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
				},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, should NOT be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeFalse())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine deployment should NOT be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeFalse())
				},
				// infra machines and boostrap should have meta names
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cp1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructure"))
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cp1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("BootstrapConfig"))
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructure"))
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("BootstrapConfig"))
				},
			},
		},
		{
			name: "Discovery with cluster resource sets shown",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping:                true,
					ShowClusterResourceSets: true,
				},
				objs: clusterObjectsWithResourceSet(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, WorkerGroup, and ClusterResourceSetGroup
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetGroup, ns1/ClusterResourceSets",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
				},
				// Machine should be leaf (no echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {},
				// Workers should have a machine deployment
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
				},
				// Machine deployment should have a group of machines (grouping)
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					"virtual.cluster.x-k8s.io/v1beta1, Kind=MachineGroup, ns1/zzz_",
				},
				// ClusterResourceSetGroup should have a ClusterResourceSet
				"virtual.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetGroup, ns1/ClusterResourceSets": {
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
				},
				// ClusterResourceSet should be a leaf
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1": {},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// ClusterResourceSetGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetGroup, ns1/ClusterResourceSets": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine deployment should be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
			},
		},
		{
			name: "Discovery with templates shown with template virtual nodes",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping:               true,
					ShowTemplates:          true,
					AddTemplateVirtualNode: true,
				},
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp").
							WithMachines(
								test.NewFakeMachine("cp1"),
							),
					).
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md1").
							WithMachineSets(
								test.NewFakeMachineSet("ms1").
									WithMachines(
										test.NewFakeMachine("m1"),
										test.NewFakeMachine("m2"),
									),
							).
							WithInfrastructureTemplate(
								test.NewFakeInfrastructureTemplate("md1"),
							),
					).
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md2").
							WithStaticBootstrapConfig().
							WithMachineSets(
								test.NewFakeMachineSet("ms2").
									WithMachines(
										test.NewFakeMachine("m3"),
										test.NewFakeMachine("m4"),
									),
							),
					).
					Objs(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, WorkerGroup, and ClusterResourceSetGroup
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine and template group
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/cp",
				},
				// Machine should be leaf (no echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {},
				// Workers should have a machine deployment
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md2",
				},
				// Machine deployment should have a group of machines (grouping) and templates group
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					"virtual.cluster.x-k8s.io/v1beta1, Kind=MachineGroup, ns1/zzz_",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md1",
				},
				// MachineDeployment TemplateGroup should have a BootstrapConfigRef and InfrastructureRef
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1",
				},
				// MachineDeployment InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1": {},
				// MachineDeployment BootstrapConfigRef should be a leaf
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1": {},
				// Machine deployment should have a group of machines (grouping) and templates group
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md2": {
					"virtual.cluster.x-k8s.io/v1beta1, Kind=MachineGroup, ns1/zzz_",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md2",
				},
				// MachineDeployment TemplateGroup using static bootstrap will only have InfrastructureRef
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md2": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md2",
				},
				// MachineDeployment InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md2": {},
				// ControlPlane TemplateGroup should have a InfrastructureRef
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/cp": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp",
				},
				// ControlPlane InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp": {},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine deployment should be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// ControlPlane TemplateGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// MachineDeployment TemplateGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// MachineDeployment InfrastructureRef should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructureTemplate"))
				},
				// MachineDeployment BootstrapConfigRef should have a meta name
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("BootstrapConfigTemplate"))
				},
				// ControlPlane InfrastructureRef should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructureTemplate"))
				},
			},
		},
		{
			name: "Discovery with templates shown without template virtual nodes",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping:               true,
					ShowTemplates:          true,
					AddTemplateVirtualNode: false,
				},
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp").
							WithMachines(
								test.NewFakeMachine("cp1"),
							),
					).
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md1").
							WithMachineSets(
								test.NewFakeMachineSet("ms1").
									WithMachines(
										test.NewFakeMachine("m1"),
										test.NewFakeMachine("m2"),
									),
							).
							WithInfrastructureTemplate(
								test.NewFakeInfrastructureTemplate("md1"),
							),
					).
					Objs(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, WorkerGroup, and ClusterResourceSetGroup
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine and template
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp",
				},
				// Machine should be leaf (no echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {},
				// Workers should have a machine deployment
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
				},
				// Machine deployment should have a group of machines (grouping) and templates
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					"virtual.cluster.x-k8s.io/v1beta1, Kind=MachineGroup, ns1/zzz_",
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1",
				},
				// MachineDeployment InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1": {},
				// MachineDeployment BootstrapConfigRef should be a leaf
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1": {},
				// ControlPlane InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp": {},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine deployment should be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// ControlPlane TemplateGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// MachineDeployment TemplateGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// MachineDeployment InfrastructureRef should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructureTemplate"))
				},
				// MachineDeployment BootstrapConfigRef should have a meta name
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("BootstrapConfigTemplate"))
				},
				// ControlPlane InfrastructureRef should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructureTemplate"))
				},
			},
		},
		{
			name: "Discovery with multiple machine deployments does not cause template virtual nodes to collide",
			args: args{
				discoverOptions: DiscoverOptions{
					Grouping:               true,
					ShowTemplates:          true,
					AddTemplateVirtualNode: true,
				},
				objs: test.NewFakeCluster("ns1", "cluster1").
					WithControlPlane(
						test.NewFakeControlPlane("cp").
							WithMachines(
								test.NewFakeMachine("cp1"),
							),
					).
					WithMachineDeployments(
						test.NewFakeMachineDeployment("md1").
							WithMachineSets(
								test.NewFakeMachineSet("ms1").
									WithMachines(
										test.NewFakeMachine("m1"),
										test.NewFakeMachine("m2"),
									),
							).
							WithInfrastructureTemplate(
								test.NewFakeInfrastructureTemplate("md1"),
							),
						test.NewFakeMachineDeployment("md2").
							WithMachineSets(
								test.NewFakeMachineSet("ms2").
									WithMachines(
										test.NewFakeMachine("m3"),
										test.NewFakeMachine("m4"),
										test.NewFakeMachine("m5"),
									),
							).
							WithInfrastructureTemplate(
								test.NewFakeInfrastructureTemplate("md2"),
							),
					).
					Objs(),
			},
			wantTree: map[string][]string{
				// Cluster should be parent of InfrastructureCluster, ControlPlane, WorkerGroup, and ClusterResourceSetGroup
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers",
				},
				// InfrastructureCluster should be leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {},
				// ControlPlane should have a machine and template group
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": {
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/cp",
				},
				// ControlPlane TemplateGroup should have a InfrastructureRef
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/cp": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp",
				},
				// ControlPlane InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp": {},
				// Machine should be leaf (no echo)
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cp1": {},
				// Workers should have 2 machine deployments
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": {
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
					"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md2",
				},
				// Machine deployment 1 should have a group of machines (grouping) and templates group
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					"virtual.cluster.x-k8s.io/v1beta1, Kind=MachineGroup, ns1/zzz_",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md1",
				},
				// MachineDeployment 1 TemplateGroup should have a BootstrapConfigRef and InfrastructureRef
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1",
				},
				// MachineDeployment 1 InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1": {},
				// MachineDeployment 1 BootstrapConfigRef should be a leaf
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1": {},
				// Machine deployment 2 should have a group of machines (grouping) and templates group
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md2": {
					"virtual.cluster.x-k8s.io/v1beta1, Kind=MachineGroup, ns1/zzz_",
					"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md2",
				},
				// MachineDeployment 2 TemplateGroup should have a BootstrapConfigRef and InfrastructureRef
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md2": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md2",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md2",
				},
				// MachineDeployment 2 InfrastructureRef should be a leaf
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md2": {},
				// MachineDeployment 2 BootstrapConfigRef should be a leaf
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md2": {},
			},
			wantNodeCheck: map[string]nodeCheck{
				// InfrastructureCluster should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ClusterInfrastructure"))
				},
				// ControlPlane should have a meta name, be a grouping object
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("ControlPlane"))
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// Workers should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=WorkerGroup, ns1/Workers": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// Machine deployment should be a grouping object
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsGroupingObject(obj)).To(BeTrue())
				},
				// ControlPlane TemplateGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/cp": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// ControlPlane InfrastructureRef should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructureTemplate"))
				},
				// MachineDeployment 1 TemplateGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// MachineDeployment 1 InfrastructureRef should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructureTemplate"))
				},
				// MachineDeployment 1 BootstrapConfigRef should have a meta name
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("BootstrapConfigTemplate"))
				},
				// MachineDeployment 2 TemplateGroup should be a virtual node
				"virtual.cluster.x-k8s.io/v1beta1, Kind=TemplateGroup, ns1/md2": func(g *WithT, obj client.Object) {
					g.Expect(IsVirtualObject(obj)).To(BeTrue())
				},
				// MachineDeployment 2 InfrastructureRef should have a meta name
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md2": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("MachineInfrastructureTemplate"))
				},
				// MachineDeployment 2 BootstrapConfigRef should have a meta name
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md2": func(g *WithT, obj client.Object) {
					g.Expect(GetMetaName(obj)).To(Equal("BootstrapConfigTemplate"))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			client, err := test.NewFakeProxy().WithObjs(tt.args.objs...).NewClient(context.Background())
			g.Expect(client).ToNot(BeNil())
			g.Expect(err).ToNot(HaveOccurred())

			tree, err := Discovery(context.TODO(), client, "ns1", "cluster1", tt.args.discoverOptions)
			g.Expect(tree).ToNot(BeNil())
			g.Expect(err).ToNot(HaveOccurred())

			for parent, wantChildren := range tt.wantTree {
				gotChildren := tree.GetObjectsByParent(types.UID(parent))
				g.Expect(gotChildren).To(HaveLen(len(wantChildren)), "%q doesn't have the expected number of children nodes", parent)

				for _, gotChild := range gotChildren {
					found := false
					for _, wantChild := range wantChildren {
						if strings.HasPrefix(string(gotChild.GetUID()), wantChild) {
							found = true
							break
						}
					}
					g.Expect(found).To(BeTrue(), "got child %q for parent %q, expecting [%s]", gotChild.GetUID(), parent, strings.Join(wantChildren, "] ["))

					if test, ok := tt.wantNodeCheck[string(gotChild.GetUID())]; ok {
						test(g, gotChild)
					}
				}
			}
		})
	}
}
