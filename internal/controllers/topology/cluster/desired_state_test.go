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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/internal/hooks"
	fakeruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client/fake"
	"sigs.k8s.io/cluster-api/internal/test/builder"
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
	t.Run("Carry over the owner reference to ClusterShim, if any", func(t *testing.T) {
		g := NewWithT(t)
		shim := clusterShim(cluster)

		// current cluster objects for the test scenario
		clusterWithInfrastructureRef := cluster.DeepCopy()
		clusterWithInfrastructureRef.Spec.InfrastructureRef = fakeRef1

		// aggregating current cluster objects into ClusterState (simulating getCurrentState)
		scope := scope.New(clusterWithInfrastructureRef)
		scope.Current.InfrastructureCluster = infrastructureClusterTemplate.DeepCopy()
		scope.Current.InfrastructureCluster.SetOwnerReferences([]metav1.OwnerReference{*ownerReferenceTo(shim)})
		scope.Blueprint = blueprint

		obj, err := computeInfrastructureCluster(ctx, scope)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())
		g.Expect(hasOwnerReferenceFrom(obj, shim)).To(BeTrue())
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
		WithControlPlaneTemplate(controlPlaneTemplate).
		Build()
	// TODO: Replace with object builder.
	// current cluster objects
	version := "v1.21.2"
	replicas := int32(3)
	duration := 10 * time.Second
	nodeDrainTimeout := metav1.Duration{Duration: duration}
	nodeVolumeDetachTimeout := metav1.Duration{Duration: duration}
	nodeDeletionTimeout := metav1.Duration{Duration: duration}
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
					Replicas:                &replicas,
					NodeDrainTimeout:        &nodeDrainTimeout,
					NodeVolumeDetachTimeout: &nodeVolumeDetachTimeout,
					NodeDeletionTimeout:     &nodeDeletionTimeout,
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

		r := &Reconciler{}

		obj, err := r.computeControlPlane(ctx, scope, nil)
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
		assertNestedField(g, obj, duration.String(), contract.ControlPlane().MachineTemplate().NodeDrainTimeout().Path()...)
		assertNestedField(g, obj, duration.String(), contract.ControlPlane().MachineTemplate().NodeVolumeDetachTimeout().Path()...)
		assertNestedField(g, obj, duration.String(), contract.ControlPlane().MachineTemplate().NodeDeletionTimeout().Path()...)
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

		r := &Reconciler{}

		obj, err := r.computeControlPlane(ctx, scope, nil)
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
		s := scope.New(cluster)
		s.Blueprint = blueprint
		s.Current.ControlPlane = &scope.ControlPlaneState{}

		r := &Reconciler{}

		obj, err := r.computeControlPlane(ctx, s, infrastructureMachineTemplate)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())

		assertTemplateToObject(g, assertTemplateInput{
			cluster:     s.Current.Cluster,
			templateRef: blueprint.ClusterClass.Spec.ControlPlane.Ref,
			template:    blueprint.ControlPlane.Template,
			currentRef:  nil,
			obj:         obj,
		})
		gotMetadata, err := contract.ControlPlane().MachineTemplate().Metadata().Get(obj)
		g.Expect(err).ToNot(HaveOccurred())

		expectedLabels := mergeMap(s.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Labels, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Labels)
		expectedLabels[clusterv1.ClusterLabelName] = cluster.Name
		expectedLabels[clusterv1.ClusterTopologyOwnedLabel] = ""
		g.Expect(gotMetadata).To(Equal(&clusterv1.ObjectMeta{
			Labels:      expectedLabels,
			Annotations: mergeMap(s.Current.Cluster.Spec.Topology.ControlPlane.Metadata.Annotations, blueprint.ClusterClass.Spec.ControlPlane.Metadata.Annotations),
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

		r := &Reconciler{}

		obj, err := r.computeControlPlane(ctx, scope, nil)
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

				r := &Reconciler{}

				obj, err := r.computeControlPlane(ctx, s, nil)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(obj).NotTo(BeNil())
				assertNestedField(g, obj, tt.expectedVersion, contract.ControlPlane().Version().Path()...)
			})
		}
	})
	t.Run("Carry over the owner reference to ClusterShim, if any", func(t *testing.T) {
		g := NewWithT(t)
		shim := clusterShim(cluster)

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
		s.Current.ControlPlane.Object.SetOwnerReferences([]metav1.OwnerReference{*ownerReferenceTo(shim)})
		s.Blueprint = blueprint

		r := &Reconciler{}

		obj, err := r.computeControlPlane(ctx, s, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(obj).ToNot(BeNil())
		g.Expect(hasOwnerReferenceFrom(obj, shim)).To(BeTrue())
	})
}

