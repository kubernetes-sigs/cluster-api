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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	addonsv1 "sigs.k8s.io/cluster-api/api/addons/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/infrastructure"
)

type moveTestsFields struct {
	objs []client.Object
}

var moveTests = []struct {
	name           string
	fields         moveTestsFields
	wantMoveGroups [][]string
	wantErr        bool
}{
	{
		name: "Cluster with ClusterClass",
		fields: moveTestsFields{
			objs: func() []client.Object {
				objs := test.NewFakeClusterClass("ns1", "class1").Objs()
				objs = append(objs, test.NewFakeCluster("ns1", "foo").WithTopologyClass("class1").Objs()...)
				return deduplicateObjects(objs)
			}(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=ClusterClass, ns1/class1",
			},
			{ // group 2
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlaneTemplate, ns1/class1",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo",
			},
			{ // group 3
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Paused Cluster",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").WithPaused().Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo",
			},
		},
		wantErr: true,
	},
	{
		name: "Paused ClusterClass",
		fields: moveTestsFields{
			objs: test.NewFakeClusterClass("ns1", "class1").WithPaused().Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=ClusterClass, ns1/class1",
			},
			{ // group 2
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlaneTemplate, ns1/class1",
			},
		},
		wantErr: true,
	},
	{
		name: "Cluster with cloud config secret with the force move label",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").WithCloudConfigSecret().Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo",
				// objects with force move flag
				"/v1, Kind=Secret, ns1/foo-cloud-config",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo",
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
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"/v1, Kind=Secret, ns1/cluster1-ca",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m1",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m2",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by Machines
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m1",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m2",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m2",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by GenericBootstrapConfigs
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
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfigTemplate, ns1/ms1",
				clusterv1.GroupVersion.String() + ", Kind=MachineSet, ns1/ms1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachineTemplate, ns1/ms1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineSets
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m1",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m2",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m1",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m2",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m2",
			},
			{ // group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by GenericBootstrapConfigs
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
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfigTemplate, ns1/md1",
				clusterv1.GroupVersion.String() + ", Kind=MachineDeployment, ns1/md1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachineTemplate, ns1/md1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineDeployments
				clusterv1.GroupVersion.String() + ", Kind=MachineSet, ns1/ms1",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by MachineSets
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m1",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m2",
			},
			{ // group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by Machines
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m1",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m2",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m2",
			},
			{ // group 6 (objects with ownerReferences in group 1,2,3,5,6)
				// owned by GenericBootstrapConfigs
				"/v1, Kind=Secret, ns1/m1",
				"/v1, Kind=Secret, ns1/m2",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with MachineDeployment with a static bootstrap config",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "cluster1").
				WithMachineDeployments(
					test.NewFakeMachineDeployment("md1").
						WithStaticBootstrapConfig().
						WithMachineSets(
							test.NewFakeMachineSet("ms1").
								WithStaticBootstrapConfig().
								WithMachines(
									test.NewFakeMachine("m1").
										WithStaticBootstrapConfig(),
									test.NewFakeMachine("m2").
										WithStaticBootstrapConfig(),
								),
						),
				).Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				clusterv1.GroupVersion.String() + ", Kind=MachineDeployment, ns1/md1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachineTemplate, ns1/md1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineDeployments
				clusterv1.GroupVersion.String() + ", Kind=MachineSet, ns1/ms1",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by MachineSets
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m1",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m2",
			},
			{ // group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by Machines
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m2",
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
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlane, ns1/cp1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachineTemplate, ns1/cp1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"/v1, Kind=Secret, ns1/cluster1-sa",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m1",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/m2",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m1",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/m2",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/m2",
			},
			{ // group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by GenericBootstrapConfigs
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
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfigTemplate, ns1/mp1",
				clusterv1.GroupVersion.String() + ", Kind=MachinePool, ns1/mp1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachineTemplate, ns1/mp1",
			},
		},
		wantErr: false,
	},
	{
		name: "Two clusters",
		fields: moveTestsFields{
			objs: func() []client.Object {
				objs := []client.Object{}
				objs = append(objs, test.NewFakeCluster("ns1", "foo").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "bar").Objs()...)
				return objs
			}(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/bar",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo",
				"/v1, Kind=Secret, ns1/bar-ca",
				"/v1, Kind=Secret, ns1/bar-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/bar",
			},
		},
	},
	{
		name: "Two clusters with a shared object",
		fields: moveTestsFields{
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
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster2",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfigTemplate, ns1/cluster1-ms1",
				clusterv1.GroupVersion.String() + ", Kind=MachineSet, ns1/cluster1-ms1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
				"/v1, Kind=Secret, ns1/cluster2-ca",
				"/v1, Kind=Secret, ns1/cluster2-kubeconfig",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfigTemplate, ns1/cluster2-ms1",
				clusterv1.GroupVersion.String() + ", Kind=MachineSet, ns1/cluster2-ms1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster2",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachineTemplate, ns1/shared", // shared object
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineSets
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/cluster1-m1",
				clusterv1.GroupVersion.String() + ", Kind=Machine, ns1/cluster2-m1",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/cluster1-m1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/cluster1-m1",
				clusterv1.GroupVersionBootstrap.String() + ", Kind=GenericBootstrapConfig, ns1/cluster2-m1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureMachine, ns1/cluster2-m1",
			},
			{ // group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by GenericBootstrapConfigs
				"/v1, Kind=Secret, ns1/cluster1-m1",
				"/v1, Kind=Secret, ns1/cluster2-m1",
			},
		},
	},
	{
		name: "A ClusterResourceSet applied to a cluster",
		fields: moveTestsFields{
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
		wantMoveGroups: [][]string{
			{ // group 1
				// Cluster
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/cluster1",
				// ClusterResourceSet
				addonsv1.GroupVersion.String() + ", Kind=ClusterResourceSet, ns1/crs1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/cluster1",
				// owned by ClusterResourceSet
				"/v1, Kind=Secret, ns1/resource-s1",
				"/v1, Kind=ConfigMap, ns1/resource-c1",
				// owned by ClusterResourceSet & Cluster
				addonsv1.GroupVersion.String() + ", Kind=ClusterResourceSetBinding, ns1/cluster1",
			},
		},
	},
	{
		name: "Cluster with ClusterClass",
		fields: moveTestsFields{
			objs: func() []client.Object {
				objs := test.NewFakeClusterClass("ns1", "class1").Objs()
				objs = append(objs, test.NewFakeCluster("ns1", "foo").WithTopologyClass("class1").Objs()...)
				return deduplicateObjects(objs)
			}(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=ClusterClass, ns1/class1",
			},
			{ // group 2
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlaneTemplate, ns1/class1",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo",
			},
			{ // group 3
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Two Clusters with two ClusterClasses",
		fields: moveTestsFields{
			objs: func() []client.Object {
				objs := test.NewFakeClusterClass("ns1", "class1").Objs()
				objs = append(objs, test.NewFakeClusterClass("ns1", "class2").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "foo1").WithTopologyClass("class1").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "foo2").WithTopologyClass("class2").Objs()...)
				return deduplicateObjects(objs)
			}(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=ClusterClass, ns1/class1",
				clusterv1.GroupVersion.String() + ", Kind=ClusterClass, ns1/class2",
			},
			{ // group 2
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlaneTemplate, ns1/class1",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo1",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureClusterTemplate, ns1/class2",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlaneTemplate, ns1/class2",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo2",
			},
			{ // group 3
				"/v1, Kind=Secret, ns1/foo1-ca",
				"/v1, Kind=Secret, ns1/foo1-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo1",
				"/v1, Kind=Secret, ns1/foo2-ca",
				"/v1, Kind=Secret, ns1/foo2-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo2",
			},
		},
		wantErr: false,
	},
	{
		name: "Two Clusters sharing one ClusterClass",
		fields: moveTestsFields{
			objs: func() []client.Object {
				objs := test.NewFakeClusterClass("ns1", "class1").Objs()
				objs = append(objs, test.NewFakeCluster("ns1", "foo1").WithTopologyClass("class1").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns1", "foo2").WithTopologyClass("class1").Objs()...)
				return deduplicateObjects(objs)
			}(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=ClusterClass, ns1/class1",
			},
			{ // group 2
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlaneTemplate, ns1/class1",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo1",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo2",
			},
			{ // group 3
				"/v1, Kind=Secret, ns1/foo1-ca",
				"/v1, Kind=Secret, ns1/foo1-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo1",
				"/v1, Kind=Secret, ns1/foo2-ca",
				"/v1, Kind=Secret, ns1/foo2-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo2",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with unused ClusterClass",
		fields: moveTestsFields{
			objs: func() []client.Object {
				objs := test.NewFakeClusterClass("ns1", "class1").Objs()
				objs = append(objs, test.NewFakeCluster("ns1", "foo1").Objs()...)
				return deduplicateObjects(objs)
			}(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=ClusterClass, ns1/class1",
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo1",
			},
			{ // group 2
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				clusterv1.GroupVersionControlPlane.String() + ", Kind=GenericControlPlaneTemplate, ns1/class1",
				"/v1, Kind=Secret, ns1/foo1-ca",
				"/v1, Kind=Secret, ns1/foo1-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo1",
			},
		},
		wantErr: false,
	},
	{
		// NOTE: External objects are CRD types installed by clusterctl, but not directly related with the CAPI hierarchy of objects. e.g. IPAM claims.
		name: "Namespaced External Objects with force move label",
		fields: moveTestsFields{
			objs: test.NewFakeExternalObject("ns1", "externalObject1").Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group1
				"external.cluster.x-k8s.io/v1beta2, Kind=GenericExternalObject, ns1/externalObject1",
			},
		},
		wantErr: false,
	},
	{
		// NOTE: External objects are CRD types installed by clusterctl, but not directly related with the CAPI hierarchy of objects. e.g. IPAM claims.
		name: "Global External Objects with force move label",
		fields: moveTestsFields{
			objs: test.NewFakeClusterExternalObject("externalObject1").Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group1
				"external.cluster.x-k8s.io/v1beta2, Kind=GenericClusterExternalObject, externalObject1",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster owning a secret with infrastructure credentials",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").
				WithCredentialSecret().Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersion.String() + ", Kind=Cluster, ns1/foo",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-credentials",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericInfrastructureCluster, ns1/foo",
			},
		},
		wantErr: false,
	},
	{
		name: "A global identity for an infrastructure provider owning a Secret with credentials in the provider's namespace",
		fields: moveTestsFields{
			objs: test.NewFakeClusterInfrastructureIdentity("infra1-identity").
				WithSecretIn("infra1-system"). // a secret in infra1-system namespace, where an infrastructure provider is installed
				Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				clusterv1.GroupVersionInfrastructure.String() + ", Kind=GenericClusterInfrastructureIdentity, infra1-identity",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, infra1-system/infra1-identity-credentials",
			},
		},
		wantErr: false,
	},
}

