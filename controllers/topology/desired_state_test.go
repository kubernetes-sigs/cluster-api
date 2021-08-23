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

	"sigs.k8s.io/cluster-api/internal/testtypes"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
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
	infrastructureClusterTemplate := testtypes.NewInfrastructureClusterTemplateBuilder(metav1.NamespaceDefault, "template1").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterClass := testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
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

	infrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "template1").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterClass := testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
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
	})
	t.Run("If there is already a reference to the infrastructureMachineTemplate, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		currentInfrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "cluster1-template1").Build()

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

	controlPlaneTemplate := testtypes.NewControlPlaneTemplateBuilder(metav1.NamespaceDefault, "template1").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterClass := testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
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
		infrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "template1").Build()
		clusterClass := testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
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
		g.Expect(gotMetadata).To(Equal(&clusterv1.ObjectMeta{
			Labels:      mergeMap(scope.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels),
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
}

func TestComputeCluster(t *testing.T) {
	g := NewWithT(t)

	// generated objects
	infrastructureCluster := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infrastructureCluster1").
		Build()
	controlPlane := testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "controlplane1").
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
	workerInfrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "linux-worker-inframachinetemplate").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	workerBootstrapTemplate := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "linux-worker-bootstraptemplate").
		Build()
	labels := map[string]string{"fizz": "buzz", "foo": "bar"}
	annotations := map[string]string{"annotation-1": "annotation-1-val"}

	md1 := testtypes.NewMachineDeploymentClassBuilder(metav1.NamespaceDefault, "class1").
		WithClass("linux-worker").
		WithLabels(labels).
		WithAnnotations(annotations).
		WithInfrastructureTemplate(workerInfrastructureMachineTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		Build()
	mcds := []clusterv1.MachineDeploymentClass{*md1}
	fakeClass := testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
		WithWorkerMachineDeploymentClasses(mcds).
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

		actual, err := computeMachineDeployment(ctx, scope, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		actualMd := actual.Object
		g.Expect(*actualMd.Spec.Replicas).To(Equal(replicas))
		g.Expect(actualMd.Spec.ClusterName).To(Equal("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("big-pool-of-machines"))

		g.Expect(actualMd.Labels).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))

		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("foo", "baz"))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("fizz", "buzz"))
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

		actual, err := computeMachineDeployment(ctx, s, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		actualMd := actual.Object
		g.Expect(*actualMd.Spec.Replicas).NotTo(Equal(currentReplicas))
		g.Expect(actualMd.Name).To(Equal("existing-deployment-1"))

		g.Expect(actualMd.Labels).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentLabelName, "big-pool-of-machines"))

		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("foo", "baz"))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(HaveKeyWithValue("fizz", "buzz"))
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

		_, err := computeMachineDeployment(ctx, scope, mdTopology)
		g.Expect(err).To(HaveOccurred())
	})
}

func TestTemplateToObject(t *testing.T) {
	template := testtypes.NewInfrastructureClusterTemplateBuilder(metav1.NamespaceDefault, "infrastructureClusterTemplate").
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
	template := testtypes.NewInfrastructureClusterTemplateBuilder(metav1.NamespaceDefault, "infrastructureClusterTemplate").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
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
