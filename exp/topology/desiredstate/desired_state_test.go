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

package desiredstate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/clustercache"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/exp/topology/scope"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/topology/clustershim"
	topologynames "sigs.k8s.io/cluster-api/internal/topology/names"
	"sigs.k8s.io/cluster-api/internal/topology/ownerrefs"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/cache"
	"sigs.k8s.io/cluster-api/util/conversion"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeScheme)
	_ = clusterv1.AddToScheme(fakeScheme)
	_ = apiextensionsv1.AddToScheme(fakeScheme)
	_ = corev1.AddToScheme(fakeScheme)
}

var (
	fakeRef1 = &corev1.ObjectReference{
		Kind:       "refKind1",
		Namespace:  "refNamespace1",
		Name:       "refName1",
		APIVersion: "refAPIVersion1",
	}

	fakeContractVersionedRef1 = clusterv1.ContractVersionedObjectReference{
		APIGroup: "refAPIGroup1",
		Kind:     "refKind1",
		Name:     "refName1",
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
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.Infrastructure.TemplateRef,
			template:          blueprint.InfrastructureClusterTemplate,
			labels:            nil,
			annotations:       nil,
			currentObjectName: "",
			obj:               obj,
		})

		// Ensure no ownership is added to generated InfrastructureCluster.
		g.Expect(obj.GetOwnerReferences()).To(BeEmpty())
	})
	t.Run("If there is already a reference to the infrastructureCluster, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		clusterWithInfrastructureRef := cluster.DeepCopy()
		clusterWithInfrastructureRef.Spec.InfrastructureRef = fakeContractVersionedRef1

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(clusterWithInfrastructureRef)
		scope.Blueprint = blueprint

		obj, err := computeInfrastructureCluster(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.Infrastructure.TemplateRef,
			template:          blueprint.InfrastructureClusterTemplate,
			labels:            nil,
			annotations:       nil,
			currentObjectName: scope.Current.Cluster.Spec.InfrastructureRef.Name,
			obj:               obj,
		})
	})
	t.Run("Carry over the owner reference to ClusterShim, if any", func(t *testing.T) {
		g := NewWithT(t)
		shim := clustershim.New(cluster)

		// current cluster objects for the test scenario
		clusterWithInfrastructureRef := cluster.DeepCopy()
		clusterWithInfrastructureRef.Spec.InfrastructureRef = fakeContractVersionedRef1

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(clusterWithInfrastructureRef)
		scope.Current.InfrastructureCluster = infrastructureClusterTemplate.DeepCopy()
		scope.Current.InfrastructureCluster.SetOwnerReferences([]metav1.OwnerReference{*ownerrefs.OwnerReferenceTo(shim, corev1.SchemeGroupVersion.WithKind("Secret"))})
		scope.Blueprint = blueprint

		obj, err := computeInfrastructureCluster(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())
		g.Expect(ownerrefs.HasOwnerReferenceFrom(obj, shim)).To(BeTrue())
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
			Topology: clusterv1.Topology{
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

		obj, err := (&generator{Client: fake.NewClientBuilder().WithObjects().Build()}).computeControlPlaneInfrastructureMachineTemplate(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef,
			template:          blueprint.ControlPlane.InfrastructureMachineTemplate,
			currentObjectName: "",
			obj:               obj,
		})

		// Ensure Cluster ownership is added to generated InfrastructureCluster.
		g.Expect(obj.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(obj.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(obj.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))
	})

	t.Run("Always generates the infrastructureMachineTemplate from the template in the cluster namespace", func(t *testing.T) {
		g := NewWithT(t)

		cluster := cluster.DeepCopy()
		cluster.Namespace = "differs"
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		obj, err := (&generator{Client: fake.NewClientBuilder().WithObjects().Build()}).computeControlPlaneInfrastructureMachineTemplate(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef,
			template:          blueprint.ControlPlane.InfrastructureMachineTemplate,
			currentObjectName: "",
			obj:               obj,
		})

		// Ensure Cluster ownership is added to generated InfrastructureCluster.
		g.Expect(obj.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(obj.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(obj.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))
	})

	t.Run("If there is already a reference to the infrastructureMachineTemplate, it preserves the reference name (v1beta1 contract)", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		currentInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cluster1-template1").Build()

		controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "controlplane").Build()
		err := contract.ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Set(controlPlane, contract.ObjToRef(currentInfrastructureMachineTemplate))
		g.Expect(err).ToNot(HaveOccurred())

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		s := scope.New(cluster)
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object:                        controlPlane,
			InfrastructureMachineTemplate: currentInfrastructureMachineTemplate,
		}
		s.Blueprint = blueprint

		scheme := runtime.NewScheme()
		g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		crd := builder.GenericControlPlaneCRD.DeepCopy()
		crd.Labels = map[string]string{
			// Set contract label for v1beta1 contract.
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, "v1beta1"): clusterv1.GroupVersionControlPlane.Version,
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()

		obj, err := (&generator{Client: client}).computeControlPlaneInfrastructureMachineTemplate(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:           s.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef,
			template:          blueprint.ControlPlane.InfrastructureMachineTemplate,
			currentObjectName: contract.ObjToRef(currentInfrastructureMachineTemplate).Name,
			obj:               obj,
		})
	})

	t.Run("If there is already a reference to the infrastructureMachineTemplate, it preserves the reference name (v1beta2 contract)", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		currentInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cluster1-template1").Build()

		controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "controlplane").Build()
		err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Set(controlPlane, ptr.To(contract.ObjToContractVersionedObjectReference(currentInfrastructureMachineTemplate)))
		g.Expect(err).ToNot(HaveOccurred())

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		s := scope.New(cluster)
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object:                        controlPlane,
			InfrastructureMachineTemplate: currentInfrastructureMachineTemplate,
		}
		s.Blueprint = blueprint

		scheme := runtime.NewScheme()
		g.Expect(apiextensionsv1.AddToScheme(scheme)).To(Succeed())
		crd := builder.GenericControlPlaneCRD.DeepCopy()
		crd.Labels = map[string]string{
			// Set contract label for v1beta1 contract.
			// Note: This is the same as on GenericControlPlaneCRD, but being explicit here for clarity.
			fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, "v1beta2"): clusterv1.GroupVersionControlPlane.Version,
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()

		obj, err := (&generator{Client: client}).computeControlPlaneInfrastructureMachineTemplate(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToTemplate(g, assertTemplateInput{
			cluster:           s.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure.TemplateRef,
			template:          blueprint.ControlPlane.InfrastructureMachineTemplate,
			currentObjectName: contract.ObjToRef(currentInfrastructureMachineTemplate).Name,
			obj:               obj,
		})
	})
}

