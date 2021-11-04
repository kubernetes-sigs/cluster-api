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

package topology

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/cluster-api/internal/builder"
	. "sigs.k8s.io/cluster-api/internal/matchers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

	// ControlPlane and ControlPlaneInfrastructureMachineTemplate objects.
	controlPlaneInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfraTemplate").
		Build()
	controlPlaneTemplateWithInfrastructureMachine := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cpTemplateWithInfra1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		Build()
	controlPlane.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})
	controlPlaneWithInfra := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	controlPlaneWithInfra.SetLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""})

	controlPlaneNotTopologyOwned := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		Build()

	// ClusterClass  objects.
	clusterClassWithControlPlaneInfra := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
		WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	clusterClassWithNoControlPlaneInfra := builder.ClusterClass(metav1.NamespaceDefault, "class2").
		Build()

	// MachineDeployment and related objects.
	emptyMachineDeployments := make(map[string]*scope.MachineDeploymentState)

	machineDeploymentInfrastructure := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").
		Build()
	machineDeploymentBootstrap := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").
		Build()

	machineDeployment := builder.MachineDeployment(metav1.NamespaceDefault, "md1").
		WithLabels(map[string]string{
			clusterv1.ClusterLabelName:                          "cluster1",
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentLabelName: "md1",
		}).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()

	tests := []struct {
		name    string
		cluster *clusterv1.Cluster
		class   *clusterv1.ClusterClass
		objects []client.Object
		want    *scope.ClusterState
		wantErr bool
	}{
		{
			name:    "Should read a Cluster when being processed by the topology controller for the first time (without references)",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").Build(),
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
			class: clusterClassWithNoControlPlaneInfra,
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
			class: clusterClassWithControlPlaneInfra,
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
			class: clusterClassWithControlPlaneInfra,
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
			class: clusterClassWithControlPlaneInfra,
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
				Build(),
			class: clusterClassWithControlPlaneInfra,
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
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {Object: machineDeployment, BootstrapTemplate: machineDeploymentBootstrap, InfrastructureMachineTemplate: machineDeploymentInfrastructure}},
			},
		},
		{
			name: "Fails if the ControlPlane references an InfrastructureMachineTemplate that does not exist",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				Build(),
			class: clusterClassWithControlPlaneInfra,
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
			class: clusterClassWithControlPlaneInfra,
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
			class: clusterClassWithControlPlaneInfra,
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
				Build(),
			class: clusterClassWithControlPlaneInfra,
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
				Build(),
			class: clusterClassWithControlPlaneInfra,
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
			name: "Fails if a Cluster has a MachineDeployment without the Bootstrap Template ref",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				Build(),
			class: clusterClassWithControlPlaneInfra,
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
				Build(),
			class: clusterClassWithControlPlaneInfra,
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Sets up a scope with a Blueprint.
			s := scope.New(tt.cluster)
			s.Blueprint = &scope.ClusterBlueprint{ClusterClass: tt.class}

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
			r := &ClusterReconciler{
				Client:                    fakeClient,
				APIReader:                 fakeClient,
				UnstructuredCachingClient: fakeClient,
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
			g.Expect(got.ControlPlane).To(Equal(tt.want.ControlPlane))
			g.Expect(got.MachineDeployments).To(Equal(tt.want.MachineDeployments))
		})
	}
}
