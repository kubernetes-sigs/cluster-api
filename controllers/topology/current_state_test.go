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

	"github.com/google/go-cmp/cmp/cmpopts"

	"sigs.k8s.io/cluster-api/internal/testtypes"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetCurrentState(t *testing.T) {
	crds := []client.Object{
		testtypes.GenericControlPlaneCRD,
		testtypes.GenericInfrastructureClusterCRD,
		testtypes.GenericControlPlaneTemplateCRD,
		testtypes.GenericInfrastructureClusterTemplateCRD,
		testtypes.GenericBootstrapConfigTemplateCRD,
		testtypes.GenericInfrastructureMachineTemplateCRD,
		testtypes.GenericInfrastructureMachineCRD,
	}

	// ignoreResourceVersion is an option to pass to cmpopts to ignore this field which is set by the fakeClient
	// TODO: Make composable version of these options in the builder package to reuse these filters across tests.
	ignoreFields := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion")

	// The following is a block creating a number of objects for use in the test cases.

	// InfrastructureCluster objects.
	infraCluster := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infraOne").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	nonExistentInfraCluster := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "does-not-exist").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()

	// ControlPlane and ControlPlaneInfrastructureMachineTemplate objects.
	controlPlaneInfrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "cpInfraTemplate").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	controlPlaneTemplateWithInfrastructureMachine := testtypes.NewControlPlaneTemplateBuilder(metav1.NamespaceDefault, "cpTemplateWithInfra1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	controlPlane := testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "cp1").
		Build()
	controlPlaneWithInfra := testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()

	// ClusterClass  objects.
	clusterClassWithControlPlaneInfra := testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
		WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
		WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()
	clusterClassWithNoControlPlaneInfra := testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class2").
		Build()

	// MachineDeployment and related objects.
	machineDeploymentInfrastructure := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infra1").
		Build()
	machineDeploymentBootstrap := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap1").
		Build()
	labelsInClass := map[string]string{clusterv1.ClusterLabelName: "cluster1", clusterv1.ClusterTopologyOwnedLabel: "", clusterv1.ClusterTopologyMachineDeploymentLabelName: "md1"}
	labelsNotInClass := map[string]string{clusterv1.ClusterLabelName: "non-existent-cluster", clusterv1.ClusterTopologyOwnedLabel: "", clusterv1.ClusterTopologyMachineDeploymentLabelName: "md1"}
	labelsUnmanaged := map[string]string{clusterv1.ClusterLabelName: "cluster1"}
	labelsManagedWithoutDeploymentName := map[string]string{clusterv1.ClusterLabelName: "cluster1", clusterv1.ClusterTopologyOwnedLabel: ""}

	emptyMachineDeployments := make(map[string]*scope.MachineDeploymentState)

	machineDeploymentInCluster := testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, "md1").
		WithLabels(labelsInClass).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()
	duplicateMachineDeploymentInCluster := testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, "duplicate-labels").
		WithLabels(labelsInClass).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()
	machineDeploymentNoBootstrap := testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, "no-bootstrap").
		WithLabels(labelsInClass).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()
	machineDeploymentNoInfrastructure := testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, "no-infra").
		WithLabels(labelsInClass).WithBootstrapTemplate(machineDeploymentBootstrap).
		Build()
	machineDeploymentOutsideCluster := testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, "wrong-cluster-label").
		WithLabels(labelsNotInClass).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()
	machineDeploymentUnmanaged := testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, "no-managed-label").
		WithLabels(labelsUnmanaged).
		WithBootstrapTemplate(machineDeploymentBootstrap).
		WithInfrastructureTemplate(machineDeploymentInfrastructure).
		Build()
	machineDeploymentWithoutDeploymentName := testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, "missing-topology-md-labelName").
		WithLabels(labelsManagedWithoutDeploymentName).
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
			name:    "Cluster exists with no references",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").Build(),
			// Expecting valid return with no ControlPlane or Infrastructure state defined and empty MachineDeployment state list
			want: &scope.ClusterState{
				Cluster:               testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Cluster with non existent Infrastructure reference only",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(nonExistentInfraCluster).
				Build(),
			objects: []client.Object{
				infraCluster,
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name: "Cluster with Infrastructure reference only",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				Build(),
			objects: []client.Object{
				infraCluster,
			},
			// Expecting valid return with no ControlPlane or MachineDeployment state defined but with a valid Infrastructure state.
			want: &scope.ClusterState{
				Cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Cluster with Infrastructure reference and ControlPlane reference, no ControlPlane Infrastructure and a ClusterClass with no Infrastructure requirement",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				WithInfrastructureCluster(infraCluster).
				Build(),
			class: clusterClassWithNoControlPlaneInfra,
			objects: []client.Object{
				controlPlane,
				infraCluster,
				clusterClassWithNoControlPlaneInfra,
			},
			// Expecting valid return with ControlPlane, no ControlPlane Infrastructure state, InfrastructureCluster state and no defined MachineDeployment state.
			want: &scope.ClusterState{
				Cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
					WithControlPlane(controlPlane).
					WithInfrastructureCluster(infraCluster).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlane, InfrastructureMachineTemplate: nil},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Cluster with Infrastructure reference and ControlPlane reference, no ControlPlane Infrastructure and a ClusterClass with an Infrastructure requirement",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
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
			name: "Cluster with ControlPlane reference and with ControlPlane Infrastructure, but no InfrastructureCluster",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlaneWithInfra).
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				controlPlaneWithInfra,
				controlPlaneInfrastructureMachineTemplate,
			},
			// Expecting valid return with valid ControlPlane state, but no ControlPlane Infrastructure, InfrastructureCluster or MachineDeployment state defined.
			want: &scope.ClusterState{
				Cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
					WithControlPlane(controlPlaneWithInfra).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Cluster with InfrastructureCluster reference ControlPlane reference and ControlPlane Infrastructure",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				WithInfrastructureCluster(infraCluster).
				WithControlPlane(controlPlaneWithInfra).
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
			},
			// Expecting valid return with valid ControlPlane state, ControlPlane Infrastructure state and InfrastructureCluster state, but no defined MachineDeployment state.
			want: &scope.ClusterState{
				Cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "Cluster with MachineDeployment state but no other states defined",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeploymentInCluster,
			},
			// Expecting valid return with valid ControlPlane, ControlPlane Infrastructure and InfrastructureCluster state, but no defined MachineDeployment state.
			want: &scope.ClusterState{
				Cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {Object: machineDeploymentInCluster, BootstrapTemplate: machineDeploymentBootstrap, InfrastructureMachineTemplate: machineDeploymentInfrastructure}},
			},
		},
		{
			name: "Class assigning ControlPlane Infrastructure and Cluster with ControlPlane reference but no ControlPlane Infrastructure",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				WithControlPlane(controlPlane).
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				controlPlane,
			},
			// Expecting error as ClusterClass references ControlPlane Infrastructure, but ControlPlane Infrastructure is missing in the cluster.
			wantErr: true,
		},
		{
			name: "Cluster with no linked MachineDeployments, InfrastructureCluster reference, ControlPlane reference and ControlPlane Infrastructure",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentOutsideCluster,
				machineDeploymentUnmanaged,
			},
			// Expect valid return with empty MachineDeployments properly filtered by label.
			want: &scope.ClusterState{
				Cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
					Build(),
				ControlPlane:          &scope.ControlPlaneState{},
				InfrastructureCluster: nil,
				MachineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name: "MachineDeployment with ClusterTopologyOwnedLabel but without correct ClusterTopologyMachineDeploymentLabelName",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentWithoutDeploymentName,
			},
			// Expect error to be thrown as no managed MachineDeployment is reconcilable unless it has a ClusterTopologyMachineDeploymentLabelName.
			wantErr: true,
		},
		{
			name: "Multiple MachineDeployments with the same ClusterTopologyOwnedLabel label",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeploymentInCluster,
				duplicateMachineDeploymentInCluster,
			},
			// Expect error as two MachineDeployments with the same ClusterTopologyOwnedLabel should not exist for one cluster
			wantErr: true,
		},
		{
			name: "Cluster with MachineDeployments, InfrastructureCluster reference, ControlPlane reference and ControlPlane Infrastructure",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
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
				machineDeploymentInCluster,
				machineDeploymentOutsideCluster,
				machineDeploymentUnmanaged,
			},
			// Expect valid return of full ClusterState with MachineDeployments properly filtered by label.
			want: &scope.ClusterState{
				Cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
					WithInfrastructureCluster(infraCluster).
					WithControlPlane(controlPlaneWithInfra).
					Build(),
				ControlPlane:          &scope.ControlPlaneState{Object: controlPlaneWithInfra, InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				InfrastructureCluster: infraCluster,
				MachineDeployments: map[string]*scope.MachineDeploymentState{
					"md1": {
						Object:                        machineDeploymentInCluster,
						BootstrapTemplate:             machineDeploymentBootstrap,
						InfrastructureMachineTemplate: machineDeploymentInfrastructure,
					},
				},
			},
		},
		{
			name: "Cluster with MachineDeployments lacking Bootstrap Template",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentInfrastructure,
				machineDeploymentNoBootstrap,
			},
			// Expect error as Bootstrap Template not defined for MachineDeployments relevant to the Cluster.
			wantErr: true,
		},
		{
			name: "Cluster with MachineDeployments lacking Infrastructure Template",
			cluster: testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
				Build(),
			class: clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
				machineDeploymentBootstrap,
				machineDeploymentNoInfrastructure,
			},
			// Expect error as Infrastructure Template not defined for MachineDeployment relevant to the Cluster.
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

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
			r := &ClusterReconciler{
				Client:                    fakeClient,
				UnstructuredCachingClient: fakeClient,
			}

			s := scope.New(tt.cluster)
			s.Blueprint = &scope.ClusterBlueprint{ClusterClass: tt.class}

			got, err := r.getCurrentState(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			// Expect the Diff resulting from each object comparison to be empty when ignoring ObjectMeta.ResourceVersion
			// This is necessary as the FakeClient adds its own ResourceVersion on object creation.
			g.Expect(cmp.Diff(tt.want.Cluster, got.Cluster, ignoreFields)).To(Equal(""),
				cmp.Diff(tt.want.Cluster, got.Cluster, ignoreFields))
			g.Expect(cmp.Diff(tt.want.MachineDeployments, got.MachineDeployments, ignoreFields)).To(Equal(""),
				cmp.Diff(tt.want.MachineDeployments, got.MachineDeployments, ignoreFields))
			g.Expect(cmp.Diff(tt.want.InfrastructureCluster, got.InfrastructureCluster, ignoreFields)).To(Equal(""),
				cmp.Diff(tt.want.InfrastructureCluster, got.InfrastructureCluster, ignoreFields))
			g.Expect(cmp.Diff(tt.want.ControlPlane, got.ControlPlane, ignoreFields)).To(Equal(""),
				cmp.Diff(tt.want.ControlPlane, got.ControlPlane, ignoreFields))
		})
	}
}
