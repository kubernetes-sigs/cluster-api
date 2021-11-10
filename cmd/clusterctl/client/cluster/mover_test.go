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
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlv1 "sigs.k8s.io/cluster-api/cmd/clusterctl/api/v1alpha3"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/internal/test/providers/infrastructure"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		name: "Cluster",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Cluster with cloud config secret with the force move label",
		fields: moveTestsFields{
			objs: test.NewFakeCluster("ns1", "foo").WithCloudConfigSecret().Objs(),
		},
		wantMoveGroups: [][]string{
			{ // group 1
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo",
				// objects with force move flag
				"/v1, Kind=Secret, ns1/foo-cloud-config",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m2",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/ms1",
				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/ms1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/ms1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineSets
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m2",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m2",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/md1",
				"cluster.x-k8s.io/v1beta1, Kind=MachineDeployment, ns1/md1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/md1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineDeployments
				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/ms1",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by MachineSets
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m2",
			},
			{ // group 5 (objects with ownerReferences in group 1,2,3,4)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m2",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlane, ns1/cp1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/cp1",
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"/v1, Kind=Secret, ns1/cluster1-sa",
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m1",
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/m2",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m1",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/m2",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/m2",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/mp1",
				"cluster.x-k8s.io/v1beta1, Kind=MachinePool, ns1/mp1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/mp1",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/bar",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
				"/v1, Kind=Secret, ns1/bar-ca",
				"/v1, Kind=Secret, ns1/bar-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/bar",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster2",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/cluster1-ms1",
				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster1-ms1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
				"/v1, Kind=Secret, ns1/cluster2-ca",
				"/v1, Kind=Secret, ns1/cluster2-kubeconfig",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfigTemplate, ns1/cluster2-ms1",
				"cluster.x-k8s.io/v1beta1, Kind=MachineSet, ns1/cluster2-ms1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster2",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachineTemplate, ns1/shared", // shared object
			},
			{ // group 3 (objects with ownerReferences in group 1,2)
				// owned by MachineSets
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster1-m1",
				"cluster.x-k8s.io/v1beta1, Kind=Machine, ns1/cluster2-m1",
			},
			{ // group 4 (objects with ownerReferences in group 1,2,3)
				// owned by Machines
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster1-m1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cluster1-m1",
				"bootstrap.cluster.x-k8s.io/v1beta1, Kind=GenericBootstrapConfig, ns1/cluster2-m1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureMachine, ns1/cluster2-m1",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/cluster1",
				// ClusterResourceSet
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSet, ns1/crs1",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/cluster1-ca",
				"/v1, Kind=Secret, ns1/cluster1-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/cluster1",
				// owned by ClusterResourceSet
				"/v1, Kind=Secret, ns1/resource-s1",
				"/v1, Kind=ConfigMap, ns1/resource-c1",
				// owned by ClusterResourceSet & Cluster
				"addons.cluster.x-k8s.io/v1beta1, Kind=ClusterResourceSetBinding, ns1/cluster1",
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
				"cluster.x-k8s.io/v1beta1, Kind=ClusterClass, ns1/class1",
			},
			{ // group 2
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlaneTemplate, ns1/class1",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo",
			},
			{ // group 3
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
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
				"cluster.x-k8s.io/v1beta1, Kind=ClusterClass, ns1/class1",
				"cluster.x-k8s.io/v1beta1, Kind=ClusterClass, ns1/class2",
			},
			{ // group 2
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlaneTemplate, ns1/class1",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo1",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureClusterTemplate, ns1/class2",
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlaneTemplate, ns1/class2",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo2",
			},
			{ // group 3
				"/v1, Kind=Secret, ns1/foo1-ca",
				"/v1, Kind=Secret, ns1/foo1-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo1",
				"/v1, Kind=Secret, ns1/foo2-ca",
				"/v1, Kind=Secret, ns1/foo2-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo2",
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
				"cluster.x-k8s.io/v1beta1, Kind=ClusterClass, ns1/class1",
			},
			{ // group 2
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlaneTemplate, ns1/class1",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo1",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo2",
			},
			{ // group 3
				"/v1, Kind=Secret, ns1/foo1-ca",
				"/v1, Kind=Secret, ns1/foo1-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo1",
				"/v1, Kind=Secret, ns1/foo2-ca",
				"/v1, Kind=Secret, ns1/foo2-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo2",
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
				"cluster.x-k8s.io/v1beta1, Kind=ClusterClass, ns1/class1",
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo1",
			},
			{ // group 2
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureClusterTemplate, ns1/class1",
				"controlplane.cluster.x-k8s.io/v1beta1, Kind=GenericControlPlaneTemplate, ns1/class1",
				"/v1, Kind=Secret, ns1/foo1-ca",
				"/v1, Kind=Secret, ns1/foo1-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo1",
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
				"external.cluster.x-k8s.io/v1beta1, Kind=GenericExternalObject, ns1/externalObject1",
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
				"external.cluster.x-k8s.io/v1beta1, Kind=GenericClusterExternalObject, /externalObject1",
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
				"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo",
			},
			{ // group 2 (objects with ownerReferences in group 1)
				// owned by Clusters
				"/v1, Kind=Secret, ns1/foo-ca",
				"/v1, Kind=Secret, ns1/foo-credentials",
				"/v1, Kind=Secret, ns1/foo-kubeconfig",
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo",
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
				"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericClusterInfrastructureIdentity, /infra1-identity",
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
			"Cluster_ns1_foo.yaml":                      `{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","metadata":{"creationTimestamp":null,"name":"foo","namespace":"ns1","resourceVersion":"999","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo"},"spec":{"controlPlaneEndpoint":{"host":"","port":0},"infrastructureRef":{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta1","kind":"GenericInfrastructureCluster","name":"foo","namespace":"ns1"}},"status":{"controlPlaneReady":false,"infrastructureReady":false}}` + "\n",
			"Secret_ns1_foo-kubeconfig.yaml":            `{"apiVersion":"v1","kind":"Secret","metadata":{"creationTimestamp":null,"name":"foo-kubeconfig","namespace":"ns1","ownerReferences":[{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","name":"foo","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-kubeconfig"}}` + "\n",
			"Secret_ns1_foo-ca.yaml":                    `{"apiVersion":"v1","kind":"Secret","metadata":{"creationTimestamp":null,"name":"foo-ca","namespace":"ns1","resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-ca"}}` + "\n",
			"GenericInfrastructureCluster_ns1_foo.yaml": `{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta1","kind":"GenericInfrastructureCluster","metadata":{"creationTimestamp":null,"labels":{"cluster.x-k8s.io/cluster-name":"foo"},"name":"foo","namespace":"ns1","ownerReferences":[{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","name":"foo","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo"}}` + "\n",
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
			"Cluster_ns1_foo.yaml":                      `{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","metadata":{"creationTimestamp":null,"name":"foo","namespace":"ns1","resourceVersion":"999","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo"},"spec":{"controlPlaneEndpoint":{"host":"","port":0},"infrastructureRef":{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta1","kind":"GenericInfrastructureCluster","name":"foo","namespace":"ns1"}},"status":{"controlPlaneReady":false,"infrastructureReady":false}}` + "\n",
			"Secret_ns1_foo-kubeconfig.yaml":            `{"apiVersion":"v1","kind":"Secret","metadata":{"creationTimestamp":null,"name":"foo-kubeconfig","namespace":"ns1","ownerReferences":[{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","name":"foo","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-kubeconfig"}}` + "\n",
			"Secret_ns1_foo-ca.yaml":                    `{"apiVersion":"v1","kind":"Secret","metadata":{"creationTimestamp":null,"name":"foo-ca","namespace":"ns1","resourceVersion":"999","uid":"/v1, Kind=Secret, ns1/foo-ca"}}` + "\n",
			"GenericInfrastructureCluster_ns1_foo.yaml": `{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta1","kind":"GenericInfrastructureCluster","metadata":{"creationTimestamp":null,"labels":{"cluster.x-k8s.io/cluster-name":"foo"},"name":"foo","namespace":"ns1","ownerReferences":[{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","name":"foo","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns1/foo"}],"resourceVersion":"999","uid":"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns1/foo"}}` + "\n",
			"Cluster_ns2_bar.yaml":                      `{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","metadata":{"creationTimestamp":null,"name":"bar","namespace":"ns2","resourceVersion":"999","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns2/bar"},"spec":{"controlPlaneEndpoint":{"host":"","port":0},"infrastructureRef":{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta1","kind":"GenericInfrastructureCluster","name":"bar","namespace":"ns2"}},"status":{"controlPlaneReady":false,"infrastructureReady":false}}` + "\n",
			"Secret_ns2_bar-kubeconfig.yaml":            `{"apiVersion":"v1","kind":"Secret","metadata":{"creationTimestamp":null,"name":"bar-kubeconfig","namespace":"ns2","ownerReferences":[{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","name":"bar","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns2/bar"}],"resourceVersion":"999","uid":"/v1, Kind=Secret, ns2/bar-kubeconfig"}}` + "\n",
			"Secret_ns2_bar-ca.yaml":                    `{"apiVersion":"v1","kind":"Secret","metadata":{"creationTimestamp":null,"name":"bar-ca","namespace":"ns2","resourceVersion":"999","uid":"/v1, Kind=Secret, ns2/bar-ca"}}` + "\n",
			"GenericInfrastructureCluster_ns2_bar.yaml": `{"apiVersion":"infrastructure.cluster.x-k8s.io/v1beta1","kind":"GenericInfrastructureCluster","metadata":{"creationTimestamp":null,"labels":{"cluster.x-k8s.io/cluster-name":"bar"},"name":"bar","namespace":"ns2","ownerReferences":[{"apiVersion":"cluster.x-k8s.io/v1beta1","kind":"Cluster","name":"bar","uid":"cluster.x-k8s.io/v1beta1, Kind=Cluster, ns2/bar"}],"resourceVersion":"999","uid":"infrastructure.cluster.x-k8s.io/v1beta1, Kind=GenericInfrastructureCluster, ns2/bar"}}` + "\n",
		},
		wantErr: false,
	},
}