var backupRestoreTests = []struct {
	name    string
	fields  moveTestsFields
	files   map[string]string
	wantErr bool
}{
	{
		name: "Cluster",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").Objs(),
		},
		files: map[string]string{
			"Cluster_ns1_foo.yaml":                      `{"apiVersion":"$CAPI","kind":"Cluster","metadata":{"name":"foo","namespace":"ns1","resourceVersion":"999","uid":"$CAPI, Kind=Cluster, ns1/foo"},"spec":{"infrastructureRef":{"apiGroup":"$INFRA_GROUP","kind":"GenericInfrastructureCluster","name":"foo"}}}` + "\n",
			"Secret_ns1_foo-kubeconfig.yaml":            `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"foo-kubeconfig","namespace":"ns1","ownerReferences":[{"apiVersion":"$CAPI","kind":"Cluster","name":"foo","uid":"$CAPI, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-kubeconfig"}}` + "\n",
			"Secret_ns1_foo-ca.yaml":                    `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"foo-ca","namespace":"ns1","resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-ca"}}` + "\n",
			"GenericInfrastructureCluster_ns1_foo.yaml": `{"apiVersion":"$INFRA","kind":"GenericInfrastructureCluster","metadata":{"labels":{"cluster.x-k8s.io/cluster-name":"foo"},"name":"foo","namespace":"ns1","ownerReferences":[{"apiVersion":"$CAPI","kind":"Cluster","name":"foo","uid":"$CAPI, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"$INFRA, Kind=GenericInfrastructureCluster, ns1/foo"}}` + "\n",
		},
		wantErr: false,
	},
	{
		name: "Many namespace cluster",
		fields: moveTestsFields{
			objs: func() []client.Object {
				objs := []client.Object{}
				objs = append(objs, test.NewFakeCluster("ns1", "foo").Objs()...)
				objs = append(objs, test.NewFakeCluster("ns2", "bar").Objs()...)
				return objs
			}(),
		},
		files: map[string]string{
			"Cluster_ns1_foo.yaml":                      `{"apiVersion":"$CAPI","kind":"Cluster","metadata":{"name":"foo","namespace":"ns1","resourceVersion":"999","uid":"$CAPI, Kind=Cluster, ns1/foo"},"spec":{"infrastructureRef":{"apiGroup":"$INFRA_GROUP","kind":"GenericInfrastructureCluster","name":"foo"}}}` + "\n",
			"Secret_ns1_foo-kubeconfig.yaml":            `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"foo-kubeconfig","namespace":"ns1","ownerReferences":[{"apiVersion":"$CAPI","kind":"Cluster","name":"foo","uid":"$CAPI, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-kubeconfig"}}` + "\n",
			"Secret_ns1_foo-ca.yaml":                    `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"foo-ca","namespace":"ns1","resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-ca"}}` + "\n",
			"GenericInfrastructureCluster_ns1_foo.yaml": `{"apiVersion":"$INFRA","kind":"GenericInfrastructureCluster","metadata":{"labels":{"cluster.x-k8s.io/cluster-name":"foo"},"name":"foo","namespace":"ns1","ownerReferences":[{"apiVersion":"$CAPI","kind":"Cluster","name":"foo","uid":"$CAPI, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"$INFRA, Kind=GenericInfrastructureCluster, ns1/foo"}}` + "\n",
			"Cluster_ns2_bar.yaml":                      `{"apiVersion":"$CAPI","kind":"Cluster","metadata":{"name":"bar","namespace":"ns2","resourceVersion":"999","uid":"$CAPI, Kind=Cluster, ns2/bar"},"spec":{"infrastructureRef":{"apiGroup":"$INFRA_GROUP","kind":"GenericInfrastructureCluster","name":"bar"}}}` + "\n",
			"Secret_ns2_bar-kubeconfig.yaml":            `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"bar-kubeconfig","namespace":"ns2","ownerReferences":[{"apiVersion":"$CAPI","kind":"Cluster","name":"bar","uid":"$CAPI, Kind=Cluster, ns2/bar"}],"resourceVersion":"999","uid":"/v1, Kind=Secret, ns2/bar-kubeconfig"}}` + "\n",
			"Secret_ns2_bar-ca.yaml":                    `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"bar-ca","namespace":"ns2","resourceVersion":"999","uid":"/v1, Kind=Secret, ns2/bar-ca"}}` + "\n",
			"GenericInfrastructureCluster_ns2_bar.yaml": `{"apiVersion":"$INFRA","kind":"GenericInfrastructureCluster","metadata":{"labels":{"cluster.x-k8s.io/cluster-name":"bar"},"name":"bar","namespace":"ns2","ownerReferences":[{"apiVersion":"$CAPI","kind":"Cluster","name":"bar","uid":"$CAPI, Kind=Cluster, ns2/bar"}],"resourceVersion":"999","uid":"$INFRA, Kind=GenericInfrastructureCluster, ns2/bar"}}` + "\n",
		},
		wantErr: false,
	},
}

