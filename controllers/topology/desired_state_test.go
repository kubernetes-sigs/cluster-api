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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/cluster-api/internal/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	fakeRef1 = &corev1.ObjectReference{
		Kind:       "refKind1",
		Namespace:  "refNamespace1",
		Name:       "refName1",
		APIVersion: "refAPIVersion1",
	}

	fakeRef2 = &corev1.ObjectReference{
		Kind:       "refKind2",
		Namespace:  "refNamespace2",
		Name:       "refName2",
		APIVersion: "refAPIVersion2",
	}
)

func TestComputeInfrastructureCluster(t *testing.T) {
	// templates and ClusterClass
	infrastructureClusterTemplate := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "template1").
		Build()
	clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate).
		Build()

	// aggregating templates and cluster class into a blueprint (simulating getBlueprint)
	blueprint := &scope.ClusterBlueprint{
		ClusterClass:                  clusterClass,
		InfrastructureClusterTemplate: infrastructureClusterTemplate,
	}

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	t.Run("Generates the infrastructureCluster from the template", func(t *testing.T) {
		g := NewWithT(t)

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		obj, err := computeInfrastructureCluster(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     scope.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.Infrastructure.Ref,
			template:    blueprint.InfrastructureClusterTemplate,
			labels:      nil,
			annotations: nil,
			currentRef:  nil,
			obj:         obj,
		})

		// Ensure no ownership is added to generated InfrastructureCluster.
		g.Expect(obj.GetOwnerReferences()).To(HaveLen(0))
	})
	t.Run("If there is already a reference to the infrastructureCluster, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		clusterWithInfrastructureRef := cluster.DeepCopy()
		clusterWithInfrastructureRef.Spec.InfrastructureRef = fakeRef1

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(clusterWithInfrastructureRef)
		scope.Blueprint = blueprint

		obj, err := computeInfrastructureCluster(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     scope.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.Infrastructure.Ref,
			template:    blueprint.InfrastructureClusterTemplate,
			labels:      nil,
			annotations: nil,
			currentRef:  scope.Current.Cluster.Spec.InfrastructureRef,
			obj:         obj,
		})
	})
}

func TestComputeControlPlaneInfrastructureMachineTemplate(t *testing.T) {
	// templates and ClusterClass
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{
				ControlPlane: clusterv1.ControlPlaneTopology{
					Metadata: clusterv1.ObjectMeta{
						Labels:      map[string]string{"l2": ""},
						Annotations: map[string]string{"a2": ""},
					},
				},
			},
		},
	}

	infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").
		Build()
	clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneMetadata(labels, annotations).
		WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).Build()

	// aggregating templates and cluster class into a blueprint (simulating getBlueprint)
	blueprint := &scope.ClusterBlueprint{
		Topology:     cluster.Spec.Topology,
		ClusterClass: clusterClass,
		ControlPlane: &scope.ControlPlaneBlueprint{
			InfrastructureMachineTemplate: infrastructureMachineTemplate,
		},
	}

	t.Run("Generates the infrastructureMachineTemplate from the template", func(t *testing.T) {
		g := NewWithT(t)

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		obj, err := computeControlPlaneInfrastructureMachineTemplate(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     scope.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref,
			template:    blueprint.ControlPlane.InfrastructureMachineTemplate,
			currentRef:  nil,
			obj:         obj,
		})

		// Ensure Cluster ownership is added to generated InfrastructureCluster.
		g.Expect(obj.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(obj.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(obj.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))
	})
	t.Run("If there is already a reference to the infrastructureMachineTemplate, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		currentInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cluster1-template1").Build()

		controlPlane := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Set(controlPlane, currentInfrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		s := scope.New(cluster)
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object:                        controlPlane,
			InfrastructureMachineTemplate: currentInfrastructureMachineTemplate,
		}
		s.Blueprint = blueprint

		obj, err := computeControlPlaneInfrastructureMachineTemplate(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     s.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.Ref,
			template:    blueprint.ControlPlane.InfrastructureMachineTemplate,
			currentRef:  contract.ObjToRef(currentInfrastructureMachineTemplate),
			obj:         obj,
		})
	})
}

