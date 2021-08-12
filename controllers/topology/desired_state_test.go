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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
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
	infrastructureClusterTemplate := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "template1").Obj()
	clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate).
		Obj()

	// aggregating templates and cluster class into topologyClass (simulating getClass)
	topologyClass := &clusterTopologyClass{
		clusterClass:                  clusterClass,
		infrastructureClusterTemplate: infrastructureClusterTemplate,
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

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeInfrastructureCluster(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.Infrastructure.Ref,
			template:    topologyClass.infrastructureClusterTemplate,
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

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: clusterWithInfrastructureRef,
		}

		obj, err := computeInfrastructureCluster(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.Infrastructure.Ref,
			template:    topologyClass.infrastructureClusterTemplate,
			labels:      nil,
			annotations: nil,
			currentRef:  current.cluster.Spec.InfrastructureRef,
			obj:         obj,
		})
	})
}

func TestComputeControlPlaneInfrastructureMachineTemplate(t *testing.T) {
	// templates and ClusterClass
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	infrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").Obj()
	clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneMetadata(labels, annotations).
		WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Obj()

	// aggregating templates and cluster class into topologyClass (simulating getClass)
	topologyClass := &clusterTopologyClass{
		clusterClass: clusterClass,
		controlPlane: controlPlaneTopologyClass{
			infrastructureMachineTemplate: infrastructureMachineTemplate,
		},
	}

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

	t.Run("Generates the infrastructureMachineTemplate from the template", func(t *testing.T) {
		g := NewWithT(t)

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeControlPlaneInfrastructureMachineTemplate(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref,
			template:    topologyClass.controlPlane.infrastructureMachineTemplate,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})
	})
	t.Run("If there is already a reference to the infrastructureMachineTemplate, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		currentInfrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "cluster1-template1").Obj()

		controlPlane := &unstructured.Unstructured{Object: map[string]interface{}{}}
		err := setNestedRef(controlPlane, currentInfrastructureMachineTemplate, "spec", "machineTemplate", "infrastructureRef")
		g.Expect(err).ToNot(HaveOccurred())

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
			controlPlane: controlPlaneTopologyState{
				object:                        controlPlane,
				infrastructureMachineTemplate: currentInfrastructureMachineTemplate,
			},
		}

		obj, err := computeControlPlaneInfrastructureMachineTemplate(topologyClass, current)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.MachineInfrastructure.Ref,
			template:    topologyClass.controlPlane.infrastructureMachineTemplate,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  objToRef(currentInfrastructureMachineTemplate),
			obj:         obj,
		})
	})
}

func TestComputeControlPlane(t *testing.T) {
	// templates and ClusterClass
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	controlPlaneTemplate := newFakeControlPlaneTemplate(metav1.NamespaceDefault, "template1").Obj()
	clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneMetadata(labels, annotations).
		WithControlPlaneTemplate(controlPlaneTemplate).
		Obj()

	// aggregating templates and cluster class into topologyClass (simulating getClass)
	topologyClass := &clusterTopologyClass{
		clusterClass: clusterClass,
		controlPlane: controlPlaneTopologyClass{
			template: controlPlaneTemplate,
		},
	}

	// current cluster objects
	version := "v1.21.2"
	replicas := 3
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

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeControlPlane(topologyClass, current, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, "spec", "version")
		assertNestedField(g, obj, int64(replicas), "spec", "replicas")
		assertNestedFieldUnset(g, obj, "spec", "machineTemplate", "infrastructureRef")
	})
	t.Run("Skips setting replicas if required", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects
		clusterWithoutReplicas := cluster.DeepCopy()
		clusterWithoutReplicas.Spec.Topology.ControlPlane.Replicas = nil

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: clusterWithoutReplicas,
		}

		obj, err := computeControlPlane(topologyClass, current, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, "spec", "version")
		assertNestedFieldUnset(g, obj, "spec", "replicas")
		assertNestedFieldUnset(g, obj, "spec", "machineTemplate", "infrastructureRef")
	})
	t.Run("Generates the ControlPlane from the template and adds the infrastructure machine template if required", func(t *testing.T) {
		g := NewWithT(t)

		// templates and ClusterClass
		infrastructureMachineTemplate := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").Obj()
		clusterClass := newFakeClusterClass(metav1.NamespaceDefault, "class1").
			WithControlPlaneMetadata(labels, annotations).
			WithControlPlaneTemplate(controlPlaneTemplate).
			WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).
			Obj()

		// aggregating templates and cluster class into topologyClass (simulating getClass)
		topologyClass := &clusterTopologyClass{
			clusterClass: clusterClass,
			controlPlane: controlPlaneTopologyClass{
				template:                      controlPlaneTemplate,
				infrastructureMachineTemplate: infrastructureMachineTemplate,
			},
		}

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: cluster,
		}

		obj, err := computeControlPlane(topologyClass, current, infrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  nil,
			obj:         obj,
		})

		assertNestedField(g, obj, version, "spec", "version")
		assertNestedField(g, obj, int64(replicas), "spec", "replicas")
		assertNestedField(g, obj, map[string]interface{}{
			"kind":       infrastructureMachineTemplate.GetKind(),
			"namespace":  infrastructureMachineTemplate.GetNamespace(),
			"name":       infrastructureMachineTemplate.GetName(),
			"apiVersion": infrastructureMachineTemplate.GetAPIVersion(),
		}, "spec", "machineTemplate", "infrastructureRef")
	})
	t.Run("If there is already a reference to the ControlPlane, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		clusterWithControlPlaneRef := cluster.DeepCopy()
		clusterWithControlPlaneRef.Spec.ControlPlaneRef = fakeRef1

		// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
		current := &clusterTopologyState{
			cluster: clusterWithControlPlaneRef,
		}

		obj, err := computeControlPlane(topologyClass, current, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     current.cluster,
			templateRef: topologyClass.clusterClass.Spec.ControlPlane.Ref,
			template:    topologyClass.controlPlane.template,
			labels:      mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Labels, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations: mergeMap(current.cluster.Spec.Topology.ControlPlane.Metadata.Annotations, topologyClass.clusterClass.Spec.ControlPlane.Metadata.Annotations),
			currentRef:  current.cluster.Spec.ControlPlaneRef,
			obj:         obj,
		})
	})
}

