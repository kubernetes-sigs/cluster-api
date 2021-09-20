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
	"sort"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestObjectGraph_getDiscoveryTypeMetaList(t *testing.T) {
	type fields struct {
		proxy Proxy
	}
	tests := []struct {
		name    string
		fields  fields
		want    map[string]*discoveryTypeInfo
		wantErr bool
	}{
		{
			name: "Return CRDs + ConfigMap & Secrets",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithObjs(
						test.FakeNamespacedCustomResourceDefinition("foo", "Bar", "v2", "v1"), // NB. foo/v1 Bar is not a storage version, so it should be ignored
						test.FakeNamespacedCustomResourceDefinition("foo", "Baz", "v1"),
					),
			},
			want: map[string]*discoveryTypeInfo{
				"bars.foo": {
					typeMeta:           metav1.TypeMeta{Kind: "Bar", APIVersion: "foo/v2"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "Namespaced",
				},
				"bazs.foo": {
					typeMeta:           metav1.TypeMeta{Kind: "Baz", APIVersion: "foo/v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "Namespaced",
				},
				"secrets.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
				"configmaps.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
			},
			wantErr: false,
		},
		{
			name: "Enforce force move for Cluster and ClusterResourceSet",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithObjs(
						test.FakeNamespacedCustomResourceDefinition("cluster.x-k8s.io", "Cluster", "v1beta1"),
						test.FakeNamespacedCustomResourceDefinition("addons.cluster.x-k8s.io", "ClusterResourceSet", "v1beta1"),
					),
			},
			want: map[string]*discoveryTypeInfo{
				"clusters.cluster.x-k8s.io": {
					typeMeta:           metav1.TypeMeta{Kind: "Cluster", APIVersion: "cluster.x-k8s.io/v1beta1"},
					forceMove:          true,
					forceMoveHierarchy: true,
					scope:              "Namespaced",
				},
				"clusterresourcesets.addons.cluster.x-k8s.io": {
					typeMeta:           metav1.TypeMeta{Kind: "ClusterResourceSet", APIVersion: "addons.cluster.x-k8s.io/v1beta1"},
					forceMove:          true,
					forceMoveHierarchy: true,
					scope:              "Namespaced",
				},
				"secrets.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
				"configmaps.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
			},
			wantErr: false,
		},
		{
			name: "Identified Cluster scoped types",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithObjs(
						test.FakeClusterCustomResourceDefinition("infrastructure.cluster.x-k8s.io", "GenericClusterInfrastructureIdentity", "v1beta1"),
					),
			},
			want: map[string]*discoveryTypeInfo{
				"genericclusterinfrastructureidentitys.infrastructure.cluster.x-k8s.io": {
					typeMeta:           metav1.TypeMeta{Kind: "GenericClusterInfrastructureIdentity", APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "Cluster",
				},
				"secrets.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
				"configmaps.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
			},
			wantErr: false,
		},
		{
			name: "Identified force move label",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithObjs(
						func() client.Object {
							crd := test.FakeNamespacedCustomResourceDefinition("foo", "Bar", "v1")
							crd.Labels[clusterctlv1.ClusterctlMoveLabelName] = ""
							return crd
						}(),
					),
			},
			want: map[string]*discoveryTypeInfo{
				"bars.foo": {
					typeMeta:           metav1.TypeMeta{Kind: "Bar", APIVersion: "foo/v1"},
					forceMove:          true,
					forceMoveHierarchy: false,
					scope:              "Namespaced",
				},
				"secrets.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
				"configmaps.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
			},
			wantErr: false,
		},
		{
			name: "Identified force move hierarchy label",
			fields: fields{
				proxy: test.NewFakeProxy().
					WithObjs(
						func() client.Object {
							crd := test.FakeNamespacedCustomResourceDefinition("foo", "Bar", "v1")
							crd.Labels[clusterctlv1.ClusterctlMoveHierarchyLabelName] = ""
							return crd
						}(),
					),
			},
			want: map[string]*discoveryTypeInfo{
				"bars.foo": {
					typeMeta:           metav1.TypeMeta{Kind: "Bar", APIVersion: "foo/v1"},
					forceMove:          true, // force move is implicit when there is forceMoveHierarchy
					forceMoveHierarchy: true,
					scope:              "Namespaced",
				},
				"secrets.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
				"configmaps.v1": {
					typeMeta:           metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					forceMove:          false,
					forceMoveHierarchy: false,
					scope:              "",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			graph := newObjectGraph(tt.fields.proxy, nil)
			err := graph.getDiscoveryTypes()
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(graph.types).To(Equal(tt.want))
		})
	}
}