func fixFilesGVS(file string) string {
	s := strings.ReplaceAll(file, "$CAPI", clusterv1.GroupVersion.String())
	s = strings.ReplaceAll(s, "$INFRA_GROUP", clusterv1.GroupVersionInfrastructure.Group)
	s = strings.ReplaceAll(s, "$INFRA", clusterv1.GroupVersionInfrastructure.String())
	return s
}

func Test_objectMover_backupTargetObject(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range backupRestoreTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			// Run backupTargetObject on nodes in graph
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			dir := t.TempDir()

			for _, node := range graph.uidToNode {
				err := mover.backupTargetObject(ctx, node, dir)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).ToNot(HaveOccurred())

				// objects are stored and serialized correctly in the temporary directory
				expectedFilename := node.getFilename()
				expectedFileContents, ok := tt.files[expectedFilename]
				if !ok {
					t.Errorf("Could not access file map: %v\n", expectedFilename)
				}
				expectedFileContents = fixFilesGVS(expectedFileContents)

				path := filepath.Join(dir, expectedFilename)
				fileContents, err := os.ReadFile(path) //nolint:gosec
				if err != nil {
					g.Expect(err).ToNot(HaveOccurred())
					return
				}

				firstFileStat, err := os.Stat(path)
				if err != nil {
					g.Expect(err).ToNot(HaveOccurred())
					return
				}

				fmt.Printf("Actual file content %v\n", string(fileContents))
				g.Expect(string(fileContents)).To(Equal(expectedFileContents))

				// Add delay so we ensure the file ModTime of updated files is different from old ones in the original files
				time.Sleep(time.Millisecond * 50)

				// Running backupTargetObject should override any existing files since it represents a new toDirectory
				err = mover.backupTargetObject(ctx, node, dir)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).ToNot(HaveOccurred())

				secondFileStat, err := os.Stat(path)
				if err != nil {
					g.Expect(err).ToNot(HaveOccurred())
					return
				}

				g.Expect(firstFileStat.ModTime()).To(BeTemporally("<", secondFileStat.ModTime()))
			}
		})
	}
}

