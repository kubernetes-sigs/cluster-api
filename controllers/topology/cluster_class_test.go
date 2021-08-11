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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetClass(t *testing.T) {
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

	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		objects      []client.Object
		want         *clusterTopologyClass
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
			want: &clusterTopologyClass{
				clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					Obj(),
				infrastructureClusterTemplate: infraClusterTemplate,
				controlPlane: controlPlaneTopologyClass{
					template: controlPlaneTemplate,
				},
				machineDeployments: map[string]machineDeploymentTopologyClass{},
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
			want: &clusterTopologyClass{
				clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
					WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
					Obj(),
				infrastructureClusterTemplate: infraClusterTemplate,
				controlPlane: controlPlaneTopologyClass{
					template:                      controlPlaneTemplateWithInfrastructureMachine,
					infrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
				},
				machineDeployments: map[string]machineDeploymentTopologyClass{},
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
				WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
				Obj(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
				workerBootstrapTemplate,
			},
			want: &clusterTopologyClass{
				clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
					Obj(),
				infrastructureClusterTemplate: infraClusterTemplate,
				controlPlane: controlPlaneTopologyClass{
					template: controlPlaneTemplate,
				},
				machineDeployments: map[string]machineDeploymentTopologyClass{
					"workerclass1": {
						infrastructureMachineTemplate: workerInfrastructureMachineTemplate,
						bootstrapTemplate:             workerBootstrapTemplate,
					},
				},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, InfrastructureMachineTemplate, Ref to missing BootstrapTemplate",
			clusterClass: newFakeClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
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
				WithWorkerMachineDeploymentTemplates("workerclass1", workerInfrastructureMachineTemplate, workerBootstrapTemplate).
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
			got, err := r.getClass(ctx, cluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}

			g.Expect(got.clusterClass).To(Equal(tt.want.clusterClass), cmp.Diff(tt.want.clusterClass, got.clusterClass))
			g.Expect(got.infrastructureClusterTemplate).To(Equal(tt.want.infrastructureClusterTemplate), cmp.Diff(tt.want.infrastructureClusterTemplate, got.infrastructureClusterTemplate))
			g.Expect(got.controlPlane).To(Equal(tt.want.controlPlane), cmp.Diff(tt.want.controlPlane, got.controlPlane, cmp.AllowUnexported(unstructured.Unstructured{}, controlPlaneTopologyClass{})))
			g.Expect(got.machineDeployments).To(Equal(tt.want.machineDeployments), cmp.Diff(tt.want.machineDeployments, got.machineDeployments, cmp.AllowUnexported(machineDeploymentTopologyClass{})))
		})
	}
}