func TestComputeControlPlane(t *testing.T) {
	g := NewWithT(t)

	// templates and ClusterClass
	labels := map[string]string{"l1": ""}
	annotations := map[string]string{"a1": ""}

	controlPlaneTemplate := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "template1").
		Build()
	controlPlaneMachineTemplateLabels := map[string]string{
		"machineTemplateLabel": "machineTemplateLabelValue",
	}
	controlPlaneMachineTemplateAnnotations := map[string]string{
		"machineTemplateAnnotation": "machineTemplateAnnotationValue",
	}
	controlPlaneTemplateWithMachineTemplate := controlPlaneTemplate.DeepCopy()
	_ = contract.ControlPlaneTemplate().Template().MachineTemplate().Metadata().Set(controlPlaneTemplateWithMachineTemplate, &clusterv1.ObjectMeta{
		Labels:      controlPlaneMachineTemplateLabels,
		Annotations: controlPlaneMachineTemplateAnnotations,
	})
	clusterClassDuration := int32(20)
	clusterClassReadinessGates := []clusterv1.MachineReadinessGate{
		{ConditionType: "foo"},
	}
	clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithControlPlaneMetadata(labels, annotations).
		WithControlPlaneReadinessGates(clusterClassReadinessGates).
		WithControlPlaneTemplate(controlPlaneTemplate).
		WithControlPlaneNodeDrainTimeout(&clusterClassDuration).
		WithControlPlaneNodeVolumeDetachTimeout(&clusterClassDuration).
		WithControlPlaneNodeDeletionTimeout(&clusterClassDuration).
		Build()
	// TODO: Replace with object builder.
	// current cluster objects
	version := "v1.21.2"
	replicas := int32(3)
	topologyDuration := int32(10)
	nodeDrainTimeout := topologyDuration
	nodeVolumeDetachTimeout := topologyDuration
	nodeDeletionTimeout := topologyDuration
	readinessGates := []clusterv1.MachineReadinessGate{
		{ConditionType: "foo"},
		{ConditionType: "bar"},
	}
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: clusterv1.Topology{
				Version: version,
				ControlPlane: clusterv1.ControlPlaneTopology{
					Metadata: clusterv1.ObjectMeta{
						Labels:      map[string]string{"l2": ""},
						Annotations: map[string]string{"a2": ""},
					},
					ReadinessGates: readinessGates,
					Replicas:       &replicas,
					Deletion: clusterv1.ControlPlaneTopologyMachineDeletionSpec{
						NodeDrainTimeoutSeconds:        &nodeDrainTimeout,
						NodeVolumeDetachTimeoutSeconds: &nodeVolumeDetachTimeout,
						NodeDeletionTimeoutSeconds:     &nodeDeletionTimeout,
					},
				},
			},
		},
	}

	jsonValue, err := json.Marshal(&clusterClassReadinessGates)
	g.Expect(err).ToNot(HaveOccurred())
	var expectedClusterClassReadinessGates []interface{}
	g.Expect(json.Unmarshal(jsonValue, &expectedClusterClassReadinessGates)).ToNot(HaveOccurred())
	jsonValue, err = json.Marshal(&readinessGates)
	g.Expect(err).ToNot(HaveOccurred())
	var expectedReadinessGates []interface{}
	g.Expect(json.Unmarshal(jsonValue, &expectedReadinessGates)).ToNot(HaveOccurred())

	scheme := runtime.NewScheme()
	_ = clusterv1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	crdV1Beta1Contract := builder.GenericControlPlaneCRD.DeepCopy()
	crdV1Beta1Contract.Labels = map[string]string{
		// Set contract label for tt.contract.
		fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, "v1beta1"): clusterv1.GroupVersionControlPlane.Version,
	}
	clientWithV1Beta1ContractCRD := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crdV1Beta1Contract).Build()
	crdV1Beta2Contract := builder.GenericControlPlaneCRD.DeepCopy()
	crdV1Beta2Contract.Labels = map[string]string{
		// Set contract label for tt.contract.
		fmt.Sprintf("%s/%s", clusterv1.GroupVersion.Group, "v1beta2"): clusterv1.GroupVersionControlPlane.Version,
	}
	clientWithV1Beta2ContractCRD := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crdV1Beta2Contract).Build()

	t.Run("Generates the ControlPlane from the template (v1beta1 contract)", func(t *testing.T) {
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

		obj, err := (&generator{Client: clientWithV1Beta1ContractCRD}).computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.TemplateRef,
			template:          blueprint.ControlPlane.Template,
			currentObjectName: "",
			obj:               obj,
			labels:            util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations:       util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
		})

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedField(g, obj, int64(replicas), contract.ControlPlane().Replicas().Path()...)
		assertNestedField(g, obj, expectedReadinessGates, contract.ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Path()...)
		assertNestedField(g, obj, (time.Duration(topologyDuration) * time.Second).String(), contract.ControlPlane().MachineTemplate().NodeDrainTimeout().Path()...)
		assertNestedField(g, obj, (time.Duration(topologyDuration) * time.Second).String(), contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Path()...)
		assertNestedField(g, obj, (time.Duration(topologyDuration) * time.Second).String(), contract.ControlPlane().MachineTemplate().NodeDeletionTimeout().Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Path()...)

		// Ensure no ownership is added to generated ControlPlane.
		g.Expect(obj.GetOwnerReferences()).To(BeEmpty())
	})
	t.Run("Generates the ControlPlane from the template (v1beta2 contract)", func(t *testing.T) {
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

		obj, err := (&generator{Client: clientWithV1Beta2ContractCRD}).computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.TemplateRef,
			template:          blueprint.ControlPlane.Template,
			currentObjectName: "",
			obj:               obj,
			labels:            util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations:       util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
		})

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedField(g, obj, int64(replicas), contract.ControlPlane().Replicas().Path()...)
		assertNestedField(g, obj, expectedReadinessGates, contract.ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Path()...)
		assertNestedField(g, obj, int64(topologyDuration), contract.ControlPlane().MachineTemplate().NodeDrainTimeoutSeconds().Path()...)
		assertNestedField(g, obj, int64(topologyDuration), contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeoutSeconds().Path()...)
		assertNestedField(g, obj, int64(topologyDuration), contract.ControlPlane().MachineTemplate().NodeDeletionTimeoutSeconds().Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().MachineTemplate().InfrastructureRef().Path()...)

		// Ensure no ownership is added to generated ControlPlane.
		g.Expect(obj.GetOwnerReferences()).To(BeEmpty())
	})
	t.Run("Generates the ControlPlane from the template using ClusterClass defaults (v1beta1 contract)", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: clusterv1.ClusterSpec{
				Topology: clusterv1.Topology{
					Version: version,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Metadata: clusterv1.ObjectMeta{
							Labels:      map[string]string{"l2": ""},
							Annotations: map[string]string{"a2": ""},
						},
						Replicas: &replicas,
						// no values for ReadinessGates, NodeDrainTimeoutSeconds, NodeVolumeDetachTimeoutSeconds, NodeDeletionTimeoutSeconds
					},
				},
			},
		}

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

		obj, err := (&generator{Client: clientWithV1Beta1ContractCRD}).computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// checking only values from CC defaults
		assertNestedField(g, obj, expectedClusterClassReadinessGates, contract.ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Path()...)
		assertNestedField(g, obj, (time.Duration(clusterClassDuration) * time.Second).String(), contract.ControlPlane().MachineTemplate().NodeDrainTimeout().Path()...)
		assertNestedField(g, obj, (time.Duration(clusterClassDuration) * time.Second).String(), contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Path()...)
		assertNestedField(g, obj, (time.Duration(clusterClassDuration) * time.Second).String(), contract.ControlPlane().MachineTemplate().NodeDeletionTimeout().Path()...)
	})
	t.Run("Generates the ControlPlane from the template using ClusterClass defaults (v1beta2 contract)", func(t *testing.T) {
		g := NewWithT(t)

		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cluster1",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: clusterv1.ClusterSpec{
				Topology: clusterv1.Topology{
					Version: version,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Metadata: clusterv1.ObjectMeta{
							Labels:      map[string]string{"l2": ""},
							Annotations: map[string]string{"a2": ""},
						},
						Replicas: &replicas,
						// no values for ReadinessGates, NodeDrainTimeoutSeconds, NodeVolumeDetachTimeoutSeconds, NodeDeletionTimeoutSeconds
					},
				},
			},
		}

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

		obj, err := (&generator{Client: clientWithV1Beta2ContractCRD}).computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// checking only values from CC defaults
		assertNestedField(g, obj, expectedClusterClassReadinessGates, contract.ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Path()...)
		assertNestedField(g, obj, int64(clusterClassDuration), contract.ControlPlane().MachineTemplate().NodeDrainTimeoutSeconds().Path()...)
		assertNestedField(g, obj, int64(clusterClassDuration), contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeoutSeconds().Path()...)
		assertNestedField(g, obj, int64(clusterClassDuration), contract.ControlPlane().MachineTemplate().NodeDeletionTimeoutSeconds().Path()...)
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

		obj, err := (&generator{Client: clientWithV1Beta2ContractCRD}).computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.TemplateRef,
			template:          blueprint.ControlPlane.Template,
			currentObjectName: "",
			obj:               obj,
			labels:            util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations:       util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
		})

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().Replicas().Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().MachineTemplate().InfrastructureRef().Path()...)
	})
	t.Run("Skips setting readinessGates if not set in Cluster and ClusterClass", func(t *testing.T) {
		g := NewWithT(t)

		clusterClassWithoutReadinessGates := clusterClass.DeepCopy()
		clusterClassWithoutReadinessGates.Spec.ControlPlane.ReadinessGates = nil

		clusterWithoutReadinessGates := cluster.DeepCopy()
		clusterWithoutReadinessGates.Spec.Topology.ControlPlane.ReadinessGates = nil

		blueprint := &scope.ClusterBlueprint{
			Topology:     clusterWithoutReadinessGates.Spec.Topology,
			ClusterClass: clusterClassWithoutReadinessGates,
			ControlPlane: &scope.ControlPlaneBlueprint{
				Template: controlPlaneTemplate,
			},
		}

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(clusterWithoutReadinessGates)
		scope.Blueprint = blueprint

		obj, err := (&generator{Client: clientWithV1Beta2ContractCRD}).computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertNestedFieldUnset(g, obj, contract.ControlPlane().MachineTemplate().ReadinessGates("v1beta1").Path()...)
		assertNestedFieldUnset(g, obj, contract.ControlPlane().MachineTemplate().ReadinessGates("v1beta2").Path()...)
	})
	t.Run("Generates the ControlPlane from the template and adds the infrastructure machine template if required (v1beta1 contract)", func(t *testing.T) {
		g := NewWithT(t)

		// templates and ClusterClass
		infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").Build()
		clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
			WithControlPlaneMetadata(labels, annotations).
			WithControlPlaneTemplate(controlPlaneTemplateWithMachineTemplate).
			WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).Build()

		// aggregating templates and cluster class into a blueprint (simulating getBlueprint)
		blueprint := &scope.ClusterBlueprint{
			Topology:     cluster.Spec.Topology,
			ClusterClass: clusterClass,
			ControlPlane: &scope.ControlPlaneBlueprint{
				Template:                      controlPlaneTemplateWithMachineTemplate,
				InfrastructureMachineTemplate: infrastructureMachineTemplate,
			},
		}

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		s := scope.New(cluster)
		s.Blueprint = blueprint
		s.Current.ControlPlane = &scope.ControlPlaneState{}

		obj, err := (&generator{Client: clientWithV1Beta1ContractCRD}).computeControlPlane(ctx, s, infrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// machineTemplate is removed from the template for assertion as we can't
		// simply compare the machineTemplate in template with the one in object as
		// computeControlPlane() adds additional fields like the timeouts to machineTemplate.
		// Note: machineTemplate ia asserted further down below instead.
		controlPlaneTemplateWithoutMachineTemplate := blueprint.ControlPlane.Template.DeepCopy()
		unstructured.RemoveNestedField(controlPlaneTemplateWithoutMachineTemplate.Object, "spec", "template", "spec", "machineTemplate")

		assertTemplateToObject(g, assertTemplateInput{
			cluster:           s.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.TemplateRef,
			template:          controlPlaneTemplateWithoutMachineTemplate,
			currentObjectName: "",
			obj:               obj,
			labels:            util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations:       util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
		})
		gotMetadata, err := contract.ControlPlane().MachineTemplate().Metadata().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())

		expectedLabels := util.MergeMap(s.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels, controlPlaneMachineTemplateLabels)
		expectedLabels[clusterv1.ClusterNameLabel] = cluster.Name
		expectedLabels[clusterv1.ClusterTopologyOwnedLabel] = ""
		g.Expect(gotMetadata).To(BeComparableTo(&clusterv1.ObjectMeta{
			Labels:      expectedLabels,
			Annotations: util.MergeMap(s.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations, controlPlaneMachineTemplateAnnotations),
		}))

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedField(g, obj, int64(replicas), contract.ControlPlane().Replicas().Path()...)
		assertNestedField(g, obj, map[string]interface{}{
			"kind":       infrastructureMachineTemplate.GetKind(),
			"namespace":  infrastructureMachineTemplate.GetNamespace(),
			"name":       infrastructureMachineTemplate.GetName(),
			"apiVersion": infrastructureMachineTemplate.GetAPIVersion(),
		}, contract.ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Path()...)

		// Ensure version is preserved if CP provider bumped the version.
		// Note: This is only necessary with the v1beta1 contract, as the ref in v1beta2 contract has apiGroup instead of apiVersion.

		// Simulate version bump by CP provider
		cpInfraRef, err := contract.ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())
		cpInfraRef.APIVersion = cpInfraRef.GroupVersionKind().Group + "/v99"
		g.Expect(contract.ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Set(obj, cpInfraRef)).To(Succeed())
		s.Current.ControlPlane = &scope.ControlPlaneState{Object: obj}

		obj, err = (&generator{Client: clientWithV1Beta1ContractCRD}).computeControlPlane(ctx, s, infrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// Verify bumped version was preserved
		cpInfraRef, err = contract.ControlPlane().MachineTemplate().InfrastructureV1Beta1Ref().Get(s.Current.ControlPlane.Object)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cpInfraRef.GroupVersionKind().Version).To(Equal("v99"))
	})
	t.Run("Generates the ControlPlane from the template and adds the infrastructure machine template if required (v1beta2 contract)", func(t *testing.T) {
		g := NewWithT(t)

		// templates and ClusterClass
		infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "template1").Build()
		clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
			WithControlPlaneMetadata(labels, annotations).
			WithControlPlaneTemplate(controlPlaneTemplateWithMachineTemplate).
			WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).Build()

		// aggregating templates and cluster class into a blueprint (simulating getBlueprint)
		blueprint := &scope.ClusterBlueprint{
			Topology:     cluster.Spec.Topology,
			ClusterClass: clusterClass,
			ControlPlane: &scope.ControlPlaneBlueprint{
				Template:                      controlPlaneTemplateWithMachineTemplate,
				InfrastructureMachineTemplate: infrastructureMachineTemplate,
			},
		}

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		s := scope.New(cluster)
		s.Blueprint = blueprint
		s.Current.ControlPlane = &scope.ControlPlaneState{}

		obj, err := (&generator{Client: clientWithV1Beta2ContractCRD}).computeControlPlane(ctx, s, infrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// machineTemplate is removed from the template for assertion as we can't
		// simply compare the machineTemplate in template with the one in object as
		// computeControlPlane() adds additional fields like the timeouts to machineTemplate.
		// Note: machineTemplate ia asserted further down below instead.
		controlPlaneTemplateWithoutMachineTemplate := blueprint.ControlPlane.Template.DeepCopy()
		unstructured.RemoveNestedField(controlPlaneTemplateWithoutMachineTemplate.Object, "spec", "template", "spec", "machineTemplate")

		assertTemplateToObject(g, assertTemplateInput{
			cluster:           s.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.TemplateRef,
			template:          controlPlaneTemplateWithoutMachineTemplate,
			currentObjectName: "",
			obj:               obj,
			labels:            util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations:       util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
		})
		gotMetadata, err := contract.ControlPlane().MachineTemplate().Metadata().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())

		expectedLabels := util.MergeMap(s.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels, controlPlaneMachineTemplateLabels)
		expectedLabels[clusterv1.ClusterNameLabel] = cluster.Name
		expectedLabels[clusterv1.ClusterTopologyOwnedLabel] = ""
		g.Expect(gotMetadata).To(BeComparableTo(&clusterv1.ObjectMeta{
			Labels:      expectedLabels,
			Annotations: util.MergeMap(s.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations, controlPlaneMachineTemplateAnnotations),
		}))

		assertNestedField(g, obj, version, contract.ControlPlane().Version().Path()...)
		assertNestedField(g, obj, int64(replicas), contract.ControlPlane().Replicas().Path()...)
		assertNestedField(g, obj, map[string]interface{}{
			"kind":     infrastructureMachineTemplate.GetKind(),
			"name":     infrastructureMachineTemplate.GetName(),
			"apiGroup": infrastructureMachineTemplate.GroupVersionKind().Group,
		}, contract.ControlPlane().MachineTemplate().InfrastructureRef().Path()...)
	})
	t.Run("If there is already a reference to the ControlPlane, it preserves the reference name", func(t *testing.T) {
		g := NewWithT(t)

		// current cluster objects for the test scenario
		clusterWithControlPlaneRef := cluster.DeepCopy()
		clusterWithControlPlaneRef.Spec.ControlPlaneRef = fakeContractVersionedRef1

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

		obj, err := (&generator{Client: clientWithV1Beta2ContractCRD}).computeControlPlane(ctx, scope, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:           scope.Current.Cluster,
			templateRef:       blueprint.ClusterClass.Spec.ControlPlane.TemplateRef,
			template:          blueprint.ControlPlane.Template,
			currentObjectName: scope.Current.Cluster.Spec.ControlPlaneRef.Name,
			obj:               obj,
			labels:            util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels),
			annotations:       util.MergeMap(blueprint.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
		})
	})
	t.Run("Should choose the correct version for control plane", func(t *testing.T) {
		// Note: in all the following tests we are setting it up so that there are not machine deployments.
		// A more extensive list of scenarios is tested in TestComputeControlPlaneVersion.
		tests := []struct {
			name                string
			currentControlPlane *unstructured.Unstructured
			topologyVersion     string
			upgradePlan         []string
			expectedVersion     string
		}{
			{
				name:                "use cluster.spec.topology.version if creating a new control plane",
				currentControlPlane: nil,
				topologyVersion:     "v1.2.3",
				expectedVersion:     "v1.2.3",
			},
			{
				name: "use cluster.spec.topology.version if the control plane is already up to date",
				currentControlPlane: builder.ControlPlane("test1", "cp1").
					WithSpecFields(map[string]interface{}{
						"spec.version": "v1.2.3",
					}).
					WithStatusFields(map[string]interface{}{
						"status.version": "v1.2.3",
					}).
					Build(),
				topologyVersion: "v1.2.3",
				upgradePlan:     nil,
				expectedVersion: "v1.2.3",
			},
			{
				name: "use controlplane.spec.version if the control plane's spec.version is not equal to status.version", // NOTE: there are a few other conditions preventing to pick up latest cluster.spec.topology.version (other than is upgrading which is test here); all those conditions are validated in TestComputeControlPlaneVersion
				currentControlPlane: builder.ControlPlane("test1", "cp1").
					WithSpecFields(map[string]interface{}{
						"spec.version": "v1.2.2",
					}).
					WithStatusFields(map[string]interface{}{
						"status.version": "v1.2.1",
					}).
					Build(),
				topologyVersion: "v1.2.3",
				upgradePlan:     []string{"v1.2.3"},
				expectedVersion: "v1.2.2",
			},
			{
				name: "use cluster.spec.topology.version if the control plane can upgrade and it is a simple upgrade",
				currentControlPlane: builder.ControlPlane("test1", "cp1").
					WithSpecFields(map[string]interface{}{
						"spec.version":  "v1.2.2",
						"spec.replicas": int64(2),
					}).
					WithStatusFields(map[string]interface{}{
						"status.version":  "v1.2.2",
						"status.replicas": int64(2),
					}).
					Build(),
				topologyVersion: "v1.2.3",
				upgradePlan:     []string{"v1.2.3"}, // Simple upgrade
				expectedVersion: "v1.2.3",
			},
			{
				name: "use intermediate version if the control plane can upgrade and it is a multistep upgrade",
				currentControlPlane: builder.ControlPlane("test1", "cp1").
					WithSpecFields(map[string]interface{}{
						"spec.version":  "v1.2.2",
						"spec.replicas": int64(2),
					}).
					WithStatusFields(map[string]interface{}{
						"status.version":  "v1.2.2",
						"status.replicas": int64(2),
					}).
					Build(),
				topologyVersion: "v1.5.3",
				upgradePlan:     []string{"v1.3.2", "v1.4.2", "v1.5.3"}, // Multistep upgrade
				expectedVersion: "v1.3.2",
			},
			{
				name: "use cluster.spec.topology.version if the control plane can upgrade and we are at the last step of a multistep upgrade",
				currentControlPlane: builder.ControlPlane("test1", "cp1").
					WithSpecFields(map[string]interface{}{
						"spec.version":  "v1.4.2",
						"spec.replicas": int64(2),
					}).
					WithStatusFields(map[string]interface{}{
						"status.version":  "v1.4.2",
						"status.replicas": int64(2),
					}).
					Build(),
				topologyVersion: "v1.5.3",
				upgradePlan:     []string{"v1.5.3"},
				expectedVersion: "v1.5.3",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				// Current cluster objects for the test scenario.
				clusterWithControlPlaneRef := cluster.DeepCopy()
				clusterWithControlPlaneRef.Spec.ControlPlaneRef = fakeContractVersionedRef1
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
				s.UpgradeTracker = scope.NewUpgradeTracker()
				s.UpgradeTracker.ControlPlane.UpgradePlan = tt.upgradePlan

				obj, err := (&generator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(crdV1Beta2Contract, clusterWithControlPlaneRef).Build()}).computeControlPlane(ctx, s, nil)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(obj).NotTo(BeNil())
				assertNestedField(g, obj, tt.expectedVersion, contract.ControlPlane().Version().Path()...)
			})
		}
	})
	t.Run("Carry over the owner reference to ClusterShim, if any", func(t *testing.T) {
		g := NewWithT(t)
		shim := clustershim.New(cluster)

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
		s := scope.New(clusterWithoutReplicas)
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.2",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.1",
				}).
				Build(),
		}
		s.Current.ControlPlane.Object.SetOwnerReferences([]metav1.OwnerReference{*ownerrefs.OwnerReferenceTo(shim, corev1.SchemeGroupVersion.WithKind("Secret"))})
		s.Blueprint = blueprint

		obj, err := (&generator{Client: clientWithV1Beta2ContractCRD}).computeControlPlane(ctx, s, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())
		g.Expect(ownerrefs.HasOwnerReferenceFrom(obj, shim)).To(BeTrue())
	})
}

