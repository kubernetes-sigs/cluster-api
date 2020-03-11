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
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
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
				"exp.cluster.x-k8s.io/v1alpha3, Kind=MachinePool, ns1/mp1",
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
	g := NewWithT(t)
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			g.Expect(err).NotTo(HaveOccurred())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("ns1", discoveryTypes)).To(Succeed())

			moveSequence := getMoveSequence(graph)
			g.Expect(moveSequence.groups).To(HaveLen(len(tt.wantMoveGroups)))

			for i, gotGroup := range moveSequence.groups {
				wantGroup := tt.wantMoveGroups[i]
				gotNodes := []string{}
				for _, node := range gotGroup {
					gotNodes = append(gotNodes, string(node.identity.UID))
				}

				g.Expect(gotNodes).To(ConsistOf(wantGroup))
			}
		})
	}
}

func Test_objectMover_move(t *testing.T) {
	g := NewWithT(t)
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			g.Expect(err).NotTo(HaveOccurred())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("ns1", discoveryTypes)).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			err = mover.move(graph, toProxy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			// check that the objects are removed from the source cluster and are created in the target cluster
			csFrom, err := graph.proxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

			csTo, err := toProxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

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

func Test_objectMover_checkProvisioningCompleted(t *testing.T) {
	g := NewWithT(t)

	type fields struct {
		objs []runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Blocks with a cluster without InfrastructureReady",
			fields: fields{
				objs: []runtime.Object{
					&clusterv1.Cluster{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Cluster",
							APIVersion: clusterv1.GroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "cluster1",
						},
						Status: clusterv1.ClusterStatus{
							InfrastructureReady:     false,
							ControlPlaneInitialized: true,
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Blocks with a cluster without ControlPlaneInitialized",
			fields: fields{
				objs: []runtime.Object{
					&clusterv1.Cluster{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Cluster",
							APIVersion: clusterv1.GroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "cluster1",
						},
						Status: clusterv1.ClusterStatus{
							InfrastructureReady:     true,
							ControlPlaneInitialized: false,
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Blocks with a cluster without ControlPlaneReady",
			fields: fields{
				objs: []runtime.Object{
					&clusterv1.Cluster{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Cluster",
							APIVersion: clusterv1.GroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "cluster1",
						},
						Spec: clusterv1.ClusterSpec{
							ControlPlaneRef: &corev1.ObjectReference{},
						},
						Status: clusterv1.ClusterStatus{
							InfrastructureReady:     true,
							ControlPlaneInitialized: true,
							ControlPlaneReady:       false,
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Blocks with a Machine Without NodeRef",
			fields: fields{
				objs: []runtime.Object{
					&clusterv1.Cluster{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Cluster",
							APIVersion: clusterv1.GroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "cluster1",
							UID:       "cluster1",
						},
						Status: clusterv1.ClusterStatus{
							InfrastructureReady:     true,
							ControlPlaneInitialized: true,
						},
					},
					&clusterv1.Machine{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Machine",
							APIVersion: clusterv1.GroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "machine1",
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       "cluster1",
									UID:        "cluster1",
								},
							},
						},
						Status: clusterv1.MachineStatus{
							NodeRef: nil,
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass",
			fields: fields{
				objs: []runtime.Object{
					&clusterv1.Cluster{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Cluster",
							APIVersion: clusterv1.GroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "cluster1",
							UID:       "cluster1",
						},
						Status: clusterv1.ClusterStatus{
							InfrastructureReady:     true,
							ControlPlaneInitialized: true,
						},
					},
					&clusterv1.Machine{
						TypeMeta: metav1.TypeMeta{
							Kind:       "Machine",
							APIVersion: clusterv1.GroupVersion.String(),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "ns1",
							Name:      "machine1",
							OwnerReferences: []metav1.OwnerReference{
								{
									APIVersion: clusterv1.GroupVersion.String(),
									Kind:       "Cluster",
									Name:       "cluster1",
									UID:        "cluster1",
								},
							},
						},
						Status: clusterv1.MachineStatus{
							NodeRef: &corev1.ObjectReference{},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			discoveryTypes, err := getFakeDiscoveryTypes(graph)
			g.Expect(err).NotTo(HaveOccurred())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("ns1", discoveryTypes)).To(Succeed())

			o := &objectMover{
				fromProxy: graph.proxy,
			}
			err = o.checkProvisioningCompleted(graph)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func Test_objectsMoverService_checkTargetProviders(t *testing.T) {
	g := NewWithT(t)

	type fields struct {
		fromProxy Proxy
	}
	type args struct {
		toProxy   Proxy
		namespace string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "move objects in single namespace, all the providers in place (lazy matching)",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi-system", "").
					WithProviderInventory("kubeadm", clusterctlv1.BootstrapProviderType, "v1.0.0", "cabpk-system", "").
					WithProviderInventory("capa", clusterctlv1.InfrastructureProviderType, "v1.0.0", "capa-system", ""),
			},
			args: args{
				namespace: "ns1", // a single namespaces
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi-system", "ns1").
					WithProviderInventory("kubeadm", clusterctlv1.BootstrapProviderType, "v1.0.0", "cabpk-system", "ns1").
					WithProviderInventory("capa", clusterctlv1.InfrastructureProviderType, "v1.0.0", "capa-system", "ns1"),
			},
			wantErr: false,
		},
		{
			name: "move objects in single namespace, all the providers in place but with a newer version (lazy matching)",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system", ""),
			},
			args: args{
				namespace: "ns1", // a single namespaces
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.1.0", "capi-system", "ns1"), // Lazy matching
			},
			wantErr: false,
		},
		{
			name: "move objects in all namespaces, all the providers in place (exact matching)",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi-system", "").
					WithProviderInventory("kubeadm", clusterctlv1.BootstrapProviderType, "v1.0.0", "cabpk-system", "").
					WithProviderInventory("capa", clusterctlv1.InfrastructureProviderType, "v1.0.0", "capa-system", ""),
			},
			args: args{
				namespace: "", // all namespaces
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi-system", "").
					WithProviderInventory("kubeadm", clusterctlv1.BootstrapProviderType, "v1.0.0", "cabpk-system", "").
					WithProviderInventory("capa", clusterctlv1.InfrastructureProviderType, "v1.0.0", "capa-system", ""),
			},
			wantErr: false,
		},
		{
			name: "move objects in all namespaces, all the providers in place but with a newer version (exact matching)",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system", ""),
			},
			args: args{
				namespace: "", // all namespaces
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.1.0", "capi-system", ""),
			},
			wantErr: false,
		},
		{
			name: "move objects in all namespaces, not exact matching",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system", ""),
			},
			args: args{
				namespace: "", // all namespaces
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.1.0", "capi-system", "ns1"), // Lazy matching only
			},
			wantErr: true,
		},
		{
			name: "fails if a provider is missing",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system", ""),
			},
			args: args{
				namespace: "", // all namespaces
				toProxy:   test.NewFakeProxy(),
			},
			wantErr: true,
		},
		{
			name: "fails if a provider version is older than expected",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system", ""),
			},
			args: args{
				namespace: "", // all namespaces
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi1-system", ""),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &objectMover{
				fromProviderInventory: newInventoryClient(tt.fields.fromProxy, nil),
			}
			err := o.checkTargetProviders(tt.args.namespace, newInventoryClient(tt.args.toProxy, nil))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