type wantGraphItem struct {
	virtual            bool
	isGlobal           bool
	forceMove          bool
	forceMoveHierarchy bool
	owners             []string
	softOwners         []string
}

type wantGraph struct {
	nodes map[string]wantGraphItem
}

func assertGraph(t *testing.T, got *objectGraph, want wantGraph) {
	t.Helper()

	g := NewWithT(t)

	g.Expect(len(got.uidToNode)).To(Equal(len(want.nodes)), "the number of nodes in the objectGraph doesn't match the number of expected nodes")

	for uid, wantNode := range want.nodes {
		gotNode, ok := got.uidToNode[types.UID(uid)]
		g.Expect(ok).To(BeTrue(), "node %q not found", uid)
		g.Expect(gotNode.virtual).To(Equal(wantNode.virtual), "node %q.virtual does not have the expected value", uid)
		g.Expect(gotNode.isGlobal).To(Equal(wantNode.isGlobal), "node %q.isGlobal does not have the expected value", uid)
		g.Expect(gotNode.forceMove).To(Equal(wantNode.forceMove), "node %q.forceMove does not have the expected value", uid)
		g.Expect(gotNode.forceMoveHierarchy).To(Equal(wantNode.forceMoveHierarchy), "node %q.forceMoveHierarchy does not have the expected value", uid)
		g.Expect(gotNode.owners).To(HaveLen(len(wantNode.owners)), "node %q.owner does not have the expected length", uid)

		for _, wantOwner := range wantNode.owners {
			found := false
			for k := range gotNode.owners {
				if k.identity.UID == types.UID(wantOwner) {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(), "node %q.owners does not contain %q", uid, wantOwner)
		}

		g.Expect(gotNode.softOwners).To(HaveLen(len(wantNode.softOwners)), "node %q.softOwners does not have the expected length", uid)

		for _, wantOwner := range wantNode.softOwners {
			found := false
			for k := range gotNode.softOwners {
				if k.identity.UID == types.UID(wantOwner) {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue(), "node %q.softOwners does not contain %q", uid, wantOwner)
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
					"1": { // the object: not virtual (observed), without owner ref
						virtual: false,
						owners:  nil,
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
					"1": { // the object: not virtual (observed), with 1 owner refs
						virtual: false,
						owners:  []string{"2"},
					},
					"2": { // the object owner: virtual (not yet observed), without owner refs
						virtual: true,
						owners:  nil,
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
					"1": { // the object: not virtual (observed), with 1 owner refs
						virtual: false,
						owners:  []string{"2"},
					},
					"2": { // the object owner: not virtual (observed), without owner refs
						virtual: false,
						owners:  nil,
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
					"1": { // the object: not virtual (observed), with 1 owner refs
						virtual: false,
						owners:  []string{"2"},
					},
					"2": { // the object owner: not virtual (observed), without owner refs
						virtual: false,
						owners:  nil,
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
	objs []client.Object
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
			},
		},
	},
	{
		name: "Cluster with cloud config secret with the force move label",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithCloudConfigSecret().Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-cloud-config": {
					forceMove: true,
				},
			},
		},
	},
	{
		name: "Two clusters",
		args: objectGraphTestArgs{
			objs: func() []client.Object {
				objs := []client.Object{}
				objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "cluster2").Objs()...)
				return objs
			}(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster2": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},
				"/v1, Kind=Secret, ns1/cluster2-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster2-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/m1": {
					owners: []string{
						"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-sa": {
					owners: []string{
						"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
					},
				},
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/ms1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/m1": {
					owners: []string{
						"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
					},
				},
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/ms1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/m1": {
					owners: []string{
						"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
					},
				},
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},

				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-sa": {
					owners: []string{
						"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1": {
					owners: []string{
						"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
					},
				},
				"/v1, Kind=Secret, ns1/m1": {
					owners: []string{
						"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
					},
				},
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=MachinePool, ns1/mp1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/mp1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/mp1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
			},
		},
	},
	{
		name: "Two clusters with shared objects",
		args: objectGraphTestArgs{
			objs: func() []client.Object {
				sharedInfrastructureTemplate := test.NewFakeInfrastructureTemplate("shared")

				objs := []client.Object{
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
		want: wantGraph{
			nodes: map[string]wantGraphItem{

				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/shared": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster1-ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/cluster1-ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster1-m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster1-ms1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cluster1-m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster1-m1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster1-m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster1-m1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-m1": {
					owners: []string{
						"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster1-m1",
					},
				},
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster2": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},
				"/v1, Kind=Secret, ns1/cluster2-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster2-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster2-ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/cluster2-ms1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},

				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster2-m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster2-ms1",
					},
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cluster2-m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster2-m1",
					},
				},
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster2-m1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster2-m1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster2-m1": {
					owners: []string{
						"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster2-m1",
					},
				},
			},
		},
	},
	{
		name: "A ClusterResourceSet applied to a cluster",
		args: objectGraphTestArgs{
			objs: func() []client.Object {
				objs := []client.Object{}
				objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)

				objs = append(objs, test.NewFakeClusterResourceSet("ns1", "crs1").
					WithSecret("resource-s1").
					WithConfigMap("resource-c1").
					ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster1")).
					Objs()...)

				return objs
			}(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster1": {
					owners: []string{
						"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/resource-s1": {
					owners: []string{
						"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
					},
				},
				"/v1, Kind=ConfigMap, ns1/resource-c1": {
					owners: []string{
						"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
					},
				},
			},
		},
	},
	{
		name: "A ClusterResourceSet applied to two clusters",
		args: objectGraphTestArgs{
			objs: func() []client.Object {
				objs := []client.Object{}
				objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "cluster2").Objs()...)

				objs = append(objs, test.NewFakeClusterResourceSet("ns1", "crs1").
					WithSecret("resource-s1").
					WithConfigMap("resource-c1").
					ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster1")).
					ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster2")).
					Objs()...)

				return objs
			}(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster2": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},
				"/v1, Kind=Secret, ns1/cluster2-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster2-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster1": {
					owners: []string{
						"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster2": {
					owners: []string{
						"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
					},
				},
				"/v1, Kind=Secret, ns1/resource-s1": {
					owners: []string{
						"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
					},
				},
				"/v1, Kind=ConfigMap, ns1/resource-c1": {
					owners: []string{
						"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
					},
				},
			},
		},
	},
	{
		// NOTE: External objects are CRD types installed by clusterctl, but not directly related with the CAPI hierarchy of objects. e.g. IPAM claims.
		name: "Namespaced External Objects with force move label",
		args: objectGraphTestArgs{
			objs: test.NewFakeExternalObject("ns1", "externalObject1").Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"external.cluster.x-k8s.io/v1beta1, Kind=GenericExternalObject, ns1/externalObject1": {
					forceMove: true,
				},
			},
		},
	},
	{
		// NOTE: External objects are CRD types installed by clusterctl, but not directly related with the CAPI hierarchy of objects. e.g. IPAM claims.
		name: "Global External Objects with force move label",
		args: objectGraphTestArgs{
			objs: test.NewFakeClusterExternalObject("externalObject1").Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"external.cluster.x-k8s.io/v1beta1, Kind=GenericClusterExternalObject, /externalObject1": {
					forceMove: true,
					isGlobal:  true,
				},
			},
		},
	},
	{
		// NOTE: Infrastructure providers global credentials are going to be stored in Secrets in the provider's namespaces.
		name: "Secrets from provider's namespace",
		args: objectGraphTestArgs{
			objs: []client.Object{
				test.NewSecret("infra-system", "credentials"),
			},
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"/v1, Kind=Secret, infra-system/credentials": {},
			},
		},
	},
	{
		name: "Cluster owning a secret with infrastructure credentials",
		args: objectGraphTestArgs{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithCredentialSecret().Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-ca": {
					softOwners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
				"/v1, Kind=Secret, ns1/cluster1-credentials": {
					owners: []string{
						"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
					},
				},
			},
		},
	},
	{
		name: "A global identity for an infrastructure provider owning a Secret with credentials in the provider's namespace",
		args: objectGraphTestArgs{
			objs: test.NewFakeClusterInfrastructureIdentity("infra1-identity").
				WithSecretIn("infra1-system"). // a secret in infra1-system namespace, where an infrastructure provider is installed
				Objs(),
		},
		want: wantGraph{
			nodes: map[string]wantGraphItem{
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericClusterInfrastructureIdentity, /infra1-identity": {
					isGlobal:           true,
					forceMove:          true,
					forceMoveHierarchy: true,
				},
				"/v1, Kind=Secret, infra1-system/infra1-identity-credentials": {
					owners: []string{
						"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericClusterInfrastructureIdentity, /infra1-identity",
					},
				},
			},
		},
	},
}