func TestComputeControlPlaneVersion(t *testing.T) {
	var testGVKs = []schema.GroupVersionKind{
		{
			Group:   "refAPIGroup1",
			Kind:    "refKind1",
			Version: "v1beta4",
		},
	}

	apiVersionGetter := func(gk schema.GroupKind) (string, error) {
		for _, gvk := range testGVKs {
			if gvk.GroupKind() == gk {
				return schema.GroupVersion{
					Group:   gk.Group,
					Version: gvk.Version,
				}.String(), nil
			}
		}
		return "", fmt.Errorf("unknown GroupVersionKind: %v", gk)
	}
	clusterv1beta1.SetAPIVersionGetter(apiVersionGetter)

	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	catalog := runtimecatalog.New()
	_ = runtimehooksv1.AddToCatalog(catalog)
	beforeClusterUpgradeGVH, _ := catalog.GroupVersionHook(runtimehooksv1.BeforeClusterUpgrade)
	beforeControlPlaneUpgradeGVH, _ := catalog.GroupVersionHook(runtimehooksv1.BeforeControlPlaneUpgrade)
	beforeWorkersUpgradeGVH, _ := catalog.GroupVersionHook(runtimehooksv1.BeforeWorkersUpgrade)
	afterWorkersUpgradeGVH, _ := catalog.GroupVersionHook(runtimehooksv1.AfterWorkersUpgrade)

	nonBlockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(10),
		},
	}
	failureBeforeClusterUpgradeResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusFailure,
			},
		},
	}

	nonBlockingBeforeControlPlaneUpgradeResponse := &runtimehooksv1.BeforeControlPlaneUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeControlPlaneUpgradeResponse := &runtimehooksv1.BeforeControlPlaneUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(10),
		},
	}
	failureBeforeControlPlaneUpgradeResponse := &runtimehooksv1.BeforeControlPlaneUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusFailure,
			},
		},
	}

	nonBlockingBeforeWorkersUpgradeResponse := &runtimehooksv1.BeforeWorkersUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingBeforeWorkersUpgradeResponse := &runtimehooksv1.BeforeWorkersUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(10),
		},
	}
	failureBeforeWorkersUpgradeResponse := &runtimehooksv1.BeforeWorkersUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusFailure,
			},
		},
	}

	nonBlockingAfterWorkersUpgradeResponse := &runtimehooksv1.AfterWorkersUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
		},
	}
	blockingAfterWorkersUpgradeResponse := &runtimehooksv1.AfterWorkersUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusSuccess,
			},
			RetryAfterSeconds: int32(10),
		},
	}
	failureAfterWorkersUpgradeResponse := &runtimehooksv1.AfterWorkersUpgradeResponse{
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: runtimehooksv1.ResponseStatusFailure,
			},
		},
	}

	tests := []struct {
		name                               string
		beforeClusterUpgradeResponse       *runtimehooksv1.BeforeClusterUpgradeResponse
		beforeControlPlaneUpgradeResponse  *runtimehooksv1.BeforeControlPlaneUpgradeResponse
		beforeWorkersUpgradeResponse       *runtimehooksv1.BeforeWorkersUpgradeResponse
		afterWorkersUpgradeResponse        *runtimehooksv1.AfterWorkersUpgradeResponse
		topologyVersion                    string
		clusterModifier                    func(c *clusterv1.Cluster)
		controlPlaneObj                    *unstructured.Unstructured
		controlPlaneUpgradePlan            []string
		machineDeploymentsUpgradePlan      []string
		machinePoolsUpgradePlan            []string
		upgradingMachineDeployments        []string
		upgradingMachinePools              []string
		expectedVersion                    string
		expectedIsPendingUpgrade           bool
		expectedIsStartingUpgrade          bool
		expectedIsWaitingForWorkersUpgrade bool
		wantErr                            bool
	}{
		{
			name:                      "should return cluster.spec.topology.version if creating a new control plane",
			topologyVersion:           "v1.2.3",
			controlPlaneObj:           nil,
			expectedVersion:           "v1.2.3",
			expectedIsPendingUpgrade:  false,
			expectedIsStartingUpgrade: false,
		},
		{
			name:            "should return cluster.spec.topology.version if the control plane is already at the target version",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version": "v1.2.3",
				}).
				WithStatusFields(map[string]interface{}{
					"status.version": "v1.2.3",
				}).
				Build(),
			controlPlaneUpgradePlan:   nil,
			expectedVersion:           "v1.2.3",
			expectedIsPendingUpgrade:  false,
			expectedIsStartingUpgrade: false,
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
			controlPlaneUpgradePlan:   []string{"v1.2.3"},
			expectedVersion:           "v1.2.2",
			expectedIsPendingUpgrade:  true,
			expectedIsStartingUpgrade: false,
		},
		{
			name:            "should return controlplane.spec.version if control plane is not upgrading and not scaling and one of the MachineDeployments and one of the MachinePools is upgrading",
			topologyVersion: "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan:     []string{"v1.2.3"},
			upgradingMachineDeployments: []string{"md1"},
			upgradingMachinePools:       []string{"mp1"},
			expectedVersion:             "v1.2.2",
			expectedIsPendingUpgrade:    true,
			expectedIsStartingUpgrade:   false,
		},
		{
			name:                              "should return cluster.spec.topology.version if control plane is not upgrading and not scaling and none of the MachineDeployments and MachinePools are upgrading - BeforeClusterUpgrade, BeforeControlPlaneUpgrade, BeforeWorkersUpgrade and AfterWorkersUpgrade hooks returns non blocking response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			beforeWorkersUpgradeResponse:      nonBlockingBeforeWorkersUpgradeResponse,
			afterWorkersUpgradeResponse:       nonBlockingAfterWorkersUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			clusterModifier: func(c *clusterv1.Cluster) {
				c.Annotations = map[string]string{
					runtimev1.PendingHooksAnnotation: "BeforeWorkersUpgrade,AfterWorkersUpgrade",
				}
			},
			controlPlaneUpgradePlan:     []string{"v1.2.3"},
			upgradingMachineDeployments: []string{},
			upgradingMachinePools:       []string{},
			expectedVersion:             "v1.2.3",
			expectedIsPendingUpgrade:    false,
			expectedIsStartingUpgrade:   true,
		},
		{
			name:                              "should return cluster.spec.topology.version if the control plane is not upgrading or scaling and none of the MachineDeployments and MachinePools are upgrading - BeforeClusterUpgrade, BeforeControlPlaneUpgrade hooks returns non blocking response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(1),
				}).
				Build(),
			controlPlaneUpgradePlan:   []string{"v1.2.3"},
			expectedVersion:           "v1.2.3",
			expectedIsPendingUpgrade:  false,
			expectedIsStartingUpgrade: true,
		},
		{
			name:                              "should return an intermediate version when upgrading by more than 1 minor and control plane should perform the first step of the upgrade sequence",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			topologyVersion:                   "v1.5.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan:     []string{"v1.3.2", "v1.4.2", "v1.5.3"},
			upgradingMachineDeployments: []string{},
			upgradingMachinePools:       []string{},
			expectedVersion:             "v1.3.2", // first step of the upgrade plan
			expectedIsPendingUpgrade:    false,    // there are still upgrade in the queue, but we are starting one (so not pending)
			expectedIsStartingUpgrade:   true,
		},
		{
			name:                              "should return cluster.spec.topology.version when performing a multi step upgrade and control plane is at the second last minor in the upgrade sequence",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			topologyVersion:                   "v1.5.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.4.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.4.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan:     []string{"v1.5.3"},
			upgradingMachineDeployments: []string{},
			upgradingMachinePools:       []string{},
			expectedVersion:             "v1.5.3", // last step of the upgrade plan
			expectedIsPendingUpgrade:    false,
			expectedIsStartingUpgrade:   true,
		},
		{
			name:                         "should remain on the current version when upgrading by more than 1 minor and MachineDeployments have to upgrade",
			beforeClusterUpgradeResponse: nonBlockingBeforeClusterUpgradeResponse,
			topologyVersion:              "v1.5.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan:            []string{"v1.3.2", "v1.4.2", "v1.5.3"},
			machineDeploymentsUpgradePlan:      []string{"v1.2.2"},
			upgradingMachineDeployments:        []string{},
			upgradingMachinePools:              []string{},
			expectedVersion:                    "v1.2.2",
			expectedIsPendingUpgrade:           true,
			expectedIsWaitingForWorkersUpgrade: true,
			expectedIsStartingUpgrade:          false,
		},
		{
			name:                         "should remain on the current version when upgrading by more than 1 minor and MachinePools have to upgrade",
			beforeClusterUpgradeResponse: nonBlockingBeforeClusterUpgradeResponse,
			topologyVersion:              "v1.5.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan:            []string{"v1.3.2", "v1.4.2", "v1.5.3"},
			machinePoolsUpgradePlan:            []string{"v1.2.2"},
			upgradingMachineDeployments:        []string{},
			upgradingMachinePools:              []string{},
			expectedVersion:                    "v1.2.2",
			expectedIsPendingUpgrade:           true,
			expectedIsWaitingForWorkersUpgrade: true,
			expectedIsStartingUpgrade:          false,
		},
		{
			name:                         "should return the controlplane.spec.version if a BeforeClusterUpgradeHook returns a blocking response",
			beforeClusterUpgradeResponse: blockingBeforeClusterUpgradeResponse,
			topologyVersion:              "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan:   []string{"v1.2.3"},
			expectedVersion:           "v1.2.2",
			expectedIsPendingUpgrade:  true,
			expectedIsStartingUpgrade: false,
		},
		{
			name:                         "should fail if the BeforeClusterUpgrade hooks returns a failure response",
			beforeClusterUpgradeResponse: failureBeforeClusterUpgradeResponse,
			topologyVersion:              "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan: []string{"v1.2.3"},
			wantErr:                 true,
		},
		{
			name:                         "should return the controlplane.spec.version if a BeforeClusterUpgradeHook annotation is set",
			beforeClusterUpgradeResponse: nonBlockingBeforeClusterUpgradeResponse,
			topologyVersion:              "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			clusterModifier: func(c *clusterv1.Cluster) {
				c.Annotations = map[string]string{
					clusterv1.BeforeClusterUpgradeHookAnnotationPrefix + "/test": "true",
				}
			},
			controlPlaneUpgradePlan:   []string{"v1.2.3"},
			expectedVersion:           "v1.2.2",
			expectedIsPendingUpgrade:  true,
			expectedIsStartingUpgrade: false,
			wantErr:                   false,
		},
		{
			name:                              "should return the controlplane.spec.version if a BeforeControlPlaneUpgrade returns a blocking response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: blockingBeforeControlPlaneUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan:   []string{"v1.2.3"},
			expectedVersion:           "v1.2.2",
			expectedIsPendingUpgrade:  true,
			expectedIsStartingUpgrade: false,
		},
		{
			name:                              "should fail if the BeforeControlPlaneUpgrade hooks returns a failure response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: failureBeforeControlPlaneUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			controlPlaneUpgradePlan: []string{"v1.2.3"},
			wantErr:                 true,
		},
		{
			name:                              "should return the controlplane.spec.version if a AfterWorkersUpgrade returns a blocking response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			afterWorkersUpgradeResponse:       blockingAfterWorkersUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			clusterModifier: func(c *clusterv1.Cluster) {
				c.Annotations = map[string]string{
					runtimev1.PendingHooksAnnotation: "AfterWorkersUpgrade",
				}
			},
			controlPlaneUpgradePlan:   []string{"v1.2.3"},
			expectedVersion:           "v1.2.2",
			expectedIsPendingUpgrade:  true,
			expectedIsStartingUpgrade: false,
		},
		{
			name:                              "should fail if the AfterWorkersUpgrade hooks returns a failure response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			afterWorkersUpgradeResponse:       failureAfterWorkersUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			clusterModifier: func(c *clusterv1.Cluster) {
				c.Annotations = map[string]string{
					runtimev1.PendingHooksAnnotation: "AfterWorkersUpgrade",
				}
			},
			controlPlaneUpgradePlan: []string{"v1.2.3"},
			wantErr:                 true,
		},
		{
			name:                              "should return the controlplane.spec.version if a BeforeWorkersUpgrade returns a blocking response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			beforeWorkersUpgradeResponse:      blockingBeforeWorkersUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			clusterModifier: func(c *clusterv1.Cluster) {
				c.Annotations = map[string]string{
					runtimev1.PendingHooksAnnotation: "BeforeWorkersUpgrade",
				}
			},
			controlPlaneUpgradePlan:   []string{"v1.2.3"},
			expectedVersion:           "v1.2.2",
			expectedIsPendingUpgrade:  true,
			expectedIsStartingUpgrade: false,
		},
		{
			name:                              "should fail if the BeforeWorkersUpgrade hooks returns a failure response",
			beforeClusterUpgradeResponse:      nonBlockingBeforeClusterUpgradeResponse,
			beforeControlPlaneUpgradeResponse: nonBlockingBeforeControlPlaneUpgradeResponse,
			beforeWorkersUpgradeResponse:      failureBeforeWorkersUpgradeResponse,
			topologyVersion:                   "v1.2.3",
			controlPlaneObj: builder.ControlPlane("test1", "cp1").
				WithSpecFields(map[string]interface{}{
					"spec.version":  "v1.2.2",
					"spec.replicas": int64(2),
				}).
				WithStatusFields(map[string]interface{}{
					"status.version":  "v1.2.2",
					"status.replicas": int64(2),
				}).
				Build(),
			clusterModifier: func(c *clusterv1.Cluster) {
				c.Annotations = map[string]string{
					runtimev1.PendingHooksAnnotation: "BeforeWorkersUpgrade",
				}
			},
			controlPlaneUpgradePlan: []string{"v1.2.3"},
			wantErr:                 true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{Topology: clusterv1.Topology{
					Version: tt.topologyVersion,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Replicas: ptr.To[int32](2),
					},
				}},
				Current: &scope.ClusterState{
					Cluster: &clusterv1.Cluster{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-cluster",
							Namespace: "test-ns",
							// Add managedFields and annotations that should be cleaned up before the Cluster is sent to the RuntimeExtension.
							ManagedFields: []metav1.ManagedFieldsEntry{
								{
									APIVersion: builder.InfrastructureGroupVersion.String(),
									Manager:    "manager",
									Operation:  "Apply",
									Time:       ptr.To(metav1.Now()),
									FieldsType: "FieldsV1",
								},
							},
							Annotations: map[string]string{
								"fizz":                             "buzz",
								corev1.LastAppliedConfigAnnotation: "should be cleaned up",
								conversion.DataAnnotation:          "should be cleaned up",
							},
						},
						// Add some more fields to check that conversion implemented when calling RuntimeExtension are properly handled.
						Spec: clusterv1.ClusterSpec{
							InfrastructureRef: clusterv1.ContractVersionedObjectReference{
								APIGroup: "refAPIGroup1",
								Kind:     "refKind1",
								Name:     "refName1",
							}},
					},
					ControlPlane: &scope.ControlPlaneState{Object: tt.controlPlaneObj},
				},
				UpgradeTracker:      scope.NewUpgradeTracker(),
				HookResponseTracker: scope.NewHookResponseTracker(),
			}
			if tt.clusterModifier != nil {
				tt.clusterModifier(s.Current.Cluster)
			}
			if len(tt.controlPlaneUpgradePlan) > 0 {
				s.UpgradeTracker.ControlPlane.UpgradePlan = tt.controlPlaneUpgradePlan
			}
			if len(tt.machineDeploymentsUpgradePlan) > 0 {
				s.UpgradeTracker.MachineDeployments.UpgradePlan = tt.machineDeploymentsUpgradePlan
			}
			if len(tt.machinePoolsUpgradePlan) > 0 {
				s.UpgradeTracker.MachinePools.UpgradePlan = tt.machinePoolsUpgradePlan
			}
			if len(tt.upgradingMachineDeployments) > 0 {
				s.UpgradeTracker.MachineDeployments.MarkUpgrading(tt.upgradingMachineDeployments...)
			}
			if len(tt.upgradingMachinePools) > 0 {
				s.UpgradeTracker.MachinePools.MarkUpgrading(tt.upgradingMachinePools...)
			}

			runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
				WithCatalog(catalog).
				WithGetAllExtensionResponses(map[runtimecatalog.GroupVersionHook][]string{
					beforeClusterUpgradeGVH:      {"foo"},
					beforeControlPlaneUpgradeGVH: {"foo"},
					beforeWorkersUpgradeGVH:      {"foo"},
					afterWorkersUpgradeGVH:       {"foo"},
				}).
				WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
					beforeClusterUpgradeGVH:      tt.beforeClusterUpgradeResponse,
					beforeControlPlaneUpgradeGVH: tt.beforeControlPlaneUpgradeResponse,
					beforeWorkersUpgradeGVH:      tt.beforeWorkersUpgradeResponse,
					afterWorkersUpgradeGVH:       tt.afterWorkersUpgradeResponse,
				}).
				WithCallAllExtensionValidations(validateClusterParameter(s.Current.Cluster)).
				Build()

			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(s.Current.Cluster).Build()

			r := &generator{
				Client:        fakeClient,
				RuntimeClient: runtimeClient,
				hookCache:     cache.New[cache.HookEntry](cache.HookCacheDefaultTTL),
			}
			version, err := r.computeControlPlaneVersion(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(version).To(Equal(tt.expectedVersion))
			g.Expect(s.UpgradeTracker.ControlPlane.IsPendingUpgrade).To(Equal(tt.expectedIsPendingUpgrade))
			g.Expect(s.UpgradeTracker.ControlPlane.IsStartingUpgrade).To(Equal(tt.expectedIsStartingUpgrade))
			g.Expect(s.UpgradeTracker.ControlPlane.IsWaitingForWorkersUpgrade).To(Equal(tt.expectedIsWaitingForWorkersUpgrade))
		})
	}
}