func TestComputeControlPlaneVersion(t *testing.T) {
	tests := []struct {
		name                string
		topologyVersion     string
		currentControlPlane *unstructured.Unstructured
		want                string
		wantErr             bool
	}{
		{
			name:                "ControlPlane does not exist",
			topologyVersion:     "v1.21.0",
			currentControlPlane: nil,
			want:                "v1.21.0",
		},
		{
			name:                "ControlPlane does exist, but without version",
			topologyVersion:     "v1.21.0",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").Obj(),
			wantErr:             true,
		},
		{
			name:            "ControlPlane does exist, with same version (no-op)",
			topologyVersion: "v1.21.0",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").
				WithVersion("v1.21.0").
				WithStatusVersion("v1.21.0").
				Obj(),
			want: "v1.21.0",
		},
		{
			name:            "ControlPlane does exist, with newer version (downgrade)",
			topologyVersion: "v1.21.0",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").
				WithVersion("v1.22.0").
				WithStatusVersion("v1.22.0").
				Obj(),
			wantErr: true,
		},
		{
			name:            "ControlPlane does exist, with invalid version",
			topologyVersion: "v1.21.0",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").
				WithVersion("invalid-version").
				WithStatusVersion("invalid-version").
				Obj(),
			wantErr: true,
		},
		{
			name:            "ControlPlane does exist, with valid version but invalid topology version",
			topologyVersion: "invalid-version",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").
				WithVersion("v1.21.0").
				WithStatusVersion("v1.21.0").
				Obj(),
			wantErr: true,
		},
		{
			name:            "ControlPlane does exist, with older version (upgrade)",
			topologyVersion: "v1.21.0",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").
				WithVersion("v1.20.0").
				WithStatusVersion("v1.20.0").
				Obj(),
			want: "v1.21.0",
		},
		{
			name:            "ControlPlane does exist, with older version (upgrade) but another upgrade is already in progress (.status.version has older version)",
			topologyVersion: "v1.22.0",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").
				WithVersion("v1.21.0").
				WithStatusVersion("v1.20.0").
				Obj(),
			want: "v1.21.0",
		},
		{
			name:            "ControlPlane does exist, with older version (upgrade) but another upgrade is already in progress (.status.version is not set)",
			topologyVersion: "v1.22.0",
			currentControlPlane: newFakeControlPlane(metav1.NamespaceDefault, "cp").
				WithVersion("v1.21.0").
				Obj(),
			want: "v1.21.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			current := &clusterTopologyState{
				cluster: &clusterv1.Cluster{
					Spec: clusterv1.ClusterSpec{
						Topology: &clusterv1.Topology{
							Version: tt.topologyVersion,
						},
					},
				},
				controlPlane: controlPlaneTopologyState{
					object: tt.currentControlPlane,
				},
			}

			got, err := computeControlPlaneVersion(current)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestComputeCluster(t *testing.T) {
	// generated objects
	infrastructureCluster := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructureCluster1").Obj()
	controlPlane := newFakeControlPlane(metav1.NamespaceDefault, "controlplane1").Obj()

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	// aggregating current cluster objects into clusterTopologyState (simulating getCurrentState)
	current := &clusterTopologyState{
		cluster: cluster,
	}

	g := NewWithT(t)

	obj := computeCluster(current, infrastructureCluster, controlPlane)
	g.Expect(obj).ToNot(BeNil())

	// TypeMeta
	g.Expect(obj.APIVersion).To(Equal(cluster.APIVersion))
	g.Expect(obj.Kind).To(Equal(cluster.Kind))

	// ObjectMeta
	g.Expect(obj.Name).To(Equal(cluster.Name))
	g.Expect(obj.Namespace).To(Equal(cluster.Namespace))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyLabelName, ""))

	// Spec
	g.Expect(obj.Spec.InfrastructureRef).To(Equal(objToRef(infrastructureCluster)))
	g.Expect(obj.Spec.ControlPlaneRef).To(Equal(objToRef(controlPlane)))
}

func TestTemplateToObject(t *testing.T) {
	template := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "infrastructureClusterTemplate").Obj()
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	t.Run("Generates an object from a template", func(t *testing.T) {
		g := NewWithT(t)
		obj, err := templateToObject(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      nil,
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
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
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// ObjectMeta
		assertTemplateToObject(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
			currentRef:  fakeRef2,
			obj:         obj,
		})
	})
}

func TestTemplateToTemplate(t *testing.T) {
	template := newFakeInfrastructureClusterTemplate(metav1.NamespaceDefault, "infrastructureClusterTemplate").Obj()
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	t.Run("Generates a template from a template", func(t *testing.T) {
		g := NewWithT(t)
		obj := templateToTemplate(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			namePrefix:            cluster.Name,
			currentObjectRef:      nil,
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
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
			labels:                labels,
			annotations:           annotations,
		})
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:     cluster,
			templateRef: fakeRef1,
			template:    template,
			labels:      labels,
			annotations: annotations,
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
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyLabelName, ""))
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
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyLabelName, ""))
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
