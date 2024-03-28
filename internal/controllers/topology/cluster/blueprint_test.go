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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestGetBlueprint(t *testing.T) {
	crds := []client.Object{
		builder.GenericInfrastructureClusterTemplateCRD,
		builder.GenericInfrastructureMachineTemplateCRD,
		builder.GenericInfrastructureMachineCRD,
		builder.GenericInfrastructureMachinePoolTemplateCRD,
		builder.GenericInfrastructureMachinePoolCRD,
		builder.GenericControlPlaneTemplateCRD,
		builder.GenericBootstrapConfigTemplateCRD,
	}

	// The following is a block creating a number of objects for use in the test cases.

	infraClusterTemplate := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infraclustertemplate1").
		Build()
	controlPlaneTemplate := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetemplate1").
		Build()

	controlPlaneInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "controlplaneinframachinetemplate1").
		Build()
	controlPlaneTemplateWithInfrastructureMachine := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "controlplanetempaltewithinfrastructuremachine1").
		WithInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
		Build()

	workerInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "workerinframachinetemplate1").
		Build()
	workerInfrastructureMachinePoolTemplate := builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "workerinframachinepooltemplate1").
		Build()
	workerBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "workerbootstraptemplate1").
		Build()
	machineHealthCheck := &clusterv1.MachineHealthCheckClass{
		NodeStartupTimeout: &metav1.Duration{
			Duration: time.Duration(1)},
	}

	machineDeployment := builder.MachineDeploymentClass("workerclass1").
		WithLabels(map[string]string{"foo": "bar"}).
		WithAnnotations(map[string]string{"a": "b"}).
		WithInfrastructureTemplate(workerInfrastructureMachineTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		WithMachineHealthCheckClass(machineHealthCheck).
		Build()

	mds := []clusterv1.MachineDeploymentClass{*machineDeployment}

	machinePools := builder.MachinePoolClass("workerclass2").
		WithLabels(map[string]string{"foo": "bar"}).
		WithAnnotations(map[string]string{"a": "b"}).
		WithInfrastructureTemplate(workerInfrastructureMachinePoolTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		Build()
	mps := []clusterv1.MachinePoolClass{*machinePools}

	// Define test cases.
	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		objects      []client.Object
		want         *scope.ClusterBlueprint
		wantErr      bool
	}{
		{
			name: "Fails if ClusterClass does not have reference to the InfrastructureClusterTemplate",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				// No InfrastructureClusterTemplate reference!
				Build(),
			wantErr: true,
		},
		{
			name: "Fails if ClusterClass references an InfrastructureClusterTemplate that does not exist",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				Build(),
			objects: []client.Object{
				// infraClusterTemplate is missing!
			},
			wantErr: true,
		},
		{
			name: "Fails if ClusterClass does not have reference to the ControlPlaneTemplate",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				// No ControlPlaneTemplate reference!
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
			},
			wantErr: true,
		},
		{
			name: "Fails if ClusterClass does not have reference to the ControlPlaneTemplate",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				// ControlPlaneTemplate is missing!
			},
			wantErr: true,
		},
		{
			name: "Should read a ClusterClass without worker classes",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					Build(),
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
				MachinePools:       map[string]*scope.MachinePoolBlueprint{},
			},
		},
		{
			name: "Should read a ClusterClass referencing an InfrastructureMachineTemplate for the ControlPlane (but without any worker class)",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
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
				MachinePools:       map[string]*scope.MachinePoolBlueprint{},
			},
		},
		{
			name: "Fails if ClusterClass references an InfrastructureMachineTemplate for the ControlPlane that does not exist",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				// controlPlaneInfrastructureMachineTemplate is missing!
			},
			wantErr: true,
		},
		{
			name: "Should read a ClusterClass with a MachineDeploymentClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClasses(mds...).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
				workerBootstrapTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					WithWorkerMachineDeploymentClasses(mds...).
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
						MachineHealthCheck:            machineHealthCheck,
					},
				},
				MachinePools: map[string]*scope.MachinePoolBlueprint{},
			},
		},
		{
			name: "Fails if ClusterClass has a MachineDeploymentClass referencing a BootstrapConfigTemplate that does not exist",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClasses(mds...).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachineTemplate,
				// workerBootstrapTemplate is missing!
			},
			wantErr: true,
		},
		{
			name: "Fails if ClusterClass has a MachineDeploymentClass referencing a InfrastructureMachineTemplate that does not exist",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachineDeploymentClasses(mds...).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerBootstrapTemplate,
				// workerInfrastructureTemplate is missing!
			},
			wantErr: true,
		},
		{
			name: "Should read a ClusterClass with a MachineHealthCheck in the ControlPlane",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
				WithControlPlaneMachineHealthCheck(machineHealthCheck).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				controlPlaneInfrastructureMachineTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					WithControlPlaneInfrastructureMachineTemplate(controlPlaneInfrastructureMachineTemplate).
					WithControlPlaneMachineHealthCheck(machineHealthCheck).
					Build(),
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template:                      controlPlaneTemplate,
					InfrastructureMachineTemplate: controlPlaneInfrastructureMachineTemplate,
					MachineHealthCheck:            machineHealthCheck,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
				MachinePools:       map[string]*scope.MachinePoolBlueprint{},
			},
		},
		{
			name: "Should read a ClusterClass with a MachinePoolClass",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachinePoolClasses(mps...).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachinePoolTemplate,
				workerBootstrapTemplate,
			},
			want: &scope.ClusterBlueprint{
				ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
					WithInfrastructureClusterTemplate(infraClusterTemplate).
					WithControlPlaneTemplate(controlPlaneTemplate).
					WithWorkerMachinePoolClasses(mps...).
					Build(),
				InfrastructureClusterTemplate: infraClusterTemplate,
				ControlPlane: &scope.ControlPlaneBlueprint{
					Template: controlPlaneTemplate,
				},
				MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{},
				MachinePools: map[string]*scope.MachinePoolBlueprint{
					"workerclass2": {
						Metadata: clusterv1.ObjectMeta{
							Labels:      map[string]string{"foo": "bar"},
							Annotations: map[string]string{"a": "b"},
						},
						InfrastructureMachinePoolTemplate: workerInfrastructureMachinePoolTemplate,
						BootstrapTemplate:                 workerBootstrapTemplate,
					},
				},
			},
		},
		{
			name: "Fails if ClusterClass has a MachinePoolClass referencing a BootstrapConfigTemplate that does not exist",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachinePoolClasses(mps...).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerInfrastructureMachinePoolTemplate,
				// workerBootstrapTemplate is missing!
			},
			wantErr: true,
		},
		{
			name: "Fails if ClusterClass has a MachinePoolClass referencing a InfrastructureMachinePoolTemplate that does not exist",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(infraClusterTemplate).
				WithControlPlaneTemplate(controlPlaneTemplate).
				WithWorkerMachinePoolClasses(mps...).
				Build(),
			objects: []client.Object{
				infraClusterTemplate,
				controlPlaneTemplate,
				workerBootstrapTemplate,
				// workerInfrastructureMachinePoolTemplate is missing!
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Set up a cluster using the ClusterClass, if any.
			cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("class1").
						Build()).
				Build()

			// If no clusterClass is defined in the test case fill in a dummy value "foo".
			if tt.clusterClass == nil {
				cluster.Spec.Topology.Class = "foo"
			}

			// Sets up the fakeClient for the test case.
			objs := []client.Object{}
			objs = append(objs, crds...)
			objs = append(objs, tt.objects...)
			if tt.clusterClass != nil {
				objs = append(objs, tt.clusterClass)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(objs...).
				Build()

			// Calls getBlueprint.
			r := &Reconciler{
				Client:                    fakeClient,
				patchHelperFactory:        dryRunPatchHelperFactory(fakeClient),
				UnstructuredCachingClient: fakeClient,
			}
			got, err := r.getBlueprint(ctx, scope.New(cluster).Current.Cluster, tt.clusterClass)

			// Checks the return error.
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.want == nil {
				g.Expect(got).To(BeNil())
				return
			}

			// Use EqualObject where an object is created and passed through the fake client. Elsewhere the Equal method
			// is enough to establish inequality.
			g.Expect(tt.want.ClusterClass).To(EqualObject(got.ClusterClass, IgnoreAutogeneratedMetadata))
			g.Expect(tt.want.InfrastructureClusterTemplate).To(EqualObject(got.InfrastructureClusterTemplate), cmp.Diff(got.InfrastructureClusterTemplate, tt.want.InfrastructureClusterTemplate))
			g.Expect(got.ControlPlane).To(BeComparableTo(tt.want.ControlPlane), cmp.Diff(got.ControlPlane, tt.want.ControlPlane))
			g.Expect(tt.want.MachineDeployments).To(BeComparableTo(got.MachineDeployments), cmp.Diff(got.MachineDeployments, tt.want.MachineDeployments))
			g.Expect(tt.want.MachinePools).To(BeComparableTo(got.MachinePools), cmp.Diff(got.MachinePools, tt.want.MachinePools))
		})
	}
}