func TestComputeCluster(t *testing.T) {
	g := NewWithT(t)

	// generated objects
	infrastructureCluster := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructureCluster1").
		Build()
	controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "controlplane1").
		WithVersion("v1.30.3").
		Build()

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	// aggregating current cluster objects into ClusterState (simulating getCurrentState)
	s := scope.New(cluster)
	s.Current.ControlPlane = &scope.ControlPlaneState{
		Object: controlPlane,
	}

	obj, err := computeCluster(ctx, s, infrastructureCluster, controlPlane)
	g.Expect(obj).ToNot(BeNil())
	g.Expect(err).ToNot(HaveOccurred())

	// TypeMeta
	g.Expect(obj.APIVersion).To(Equal(cluster.APIVersion))
	g.Expect(obj.Kind).To(Equal(cluster.Kind))

	// ObjectMeta
	g.Expect(obj.Name).To(Equal(cluster.Name))
	g.Expect(obj.Namespace).To(Equal(cluster.Namespace))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, cluster.Name))
	g.Expect(obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyOwnedLabel, ""))
	g.Expect(obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.ClusterTopologyUpgradeStepAnnotation, ""))

	// Spec
	g.Expect(obj.Spec.InfrastructureRef).To(BeComparableTo(contract.ObjToContractVersionedObjectReference(infrastructureCluster)))
	g.Expect(obj.Spec.ControlPlaneRef).To(BeComparableTo(contract.ObjToContractVersionedObjectReference(controlPlane)))

	// Surfaces the ClusterTopologyUpgradeStepAnnotation annotation during upgrades.
	annotations := s.Current.Cluster.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[runtimev1.PendingHooksAnnotation] = "AfterClusterUpgrade"
	s.Current.Cluster.SetAnnotations(annotations)

	obj, err = computeCluster(ctx, s, infrastructureCluster, controlPlane)
	g.Expect(obj).ToNot(BeNil())
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.ClusterTopologyUpgradeStepAnnotation, "v1.30.3"))

	// Delete ClusterTopologyUpgradeStepAnnotation annotation after upgrade is completed.
	delete(annotations, runtimev1.PendingHooksAnnotation)
	s.Current.Cluster.SetAnnotations(annotations)

	obj, err = computeCluster(ctx, s, infrastructureCluster, controlPlane)
	g.Expect(obj).ToNot(BeNil())
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(obj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.ClusterTopologyUpgradeStepAnnotation, ""))
}

func TestComputeMachineDeployment(t *testing.T) {
	workerInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "linux-worker-inframachinetemplate").
		Build()
	workerBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "linux-worker-bootstraptemplate").
		Build()
	labels := map[string]string{"fizzLabel": "buzz", "fooLabel": "bar"}
	annotations := map[string]string{"fizzAnnotation": "buzz", "fooAnnotation": "bar"}

	unhealthyNodeConditions := []clusterv1.UnhealthyNodeCondition{
		{
			Type:           corev1.NodeReady,
			Status:         corev1.ConditionUnknown,
			TimeoutSeconds: ptr.To(int32(5 * 60)),
		},
		{
			Type:           corev1.NodeReady,
			Status:         corev1.ConditionFalse,
			TimeoutSeconds: ptr.To(int32(5 * 60)),
		},
	}

	unhealthyMachineConditions := []clusterv1.UnhealthyMachineCondition{
		{
			Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
			Status:         metav1.ConditionUnknown,
			TimeoutSeconds: ptr.To(int32(5 * 60)),
		},
		{
			Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
			Status:         metav1.ConditionFalse,
			TimeoutSeconds: ptr.To(int32(5 * 60)),
		},
	}

	nodeTimeoutDuration := ptr.To(int32(1))

	clusterClassFailureDomain := "A"
	clusterClassDuration := int32(20)
	var clusterClassMinReadySeconds int32 = 20
	clusterClassStrategy := clusterv1.MachineDeploymentClassRolloutStrategy{
		Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
	}
	clusterClassMDStrategy := clusterv1.MachineDeploymentRolloutStrategy{
		Type: clusterv1.OnDeleteMachineDeploymentStrategyType,
	}
	clusterClassHealthCheck := clusterv1.MachineDeploymentClassHealthCheck{
		Remediation: clusterv1.MachineDeploymentClassHealthCheckRemediation{
			MaxInFlight: ptr.To(intstr.FromInt32(5)),
		},
	}
	clusterClassDeletionOrder := clusterv1.NewestMachineSetDeletionOrder
	clusterClassReadinessGates := []clusterv1.MachineReadinessGate{
		{ConditionType: "foo"},
	}
	md1 := builder.MachineDeploymentClass("linux-worker").
		WithLabels(labels).
		WithAnnotations(annotations).
		WithInfrastructureTemplate(workerInfrastructureMachineTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
			Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
				UnhealthyNodeConditions:    unhealthyNodeConditions,
				UnhealthyMachineConditions: unhealthyMachineConditions,
				NodeStartupTimeoutSeconds:  nodeTimeoutDuration,
			},
		}).
		WithReadinessGates(clusterClassReadinessGates).
		WithFailureDomain(clusterClassFailureDomain).
		WithNodeDrainTimeout(&clusterClassDuration).
		WithNodeVolumeDetachTimeout(&clusterClassDuration).
		WithNodeDeletionTimeout(&clusterClassDuration).
		WithMinReadySeconds(&clusterClassMinReadySeconds).
		WithStrategy(clusterClassStrategy).
		WithDeletionOrder(clusterClassDeletionOrder).
		WithMachineHealthCheckClass(clusterClassHealthCheck).
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
			Topology: clusterv1.Topology{
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
				HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
					Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
						UnhealthyNodeConditions:    unhealthyNodeConditions,
						UnhealthyMachineConditions: unhealthyMachineConditions,
						NodeStartupTimeoutSeconds:  ptr.To(int32(1)),
					},
				},
			},
		},
	}

	replicas := int32(5)
	topologyFailureDomain := "B"
	topologyDuration := int32(10)
	var topologyMinReadySeconds int32 = 10
	topologyStrategy := clusterv1.MachineDeploymentTopologyRolloutStrategy{
		Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
	}
	topologyMDStrategy := clusterv1.MachineDeploymentRolloutStrategy{
		Type: clusterv1.RollingUpdateMachineDeploymentStrategyType,
	}
	topologyHealthCheck := clusterv1.MachineDeploymentTopologyHealthCheck{
		Remediation: clusterv1.MachineDeploymentTopologyHealthCheckRemediation{
			MaxInFlight: ptr.To(intstr.FromInt32(1)),
		},
	}
	topologyDeletionOrder := clusterv1.OldestMachineSetDeletionOrder
	readinessGates := []clusterv1.MachineReadinessGate{
		{ConditionType: "foo"},
		{ConditionType: "bar"},
	}
	mdTopology := clusterv1.MachineDeploymentTopology{
		Metadata: clusterv1.ObjectMeta{
			Labels: map[string]string{
				// Should overwrite the label from the MachineDeployment class.
				"fooLabel": "baz",
			},
			Annotations: map[string]string{
				// Should overwrite the annotation from the MachineDeployment class.
				"fooAnnotation": "baz",
				// These annotations should not be propagated to the MachineDeployment.
				clusterv1.ClusterTopologyDeferUpgradeAnnotation:        "",
				clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation: "",
			},
		},
		Class:          "linux-worker",
		Name:           "big-pool-of-machines",
		Replicas:       &replicas,
		FailureDomain:  topologyFailureDomain,
		ReadinessGates: readinessGates,
		HealthCheck:    topologyHealthCheck,
		Deletion: clusterv1.MachineDeploymentTopologyMachineDeletionSpec{
			Order:                          topologyDeletionOrder,
			NodeDrainTimeoutSeconds:        &topologyDuration,
			NodeVolumeDetachTimeoutSeconds: &topologyDuration,
			NodeDeletionTimeoutSeconds:     &topologyDuration,
		},
		MinReadySeconds: &topologyMinReadySeconds,
		Rollout: clusterv1.MachineDeploymentTopologyRolloutSpec{
			Strategy: topologyStrategy,
		},
	}

	t.Run("Generates the machine deployment and the referenced templates", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		e := generator{}

		actual, err := e.computeMachineDeployment(ctx, scope, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(actual.BootstrapTemplate.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentNameLabel, "big-pool-of-machines"))

		// Ensure Cluster ownership is added to generated BootstrapTemplate.
		g.Expect(actual.BootstrapTemplate.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(actual.BootstrapTemplate.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(actual.BootstrapTemplate.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))

		g.Expect(actual.InfrastructureMachineTemplate.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachineDeploymentNameLabel, "big-pool-of-machines"))

		// Ensure Cluster ownership is added to generated InfrastructureMachineTemplate.
		g.Expect(actual.InfrastructureMachineTemplate.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(actual.InfrastructureMachineTemplate.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(actual.InfrastructureMachineTemplate.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))

		actualMd := actual.Object
		g.Expect(*actualMd.Spec.Replicas).To(Equal(replicas))
		g.Expect(actualMd.Spec.Rollout.Strategy).To(BeComparableTo(topologyMDStrategy))
		g.Expect(actualMd.Spec.Template.Spec.MinReadySeconds).To(HaveValue(Equal(topologyMinReadySeconds)))
		g.Expect(actualMd.Spec.Template.Spec.FailureDomain).To(Equal(topologyFailureDomain))
		g.Expect(actualMd.Spec.Remediation.MaxInFlight).To(Equal(topologyHealthCheck.Remediation.MaxInFlight))
		g.Expect(actualMd.Spec.Deletion.Order).To(Equal(topologyDeletionOrder))
		g.Expect(*actualMd.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).To(Equal(topologyDuration))
		g.Expect(*actualMd.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).To(Equal(topologyDuration))
		g.Expect(*actualMd.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(topologyDuration))
		g.Expect(actualMd.Spec.Template.Spec.ReadinessGates).To(Equal(readinessGates))
		g.Expect(actualMd.Spec.ClusterName).To(Equal("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("cluster1"))
		g.Expect(actualMd.Name).To(ContainSubstring("big-pool-of-machines"))

		expectedAnnotations := util.MergeMap(mdTopology.Metadata.Annotations, md1.Metadata.Annotations)
		delete(expectedAnnotations, clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation)
		delete(expectedAnnotations, clusterv1.ClusterTopologyDeferUpgradeAnnotation)
		g.Expect(actualMd.Annotations).To(Equal(expectedAnnotations))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations))

		g.Expect(actualMd.Labels).To(BeComparableTo(util.MergeMap(mdTopology.Metadata.Labels, md1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "big-pool-of-machines",
		})))
		g.Expect(actualMd.Spec.Selector.MatchLabels).To(Equal(map[string]string{
			clusterv1.ClusterNameLabel:                          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "big-pool-of-machines",
		}))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(BeComparableTo(util.MergeMap(mdTopology.Metadata.Labels, md1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "big-pool-of-machines",
		})))

		g.Expect(actualMd.Spec.Template.Spec.InfrastructureRef.Name).ToNot(Equal("linux-worker-inframachinetemplate"))
		g.Expect(actualMd.Spec.Template.Spec.Bootstrap.ConfigRef.Name).ToNot(Equal("linux-worker-bootstraptemplate"))
	})
	t.Run("Generates the machine deployment and the referenced templates using ClusterClass defaults", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		mdTopology := clusterv1.MachineDeploymentTopology{
			Metadata: clusterv1.ObjectMeta{
				Labels: map[string]string{"foo": "baz"},
			},
			Class:    "linux-worker",
			Name:     "big-pool-of-machines",
			Replicas: &replicas,
			// missing ReadinessGates, FailureDomain, NodeDrainTimeoutSeconds, NodeVolumeDetachTimeoutSeconds, NodeDeletionTimeoutSeconds, MinReadySeconds, Strategy, deletion.Order, remediation
		}

		e := generator{}

		actual, err := e.computeMachineDeployment(ctx, scope, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		// checking only values from CC defaults
		actualMd := actual.Object
		g.Expect(actualMd.Spec.Rollout.Strategy).To(BeComparableTo(clusterClassMDStrategy))
		g.Expect(actualMd.Spec.Template.Spec.MinReadySeconds).To(HaveValue(Equal(clusterClassMinReadySeconds)))
		g.Expect(actualMd.Spec.Template.Spec.FailureDomain).To(Equal(clusterClassFailureDomain))
		g.Expect(actualMd.Spec.Template.Spec.ReadinessGates).To(Equal(clusterClassReadinessGates))
		g.Expect(actualMd.Spec.Remediation.MaxInFlight).To(Equal(clusterClassHealthCheck.Remediation.MaxInFlight))
		g.Expect(actualMd.Spec.Deletion.Order).To(Equal(clusterClassDeletionOrder))
		g.Expect(*actualMd.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).To(Equal(clusterClassDuration))
		g.Expect(*actualMd.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).To(Equal(clusterClassDuration))
		g.Expect(*actualMd.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(clusterClassDuration))
	})

	t.Run("Skips setting readinessGates if not set in Cluster and ClusterClass", func(t *testing.T) {
		g := NewWithT(t)

		clusterClassWithoutReadinessGates := fakeClass.DeepCopy()
		clusterClassWithoutReadinessGates.Spec.Workers.MachineDeployments[0].ReadinessGates = nil

		blueprint := &scope.ClusterBlueprint{
			Topology:     cluster.Spec.Topology,
			ClusterClass: clusterClassWithoutReadinessGates,
			MachineDeployments: map[string]*scope.MachineDeploymentBlueprint{
				"linux-worker": {
					Metadata: clusterv1.ObjectMeta{
						Labels:      labels,
						Annotations: annotations,
					},
					BootstrapTemplate:             workerBootstrapTemplate,
					InfrastructureMachineTemplate: workerInfrastructureMachineTemplate,
					HealthCheck: clusterv1.MachineDeploymentClassHealthCheck{
						Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
							UnhealthyNodeConditions:    unhealthyNodeConditions,
							UnhealthyMachineConditions: unhealthyMachineConditions,
							NodeStartupTimeoutSeconds:  ptr.To(int32(1)),
						},
					},
				},
			},
		}

		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		mdTopology := clusterv1.MachineDeploymentTopology{
			Metadata: clusterv1.ObjectMeta{
				Labels: map[string]string{"foo": "baz"},
			},
			Class:    "linux-worker",
			Name:     "big-pool-of-machines",
			Replicas: &replicas,
			// missing ReadinessGates
		}

		e := generator{}

		actual, err := e.computeMachineDeployment(ctx, scope, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		// checking only values from CC defaults
		actualMd := actual.Object
		g.Expect(actualMd.Spec.Template.Spec.ReadinessGates).To(BeNil())
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
						Version: version,
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: contract.ObjToContractVersionedObjectReference(workerBootstrapTemplate),
						},
						InfrastructureRef: contract.ObjToContractVersionedObjectReference(workerInfrastructureMachineTemplate),
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

		e := generator{}

		actual, err := e.computeMachineDeployment(ctx, s, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())

		actualMd := actual.Object

		g.Expect(*actualMd.Spec.Replicas).NotTo(Equal(currentReplicas))
		g.Expect(actualMd.Spec.Template.Spec.FailureDomain).To(Equal(topologyFailureDomain))
		g.Expect(actualMd.Name).To(Equal("existing-deployment-1"))

		expectedAnnotations := util.MergeMap(mdTopology.Metadata.Annotations, md1.Metadata.Annotations)
		delete(expectedAnnotations, clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation)
		delete(expectedAnnotations, clusterv1.ClusterTopologyDeferUpgradeAnnotation)
		g.Expect(actualMd.Annotations).To(Equal(expectedAnnotations))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations))

		g.Expect(actualMd.Labels).To(BeComparableTo(util.MergeMap(mdTopology.Metadata.Labels, md1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "big-pool-of-machines",
		})))
		g.Expect(actualMd.Spec.Selector.MatchLabels).To(BeComparableTo(map[string]string{
			clusterv1.ClusterNameLabel:                          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "big-pool-of-machines",
		}))
		g.Expect(actualMd.Spec.Template.ObjectMeta.Labels).To(BeComparableTo(util.MergeMap(mdTopology.Metadata.Labels, md1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                          cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:                 "",
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: "big-pool-of-machines",
		})))

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

		e := generator{}

		_, err := e.computeMachineDeployment(ctx, scope, mdTopology)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("Should choose the correct version for machine deployment", func(t *testing.T) {
		controlPlaneStable123 := builder.ControlPlane("test1", "cp1").
			WithSpecFields(map[string]interface{}{
				"spec.version":  "v1.2.3",
				"spec.replicas": int64(2),
			}).
			WithStatusFields(map[string]interface{}{
				"status.version":          "v1.2.3",
				"status.replicas":         int64(2),
				"status.upToDateReplicas": int64(2),
				"status.readyReplicas":    int64(2),
			}).
			Build()

		// Note: in all the following tests we are setting it up so that the control plane is already
		// stable at the topology version.
		// A more extensive list of scenarios is tested in TestComputeMachineDeploymentVersion.
		tests := []struct {
			name                        string
			upgradingMachineDeployments []string
			currentMDVersion            *string
			upgradeConcurrency          string
			topologyVersion             string
			upgradePlan                 []string
			expectedVersion             string
		}{
			{
				name:                        "use cluster.spec.topology.version if creating a new machine deployment",
				upgradingMachineDeployments: []string{},
				upgradeConcurrency:          "1",
				currentMDVersion:            nil,
				topologyVersion:             "v1.2.3",
				upgradePlan:                 []string{"v1.2.3"},
				expectedVersion:             "v1.2.3",
			},
			{
				name:                        "use cluster.spec.topology.version if creating a new machine deployment while another machine deployment is upgrading",
				upgradingMachineDeployments: []string{"upgrading-md1"},
				upgradeConcurrency:          "1",
				currentMDVersion:            nil,
				topologyVersion:             "v1.2.3",
				upgradePlan:                 []string{"v1.2.3"},
				expectedVersion:             "v1.2.3",
			},
			{
				name:                        "use machine deployment's spec.template.spec.version if one of the machine deployments is upgrading, concurrency limit reached",
				upgradingMachineDeployments: []string{"upgrading-md1"},
				upgradeConcurrency:          "1",
				currentMDVersion:            ptr.To("v1.2.2"),
				topologyVersion:             "v1.2.3",
				upgradePlan:                 []string{"v1.2.3"},
				expectedVersion:             "v1.2.2",
			},
			{
				name:                        "use cluster.spec.topology.version if one of the machine deployments is upgrading, concurrency limit not reached",
				upgradingMachineDeployments: []string{"upgrading-md1"},
				upgradeConcurrency:          "2",
				currentMDVersion:            ptr.To("v1.2.2"),
				topologyVersion:             "v1.2.3",
				upgradePlan:                 []string{"v1.2.3"},
				expectedVersion:             "v1.2.3",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				testCluster := cluster.DeepCopy()
				if testCluster.Annotations == nil {
					testCluster.Annotations = map[string]string{}
				}
				testCluster.Annotations[clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation] = tt.upgradeConcurrency

				s := scope.New(testCluster)
				s.Blueprint = blueprint
				s.Blueprint.Topology.Version = tt.topologyVersion
				s.Blueprint.Topology.ControlPlane = clusterv1.ControlPlaneTopology{
					Replicas: ptr.To[int32](2),
				}
				s.Blueprint.Topology.Workers = clusterv1.WorkersTopology{}

				mdsState := scope.MachineDeploymentsStateMap{}
				if tt.currentMDVersion != nil {
					// testing a case with an existing machine deployment
					// add the stable machine deployment to the current machine deployments state
					md := builder.MachineDeployment("test-namespace", "big-pool-of-machines").
						WithGeneration(1).
						WithReplicas(2).
						WithVersion(*tt.currentMDVersion).
						WithStatus(clusterv1.MachineDeploymentStatus{
							ObservedGeneration: 2,
							Replicas:           ptr.To[int32](2),
							ReadyReplicas:      ptr.To[int32](2),
							UpToDateReplicas:   ptr.To[int32](2),
							AvailableReplicas:  ptr.To[int32](2),
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

				mdTopology := clusterv1.MachineDeploymentTopology{
					Class:    "linux-worker",
					Name:     "big-pool-of-machines",
					Replicas: ptr.To[int32](2),
				}
				s.UpgradeTracker.MachineDeployments.MarkUpgrading(tt.upgradingMachineDeployments...)
				if tt.upgradePlan != nil {
					s.UpgradeTracker.MachineDeployments.UpgradePlan = tt.upgradePlan
				}

				e := generator{}

				obj, err := e.computeMachineDeployment(ctx, s, mdTopology)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(obj.Object.Spec.Template.Spec.Version).To(Equal(tt.expectedVersion))
			})
		}
	})

	t.Run("Should correctly generate a MachineHealthCheck for the MachineDeployment", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint
		mdTopology := clusterv1.MachineDeploymentTopology{
			Class: "linux-worker",
			Name:  "big-pool-of-machines",
		}

		e := generator{}

		actual, err := e.computeMachineDeployment(ctx, scope, mdTopology)
		g.Expect(err).ToNot(HaveOccurred())
		// Check that the ClusterName and selector are set properly for the MachineHealthCheck.
		g.Expect(actual.MachineHealthCheck.Spec.ClusterName).To(Equal(cluster.Name))
		g.Expect(actual.MachineHealthCheck.Spec.Selector).To(BeComparableTo(metav1.LabelSelector{MatchLabels: map[string]string{
			clusterv1.ClusterTopologyOwnedLabel:                 actual.Object.Spec.Selector.MatchLabels[clusterv1.ClusterTopologyOwnedLabel],
			clusterv1.ClusterTopologyMachineDeploymentNameLabel: actual.Object.Spec.Selector.MatchLabels[clusterv1.ClusterTopologyMachineDeploymentNameLabel],
		}}))

		// Check that the NodeStartupTime is set as expected.
		g.Expect(actual.MachineHealthCheck.Spec.Checks.NodeStartupTimeoutSeconds).To(Equal(nodeTimeoutDuration))

		// Check that UnhealthyNodeConditions are set as expected.
		g.Expect(actual.MachineHealthCheck.Spec.Checks.UnhealthyNodeConditions).To(BeComparableTo(unhealthyNodeConditions))

		// Check that UnhealthyMachineConditions are set as expected.
		g.Expect(actual.MachineHealthCheck.Spec.Checks.UnhealthyMachineConditions).To(BeComparableTo(unhealthyMachineConditions))
	})
}