func Test_objectMover_restoreTargetObject(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range backupRestoreTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			dir := t.TempDir()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraph()

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run restoreTargetObject
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			// Write go string slice to directory
			for _, file := range tt.files {
				tempFile, err := os.CreateTemp(dir, "obj")
				g.Expect(err).ToNot(HaveOccurred())

				_, err = tempFile.WriteString(fixFilesGVS(file))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(tempFile.Close()).To(Succeed())
			}

			objs, err := mover.filesToObjs(dir)
			g.Expect(err).ToNot(HaveOccurred())

			for i := range objs {
				g.Expect(graph.addRestoredObj(&objs[i])).ToNot(HaveOccurred())
			}

			for _, node := range graph.uidToNode {
				err = mover.restoreTargetObject(ctx, node, toProxy)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).ToNot(HaveOccurred())

				// Check objects are in new restored cluster
				csTo, err := toProxy.NewClient(ctx)
				g.Expect(err).ToNot(HaveOccurred())

				key := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are created in the target cluster
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("error = %v when checking for %s %v created in target cluster", err, oTo.GetKind(), key)
					continue
				}

				// Re-running restoreTargetObjects won't override existing objects
				err = mover.restoreTargetObject(ctx, node, toProxy)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).ToNot(HaveOccurred())

				// Check objects are in new restored cluster
				csAfter, err := toProxy.NewClient(ctx)
				g.Expect(err).ToNot(HaveOccurred())

				keyAfter := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are created in the target cluster
				oAfter := &unstructured.Unstructured{}
				oAfter.SetAPIVersion(node.identity.APIVersion)
				oAfter.SetKind(node.identity.Kind)

				if err := csAfter.Get(ctx, keyAfter, oAfter); err != nil {
					t.Errorf("error = %v when checking for %s %v created in target cluster", err, oAfter.GetKind(), key)
					continue
				}

				g.Expect(oAfter.GetAPIVersion()).Should(Equal(oTo.GetAPIVersion()))
				g.Expect(oAfter.GetName()).Should(Equal(oTo.GetName()))
				g.Expect(oAfter.GetCreationTimestamp()).Should(Equal(oTo.GetCreationTimestamp()))
				g.Expect(oAfter.GetUID()).Should(Equal(oTo.GetUID()))
				g.Expect(oAfter.GetOwnerReferences()).Should(BeComparableTo(oTo.GetOwnerReferences()))
			}
		})
	}
}

func Test_objectMover_toDirectory(t *testing.T) {
	tests := []struct {
		name    string
		fields  moveTestsFields
		files   map[string]string
		wantErr bool
	}{
		{
			name: "Cluster is paused",
			fields: moveTestsFields{
				objs: test.NewFakeCluster("ns1", "foo").WithPaused().Objs(),
			},
			wantErr: true,
		},
		{
			name: "ClusterClass is paused",
			fields: moveTestsFields{
				objs: test.NewFakeClusterClass("ns1", "foo").WithPaused().Objs(),
			},
			wantErr: true,
		},
	}
	tests = append(tests, backupRestoreTests...)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			// Run toDirectory
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			dir := t.TempDir()

			err := mover.toDirectory(ctx, graph, dir)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			// check that the objects are stored in the temporary directory but not deleted from the source cluster
			csFrom, err := graph.proxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			missingFiles := []string{}
			for _, node := range graph.uidToNode {
				key := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are not deleted from the source cluster
				oFrom := &unstructured.Unstructured{}
				oFrom.SetAPIVersion(node.identity.APIVersion)
				oFrom.SetKind(node.identity.Kind)

				err := csFrom.Get(ctx, key, oFrom)
				g.Expect(err).ToNot(HaveOccurred())

				// objects are stored in the temporary directory with the expected filename
				files, err := os.ReadDir(dir)
				g.Expect(err).ToNot(HaveOccurred())

				expectedFilename := node.getFilename()
				found := false
				for _, f := range files {
					if strings.Contains(f.Name(), expectedFilename) {
						found = true
					}
				}

				if !found {
					missingFiles = append(missingFiles, expectedFilename)
				}
			}

			g.Expect(missingFiles).To(BeEmpty())
		})
	}
}

func Test_objectMover_filesToObjs(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range backupRestoreTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dir := t.TempDir()

			for _, fileName := range tt.files {
				path := filepath.Join(dir, fileName)
				file, err := os.Create(path) //nolint:gosec // No security issue: unit test.
				if err != nil {
					return
				}

				_, err = file.WriteString(fixFilesGVS(tt.files[fileName]))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(file.Close()).To(Succeed())
			}

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Run filesToObjs
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			objs, err := mover.filesToObjs(dir)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			missingObjs := []unstructured.Unstructured{}
			for _, obj := range objs {
				found := false
				for _, expected := range tt.fields.objs {
					if expected.GetName() == obj.GetName() && expected.GetObjectKind() == obj.GetObjectKind() {
						found = true
					}
				}

				if !found {
					missingObjs = append(missingObjs, obj)
				}
			}

			g.Expect(missingObjs).To(BeEmpty())
		})
	}
}