func getDetachedObjectGraphWihObjs(objs []client.Object) (*objectGraph, error) {
	graph := newObjectGraph(nil, nil) // detached from any cluster
	for _, o := range objs {
		u := &unstructured.Unstructured{}
		if err := test.FakeScheme.Convert(o, u, nil); err != nil {
			return nil, errors.Wrap(err, "failed to convert object in unstructured")
		}
		graph.addObj(u)
	}

	// given that we are not relying on discovery while testing in "detached mode (without a fake client)" it is required to:
	for _, node := range graph.getNodes() {
		// enforce forceMoveHierarchy for Clusters, ClusterResourceSets, GenericClusterInfrastructureIdentity
		if node.identity.Kind == "Cluster" || node.identity.Kind == "ClusterResourceSet" || node.identity.Kind == "GenericClusterInfrastructureIdentity" {
			node.forceMove = true
			node.forceMoveHierarchy = true
		}
		// enforce forceMove for GenericExternalObject, GenericClusterExternalObject
		if node.identity.Kind == "GenericExternalObject" || node.identity.Kind == "GenericClusterExternalObject" {
			node.forceMove = true
		}
		// enforce isGlobal for GenericClusterInfrastructureIdentity and GenericClusterExternalObject
		if node.identity.Kind == "GenericClusterInfrastructureIdentity" || node.identity.Kind == "GenericClusterExternalObject" {
			node.isGlobal = true
		}
	}

	return graph, nil
}