func TestComputeControlPlane(t *testing.T) {
	// templates and ClusterClass
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	controlPlaneTemplate := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "template1").
		Build()
	clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneMetadata(labels, annotations).
		WithControlPlaneTemplate(controlPlaneTemplate).Build()
	//TODO: Replace with object builder.
	// current cluster objects
	version := "v1.21.2"
	replicas := int32(3)
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{
				Version: version,
				ControlPlane: clusterv1.ControlPlaneTopology{
					Metadata: clusterv1.ObjectMeta{
						Labels:      map[string]string{"l2": ""},
						Annotations: map[string]string{"a2": ""},
					},
					Replicas: &replicas,
				},
			},
		},
	}

	t.Run("Generates the ControlPlane from the template", func(t *testing.T) {
		g := NewWithT(t)

		blueprint := &scope.ClusterBlueprint{
			Topology:     cluster.Spec.Topology,
			ClusterClass: clusterClass,
			ControlPlane: &scope.ControlPlaneBlueprint{
				Template: controlPlaneTemplate,
			},
		}

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		obj, err := computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     scope.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.ControlPlane.Ref,
			template:    blueprint.ControlPlane.Template,
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedField(g, obj, int64(replicas), contract.ControlPlane().Replicas().Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().MachineTemplate().InfrastructureRef().Path()...)

		// Ensure no ownership is added to generated ControlPlane.
		g.Expect(obj.GetOwnerReferences()).To(HaveLen(0))
	})
	t.Run("Skips setting replicas if required", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects
		clusterWithoutReplicas := cluster.DeepCopy()
		clusterWithoutReplicas.Spec.Topology.ControlPlane.Replicas = nil

		blueprint := &scope.ClusterBlueprint{
			Topology:     clusterWithoutReplicas.Spec.Topology,
			ClusterClass: clusterClass,
			ControlPlane: &scope.ControlPlaneBlueprint{
				Template: controlPlaneTemplate,
			},
		}

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(clusterWithoutReplicas)
		scope.Blueprint = blueprint

		obj, err := computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     scope.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.ControlPlane.Ref,
			template:    blueprint.ControlPlane.Template,
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().Replicas().Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().MachineTemplate().InfrastructureRef().Path()...)
	})
	t.Run("Generates the ControlPlane from the template and adds the infrastructure machine template if required", func(t *testing.T) {
		g := NewWithT(t)

		// templates and ClusterClass
		infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").Build()
		clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
			WithControlPlaneMetadata(labels, annotations).
			WithControlPlaneTemplate(controlPlaneTemplate).
			WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).Build()

		// aggregating templates and cluster class into a blueprint (simulating getBlueprint)
		blueprint := &scope.ClusterBlueprint{
			Topology:     cluster.Spec.Topology,
			ClusterClass: clusterClass,
			ControlPlane: &scope.ControlPlaneBlueprint{
				Template:                      controlPlaneTemplate,
				InfrastructureMachineTemplate: infrastructureMachineTemplate,
			},
		}

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		obj, err := computeControlPlane(ctx, scope, infrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     scope.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.ControlPlane.Ref,
			template:    blueprint.ControlPlane.Template,
			currentRef:  nil,
			obj:         obj,
		})
		gotMetadata, err := contract.ControlPlane().MachineTemplate().Metadata().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())

		expectedLabels := mergeMap(scope.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels)
		expectedLabels[clusterv1.ClusterLabelName] = cluster.Name
		expectedLabels[clusterv1.ClusterTopologyOwnedLabel] = ""
		g.Expect(gotMetadata).To(Equal(&clusterv1.ObjectMeta{
			Labels:      expectedLabels,
			Annotations: mergeMap(scope.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
		}))

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedField(g, obj, int64(replicas), contract.ControlPlane().Replicas().Path()...)
		assertNestedField(g, obj, map[string]interface{}{
			"kind":       infrastructureMachineTemplate.GetKind(),
			"namespace":  infrastructureMachineTemplate.GetNamespace(),
			"name":       infrastructureMachineTemplate.GetName(),
			"apiVersion": infrastructureMachineTemplate.GetAPIVersion(),
		}, contract.ControlPlane().MachineTemplate().InfrastructureRef().Path()...)
	})
	t.Run("If there is already a reference to the ControlPlane, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		clusterWithControlPlaneRef := cluster.DeepCopy()
		clusterWithControlPlaneRef.Spec.ControlPlaneRef = fakeRef1

		blueprint := &scope.ClusterBlueprint{
			Topology:     clusterWithControlPlaneRef.Spec.Topology,
			ClusterClass: clusterClass,
			ControlPlane: &scope.ControlPlaneBlueprint{
				Template: controlPlaneTemplate,
			},
		}

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(clusterWithControlPlaneRef)
		scope.Blueprint = blueprint

		obj, err := computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     scope.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.ControlPlane.Ref,
			template:    blueprint.ControlPlane.Template,
			currentRef:  scope.Current.Cluster.Spec.ControlPlaneRef,
			obj:         obj,
		})
	})
	t.Run("Should choose the correct version for control plane", func(t *testing.T) {
		// Note: in all of the following tests we are setting it up so that there are not machine deployments.
		// A more extensive list of scenarios is tested in TestComputeControlPlaneVersion.
		tests := []struct {
			name                string
			currentControlPlane *unstructured.Unstructured
			topologyVersion     string
			expectedVersion     string
		}{
			{
				name:                "use cluster.spec.topology.version if creating a new control plane",
				currentControlPlane: nil,
				topologyVersion:     "v1.2.3",
				expectedVersion:     "v1.2.3",
			},
			{
				name: "use controlplane.spec.version if the control plane's spec.version is not equal to status.version",
				currentControlPlane: builder.ControlPlane("test1", "cp1").
					WithSpecFields(map[string]interface{}{
						"spec.version": "v1.2.2",
					}).
					WithStatusFields(map[string]interface{}{
						"status.version": "v1.2.1",
					}).
					Build(),
				topologyVersion: "v1.2.3",
				expectedVersion: "v1.2.2",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				// Current cluster objects for the test scenario.
				clusterWithControlPlaneRef := cluster.DeepCopy()
				clusterWithControlPlaneRef.Spec.ControlPlaneRef = fakeRef1
				clusterWithControlPlaneRef.Spec.Topology.Version = tt.topologyVersion

				blueprint := &scope.ClusterBlueprint{
					Topology:     clusterWithControlPlaneRef.Spec.Topology,
					ClusterClass: clusterClass,
					ControlPlane: &scope.ControlPlaneBlueprint{
						Template: controlPlaneTemplate,
					},
				}

				// Aggregating current cluster objects into ClusterState (simulating getCurrentState).
				s := scope.New(clusterWithControlPlaneRef)
				s.Blueprint = blueprint
				s.Current.ControlPlane = &scope.ControlPlaneState{
					Object: tt.currentControlPlane,
				}

				obj, err := computeControlPlane(ctx, s, nil)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(obj).NotTo(BeNil())
				assertNestedField(g, obj, tt.expectedVersion, contract.ControlPlane().Version().Path()...)
			})
		}
	})
}

