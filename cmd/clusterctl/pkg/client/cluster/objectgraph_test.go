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
	"fmt"
	"reflect"
	"sort"
	"testing"

	logrtesting "github.com/go-logr/logr/testing"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/pkg/internal/test"
)

func TestObjectGraph_getDiscoveryTypeMetaList(t *testing.T) {
	type fields struct {
		proxy Proxy
	}
	tests := []struct {
		name    string
		fields  fields
		want    []metav1.TypeMeta
		wantErr bool
	}{
		{
			name: "Return CRDs + ConfigMap & Secrets",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithObjs(
						test.FakeCustomResourceDefinition("foo", "Bar", "v2", "v1"), // NB. foo/v1 Bar is not a storage version, so it should be ignored
						test.FakeCustomResourceDefinition("foo", "Baz", "v1"),
					),
			},
			want: []metav1.TypeMeta{
				{APIVersion: "foo/v2", Kind: "Bar"},
				{APIVersion: "foo/v1", Kind: "Baz"},
				{APIVersion: "v1", Kind: "Secret"},
				{APIVersion: "v1", Kind: "ConfigMap"},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := newObjectGraph(tt.fields.proxy, nil)
			got, err := graph.getDiscoveryTypes()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// sort lists in a predictable way
			sort.Slice(got, sortTypeMetaList(got))
			sort.Slice(tt.want, sortTypeMetaList(tt.want))

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got = %v, wantGraph %v", got, tt.want)
			}
		})
	}
}

func sortTypeMetaList(list []metav1.TypeMeta) func(i int, j int) bool {
	return func(i, j int) bool {
		return list[i].GroupVersionKind().String() < list[j].GroupVersionKind().String()
	}
}

type wantGraphItem struct {
	virtual        bool
	dependents     []string
	softDependents []string
}

type wantGraph struct {
	nodes map[string]wantGraphItem
}

func assertGraph(t *testing.T, got *objectGraph, want wantGraph) {
	if len(got.uidToNode) != len(want.nodes) {
		t.Fatalf("got = %d nodes, want %d nodes", len(got.uidToNode), len(want.nodes))
	}

	for uid, wantNode := range want.nodes {
		gotNode, ok := got.uidToNode[types.UID(uid)]
		if !ok {
			t.Fatalf("failed to get node with uid = %s", uid)
		}

		if gotNode.virtual != wantNode.virtual {
			t.Errorf("node with uid = %s, got virtual = %t, want %t", uid, gotNode.virtual, wantNode.virtual)
		}

		if len(gotNode.dependents) != len(wantNode.dependents) {
			t.Fatalf("node with uid = %s, got dependents = %d, want %d", uid, len(gotNode.dependents), len(wantNode.dependents))
		}

		for _, wantDependants := range wantNode.dependents {
			found := false
			for k := range gotNode.dependents {
				if k.identity.UID == types.UID(wantDependants) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("node with uid = %s, failed to get dependents %s", uid, wantDependants)
			}
		}

		if len(gotNode.softDependents) != len(wantNode.softDependents) {
			t.Fatalf("node with uid = %s, got softDependents = %d, want %d", uid, len(gotNode.softDependents), len(wantNode.softDependents))
		}

		for _, wantDependants := range wantNode.softDependents {
			found := false
			for k := range gotNode.softDependents {
				if k.identity.UID == types.UID(wantDependants) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("node with uid = %s, failed to get dependents %s", uid, wantDependants)
			}
		}
	}
}