func TestObjectGraph_addObj_WithFakeObjects(t *testing.T) {
	// NB. we are testing the graph is properly built starting from objects (this test) or from the same objects read from the cluster (TestGraphBuilder_Discovery)
	for _, tt := range objectGraphsTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			graph, err := getDetachedObjectGraphWihObjs(tt.args.objs)
			g.Expect(err).NotTo(HaveOccurred())

			// call setSoftOwnership so there is functional parity with discovery
			graph.setSoftOwnership()

			assertGraph(t, graph, tt.want)
		})
	}
}

func getObjectGraphWithObjs(objs []client.Object) *objectGraph {
	fromProxy := getFakeProxyWithCRDs()

	for _, o := range objs {
		fromProxy.WithObjs(o)
	}

	fromProxy.WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.2.3", "infra1-system")
	inventory := newInventoryClient(fromProxy, fakePollImmediateWaiter)

	return newObjectGraph(fromProxy, inventory)
}

func getObjectGraph() *objectGraph {
	// build object graph from file
	fromProxy := getFakeProxyWithCRDs()

	fromProxy.WithProviderInventory("infra1", clusterctlv1.InfrastructureProviderType, "v1.2.3", "infra1-system")
	inventory := newInventoryClient(fromProxy, fakePollImmediateWaiter)

	return newObjectGraph(fromProxy, inventory)
}

func getFakeProxyWithCRDs() *test.FakeProxy {
	proxy := test.NewFakeProxy()
	for _, o := range test.FakeCRDList() {
		proxy.WithObjs(o)
	}
	return proxy
}

func getFakeDiscoveryTypes(graph *objectGraph) error {
	if err := graph.getDiscoveryTypes(); err != nil {
		return err
	}

	// Given that the Fake client behaves in a different way than real client, for this test we are required to add the List suffix to all the types.
	for _, discoveryType := range graph.types {
		discoveryType.typeMeta.Kind = fmt.Sprintf("%sList", discoveryType.typeMeta.Kind)
	}
	return nil
}