func TestComputeControlPlaneVersion(t *testing.T) {
	// Note: the version used by the machine deployments does
	// not affect how we determining the control plane version.
	// We only want to know if the machine deployments are stable.
	//
	// A machine deployment is considere stable if all the following are true:
	// - md.spec.replicas == md.status.replicas
	// - md.spec.replicas == md.status.updatedReplicas
	// - md.spec.replicas == md.status.readyReplicas
	// - md.Generation < md.status.observedGeneration
	//
	// A machine deployment is considered upgrading if any of the above conditions
	// is false.
	machineDeploymentStable := builder.MachineDeployment("test-namespace", "md1").
		WithGeneration(int64(1)).
		WithReplicas(int32(2)).
		WithStatus(clusterv1.MachineDeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           2,
			UpdatedReplicas:    2,
			AvailableReplicas:  2,
			ReadyReplicas:      2,
		}).
		Build()
	machineDeploymentRollingOut := builder.MachineDeployment("test-namespace", "md2").
		WithGeneration(int64(1)).
		WithReplicas(int32(2)).
		WithStatus(clusterv1.MachineDeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           1,
			UpdatedReplicas:    1,
			AvailableReplicas:  1,
			ReadyReplicas:      1,
		}).
		Build()

	tests := []struct {
		name                    string
		topologyVersion         string
		controlPlaneObj         *unstructured.Unstructured
		machineDeploymentsState scope.MachineDeploymentsStateMap
		expectedVersion         string
	}{
		{
			name:            "should return cluster.spec.topology.version if creating a new control plane",
			topologyVersion: "v1.2.3",
			controlPlaneObj: nil,
			expectedVersion: "v1.2.3",
		},
		{
			// Control plane is not upgrading implies that controlplane.spec.version is equal to controlplane.status.version.
			// Control plane is not scaling implies that controlplane.spec.replicas is equal to controlplane.status.replicas,
			// Controlplane.status.updatedReplicas and controlplane.status.readyReplicas.
			name:            "should return cluster.spec.topology.version if the control plane is not upgrading and not scaling",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":         "v1.2.2",
					"status.replicas":        int64(2),
					"status.updatedReplicas": int64(2),
					"status.readyReplicas":   int64(2),
				}).
				Build(),
			expectedVersion: "v1.2.3",
		},
		{
			// Control plane is considered upgrading if controlplane.spec.version is not equal to controlplane.status.version.
			name:            "should return controlplane.spec.version if the control plane is upgrading",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.1",
				}).
				Build(),
			expectedVersion: "v1.2.2",
		},
		{
			// Control plane is considered scaling if controlplane.spec.replicas is not equal to any of
			// controlplane.status.replicas, controlplane.status.readyReplicas, controlplane.status.updatedReplicas.
			name:            "should return controlplane.spec.version if the control plane is scaling",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":         "v1.2.2",
					"status.replicas":        int64(1),
					"status.updatedReplicas": int64(1),
					"status.readyReplicas":   int64(1),
				}).
				Build(),
			expectedVersion: "v1.2.2",
		},
		{
			name:            "should return controlplane.spec.version if control plane is not upgrading and not scaling and one of the machine deployments is rolling out",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":         "v1.2.2",
					"status.replicas":        int64(2),
					"status.updatedReplicas": int64(2),
					"status.readyReplicas":   int64(2),
				}).
				Build(),
			machineDeploymentsState: scope.MachineDeploymentsStateMap{
				"md1": &scope.MachineDeploymentState{Object: machineDeploymentStable},
				"md2": &scope.MachineDeploymentState{Object: machineDeploymentRollingOut},
			},
			expectedVersion: "v1.2.2",
		},
		{
			name:            "should return cluster.spec.topology.version if control plane is not upgrading and not scaling and none of the machine deployments are rolling out",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":         "v1.2.2",
					"status.replicas":        int64(2),
					"status.updatedReplicas": int64(2),
					"status.readyReplicas":   int64(2),
				}).
				Build(),
			machineDeploymentsState: scope.MachineDeploymentsStateMap{
				"md1": &scope.MachineDeploymentState{Object: machineDeploymentStable},
				"md2": &scope.MachineDeploymentState{Object: machineDeploymentStable},
			},
			expectedVersion: "v1.2.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{Topology: &clusterv1.Topology{
					Version: tt.topologyVersion,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Replicas: pointer.Int32(2),
					},
				}},
				Current: &scope.ClusterState{
					ControlPlane:       &scope.ControlPlaneState{Object: tt.controlPlaneObj},
					MachineDeployments: tt.machineDeploymentsState,
				},
			}
			version, err := computeControlPlaneVersion(s)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(version).To(Equal(tt.expectedVersion))
		})
	}
}