func TestObjectGraph_addObj(t *testing.T) {
	type args struct {
		objs []*unstructured.Unstructured
	}

	tests := []struct {
		name string
		args args
		want wantGraph
	}{
		{
			name: "Add a single object",
			args: args{
				objs: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "a/v1",
							"kind":       "A",
							"metadata": map[string]interface{}{
								"namespace": "ns",
								"name":      "foo",
								"uid":       "1",
							},
						},
					},
				},
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"1": { // the object: not virtual (observed), without dependents
						virtual:    false,
						dependents: nil,
					},
				},
			},
		},
		{
			name: "Add a single object with an owner ref",
			args: args{
				objs: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "a/v1",
							"kind":       "A",
							"metadata": map[string]interface{}{
								"namespace": "ns",
								"name":      "foo",
								"uid":       "1",
								"ownerReferences": []interface{}{
									map[string]interface{}{
										"apiVersion": "b/v1",
										"kind":       "B",
										"name":       "bar",
										"uid":        "2",
									},
								},
							},
						},
					},
				},
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"1": { // the object: not virtual (observed), without dependents
						virtual:    false,
						dependents: nil,
					},
					"2": { // the object owner: virtual (not yet observed), with 1 dependents
						virtual:    true,
						dependents: []string{"1"},
					},
				},
			},
		},
		{
			name: "Add an object with an owner ref and its owner",
			args: args{
				objs: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "a/v1",
							"kind":       "A",
							"metadata": map[string]interface{}{
								"namespace": "ns",
								"name":      "foo",
								"uid":       "1",
								"ownerReferences": []interface{}{
									map[string]interface{}{
										"apiVersion": "b/v1",
										"kind":       "B",
										"name":       "bar",
										"uid":        "2",
									},
								},
							},
						},
					},
					{
						Object: map[string]interface{}{
							"apiVersion": "b/v1",
							"kind":       "B",
							"metadata": map[string]interface{}{
								"namespace": "ns",
								"name":      "bar",
								"uid":       "2",
							},
						},
					},
				},
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"1": { // the object: not virtual (observed), without dependents
						virtual:    false,
						dependents: nil,
					},
					"2": { // the object owner: not virtual (observed), with 1 dependents
						virtual:    false,
						dependents: []string{"1"},
					},
				},
			},
		},
		{
			name: "Add an object with an owner ref and its owner (reverse discovery order)",
			args: args{
				objs: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "b/v1",
							"kind":       "B",
							"metadata": map[string]interface{}{
								"namespace": "ns",
								"name":      "bar",
								"uid":       "2",
							},
						},
					},
					{
						Object: map[string]interface{}{
							"apiVersion": "a/v1",
							"kind":       "A",
							"metadata": map[string]interface{}{
								"namespace": "ns",
								"name":      "foo",
								"uid":       "1",
								"ownerReferences": []interface{}{
									map[string]interface{}{
										"apiVersion": "b/v1",
										"kind":       "B",
										"name":       "bar",
										"uid":        "2",
									},
								},
							},
						},
					},
				},
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"1": { // the object: not virtual, without dependents
						virtual:    false,
						dependents: nil,
					},
					"2": { // the object owner: not virtual (observed), with 1 dependents
						virtual:    false,
						dependents: []string{"1"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := newObjectGraph(nil, nil)
			for _, o := range tt.args.objs {
				graph.addObj(o)
			}

			assertGraph(t, graph, tt.want)
		})
	}
}

type objectGraphTestArgs struct {
	objs []runtime.Object
}