func TestComputeMachinePool(t *testing.T) {
	workerInfrastructureMachinePool := builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "linux-worker-inframachinepool").
		Build()
	workerInfrastructureMachinePoolTemplate := builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "linux-worker-inframachinepooltemplate").
		Build()
	workerBootstrapConfig := builder.BootstrapTemplate(metav1.NamespaceDefault, "linux-worker-bootstrap").
		Build()
	workerBootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "linux-worker-bootstraptemplate").
		Build()
	labels := map[string]string{"fizzLabel": "buzz", "fooLabel": "bar"}
	annotations := map[string]string{"fizzAnnotation": "buzz", "fooAnnotation": "bar"}

	clusterClassDuration := int32(20)
	clusterClassFailureDomains := []string{"A", "B"}
	var clusterClassMinReadySeconds int32 = 20
	mp1 := builder.MachinePoolClass("linux-worker").
		WithLabels(labels).
		WithAnnotations(annotations).
		WithInfrastructureTemplate(workerInfrastructureMachinePoolTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		WithFailureDomains("A", "B").
		WithNodeDrainTimeout(&clusterClassDuration).
		WithNodeVolumeDetachTimeout(&clusterClassDuration).
		WithNodeDeletionTimeout(&clusterClassDuration).
		WithMinReadySeconds(&clusterClassMinReadySeconds).
		Build()
	mcps := []clusterv1.MachinePoolClass{*mp1}
	fakeClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithWorkerMachinePoolClasses(mcps...).
		Build()

	version := "v1.21.3"
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			Topology: clusterv1.Topology{
				Version: version,
			},
		},
	}

	blueprint := &scope.ClusterBlueprint{
		Topology:     cluster.Spec.Topology,
		ClusterClass: fakeClass,
		MachinePools: map[string]*scope.MachinePoolBlueprint{
			"linux-worker": {
				Metadata: clusterv1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				BootstrapTemplate:                 workerBootstrapTemplate,
				InfrastructureMachinePoolTemplate: workerInfrastructureMachinePoolTemplate,
			},
		},
	}

	replicas := int32(5)
	topologyFailureDomains := []string{"A", "B"}
	topologyDuration := int32(10)
	var topologyMinReadySeconds int32 = 10
	mpTopology := clusterv1.MachinePoolTopology{
		Metadata: clusterv1.ObjectMeta{
			Labels: map[string]string{
				// Should overwrite the label from the MachinePool class.
				"fooLabel": "baz",
			},
			Annotations: map[string]string{
				// Should overwrite the annotation from the MachinePool class.
				"fooAnnotation": "baz",
				// These annotations should not be propagated to the MachinePool.
				clusterv1.ClusterTopologyDeferUpgradeAnnotation:        "",
				clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation: "",
			},
		},
		Class:          "linux-worker",
		Name:           "big-pool-of-machines",
		Replicas:       &replicas,
		FailureDomains: topologyFailureDomains,
		Deletion: clusterv1.MachinePoolTopologyMachineDeletionSpec{
			NodeDrainTimeoutSeconds:        &topologyDuration,
			NodeVolumeDetachTimeoutSeconds: &topologyDuration,
			NodeDeletionTimeoutSeconds:     &topologyDuration,
		},
		MinReadySeconds: &topologyMinReadySeconds,
	}

	t.Run("Generates the machine pool and the referenced templates", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		e := generator{}

		actual, err := e.computeMachinePool(ctx, scope, mpTopology)
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(actual.BootstrapObject.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachinePoolNameLabel, "big-pool-of-machines"))

		// Ensure Cluster ownership is added to generated BootstrapObject.
		g.Expect(actual.BootstrapObject.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(actual.BootstrapObject.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(actual.BootstrapObject.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))

		g.Expect(actual.InfrastructureMachinePoolObject.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterTopologyMachinePoolNameLabel, "big-pool-of-machines"))

		// Ensure Cluster ownership is added to generated InfrastructureMachinePool.
		g.Expect(actual.InfrastructureMachinePoolObject.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(actual.InfrastructureMachinePoolObject.GetOwnerReferences()[0].Kind).To(Equal("Cluster"))
		g.Expect(actual.InfrastructureMachinePoolObject.GetOwnerReferences()[0].Name).To(Equal(cluster.Name))

		actualMp := actual.Object
		g.Expect(*actualMp.Spec.Replicas).To(Equal(replicas))
		g.Expect(actualMp.Spec.FailureDomains).To(Equal(topologyFailureDomains))
		g.Expect(actualMp.Spec.Template.Spec.MinReadySeconds).To(HaveValue(Equal(topologyMinReadySeconds)))
		g.Expect(*actualMp.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).To(Equal(topologyDuration))
		g.Expect(*actualMp.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).To(Equal(topologyDuration))
		g.Expect(*actualMp.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(topologyDuration))
		g.Expect(actualMp.Spec.ClusterName).To(Equal("cluster1"))
		g.Expect(actualMp.Name).To(ContainSubstring("cluster1"))
		g.Expect(actualMp.Name).To(ContainSubstring("big-pool-of-machines"))

		expectedAnnotations := util.MergeMap(mpTopology.Metadata.Annotations, mp1.Metadata.Annotations)
		delete(expectedAnnotations, clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation)
		delete(expectedAnnotations, clusterv1.ClusterTopologyDeferUpgradeAnnotation)
		g.Expect(actualMp.Annotations).To(Equal(expectedAnnotations))
		g.Expect(actualMp.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations))

		g.Expect(actualMp.Labels).To(BeComparableTo(util.MergeMap(mpTopology.Metadata.Labels, mp1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                    cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:           "",
			clusterv1.ClusterTopologyMachinePoolNameLabel: "big-pool-of-machines",
		})))
		g.Expect(actualMp.Spec.Template.ObjectMeta.Labels).To(BeComparableTo(util.MergeMap(mpTopology.Metadata.Labels, mp1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                    cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:           "",
			clusterv1.ClusterTopologyMachinePoolNameLabel: "big-pool-of-machines",
		})))

		g.Expect(actualMp.Spec.Template.Spec.InfrastructureRef.Name).ToNot(Equal("linux-worker-inframachinetemplate"))
		g.Expect(actualMp.Spec.Template.Spec.Bootstrap.ConfigRef.Name).ToNot(Equal("linux-worker-bootstraptemplate"))
	})
	t.Run("Generates the machine pool and the referenced templates using ClusterClass defaults", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		mpTopology := clusterv1.MachinePoolTopology{
			Metadata: clusterv1.ObjectMeta{
				Labels: map[string]string{"foo": "baz"},
			},
			Class:    "linux-worker",
			Name:     "big-pool-of-machines",
			Replicas: &replicas,
			// missing FailureDomain, NodeDrainTimeoutSeconds, NodeVolumeDetachTimeoutSeconds, NodeDeletionTimeoutSeconds, MinReadySeconds
		}

		e := generator{}

		actual, err := e.computeMachinePool(ctx, scope, mpTopology)
		g.Expect(err).ToNot(HaveOccurred())

		// checking only values from CC defaults
		actualMp := actual.Object
		g.Expect(actualMp.Spec.FailureDomains).To(Equal(clusterClassFailureDomains))
		g.Expect(actualMp.Spec.Template.Spec.MinReadySeconds).To(HaveValue(Equal(clusterClassMinReadySeconds)))
		g.Expect(*actualMp.Spec.Template.Spec.Deletion.NodeDrainTimeoutSeconds).To(Equal(clusterClassDuration))
		g.Expect(*actualMp.Spec.Template.Spec.Deletion.NodeVolumeDetachTimeoutSeconds).To(Equal(clusterClassDuration))
		g.Expect(*actualMp.Spec.Template.Spec.Deletion.NodeDeletionTimeoutSeconds).To(Equal(clusterClassDuration))
	})

	t.Run("If there is already a machine pool, it preserves the object name and the reference names", func(t *testing.T) {
		g := NewWithT(t)
		s := scope.New(cluster)
		s.Blueprint = blueprint

		currentReplicas := int32(3)
		currentMp := &clusterv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing-pool-1",
			},
			Spec: clusterv1.MachinePoolSpec{
				Replicas: &currentReplicas,
				Template: clusterv1.MachineTemplateSpec{
					Spec: clusterv1.MachineSpec{
						Version: version,
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: contract.ObjToContractVersionedObjectReference(workerBootstrapConfig),
						},
						InfrastructureRef: contract.ObjToContractVersionedObjectReference(workerInfrastructureMachinePool),
					},
				},
			},
		}
		s.Current.MachinePools = map[string]*scope.MachinePoolState{
			"big-pool-of-machines": {
				Object:                          currentMp,
				BootstrapObject:                 workerBootstrapConfig,
				InfrastructureMachinePoolObject: workerInfrastructureMachinePool,
			},
		}

		e := generator{}

		actual, err := e.computeMachinePool(ctx, s, mpTopology)
		g.Expect(err).ToNot(HaveOccurred())

		actualMp := actual.Object

		g.Expect(*actualMp.Spec.Replicas).NotTo(Equal(currentReplicas))
		g.Expect(actualMp.Spec.FailureDomains).To(Equal(topologyFailureDomains))
		g.Expect(actualMp.Name).To(Equal("existing-pool-1"))

		expectedAnnotations := util.MergeMap(mpTopology.Metadata.Annotations, mp1.Metadata.Annotations)
		delete(expectedAnnotations, clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation)
		delete(expectedAnnotations, clusterv1.ClusterTopologyDeferUpgradeAnnotation)
		g.Expect(actualMp.Annotations).To(Equal(expectedAnnotations))
		g.Expect(actualMp.Spec.Template.ObjectMeta.Annotations).To(Equal(expectedAnnotations))

		g.Expect(actualMp.Labels).To(BeComparableTo(util.MergeMap(mpTopology.Metadata.Labels, mp1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                    cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:           "",
			clusterv1.ClusterTopologyMachinePoolNameLabel: "big-pool-of-machines",
		})))
		g.Expect(actualMp.Spec.Template.ObjectMeta.Labels).To(BeComparableTo(util.MergeMap(mpTopology.Metadata.Labels, mp1.Metadata.Labels, map[string]string{
			clusterv1.ClusterNameLabel:                    cluster.Name,
			clusterv1.ClusterTopologyOwnedLabel:           "",
			clusterv1.ClusterTopologyMachinePoolNameLabel: "big-pool-of-machines",
		})))

		g.Expect(actualMp.Spec.Template.Spec.InfrastructureRef.Name).To(Equal("linux-worker-inframachinepool"))
		g.Expect(actualMp.Spec.Template.Spec.Bootstrap.ConfigRef.Name).To(Equal("linux-worker-bootstrap"))
	})

	t.Run("If a machine pool references a topology class that does not exist, machine pool generation fails", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint

		mpTopology = clusterv1.MachinePoolTopology{
			Metadata: clusterv1.ObjectMeta{
				Labels: map[string]string{"foo": "baz"},
			},
			Class: "windows-worker",
			Name:  "big-pool-of-machines",
		}

		e := generator{}

		_, err := e.computeMachinePool(ctx, scope, mpTopology)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("Should choose the correct version for machine pool", func(t *testing.T) {
		controlPlaneStable123 := builder.ControlPlane("test1", "cp1").
			WithSpecFields(map[string]interface{}{
				"spec.version":  "v1.2.3",
				"spec.replicas": int64(2),
			}).
			WithStatusFields(map[string]interface{}{
				"status.version":          "v1.2.3",
				"status.replicas":         int64(2),
				"status.upToDateReplicas": int64(2),
				"status.readyReplicas":    int64(2),
			}).
			Build()

		// Note: in all the following tests we are setting it up so that the control plane is already
		// stable at the topology version.
		// A more extensive list of scenarios is tested in TestComputeMachinePoolVersion.
		tests := []struct {
			name                  string
			upgradingMachinePools []string
			currentMPVersion      *string
			upgradeConcurrency    string
			topologyVersion       string
			upgradePlan           []string
			expectedVersion       string
		}{
			{
				name:                  "use cluster.spec.topology.version if creating a new machine pool",
				upgradingMachinePools: []string{},
				upgradeConcurrency:    "1",
				currentMPVersion:      nil,
				topologyVersion:       "v1.2.3",
				upgradePlan:           []string{"v1.2.3"},
				expectedVersion:       "v1.2.3",
			},
			{
				name:                  "use cluster.spec.topology.version if creating a new machine pool while another machine pool is upgrading",
				upgradingMachinePools: []string{"upgrading-mp1"},
				upgradeConcurrency:    "1",
				currentMPVersion:      nil,
				topologyVersion:       "v1.2.3",
				upgradePlan:           []string{"v1.2.3"},
				expectedVersion:       "v1.2.3",
			},
			{
				name:                  "use machine pool's spec.template.spec.version if one of the machine pools is upgrading, concurrency limit reached",
				upgradingMachinePools: []string{"upgrading-mp1"},
				upgradeConcurrency:    "1",
				currentMPVersion:      ptr.To("v1.2.2"),
				topologyVersion:       "v1.2.3",
				upgradePlan:           []string{"v1.2.3"},
				expectedVersion:       "v1.2.2",
			},
			{
				name:                  "use cluster.spec.topology.version if one of the machine pools is upgrading, concurrency limit not reached",
				upgradingMachinePools: []string{"upgrading-mp1"},
				upgradeConcurrency:    "2",
				currentMPVersion:      ptr.To("v1.2.2"),
				topologyVersion:       "v1.2.3",
				upgradePlan:           []string{"v1.2.3"},
				expectedVersion:       "v1.2.3",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				testCluster := cluster.DeepCopy()
				if testCluster.Annotations == nil {
					testCluster.Annotations = map[string]string{}
				}
				testCluster.Annotations[clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation] = tt.upgradeConcurrency

				s := scope.New(testCluster)
				s.Blueprint = blueprint
				s.Blueprint.Topology.Version = tt.topologyVersion
				s.Blueprint.Topology.ControlPlane = clusterv1.ControlPlaneTopology{
					Replicas: ptr.To[int32](2),
				}
				s.Blueprint.Topology.Workers = clusterv1.WorkersTopology{}

				mpsState := scope.MachinePoolsStateMap{}
				if tt.currentMPVersion != nil {
					// testing a case with an existing machine pool
					// add the stable machine pool to the current machine pools state
					mp := builder.MachinePool("test-namespace", "big-pool-of-machines").
						WithReplicas(2).
						WithVersion(*tt.currentMPVersion).
						WithStatus(clusterv1.MachinePoolStatus{
							ObservedGeneration: 2,
							Replicas:           ptr.To(int32(2)),
							Deprecated: &clusterv1.MachinePoolDeprecatedStatus{
								V1Beta1: &clusterv1.MachinePoolV1Beta1DeprecatedStatus{
									ReadyReplicas:     2,
									AvailableReplicas: 2,
								},
							},
						}).
						Build()
					mpsState = duplicateMachinePoolsState(mpsState)
					mpsState["big-pool-of-machines"] = &scope.MachinePoolState{
						Object: mp,
					}
				}
				s.Current.MachinePools = mpsState
				s.Current.ControlPlane = &scope.ControlPlaneState{
					Object: controlPlaneStable123,
				}

				mpTopology := clusterv1.MachinePoolTopology{
					Class:    "linux-worker",
					Name:     "big-pool-of-machines",
					Replicas: ptr.To[int32](2),
				}
				s.UpgradeTracker.MachinePools.MarkUpgrading(tt.upgradingMachinePools...)
				if tt.upgradePlan != nil {
					s.UpgradeTracker.MachinePools.UpgradePlan = tt.upgradePlan
				}

				e := generator{}

				obj, err := e.computeMachinePool(ctx, s, mpTopology)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(obj.Object.Spec.Template.Spec.Version).To(Equal(tt.expectedVersion))
			})
		}
	})
}