func TestComputeCluster(t *testing.T) {
	g := NewWithT(t)

	// generated objects
	infrastructureCluster := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructureCluster1").
		Build()
	controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "controlplane1").
		Build()

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	// aggregating current cluster objects into ClusterState (simulating getCurrentState)
	scope := scope.New(cluster)

	obj := computeCluster(ctx, scope, infrastructureCluster, controlPlane)
	g.Expect(obj).ToNot(BeNil())

	// TypeMeta
	g.Expect(obj.APIVersion).To(Equal(cluster.APIVersion))
	g.Expect(obj.Kind).To(Equal(cluster.Kind))

	// ObjectMeta
	g.Expect(obj.Name).To(Equal(cluster.Name))
	g.Expect(obj.Namespace).To(Equal(cluster.Namespace))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyOwnedLabel, ""))

	// Spec
	g.Expect(obj.Spec.InfrastructureRef).To(Equal(contract.ObjToRef(infrastructureCluster)))
	g.Expect(obj.Spec.ControlPlaneRef).To(Equal(contract.ObjToRef(controlPlane)))
}

func TestComputeMachineDeployment(t *testing.T) {
	workerInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "linux-worker-inframachinetemplate").
		Build()
	workerBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "linux-worker-bootstraptemplate").
		Build()
	labels := map[string]string{"fizz": "buzz", "foo": "bar"}
	annotations := map[string]string{"annotation-1": "annotation-1-val"}

	md1 := builder.MachineDeploymentClass("linux-worker").
		WithLabels(labels).
		WithAnnotations(annotations).
		WithInfrastructureTemplate(workerInfrastructureMachineTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		Build()
	mcds := []clusterv1.MachineDeploymentClass{*md1}
	fakeClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithWorkerMachineDeploymentClasses(mcds...).
		Build()

	version := "v1.21.2"
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: &clusterv1.Topology{
				Version: version,
			},
		},
	}

	blueprint := &scope.ClusterBlueprint{
		Topology:     cluster.Spec.Topology,
		ClusterClass: fakeClass,
		MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
			"linux-worker": {
				Metadata: clusterv1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				BootstrapTemplate:             workerBootstrapTemplate,
				InfrastructureMachineTemplate: workerInfrastructureMachineTemplate,
			},
		},
	}

	replicas := int32(5)
	mdTopology := clusterv1.MachineDeploymentTopology{
		Metadata: clusterv1.ObjectMeta{
			Labels: map[string]string{"foo": "baz"},
		},
		Class:    "linux-worker",
		Name:     "big-pool-of-machines",
		Replicas: &replicas,
	}

	t.Run("Generates the machine deployment and the referenced templates", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		actual, err := computeMachineDeployment(ctx, scope, nil, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(actual.BootstrapTemplate.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))

		// Ensure Cluster ownership is added to generated BootstrapTemplate.
		g.Expect(actual.BootstrapTemplate.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(actual.BootstrapTemplate.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(actual.BootstrapTemplate.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))

		g.Expect(actual.InfrastructureMachineTemplate.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))

		// Ensure Cluster ownership is added to generated InfrastructureMachineTemplate.
		g.Expect(actual.InfrastructureMachineTemplate.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(actual.InfrastructureMachineTemplate.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(actual.InfrastructureMachineTemplate.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))

		actualMd := actual.Object
		g.Expect(*actualMd.Spec.Replicas).To(Equal(replicas))
		g.Expect(actualMd.Spec.ClusterName).To(Equal("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("big-pool-of-machines"))

		g.Expect(actualMd.Labels).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))
		g.Expect(actualMd.Labels).To(HaveKey(clusterv1.ClusterTopologyOwnedLabel))
		g.Expect(controllerutil.ContainsFinalizer(actualMd, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeTrue())

		g.Expect(actualMd.Spec.Selector.MatchLabels).To(HaveKey(clusterv1.ClusterTopologyOwnedLabel))
		g.Expect(actualMd.Spec.Selector.MatchLabels).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))

		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("foo", "baz"))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("fizz", "buzz"))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKey(clusterv1.ClusterTopologyOwnedLabel))
		g.Expect(actualMd.Spec.Template.Spec.InfrastructureRef.Name).ToNot(Equal("linux-worker-inframachinetemplate"))
		g.Expect(actualMd.Spec.Template.Spec.Bootstrap.ConfigRef.Name).ToNot(Equal("linux-worker-bootstraptemplate"))
	})

	t.Run("If there is already a machine deployment, it preserves the object name and the reference names", func(t *testing.T) {
		g := NewWithT(t)
		s := scope.New(cluster)
		s.Blueprint = blueprint

		currentReplicas := int32(3)
		currentMd := &clusterv1.MachineDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing-deployment-1",
			},
			Spec: clusterv1.MachineDeploymentSpec{
				Replicas: &currentReplicas,
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{
						Version: pointer.String("v1.21.2"),
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: contract.ObjToRef(workerBootstrapTemplate),
						},
						InfrastructureRef: *contract.ObjToRef(workerInfrastructureMachineTemplate),
					},
				},
			},
		}
		s.Current.MachineDeployments = map[string]*scope.MachineDeploymentState{
			"big-pool-of-machines": {
				Object:                        currentMd,
				BootstrapTemplate:             workerBootstrapTemplate,
				InfrastructureMachineTemplate: workerInfrastructureMachineTemplate,
			},
		}

		actual, err := computeMachineDeployment(ctx, s, nil, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		actualMd := actual.Object

		g.Expect(*actualMd.Spec.Replicas).NotTo(Equal(currentReplicas))
		g.Expect(actualMd.Name).To(Equal("existing-deployment-1"))

		g.Expect(actualMd.Labels).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))
		g.Expect(actualMd.Labels).To(HaveKey(clusterv1.ClusterTopologyOwnedLabel))
		g.Expect(controllerutil.ContainsFinalizer(actualMd, clusterv1.MachineDeploymentTopologyFinalizer)).To(BeFalse())

		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("foo", "baz"))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("fizz", "buzz"))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKey(clusterv1.ClusterTopologyOwnedLabel))
		g.Expect(actualMd.Spec.Template.Spec.InfrastructureRef.Name).To(Equal("linux-worker-inframachinetemplate"))
		g.Expect(actualMd.Spec.Template.Spec.Bootstrap.ConfigRef.Name).To(Equal("linux-worker-bootstraptemplate"))
	})

	t.Run("If a machine deployment references a topology class that does not exist, machine deployment generation fails", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		mdTopology = clusterv1.MachineDeploymentTopology{
			Metadata: clusterv1.ObjectMeta{
				Labels: map[string]string{"foo": "baz"},
			},
			Class: "windows-worker",
			Name:  "big-pool-of-machines",
		}

		_, err := computeMachineDeployment(ctx, scope, nil, mdTopology)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("Should choose the correct version for machine deployment", func(t *testing.T) {
		controlPlaneStable123 := builder.ControlPlane("test1", "cp1").
			WithSpecFields(map[string]interface{}{
				"spec.version":  "v1.2.3",
				"spec.replicas": int64(2),
			}).
			WithStatusFields(map[string]interface{}{
				"status.version":         "v1.2.3",
				"status.replicas":        int64(2),
				"status.updatedReplicas": int64(2),
				"status.readyReplicas":   int64(2),
			}).
			Build()

		machineDeploymentStable := builder.MachineDeployment("test-namespace", "md-1").
			WithGeneration(1).
			WithReplicas(2).
			WithStatus(clusterv1.MachineDeploymentStatus{
				ObservedGeneration: 2,
				Replicas:           2,
				UpdatedReplicas:    2,
				AvailableReplicas:  2,
			}).
			Build()

		machineDeploymentRollingOut := builder.MachineDeployment("test-namespace", "md-1").
			WithGeneration(1).
			WithReplicas(2).
			WithStatus(clusterv1.MachineDeploymentStatus{
				ObservedGeneration: 2,
				Replicas:           1,
				UpdatedReplicas:    1,
				AvailableReplicas:  1,
			}).
			Build()

		machineDeploymentsStateRollingOut := scope.MachineDeploymentsStateMap{
			"class-1": &scope.MachineDeploymentState{Object: machineDeploymentStable},
			"class-2": &scope.MachineDeploymentState{Object: machineDeploymentRollingOut},
		}

		// Note: in all the following tests we are setting it up so that the control plane is already
		// stable at the topology version.
		// A more extensive list of sceniarios is tested in TestComputeMachineDeploymentVersion.
		tests := []struct {
			name                    string
			machineDeploymentsState scope.MachineDeploymentsStateMap
			currentMDVersion        *string
			topologyVersion         string
			expectedVersion         string
		}{
			{
				name:                    "use cluster.spec.topology.version if creating a new machine deployment",
				machineDeploymentsState: nil,
				currentMDVersion:        nil,
				topologyVersion:         "v1.2.3",
				expectedVersion:         "v1.2.3",
			},
			{
				name:                    "use machine deployment's spec.template.spec.version if one of the machine deployments is rolling out",
				machineDeploymentsState: machineDeploymentsStateRollingOut,
				currentMDVersion:        pointer.String("v1.2.2"),
				topologyVersion:         "v1.2.3",
				expectedVersion:         "v1.2.2",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				s := scope.New(cluster)
				s.Blueprint = blueprint
				s.Blueprint.Topology.Version = tt.topologyVersion
				s.Blueprint.Topology.ControlPlane = clusterv1.ControlPlaneTopology{
					Replicas: pointer.Int32(2),
				}

				mdsState := tt.machineDeploymentsState
				if tt.currentMDVersion != nil {
					// testing a case with an existing machine deployment
					// add the stable machine deployment to the current machine deployments state
					md := builder.MachineDeployment("test-namespace", "big-pool-of-machines").
						WithGeneration(1).
						WithReplicas(2).
						WithVersion(*tt.currentMDVersion).
						WithStatus(clusterv1.MachineDeploymentStatus{
							ObservedGeneration: 2,
							Replicas:           2,
							UpdatedReplicas:    2,
							AvailableReplicas:  2,
						}).
						Build()
					mdsState = duplicateMachineDeploymentsState(mdsState)
					mdsState["big-pool-of-machines"] = &scope.MachineDeploymentState{
						Object: md,
					}
				}
				s.Current.MachineDeployments = mdsState
				s.Current.ControlPlane = &scope.ControlPlaneState{
					Object: controlPlaneStable123,
				}
				desiredControlPlaneState := &scope.ControlPlaneState{
					Object: controlPlaneStable123,
				}

				mdTopology := clusterv1.MachineDeploymentTopology{
					Class:    "linux-worker",
					Name:     "big-pool-of-machines",
					Replicas: pointer.Int32(2),
				}

				obj, err := computeMachineDeployment(ctx, s, desiredControlPlaneState, mdTopology)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(*obj.Object.Spec.Template.Spec.Version).To(Equal(tt.expectedVersion))
			})
		}
	})
}