func TestObjectGraph_Discovery(t *testing.T) {
	// NB. we are testing the graph is properly built starting from objects (TestGraphBuilder_addObj_WithFakeObjects) or from the same objects read from the cluster (this test).
	for _, tt := range objectGraphsTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create an objectGraph bound to a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.args.objs)

			// Get all the types to be considered for discovery
			err := getFakeDiscoveryTypes(graph)
			g.Expect(err).NotTo(HaveOccurred())

			// finally test discovery
			err = graph.Discovery("")
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			assertGraph(t, graph, tt.want)
		})
	}
}

func TestObjectGraph_DiscoveryByNamespace(t *testing.T) {
	type args struct {
		namespace string
		objs      []client.Object
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
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
					objs = append(objs, test.NewFakeCluster("ns2", "cluster1").Objs()...)
					return objs
				}(),
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
						forceMove:          true,
						forceMoveHierarchy: true,
					},
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
						owners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
						},
					},
					"/v1, Kind=Secret, ns1/cluster1-ca": {
						softOwners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
						},
					},
					"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
						owners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
						},
					},
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns2/cluster1": {
						forceMove:          true,
						forceMoveHierarchy: true,
					},
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns2/cluster1": {
						owners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns2/cluster1",
						},
					},
					"/v1, Kind=Secret, ns2/cluster1-ca": {
						softOwners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns2/cluster1", // NB. this secret is not linked to the cluster through owner ref
						},
					},
					"/v1, Kind=Secret, ns2/cluster1-kubeconfig": {
						owners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns2/cluster1",
						},
					},
				},
			},
		},
		{
			name: "two clusters, in different namespaces, read only 1",
			args: args{
				namespace: "ns1", // read only from ns1
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
					objs = append(objs, test.NewFakeCluster("ns2", "cluster1").Objs()...)
					return objs
				}(),
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
						forceMove:          true,
						forceMoveHierarchy: true,
					},
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1": {
						owners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
						},
					},
					"/v1, Kind=Secret, ns1/cluster1-ca": {
						softOwners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // NB. this secret is not linked to the cluster through owner ref
						},
					},
					"/v1, Kind=Secret, ns1/cluster1-kubeconfig": {
						owners: []string{
							"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
						},
					},
				},
			},
		},
		{
			// NOTE: External objects are CRD types installed by clusterctl, but not directly related with the CAPI hierarchy of objects. e.g. IPAM claims.
			name: "Namespaced External Objects with force move label",
			args: args{
				namespace: "ns1",                                                       // read only from ns1
				objs:      test.NewFakeExternalObject("ns1", "externalObject1").Objs(), // Fake external object with
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"external.cluster.x-k8s.io/v1beta1, Kind=GenericExternalObject, ns1/externalObject1": {
						forceMove: true,
					},
				},
			},
		},
		{
			// NOTE: Infrastructure providers global credentials are going to be stored in Secrets in the provider's namespaces.
			name: "Secrets from provider's namespace (e.g. credentials) should always be read",
			args: args{
				namespace: "ns1", // read only from ns1
				objs: []client.Object{
					test.NewSecret("infra1-system", "infra1-credentials"), // a secret in infra1-system namespace, where an infrastructure provider is installed
				},
			},
			want: wantGraph{
				nodes: map[string]wantGraphItem{
					"/v1, Kind=Secret, infra1-system/infra1-credentials": {},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create an objectGraph bound to a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.args.objs)

			// Get all the types to be considered for discovery
			err := getFakeDiscoveryTypes(graph)
			g.Expect(err).NotTo(HaveOccurred())

			// finally test discovery
			err = graph.Discovery(tt.args.namespace)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			assertGraph(t, graph, tt.want)
		})
	}
}

