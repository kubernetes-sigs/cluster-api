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

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetCurrentState(t *testing.T) {
	fakeScheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)

	crds := []client.Object{
		fakeControlPlaneCRD,
		fakeInfrastuctureClusterCRD,
		fakeControlPlaneTemplateCRD,
		fakeInfrastructureClusterTemplateCRD,
		fakeBootstrapTemplateCRD,
		fakeInfrastructureMachineTemplateCRD,
	}

	// The following is a block creating a number of fake objects for use in the test cases.

	// InfrastructureCluster fake objects.
	infraCluster := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infraOne").Obj()
	nonExistentInfraCluster := newFakeInfrastructureCluster(metav1.NamespaceDefault, "does-not-exist").Obj()

	// ControlPlane and ControlPlaneInfrastructureMachineTemplate fake objects.
	controlPlaneInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfraTemplate").Obj()
	controlPlaneTemplateWithInfrastructureMachine := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "cpTemplateWithInfra1").WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).Obj()
	controlPlane := newFakeControlPlane(metav1.NamespaceDefault, "cp1").Obj()
	controlPlaneWithInfra := newFakeControlPlane(metav1.NamespaceDefault, "cp1").WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).Obj()

	// ClusterClass fake objects.
	clusterClassWithControlPlaneInfra := newFakeClusterClass(metav1.NamespaceDefault, "class1").WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).Obj()
	clusterClassWithNoControlPlaneInfra := newFakeClusterClass(metav1.NamespaceDefault, "class2").Obj()

	// MachineDeployment and related objects.
	machineDeploymentInfrastructure := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Obj()
	machineDeploymentBootstrap := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Obj()
	labelsInClass := map[string]string{clusterv1.ClusterLabelName: "cluster1", clusterv1.ClusterTopologyLabelName: "", clusterv1.ClusterTopologyMachineDeploymentLabelName: "md1"}
	labelsNotInClass := map[string]string{clusterv1.ClusterLabelName: "non-existent-cluster", clusterv1.ClusterTopologyLabelName: "", clusterv1.ClusterTopologyMachineDeploymentLabelName: "md1"}
	labelsUnmanaged := map[string]string{clusterv1.ClusterLabelName: "cluster1"}
	labelsManagedWithoutDeploymentName := map[string]string{clusterv1.ClusterLabelName: "cluster1", clusterv1.ClusterTopologyLabelName: ""}
	machineDeploymentInCluster := newFakeMachineDeployment(metav1.NamespaceDefault, "proper-labels").WithLabels(labelsInClass).WithBootstrapRef(machineDeploymentBootstrap).WithInfraTemplateRef(machineDeploymentInfrastructure).Obj()
	duplicateMachineDeploymentInCluster := newFakeMachineDeployment(metav1.NamespaceDefault, "duplicate-labels").WithLabels(labelsInClass).WithBootstrapRef(machineDeploymentBootstrap).WithInfraTemplateRef(machineDeploymentInfrastructure).Obj()
	machineDeploymentNoBootstrap := newFakeMachineDeployment(metav1.NamespaceDefault, "no-bootstrap").WithLabels(labelsInClass).WithInfraTemplateRef(machineDeploymentInfrastructure).Obj()
	machineDeploymentNoInfrastructure := newFakeMachineDeployment(metav1.NamespaceDefault, "no-infra").WithLabels(labelsInClass).WithBootstrapRef(machineDeploymentBootstrap).Obj()
	machineDeploymentOutsideCluster := newFakeMachineDeployment(metav1.NamespaceDefault, "wrong-cluster-label").WithLabels(labelsNotInClass).WithBootstrapRef(machineDeploymentBootstrap).WithInfraTemplateRef(machineDeploymentInfrastructure).Obj()
	machineDeploymentUnmanaged := newFakeMachineDeployment(metav1.NamespaceDefault, "no-managed-label").WithLabels(labelsUnmanaged).WithBootstrapRef(machineDeploymentBootstrap).WithInfraTemplateRef(machineDeploymentInfrastructure).Obj()
	machineDeploymentWithoutDeploymentName := newFakeMachineDeployment(metav1.NamespaceDefault, "missing-topology-md-labelName").WithLabels(labelsManagedWithoutDeploymentName).WithBootstrapRef(machineDeploymentBootstrap).WithInfraTemplateRef(machineDeploymentInfrastructure).Obj()
	emptyMachineDeployments := make(map[string]machineDeploymentTopologyState)

	tests := []struct {
		name    string
		cluster *clusterv1.Cluster
		class   *clusterv1.ClusterClass
		objects []client.Object
		want    *clusterTopologyState
		wantErr bool
	}{
		{
			name:    "Cluster exists with no references",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
			// Expecting valid return with no ControlPlane or Infrastructure state defined and empty MachineDeployment state list
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
				controlPlane:          controlPlaneTopologyState{},
				infrastructureCluster: nil,
				machineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name:    "Cluster with non existent Infrastructure reference only",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithInfrastructureCluster(nonExistentInfraCluster).Obj(),
			objects: []client.Object{
				infraCluster,
			},
			wantErr: true, // this test fails as partial reconcile is undefined.
		},
		{
			name:    "Cluster with Infrastructure reference only",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithInfrastructureCluster(infraCluster).Obj(),
			objects: []client.Object{
				infraCluster,
			},
			// Expecting valid return with no ControlPlane or MachineDeployment state defined but with a valid Infrastructure state.
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").WithInfrastructureCluster(infraCluster).Obj(),
				controlPlane:          controlPlaneTopologyState{},
				infrastructureCluster: infraCluster,
				machineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name:    "Cluster with Infrastructure reference and ControlPlane reference, no ControlPlane Infrastructure and a ClusterClass with no Infrastructure requirement",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithControlPlane(controlPlane).WithInfrastructureCluster(infraCluster).Obj(),
			class:   clusterClassWithNoControlPlaneInfra,
			objects: []client.Object{
				controlPlane,
				infraCluster,
				clusterClassWithNoControlPlaneInfra,
			},
			// Expecting valid return with ControlPlane, no ControlPlane Infrastructure state, InfrastructureCluster state and no defined MachineDeployment state.
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").WithControlPlane(controlPlane).WithInfrastructureCluster(infraCluster).Obj(),
				controlPlane:          controlPlaneTopologyState{object: controlPlane, infrastructureMachineTemplate: nil},
				infrastructureCluster: infraCluster,
				machineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name:    "Cluster with Infrastructure reference and ControlPlane reference, no ControlPlane Infrastructure and a ClusterClass with an Infrastructure requirement",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithControlPlane(controlPlane).WithInfrastructureCluster(infraCluster).Obj(),
			class:   clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				controlPlane,
				infraCluster,
				clusterClassWithControlPlaneInfra,
			},
			// Expecting error from ControlPlane having no valid ControlPlane Infrastructure with ClusterClass requiring ControlPlane Infrastructure.
			wantErr: true,
		},
		{
			name:    "Cluster with ControlPlane reference and with ControlPlane Infrastructure, but no InfrastructureCluster",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithControlPlane(controlPlaneWithInfra).Obj(),
			class:   clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				controlPlaneWithInfra,
				controlPlaneInfrastructureMachineTemplate,
			},
			// Expecting valid return with valid ControlPlane state, but no ControlPlane Infrastructure, InfrastructureCluster or MachineDeployment state defined.
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").WithControlPlane(controlPlaneWithInfra).Obj(),
				controlPlane:          controlPlaneTopologyState{object: controlPlaneWithInfra, infrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				infrastructureCluster: nil,
				machineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name:    "Cluster with InfrastructureCluster reference ControlPlane reference and ControlPlane Infrastructure",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithInfrastructureCluster(infraCluster).WithControlPlane(controlPlaneWithInfra).Obj(),
			class:   clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				infraCluster,
				clusterClassWithControlPlaneInfra,
				controlPlaneInfrastructureMachineTemplate,
				controlPlaneWithInfra,
			},
			// Expecting valid return with valid ControlPlane state, ControlPlane Infrastructure state and InfrastructureCluster state, but no defined MachineDeployment state.
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").WithInfrastructureCluster(infraCluster).WithControlPlane(controlPlaneWithInfra).Obj(),
				controlPlane:          controlPlaneTopologyState{object: controlPlaneWithInfra, infrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				infrastructureCluster: infraCluster,
				machineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name:    "Cluster with MachineDeployment state but no other states defined",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
			class:   clusterClassWithControlPlaneInfra,
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
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
				controlPlane:          controlPlaneTopologyState{},
				infrastructureCluster: nil,
				machineDeployments: map[string]machineDeploymentTopologyState{
					"md1": {object: machineDeploymentInCluster, bootstrapTemplate: machineDeploymentBootstrap, infrastructureMachineTemplate: machineDeploymentInfrastructure}},
			},
		},
		{
			name:    "Class assigning ControlPlane Infrastructure and Cluster with ControlPlane reference but no ControlPlane Infrastructure",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithControlPlane(controlPlane).Obj(),
			class:   clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				controlPlane,
			},
			// Expecting error as ClusterClass references ControlPlane Infrastructure, but ControlPlane Infrastructure is missing in the cluster.
			wantErr: true,
		},
		{
			name:    "Cluster with no linked MachineDeployments, InfrastructureCluster reference, ControlPlane reference and ControlPlane Infrastructure",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
			class:   clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentOutsideCluster,
				machineDeploymentUnmanaged,
			},
			// Expect valid return with empty MachineDeployments properly filtered by label.
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
				controlPlane:          controlPlaneTopologyState{},
				infrastructureCluster: nil,
				machineDeployments:    emptyMachineDeployments,
			},
		},
		{
			name:    "MachineDeployment with ClusterTopologyLabelName but without correct ClusterTopologyMachineDeploymentLabelName",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
			class:   clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentWithoutDeploymentName,
			},
			// Expect error to be thrown as no managed MachineDeployment is reconcilable unless it has a ClusterTopologyMachineDeploymentLabelName.
			wantErr: true,
		},
		{
			name:    "Multiple MachineDeployments with the same ClusterTopologyLabelName label",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
			class:   clusterClassWithControlPlaneInfra,
			objects: []client.Object{
				clusterClassWithControlPlaneInfra,
				machineDeploymentInfrastructure,
				machineDeploymentBootstrap,
				machineDeploymentInCluster,
				duplicateMachineDeploymentInCluster,
			},
			// Expect error as two MachineDeployments with the same ClusterTopologyLabelName should not exist for one cluster
			wantErr: true,
		},
		{
			name:    "Cluster with MachineDeployments, InfrastructureCluster reference, ControlPlane reference and ControlPlane Infrastructure",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").WithInfrastructureCluster(infraCluster).WithControlPlane(controlPlaneWithInfra).Obj(),
			class:   clusterClassWithControlPlaneInfra,
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
			// Expect valid return of full clusterTopologyState with MachineDeployments properly filtered by label.
			want: &clusterTopologyState{
				cluster:               newFakeCluster(metav1.NamespaceDefault, "cluster1").WithInfrastructureCluster(infraCluster).WithControlPlane(controlPlaneWithInfra).Obj(),
				controlPlane:          controlPlaneTopologyState{object: controlPlaneWithInfra, infrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate},
				infrastructureCluster: infraCluster,
				machineDeployments: map[string]machineDeploymentTopologyState{
					"md1": {object: machineDeploymentInCluster, bootstrapTemplate: machineDeploymentBootstrap, infrastructureMachineTemplate: machineDeploymentInfrastructure}},
			},
		},
		{
			name:    "Cluster with MachineDeployments lacking Bootstrap Template",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
			class:   clusterClassWithControlPlaneInfra,
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
			name:    "Cluster with MachineDeployments lacking Infrastructure Template",
			cluster: newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj(),
			class:   clusterClassWithControlPlaneInfra,
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
			got, err := r.getCurrentState(ctx, tt.cluster, tt.class)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got.cluster).To(Equal(tt.want.cluster), cmp.Diff(tt.want.cluster, got.cluster))
			g.Expect(got.infrastructureCluster).To(Equal(tt.want.infrastructureCluster), cmp.Diff(tt.want.infrastructureCluster, got.infrastructureCluster))
			g.Expect(got.controlPlane).To(Equal(tt.want.controlPlane), cmp.Diff(tt.want.controlPlane, got.controlPlane, cmp.AllowUnexported(unstructured.Unstructured{}, controlPlaneTopologyState{})))
			g.Expect(got.machineDeployments).To(Equal(tt.want.machineDeployments), cmp.Diff(tt.want.machineDeployments, got.machineDeployments, cmp.AllowUnexported(machineDeploymentTopologyState{})))
		})
	}
}