func Test_objectMover_fromDirectory(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range backupRestoreTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			dir := t.TempDir()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraph()

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run fromDirectory
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			// Write go string slice to directory
			for _, file := range tt.files {
				tempFile, err := os.CreateTemp(dir, "obj")
				g.Expect(err).ToNot(HaveOccurred())

				_, err = tempFile.WriteString(fixFilesGVS(file))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(tempFile.Close()).To(Succeed())
			}

			objs, err := mover.filesToObjs(dir)
			g.Expect(err).ToNot(HaveOccurred())

			for i := range objs {
				g.Expect(graph.addRestoredObj(&objs[i])).ToNot(HaveOccurred())
			}

			// fromDirectory works on the target cluster which does not yet have objs to discover
			// instead set the owners and tenants correctly on object graph like how ObjectMover.Restore does
			// https://github.com/kubernetes-sigs/cluster-api/blob/main/cmd/clusterctl/client/cluster/mover.go#L129-L132
			graph.setSoftOwnership()
			graph.setTenants()
			graph.checkVirtualNode()

			err = mover.fromDirectory(ctx, graph, toProxy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			// Check objects are in new restored cluster
			csTo, err := toProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			for _, node := range graph.uidToNode {
				key := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are created in the target cluster
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("error = %v when checking for %s %v created in target cluster", err, oTo.GetKind(), key)
					continue
				}
			}
		})
	}
}

func Test_getMoveSequence(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

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

func Test_objectMover_move_dryRun(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move
			mover := objectMover{
				fromProxy: graph.proxy,
				dryRun:    true,
			}

			err := mover.move(ctx, graph, toProxy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			// check that the objects are kept in the source cluster and are not created in the target cluster
			csFrom, err := graph.proxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			csTo, err := toProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())
			for _, node := range graph.uidToNode {
				key := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}
				// objects are kept in source cluster as it's dry run
				oFrom := &unstructured.Unstructured{}
				oFrom.SetAPIVersion(node.identity.APIVersion)
				oFrom.SetKind(node.identity.Kind)

				if err := csFrom.Get(ctx, key, oFrom); err != nil {
					t.Errorf("error = %v when checking for %s %v kept in source cluster", err, oFrom.GetKind(), key)
					continue
				}

				// objects are not created in target cluster as it's dry run
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				err := csTo.Get(ctx, key, oTo)
				if err == nil {
					if oFrom.GetNamespace() != "" {
						t.Errorf("%s %v created in target cluster which should not", oFrom.GetKind(), key)
						continue
					}
				} else if !apierrors.IsNotFound(err) {
					t.Errorf("error = %v when checking for %s %v should not created ojects in target cluster", err, oFrom.GetKind(), key)
					continue
				}
			}
		})
	}
}

func Test_objectMover_move(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move
			mover := objectMover{
				fromProxy: graph.proxy,
			}
			err := mover.move(ctx, graph, toProxy)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			// check that the objects are removed from the source cluster and are created in the target cluster
			csFrom, err := graph.proxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			csTo, err := toProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

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
					if !node.isGlobal && !node.isGlobalHierarchy && !node.shouldNotDelete {
						t.Errorf("%s %v not deleted in source cluster", oFrom.GetKind(), key)
						continue
					}
				} else if !apierrors.IsNotFound(err) {
					t.Errorf("error = %v when checking for %s %v deleted in source cluster", err, oFrom.GetKind(), key)
					continue
				}

				// objects are created in the target cluster
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("error = %v when checking for %s %v created in target cluster", err, oFrom.GetKind(), key)
					continue
				}
			}
		})
	}
}

func Test_objectMover_move_with_Mutator(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	// we use same mutator function for all tests and validate outcome based on input.
	for _, tt := range moveTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			toNamespace := "foobar"
			updateKnownKinds := map[string][][]string{
				"Cluster": {
					{"metadata", "namespace"},
					{"spec", "controlPlaneRef", "namespace"},
					{"spec", "infrastructureRef", "namespace"},
					{"unknown", "field", "does", "not", "cause", "errors"},
				},
				"KubeadmControlPlane": {
					{"spec", "machineTemplate", "infrastructureRef", "namespace"},
				},
				"Machine": {
					{"spec", "bootstrap", "configRef", "namespace"},
					{"spec", "infrastructureRef", "namespace"},
				},
			}
			var namespaceMutator ResourceMutatorFunc = func(u *unstructured.Unstructured) error {
				if u == nil || u.Object == nil {
					return nil
				}
				if u.GetNamespace() != "" {
					u.SetNamespace(toNamespace)
				}
				if fields, knownKind := updateKnownKinds[u.GetKind()]; knownKind {
					for _, nsField := range fields {
						_, exists, err := unstructured.NestedFieldNoCopy(u.Object, nsField...)
						g.Expect(err).ToNot(HaveOccurred())
						if exists {
							g.Expect(unstructured.SetNestedField(u.Object, toNamespace, nsField...)).To(Succeed())
						}
					}
				}
				return nil
			}

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move with mutators
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			err := mover.move(ctx, graph, toProxy, namespaceMutator)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			// check that the objects are removed from the source cluster and are created in the target cluster
			csFrom, err := graph.proxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			csTo, err := toProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

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
					if !node.isGlobal && !node.isGlobalHierarchy && !node.shouldNotDelete {
						t.Errorf("%s %v not deleted in source cluster", oFrom.GetKind(), key)
						continue
					}
				} else if !apierrors.IsNotFound(err) {
					t.Errorf("error = %v when checking for %s %v deleted in source cluster", err, oFrom.GetKind(), key)
					continue
				}

				// objects are created in the target cluster
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)
				if !node.isGlobal {
					key.Namespace = toNamespace
				}

				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("error = %v when checking for %s %v created in target cluster", err, oFrom.GetKind(), key)
					continue
				}
				if fields, knownKind := updateKnownKinds[oTo.GetKind()]; knownKind {
					for _, nsField := range fields {
						value, exists, err := unstructured.NestedFieldNoCopy(oTo.Object, nsField...)
						g.Expect(err).ToNot(HaveOccurred())
						if exists {
							g.Expect(value).To(Equal(toNamespace))
						}
					}
				}
			}
		})
	}
}