func TestComputeMachineDeploymentVersion(t *testing.T) {
	controlPlaneObj := builder.ControlPlane("test1", "cp1").
		Build()

	mdName := "md-1"
	currentMachineDeploymentState := &scope.MachineDeploymentState{Object: builder.MachineDeployment("test1", mdName).WithVersion("v1.2.2").Build()}

	tests := []struct {
		name                                 string
		machineDeploymentTopology            clusterv1.MachineDeploymentTopology
		currentMachineDeploymentState        *scope.MachineDeploymentState
		upgradingMachineDeployments          []string
		upgradeConcurrency                   int
		controlPlanePendingUpgrade           bool
		controlPlaneWaitingForWorkersUpgrade bool
		controlPlaneStartingUpgrade          bool
		controlPlaneUpgrading                bool
		controlPlaneProvisioning             bool
		afterControlPlaneUpgradeHookBlocking bool
		beforeWorkersUpgradeHookBlocking     bool
		topologyVersion                      string
		upgradePlan                          []string
		expectedVersion                      string
		expectPendingCreate                  bool
		expectPendingUpgrade                 bool
	}{
		{
			name:                          "should return cluster.spec.topology.version if creating a new machine deployment and if control plane is stable - not marked as pending create",
			currentMachineDeploymentState: nil,
			machineDeploymentTopology: clusterv1.MachineDeploymentTopology{
				Name: "md-topology-1",
			},
			topologyVersion:     "v1.2.3",
			expectedVersion:     "v1.2.3",
			expectPendingCreate: false,
		},
		{
			name:                  "should return cluster.spec.topology.version if creating a new machine deployment and if control plane is not stable - marked as pending create",
			controlPlaneUpgrading: true,
			machineDeploymentTopology: clusterv1.MachineDeploymentTopology{
				Name: "md-topology-1",
			},
			topologyVersion:     "v1.2.3",
			expectedVersion:     "v1.2.3",
			expectPendingCreate: true,
		},
		{
			name: "should return machine deployment's spec.template.spec.version if upgrade is deferred",
			machineDeploymentTopology: clusterv1.MachineDeploymentTopology{
				Metadata: clusterv1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyDeferUpgradeAnnotation: "",
					},
				},
			},
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{},
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.3"},
			expectedVersion:               "v1.2.2",
			expectPendingUpgrade:          true,
		},
		{
			// Control plane is considered pending an upgrade if topology version did not yet propagate to the control plane.
			name:                          "should return machine deployment's spec.template.spec.version if control plane is pending upgrading",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{},
			controlPlanePendingUpgrade:    true,
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.3"},
			expectedVersion:               "v1.2.2",
			expectPendingUpgrade:          true,
		},
		{
			// Control plane is considered upgrading if the control plane's spec.version and status.version is not equal.
			name:                          "should return machine deployment's spec.template.spec.version if control plane is upgrading",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{},
			controlPlaneUpgrading:         true,
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.3"},
			expectedVersion:               "v1.2.2",
			expectPendingUpgrade:          true,
		},
		{
			// Control plane is considered ready to upgrade if spec.version of current and desired control planes are not equal.
			name:                          "should return machine deployment's spec.template.spec.version if control plane is starting upgrade",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{},
			controlPlaneStartingUpgrade:   true,
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.3"},
			expectedVersion:               "v1.2.2",
			expectPendingUpgrade:          true,
		},
		{
			name:                          "should return machine deployment's spec.template.spec.version if the Machine deployment already performed the upgrade step",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{},
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.2", "v1.2.3"},
			expectedVersion:               "v1.2.2",
			expectPendingUpgrade:          true,
		},
		{
			name:                          "should return cluster.spec.topology.version if the control plane is not upgrading, not scaling, not ready to upgrade and none of the machine deployments are upgrading",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{},
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.3"},
			expectedVersion:               "v1.2.3",
			expectPendingUpgrade:          false,
		},
		{
			name:                          "should return next version from the upgrade plan if mutistep upgrade, if the control plane is not upgrading, not scaling, not ready to upgrade and none of the machine deployments are upgrading",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{},
			topologyVersion:               "v1.4.3",
			upgradePlan:                   []string{"v1.3.3", "v1.4.3"},
			expectedVersion:               "v1.3.3",
			expectPendingUpgrade:          false,
		},
		{
			// Control plane is considered pending an upgrade if topology version did not yet propagate to the control plane.
			name:                                 "should return next version from the upgrade plan if mutistep upgrade, if the control plane is pending an upgrade but this requires workers to upgrade first",
			currentMachineDeploymentState:        currentMachineDeploymentState,
			upgradingMachineDeployments:          []string{},
			controlPlanePendingUpgrade:           true,
			controlPlaneWaitingForWorkersUpgrade: true,
			topologyVersion:                      "v1.4.3",
			upgradePlan:                          []string{"v1.3.3", "v1.4.3"},
			expectedVersion:                      "v1.3.3",
			expectPendingUpgrade:                 false,
		},
		{
			name:                                 "should return machine deployment's spec.template.spec.version if control plane is stable, other machine deployments are upgrading, concurrency limit not reached but AfterControlPlaneUpgrade hook is blocking",
			currentMachineDeploymentState:        currentMachineDeploymentState,
			upgradingMachineDeployments:          []string{"upgrading-md1"},
			upgradeConcurrency:                   2,
			afterControlPlaneUpgradeHookBlocking: true,
			topologyVersion:                      "v1.2.3",
			upgradePlan:                          []string{"v1.2.3"},
			expectedVersion:                      "v1.2.2",
			expectPendingUpgrade:                 true,
		},
		{
			name:                             "should return machine deployment's spec.template.spec.version if control plane is stable, other machine deployments are upgrading, concurrency limit not reached but BeforeWorkersUpgrade hook is blocking",
			currentMachineDeploymentState:    currentMachineDeploymentState,
			upgradingMachineDeployments:      []string{"upgrading-md1"},
			upgradeConcurrency:               2,
			beforeWorkersUpgradeHookBlocking: true,
			topologyVersion:                  "v1.2.3",
			upgradePlan:                      []string{"v1.2.3"},
			expectedVersion:                  "v1.2.2",
			expectPendingUpgrade:             true,
		},
		{
			name:                          "should return cluster.spec.topology.version if control plane is stable, other machine deployments are upgrading, concurrency limit not reached",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{"upgrading-md1"},
			upgradeConcurrency:            2,
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.3"},
			expectedVersion:               "v1.2.3",
			expectPendingUpgrade:          false,
		},
		{
			name:                          "should return machine deployment's spec.template.spec.version if control plane is stable, other machine deployments are upgrading, concurrency limit reached",
			currentMachineDeploymentState: currentMachineDeploymentState,
			upgradingMachineDeployments:   []string{"upgrading-md1", "upgrading-md2"},
			upgradeConcurrency:            2,
			topologyVersion:               "v1.2.3",
			upgradePlan:                   []string{"v1.2.3"},
			expectedVersion:               "v1.2.2",
			expectPendingUpgrade:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{Topology: clusterv1.Topology{
					Version: tt.topologyVersion,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Replicas: ptr.To[int32](2),
					},
					Workers: clusterv1.WorkersTopology{},
				}},
				Current: &scope.ClusterState{
					ControlPlane: &scope.ControlPlaneState{Object: controlPlaneObj},
				},
				UpgradeTracker:      scope.NewUpgradeTracker(scope.MaxMDUpgradeConcurrency(tt.upgradeConcurrency)),
				HookResponseTracker: scope.NewHookResponseTracker(),
			}
			if tt.upgradePlan != nil {
				s.UpgradeTracker.MachineDeployments.UpgradePlan = tt.upgradePlan
			}
			if tt.afterControlPlaneUpgradeHookBlocking {
				s.HookResponseTracker.Add(runtimehooksv1.AfterControlPlaneUpgrade, &runtimehooksv1.AfterControlPlaneUpgradeResponse{
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
						RetryAfterSeconds: 10,
					},
				})
			}
			if tt.beforeWorkersUpgradeHookBlocking {
				s.HookResponseTracker.Add(runtimehooksv1.BeforeWorkersUpgrade, &runtimehooksv1.BeforeWorkersUpgradeResponse{
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
						RetryAfterSeconds: 10,
					},
				})
			}
			s.UpgradeTracker.ControlPlane.IsStartingUpgrade = tt.controlPlaneStartingUpgrade
			s.UpgradeTracker.ControlPlane.IsUpgrading = tt.controlPlaneUpgrading
			s.UpgradeTracker.ControlPlane.IsProvisioning = tt.controlPlaneProvisioning
			s.UpgradeTracker.ControlPlane.IsPendingUpgrade = tt.controlPlanePendingUpgrade
			s.UpgradeTracker.ControlPlane.IsWaitingForWorkersUpgrade = tt.controlPlaneWaitingForWorkersUpgrade
			s.UpgradeTracker.MachineDeployments.MarkUpgrading(tt.upgradingMachineDeployments...)

			e := generator{}

			version, err := e.computeMachineDeploymentVersion(ctx, s, tt.machineDeploymentTopology, tt.currentMachineDeploymentState)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(version).To(Equal(tt.expectedVersion))

			if tt.currentMachineDeploymentState != nil {
				// Verify that if the upgrade is pending it is captured in the upgrade tracker.
				if tt.expectPendingUpgrade {
					g.Expect(s.UpgradeTracker.MachineDeployments.IsPendingUpgrade(mdName)).To(BeTrue(), "MachineDeployment should be marked as pending upgrade")
				} else {
					g.Expect(s.UpgradeTracker.MachineDeployments.IsPendingUpgrade(mdName)).To(BeFalse(), "MachineDeployment should not be marked as pending upgrade")
				}
			} else {
				// Verify that if create the pending it is capture in the tracker.
				if tt.expectPendingCreate {
					g.Expect(s.UpgradeTracker.MachineDeployments.IsPendingCreate(tt.machineDeploymentTopology.Name)).To(BeTrue(), "MachineDeployment topology should be marked as pending create")
				} else {
					g.Expect(s.UpgradeTracker.MachineDeployments.IsPendingCreate(tt.machineDeploymentTopology.Name)).To(BeFalse(), "MachineDeployment topology should not be marked as pending create")
				}
			}
		})
	}
}