func Test_objectGraph_setSoftOwnership(t *testing.T) {
	type fields struct {
		objs []client.Object
	}
	tests := []struct {
		name        string
		fields      fields
		wantSecrets map[string][]string
	}{
		{
			name: "A cluster with a soft owned secret",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "foo").Objs(),
			},
			wantSecrets: map[string][]string{ // wantSecrets is a map[node UID] --> list of soft owner UIDs
				"/v1, Kind=Secret, ns1/foo-ca": { // the ca secret has no explicit OwnerRef to the cluster, so it should be identified as a soft ownership
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo",
				},
				"/v1, Kind=Secret, ns1/foo-kubeconfig": {}, // the kubeconfig secret has explicit OwnerRef to the cluster, so it should NOT be identified as a soft ownership
			},
		},
		{
			name: "A cluster with a soft owned secret (cluster name with - in the middle)",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "foo-bar").Objs(),
			},
			wantSecrets: map[string][]string{ // wantSecrets is a map[node UID] --> list of soft owner UIDs
				"/v1, Kind=Secret, ns1/foo-bar-ca": { // the ca secret has no explicit OwnerRef to the cluster, so it should be identified as a soft ownership
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo-bar",
				},
				"/v1, Kind=Secret, ns1/foo-bar-kubeconfig": {}, // the kubeconfig secret has explicit OwnerRef to the cluster, so it should NOT be identified as a soft ownership
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			graph, err := getDetachedObjectGraphWihObjs(tt.fields.objs)
			g.Expect(err).NotTo(HaveOccurred())

			graph.setSoftOwnership()

			gotSecrets := graph.getSecrets()
			g.Expect(gotSecrets).To(HaveLen(len(tt.wantSecrets)))

			for _, secret := range gotSecrets {
				wantObjects, ok := tt.wantSecrets[string(secret.identity.UID)]
				g.Expect(ok).To(BeTrue())

				gotObjects := []string{}
				for softOwners := range secret.softOwners {
					gotObjects = append(gotObjects, string(softOwners.identity.UID))
				}

				g.Expect(gotObjects).To(ConsistOf(wantObjects))
			}
		})
	}
}