func Test_objectMover_backupTargetObject(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range backupRestoreTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

			// Run backupTargetObject on nodes in graph
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			dir, err := os.MkdirTemp("/tmp", "cluster-api")
			if err != nil {
				t.Error(err)
			}
			defer os.RemoveAll(dir)

			for _, node := range graph.uidToNode {
				err = mover.backupTargetObject(node, dir)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).NotTo(HaveOccurred())

				// objects are stored and serialized correctly in the temporary directory
				expectedFilename := node.getFilename()
				expectedFileContents, ok := tt.files[expectedFilename]
				if !ok {
					t.Errorf("Could not access file map: %v\n", expectedFilename)
				}

				path := filepath.Join(dir, expectedFilename)
				fileContents, err := os.ReadFile(path) //nolint:gosec
				if err != nil {
					g.Expect(err).NotTo(HaveOccurred())
					return
				}

				firstFileStat, err := os.Stat(path)
				if err != nil {
					g.Expect(err).NotTo(HaveOccurred())
					return
				}

				fmt.Printf("Actual file content %v\n", string(fileContents))
				g.Expect(string(fileContents)).To(Equal(expectedFileContents))

				// Add delay so we ensure the file ModTime of updated files is different from old ones in the original files
				time.Sleep(time.Millisecond * 5)

				// Running backupTargetObject should override any existing files since it represents a new backup
				err = mover.backupTargetObject(node, dir)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).NotTo(HaveOccurred())

				secondFileStat, err := os.Stat(path)
				if err != nil {
					g.Expect(err).NotTo(HaveOccurred())
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

			// temporary directory
			dir, err := os.MkdirTemp("/tmp", "cluster-api")
			if err != nil {
				g.Expect(err).NotTo(HaveOccurred())
			}
			defer os.RemoveAll(dir)

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraph()

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run restoreTargetObject
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			// Write go string slice to directory
			for _, file := range tt.files {
				tempFile, err := os.CreateTemp(dir, "obj")
				g.Expect(err).NotTo(HaveOccurred())

				_, err = tempFile.Write([]byte(file))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(tempFile.Close()).To(Succeed())
			}

			objs, err := mover.filesToObjs(dir)
			g.Expect(err).NotTo(HaveOccurred())

			for i := range objs {
				g.Expect(graph.addRestoredObj(&objs[i])).NotTo(HaveOccurred())
			}

			for _, node := range graph.uidToNode {
				err = mover.restoreTargetObject(node, toProxy)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).NotTo(HaveOccurred())

				// Check objects are in new restored cluster
				csTo, err := toProxy.NewClient()
				g.Expect(err).NotTo(HaveOccurred())

				key := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are created in the target cluster
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				if err := csTo.Get(ctx, key, oTo); err != nil {
					t.Errorf("error = %v when checking for %v created in target cluster", err, key)
					continue
				}

				// Re-running restoreTargetObjects won't override existing objects
				err = mover.restoreTargetObject(node, toProxy)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					return
				}

				g.Expect(err).NotTo(HaveOccurred())

				// Check objects are in new restored cluster
				csAfter, err := toProxy.NewClient()
				g.Expect(err).NotTo(HaveOccurred())

				keyAfter := client.ObjectKey{
					Namespace: node.identity.Namespace,
					Name:      node.identity.Name,
				}

				// objects are created in the target cluster
				oAfter := &unstructured.Unstructured{}
				oAfter.SetAPIVersion(node.identity.APIVersion)
				oAfter.SetKind(node.identity.Kind)

				if err := csAfter.Get(ctx, keyAfter, oAfter); err != nil {
					t.Errorf("error = %v when checking for %v created in target cluster", err, key)
					continue
				}

				g.Expect(oAfter.GetAPIVersion()).Should(Equal(oTo.GetAPIVersion()))
				g.Expect(oAfter.GetName()).Should(Equal(oTo.GetName()))
				g.Expect(oAfter.GetCreationTimestamp()).Should(Equal(oTo.GetCreationTimestamp()))
				g.Expect(oAfter.GetUID()).Should(Equal(oTo.GetUID()))
				g.Expect(oAfter.GetOwnerReferences()).Should(Equal(oTo.GetOwnerReferences()))
			}
		})
	}
}