func TestComputeMachinePoolVersion(t *testing.T) {
	controlPlaneObj := builder.ControlPlane("test1", "cp1").
		Build()

	mpName := "mp-1"
	currentMachinePoolState := &scope.MachinePoolState{Object: builder.MachinePool("test1", mpName).WithVersion("v1.2.2").Build()}

	tests := []struct {
		name                                 string
		machinePoolTopology                  clusterv1.MachinePoolTopology
		currentMachinePoolState              *scope.MachinePoolState
		upgradingMachinePools                []string
		upgradeConcurrency                   int
		controlPlanePendingUpgrade           bool
		controlPlaneWaitingForWorkersUpgrade bool
		controlPlaneStartingUpgrade          bool
		controlPlaneUpgrading                bool
		controlPlaneProvisioning             bool
		afterControlPlaneUpgradeHookBlocking bool
		beforeWorkersUpgradeHookBlocking     bool
		topologyVersion                      string
		upgradePlan                          []string
		expectedVersion                      string
		expectPendingCreate                  bool
		expectPendingUpgrade                 bool
	}{
		{
			name:                    "should return cluster.spec.topology.version if creating a new MachinePool and if control plane is stable - not marked as pending create",
			currentMachinePoolState: nil,
			machinePoolTopology: clusterv1.MachinePoolTopology{
				Name: "mp-topology-1",
			},
			topologyVersion:     "v1.2.3",
			expectedVersion:     "v1.2.3",
			expectPendingCreate: false,
		},
		{
			name:                  "should return cluster.spec.topology.version if creating a new MachinePool and if control plane is not stable - marked as pending create",
			controlPlaneUpgrading: true,
			machinePoolTopology: clusterv1.MachinePoolTopology{
				Name: "mp-topology-1",
			},
			topologyVersion:     "v1.2.3",
			expectedVersion:     "v1.2.3",
			expectPendingCreate: true,
		},
		{
			name: "should return MachinePool's spec.template.spec.version if upgrade is deferred",
			machinePoolTopology: clusterv1.MachinePoolTopology{
				Metadata: clusterv1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyDeferUpgradeAnnotation: "",
					},
				},
			},
			currentMachinePoolState: currentMachinePoolState,
			upgradingMachinePools:   []string{},
			topologyVersion:         "v1.2.3",
			upgradePlan:             []string{"v1.2.3"},
			expectedVersion:         "v1.2.2",
			expectPendingUpgrade:    true,
		},
		{
			// Control plane is considered pending an upgrade if topology version did not yet propagate to the control plane.
			name:                       "should return machine MachinePool's spec.template.spec.version if control plane is pending upgrading",
			currentMachinePoolState:    currentMachinePoolState,
			upgradingMachinePools:      []string{},
			controlPlanePendingUpgrade: true,
			topologyVersion:            "v1.2.3",
			upgradePlan:                []string{"v1.2.3"},
			expectedVersion:            "v1.2.2",
			expectPendingUpgrade:       true,
		},
		{
			// Control plane is considered upgrading if the control plane's spec.version and status.version is not equal.
			name:                    "should return MachinePool's spec.template.spec.version if control plane is upgrading",
			currentMachinePoolState: currentMachinePoolState,
			upgradingMachinePools:   []string{},
			controlPlaneUpgrading:   true,
			topologyVersion:         "v1.2.3",
			upgradePlan:             []string{"v1.2.3"},
			expectedVersion:         "v1.2.2",
			expectPendingUpgrade:    true,
		},
		{
			// Control plane is considered ready to upgrade if spec.version of current and desired control planes are not equal.
			name:                        "should return MachinePool's spec.template.spec.version if control plane is starting upgrade",
			currentMachinePoolState:     currentMachinePoolState,
			upgradingMachinePools:       []string{},
			controlPlaneStartingUpgrade: true,
			topologyVersion:             "v1.2.3",
			upgradePlan:                 []string{"v1.2.3"},
			expectedVersion:             "v1.2.2",
			expectPendingUpgrade:        true,
		},
		{
			name:                    "should return MachinePool's spec.template.spec.version if the MachinePool already performed the upgrade step",
			currentMachinePoolState: currentMachinePoolState,
			upgradingMachinePools:   []string{},
			topologyVersion:         "v1.2.3",
			upgradePlan:             []string{"v1.2.2", "v1.2.3"},
			expectedVersion:         "v1.2.2",
			expectPendingUpgrade:    true,
		},
		{
			name:                    "should return cluster.spec.topology.version if the control plane is not upgrading, not scaling, not ready to upgrade and none of the MachinePools are upgrading",
			currentMachinePoolState: currentMachinePoolState,
			upgradingMachinePools:   []string{},
			topologyVersion:         "v1.2.3",
			upgradePlan:             []string{"v1.2.3"},
			expectedVersion:         "v1.2.3",
			expectPendingUpgrade:    false,
		},
		{
			name:                    "should return next version in the upgrade plan if multistep upgrade, if the control plane is not upgrading, not scaling, not ready to upgrade and none of the MachinePools are upgrading",
			currentMachinePoolState: currentMachinePoolState,
			upgradingMachinePools:   []string{},
			topologyVersion:         "v1.4.3",
			upgradePlan:             []string{"v1.3.3", "v1.4.3"},
			expectedVersion:         "v1.3.3",
			expectPendingUpgrade:    false,
		},
		{
			// Control plane is considered pending an upgrade if topology version did not yet propagate to the control plane.
			name:                                 "should return next version in the upgrade plan if multistep upgrade, if the control plane is pending an upgrade but this requires workers to upgrade first",
			currentMachinePoolState:              currentMachinePoolState,
			upgradingMachinePools:                []string{},
			controlPlanePendingUpgrade:           true,
			controlPlaneWaitingForWorkersUpgrade: true,
			topologyVersion:                      "v1.4.3",
			upgradePlan:                          []string{"v1.3.3", "v1.4.3"},
			expectedVersion:                      "v1.3.3",
			expectPendingUpgrade:                 false,
		},
		{
			name:                                 "should return MachinePool's spec.template.spec.version if control plane is stable, other MachinePools are upgrading, concurrency limit not reached but AfterControlPlaneUpgrade hook is blocking",
			currentMachinePoolState:              currentMachinePoolState,
			upgradingMachinePools:                []string{"upgrading-mp1"},
			upgradeConcurrency:                   2,
			afterControlPlaneUpgradeHookBlocking: true,
			topologyVersion:                      "v1.2.3",
			upgradePlan:                          []string{"v1.2.3"},
			expectedVersion:                      "v1.2.2",
			expectPendingUpgrade:                 true,
		},
		{
			name:                             "should return MachinePool's spec.template.spec.version if control plane is stable, other MachinePools are upgrading, concurrency limit not reached but BeforeWorkersUpgrade hook is blocking",
			currentMachinePoolState:          currentMachinePoolState,
			upgradingMachinePools:            []string{"upgrading-mp1"},
			upgradeConcurrency:               2,
			beforeWorkersUpgradeHookBlocking: true,
			topologyVersion:                  "v1.2.3",
			upgradePlan:                      []string{"v1.2.3"},
			expectedVersion:                  "v1.2.2",
			expectPendingUpgrade:             true,
		},
		{
			name:                    "should return cluster.spec.topology.version if control plane is stable, other MachinePools are upgrading, concurrency limit not reached",
			currentMachinePoolState: currentMachinePoolState,
			upgradingMachinePools:   []string{"upgrading-mp1"},
			upgradeConcurrency:      2,
			topologyVersion:         "v1.2.3",
			upgradePlan:             []string{"v1.2.3"},
			expectedVersion:         "v1.2.3",
			expectPendingUpgrade:    false,
		},
		{
			name:                    "should return MachinePool's spec.template.spec.version if control plane is stable, other MachinePools are upgrading, concurrency limit reached",
			currentMachinePoolState: currentMachinePoolState,
			upgradingMachinePools:   []string{"upgrading-mp1", "upgrading-mp2"},
			upgradeConcurrency:      2,
			topologyVersion:         "v1.2.3",
			upgradePlan:             []string{"v1.2.3"},
			expectedVersion:         "v1.2.2",
			expectPendingUpgrade:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := &scope.Scope{
				Blueprint: &scope.ClusterBlueprint{Topology: clusterv1.Topology{
					Version: tt.topologyVersion,
					ControlPlane: clusterv1.ControlPlaneTopology{
						Replicas: ptr.To[int32](2),
					},
					Workers: clusterv1.WorkersTopology{},
				}},
				Current: &scope.ClusterState{
					ControlPlane: &scope.ControlPlaneState{Object: controlPlaneObj},
				},
				UpgradeTracker:      scope.NewUpgradeTracker(scope.MaxMPUpgradeConcurrency(tt.upgradeConcurrency)),
				HookResponseTracker: scope.NewHookResponseTracker(),
			}
			if tt.afterControlPlaneUpgradeHookBlocking {
				s.HookResponseTracker.Add(runtimehooksv1.AfterControlPlaneUpgrade, &runtimehooksv1.AfterControlPlaneUpgradeResponse{
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
						RetryAfterSeconds: 10,
					},
				})
			}
			if tt.beforeWorkersUpgradeHookBlocking {
				s.HookResponseTracker.Add(runtimehooksv1.BeforeWorkersUpgrade, &runtimehooksv1.BeforeWorkersUpgradeResponse{
					CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
						RetryAfterSeconds: 10,
					},
				})
			}
			s.UpgradeTracker.ControlPlane.IsStartingUpgrade = tt.controlPlaneStartingUpgrade
			s.UpgradeTracker.ControlPlane.IsUpgrading = tt.controlPlaneUpgrading
			s.UpgradeTracker.ControlPlane.IsProvisioning = tt.controlPlaneProvisioning
			s.UpgradeTracker.ControlPlane.IsPendingUpgrade = tt.controlPlanePendingUpgrade
			s.UpgradeTracker.ControlPlane.IsWaitingForWorkersUpgrade = tt.controlPlaneWaitingForWorkersUpgrade
			s.UpgradeTracker.MachinePools.MarkUpgrading(tt.upgradingMachinePools...)
			if tt.upgradePlan != nil {
				s.UpgradeTracker.MachinePools.UpgradePlan = tt.upgradePlan
			}

			e := generator{}

			version, err := e.computeMachinePoolVersion(ctx, s, tt.machinePoolTopology, tt.currentMachinePoolState)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(version).To(Equal(tt.expectedVersion))

			if tt.currentMachinePoolState != nil {
				// Verify that if the upgrade is pending it is captured in the upgrade tracker.
				if tt.expectPendingUpgrade {
					g.Expect(s.UpgradeTracker.MachinePools.IsPendingUpgrade(mpName)).To(BeTrue(), "MachinePool should be marked as pending upgrade")
				} else {
					g.Expect(s.UpgradeTracker.MachinePools.IsPendingUpgrade(mpName)).To(BeFalse(), "MachinePool should not be marked as pending upgrade")
				}
			} else {
				// Verify that if create the pending it is capture in the tracker.
				if tt.expectPendingCreate {
					g.Expect(s.UpgradeTracker.MachinePools.IsPendingCreate(tt.machinePoolTopology.Name)).To(BeTrue(), "MachinePool topology should be marked as pending create")
				} else {
					g.Expect(s.UpgradeTracker.MachinePools.IsPendingCreate(tt.machinePoolTopology.Name)).To(BeFalse(), "MachinePool topology should not be marked as pending create")
				}
			}
		})
	}
}

func TestIsMachineDeploymentDeferred(t *testing.T) {
	clusterTopology := clusterv1.Topology{
		Workers: clusterv1.WorkersTopology{
			MachineDeployments: []clusterv1.MachineDeploymentTopology{
				{
					Name: "md-with-defer-upgrade",
					Metadata: clusterv1.ObjectMeta{
						Annotations: map[string]string{
							clusterv1.ClusterTopologyDeferUpgradeAnnotation: "",
						},
					},
				},
				{
					Name: "md-without-annotations",
				},
				{
					Name: "md-with-hold-upgrade-sequence",
					Metadata: clusterv1.ObjectMeta{
						Annotations: map[string]string{
							clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation: "",
						},
					},
				},
				{
					Name: "md-after-md-with-hold-upgrade-sequence",
				},
			},
		},
	}

	tests := []struct {
		name       string
		mdTopology clusterv1.MachineDeploymentTopology
		deferred   bool
	}{
		{
			name: "MD with defer-upgrade annotation is deferred",
			mdTopology: clusterv1.MachineDeploymentTopology{
				Name: "md-with-defer-upgrade",
				Metadata: clusterv1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyDeferUpgradeAnnotation: "",
					},
				},
			},
			deferred: true,
		},
		{
			name: "MD without annotations is not deferred",
			mdTopology: clusterv1.MachineDeploymentTopology{
				Name: "md-without-annotations",
			},
			deferred: false,
		},
		{
			name: "MD with hold-upgrade-sequence annotation is deferred",
			mdTopology: clusterv1.MachineDeploymentTopology{
				Name: "md-with-hold-upgrade-sequence",
				Metadata: clusterv1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation: "",
					},
				},
			},
			deferred: true,
		},
		{
			name: "MD after MD with hold-upgrade-sequence is deferred",
			mdTopology: clusterv1.MachineDeploymentTopology{
				Name: "md-after-md-with-hold-upgrade-sequence",
			},
			deferred: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isMachineDeploymentDeferred(clusterTopology, tt.mdTopology)).To(Equal(tt.deferred))
		})
	}
}