func TestComputeMachineDeploymentVersion(t *testing.T) {
	controlPlaneStable122 := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version":  "v1.2.2",
			"spec.replicas": int64(2),
		}).
		WithStatusFields(map[string]interface{}{
			"status.version":         "v1.2.2",
			"status.replicas":        int64(2),
			"status.updatedReplicas": int64(2),
			"status.readyReplicas":   int64(2),
		}).
		Build()
	controlPlaneStable123 := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version":  "v1.2.3",
			"spec.replicas": int64(2),
		}).
		WithStatusFields(map[string]interface{}{
			"status.version":         "v1.2.3",
			"status.replicas":        int64(2),
			"status.updatedReplicas": int64(2),
			"status.readyReplicas":   int64(2),
		}).
		Build()
	controlPlaneUpgrading := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version": "v1.2.3",
		}).
		WithStatusFields(map[string]interface{}{
			"status.version": "v1.2.1",
		}).
		Build()
	controlPlaneScaling := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version":  "v1.2.3",
			"spec.replicas": int64(2),
		}).
		WithStatusFields(map[string]interface{}{
			"status.version":         "v1.2.3",
			"status.replicas":        int64(1),
			"status.updatedReplicas": int64(1),
			"status.readyReplicas":   int64(1),
		}).
		Build()
	controlPlaneDesired := builder.ControlPlane("test1", "cp1").
		WithSpecFields(map[string]interface{}{
			"spec.version": "v1.2.3",
		}).
		Build()

	// A machine deployment is considere stable if all the following are true:
	// - md.spec.replicas == md.status.replicas
	// - md.spec.replicas == md.status.updatedReplicas
	// - md.spec.replicas == md.status.readyReplicas
	// - md.Generation < md.status.observedGeneration
	//
	// A machine deployment is considered upgrading if any of the above conditions
	// is false.
	machineDeploymentStable := builder.MachineDeployment("test-namespace", "md-1").
		WithGeneration(1).
		WithReplicas(2).
		WithStatus(clusterv1.MachineDeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           2,
			UpdatedReplicas:    2,
			AvailableReplicas:  2,
			ReadyReplicas:      2,
		}).
		Build()
	machineDeploymentRollingOut := builder.MachineDeployment("test-namespace", "md-2").
		WithGeneration(1).
		WithReplicas(2).
		WithStatus(clusterv1.MachineDeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           1,
			UpdatedReplicas:    1,
			AvailableReplicas:  1,
			ReadyReplicas:      1,
		}).
		Build()

	machineDeploymentsStateStable := scope.MachineDeploymentsStateMap{
		"md1": &scope.MachineDeploymentState{Object: machineDeploymentStable},
		"md2": &scope.MachineDeploymentState{Object: machineDeploymentStable},
	}
	machineDeploymentsStateRollingOut := scope.MachineDeploymentsStateMap{
		"md1": &scope.MachineDeploymentState{Object: machineDeploymentStable},
		"md2": &scope.MachineDeploymentState{Object: machineDeploymentRollingOut},
	}

	tests := []struct {
		name                          string
		currentMachineDeploymentState *scope.MachineDeploymentState
		machineDeploymentsStateMap    scope.MachineDeploymentsStateMap
		currentControlPlane           *unstructured.Unstructured
		desiredControlPlane           *unstructured.Unstructured
		topologyVersion               string
		expectedVersion               string
	}{
		{
			name:                          "should return cluster.spec.topology.version if creating a new machine deployment",
			currentMachineDeploymentState: nil,
			machineDeploymentsStateMap:    make(scope.MachineDeploymentsStateMap),
			topologyVersion:               "v1.2.3",
			expectedVersion:               "v1.2.3",
		},
		{
			name:                          "should return machine deployment's spec.template.spec.version if any one of the machine deployments is rolling out",
			currentMachineDeploymentState: &scope.MachineDeploymentState{Object: builder.MachineDeployment("test1", "md-current").WithVersion("v1.2.2").Build()},
			machineDeploymentsStateMap:    machineDeploymentsStateRollingOut,
			currentControlPlane:           controlPlaneStable123,
			desiredControlPlane:           controlPlaneDesired,
			topologyVersion:               "v1.2.3",
			expectedVersion:               "v1.2.2",
		},
		{
			// Control plane is considered upgrading if the control plane's spec.version and status.version is not equal.
			name:                          "should return machine deployment's spec.template.spec.version if control plane is upgrading",
			currentMachineDeploymentState: &scope.MachineDeploymentState{Object: builder.MachineDeployment("test1", "md-current").WithVersion("v1.2.2").Build()},
			machineDeploymentsStateMap:    machineDeploymentsStateStable,
			currentControlPlane:           controlPlaneUpgrading,
			topologyVersion:               "v1.2.3",
			expectedVersion:               "v1.2.2",
		},
		{
			// Control plane is considered ready to upgrade if spec.version of current and desired control planes are not equal.
			name:                          "should return machine deployment's spec.template.spec.version if control plane is ready to upgrade",
			currentMachineDeploymentState: &scope.MachineDeploymentState{Object: builder.MachineDeployment("test1", "md-current").WithVersion("v1.2.2").Build()},
			machineDeploymentsStateMap:    machineDeploymentsStateStable,
			currentControlPlane:           controlPlaneStable122,
			desiredControlPlane:           controlPlaneDesired,
			topologyVersion:               "v1.2.3",
			expectedVersion:               "v1.2.2",
		},
		{
			// Control plane is considered scaling if its spec.replicas is not equal to any of status.replicas, status.readyReplicas or status.updatedReplicas.
			name:                          "should return machine deployment's spec.template.spec.version if control plane is scaling",
			currentMachineDeploymentState: &scope.MachineDeploymentState{Object: builder.MachineDeployment("test1", "md-current").WithVersion("v1.2.2").Build()},
			machineDeploymentsStateMap:    machineDeploymentsStateStable,
			currentControlPlane:           controlPlaneScaling,
			topologyVersion:               "v1.2.3",
			expectedVersion:               "v1.2.2",
		},
		{
			name:                          "should return cluster.spec.topology.version if the control plane is not upgrading, not scaling, not ready to upgrade and none of the machine deployments are rolling out",
			currentMachineDeploymentState: &scope.MachineDeploymentState{Object: builder.MachineDeployment("test1", "md-current").WithVersion("v1.2.2").Build()},
			machineDeploymentsStateMap:    machineDeploymentsStateStable,
			currentControlPlane:           controlPlaneStable123,
			desiredControlPlane:           controlPlaneDesired,
			topologyVersion:               "v1.2.3",
			expectedVersion:               "v1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{Topology: &clusterv1.Topology{
					Version: tt.topologyVersion,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Replicas: pointer.Int32(2),
					},
				}},
				Current: &scope.ClusterState{
					ControlPlane:       &scope.ControlPlaneState{Object: tt.currentControlPlane},
					MachineDeployments: tt.machineDeploymentsStateMap,
				},
				UpgradeTracker: scope.NewUpgradeTracker(),
			}
			desiredControlPlaneState := &scope.ControlPlaneState{Object: tt.desiredControlPlane}
			version, err := computeMachineDeploymentVersion(s, desiredControlPlaneState, tt.currentMachineDeploymentState)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(version).To(Equal(tt.expectedVersion))
		})
	}
}