var objectGraphsTests = []struct {
	name    string
	args    objectGraphTestArgs
	want    wantGraph
	wantErr bool
}{
	{
		name: "Cluster",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
						"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster1-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
				"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},
			},
		},
	},
	{
		name: "Two clusters",
		args: objectGraphTestArgs{
			objs: func() []runtime.Object {
				objs := []runtime.Object{}
				objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "cluster2").Objs()...)
				return objs
			}(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
						"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster1-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
				"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster2": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster2",
						"/v1, Kind=Secret, ns1/cluster2-kubeconfig",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster2-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster2": {},
				"/v1, Kind=Secret, ns1/cluster2-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
				"/v1, Kind=Secret, ns1/cluster2-kubeconfig": {},
			},
		},
	},
	{
		name: "Cluster with machine",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachines(
					test.NewFakeMachine("m1"),
				).Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
						"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
						"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster1-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
				"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},

				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
						"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1": {},
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1": {
					dependents: []string{
						"/v1, Kind=Secret, ns1/m1",
						"/v1, Kind=Secret, ns1/cluster1-sa",
					},
				},
				"/v1, Kind=Secret, ns1/m1":          {},
				"/v1, Kind=Secret, ns1/cluster1-sa": {},
			},
		},
	},
	{
		name: "Cluster with MachineSet",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachineSets(
					test.NewFakeMachineSet("ms1").
						WithMachines(test.NewFakeMachine("m1")),
				).Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
						"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
						"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/ms1",
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/ms1",
						"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/ms1",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster1-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
				"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},

				"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/ms1": {
					dependents: []string{
						"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/ms1": {},
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/ms1":            {},

				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
						"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1": {},
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1": {
					dependents: []string{
						"/v1, Kind=Secret, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/m1": {},
			},
		},
	},
	{
		name: "Cluster with MachineDeployment",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachineDeployments(
					test.NewFakeMachineDeployment("md1").
						WithMachineSets(
							test.NewFakeMachineSet("ms1").
								WithMachines(
									test.NewFakeMachine("m1"),
								),
						),
				).Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
						"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
						"cluster.x-k8s.io/v1alpha3, Kind=MachineDeployment, ns1/md1",
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/md1",
						"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/md1",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster1-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
				"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},

				"cluster.x-k8s.io/v1alpha3, Kind=MachineDeployment, ns1/md1": {
					dependents: []string{
						"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/ms1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/md1": {},
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/md1":            {},

				"cluster.x-k8s.io/v1alpha3, Kind=MachineSet, ns1/ms1": {
					dependents: []string{
						"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
					},
				},

				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
						"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1": {},
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1": {
					dependents: []string{
						"/v1, Kind=Secret, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/m1": {},
			},
		},
	},
	{
		name: "Cluster with Control Plane",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithControlPlane(
					test.NewFakeControlPlane("cp1").
						WithMachines(
							test.NewFakeMachine("m1"),
						),
				).Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
						"controlplane.cluster.x-k8s.io/v1alpha3, Kind=DummyControlPlane, ns1/cp1",
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/cp1",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster1-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
				"/v1, Kind=Secret, ns1/cluster1-ca": {}, //NB. this secret is not linked to the cluster through owner ref

				"controlplane.cluster.x-k8s.io/v1alpha3, Kind=DummyControlPlane, ns1/cp1": {
					dependents: []string{
						"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1",
						"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
						"/v1, Kind=Secret, ns1/cluster1-sa",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/cp1": {},
				"/v1, Kind=Secret, ns1/cluster1-sa":         {},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},

				"cluster.x-k8s.io/v1alpha3, Kind=Machine, ns1/m1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1",
						"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachine, ns1/m1": {},
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfig, ns1/m1": {
					dependents: []string{
						"/v1, Kind=Secret, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/m1": {},
			},
		},
	},
	{
		name: "Cluster with MachinePool",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachinePools(
					test.NewFakeMachinePool("mp1"),
				).Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
					dependents: []string{
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
						"cluster.x-k8s.io/v1alpha3, Kind=MachinePool, ns1/mp1",
						"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
						"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/mp1",
						"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/mp1",
					},
					softDependents: []string{
						"/v1, Kind=Secret, ns1/cluster1-ca",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
				"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},

				"cluster.x-k8s.io/v1alpha3, Kind=MachinePool, ns1/mp1":                                       {},
				"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureMachineTemplate, ns1/mp1": {},
				"bootstrap.cluster.x-k8s.io/v1alpha3, Kind=DummyBootstrapConfigTemplate, ns1/mp1":            {},
			},
		},
	},
}

func getDetachedObjectGraphWihObjs(objs []runtime.Object) (*objectGraph, error) {
	graph := newObjectGraph(nil, nil) // detached from any cluster
	for _, o := range objs {
		u := &unstructured.Unstructured{}
		if err := test.FakeScheme.Convert(o, u, nil); err != nil { //nolint
			return nil, errors.Wrap(err, "failed to convert object in unstructured")
		}
		graph.addObj(u)
	}
	return graph, nil
}

func TestObjectGraph_addObj_WithFakeObjects(t *testing.T) {
	// NB. we are testing the graph is properly built starting from objects (this test) or from the same objects read from the cluster (TestGraphBuilder_Discovery)
	for _, tt := range objectGraphsTests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := getDetachedObjectGraphWihObjs(tt.args.objs)
			if err != nil {
				t.Fatal(err)
			}

			// call setSoftDependents so there is functional parity with discovery
			graph.setSoftDependents()

			assertGraph(t, graph, tt.want)
		})
	}
}

func getObjectGraphWithObjs(objs []runtime.Object) *objectGraph {
	fromProxy := getFakeProxyWithCRDs()

	for _, o := range objs {
		fromProxy.WithObjs(o)
	}

	return newObjectGraph(fromProxy, logrtesting.NullLogger{})
}

func getFakeProxyWithCRDs() *test.FakeProxy {
	proxy := test.NewFakeProxy()
	for _, o := range test.FakeCRDList() {
		proxy.WithObjs(o)
	}
	return proxy
}

func getFakeDiscoveryTypes(graph *objectGraph) ([]metav1.TypeMeta, error) {
	discoveryTypes, err := graph.getDiscoveryTypes()
	if err != nil {
		return nil, err
	}

	// Given that the Fake client behaves in a different way than real client, for this test we are required to add the List suffix to all the types.
	for i := range discoveryTypes {
		discoveryTypes[i].Kind = fmt.Sprintf("%sList", discoveryTypes[i].Kind)
	}
	return discoveryTypes, nil
}

