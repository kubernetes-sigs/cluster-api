/*
Copyright 2021 The Kubernetes Authors.

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
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestGetCurrentState(t *testing.T) {
	crds := []client.Object{
		builder.GenericControlPlaneCRD,
		builder.GenericInfrastructureClusterCRD,
		builder.GenericControlPlaneTemplateCRD,
		builder.GenericInfrastructureClusterTemplateCRD,
		builder.GenericBootstrapConfigTemplateCRD,
		builder.GenericInfrastructureMachineTemplateCRD,
		builder.GenericInfrastructureMachineCRD,
	}

	// The following is a block creating a number of objects for use in the test cases.

	// InfrastructureCluster objects.
	infraCluster := builder.InfrastructureCluster(metav1.NamespaceDefault, "infraOne").
		Build()
	infraCluster.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	infraClusterNotTopologyOwned := builder.InfrastructureCluster(metav1.NamespaceDefault, "infraOne").
		Build()
	infraClusterTemplate := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infraTemplateOne").
		Build()

	// ControlPlane and ControlPlaneInfrastructureMachineTemplate objects.
	controlPlaneInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfraTemplate").
		Build()
	controlPlaneInfrastructureMachineTemplate.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	controlPlaneInfrastructureMachineTemplateNotTopologyOwned := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfraTemplate").
		Build()
	controlPlaneTemplateWithInfrastructureMachine := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cpTemplateWithInfra1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	controlPlaneTemplateWithInfrastructureMachineNotTopologyOwned := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cpTemplateWithInfra1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplateNotTopologyOwned).
		Build()
	controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		Build()
	controlPlane.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	controlPlaneWithInfra := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	controlPlaneWithInfra.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	controlPlaneWithInfraNotTopologyOwned := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplateNotTopologyOwned).
		Build()
	controlPlaneNotTopologyOwned := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		Build()

	// ClusterClass  objects.
	clusterClassWithControlPlaneInfra := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
		WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	clusterClassWithControlPlaneInfraNotTopologyOwned := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachineNotTopologyOwned).
		WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplateNotTopologyOwned).
		Build()
	clusterClassWithNoControlPlaneInfra := builder.ClusterClass(metav1.NamespaceDefault, "class2").
		Build()

	// MachineDeployment and related objects.
	emptyMachineDeployments := make(map[string]*scope.MachineDeploymentState)

	machineDeploymentInfrastructure := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").
		Build()
	machineDeploymentInfrastructure.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	machineDeploymentBootstrap := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").
		Build()
	machineDeploymentBootstrap.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})

	machineDeployment := builder.MachineDeployment(metav1.NamespaceDefault, "md1").
		WithLabels(map[string]string{
			clusterv1.ClusterLabelName:                          "cluster1",
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentLabelName: "md1",
		}).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()
	machineDeployment2 := builder.MachineDeployment(metav1.NamespaceDefault, "md2").
		WithLabels(map[string]string{
			clusterv1.ClusterLabelName:                          "cluster1",
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentLabelName: "md2",
		}).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()

	// MachineHealthChecks for the MachineDeployment and the ControlPlane.
	machineHealthCheckForMachineDeployment := builder.MachineHealthCheck(machineDeployment.Namespace, machineDeployment.Name).
		WithSelector(*selectorForMachineDeploymentMHC(machineDeployment)).
		WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		}).
		WithClusterName("cluster1").
		Build()

	machineHealthCheckForControlPlane := builder.MachineHealthCheck(controlPlane.GetNamespace(), controlPlane.GetName()).
		WithSelector(*selectorForControlPlaneMHC()).
		WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		}).
		WithClusterName("cluster1").
		Build()

	tests := []struct {
		name      string
		cluster   *clusterv1.Cluster
		blueprint *scope.ClusterBlueprint
		objects   []client.Object
		want      *scope.ClusterState
		wantErr   bool
	}{
		{
			name:      "Should read a Cluster when being processed by the topology controller for the first time (without references)",
			cluster:   builder.Cluster(metav1.NamespaceDefault, "cluster1").Build(),
			blueprint: &scope.ClusterBlueprint{},
			// Expecting valid return with no ControlPlane or Infrastructure state defined and empty MachineDeployment state list
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					// No InfrastructureCluster or ControlPlane references!
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Fails if the Cluster references an InfrastructureCluster that does not exist",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				InfrastructureClusterTemplate: infraClusterTemplate,
			},
			objects: []client.Object{
				// InfrastructureCluster is missing!
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name: "Fails if the Cluster references an InfrastructureCluster that is not topology owned",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraClusterNotTopologyOwned).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				InfrastructureClusterTemplate: infraClusterTemplate,
			},
			objects: []client.Object{
				infraClusterNotTopologyOwned,
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name: "Fails if the Cluster references an Control Plane that is not topology owned",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlaneNotTopologyOwned).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplateWithInfrastructureMachine,
				},
			},
			objects: []client.Object{
				controlPlaneNotTopologyOwned,
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name: "Should read  a partial Cluster (with InfrastructureCluster only)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				// No ControlPlane reference!
				Build(),
			blueprint: &scope.ClusterBlueprint{
				InfrastructureClusterTemplate: infraClusterTemplate,
			},
			objects: []client.Object{
				infraCluster,
			},
			// Expecting valid return with no ControlPlane or MachineDeployment state defined but with a valid Infrastructure state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Should read a partial Cluster (with InfrastructureCluster, ControlPlane, but without workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				WithInfrastructureCluster(infraCluster).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithNoControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplateWithInfrastructureMachine,
				},
			},
			objects: []client.Object{
				controlPlane,
				infraCluster,
				clusterClassWithNoControlPlaneInfra,
				// Workers are missing!
			},
			// Expecting valid return with ControlPlane, no ControlPlane Infrastructure state, InfrastructureCluster state and no defined MachineDeployment state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithControlPlane(controlPlane).
					WithInfrastructureCluster(infraCluster).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlane, InfrastructureMachineTemplate: nil},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Fails if the ClusterClass requires InfrastructureMachine for the ControlPlane, but the ControlPlane object does not have a reference for it",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				WithInfrastructureCluster(infraCluster).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplateWithInfrastructureMachine,
				},
			},
			objects: []client.Object{
				controlPlane,
				infraCluster,
				clusterClassWithControlPlaneInfra,
			},
			// Expecting error from ControlPlane having no valid ControlPlane Infrastructure with ClusterClass requiring ControlPlane Infrastructure.
			wantErr: true,
		},
		{
			name: "Should read a partial Cluster (with ControlPlane and ControlPlane InfrastructureMachineTemplate, but without InfrastructureCluster and workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				// No InfrastructureCluster!
				WithControlPlane(controlPlaneWithInfra).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				controlPlaneWithInfra,
				controlPlaneInfrastructureMachineTemplate,
				// Workers are missing!
			},
			// Expecting valid return with valid ControlPlane state, but no ControlPlane Infrastructure, InfrastructureCluster or MachineDeployment state defined.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithControlPlane(controlPlaneWithInfra).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Should read a partial Cluster (with InfrastructureCluster ControlPlane and ControlPlane InfrastructureMachineTemplate, but without workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				// Workers are missing!
			},
			// Expecting valid return with valid ControlPlane state, ControlPlane Infrastructure state and InfrastructureCluster state, but no defined MachineDeployment state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Should read a Cluster (with InfrastructureCluster, ControlPlane and ControlPlane InfrastructureMachineTemplate, workers)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
			},
			// Expecting valid return with valid ControlPlane, ControlPlane Infrastructure and InfrastructureCluster state, but no defined MachineDeployment state.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						Build()).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {Object: machineDeployment, BootstrapTemplate: machineDeploymentBootstrap, InfrastructureMachineTemplate: machineDeploymentInfrastructure}},
			},
		},
		{
			name: "Fails if the ControlPlane references an InfrastructureMachineTemplate that is not topology owned",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				// No InfrastructureCluster!
				WithControlPlane(controlPlaneWithInfraNotTopologyOwned).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfraNotTopologyOwned,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				controlPlaneWithInfraNotTopologyOwned,
				controlPlaneInfrastructureMachineTemplateNotTopologyOwned,
			},
			wantErr: true,
		},
		{
			name: "Fails if the ControlPlane references an InfrastructureMachineTemplate that does not exist",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				controlPlane,
				// InfrastructureMachineTemplate is missing!
			},
			// Expecting error as ClusterClass references ControlPlane Infrastructure, but ControlPlane Infrastructure is missing in the cluster.
			wantErr: true,
		},
		{
			name: "Should ignore unmanaged MachineDeployments and MachineDeployments belonging to other clusters",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				Build(),
			blueprint: &scope.ClusterBlueprint{ClusterClass: clusterClassWithControlPlaneInfra},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				builder.MachineDeployment(metav1.NamespaceDefault, "no-managed-label").
					WithLabels(map[string]string{
						clusterv1.ClusterLabelName: "cluster1",
						// topology.cluster.x-k8s.io/owned label is missing (unmanaged)!
					}).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
				builder.MachineDeployment(metav1.NamespaceDefault, "wrong-cluster-label").
					WithLabels(map[string]string{
						clusterv1.ClusterLabelName:                          "another-cluster",
						clusterv1.ClusterTopologyOwnedLabel:                 "",
						clusterv1.ClusterTopologyMachineDeploymentLabelName: "md1",
					}).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
			},
			// Expect valid return with empty MachineDeployments properly filtered by label.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Fails if there are MachineDeployments without the topology.cluster.x-k8s.io/deployment-name",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				Build(),
			blueprint: &scope.ClusterBlueprint{ClusterClass: clusterClassWithControlPlaneInfra},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				builder.MachineDeployment(metav1.NamespaceDefault, "missing-topology-md-labelName").
					WithLabels(map[string]string{
						clusterv1.ClusterLabelName:          "cluster1",
						clusterv1.ClusterTopologyOwnedLabel: "",
						// topology.cluster.x-k8s.io/deployment-name label is missing!
					}).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
			},
			// Expect error to be thrown as no managed MachineDeployment is reconcilable unless it has a ClusterTopologyMachineDeploymentLabelName.
			wantErr: true,
		},
		{
			name: "Fails if there are MachineDeployments with the same topology.cluster.x-k8s.io/deployment-name",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass: clusterClassWithControlPlaneInfra,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
			},
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				builder.MachineDeployment(metav1.NamespaceDefault, "duplicate-labels").
					WithLabels(machineDeployment.Labels). // Another machine deployment with the same labels.
					WithBootstrapTemplate(machineDeploymentBootstrap).
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
			},
			// Expect error as two MachineDeployments with the same ClusterTopologyOwnedLabel should not exist for one cluster
			wantErr: true,
		},
		{
			name: "Should read a full Cluster (With InfrastructureCluster, ControlPlane and ControlPlane Infrastructure, MachineDeployments)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
			},
			// Expect valid return of full ClusterState with MachineDeployments properly filtered by label.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						Build()).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeployment,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
		},
		{
			name: "Should read a full Cluster, even if a MachineDeployment topology has been deleted and the MachineDeployment still exists",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				machineDeployment2,
			},
			// Expect valid return of full ClusterState with MachineDeployments properly filtered by label.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						Build()).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeployment,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
					"md2": {
						Object:                        machineDeployment2,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
		},
		{
			name: "Fails if a Cluster has a MachineDeployment without the Bootstrap Template ref",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				builder.MachineDeployment(metav1.NamespaceDefault, "no-bootstrap").
					WithLabels(machineDeployment.Labels).
					// No BootstrapTemplate reference!
					WithInfrastructureTemplate(machineDeploymentInfrastructure).
					Build(),
			},
			// Expect error as Bootstrap Template not defined for MachineDeployments relevant to the Cluster.
			wantErr: true,
		},
		{
			name: "Fails if a Cluster has a MachineDeployments without the InfrastructureMachineTemplate ref",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentBootstrap,
				builder.MachineDeployment(metav1.NamespaceDefault, "no-infra").
					WithLabels(machineDeployment.Labels).
					WithBootstrapTemplate(machineDeploymentBootstrap).
					// No InfrastructureMachineTemplate reference!
					Build(),
			},
			// Expect error as Infrastructure Template not defined for MachineDeployment relevant to the Cluster.
			wantErr: true,
		},
		{
			name: "Pass reading a full Cluster with MachineHealthChecks for ControlPlane and MachineDeployment)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				WithTopology(builder.ClusterTopology().
					WithMachineDeployment(clusterv1.MachineDeploymentTopology{
						Class: "mdClass",
						Name:  "md1",
					}).
					Build()).
				Build(),
			blueprint: &scope.ClusterBlueprint{
				ClusterClass:                  clusterClassWithControlPlaneInfra,
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"mdClass": {
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeployment,
				machineHealthCheckForMachineDeployment,
				machineHealthCheckForControlPlane,
			},
			// Expect valid return of full ClusterState with MachineHealthChecks for both ControlPlane and MachineDeployment.
			want: &scope.ClusterState{
				Cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					WithTopology(builder.ClusterTopology().
						WithMachineDeployment(clusterv1.MachineDeploymentTopology{
							Class: "mdClass",
							Name:  "md1",
						}).
						Build()).
					Build(),
				ControlPlane: &scope.ControlPlaneState{
					Object:                        controlPlaneWithInfra,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
					MachineHealthCheck:            machineHealthCheckForControlPlane,
				},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeployment,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
						MachineHealthCheck:            machineHealthCheckForMachineDeployment,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Sets up a scope with a Blueprint.
			s := scope.New(tt.cluster)
			s.Blueprint = tt.blueprint

			// Sets up the fakeClient for the test case.
			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)
			if tt.cluster != nil {
				objs = append(objs, tt.cluster)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			// Calls getCurrentState.
			r := &Reconciler{
				Client:                    fakeClient,
				APIReader:                 fakeClient,
				UnstructuredCachingClient: fakeClient,
				patchHelperFactory:        dryRunPatchHelperFactory(fakeClient),
			}
			got, err := r.getCurrentState(ctx, s)

			// Checks the return error.
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}

			// Use EqualObject where the compared object is passed through the fakeClient. Elsewhere the Equal method is
			// good enough to establish equality.
			g.Expect(got.Cluster).To(EqualObject(tt.want.Cluster, IgnoreAutogeneratedMetadata))
			g.Expect(got.InfrastructureCluster).To(EqualObject(tt.want.InfrastructureCluster))
			g.Expect(got.ControlPlane).To(Equal(tt.want.ControlPlane), cmp.Diff(got.ControlPlane, tt.want.ControlPlane))
			g.Expect(got.MachineDeployments).To(Equal(tt.want.MachineDeployments))
		})
	}
}

func TestAlignRefAPIVersion(t *testing.T) {
	tests := []struct {
		name                     string
		templateFromClusterClass *unstructured.Unstructured
		currentRef               *corev1.ObjectReference
		want                     *corev1.ObjectReference
		wantErr                  bool
	}{
		{
			name: "Error for invalid apiVersion",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"kind":       "DockerCluster",
			}},
			currentRef: &corev1.ObjectReference{
				APIVersion: "invalid/api/version",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			wantErr: true,
		},
		{
			name: "Use apiVersion from ClusterClass: group and kind is the same",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"kind":       "DockerCluster",
			}},
			currentRef: &corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			want: &corev1.ObjectReference{
				// Group & kind is the same => apiVersion is taken from ClusterClass.
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Use apiVersion from currentRef: kind is different",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "bootstrap.cluster.x-k8s.io/v1alpha4",
				"kind":       "DifferentConfigTemplate",
			}},
			currentRef: &corev1.ObjectReference{
				APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:       "KubeadmConfigTemplate",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			want: &corev1.ObjectReference{
				// kind is different => apiVersion is taken from currentRef.
				APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:       "KubeadmConfigTemplate",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Use apiVersion from currentRef: group is different",
			templateFromClusterClass: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "bootstrap2.cluster.x-k8s.io/v1beta1",
				"kind":       "KubeadmConfigTemplate",
			}},
			currentRef: &corev1.ObjectReference{
				APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:       "KubeadmConfigTemplate",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			want: &corev1.ObjectReference{
				// group is different => apiVersion is taken from currentRef.
				APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
				Kind:       "KubeadmConfigTemplate",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := alignRefAPIVersion(tt.templateFromClusterClass, tt.currentRef)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
