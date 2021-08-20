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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetBlueprint(t *testing.T) {
	crds := []client.Object{
		fakeInfrastructureClusterTemplateCRD,
		fakeControlPlaneTemplateCRD,
		fakeInfrastructureMachineTemplateCRD,
		fakeBootstrapTemplateCRD,
	}

	infraClusterTemplate := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "infraclustertemplate1").Obj()
	controlPlaneTemplate := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetemplate1").Obj()

	controlPlaneInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "controlplaneinframachinetemplate1").Obj()
	controlPlaneTemplateWithInfrastructureMachine := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetempaltewithinfrastructuremachine1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Obj()

	workerInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "workerinframachinetemplate1").Obj()
	workerBootstrapTemplate := newFakeBootstrapTemplate(metav1.NamespaceDefault, "workerbootstraptemplate1").Obj()

	var (
		labels      map[string]string
		annotations map[string]string
	)

	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		objects      []client.Object
		want         *scope.ClusterBlueprint
		wantErr      bool
	}{
		{
			name:    "ClusterClass does not exist",
			wantErr: true,
		},
		{
			name:         "ClusterClass exists without references",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "clusterclass1").Obj(),
			wantErr:      true,
		},
		{
			name: "Ref to missing InfraClusterTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				Obj(),
			wantErr: true,
		},
		{
			name: "Valid ref to InfraClusterTemplate, Ref to missing ControlPlaneTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate and ControlPlaneTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					Obj(),
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate and ControlPlaneInfrastructureMachineTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplateWithInfrastructureMachine,
				controlPlaneInfrastructureMachineTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
					WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
					Obj(),
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplateWithInfrastructureMachine,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, Ref to missing ControlPlaneInfrastructureMachineTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, worker InfrastructureMachineTemplate and BootstrapTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClass("workerclass1", map[string]string{"foo": "bar"}, map[string]string{"a": "b"}, workerInfrastructureMachineTemplate, workerBootstrapTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
				workerBootstrapTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					WithWorkerMachineDeploymentClass("workerclass1", map[string]string{"foo": "bar"}, map[string]string{"a": "b"}, workerInfrastructureMachineTemplate, workerBootstrapTemplate).
					Obj(),
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
					"workerclass1": {
						Metadata: clusterv1.ObjectMeta{
							Labels:      map[string]string{"foo": "bar"},
							Annotations: map[string]string{"a": "b"},
						},
						InfrastructureMachineTemplate: workerInfrastructureMachineTemplate,
						BootstrapTemplate:             workerBootstrapTemplate,
					},
				},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, InfrastructureMachineTemplate, Ref to missing BootstrapTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClass("workerclass1", labels, annotations, workerInfrastructureMachineTemplate, workerBootstrapTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, worker BootstrapTemplate, Ref to missing InfrastructureMachineTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClass("workerclass1", labels, annotations, workerInfrastructureMachineTemplate, workerBootstrapTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerBootstrapTemplate,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)

			cluster := newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj()

			if tt.clusterClass != nil {
				cluster.Spec.Topology = &clusterv1.Topology{
					Class: tt.clusterClass.Name,
				}
				objs = append(objs, tt.clusterClass)
			} else {
				cluster.Spec.Topology = &clusterv1.Topology{
					Class: "foo",
				}
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			r := &ClusterReconciler{
				Client:                    fakeClient,
				UnstructuredCachingClient: fakeClient,
			}
			got, err := r.getBlueprint(ctx, scope.New(cluster).Current.Cluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}

			g.Expect(got.ClusterClass).To(Equal(tt.want.ClusterClass), cmp.Diff(tt.want.ClusterClass, got.ClusterClass))
			g.Expect(got.InfrastructureClusterTemplate).To(Equal(tt.want.InfrastructureClusterTemplate), cmp.Diff(tt.want.InfrastructureClusterTemplate, got.InfrastructureClusterTemplate))
			g.Expect(got.ControlPlane).To(Equal(tt.want.ControlPlane), cmp.Diff(tt.want.ControlPlane, got.ControlPlane, cmp.AllowUnexported(unstructured.Unstructured{}, scope.ControlPlaneBlueprint{})))
			g.Expect(got.MachineDeployments).To(Equal(tt.want.MachineDeployments), cmp.Diff(tt.want.MachineDeployments, got.MachineDeployments, cmp.AllowUnexported(scope.MachineDeploymentBlueprint{})))
		})
	}
}