func TestObjectGraph_Discovery(t *testing.T) {
	// NB. we are testing the graph is properly built starting from objects (TestGraphBuilder_addObj_WithFakeObjects) or from the same objects read from the cluster (this test).
	for _, tt := range objectGraphsTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound to a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.args.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			if err != nil {
				t.Fatal(err)
			}

			// finally test discovery
			err = graph.Discovery("ns1", discoveryTypes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			assertGraph(t, graph, tt.want)
		})
	}
}

func TestObjectGraph_DiscoveryByNamespace(t *testing.T) {
	type args struct {
		namespace string
		objs      []runtime.Object
	}
	var tests = []struct {
		name    string
		args    args
		want    wantGraph
		wantErr bool
	}{
		{
			name: "two clusters, in different namespaces, read both",
			args: args{
				namespace: "", // read all the namespaces
				objs: func() []runtime.Object {
					objs := []runtime.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
					objs = append(objs, test.NewFakeCluster("ns2", "cluster1").Objs()...)
					return objs
				}(),
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
						dependents: []string{
							"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
							"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
						},
						softDependents: []string{
							"/v1, Kind=Secret, ns1/cluster1-ca",
						},
					},
					"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
					"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
					"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},
					"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns2/cluster1": {
						dependents: []string{
							"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns2/cluster1",
							"/v1, Kind=Secret, ns2/cluster1-kubeconfig",
						},
						softDependents: []string{
							"/v1, Kind=Secret, ns2/cluster1-ca",
						},
					},
					"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns2/cluster1": {},
					"/v1, Kind=Secret, ns2/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
					"/v1, Kind=Secret, ns2/cluster1-kubeconfig": {},
				},
			},
		},
		{
			name: "two clusters, in different namespaces, read only 1",
			args: args{
				namespace: "ns1", // read only from ns1
				objs: func() []runtime.Object {
					objs := []runtime.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
					objs = append(objs, test.NewFakeCluster("ns2", "cluster1").Objs()...)
					return objs
				}(),
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/cluster1": {
						dependents: []string{
							"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1",
							"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
						},
						softDependents: []string{
							"/v1, Kind=Secret, ns1/cluster1-ca",
						},
					},
					"infrastructure.cluster.x-k8s.io/v1alpha3, Kind=DummyInfrastructureCluster, ns1/cluster1": {},
					"/v1, Kind=Secret, ns1/cluster1-ca":         {}, //NB. this secret is not linked to the cluster through owner ref
					"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound to a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.args.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			if err != nil {
				t.Fatal(err)
			}

			// finally test discovery
			err = graph.Discovery(tt.args.namespace, discoveryTypes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			assertGraph(t, graph, tt.want)
		})
	}
}

func Test_objectGraph_setSoftDependents(t *testing.T) {
	type fields struct {
		objs []runtime.Object
	}
	tests := []struct {
		name         string
		fields       fields
		wantClusters map[string][]string
	}{
		{
			name: "A cluster with a soft dependent secret",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "foo").Objs(),
			},
			wantClusters: map[string][]string{ // wantClusters is a map[Cluster.UID] --> list of UIDs
				"cluster.x-k8s.io/v1alpha3, Kind=Cluster, ns1/foo": {
					"/v1, Kind=Secret, ns1/foo-ca", // the ca secret has no explicit OwnerRef to the cluster, so should be identified as a soft dependent
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := getDetachedObjectGraphWihObjs(tt.fields.objs)
			if err != nil {
				t.Fatal(err)
			}

			graph.setSoftDependents()

			gotClusters := graph.getClusters()
			if len(gotClusters) != len(tt.wantClusters) {
				t.Fatalf("got = %d clusters, want %d", len(gotClusters), len(tt.wantClusters))
			}

			for _, cluster := range gotClusters {
				wantObjects, ok := tt.wantClusters[string(cluster.identity.UID)]
				if !ok {
					t.Fatalf("got = %s, not included in the expected cluster list", cluster.identity.UID)
				}

				gotObjects := []string{}
				for softDependent := range cluster.softDependents {
					gotObjects = append(gotObjects, string(softDependent.identity.UID))
				}

				sort.Strings(wantObjects)
				sort.Strings(gotObjects)

				if !reflect.DeepEqual(gotObjects, wantObjects) {
					t.Fatalf("cluster %s, got = %s, expected = %s", cluster.identity.UID, gotObjects, wantObjects)
				}
			}
		})
	}
}