func Test_objectGraph_setClusterTenants(t *testing.T) {
	type fields struct {
		objs []client.Object
	}
	tests := []struct {
		name         string
		fields       fields
		wantClusters map[string][]string
	}{
		{
			name: "One cluster",
			fields: fields{
				objs: test.NewFakeCluster("ns1", "foo").Objs(),
			},
			wantClusters: map[string][]string{ // wantClusters is a map[Cluster.UID] --> list of UIDs
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo": {
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo", // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
					"/v1, Kind=Secret, ns1/foo-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/foo-kubeconfig",
				},
			},
		},
		{
			name: "Object not owned by a cluster should be ignored",
			fields: fields{
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "foo").Objs()...)
					objs = append(objs, test.NewFakeInfrastructureTemplate("orphan")) // orphan object, not owned by  any cluster
					return objs
				}(),
			},
			wantClusters: map[string][]string{ // wantClusters is a map[Cluster.UID] --> list of UIDs
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo": {
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo", // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
					"/v1, Kind=Secret, ns1/foo-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/foo-kubeconfig",
				},
			},
		},
		{
			name: "Two clusters",
			fields: fields{
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "foo").Objs()...)
					objs = append(objs, test.NewFakeCluster("ns1", "bar").Objs()...)
					return objs
				}(),
			},
			wantClusters: map[string][]string{ // wantClusters is a map[Cluster.UID] --> list of UIDs
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo": {
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo", // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
					"/v1, Kind=Secret, ns1/foo-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/foo-kubeconfig",
				},
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/bar": {
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/bar", // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/bar",
					"/v1, Kind=Secret, ns1/bar-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/bar-kubeconfig",
				},
			},
		},
		{
			name: "Two clusters with a shared object",
			fields: fields{
				objs: func() []client.Object {
					sharedInfrastructureTemplate := test.NewFakeInfrastructureTemplate("shared")

					objs := []client.Object{
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
			wantClusters: map[string][]string{ // wantClusters is a map[Cluster.UID] --> list of UIDs
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/shared", // the shared object should be in both lists
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",                                           // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"/v1, Kind=Secret, ns1/cluster1-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
					"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster1-ms1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/cluster1-ms1",
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster1-m1",
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cluster1-m1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster1-m1",
					"/v1, Kind=Secret, ns1/cluster1-m1",
				},
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/shared", // the shared object should be in both lists
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",                                           // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster2",
					"/v1, Kind=Secret, ns1/cluster2-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/cluster2-kubeconfig",
					"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster2-ms1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/cluster2-ms1",
					"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster2-m1",
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cluster2-m1",
					"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster2-m1",
					"/v1, Kind=Secret, ns1/cluster2-m1",
				},
			},
		},
		{
			name: "A ClusterResourceSet applied to a cluster",
			fields: fields{
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)

					objs = append(objs, test.NewFakeClusterResourceSet("ns1", "crs1").
						WithSecret("resource-s1").
						WithConfigMap("resource-c1").
						ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster1")).
						Objs()...)

					return objs
				}(),
			},
			wantClusters: map[string][]string{ // wantClusters is a map[Cluster.UID] --> list of UIDs
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"/v1, Kind=Secret, ns1/cluster1-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster1", // ClusterResourceSetBinding are owned by the cluster
				},
			},
		},
		{
			name: "A ClusterResourceSet applied to two clusters",
			fields: fields{
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
					objs = append(objs, test.NewFakeCluster("ns1", "cluster2").Objs()...)

					objs = append(objs, test.NewFakeClusterResourceSet("ns1", "crs1").
						WithSecret("resource-s1").
						WithConfigMap("resource-c1").
						ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster1")).
						ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster2")).
						Objs()...)

					return objs
				}(),
			},
			wantClusters: map[string][]string{ // wantClusters is a map[Cluster.UID] --> list of UIDs
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1": {
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1", // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
					"/v1, Kind=Secret, ns1/cluster1-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster1", // ClusterResourceSetBinding are owned by the cluster
				},
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2": {
					"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2", // the cluster should be tenant of itself
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster2",
					"/v1, Kind=Secret, ns1/cluster2-ca", // the ca secret is a soft owned
					"/v1, Kind=Secret, ns1/cluster2-kubeconfig",
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster2", // ClusterResourceSetBinding are owned by the cluster
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gb, err := getDetachedObjectGraphWihObjs(tt.fields.objs)
			g.Expect(err).NotTo(HaveOccurred())

			// we want to check that soft dependent nodes are considered part of the cluster, so we make sure to call SetSoftDependants before SetClusterTenants
			gb.setSoftOwnership()

			// finally test SetTenants
			gb.setTenants()

			gotClusters := gb.getClusters()
			sort.Slice(gotClusters, func(i, j int) bool {
				return gotClusters[i].identity.UID < gotClusters[j].identity.UID
			})

			g.Expect(gotClusters).To(HaveLen(len(tt.wantClusters)))

			for _, cluster := range gotClusters {
				wantTenants, ok := tt.wantClusters[string(cluster.identity.UID)]
				g.Expect(ok).To(BeTrue())

				gotTenants := []string{}
				for _, node := range gb.uidToNode {
					for c := range node.tenant {
						if c.identity.UID == cluster.identity.UID {
							gotTenants = append(gotTenants, string(node.identity.UID))
							g.Expect(node.isGlobalHierarchy).To(BeFalse()) // We should make sure that everything below a Cluster is not considered global
						}
					}
				}

				g.Expect(gotTenants).To(ConsistOf(wantTenants))
			}
		})
	}
}