func TestTemplateToObject(t *testing.T) {
	template := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infrastructureClusterTemplate").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	t.Run("Generates an object from a template", func(t *testing.T) {
		g := NewWithT(t)
		obj, err := templateToObject(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      nil,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			currentRef:  nil,
			obj:         obj,
		})
	})
	t.Run("Overrides the generated name if there is already a reference", func(t *testing.T) {
		g := NewWithT(t)
		obj, err := templateToObject(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      fakeRef2,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// ObjectMeta
		assertTemplateToObject(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			currentRef:  fakeRef2,
			obj:         obj,
		})
	})
}

func TestTemplateToTemplate(t *testing.T) {
	template := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infrastructureClusterTemplate").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	annotations := template.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[corev1.LastAppliedConfigAnnotation] = "foo"
	template.SetAnnotations(annotations)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	t.Run("Generates a template from a template", func(t *testing.T) {
		g := NewWithT(t)
		obj := templateToTemplate(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      nil,
		})
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			currentRef:  nil,
			obj:         obj,
		})
	})
	t.Run("Overrides the generated name if there is already a reference", func(t *testing.T) {
		g := NewWithT(t)
		obj := templateToTemplate(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      fakeRef2,
		})
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			currentRef:  fakeRef2,
			obj:         obj,
		})
	})
}

