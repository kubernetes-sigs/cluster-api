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

package cluster

import (
	"reflect"
	"sort"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type moveTestsFields struct {
	objs []runtime.Object
}

var moveTests = []struct {
	name           string
	fields         moveTestsFields
	wantMoveGroups [][]string
	wantErr        bool
}{
	{
		name: "Cluster",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").Objs(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/foo",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with machine",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachines(
					test.NewFakeMachine("m1"),
					test.NewFakeMachine("m2"),
				).Objs(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
			},
			{ //group 3 (objects with ownerReferences in group 1,2)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m2",
			},
			{ //group 4 (objects with ownerReferences in group 1,2,3)
				// owned by DummyBootstrapConfigs
				"/v1, Kind=Secret, ns1/cluster1-sa",
				"/v1, Kind=Secret, ns1/m1",
				"/v1, Kind=Secret, ns1/m2",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with MachineSet",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachineSets(
					test.NewFakeMachineSet("ms1").
						WithMachines(
							test.NewFakeMachine("m1"),
							test.NewFakeMachine("m2"),
						),
				).Objs(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/ms1",
				"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/ms1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/ms1",
			},
			{ //group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineSets
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m2",
			},
			{ //group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m2",
			},
			{ //group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by DummyBootstrapConfigs
				"/v1, Kind=Secret, ns1/m1",
				"/v1, Kind=Secret, ns1/m2",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with MachineDeployment",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachineDeployments(
					test.NewFakeMachineDeployment("md1").
						WithMachineSets(
							test.NewFakeMachineSet("ms1").
								WithMachines(
									test.NewFakeMachine("m1"),
									test.NewFakeMachine("m2"),
								),
						),
				).Objs(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/md1",
				"cluster.x-k8s.io/v1alpha3, Kind=MachineDeployment, ns1/md1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/md1",
			},
			{ //group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineDeployments
				"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/ms1",
			},
			{ //group 4 (objects with ownerReferences in group 1,2,3)
				// owned by MachineSets
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m2",
			},
			{ //group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m2",
			},
			{ //group 6 (objects with ownerReferences in group 1,2,3,5,6)
				// owned by DummyBootstrapConfigs
				"/v1, Kind=Secret, ns1/m1",
				"/v1, Kind=Secret, ns1/m2",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with Control Plane",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithControlPlane(
					test.NewFakeControlPlane("cp1").
						WithMachines(
							test.NewFakeMachine("m1"),
							test.NewFakeMachine("m2"),
						),
				).Objs(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"controlplane.cluster.x-k8s.io/v1alpha3, Kind=DummyControlPlane, ns1/cp1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/cp1",
			},
			{ //group 3 (objects with ownerReferences in group 1,2)
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"/v1, Kind=Secret, ns1/cluster1-sa",
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m2",
			},
			{ //group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m2",
			},
			{ //group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by DummyBootstrapConfigs
				"/v1, Kind=Secret, ns1/m1",
				"/v1, Kind=Secret, ns1/m2",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with MachinePool",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachinePools(
					test.NewFakeMachinePool("mp1"),
				).Objs(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/mp1",
				"cluster.x-k8s.io/v1alpha3, Kind=MachinePool, ns1/mp1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/mp1",
			},
		},
		wantErr: false,
	},
	{
		name: "Two clusters",
		fields: moveTestsFields{
			objs: func() []runtime.Object {
				objs := []runtime.Object{}
				objs = append(objs, test.NewFakeCluster("ns1", "foo").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "bar").Objs()...)
				return objs
			}(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/foo",
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/bar",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/foo",
				"/v1, Kind=Secret, ns1/bar-ca",
				"/v1, Kind=Secret, ns1/bar-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/bar",
			},
		},
	},
	{
		name: "Two clusters with a shared object",
		fields: moveTestsFields{
			objs: func() []runtime.Object {
				sharedInfrastructureTemplate := test.NewFakeInfrastructureTemplate("shared")

				objs := []runtime.Object{
					sharedInfrastructureTemplate,
				}

				objs = append(objs, test.NewFakeCluster("ns1", "cluster1").
					WithMachineSets(
						test.NewFakeMachineSet("cluster1-ms1").
							WithInfrastructureTemplate(sharedInfrastructureTemplate).
							WithMachines(
								test.NewFakeMachine("cluster1-m1"),
							),
					).Objs()...)

				objs = append(objs, test.NewFakeCluster("ns1", "cluster2").
					WithMachineSets(
						test.NewFakeMachineSet("cluster2-ms1").
							WithInfrastructureTemplate(sharedInfrastructureTemplate).
							WithMachines(
								test.NewFakeMachine("cluster2-m1"),
							),
					).Objs()...)

				return objs
			}(),
		},
		wantMoveGroups: [][]string{
			{ //group 1
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1",
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster2",
			},
			{ //group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/cluster1-ms1",
				"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/cluster1-ms1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
				"/v1, Kind=Secret, ns1/cluster2-ca",
				"/v1, Kind=Secret, ns1/cluster2-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/cluster2-ms1",
				"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/cluster2-ms1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster2",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/shared", //shared object
			},
			{ //group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineSets
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/cluster1-m1",
				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/cluster2-m1",
			},
			{ //group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/cluster1-m1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/cluster1-m1",
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/cluster2-m1",
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/cluster2-m1",
			},
			{ //group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by DummyBootstrapConfigs
				"/v1, Kind=Secret, ns1/cluster1-m1",
				"/v1, Kind=Secret, ns1/cluster2-m1",
			},
		},
	},
}