func Test_objectMover_backup(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range backupRestoreTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

			// Run backup
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			dir, err := os.MkdirTemp("/tmp", "cluster-api")
			if err != nil {
				t.Error(err)
			}
			defer os.RemoveAll(dir)

			err = mover.backup(graph, dir)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			// check that the objects are stored in the temporary directory but not deleted from the source cluster
			csFrom, err := graph.proxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

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
				g.Expect(err).NotTo(HaveOccurred())

				// objects are stored in the temporary directory with the expected filename
				files, err := os.ReadDir(dir)
				g.Expect(err).NotTo(HaveOccurred())

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

			dir, err := os.MkdirTemp("/tmp", "cluster-api")
			if err != nil {
				t.Error(err)
			}
			defer os.RemoveAll(dir)

			for _, fileName := range tt.files {
				path := filepath.Join(dir, fileName)
				file, err := os.Create(path)
				if err != nil {
					return
				}

				_, err = file.WriteString(tt.files[fileName])
				g.Expect(err).NotTo(HaveOccurred())
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

			g.Expect(err).NotTo(HaveOccurred())

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

func Test_objectMover_restore(t *testing.T) {
	// NB. we are testing the move and move sequence using the same set of moveTests, but checking the results at different stages of the move process
	for _, tt := range backupRestoreTests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// temporary directory
			dir, err := os.MkdirTemp("/tmp", "cluster-api")
			if err != nil {
				g.Expect(err).NotTo(HaveOccurred())
			}
			defer os.RemoveAll(dir)

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraph()

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run restore
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			// Write go string slice to directory
			for _, file := range tt.files {
				tempFile, err := os.CreateTemp(dir, "obj")
				g.Expect(err).NotTo(HaveOccurred())

				_, err = tempFile.Write([]byte(file))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(tempFile.Close()).To(Succeed())
			}

			objs, err := mover.filesToObjs(dir)
			g.Expect(err).NotTo(HaveOccurred())

			for i := range objs {
				g.Expect(graph.addRestoredObj(&objs[i])).NotTo(HaveOccurred())
			}

			// restore works on the target cluster which does not yet have objs to discover
			// instead set the owners and tenants correctly on object graph like how ObjectMover.Restore does
			// https://github.com/kubernetes-sigs/cluster-api/blob/main/cmd/clusterctl/client/cluster/mover.go#L129-L132
			graph.setSoftOwnership()
			graph.setTenants()
			graph.checkVirtualNode()

			err = mover.restore(graph, toProxy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			// Check objects are in new restored cluster
			csTo, err := toProxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

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
					t.Errorf("error = %v when checking for %v created in target cluster", err, key)
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

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

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

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move
			mover := objectMover{
				fromProxy: graph.proxy,
				dryRun:    true,
			}

			err := mover.move(graph, toProxy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())

			// check that the objects are kept in the source cluster and are not created in the target cluster
			csFrom, err := graph.proxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

			csTo, err := toProxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())
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
					t.Errorf("error = %v when checking for %v kept in source cluster", err, key)
					continue
				}

				// objects are not created in target cluster as it's dry run
				oTo := &unstructured.Unstructured{}
				oTo.SetAPIVersion(node.identity.APIVersion)
				oTo.SetKind(node.identity.Kind)

				err := csTo.Get(ctx, key, oTo)
				if err == nil {
					if oFrom.GetNamespace() != "" {
						t.Errorf("%v created in target cluster which should not", key)
						continue
					}
				} else if !apierrors.IsNotFound(err) {
					t.Errorf("error = %v when checking for %v should not created ojects in target cluster", err, key)
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

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

			// gets a fakeProxy to an empty cluster with all the required CRDs
			toProxy := getFakeProxyWithCRDs()

			// Run move
			mover := objectMover{
				fromProxy: graph.proxy,
			}

			err := mover.move(graph, toProxy)
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
					if !node.isGlobal && !node.isGlobalHierarchy {
						t.Errorf("%v not deleted in source cluster", key)
						continue
					}
				} else if !apierrors.IsNotFound(err) {
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
							InfrastructureReady: false,
							Conditions: clusterv1.Conditions{
								*conditions.TrueCondition(clusterv1.ControlPlaneInitializedCondition),
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
							InfrastructureReady: true,
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
							InfrastructureReady: true,
							Conditions: clusterv1.Conditions{
								*conditions.FalseCondition(clusterv1.ControlPlaneInitializedCondition, "", clusterv1.ConditionSeverityInfo, ""),
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
							ControlPlaneRef: &corev1.ObjectReference{},
						},
						Status: clusterv1.ClusterStatus{
							InfrastructureReady: true,
							Conditions: clusterv1.Conditions{
								*conditions.TrueCondition(clusterv1.ControlPlaneInitializedCondition),
							},
							ControlPlaneReady: false,
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
							InfrastructureReady: true,
							Conditions: clusterv1.Conditions{
								*conditions.TrueCondition(clusterv1.ControlPlaneInitializedCondition),
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
							InfrastructureReady: true,
							Conditions: clusterv1.Conditions{
								*conditions.TrueCondition(clusterv1.ControlPlaneInitializedCondition),
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
			g := NewWithT(t)

			// Create an objectGraph bound a source cluster with all the CRDs for the types involved in the test.
			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

			o := &objectMover{
				fromProxy: graph.proxy,
			}
			err := o.checkProvisioningCompleted(graph)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
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

			o := &objectMover{
				fromProviderInventory: newInventoryClient(tt.fields.fromProxy, nil),
			}
			err := o.checkTargetProviders(newInventoryClient(tt.args.toProxy, nil))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
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

			mover := objectMover{
				fromProxy: test.NewFakeProxy(),
			}

			err := mover.ensureNamespace(tt.args.toProxy, tt.args.namespace)
			g.Expect(err).NotTo(HaveOccurred())

			// Check that the namespaces either existed or were created in the
			// target.
			csTo, err := tt.args.toProxy.NewClient()
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

			graph := getObjectGraphWithObjs(tt.fields.objs)

			// Get all the types to be considered for discovery
			g.Expect(getFakeDiscoveryTypes(graph)).To(Succeed())

			// Trigger discovery the content of the source cluster
			g.Expect(graph.Discovery("")).To(Succeed())

			mover := objectMover{
				fromProxy: graph.proxy,
			}

			err := mover.ensureNamespaces(graph, tt.args.toProxy)
			g.Expect(err).NotTo(HaveOccurred())

			// Check that the namespaces either existed or were created in the
			// target.
			csTo, err := tt.args.toProxy.NewClient()
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
						APIVersion: "cluster.x-k8s.io/v1beta1",
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
						APIVersion: "cluster.x-k8s.io/v1beta1",
					},
					owners: map[*node]ownerReferenceAttributes{
						{
							identity: corev1.ObjectReference{
								Kind:       "Something",
								Namespace:  "ns1",
								Name:       "bar",
								APIVersion: "cluster.x-k8s.io/v1beta1",
							},
						}: {
							Controller: pointer.BoolPtr(true),
						},
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(toClient.Get(ctx, key, c)).ToNot(HaveOccurred())
				g.Expect(c.OwnerReferences).To(HaveLen(1))
				g.Expect(c.OwnerReferences[0].Controller).To(Equal(pointer.BoolPtr(true)))
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
						APIVersion: "cluster.x-k8s.io/v1beta1",
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(toClient.Get(ctx, key, c)).ToNot(HaveOccurred())
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
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
					},
					isGlobal: true,
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &infrastructure.GenericClusterInfrastructureIdentity{}
				key := client.ObjectKey{
					Name: "foo",
				}
				g.Expect(toClient.Get(ctx, key, c)).ToNot(HaveOccurred())
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
				g.Expect(toClient.Get(ctx, key, c)).ToNot(HaveOccurred())
				g.Expect(c.Annotations).ToNot(BeEmpty())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mover := objectMover{
				fromProxy: tt.args.fromProxy,
			}

			err := mover.createTargetObject(tt.args.node, tt.args.toProxy)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			toClient, err := tt.args.toProxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

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
						APIVersion: "cluster.x-k8s.io/v1beta1",
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(apierrors.IsNotFound(toClient.Get(ctx, key, c))).To(BeTrue())
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
						APIVersion: "cluster.x-k8s.io/v1beta1",
					},
				},
			},
			want: func(g *WithT, toClient client.Client) {
				c := &clusterv1.Cluster{}
				key := client.ObjectKey{
					Namespace: "ns1",
					Name:      "foo",
				}
				g.Expect(apierrors.IsNotFound(toClient.Get(ctx, key, c))).To(BeTrue())
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
						APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
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
				g.Expect(apierrors.IsNotFound(toClient.Get(ctx, key, c))).To(BeTrue())
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
				g.Expect(apierrors.IsNotFound(toClient.Get(ctx, key, c))).To(BeTrue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mover := objectMover{
				fromProxy: tt.args.fromProxy,
			}

			err := mover.deleteSourceObject(tt.args.node)
			g.Expect(err).NotTo(HaveOccurred())

			fromClient, err := tt.args.fromProxy.NewClient()
			g.Expect(err).NotTo(HaveOccurred())

			tt.want(g, fromClient)
		})
	}
}