func Test_objectGraph_setCRSTenants(t *testing.T) {
	type fields struct {
		objs []client.Object
	}
	tests := []struct {
		name     string
		fields   fields
		wantCRSs map[string][]string
	}{
		{
			name: "A ClusterResourceSet applied to a cluster",
			fields: fields{
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)

					objs = append(objs, test.NewFakeClusterResourceSet("ns1", "crs1").
						WithSecret("resource-s1").
						WithConfigMap("resource-c1").
						ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster1")).
						Objs()...)

					return objs
				}(),
			},
			wantCRSs: map[string][]string{ // wantCRDs is a map[ClusterResourceSet.UID] --> list of UIDs
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1": {
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",            // the ClusterResourceSet should be tenant of itself
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster1", // ClusterResourceSetBinding are owned by ClusterResourceSet
					"/v1, Kind=Secret, ns1/resource-s1",                                             // resource are owned by ClusterResourceSet
					"/v1, Kind=ConfigMap, ns1/resource-c1",                                          // resource are owned by ClusterResourceSet
				},
			},
		},
		{
			name: "A ClusterResourceSet applied to two clusters",
			fields: fields{
				objs: func() []client.Object {
					objs := []client.Object{}
					objs = append(objs, test.NewFakeCluster("ns1", "cluster1").Objs()...)
					objs = append(objs, test.NewFakeCluster("ns1", "cluster2").Objs()...)

					objs = append(objs, test.NewFakeClusterResourceSet("ns1", "crs1").
						WithSecret("resource-s1").
						WithConfigMap("resource-c1").
						ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster1")).
						ApplyToCluster(test.SelectClusterObj(objs, "ns1", "cluster2")).
						Objs()...)

					return objs
				}(),
			},
			wantCRSs: map[string][]string{ // wantCRDs is a map[ClusterResourceSet.UID] --> list of UIDs
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1": {
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",            // the ClusterResourceSet should be tenant of itself
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster1", // ClusterResourceSetBinding are owned by ClusterResourceSet
					"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster2", // ClusterResourceSetBinding are owned by ClusterResourceSet
					"/v1, Kind=Secret, ns1/resource-s1",                                             // resource are owned by ClusterResourceSet
					"/v1, Kind=ConfigMap, ns1/resource-c1",                                          // resource are owned by ClusterResourceSet
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gb, err := getDetachedObjectGraphWihObjs(tt.fields.objs)
			g.Expect(err).NotTo(HaveOccurred())

			gb.setTenants()

			gotCRSs := gb.getCRSs()
			sort.Slice(gotCRSs, func(i, j int) bool {
				return gotCRSs[i].identity.UID < gotCRSs[j].identity.UID
			})

			g.Expect(gotCRSs).To(HaveLen(len(tt.wantCRSs)))

			for _, crs := range gotCRSs {
				wantTenants, ok := tt.wantCRSs[string(crs.identity.UID)]
				g.Expect(ok).To(BeTrue())

				gotTenants := []string{}
				for _, node := range gb.uidToNode {
					for c := range node.tenant {
						if c.identity.UID == crs.identity.UID {
							gotTenants = append(gotTenants, string(node.identity.UID))
							g.Expect(node.isGlobalHierarchy).To(BeFalse()) // We should make sure that everything below a CRS is not considered global
						}
					}
				}

				g.Expect(gotTenants).To(ConsistOf(wantTenants))
			}
		})
	}
}

func Test_objectGraph_setGlobalIdentityTenants(t *testing.T) {
	type fields struct {
		objs []client.Object
	}
	tests := []struct {
		name         string
		fields       fields
		wantIdentity map[string][]string
	}{
		{
			name: "A global identity for an infrastructure provider owning a Secret with credentials in the provider's namespace",
			fields: fields{
				objs: test.NewFakeClusterInfrastructureIdentity("infra1-identity").
					WithSecretIn("infra1-system"). // a secret in infra1-system namespace, where an infrastructure provider is installed
					Objs(),
			},
			wantIdentity: map[string][]string{ // wantCRDs is a map[ClusterResourceSet.UID] --> list of UIDs
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericClusterInfrastructureIdentity, /infra1-identity": {
					"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericClusterInfrastructureIdentity, /infra1-identity", // the global identity should be tenant of itself
					"/v1, Kind=Secret, infra1-system/infra1-identity-credentials",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			gb, err := getDetachedObjectGraphWihObjs(tt.fields.objs)
			g.Expect(err).NotTo(HaveOccurred())

			gb.setTenants()

			gotIdentity := []*node{}
			for _, n := range gb.getNodes() {
				if n.forceMoveHierarchy {
					gotIdentity = append(gotIdentity, n)
				}
			}
			sort.Slice(gotIdentity, func(i, j int) bool {
				return gotIdentity[i].identity.UID < gotIdentity[j].identity.UID
			})
			g.Expect(gotIdentity).To(HaveLen(len(tt.wantIdentity)))

			for _, i := range gotIdentity {
				wantTenants, ok := tt.wantIdentity[string(i.identity.UID)]
				g.Expect(ok).To(BeTrue())

				gotTenants := []string{}
				for _, node := range gb.uidToNode {
					for c := range node.tenant {
						if c.identity.UID == i.identity.UID {
							gotTenants = append(gotTenants, string(node.identity.UID))
							g.Expect(node.isGlobalHierarchy).To(BeTrue()) // We should make sure that everything below a global object is considered global
						}
					}
				}

				g.Expect(gotTenants).To(ConsistOf(wantTenants))
			}
		})
	}
}