func TestIsMachinePoolDeferred(t *testing.T) {
	clusterTopology := clusterv1.Topology{
		Workers: clusterv1.WorkersTopology{
			MachinePools: []clusterv1.MachinePoolTopology{
				{
					Name: "mp-with-defer-upgrade",
					Metadata: clusterv1.ObjectMeta{
						Annotations: map[string]string{
							clusterv1.ClusterTopologyDeferUpgradeAnnotation: "",
						},
					},
				},
				{
					Name: "mp-without-annotations",
				},
				{
					Name: "mp-with-hold-upgrade-sequence",
					Metadata: clusterv1.ObjectMeta{
						Annotations: map[string]string{
							clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation: "",
						},
					},
				},
				{
					Name: "mp-after-mp-with-hold-upgrade-sequence",
				},
			},
		},
	}

	tests := []struct {
		name       string
		mpTopology clusterv1.MachinePoolTopology
		deferred   bool
	}{
		{
			name: "MP with defer-upgrade annotation is deferred",
			mpTopology: clusterv1.MachinePoolTopology{
				Name: "mp-with-defer-upgrade",
				Metadata: clusterv1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyDeferUpgradeAnnotation: "",
					},
				},
			},
			deferred: true,
		},
		{
			name: "MP without annotations is not deferred",
			mpTopology: clusterv1.MachinePoolTopology{
				Name: "mp-without-annotations",
			},
			deferred: false,
		},
		{
			name: "MP with hold-upgrade-sequence annotation is deferred",
			mpTopology: clusterv1.MachinePoolTopology{
				Name: "mp-with-hold-upgrade-sequence",
				Metadata: clusterv1.ObjectMeta{
					Annotations: map[string]string{
						clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation: "",
					},
				},
			},
			deferred: true,
		},
		{
			name: "MP after mp with hold-upgrade-sequence is deferred",
			mpTopology: clusterv1.MachinePoolTopology{
				Name: "mp-after-mp-with-hold-upgrade-sequence",
			},
			deferred: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isMachinePoolDeferred(clusterTopology, tt.mpTopology)).To(Equal(tt.deferred))
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
			nameGenerator:         topologynames.SimpleNameGenerator(cluster.Name),
			currentObjectName:     "",
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster: cluster,
			templateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       fakeRef1.Kind,
				Name:       fakeRef1.Name,
				APIVersion: fakeRef1.APIVersion,
			},
			template:          template,
			currentObjectName: "",
			obj:               obj,
		})
	})
	t.Run("Overrides the generated name if there is already a reference", func(t *testing.T) {
		g := NewWithT(t)
		obj, err := templateToObject(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			nameGenerator:         topologynames.SimpleNameGenerator(cluster.Name),
			currentObjectName:     fakeRef2.Name,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		// ObjectMeta
		assertTemplateToObject(g, assertTemplateInput{
			cluster: cluster,
			templateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       fakeRef1.Kind,
				Name:       fakeRef1.Name,
				APIVersion: fakeRef1.APIVersion,
			},
			template:          template,
			currentObjectName: fakeRef2.Name,
			obj:               obj,
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
		obj, err := templateToTemplate(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			nameGenerator:         topologynames.SimpleNameGenerator(cluster.Name),
			currentObjectName:     "",
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster: cluster,
			templateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       fakeRef1.Kind,
				Name:       fakeRef1.Name,
				APIVersion: fakeRef1.APIVersion,
			},
			template:          template,
			currentObjectName: "",
			obj:               obj,
		})
	})
	t.Run("Overrides the generated name if there is already a reference", func(t *testing.T) {
		g := NewWithT(t)
		obj, err := templateToTemplate(templateToInput{
			template:              template,
			templateClonedFromRef: fakeRef1,
			cluster:               cluster,
			nameGenerator:         topologynames.SimpleNameGenerator(cluster.Name),
			currentObjectName:     fakeRef2.Name,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())
		assertTemplateToTemplate(g, assertTemplateInput{
			cluster: cluster,
			templateRef: clusterv1.ClusterClassTemplateReference{
				Kind:       fakeRef1.Kind,
				Name:       fakeRef1.Name,
				APIVersion: fakeRef1.APIVersion,
			},
			template:          template,
			currentObjectName: fakeRef2.Name,
			obj:               obj,
		})
	})
}

type assertTemplateInput struct {
	cluster             *clusterv1.Cluster
	templateRef         clusterv1.ClusterClassTemplateReference
	template            *unstructured.Unstructured
	labels, annotations map[string]string
	currentObjectName   string
	obj                 *unstructured.Unstructured
}

func assertTemplateToObject(g *WithT, in assertTemplateInput) {
	// TypeMeta
	g.Expect(in.obj.GetAPIVersion()).To(Equal(in.template.GetAPIVersion()))
	g.Expect(in.obj.GetKind()).To(Equal(strings.TrimSuffix(in.template.GetKind(), "Template")))

	// ObjectMeta
	if in.currentObjectName != "" {
		g.Expect(in.obj.GetName()).To(Equal(in.currentObjectName))
	} else {
		g.Expect(in.obj.GetName()).To(HavePrefix(in.cluster.Name))
	}
	g.Expect(in.obj.GetNamespace()).To(Equal(in.cluster.Namespace))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, in.cluster.Name))
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
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())

	cloneSpec, ok, err := unstructured.NestedMap(in.obj.UnstructuredContent(), "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	for k, v := range expectedSpec {
		if k == "machineTemplate" {
			// We don't expect machineTemplate to be the same as in the template (as the template does not contain the infrastructureRef).
			g.Expect(cloneSpec).To(HaveKey(k))
			continue
		}
		g.Expect(cloneSpec).To(HaveKeyWithValue(k, v))
	}
}

func assertTemplateToTemplate(g *WithT, in assertTemplateInput) {
	// TypeMeta
	g.Expect(in.obj.GetAPIVersion()).To(Equal(in.template.GetAPIVersion()))
	g.Expect(in.obj.GetKind()).To(Equal(in.template.GetKind()))

	// ObjectMeta
	if in.currentObjectName != "" {
		g.Expect(in.obj.GetName()).To(Equal(in.currentObjectName))
	} else {
		g.Expect(in.obj.GetName()).To(HavePrefix(in.cluster.Name))
	}
	g.Expect(in.obj.GetNamespace()).To(Equal(in.cluster.Namespace))
	g.Expect(in.obj.GetLabels()).To(HaveKeyWithValue(clusterv1.ClusterNameLabel, in.cluster.Name))
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
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())

	cloneSpec, ok, err := unstructured.NestedMap(in.obj.UnstructuredContent(), "spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(cloneSpec).To(BeComparableTo(expectedSpec))
}

func assertNestedField(g *WithT, obj *unstructured.Unstructured, value interface{}, fields ...string) {
	v, ok, err := unstructured.NestedFieldCopy(obj.UnstructuredContent(), fields...)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(v).To(BeComparableTo(value))
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

func duplicateMachinePoolsState(s scope.MachinePoolsStateMap) scope.MachinePoolsStateMap {
	n := make(scope.MachinePoolsStateMap)
	for k, v := range s {
		n[k] = v
	}
	return n
}

func TestMergeMap(t *testing.T) {
	t.Run("Merge maps", func(t *testing.T) {
		g := NewWithT(t)

		m := util.MergeMap(
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

		m := util.MergeMap(map[string]string{}, map[string]string{})
		g.Expect(m).To(BeNil())
	})
}

func Test_computeMachineHealthCheck(t *testing.T) {
	mhcChecks := clusterv1.MachineHealthCheckChecks{
		UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
			{
				Type:           corev1.NodeReady,
				Status:         corev1.ConditionUnknown,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
			{
				Type:           corev1.NodeReady,
				Status:         corev1.ConditionFalse,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
		},
		UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
			{
				Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
				Status:         metav1.ConditionUnknown,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
			{
				Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
				Status:         metav1.ConditionFalse,
				TimeoutSeconds: ptr.To(int32(5 * 60)),
			},
		},
		NodeStartupTimeoutSeconds: ptr.To(int32(1)),
	}
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{
		"foo": "bar",
	}}
	healthCheckTarget := builder.MachineDeployment("ns1", "md1").Build()
	cluster := builder.Cluster("ns1", "cluster1").Build()
	want := &clusterv1.MachineHealthCheck{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "MachineHealthCheck",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "md1",
			Namespace: "ns1",
			// Label is added by defaulting values using MachineHealthCheck.Default()
			Labels: map[string]string{
				"cluster.x-k8s.io/cluster-name":     "cluster1",
				clusterv1.ClusterTopologyOwnedLabel: "",
			},
			OwnerReferences: []metav1.OwnerReference{
				*ownerrefs.OwnerReferenceTo(cluster, clusterv1.GroupVersion.WithKind("Cluster")),
			},
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName: cluster.Name,
			Selector: metav1.LabelSelector{MatchLabels: map[string]string{
				"foo": "bar",
			}},
			Checks: clusterv1.MachineHealthCheckChecks{
				UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionUnknown,
						TimeoutSeconds: ptr.To(int32(5 * 60)),
					},
					{
						Type:           corev1.NodeReady,
						Status:         corev1.ConditionFalse,
						TimeoutSeconds: ptr.To(int32(5 * 60)),
					},
				},
				UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
					{
						Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
						Status:         metav1.ConditionUnknown,
						TimeoutSeconds: ptr.To(int32(5 * 60)),
					},
					{
						Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
						Status:         metav1.ConditionFalse,
						TimeoutSeconds: ptr.To(int32(5 * 60)),
					},
				},
				NodeStartupTimeoutSeconds: ptr.To(int32(1)),
			},
		},
	}

	t.Run("set all fields correctly", func(t *testing.T) {
		g := NewWithT(t)

		got := computeMachineHealthCheck(ctx, healthCheckTarget, selector, cluster, mhcChecks, clusterv1.MachineHealthCheckRemediation{})

		g.Expect(got).To(BeComparableTo(want), cmp.Diff(got, want))
	})
}

func TestCalculateRefDesiredAPIVersion(t *testing.T) {
	tests := []struct {
		name                    string
		currentRef              *corev1.ObjectReference
		desiredReferencedObject *unstructured.Unstructured
		want                    *corev1.ObjectReference
		wantErr                 bool
	}{
		{
			name: "Return desired ref if current ref is nil",
			desiredReferencedObject: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"kind":       "DockerCluster",
				"metadata": map[string]interface{}{
					"name":      "my-cluster-abc",
					"namespace": metav1.NamespaceDefault,
				},
			}},
			want: &corev1.ObjectReference{
				APIVersion: clusterv1.GroupVersionInfrastructure.String(),
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Error for invalid apiVersion",
			currentRef: &corev1.ObjectReference{
				APIVersion: "invalid/api/version",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			desiredReferencedObject: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"kind":       "DockerCluster",
				"metadata": map[string]interface{}{
					"name":      "my-cluster-abc",
					"namespace": metav1.NamespaceDefault,
				},
			}},
			wantErr: true,
		},
		{
			name: "Return desired ref if group changed",
			currentRef: &corev1.ObjectReference{
				APIVersion: clusterv1.GroupVersionInfrastructure.String(),
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			desiredReferencedObject: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "infrastructure2.cluster.x-k8s.io/v1beta1",
				"kind":       "DockerCluster",
				"metadata": map[string]interface{}{
					"name":      "my-cluster-abc",
					"namespace": metav1.NamespaceDefault,
				},
			}},
			want: &corev1.ObjectReference{
				// Group changed => apiVersion is taken from desired.
				APIVersion: "infrastructure2.cluster.x-k8s.io/v1beta1",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Return desired ref if kind changed",
			currentRef: &corev1.ObjectReference{
				APIVersion: clusterv1.GroupVersionInfrastructure.String(),
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			desiredReferencedObject: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"kind":       "DockerCluster2",
				"metadata": map[string]interface{}{
					"name":      "my-cluster-abc",
					"namespace": metav1.NamespaceDefault,
				},
			}},
			want: &corev1.ObjectReference{
				// Kind changed => apiVersion is taken from desired.
				APIVersion: clusterv1.GroupVersionInfrastructure.String(),
				Kind:       "DockerCluster2",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
		{
			name: "Return current apiVersion if group and kind are the same",
			currentRef: &corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			desiredReferencedObject: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": clusterv1.GroupVersionInfrastructure.String(),
				"kind":       "DockerCluster",
				"metadata": map[string]interface{}{
					"name":      "my-cluster-abc",
					"namespace": metav1.NamespaceDefault,
				},
			}},
			want: &corev1.ObjectReference{
				// Group and kind are the same => apiVersion is taken from currentRef.
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta2",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := calculateRefDesiredAPIVersion(tt.currentRef, tt.desiredReferencedObject)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got).To(BeComparableTo(tt.want))
		})
	}
}

func TestGenerate(t *testing.T) {
	// templates and ClusterClass
	infrastructureClusterTemplate := builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "template1").
		Build()
	controlPlaneTemplate := builder.ControlPlaneTemplate(metav1.NamespaceDefault, "template1").
		Build()
	clusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithInfrastructureClusterTemplate(infrastructureClusterTemplate).
		WithControlPlaneTemplate(controlPlaneTemplate).
		Build()

	// aggregating templates and cluster class into a blueprint
	blueprint := &scope.ClusterBlueprint{
		ClusterClass:                  clusterClass,
		InfrastructureClusterTemplate: infrastructureClusterTemplate,
		ControlPlane: &scope.ControlPlaneBlueprint{
			Template: controlPlaneTemplate,
		},
	}

	// current cluster objects
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: metav1.NamespaceDefault,
		},
	}

	version := "v1.33.0"
	workerInfrastructureMachinePool := builder.InfrastructureMachinePoolTemplate(metav1.NamespaceDefault, "linux-worker-inframachinepool").
		Build()
	workerBootstrapConfig := builder.BootstrapTemplate(metav1.NamespaceDefault, "linux-worker-bootstrap").
		Build()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-0",
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion: "v1.32.0", // Not yet upgraded to v1.33.0.
			},
		},
	}
	crd := builder.GenericControlPlaneCRD.DeepCopy()

	t.Run("Generate desired state and verify MP is marked as upgrading", func(t *testing.T) {
		g := NewWithT(t)

		fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(node, crd).Build()
		fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().Build()
		clusterCache := clustercache.NewFakeClusterCache(fakeClient, client.ObjectKey{Name: cluster.Name, Namespace: cluster.Namespace})

		desiredStateGenerator, err := NewGenerator(
			fakeClient,
			clusterCache,
			fakeRuntimeClient,
			cache.New[cache.HookEntry](cache.HookCacheDefaultTTL),
			cache.New[GenerateUpgradePlanCacheEntry](10*time.Minute),
		)
		g.Expect(err).ToNot(HaveOccurred())

		s := scope.New(cluster)
		s.Blueprint = blueprint

		mp := builder.MachinePool(metav1.NamespaceDefault, "existing-pool").
			WithVersion(version).
			WithReplicas(3).
			WithBootstrap(workerBootstrapConfig).
			WithInfrastructure(workerInfrastructureMachinePool).
			WithStatus(clusterv1.MachinePoolStatus{
				NodeRefs: []corev1.ObjectReference{
					{
						Kind:      "Node",
						Namespace: metav1.NamespaceDefault,
						Name:      "node-0",
					},
				},
			}).
			Build()

		s.Current.MachinePools = map[string]*scope.MachinePoolState{
			"pool-of-machines": {
				Object:                          mp,
				BootstrapObject:                 workerBootstrapConfig,
				InfrastructureMachinePoolObject: workerInfrastructureMachinePool,
			},
		}

		// Get the desired state.
		desiredState, err := desiredStateGenerator.Generate(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(desiredState).ToNot(BeNil())
		g.Expect(desiredState.Cluster).ToNot(BeNil())
		g.Expect(desiredState.InfrastructureCluster).ToNot(BeNil())
		g.Expect(desiredState.ControlPlane).ToNot(BeNil())
		// Verify MP is marked as upgrading
		g.Expect(s.UpgradeTracker.MachinePools.UpgradingNames()).To(ConsistOf(mp.Name))
	})
}
