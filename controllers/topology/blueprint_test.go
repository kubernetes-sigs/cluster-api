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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	. "sigs.k8s.io/cluster-api/internal/testtypes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetBlueprint(t *testing.T) {
	crds := []client.Object{
		GenericInfrastructureClusterTemplateCRD,
		GenericInfrastructureMachineTemplateCRD,
		GenericInfrastructureMachineCRD,
		GenericControlPlaneTemplateCRD,
		GenericBootstrapConfigTemplateCRD,
	}

	infraClusterTemplate := NewInfrastructureClusterTemplateBuilder(metav1.NamespaceDefault, "infraclustertemplate1").
		Build()
	controlPlaneTemplate := NewControlPlaneTemplateBuilder(metav1.NamespaceDefault, "controlplanetemplate1").
		Build()

	controlPlaneInfrastructureMachineTemplate := NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "controlplaneinframachinetemplate1").
		Build()
	controlPlaneTemplateWithInfrastructureMachine := NewControlPlaneTemplateBuilder(metav1.NamespaceDefault, "controlplanetempaltewithinfrastructuremachine1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()

	workerInfrastructureMachineTemplate := NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "workerinframachinetemplate1").
		Build()
	workerBootstrapTemplate := NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "workerbootstraptemplate1").
		Build()
	machineDeployment := NewMachineDeploymentClassBuilder(metav1.NamespaceDefault, "machinedeployment1").
		WithClass("workerclass1").
		WithLabels(map[string]string{"foo": "bar"}).
		WithAnnotations(map[string]string{"a": "b"}).
		WithInfrastructureTemplate(workerInfrastructureMachineTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		Build()
	mds := []clusterv1.MachineDeploymentClass{*machineDeployment}

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
			name: "ClusterClass exists without references",
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "clusterclass1").
				Build(),
			wantErr: true,
		},
		{
			name: "Ref to missing InfraClusterTemplate",
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				Build(),
			wantErr: true,
		},
		{
			name: "Valid ref to InfraClusterTemplate, Ref to missing ControlPlaneTemplate",
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate and ControlPlaneTemplate",
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					Build(),
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
			},
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate and ControlPlaneInfrastructureMachineTemplate",
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplateWithInfrastructureMachine,
				controlPlaneInfrastructureMachineTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplateWithInfrastructureMachine).
					WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
					Build(),
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
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, worker InfrastructureMachineTemplate and BootstrapTemplate",
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClasses(mds).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
				workerBootstrapTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					WithWorkerMachineDeploymentClasses(mds).
					Build(),
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
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClasses(mds).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
			},
			wantErr: true,
		},
		{
			name: "Valid refs to InfraClusterTemplate, ControlPlaneTemplate, worker BootstrapTemplate, Ref to missing InfrastructureMachineTemplate",
			clusterClass: NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClasses(mds).
				Build(),
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

			cluster := NewClusterBuilder(metav1.NamespaceDefault, "cluster1").Build()

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
			// Use EqualObject where an object is created and passed through the fake client. Elsewhere the Equal methoc
			// is enough to establish inequality.
			g.Expect(tt.want.ClusterClass).To(EqualObject(got.ClusterClass, IgnoreAutogeneratedMetadata))
			g.Expect(tt.want.InfrastructureClusterTemplate).To(EqualObject(got.InfrastructureClusterTemplate))
			g.Expect(got.ControlPlane).To(Equal(tt.want.ControlPlane))
			g.Expect(tt.want.MachineDeployments).To(Equal(got.MachineDeployments))
		})
	}
}