type assertTemplateInput struct {
	cluster             *clusterv1.Cluster
	templateRef         *corev1.ObjectReference
	template            *unstructured.Unstructured
	labels, annotations map[string]string
	currentRef          *corev1.ObjectReference
	obj                 *unstructured.Unstructured
}

func assertTemplateToObject(g *WithT, in assertTemplateInput) {
	// TypeMeta
	g.Expect(in.obj.GetAPIVersion()).To(Equal(in.template.GetAPIVersion()))
	g.Expect(in.obj.GetKind()).To(Equal(strings.TrimSuffix(in.template.GetKind(), "Template")))

	// ObjectMeta
	if in.currentRef != nil {
		g.Expect(in.obj.GetName()).To(Equal(in.currentRef.Name))
	} else {
		g.Expect(in.obj.GetName()).To(HavePrefix(in.cluster.Name))
	}
	g.Expect(in.obj.GetNamespace()).To(Equal(in.cluster.Namespace))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterLabelName, in.cluster.Name))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyOwnedLabel, ""))
	for k, v := range in.labels {
		g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(k, v))
	}
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, in.templateRef.GroupVersionKind().GroupKind().String()))
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, in.templateRef.Name))
	for k, v := range in.annotations {
		g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(k, v))
	}
	// Spec
	expectedSpec, ok, err := unstructured.NestedMap(in.template.UnstructuredContent(), "spec", "template", "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())

	cloneSpec, ok, err := unstructured.NestedMap(in.obj.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	for k, v := range expectedSpec {
		g.Expect(cloneSpec).To(HaveKeyWithValue(k, v))
	}
}

func assertTemplateToTemplate(g *WithT, in assertTemplateInput) {
	// TypeMeta
	g.Expect(in.obj.GetAPIVersion()).To(Equal(in.template.GetAPIVersion()))
	g.Expect(in.obj.GetKind()).To(Equal(in.template.GetKind()))

	// ObjectMeta
	if in.currentRef != nil {
		g.Expect(in.obj.GetName()).To(Equal(in.currentRef.Name))
	} else {
		g.Expect(in.obj.GetName()).To(HavePrefix(in.cluster.Name))
	}
	g.Expect(in.obj.GetNamespace()).To(Equal(in.cluster.Namespace))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterLabelName, in.cluster.Name))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyOwnedLabel, ""))
	for k, v := range in.labels {
		g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(k, v))
	}
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, in.templateRef.GroupVersionKind().GroupKind().String()))
	g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, in.templateRef.Name))
	g.Expect(in.obj.GetAnnotations()).ToNot(HaveKey(corev1.LastAppliedConfigAnnotation))
	for k, v := range in.annotations {
		g.Expect(in.obj.GetAnnotations()).To(HaveKeyWithValue(k, v))
	}
	// Spec
	expectedSpec, ok, err := unstructured.NestedMap(in.template.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())

	cloneSpec, ok, err := unstructured.NestedMap(in.obj.UnstructuredContent(), "spec")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(cloneSpec).To(Equal(expectedSpec))
}