func TestComputeControlPlaneVersion(t *testing.T) {
	t.Run("Compute control plane version under various circumstances", func(t *testing.T) {
		defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()

		// Note: the version used by the machine deployments does
		// not affect how we determining the control plane version.
		// We only want to know if the machine deployments are stable.
		//
		// A machine deployment is considered stable if all the following are true:
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

		catalog := runtimecatalog.New()
		_ = runtimehooksv1.AddToCatalog(catalog)

		beforeClusterUpgradeGVH, err := catalog.GroupVersionHook(runtimehooksv1.BeforeClusterUpgrade)
		if err != nil {
			panic("unable to compute GVH")
		}

		tests := []struct {
			name                    string
			hookResponse            *runtimehooksv1.BeforeClusterUpgradeResponse
			topologyVersion         string
			controlPlaneObj         *unstructured.Unstructured
			machineDeploymentsState scope.MachineDeploymentsStateMap
			expectedVersion         string
			wantErr                 bool
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
				hookResponse:    nonBlockingBeforeClusterUpgradeResponse,
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
				name:            "should return cluster.spec.topology.version if control plane is not upgrading and not scaling and none of the machine deployments are rolling out - hook returns non blocking response",
				hookResponse:    nonBlockingBeforeClusterUpgradeResponse,
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
			{
				name:            "should return the controlplane.spec.version if the BeforeClusterUpgrade hooks returns a blocking response",
				hookResponse:    blockingBeforeClusterUpgradeResponse,
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
				expectedVersion: "v1.2.2",
			},
			{
				name:            "should fail if the BeforeClusterUpgrade hooks returns a failure response",
				hookResponse:    failureBeforeClusterUpgradeResponse,
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
				expectedVersion: "v1.2.2",
				wantErr:         true,
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
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster",
								Namespace: "test-ns",
							},
						},
						ControlPlane:       &scope.ControlPlaneState{Object: tt.controlPlaneObj},
						MachineDeployments: tt.machineDeploymentsState,
					},
					UpgradeTracker:      scope.NewUpgradeTracker(),
					HookResponseTracker: scope.NewHookResponseTracker(),
				}

				runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCatalog(catalog).
					WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
						beforeClusterUpgradeGVH: tt.hookResponse,
					}).
					Build()

				fakeClient := fake.NewClientBuilder().WithObjects(s.Current.Cluster).Build()

				r := &Reconciler{
					Client:        fakeClient,
					APIReader:     fakeClient,
					RuntimeClient: runtimeClient,
				}
				version, err := r.computeControlPlaneVersion(ctx, s)
				if tt.wantErr {
					g.Expect(err).NotTo(BeNil())
				} else {
					g.Expect(err).To(BeNil())
					g.Expect(version).To(Equal(tt.expectedVersion))
				}
			})
		}
	})

	t.Run("Calling AfterControlPlaneUpgrade hook", func(t *testing.T) {
		defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()

		catalog := runtimecatalog.New()
		_ = runtimehooksv1.AddToCatalog(catalog)

		afterControlPlaneUpgradeGVH, err := catalog.GroupVersionHook(runtimehooksv1.AfterControlPlaneUpgrade)
		if err != nil {
			panic(err)
		}

		blockingResponse := &runtimehooksv1.AfterControlPlaneUpgradeResponse{
			CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
				RetryAfterSeconds: int32(10),
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
			},
		}
		nonBlockingResponse := &runtimehooksv1.AfterControlPlaneUpgradeResponse{
			CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
				RetryAfterSeconds: int32(0),
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
			},
		}
		failureResponse := &runtimehooksv1.AfterControlPlaneUpgradeResponse{
			CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusFailure,
				},
			},
		}

		topologyVersion := "v1.2.3"
		lowerVersion := "v1.2.2"
		controlPlaneStable := builder.ControlPlane("test-ns", "cp1").
			WithSpecFields(map[string]interface{}{
				"spec.version":  topologyVersion,
				"spec.replicas": int64(2),
			}).
			WithStatusFields(map[string]interface{}{
				"status.version":         topologyVersion,
				"status.replicas":        int64(2),
				"status.updatedReplicas": int64(2),
				"status.readyReplicas":   int64(2),
			}).
			Build()

		controlPlaneUpgrading := builder.ControlPlane("test-ns", "cp1").
			WithSpecFields(map[string]interface{}{
				"spec.version":  topologyVersion,
				"spec.replicas": int64(2),
			}).
			WithStatusFields(map[string]interface{}{
				"status.version":         lowerVersion,
				"status.replicas":        int64(2),
				"status.updatedReplicas": int64(2),
				"status.readyReplicas":   int64(2),
			}).
			Build()

		controlPlaneProvisioning := builder.ControlPlane("test-ns", "cp1").
			WithSpecFields(map[string]interface{}{
				"spec.version":  "v1.2.2",
				"spec.replicas": int64(2),
			}).
			WithStatusFields(map[string]interface{}{
				"status.version": "",
			}).
			Build()

		tests := []struct {
			name                string
			s                   *scope.Scope
			hookResponse        *runtimehooksv1.AfterControlPlaneUpgradeResponse
			wantIntentToCall    bool
			wantHookToBeCalled  bool
			wantAllowMDUpgrades bool
			wantErr             bool
		}{
			{
				name: "should not call hook if it is not marked",
				s: &scope.Scope{
					Blueprint: &scope.ClusterBlueprint{
						Topology: &clusterv1.Topology{
							Version:      topologyVersion,
							ControlPlane: clusterv1.ControlPlaneTopology{},
						},
					},
					Current: &scope.ClusterState{
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster",
								Namespace: "test-ns",
							},
							Spec: clusterv1.ClusterSpec{},
						},
						ControlPlane: &scope.ControlPlaneState{
							Object: controlPlaneStable,
						},
					},
					UpgradeTracker:      scope.NewUpgradeTracker(),
					HookResponseTracker: scope.NewHookResponseTracker(),
				},
				wantIntentToCall:   false,
				wantHookToBeCalled: false,
				wantErr:            false,
			},
			{
				name: "should not call hook if the control plane is provisioning - there is intent to call hook",
				s: &scope.Scope{
					Blueprint: &scope.ClusterBlueprint{
						Topology: &clusterv1.Topology{
							Version:      topologyVersion,
							ControlPlane: clusterv1.ControlPlaneTopology{},
						},
					},
					Current: &scope.ClusterState{
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster",
								Namespace: "test-ns",
								Annotations: map[string]string{
									runtimev1.PendingHooksAnnotation: "AfterControlPlaneUpgrade",
								},
							},
							Spec: clusterv1.ClusterSpec{},
						},
						ControlPlane: &scope.ControlPlaneState{
							Object: controlPlaneProvisioning,
						},
					},
					UpgradeTracker:      scope.NewUpgradeTracker(),
					HookResponseTracker: scope.NewHookResponseTracker(),
				},
				wantIntentToCall:   true,
				wantHookToBeCalled: false,
				wantErr:            false,
			},
			{
				name: "should not call hook if the control plane is upgrading - there is intent to call hook",
				s: &scope.Scope{
					Blueprint: &scope.ClusterBlueprint{
						Topology: &clusterv1.Topology{
							Version:      topologyVersion,
							ControlPlane: clusterv1.ControlPlaneTopology{},
						},
					},
					Current: &scope.ClusterState{
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster",
								Namespace: "test-ns",
								Annotations: map[string]string{
									runtimev1.PendingHooksAnnotation: "AfterControlPlaneUpgrade",
								},
							},
							Spec: clusterv1.ClusterSpec{},
						},
						ControlPlane: &scope.ControlPlaneState{
							Object: controlPlaneUpgrading,
						},
					},
					UpgradeTracker:      scope.NewUpgradeTracker(),
					HookResponseTracker: scope.NewHookResponseTracker(),
				},
				wantIntentToCall:   true,
				wantHookToBeCalled: false,
				wantErr:            false,
			},
			{
				name: "should call hook if the control plane is at desired version - non blocking response should remove hook from pending hooks list and allow MD upgrades",
				s: &scope.Scope{
					Blueprint: &scope.ClusterBlueprint{
						Topology: &clusterv1.Topology{
							Version:      topologyVersion,
							ControlPlane: clusterv1.ControlPlaneTopology{},
						},
					},
					Current: &scope.ClusterState{
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster",
								Namespace: "test-ns",
								Annotations: map[string]string{
									runtimev1.PendingHooksAnnotation: "AfterControlPlaneUpgrade",
								},
							},
							Spec: clusterv1.ClusterSpec{},
						},
						ControlPlane: &scope.ControlPlaneState{
							Object: controlPlaneStable,
						},
					},
					UpgradeTracker:      scope.NewUpgradeTracker(),
					HookResponseTracker: scope.NewHookResponseTracker(),
				},
				hookResponse:        nonBlockingResponse,
				wantIntentToCall:    false,
				wantHookToBeCalled:  true,
				wantAllowMDUpgrades: true,
				wantErr:             false,
			},
			{
				name: "should call hook if the control plane is at desired version - blocking response should leave the hook in pending hooks list and block MD upgrades",
				s: &scope.Scope{
					Blueprint: &scope.ClusterBlueprint{
						Topology: &clusterv1.Topology{
							Version:      topologyVersion,
							ControlPlane: clusterv1.ControlPlaneTopology{},
						},
					},
					Current: &scope.ClusterState{
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster",
								Namespace: "test-ns",
								Annotations: map[string]string{
									runtimev1.PendingHooksAnnotation: "AfterControlPlaneUpgrade",
								},
							},
							Spec: clusterv1.ClusterSpec{},
						},
						ControlPlane: &scope.ControlPlaneState{
							Object: controlPlaneStable,
						},
					},
					UpgradeTracker:      scope.NewUpgradeTracker(),
					HookResponseTracker: scope.NewHookResponseTracker(),
				},
				hookResponse:        blockingResponse,
				wantIntentToCall:    true,
				wantHookToBeCalled:  true,
				wantAllowMDUpgrades: false,
				wantErr:             false,
			},
			{
				name: "should call hook if the control plane is at desired version - failure response should leave the hook in pending hooks list",
				s: &scope.Scope{
					Blueprint: &scope.ClusterBlueprint{
						Topology: &clusterv1.Topology{
							Version:      topologyVersion,
							ControlPlane: clusterv1.ControlPlaneTopology{},
						},
					},
					Current: &scope.ClusterState{
						Cluster: &clusterv1.Cluster{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-cluster",
								Namespace: "test-ns",
								Annotations: map[string]string{
									runtimev1.PendingHooksAnnotation: "AfterControlPlaneUpgrade",
								},
							},
							Spec: clusterv1.ClusterSpec{},
						},
						ControlPlane: &scope.ControlPlaneState{
							Object: controlPlaneStable,
						},
					},
					UpgradeTracker:      scope.NewUpgradeTracker(),
					HookResponseTracker: scope.NewHookResponseTracker(),
				},
				hookResponse:       failureResponse,
				wantIntentToCall:   true,
				wantHookToBeCalled: true,
				wantErr:            true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)

				fakeRuntimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
					WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
						afterControlPlaneUpgradeGVH: tt.hookResponse,
					}).
					WithCatalog(catalog).
					Build()

				fakeClient := fake.NewClientBuilder().WithObjects(tt.s.Current.Cluster).Build()

				r := &Reconciler{
					Client:        fakeClient,
					APIReader:     fakeClient,
					RuntimeClient: fakeRuntimeClient,
				}

				_, err := r.computeControlPlaneVersion(ctx, tt.s)
				g.Expect(fakeRuntimeClient.CallAllCount(runtimehooksv1.AfterControlPlaneUpgrade) == 1).To(Equal(tt.wantHookToBeCalled))
				g.Expect(hooks.IsPending(runtimehooksv1.AfterControlPlaneUpgrade, tt.s.Current.Cluster)).To(Equal(tt.wantIntentToCall))
				g.Expect(err != nil).To(Equal(tt.wantErr))
				if tt.wantHookToBeCalled && !tt.wantErr {
					g.Expect(tt.s.UpgradeTracker.MachineDeployments.AllowUpgrade()).To(Equal(tt.wantAllowMDUpgrades))
				}
			})
		}
	})

	t.Run("register intent to call AfterClusterUpgrade and AfterControlPlaneUpgrade hooks", func(t *testing.T) {
		defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()

		catalog := runtimecatalog.New()
		_ = runtimehooksv1.AddToCatalog(catalog)
		beforeClusterUpgradeGVH, err := catalog.GroupVersionHook(runtimehooksv1.BeforeClusterUpgrade)
		if err != nil {
			panic("unable to compute GVH")
		}
		beforeClusterUpgradeNonBlockingResponse := &runtimehooksv1.BeforeClusterUpgradeResponse{
			CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
				CommonResponse: runtimehooksv1.CommonResponse{
					Status: runtimehooksv1.ResponseStatusSuccess,
				},
			},
		}

		controlPlaneStable := builder.ControlPlane("test-ns", "cp1").
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

		s := &scope.Scope{
			Blueprint: &scope.ClusterBlueprint{Topology: &clusterv1.Topology{
				Version: "v1.2.3",
				ControlPlane: clusterv1.ControlPlaneTopology{
					Replicas: pointer.Int32(2),
				},
			}},
			Current: &scope.ClusterState{
				Cluster: &clusterv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: "test-ns",
					},
				},
				ControlPlane: &scope.ControlPlaneState{Object: controlPlaneStable},
			},
			UpgradeTracker:      scope.NewUpgradeTracker(),
			HookResponseTracker: scope.NewHookResponseTracker(),
		}

		runtimeClient := fakeruntimeclient.NewRuntimeClientBuilder().
			WithCatalog(catalog).
			WithCallAllExtensionResponses(map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject{
				beforeClusterUpgradeGVH: beforeClusterUpgradeNonBlockingResponse,
			}).
			Build()

		fakeClient := fake.NewClientBuilder().WithObjects(s.Current.Cluster).Build()

		r := &Reconciler{
			Client:        fakeClient,
			APIReader:     fakeClient,
			RuntimeClient: runtimeClient,
		}

		desiredVersion, err := r.computeControlPlaneVersion(ctx, s)
		g := NewWithT(t)
		g.Expect(err).To(BeNil())
		// When successfully picking up the new version the intent to call AfterControlPlaneUpgrade and AfterClusterUpgrade hooks should be registered.
		g.Expect(desiredVersion).To(Equal("v1.2.3"))
		g.Expect(hooks.IsPending(runtimehooksv1.AfterControlPlaneUpgrade, s.Current.Cluster)).To(BeTrue())
		g.Expect(hooks.IsPending(runtimehooksv1.AfterClusterUpgrade, s.Current.Cluster)).To(BeTrue())
	})
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

	obj, err := computeCluster(ctx, scope, infrastructureCluster, controlPlane)
	g.Expect(err).ToNot(HaveOccurred())
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

	unhealthyConditions := []clusterv1.UnhealthyCondition{
		{
			Type:    corev1.NodeReady,
			Status:  corev1.ConditionUnknown,
			Timeout: metav1.Duration{Duration: 5 * time.Minute},
		},
		{
			Type:    corev1.NodeReady,
			Status:  corev1.ConditionFalse,
			Timeout: metav1.Duration{Duration: 5 * time.Minute},
		},
	}
	nodeTimeoutDuration := &metav1.Duration{Duration: time.Duration(1)}

	md1 := builder.MachineDeploymentClass("linux-worker").
		WithLabels(labels).
		WithAnnotations(annotations).
		WithInfrastructureTemplate(workerInfrastructureMachineTemplate).
		WithBootstrapTemplate(workerBootstrapTemplate).
		WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{
			UnhealthyConditions: unhealthyConditions,
			NodeStartupTimeout:  nodeTimeoutDuration,
		}).
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
				MachineHealthCheck: &clusterv1.MachineHealthCheckClass{
					UnhealthyConditions: unhealthyConditions,
					NodeStartupTimeout: &metav1.Duration{
						Duration: time.Duration(1)},
				},
			},
		},
	}

	replicas := int32(5)
	failureDomain := "always-up-region"
	nodeDrainTimeout := metav1.Duration{Duration: 10 * time.Second}
	nodeVolumeDetachTimeout := metav1.Duration{Duration: 10 * time.Second}
	nodeDeletionTimeout := metav1.Duration{Duration: 10 * time.Second}
	minReadySeconds := int32(5)
	mdTopology := clusterv1.MachineDeploymentTopology{
		Metadata: clusterv1.ObjectMeta{
			Labels: map[string]string{"foo": "baz"},
		},
		Class:                   "linux-worker",
		Name:                    "big-pool-of-machines",
		Replicas:                &replicas,
		FailureDomain:           &failureDomain,
		NodeDrainTimeout:        &nodeDrainTimeout,
		NodeVolumeDetachTimeout: &nodeVolumeDetachTimeout,
		NodeDeletionTimeout:     &nodeDeletionTimeout,
		MinReadySeconds:         &minReadySeconds,
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
		g.Expect(*actualMd.Spec.MinReadySeconds).To(Equal(minReadySeconds))
		g.Expect(*actualMd.Spec.Template.Spec.FailureDomain).To(Equal(failureDomain))
		g.Expect(*actualMd.Spec.Template.Spec.NodeDrainTimeout).To(Equal(nodeDrainTimeout))
		g.Expect(*actualMd.Spec.Template.Spec.NodeDeletionTimeout).To(Equal(nodeDeletionTimeout))
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
		g.Expect(*actualMd.Spec.Template.Spec.FailureDomain).To(Equal(failureDomain))
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
		// A more extensive list of scenarios is tested in TestComputeMachineDeploymentVersion.
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

	t.Run("Should correctly generate a MachineHealthCheck for the MachineDeployment", func(t *testing.T) {
		g := NewWithT(t)
		scope := scope.New(cluster)
		scope.Blueprint = blueprint
		mdTopology := clusterv1.MachineDeploymentTopology{
			Class: "linux-worker",
			Name:  "big-pool-of-machines",
		}

		actual, err := computeMachineDeployment(ctx, scope, nil, mdTopology)
		g.Expect(err).To(BeNil())
		// Check that the ClusterName and selector are set properly for the MachineHealthCheck.
		g.Expect(actual.MachineHealthCheck.Spec.ClusterName).To(Equal(cluster.Name))
		g.Expect(actual.MachineHealthCheck.Spec.Selector).To(Equal(metav1.LabelSelector{MatchLabels: map[string]string{
			clusterv1.ClusterTopologyOwnedLabel:                 actual.Object.Spec.Selector.MatchLabels[clusterv1.ClusterTopologyOwnedLabel],
			clusterv1.ClusterTopologyMachineDeploymentLabelName: actual.Object.Spec.Selector.MatchLabels[clusterv1.ClusterTopologyMachineDeploymentLabelName],
		}}))

		// Check that the NodeStartupTime is set as expected.
		g.Expect(actual.MachineHealthCheck.Spec.NodeStartupTimeout).To(Equal(nodeTimeoutDuration))

		// Check that UnhealthyConditions are set as expected.
		g.Expect(actual.MachineHealthCheck.Spec.UnhealthyConditions).To(Equal(unhealthyConditions))
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

	// A machine deployment is considered stable if all the following are true:
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

func Test_computeMachineHealthCheck(t *testing.T) {
	maxUnhealthyValue := intstr.FromString("100%")
	mhcSpec := &clusterv1.MachineHealthCheckClass{
		UnhealthyConditions: []clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionFalse,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		},
		NodeStartupTimeout: &metav1.Duration{
			Duration: time.Duration(1)},
	}
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{
		"foo": "bar",
	}}
	healthCheckTarget := builder.MachineDeployment("ns1", "md1").Build()
	clusterName := "cluster1"
	want := &clusterv1.MachineHealthCheck{
		TypeMeta: metav1.TypeMeta{
			Kind:       clusterv1.GroupVersion.WithKind("MachineHealthCheck").Kind,
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "md1",
			Namespace: "ns1",
			// Label is added by defaulting values using MachineHealthCheck.Default()
			Labels: map[string]string{"cluster.x-k8s.io/cluster-name": "cluster1"},
		},
		Spec: clusterv1.MachineHealthCheckSpec{
			ClusterName: "cluster1",
			Selector: metav1.LabelSelector{MatchLabels: map[string]string{
				"foo": "bar",
			}},
			// MaxUnhealthy is added by defaulting values using MachineHealthCheck.Default()
			MaxUnhealthy: &maxUnhealthyValue,
			UnhealthyConditions: []clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Timeout: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
			NodeStartupTimeout: &metav1.Duration{
				Duration: time.Duration(1)},
		},
	}

	t.Run("set all fields correctly", func(t *testing.T) {
		g := NewWithT(t)

		got := computeMachineHealthCheck(healthCheckTarget, selector, clusterName, mhcSpec)

		g.Expect(got).To(Equal(want), cmp.Diff(got, want))
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
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"kind":       "DockerCluster",
				"metadata": map[string]interface{}{
					"name":      "my-cluster-abc",
					"namespace": metav1.NamespaceDefault,
				},
			}},
			want: &corev1.ObjectReference{
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
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
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
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
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
				Kind:       "DockerCluster",
				Name:       "my-cluster-abc",
				Namespace:  metav1.NamespaceDefault,
			},
			desiredReferencedObject: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"kind":       "DockerCluster2",
				"metadata": map[string]interface{}{
					"name":      "my-cluster-abc",
					"namespace": metav1.NamespaceDefault,
				},
			}},
			want: &corev1.ObjectReference{
				// Kind changed => apiVersion is taken from desired.
				APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
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
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
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

			g.Expect(got).To(Equal(tt.want))
		})
	}
}