func Test_objectMover_checkProvisioningCompleted(t *testing.T) {
	type fields struct {
		objs []client.Object
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Blocks with a cluster without InfrastructureReady",
			fields: fields{
				objs: []client.Object{
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
							Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(false)},
							Conditions: []metav1.Condition{
								{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Blocks with a cluster without ControlPlaneInitialized",
			fields: fields{
				objs: []client.Object{
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
							Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Blocks with a cluster with ControlPlaneInitialized=False",
			fields: fields{
				objs: []client.Object{
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
							Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
							Conditions: []metav1.Condition{
								{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionFalse},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Blocks with a cluster without ControlPlaneReady",
			fields: fields{
				objs: []client.Object{
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
							ControlPlaneRef: clusterv1.ContractVersionedObjectReference{
								Name: "cp1",
							},
						},
						Status: clusterv1.ClusterStatus{
							Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
							Conditions: []metav1.Condition{
								{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Blocks with a Machine Without NodeRef",
			fields: fields{
				objs: []client.Object{
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
							Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true)},
							Conditions: []metav1.Condition{
								{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue},
							},
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
							// NodeRef is not set
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass",
			fields: fields{
				objs: []client.Object{
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
							Initialization: clusterv1.ClusterInitializationStatus{InfrastructureProvisioned: ptr.To(true), ControlPlaneInitialized: ptr.To(true)},
							Conditions: []metav1.Condition{
								{Type: clusterv1.ClusterControlPlaneInitializedCondition, Status: metav1.ConditionTrue},
							},
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
							NodeRef: clusterv1.MachineNodeReference{
								Name: "machine1",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			o := &objectMover{
				fromProxy: graph.proxy,
			}
			err := o.checkProvisioningCompleted(ctx, graph)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_objectsMoverService_checkTargetProviders(t *testing.T) {
	type fields struct {
		fromProxy Proxy
	}
	type args struct {
		toProxy Proxy
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "all the providers in place (lazy matching)",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi-system").
					WithProviderInventory("kubeadm", clusterctlv1.BootstrapProviderType, "v1.0.0", "cabpk-system").
					WithProviderInventory("capa", clusterctlv1.InfrastructureProviderType, "v1.0.0", "capa-system"),
			},
			args: args{
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi-system").
					WithProviderInventory("kubeadm", clusterctlv1.BootstrapProviderType, "v1.0.0", "cabpk-system").
					WithProviderInventory("capa", clusterctlv1.InfrastructureProviderType, "v1.0.0", "capa-system"),
			},
			wantErr: false,
		},
		{
			name: "all the providers in place but with a newer version (lazy matching)",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system"),
			},
			args: args{
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.1.0", "capi-system"), // Lazy matching
			},
			wantErr: false,
		},
		{
			name: "fails if a provider is missing",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system"),
			},
			args: args{
				toProxy: test.NewFakeProxy(),
			},
			wantErr: true,
		},
		{
			name: "fails if a provider version is older than expected",
			fields: fields{
				fromProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v2.0.0", "capi-system"),
			},
			args: args{
				toProxy: test.NewFakeProxy().
					WithProviderInventory("capi", clusterctlv1.CoreProviderType, "v1.0.0", "capi1-system"),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			o := &objectMover{
				fromProviderInventory: newInventoryClient(tt.fields.fromProxy, nil, currentContractVersion),
			}
			err := o.checkTargetProviders(ctx, newInventoryClient(tt.args.toProxy, nil, currentContractVersion))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_objectMoverService_ensureNamespace(t *testing.T) {
	type args struct {
		toProxy   Proxy
		namespace string
	}

	namespace1 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace-1",
		},
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "ensureNamespace doesn't fail given an existing namespace",
			args: args{
				// Create a fake cluster target with namespace-1 already existing
				toProxy: test.NewFakeProxy().WithObjs(namespace1),
				// Ensure namespace-1 gets created
				namespace: "namespace-1",
			},
		},
		{
			name: "ensureNamespace doesn't fail if the namespace does not already exist in the target",
			args: args{
				// Create a fake empty client
				toProxy: test.NewFakeProxy(),
				// Ensure namespace-2 gets created
				namespace: "namespace-2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			mover := objectMover{
				fromProxy: test.NewFakeProxy(),
			}

			err := mover.ensureNamespace(ctx, tt.args.toProxy, tt.args.namespace)
			g.Expect(err).ToNot(HaveOccurred())

			// Check that the namespaces either existed or were created in the
			// target.
			csTo, err := tt.args.toProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			ns := &corev1.Namespace{}
			key := client.ObjectKey{
				// Search for this namespace
				Name: tt.args.namespace,
			}

			err = csTo.Get(ctx, key, ns)
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func Test_objectMoverService_ensureNamespaces(t *testing.T) {
	type args struct {
		toProxy Proxy
	}
	type fields struct {
		objs []client.Object
	}

	// Create some test runtime objects to be used in the tests
	namespace1 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace-1",
		},
	}

	cluster1 := test.NewFakeCluster("namespace-1", "cluster-1")
	cluster2 := test.NewFakeCluster("namespace-2", "cluster-2")
	cluster3 := test.NewFakeCluster("namespace-1", "cluster-3").WithTopologyClass("cluster-class-1").WithTopologyClassNamespace("namespace-2")
	clusterClass1 := test.NewFakeClusterClass("namespace-2", "cluster-class-1")
	globalObj := test.NewFakeClusterExternalObject("eo-1")

	clustersObjs := append(cluster1.Objs(), cluster2.Objs()...)

	tests := []struct {
		name               string
		fields             fields
		args               args
		expectedNamespaces []string
	}{
		{
			name: "ensureNamespaces doesn't fail given an existing namespace",
			fields: fields{
				objs: cluster1.Objs(),
			},
			args: args{
				toProxy: test.NewFakeProxy(),
			},
			expectedNamespaces: []string{"namespace-1"},
		},
		{
			name: "ensureNamespaces moves namespace-1 and namespace-2 to target",
			fields: fields{
				objs: clustersObjs,
			},
			args: args{
				toProxy: test.NewFakeProxy(),
			},
			expectedNamespaces: []string{"namespace-1", "namespace-2"},
		},
		{
			name: "ensureNamespaces moves namespace-1 and namespace-2 from cross-namespace CC reference",
			fields: fields{
				objs: append(cluster3.Objs(), clusterClass1.Objs()...),
			},
			args: args{
				toProxy: test.NewFakeProxy(),
			},
			expectedNamespaces: []string{"namespace-1", "namespace-2"},
		},
		{
			name: "ensureNamespaces moves namespace-2 to target which already has namespace-1",
			fields: fields{
				objs: cluster2.Objs(),
			},
			args: args{
				toProxy: test.NewFakeProxy().WithObjs(namespace1),
			},
			expectedNamespaces: []string{"namespace-1", "namespace-2"},
		},
		{
			name: "ensureNamespaces doesn't fail if no namespace is specified (cluster-wide)",
			fields: fields{
				objs: globalObj.Objs(),
			},
			args: args{
				toProxy: test.NewFakeProxy(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// Trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			mover := objectMover{
				fromProxy: graph.proxy,
			}

			err := mover.ensureNamespaces(ctx, graph, tt.args.toProxy)
			g.Expect(err).ToNot(HaveOccurred())

			// Check that the namespaces either existed or were created in the
			// target.
			csTo, err := tt.args.toProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			namespaces := &corev1.NamespaceList{}

			err = csTo.List(ctx, namespaces, client.Continue(namespaces.Continue))
			g.Expect(err).ToNot(HaveOccurred())

			// Ensure length of namespaces matches what's expected to ensure we're handling
			// cluster-wide (namespace of "") objects
			g.Expect(namespaces.Items).To(HaveLen(len(tt.expectedNamespaces)))

			// Loop through each expected result to ensure that it is found in
			// the actual results.
			for _, expected := range tt.expectedNamespaces {
				exists := false
				for _, item := range namespaces.Items {
					if item.Name == expected {
						exists = true
					}
				}
				// If at any point a namespace was not found, it must have not
				// been moved to the target successfully.
				if !exists {
					t.Errorf("namespace: %v not found in target cluster", expected)
				}
			}
		})
	}
}

func Test_createTargetObject(t *testing.T) {
	type args struct {
		fromProxy Proxy
		toProxy   Proxy
		node      *node
		mutators  []ResourceMutatorFunc
	}

	tests := []struct {
		name    string
		args    args
		want    func(*WithT, client.Client)
		wantErr bool
	}{
		{
			name: "fails if the object is missing from the source",
			args: args{
				fromProxy: test.NewFakeProxy(),
				toProxy: test.NewFakeProxy().WithObjs(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "ns1",
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Cluster",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersion.String(),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "creates the object with owner references if not exists",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "ns1",
						},
					},
				),
				toProxy: test.NewFakeProxy(),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Cluster",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersion.String(),
					},
					owners: map[*node]ownerReferenceAttributes{
						{
							identity: corev1.ObjectReference{
								Kind:       "Something",
								Namespace:  "ns1",
								Name:       "bar",
								APIVersion: clusterv1.GroupVersion.String(),
							},
						}: {
							Controller: ptr.To(true),
						},
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				ns := &corev1.Namespace{}
				nsKey := client.ObjectKey{
					Name: "ns1",
				}
				g.Expect(toClient.Get(context.Background(), nsKey, ns)).To(Succeed())
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(toClient.Get(context.Background(), key, c)).ToNot(HaveOccurred())
				g.Expect(c.OwnerReferences).To(HaveLen(1))
				g.Expect(c.OwnerReferences[0].Controller).To(Equal(ptr.To(true)))
			},
		},
		{
			name: "updates the object if it already exists and the object is not Global/GlobalHierarchy",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "ns1",
						},
					},
				),
				toProxy: test.NewFakeProxy().WithObjs(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "foo",
							Namespace:   "ns1",
							Annotations: map[string]string{"foo": "bar"},
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Cluster",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersion.String(),
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(toClient.Get(context.Background(), key, c)).ToNot(HaveOccurred())
				g.Expect(c.Annotations).To(BeEmpty())
			},
		},
		{
			name: "updates object whose namespace is mutated, if it already exists and the object is not Global/GlobalHierarchy",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "ns1",
						},
					},
				),
				toProxy: test.NewFakeProxy().WithObjs(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "foo",
							Namespace:   "mutatedns1",
							Annotations: map[string]string{"foo": "bar"},
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Cluster",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersion.String(),
					},
				},
				mutators: []ResourceMutatorFunc{
					func(u *unstructured.Unstructured) error {
						return unstructured.SetNestedField(u.Object,
							"mutatedns1",
							"metadata", "namespace")
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "mutatedns1",
					Name:      "foo",
				}
				g.Expect(toClient.Get(context.Background(), key, c)).ToNot(HaveOccurred())
				g.Expect(c.Annotations).To(BeEmpty())
			},
		},
		{
			name: "should not update Global objects",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&infrastructure.GenericClusterInfrastructureIdentity{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
					},
				),
				toProxy: test.NewFakeProxy().WithObjs(
					&infrastructure.GenericClusterInfrastructureIdentity{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "foo",
							Annotations: map[string]string{"foo": "bar"},
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "GenericClusterInfrastructureIdentity",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersionInfrastructure.String(),
					},
					isGlobal: true,
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &infrastructure.GenericClusterInfrastructureIdentity{}
				key := client.ObjectKey{
					Name: "foo",
				}
				g.Expect(toClient.Get(context.Background(), key, c)).ToNot(HaveOccurred())
				g.Expect(c.Annotations).ToNot(BeEmpty())
			},
		},
		{
			name: "should not update Global Hierarchy objects",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "ns1",
						},
					},
				),
				toProxy: test.NewFakeProxy().WithObjs(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:        "foo",
							Namespace:   "ns1",
							Annotations: map[string]string{"foo": "bar"},
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Secret",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: "v1",
					},
					isGlobalHierarchy: true,
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &corev1.Secret{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(toClient.Get(context.Background(), key, c)).ToNot(HaveOccurred())
				g.Expect(c.Annotations).ToNot(BeEmpty())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			mover := objectMover{
				fromProxy: tt.args.fromProxy,
			}

			err := mover.createTargetObject(ctx, tt.args.node, tt.args.toProxy, tt.args.mutators, sets.New[string]())
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			toClient, err := tt.args.toProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			tt.want(g, toClient)
		})
	}
}

func Test_deleteSourceObject(t *testing.T) {
	type args struct {
		fromProxy Proxy
		node      *node
	}

	tests := []struct {
		name string
		args args
		want func(*WithT, client.Client)
	}{
		{
			name: "no op if the object is already deleted from source",
			args: args{
				fromProxy: test.NewFakeProxy(),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Cluster",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersion.String(),
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(apierrors.IsNotFound(toClient.Get(context.Background(), key, c))).To(BeTrue())
			},
		},
		{
			name: "deletes from source if the object is not is not Global/GlobalHierarchy",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "ns1",
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Cluster",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersion.String(),
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(apierrors.IsNotFound(toClient.Get(context.Background(), key, c))).To(BeTrue())
			},
		},
		{
			name: "does not delete from source if the object is not is Global",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&infrastructure.GenericClusterInfrastructureIdentity{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "GenericClusterInfrastructureIdentity",
						Name:       "foo",
						APIVersion: clusterv1.GroupVersionInfrastructure.String(),
					},
					isGlobal: true,
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(apierrors.IsNotFound(toClient.Get(context.Background(), key, c))).To(BeTrue())
			},
		},
		{
			name: "does not delete from source if the object is not is Global Hierarchy",
			args: args{
				fromProxy: test.NewFakeProxy().WithObjs(
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "ns1",
						},
					},
				),
				node: &node{
					identity: corev1.ObjectReference{
						Kind:       "Secret",
						Namespace:  "ns1",
						Name:       "foo",
						APIVersion: "v1",
					},
					isGlobalHierarchy: true,
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(apierrors.IsNotFound(toClient.Get(context.Background(), key, c))).To(BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx := context.Background()

			mover := objectMover{
				fromProxy: tt.args.fromProxy,
			}

			err := mover.deleteSourceObject(ctx, tt.args.node)
			g.Expect(err).ToNot(HaveOccurred())

			fromClient, err := tt.args.fromProxy.NewClient(ctx)
			g.Expect(err).ToNot(HaveOccurred())

			tt.want(g, fromClient)
		})
	}
}