func assertNestedField(g *WithT, obj *unstructured.Unstructured, value interface{}, fields ...string) {
	v, ok, err := unstructured.NestedFieldCopy(obj.UnstructuredContent(), fields...)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(v).To(Equal(value))
}

func assertNestedFieldUnset(g *WithT, obj *unstructured.Unstructured, fields ...string) {
	_, ok, err := unstructured.NestedFieldCopy(obj.UnstructuredContent(), fields...)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeFalse())
}

func duplicateMachineDeploymentsState(s scope.MachineDeploymentsStateMap) scope.MachineDeploymentsStateMap {
	n := make(scope.MachineDeploymentsStateMap)
	for k, v := range s {
		n[k] = v
	}
	return n
}

func TestMergeMap(t *testing.T) {
	t.Run("Merge maps", func(t *testing.T) {
		g := NewWithT(t)

		m := mergeMap(
			map[string]string{
				"a": "a",
				"b": "b",
			}, map[string]string{
				"a": "ax",
				"c": "c",
			},
		)
		g.Expect(m).To(HaveKeyWithValue("a", "a"))
		g.Expect(m).To(HaveKeyWithValue("b", "b"))
		g.Expect(m).To(HaveKeyWithValue("c", "c"))
	})
	t.Run("Nils empty maps", func(t *testing.T) {
		g := NewWithT(t)

		m := mergeMap(map[string]string{}, map[string]string{})
		g.Expect(m).To(BeNil())
	})
}