func Test_getMoveSequence(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			if err != nil {
				t.Fatal(err)
			}

			// trigger discovery the content of the source cluster
			if err := graph.Discovery("ns1", discoveryTypes); err != nil {
				t.Fatal(err)
			}

			moveSequence := getMoveSequence(graph)
			if len(moveSequence.groups) != len(tt.wantMoveGroups) {
				t.Fatalf("got = %d groups, want %d", len(moveSequence.groups), len(tt.wantMoveGroups))
			}
			for i, gotGroup := range moveSequence.groups {
				wantGroup := tt.wantMoveGroups[i]
				gotNodes := []string{}
				for _, node := range gotGroup {
					gotNodes = append(gotNodes, string(node.identity.UID))
				}

				sort.Strings(gotNodes)
				sort.Strings(wantGroup)

				if !reflect.DeepEqual(gotNodes, wantGroup) {
					t.Errorf("group[%d], got = %s, expected = %s", i, gotNodes, wantGroup)
				}
			}
		})
	}
}

func Test_objectMover_move(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			if err != nil {
				t.Fatal(err)
			}

			// trigger discovery the content of the source cluster
			if err := graph.Discovery("ns1", discoveryTypes); err != nil {
				t.Fatal(err)
			}

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move
			mover := objectMover{
				fromProxy: graph.proxy,
				log:       graph.log,
			}

			err = mover.move(graph, toProxy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// check that the objects are removed from the source cluster and are created in the target cluster
			csFrom, err := graph.proxy.NewClient()
			if err != nil {
				t.Fatal(err)
			}
			csTo, err := toProxy.NewClient()
			if err != nil {
				t.Fatal(err)
			}

			for _, node := range graph.uidToNode {
				key := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are deleted from the source cluster
				oFrom := &unstructured.Unstructured{}
				oFrom.SetAPIVersion(node.identity.APIVersion)
				oFrom.SetKind(node.identity.Kind)

				err := csFrom.Get(ctx, key, oFrom)
				if err == nil {
					t.Errorf("%v not deleted in source cluster", key)
					continue
				}
				if !apierrors.IsNotFound(err) {
					t.Errorf("error = %v when checking for %v deleted in source cluster", err, key)
					continue
				}

				// objects are created in the target cluster
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("error = %v when checking for %v created in target cluster", err, key)
					continue
				}
			}
		})
	}
}