func TestWaitReadyForMove(t *testing.T) {
	tests := []struct {
		name        string
		moveBlocked bool
		doUnblock   bool
		wantErr     bool
	}{
		{
			name:        "moving blocked cluster should fail",
			moveBlocked: true,
			wantErr:     true,
		},
		{
			name:        "moving unblocked cluster should succeed",
			moveBlocked: false,
			wantErr:     false,
		},
		{
			name:        "moving blocked cluster that is eventually unblocked should succeed",
			moveBlocked: true,
			doUnblock:   true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			clusterName := "foo"
			clusterNamespace := "ns1"
			objs := test.NewFakeCluster(clusterNamespace, clusterName).Objs()

			ctx := context.Background()

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(objs)

			if tt.moveBlocked {
				c, err := graph.proxy.NewClient(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				cluster := &clusterv1.Cluster{}
				err = c.Get(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, cluster)
				g.Expect(err).NotTo(HaveOccurred())
				anns := cluster.GetAnnotations()
				if anns == nil {
					anns = make(map[string]string)
				}
				anns[clusterctlv1.BlockMoveAnnotation] = "anything"
				cluster.SetAnnotations(anns)

				g.Expect(c.Update(ctx, cluster)).To(Succeed())

				if tt.doUnblock {
					go func() {
						time.Sleep(50 * time.Millisecond)
						delete(cluster.Annotations, clusterctlv1.BlockMoveAnnotation)
						g.Expect(c.Update(ctx, cluster)).To(Succeed())
					}()
				}
			}

			// Get all the types to be considered for discovery
			g.Expect(graph.getDiscoveryTypes(ctx)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery(ctx, "")).To(Succeed())

			backoff := wait.Backoff{
				Steps: 1,
			}
			if tt.doUnblock {
				backoff = wait.Backoff{
					Duration: 20 * time.Millisecond,
					Steps:    10,
				}
			}
			err := waitReadyForMove(ctx, graph.proxy, graph.getMoveNodes(), false, backoff)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func Test_applyMutators(t *testing.T) {
	tests := []struct {
		name     string
		object   client.Object
		mutators []ResourceMutatorFunc
		want     *unstructured.Unstructured
		wantErr  bool
	}{
		{
			name: "do nothing if object is nil",
		},
		{
			name:   "do nothing if mutators is a nil slice",
			object: test.NewFakeCluster("example", "example").Objs()[0],
			want: func() *unstructured.Unstructured {
				g := NewWithT(t)
				obj := test.NewFakeCluster("example", "example").Objs()[0]
				u := &unstructured.Unstructured{}
				to, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
				g.Expect(err).NotTo(HaveOccurred())
				u.SetUnstructuredContent(to)
				return u
			}(),
		},
		{
			name:     "return error if any element in mutators slice is nil",
			mutators: []ResourceMutatorFunc{nil},
			object:   test.NewFakeCluster("example", "example").Objs()[0],
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := applyMutators(tt.object, tt.mutators...)
			g.Expect(got).To(Equal(tt.want))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
