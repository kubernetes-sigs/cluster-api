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
	"context"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/internal/contract"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/mergepatch"
	"sigs.k8s.io/cluster-api/internal/controllers/topology/cluster/scope"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	. "sigs.k8s.io/cluster-api/internal/test/matchers"
	"sigs.k8s.io/cluster-api/util/patch"
)

var (
	IgnoreTopologyManagedFieldAnnotation = IgnorePaths{
		{"metadata", "annotations", clusterv1.ClusterTopologyManagedFieldsAnnotation},
	}
)

func TestReconcileShim(t *testing.T) {
	infrastructureCluster := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").Build()
	controlPlane := builder.ControlPlane(metav1.NamespaceDefault, "infrastructure-cluster1").Build()
	cluster1 := builder.Cluster(metav1.NamespaceDefault, "cluster1").Build()
	cluster1Shim := clusterShim(cluster1)

	t.Run("Shim gets created when InfrastructureCluster and ControlPlane object have to be created", func(t *testing.T) {
		g := NewWithT(t)

		// Create a scope with a cluster and InfrastructureCluster yet to be created.
		s := scope.New(cluster1)
		s.Desired = &scope.ClusterState{
			InfrastructureCluster: infrastructureCluster.DeepCopy(),
			ControlPlane: &scope.ControlPlaneState{
				Object: controlPlane.DeepCopy(),
			},
		}

		// Set up an empty fake client.
		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			Build()

		// Run reconcileClusterShim.
		r := Reconciler{
			Client: fakeClient,
		}
		err := r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(err).ToNot(HaveOccurred())

		// Check shim is assigned as owner for InfrastructureCluster and ControlPlane objects.
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()[0].Name).To(Equal(shim.Name))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()[0].Name).To(Equal(shim.Name))
	})
	t.Run("Shim creation is re-entrant", func(t *testing.T) {
		g := NewWithT(t)

		// Create a scope with a cluster and InfrastructureCluster yet to be created.
		s := scope.New(cluster1)
		s.Desired = &scope.ClusterState{
			InfrastructureCluster: infrastructureCluster.DeepCopy(),
			ControlPlane: &scope.ControlPlaneState{
				Object: controlPlane.DeepCopy(),
			},
		}

		// Set up a fake client with an already existing shim.
		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(cluster1Shim.DeepCopy()).
			Build()

		// Run reconcileClusterShim.
		r := Reconciler{
			Client: fakeClient,
		}
		err := r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(err).ToNot(HaveOccurred())

		// Check shim is assigned as owner for InfrastructureCluster and ControlPlane objects.
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.InfrastructureCluster.GetOwnerReferences()[0].Name).To(Equal(shim.Name))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()).To(HaveLen(1))
		g.Expect(s.Desired.ControlPlane.Object.GetOwnerReferences()[0].Name).To(Equal(shim.Name))
	})
	t.Run("Shim is not deleted if InfrastructureCluster and ControlPlane object are waiting to be reconciled", func(t *testing.T) {
		g := NewWithT(t)

		// Create a scope with a cluster and InfrastructureCluster created but not yet reconciled.
		s := scope.New(cluster1)
		s.Current.InfrastructureCluster = infrastructureCluster.DeepCopy()
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object: controlPlane.DeepCopy(),
		}

		// Add the shim as a temporary owner for the InfrastructureCluster and ControlPlane.
		ownerRefs := s.Current.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		s.Current.InfrastructureCluster.SetOwnerReferences(ownerRefs)
		ownerRefs = s.Current.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		s.Current.ControlPlane.Object.SetOwnerReferences(ownerRefs)

		// Set up a fake client with an already existing shim.
		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(cluster1Shim.DeepCopy()).
			Build()

		// Run reconcileClusterShim.
		r := Reconciler{
			Client: fakeClient,
		}
		err := r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(err).ToNot(HaveOccurred())
	})
	t.Run("Shim gets deleted when InfrastructureCluster and ControlPlane object have been reconciled", func(t *testing.T) {
		g := NewWithT(t)

		// Create a scope with a cluster and InfrastructureCluster created and reconciled.
		s := scope.New(cluster1)
		s.Current.InfrastructureCluster = infrastructureCluster.DeepCopy()
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object: controlPlane.DeepCopy(),
		}

		// Add the shim as a temporary owner for the InfrastructureCluster and ControlPlane.
		// Add the cluster as a final owner for the InfrastructureCluster and ControlPlane (reconciled).
		ownerRefs := s.Current.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.InfrastructureCluster.SetOwnerReferences(ownerRefs)
		ownerRefs = s.Current.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1Shim))
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.ControlPlane.Object.SetOwnerReferences(ownerRefs)

		// Set up a fake client with an already existing shim.
		fakeClient := fake.NewClientBuilder().
			WithScheme(fakeScheme).
			WithObjects(cluster1Shim.DeepCopy()).
			Build()

		// Run reconcileClusterShim.
		r := Reconciler{
			Client: fakeClient,
		}
		err := r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())

		// Check cluster shim exists.
		shim := cluster1Shim.DeepCopy()
		err = r.Client.Get(ctx, client.ObjectKeyFromObject(shim), shim)
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
	t.Run("No op if InfrastructureCluster and ControlPlane object have been reconciled and shim is gone", func(t *testing.T) {
		g := NewWithT(t)

		// Create a scope with a cluster and InfrastructureCluster created and reconciled.
		s := scope.New(cluster1)
		s.Current.InfrastructureCluster = infrastructureCluster.DeepCopy()
		s.Current.ControlPlane = &scope.ControlPlaneState{
			Object: controlPlane.DeepCopy(),
		}

		// Add the cluster as a final owner for the InfrastructureCluster and ControlPlane (reconciled).
		ownerRefs := s.Current.InfrastructureCluster.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.InfrastructureCluster.SetOwnerReferences(ownerRefs)
		ownerRefs = s.Current.ControlPlane.Object.GetOwnerReferences()
		ownerRefs = append(ownerRefs, *ownerReferenceTo(cluster1))
		s.Current.ControlPlane.Object.SetOwnerReferences(ownerRefs)

		// Run reconcileClusterShim using a nil client, so an error will be triggered if any operation is attempted
		r := Reconciler{
			Client: nil,
		}
		err := r.reconcileClusterShim(ctx, s)
		g.Expect(err).ToNot(HaveOccurred())
	})
}

func TestReconcileCluster(t *testing.T) {
	cluster1 := builder.Cluster(metav1.NamespaceDefault, "cluster1").
		Build()
	cluster1WithReferences := builder.Cluster(metav1.NamespaceDefault, "cluster1").
		WithInfrastructureCluster(builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").
			Build()).
		WithControlPlane(builder.ControlPlane(metav1.NamespaceDefault, "control-plane1").Build()).
		Build()
	cluster2WithReferences := cluster1WithReferences.DeepCopy()
	cluster2WithReferences.SetGroupVersionKind(cluster1WithReferences.GroupVersionKind())
	cluster2WithReferences.Name = "cluster2"

	tests := []struct {
		name    string
		current *clusterv1.Cluster
		desired *clusterv1.Cluster
		want    *clusterv1.Cluster
		wantErr bool
	}{
		{
			name:    "Should update the cluster if infrastructure and control plane references are not set",
			current: cluster1,
			desired: cluster1WithReferences,
			want:    cluster1WithReferences,
			wantErr: false,
		},
		{
			name:    "Should be a no op if infrastructure and control plane references are already set",
			current: cluster2WithReferences,
			desired: cluster2WithReferences,
			want:    cluster2WithReferences,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeObjs := make([]client.Object, 0)
			if tt.current != nil {
				fakeObjs = append(fakeObjs, tt.current)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			s := scope.New(tt.current)

			s.Desired = &scope.ClusterState{Cluster: tt.desired}

			r := Reconciler{
				Client:   fakeClient,
				recorder: env.GetEventRecorderFor("test"),
			}
			err := r.reconcileCluster(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			got := tt.want.DeepCopy()
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.want), got)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got.Spec.InfrastructureRef).To(EqualObject(tt.want.Spec.InfrastructureRef))
			g.Expect(got.Spec.ControlPlaneRef).To(EqualObject(tt.want.Spec.ControlPlaneRef))
		})
	}
}

func TestReconcileInfrastructureCluster(t *testing.T) {
	g := NewWithT(t)

	clusterInfrastructure1 := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure2 := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster2").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure3 := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster3").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure3WithInstanceSpecificChanges := clusterInfrastructure3.DeepCopy()
	clusterInfrastructure3WithInstanceSpecificChanges.SetLabels(map[string]string{"foo": "bar"})
	clusterInfrastructure4 := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster4").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure4WithTemplateOverridingChanges := clusterInfrastructure4.DeepCopy()
	err := unstructured.SetNestedField(clusterInfrastructure4WithTemplateOverridingChanges.UnstructuredContent(), false, "spec", "fakeSetting")
	g.Expect(err).ToNot(HaveOccurred())
	clusterInfrastructure5 := builder.InfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster5").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()

	tests := []struct {
		name    string
		current *unstructured.Unstructured
		desired *unstructured.Unstructured
		want    *unstructured.Unstructured
		wantErr bool
	}{
		{
			name:    "Should create desired InfrastructureCluster if the current does not exists yet",
			current: nil,
			desired: clusterInfrastructure1,
			want:    clusterInfrastructure1,
			wantErr: false,
		},
		{
			name:    "No-op if current InfrastructureCluster is equal to desired",
			current: clusterInfrastructure2,
			desired: clusterInfrastructure2,
			want:    clusterInfrastructure2,
			wantErr: false,
		},
		{
			name:    "Should preserve instance specific changes",
			current: clusterInfrastructure3WithInstanceSpecificChanges,
			desired: clusterInfrastructure3,
			want:    clusterInfrastructure3WithInstanceSpecificChanges,
			wantErr: false,
		},
		{
			name:    "Should restore template values if overridden",
			current: clusterInfrastructure4WithTemplateOverridingChanges,
			desired: clusterInfrastructure4,
			want:    clusterInfrastructure4,
			wantErr: false,
		},
		{
			name:    "Fails for incompatible changes",
			current: clusterInfrastructure5,
			desired: clusterInfrastructure1,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeObjs := make([]client.Object, 0)
			if tt.current != nil {
				fakeObjs = append(fakeObjs, tt.current)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			s := scope.New(&clusterv1.Cluster{})
			s.Current.InfrastructureCluster = tt.current

			s.Desired = &scope.ClusterState{InfrastructureCluster: tt.desired}

			r := Reconciler{
				Client:   fakeClient,
				recorder: env.GetEventRecorderFor("test"),
			}
			err := r.reconcileInfrastructureCluster(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			got := tt.want.DeepCopy() // this is required otherwise Get will modify tt.want
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.want), got)
			g.Expect(err).ToNot(HaveOccurred())

			// Spec
			wantSpec, ok, err := unstructured.NestedMap(tt.want.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			gotSpec, ok, err := unstructured.NestedMap(got.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())
			for k, v := range wantSpec {
				g.Expect(gotSpec).To(HaveKeyWithValue(k, v))
			}
		})
	}
}

func TestReconcileControlPlaneObject(t *testing.T) {
	g := NewWithT(t)
	// Create InfrastructureMachineTemplates for test cases
	infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()
	infrastructureMachineTemplate2 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra2").Build()

	// Infrastructure object with a different Kind.
	incompatibleInfrastructureMachineTemplate := infrastructureMachineTemplate2.DeepCopy()
	incompatibleInfrastructureMachineTemplate.SetKind("incompatibleInfrastructureMachineTemplate")
	updatedInfrastructureMachineTemplate := infrastructureMachineTemplate.DeepCopy()
	err := unstructured.SetNestedField(updatedInfrastructureMachineTemplate.UnstructuredContent(), true, "spec", "differentSetting")
	g.Expect(err).ToNot(HaveOccurred())

	// Create cluster class which does not require controlPlaneInfrastructure.
	ccWithoutControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{}
	// Create clusterClasses requiring controlPlaneInfrastructure and one not.
	ccWithControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{InfrastructureMachineTemplate: infrastructureMachineTemplate}
	// Create ControlPlaneObjects for test cases.
	controlPlane1 := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Build()
	controlPlane2 := builder.ControlPlane(metav1.NamespaceDefault, "cp2").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate2).
		Build()
	// ControlPlane object with novel field in the spec.
	controlPlane3 := controlPlane1.DeepCopy()
	err = unstructured.SetNestedField(controlPlane3.UnstructuredContent(), true, "spec", "differentSetting")
	g.Expect(err).ToNot(HaveOccurred())
	// ControlPlane object with a new label.
	controlPlaneWithInstanceSpecificChanges := controlPlane1.DeepCopy()
	controlPlaneWithInstanceSpecificChanges.SetLabels(map[string]string{"foo": "bar"})
	// ControlPlane object with instance specific machine template labels.
	controlPlaneWithInstanceSpecificMachineTemplateLabels := controlPlane1.DeepCopy()
	err = contract.ControlPlane().MachineTemplate().Metadata().Set(controlPlaneWithInstanceSpecificMachineTemplateLabels, &clusterv1.ObjectMeta{Labels: map[string]string{"foo": "bar"}})
	g.Expect(err).ToNot(HaveOccurred())

	tests := []struct {
		name    string
		class   *scope.ControlPlaneBlueprint
		current *scope.ControlPlaneState
		desired *scope.ControlPlaneState
		want    *scope.ControlPlaneState
		wantErr bool
	}{
		{
			name:    "Should create desired ControlPlane if the current does not exist",
			class:   ccWithoutControlPlaneInfrastructure,
			current: nil,
			desired: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: false,
		},
		{
			name:    "Fail on updating ControlPlaneObject with incompatible changes, here a different Kind for the infrastructureMachineTemplate",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane2.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: true,
		},
		{
			name:    "Update to ControlPlaneObject with no update to the underlying infrastructure",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane3.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlane3.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: false,
		},
		{
			name:    "Update to ControlPlaneObject with underlying infrastructure.",
			class:   ccWithControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{InfrastructureMachineTemplate: nil},
			desired: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: false,
		},
		{
			name:    "Update to ControlPlaneObject with no underlying infrastructure",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane3.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlane3.DeepCopy()},
			wantErr: false,
		},
		{
			name:    "Preserve specific changes to the ControlPlaneObject",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlaneWithInstanceSpecificChanges.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlaneWithInstanceSpecificChanges.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: false,
		},
		{
			name:    "Enforce machineTemplate.metadata",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlaneWithInstanceSpecificMachineTemplateLabels.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeObjs := make([]client.Object, 0)
			s := scope.New(builder.Cluster(metav1.NamespaceDefault, "cluster1").Build())
			s.Blueprint = &scope.ClusterBlueprint{
				ClusterClass: &clusterv1.ClusterClass{},
			}
			if tt.class.InfrastructureMachineTemplate != nil {
				s.Blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
					Ref: contract.ObjToRef(tt.class.InfrastructureMachineTemplate),
				}
			}

			s.Current.ControlPlane = &scope.ControlPlaneState{}
			if tt.current != nil {
				s.Current.ControlPlane = tt.current
				if tt.current.Object != nil {
					fakeObjs = append(fakeObjs, tt.current.Object)
				}
				if tt.current.InfrastructureMachineTemplate != nil {
					fakeObjs = append(fakeObjs, tt.current.InfrastructureMachineTemplate)
				}
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			r := Reconciler{
				Client:   fakeClient,
				recorder: env.GetEventRecorderFor("test"),
			}

			s.Desired = &scope.ClusterState{
				ControlPlane: &scope.ControlPlaneState{
					Object:                        tt.desired.Object,
					InfrastructureMachineTemplate: tt.desired.InfrastructureMachineTemplate,
				},
			}

			// Run reconcileControlPlane with the states created in the initial section of the test.
			err := r.reconcileControlPlane(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Create ControlPlane object for fetching data into
			gotControlPlaneObject := builder.ControlPlane("", "").Build()
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.want.Object), gotControlPlaneObject)
			g.Expect(err).ToNot(HaveOccurred())

			// Get the spec from the ControlPlaneObject we are expecting
			wantControlPlaneObjectSpec, ok, err := unstructured.NestedMap(tt.want.Object.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			// Get the spec from the ControlPlaneObject we got from the client.Get
			gotControlPlaneObjectSpec, ok, err := unstructured.NestedMap(gotControlPlaneObject.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())
			for k, v := range wantControlPlaneObjectSpec {
				g.Expect(gotControlPlaneObjectSpec).To(HaveKeyWithValue(k, v))
			}
			for k, v := range tt.want.Object.GetLabels() {
				g.Expect(gotControlPlaneObject.GetLabels()).To(HaveKeyWithValue(k, v))
			}
		})
	}
}

func TestReconcileControlPlaneInfrastructureMachineTemplate(t *testing.T) {
	g := NewWithT(t)

	// Create InfrastructureMachineTemplates for test cases
	infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").
		Build()
	infrastructureMachineTemplate2 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra2").
		Build()

	// Create the blueprint mandating controlPlaneInfrastructure.
	blueprint := &scope.ClusterBlueprint{
		ClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
			WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).
			Build(),
		ControlPlane: &scope.ControlPlaneBlueprint{
			InfrastructureMachineTemplate: infrastructureMachineTemplate,
		},
	}

	// Create Cluster object for test cases.
	cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").Build()

	// Infrastructure object with a different Kind.
	incompatibleInfrastructureMachineTemplate := infrastructureMachineTemplate2.DeepCopy()
	incompatibleInfrastructureMachineTemplate.SetKind("incompatibleInfrastructureMachineTemplate")
	updatedInfrastructureMachineTemplate := infrastructureMachineTemplate.DeepCopy()
	err := unstructured.SetNestedField(updatedInfrastructureMachineTemplate.UnstructuredContent(), true, "spec", "differentSetting")
	g.Expect(err).ToNot(HaveOccurred())
	// Create ControlPlaneObjects for test cases.
	controlPlane1 := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Build()
	// ControlPlane object with novel field in the spec.
	controlPlane2 := controlPlane1.DeepCopy()
	err = unstructured.SetNestedField(controlPlane2.UnstructuredContent(), true, "spec", "differentSetting")
	g.Expect(err).ToNot(HaveOccurred())
	// ControlPlane object with a new label.
	controlPlaneWithInstanceSpecificChanges := controlPlane1.DeepCopy()
	controlPlaneWithInstanceSpecificChanges.SetLabels(map[string]string{"foo": "bar"})
	// ControlPlane object with the same name as controlPlane1 but a different InfrastructureMachineTemplate
	controlPlane3 := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(updatedInfrastructureMachineTemplate).
		Build()

	tests := []struct {
		name    string
		current *scope.ControlPlaneState
		desired *scope.ControlPlaneState
		want    *scope.ControlPlaneState
		wantErr bool
	}{
		{
			name:    "Create desired InfrastructureMachineTemplate where it doesn't exist",
			current: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: false,
		},
		{
			name:    "Update desired InfrastructureMachineTemplate connected to controlPlane",
			current: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane3, InfrastructureMachineTemplate: updatedInfrastructureMachineTemplate},
			want:    &scope.ControlPlaneState{Object: controlPlane3, InfrastructureMachineTemplate: updatedInfrastructureMachineTemplate},
			wantErr: false,
		},
		{
			name:    "Fail on updating infrastructure with incompatible changes",
			current: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			desired: &scope.ControlPlaneState{Object: controlPlane1.DeepCopy(), InfrastructureMachineTemplate: incompatibleInfrastructureMachineTemplate},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeObjs := make([]client.Object, 0)
			s := scope.New(cluster)
			s.Blueprint = blueprint
			if tt.current != nil {
				s.Current.ControlPlane = tt.current
				if tt.current.Object != nil {
					fakeObjs = append(fakeObjs, tt.current.Object)
				}
				if tt.current.InfrastructureMachineTemplate != nil {
					fakeObjs = append(fakeObjs, tt.current.InfrastructureMachineTemplate)
				}
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			r := Reconciler{
				Client:   fakeClient,
				recorder: env.GetEventRecorderFor("test"),
			}
			s.Desired = &scope.ClusterState{ControlPlane: &scope.ControlPlaneState{Object: tt.desired.Object, InfrastructureMachineTemplate: tt.desired.InfrastructureMachineTemplate}}

			// Run reconcileControlPlane with the states created in the initial section of the test.
			err := r.reconcileControlPlane(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			// Create ControlPlane object for fetching data into
			gotControlPlaneObject := builder.ControlPlane("", "").Build()
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.want.Object), gotControlPlaneObject)
			g.Expect(err).ToNot(HaveOccurred())

			// Check to see if the controlPlaneObject has been updated with a new template.
			// This check is just for the naming format uses by generated templates - here it's templateName-*
			// This check is only performed when we had an initial template that has been changed
			if tt.current.InfrastructureMachineTemplate != nil {
				item, err := contract.ControlPlane().MachineTemplate().InfrastructureRef().Get(gotControlPlaneObject)
				g.Expect(err).ToNot(HaveOccurred())
				pattern := fmt.Sprintf("%s.*", controlPlaneInfrastructureMachineTemplateNamePrefix(s.Current.Cluster.Name))
				fmt.Println(pattern, item.Name)
				ok, err := regexp.Match(pattern, []byte(item.Name))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ok).To(BeTrue())
			}

			// Create object to hold the queried InfrastructureMachineTemplate
			gotInfrastructureMachineTemplate := builder.InfrastructureMachineTemplate("", "").Build()
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.want.InfrastructureMachineTemplate), gotInfrastructureMachineTemplate)
			g.Expect(err).ToNot(HaveOccurred())

			// Get the spec from the InfrastructureMachineTemplate we are expecting
			wantInfrastructureMachineTemplateSpec, ok, err := unstructured.NestedMap(tt.want.InfrastructureMachineTemplate.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			// Get the spec from the InfrastructureMachineTemplate we got from the client.Get
			gotInfrastructureMachineTemplateSpec, ok, err := unstructured.NestedMap(gotInfrastructureMachineTemplate.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			// Compare all keys and values in the InfrastructureMachineTemplate Spec
			for k, v := range wantInfrastructureMachineTemplateSpec {
				g.Expect(gotInfrastructureMachineTemplateSpec).To(HaveKeyWithValue(k, v))
			}

			// Check to see that labels are as expected on the object
			for k, v := range tt.want.InfrastructureMachineTemplate.GetLabels() {
				g.Expect(gotInfrastructureMachineTemplate.GetLabels()).To(HaveKeyWithValue(k, v))
			}
		})
	}
}

func TestReconcileControlPlaneMachineHealthCheck(t *testing.T) {
	g := NewWithT(t)
	// Create InfrastructureMachineTemplates for test cases
	infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()

	mhcClass := &clusterv1.MachineHealthCheckClass{
		UnhealthyConditions: []clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		},
	}
	maxUnhealthy := intstr.Parse("45%")
	// Create clusterClasses requiring controlPlaneInfrastructure and one not.
	ccWithControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{
		InfrastructureMachineTemplate: infrastructureMachineTemplate,
		MachineHealthCheck:            mhcClass,
	}
	ccWithoutControlPlaneInfrastructure := &scope.ControlPlaneBlueprint{
		MachineHealthCheck: mhcClass,
	}

	// Create ControlPlane Object.
	controlPlane1 := builder.ControlPlane(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Build()

	mhcBuilder := builder.MachineHealthCheck(metav1.NamespaceDefault, "cp1").
		WithSelector(*selectorForControlPlaneMHC()).
		WithUnhealthyConditions(mhcClass.UnhealthyConditions).
		WithClusterName("cluster1")

	tests := []struct {
		name    string
		class   *scope.ControlPlaneBlueprint
		current *scope.ControlPlaneState
		desired *scope.ControlPlaneState
		want    *clusterv1.MachineHealthCheck
	}{
		{
			name:    "Should create desired ControlPlane MachineHealthCheck for a new ControlPlane",
			class:   ccWithControlPlaneInfrastructure,
			current: nil,
			desired: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.Build()},
			want: mhcBuilder.
				WithOwnerReferences([]metav1.OwnerReference{*ownerReferenceTo(controlPlane1)}).
				Build(),
		},
		{
			name:  "Should not create ControlPlane MachineHealthCheck when no MachineInfrastructure is defined",
			class: ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{
				Object: controlPlane1.DeepCopy(),
				// Note this creation would be blocked by the validation Webhook. MHC with no MachineInfrastructure is not allowed.
				MachineHealthCheck: mhcBuilder.Build()},
			desired: &scope.ControlPlaneState{
				Object: controlPlane1.DeepCopy(),
				// ControlPlane does not have defined MachineInfrastructure.
				//InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
			},
			want: nil,
		},
		{
			name:  "Should update ControlPlane MachineHealthCheck when changed in desired state",
			class: ccWithControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.Build()},
			desired: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.WithMaxUnhealthy(&maxUnhealthy).Build(),
			},
			// Want to get the updated version of the MachineHealthCheck after reconciliation.
			want: mhcBuilder.WithMaxUnhealthy(&maxUnhealthy).Build(),
		},
		{
			name:  "Should delete ControlPlane MachineHealthCheck when removed from desired state",
			class: ccWithControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				MachineHealthCheck:            mhcBuilder.Build()},
			desired: &scope.ControlPlaneState{
				Object:                        controlPlane1.DeepCopy(),
				InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy(),
				// MachineHealthCheck removed from the desired state of the ControlPlane
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeObjs := make([]client.Object, 0)
			s := scope.New(builder.Cluster(metav1.NamespaceDefault, "cluster1").Build())
			s.Blueprint = &scope.ClusterBlueprint{
				ClusterClass: &clusterv1.ClusterClass{},
			}
			if tt.class.InfrastructureMachineTemplate != nil {
				s.Blueprint.ClusterClass.Spec.ControlPlane.MachineInfrastructure = &clusterv1.LocalObjectTemplate{
					Ref: contract.ObjToRef(tt.class.InfrastructureMachineTemplate),
				}
			}

			s.Current.ControlPlane = &scope.ControlPlaneState{}
			if tt.current != nil {
				s.Current.ControlPlane = tt.current
				if tt.current.Object != nil {
					fakeObjs = append(fakeObjs, tt.current.Object)
				}
				if tt.current.InfrastructureMachineTemplate != nil {
					fakeObjs = append(fakeObjs, tt.current.InfrastructureMachineTemplate)
				}
				if tt.current.MachineHealthCheck != nil {
					fakeObjs = append(fakeObjs, tt.current.MachineHealthCheck)
				}
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			r := Reconciler{
				Client:   fakeClient,
				recorder: env.GetEventRecorderFor("test"),
			}

			s.Desired = &scope.ClusterState{
				ControlPlane: tt.desired,
			}

			// Run reconcileControlPlane with the states created in the initial section of the test.
			err := r.reconcileControlPlane(ctx, s)
			g.Expect(err).ToNot(HaveOccurred())

			// Create MachineHealthCheck object for fetching data into
			gotMHC := &clusterv1.MachineHealthCheck{}
			err = fakeClient.Get(ctx, client.ObjectKey{Namespace: controlPlane1.GetNamespace(), Name: controlPlane1.GetName()}, gotMHC)

			// Nil case: If we want to find nothing (i.e. delete or MHC not created) and the Get call returns a NotFound error from the API the test succeeds.
			if tt.want == nil && apierrors.IsNotFound(err) {
				return
			}

			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotMHC).To(EqualObject(tt.want, IgnoreAutogeneratedMetadata))
		})
	}
}

func TestReconcileMachineDeployments(t *testing.T) {
	g := NewWithT(t)

	infrastructureMachineTemplate1 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-1").Build()
	bootstrapTemplate1 := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-1").Build()
	md1 := newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate1, bootstrapTemplate1, nil)

	infrastructureMachineTemplate2 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-2").Build()
	bootstrapTemplate2 := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-2").Build()
	md2 := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2, bootstrapTemplate2, nil)
	infrastructureMachineTemplate2WithChanges := infrastructureMachineTemplate2.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplate2WithChanges.Object, "foo", "spec", "template", "spec")).To(Succeed())
	md2WithRotatedInfrastructureMachineTemplate := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2WithChanges, bootstrapTemplate2, nil)

	infrastructureMachineTemplate3 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-3").Build()
	bootstrapTemplate3 := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-3").Build()
	md3 := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3, nil)
	bootstrapTemplate3WithChanges := bootstrapTemplate3.DeepCopy()
	g.Expect(unstructured.SetNestedField(bootstrapTemplate3WithChanges.Object, "foo", "spec", "template", "spec")).To(Succeed())
	md3WithRotatedBootstrapTemplate := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges, nil)
	bootstrapTemplate3WithChangeKind := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate3WithChangeKind.SetKind("AnotherGenericBootstrapTemplate")
	md3WithRotatedBootstrapTemplateChangedKind := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges, nil)

	infrastructureMachineTemplate4 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-4").Build()
	bootstrapTemplate4 := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-4").Build()
	md4 := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4, bootstrapTemplate4, nil)
	infrastructureMachineTemplate4WithChanges := infrastructureMachineTemplate4.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplate4WithChanges.Object, "foo", "spec", "template", "spec")).To(Succeed())
	bootstrapTemplate4WithChanges := bootstrapTemplate4.DeepCopy()
	g.Expect(unstructured.SetNestedField(bootstrapTemplate4WithChanges.Object, "foo", "spec", "template", "spec")).To(Succeed())
	md4WithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4WithChanges, bootstrapTemplate4WithChanges, nil)

	infrastructureMachineTemplate4m := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-4m").Build()
	bootstrapTemplate4m := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-4m").Build()
	md4m := newFakeMachineDeploymentTopologyState("md-4m", infrastructureMachineTemplate4m, bootstrapTemplate4m, nil)
	infrastructureMachineTemplate4mWithChanges := infrastructureMachineTemplate4m.DeepCopy()
	infrastructureMachineTemplate4mWithChanges.SetLabels(map[string]string{"foo": "bar"})
	bootstrapTemplate4mWithChanges := bootstrapTemplate4m.DeepCopy()
	bootstrapTemplate4mWithChanges.SetLabels(map[string]string{"foo": "bar"})
	md4mWithInPlaceUpdatedTemplates := newFakeMachineDeploymentTopologyState("md-4m", infrastructureMachineTemplate4mWithChanges, bootstrapTemplate4mWithChanges, nil)

	infrastructureMachineTemplate5 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-5").Build()
	bootstrapTemplate5 := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-5").Build()
	md5 := newFakeMachineDeploymentTopologyState("md-5", infrastructureMachineTemplate5, bootstrapTemplate5, nil)
	infrastructureMachineTemplate5WithChangedKind := infrastructureMachineTemplate5.DeepCopy()
	infrastructureMachineTemplate5WithChangedKind.SetKind("ChangedKind")
	md5WithChangedInfrastructureMachineTemplateKind := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate5WithChangedKind, bootstrapTemplate5, nil)

	infrastructureMachineTemplate6 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-6").Build()
	bootstrapTemplate6 := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-6").Build()
	md6 := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6, nil)
	bootstrapTemplate6WithChangedNamespace := bootstrapTemplate6.DeepCopy()
	bootstrapTemplate6WithChangedNamespace.SetNamespace("ChangedNamespace")
	md6WithChangedBootstrapTemplateNamespace := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6WithChangedNamespace, nil)

	infrastructureMachineTemplate7 := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-7").Build()
	bootstrapTemplate7 := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-7").Build()
	md7 := newFakeMachineDeploymentTopologyState("md-7", infrastructureMachineTemplate7, bootstrapTemplate7, nil)

	infrastructureMachineTemplate8Create := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-create").Build()
	bootstrapTemplate8Create := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-create").Build()
	md8Create := newFakeMachineDeploymentTopologyState("md-8-create", infrastructureMachineTemplate8Create, bootstrapTemplate8Create, nil)
	infrastructureMachineTemplate8Delete := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-delete").Build()
	bootstrapTemplate8Delete := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-delete").Build()
	md8Delete := newFakeMachineDeploymentTopologyState("md-8-delete", infrastructureMachineTemplate8Delete, bootstrapTemplate8Delete, nil)
	infrastructureMachineTemplate8Update := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-update").Build()
	bootstrapTemplate8Update := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-update").Build()
	md8Update := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8Update, bootstrapTemplate8Update, nil)
	infrastructureMachineTemplate8UpdateWithChanges := infrastructureMachineTemplate8Update.DeepCopy()
	g.Expect(unstructured.SetNestedField(infrastructureMachineTemplate8UpdateWithChanges.Object, "foo", "spec", "template", "spec")).To(Succeed())
	bootstrapTemplate8UpdateWithChanges := bootstrapTemplate3.DeepCopy()
	g.Expect(unstructured.SetNestedField(bootstrapTemplate8UpdateWithChanges.Object, "foo", "spec", "template", "spec")).To(Succeed())
	md8UpdateWithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8UpdateWithChanges, bootstrapTemplate8UpdateWithChanges, nil)

	infrastructureMachineTemplate9m := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-9m").Build()
	bootstrapTemplate9m := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-9m").Build()
	md9 := newFakeMachineDeploymentTopologyState("md-9m", infrastructureMachineTemplate9m, bootstrapTemplate9m, nil)
	md9.Object.Spec.Template.ObjectMeta.Labels = map[string]string{clusterv1.ClusterLabelName: "cluster-name"}
	md9.Object.Spec.Selector.MatchLabels = map[string]string{clusterv1.ClusterLabelName: "cluster-name"}
	md9WithInstanceSpecificTemplateMetadataAndSelector := newFakeMachineDeploymentTopologyState("md-9m", infrastructureMachineTemplate9m, bootstrapTemplate9m, nil)
	md9WithInstanceSpecificTemplateMetadataAndSelector.Object.Spec.Template.ObjectMeta.Labels = map[string]string{"foo": "bar"}
	md9WithInstanceSpecificTemplateMetadataAndSelector.Object.Spec.Selector.MatchLabels = map[string]string{"foo": "bar"}

	tests := []struct {
		name                                      string
		current                                   []*scope.MachineDeploymentState
		desired                                   []*scope.MachineDeploymentState
		want                                      []*scope.MachineDeploymentState
		wantInfrastructureMachineTemplateRotation map[string]bool
		wantBootstrapTemplateRotation             map[string]bool
		wantErr                                   bool
	}{
		{
			name:    "Should create desired MachineDeployment if the current does not exists yet",
			current: nil,
			desired: []*scope.MachineDeploymentState{md1},
			want:    []*scope.MachineDeploymentState{md1},
			wantErr: false,
		},
		{
			name:    "No-op if current MachineDeployment is equal to desired",
			current: []*scope.MachineDeploymentState{md1},
			desired: []*scope.MachineDeploymentState{md1},
			want:    []*scope.MachineDeploymentState{md1},
			wantErr: false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate rotation",
			current: []*scope.MachineDeploymentState{md2},
			desired: []*scope.MachineDeploymentState{md2WithRotatedInfrastructureMachineTemplate},
			want:    []*scope.MachineDeploymentState{md2WithRotatedInfrastructureMachineTemplate},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-2": true},
			wantErr: false,
		},
		{
			name:                          "Should update MachineDeployment with BootstrapTemplate rotation",
			current:                       []*scope.MachineDeploymentState{md3},
			desired:                       []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplate},
			want:                          []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplate},
			wantBootstrapTemplateRotation: map[string]bool{"md-3": true},
			wantErr:                       false,
		},
		{
			name:                          "Should update MachineDeployment with BootstrapTemplate rotation with changed kind",
			current:                       []*scope.MachineDeploymentState{md3},
			desired:                       []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplateChangedKind},
			want:                          []*scope.MachineDeploymentState{md3WithRotatedBootstrapTemplateChangedKind},
			wantBootstrapTemplateRotation: map[string]bool{"md-3": true},
			wantErr:                       false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate and BootstrapTemplate rotation",
			current: []*scope.MachineDeploymentState{md4},
			desired: []*scope.MachineDeploymentState{md4WithRotatedTemplates},
			want:    []*scope.MachineDeploymentState{md4WithRotatedTemplates},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-4": true},
			wantBootstrapTemplateRotation:             map[string]bool{"md-4": true},
			wantErr:                                   false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate and BootstrapTemplate without rotation",
			current: []*scope.MachineDeploymentState{md4m},
			desired: []*scope.MachineDeploymentState{md4mWithInPlaceUpdatedTemplates},
			want:    []*scope.MachineDeploymentState{md4mWithInPlaceUpdatedTemplates},
			wantErr: false,
		},
		{
			name:    "Should fail update MachineDeployment because of changed InfrastructureMachineTemplate kind",
			current: []*scope.MachineDeploymentState{md5},
			desired: []*scope.MachineDeploymentState{md5WithChangedInfrastructureMachineTemplateKind},
			wantErr: true,
		},
		{
			name:    "Should fail update MachineDeployment because of changed BootstrapTemplate namespace",
			current: []*scope.MachineDeploymentState{md6},
			desired: []*scope.MachineDeploymentState{md6WithChangedBootstrapTemplateNamespace},
			wantErr: true,
		},
		{
			name:    "Should delete MachineDeployment",
			current: []*scope.MachineDeploymentState{md7},
			desired: []*scope.MachineDeploymentState{},
			want:    []*scope.MachineDeploymentState{},
			wantErr: false,
		},
		{
			name:    "Should create, update and delete MachineDeployments",
			current: []*scope.MachineDeploymentState{md8Update, md8Delete},
			desired: []*scope.MachineDeploymentState{md8Create, md8UpdateWithRotatedTemplates},
			want:    []*scope.MachineDeploymentState{md8Create, md8UpdateWithRotatedTemplates},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-8-update": true},
			wantBootstrapTemplateRotation:             map[string]bool{"md-8-update": true},
			wantErr:                                   false,
		},
		{
			name:    "Enforce template metadata",
			current: []*scope.MachineDeploymentState{md9WithInstanceSpecificTemplateMetadataAndSelector},
			desired: []*scope.MachineDeploymentState{md9},
			want:    []*scope.MachineDeploymentState{md9},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeObjs := make([]client.Object, 0)
			for _, mdts := range tt.current {
				fakeObjs = append(fakeObjs, mdts.Object)
				fakeObjs = append(fakeObjs, mdts.InfrastructureMachineTemplate)
				fakeObjs = append(fakeObjs, mdts.BootstrapTemplate)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			currentMachineDeploymentStates := toMachineDeploymentTopologyStateMap(tt.current)
			s := scope.New(builder.Cluster(metav1.NamespaceDefault, "cluster-1").Build())
			s.Current.MachineDeployments = currentMachineDeploymentStates

			s.Desired = &scope.ClusterState{MachineDeployments: toMachineDeploymentTopologyStateMap(tt.desired)}

			r := Reconciler{
				Client:   fakeClient,
				recorder: env.GetEventRecorderFor("test"),
			}
			err := r.reconcileMachineDeployments(ctx, s)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			var gotMachineDeploymentList clusterv1.MachineDeploymentList
			g.Expect(fakeClient.List(ctx, &gotMachineDeploymentList)).To(Succeed())
			g.Expect(gotMachineDeploymentList.Items).To(HaveLen(len(tt.want)))

			for _, wantMachineDeploymentState := range tt.want {
				for _, gotMachineDeployment := range gotMachineDeploymentList.Items {
					if wantMachineDeploymentState.Object.Name != gotMachineDeployment.Name {
						continue
					}
					currentMachineDeploymentTopologyName := wantMachineDeploymentState.Object.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
					currentMachineDeploymentState := currentMachineDeploymentStates[currentMachineDeploymentTopologyName]

					// Compare MachineDeployment.
					// Note: We're intentionally only comparing Spec as otherwise we would have to account for
					// empty vs. filled out TypeMeta.
					g.Expect(gotMachineDeployment.Spec).To(Equal(wantMachineDeploymentState.Object.Spec))

					// Compare BootstrapTemplate.
					gotBootstrapTemplateRef := gotMachineDeployment.Spec.Template.Spec.Bootstrap.ConfigRef
					gotBootstrapTemplate := unstructured.Unstructured{}
					gotBootstrapTemplate.SetKind(gotBootstrapTemplateRef.Kind)
					gotBootstrapTemplate.SetAPIVersion(gotBootstrapTemplateRef.APIVersion)

					err = fakeClient.Get(ctx, client.ObjectKey{
						Namespace: gotBootstrapTemplateRef.Namespace,
						Name:      gotBootstrapTemplateRef.Name,
					}, &gotBootstrapTemplate)

					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(&gotBootstrapTemplate).To(EqualObject(wantMachineDeploymentState.BootstrapTemplate, IgnoreAutogeneratedMetadata, IgnoreTopologyManagedFieldAnnotation))

					// Check BootstrapTemplate rotation if there was a previous MachineDeployment/Template.
					if currentMachineDeploymentState != nil && currentMachineDeploymentState.BootstrapTemplate != nil {
						if tt.wantBootstrapTemplateRotation[gotMachineDeployment.Name] {
							g.Expect(currentMachineDeploymentState.BootstrapTemplate.GetName()).ToNot(Equal(gotBootstrapTemplate.GetName()))
						} else {
							g.Expect(currentMachineDeploymentState.BootstrapTemplate.GetName()).To(Equal(gotBootstrapTemplate.GetName()))
						}
					}

					// Compare InfrastructureMachineTemplate.
					gotInfrastructureMachineTemplateRef := gotMachineDeployment.Spec.Template.Spec.InfrastructureRef
					gotInfrastructureMachineTemplate := unstructured.Unstructured{}
					gotInfrastructureMachineTemplate.SetKind(gotInfrastructureMachineTemplateRef.Kind)
					gotInfrastructureMachineTemplate.SetAPIVersion(gotInfrastructureMachineTemplateRef.APIVersion)

					err = fakeClient.Get(ctx, client.ObjectKey{
						Namespace: gotInfrastructureMachineTemplateRef.Namespace,
						Name:      gotInfrastructureMachineTemplateRef.Name,
					}, &gotInfrastructureMachineTemplate)

					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(&gotInfrastructureMachineTemplate).To(EqualObject(wantMachineDeploymentState.InfrastructureMachineTemplate, IgnoreAutogeneratedMetadata, IgnoreTopologyManagedFieldAnnotation))

					// Check InfrastructureMachineTemplate rotation if there was a previous MachineDeployment/Template.
					if currentMachineDeploymentState != nil && currentMachineDeploymentState.InfrastructureMachineTemplate != nil {
						if tt.wantInfrastructureMachineTemplateRotation[gotMachineDeployment.Name] {
							g.Expect(currentMachineDeploymentState.InfrastructureMachineTemplate.GetName()).ToNot(Equal(gotInfrastructureMachineTemplate.GetName()))
						} else {
							g.Expect(currentMachineDeploymentState.InfrastructureMachineTemplate.GetName()).To(Equal(gotInfrastructureMachineTemplate.GetName()))
						}
					}
				}
			}
		})
	}
}

// TestReconcileReferencedObjectSequences tests multiple subsequent calls to reconcileReferencedObject
// for a control-plane object to verify that the objects are reconciled as expected by tracking managed fields correctly.
// NOTE: by Extension this tests validates managed field handling in mergePatches, and thus its usage in other parts of the
// codebase.
func TestReconcileReferencedObjectSequences(t *testing.T) {
	type object struct {
		spec map[string]interface{}
	}

	type externalStep struct {
		name string
		// object is the state of the control-plane object after an external modification.
		object object
	}
	type reconcileStep struct {
		name string
		// desired is the desired control-plane object handed over to reconcileReferencedObject.
		desired object
		// want is the expected control-plane object after calling reconcileReferencedObject.
		want object
	}

	tests := []struct {
		name           string
		reconcileSteps []interface{}
	}{
		{
			name: "Should drop nested field",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// and most specifically it verifies that when a field in a template is deleted, it gets deleted
			// from the generated object (and it is not being treated as instance specific value).
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile KCP",
					desired: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "Drop enable-hostpath-provisioner",
					desired: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									// enable-hostpath-provisioner has been removed by e.g a change in ClusterClass (and extraArgs with it).
									"controllerManager": map[string]interface{}{},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										// Reconcile to drop enable-hostpath-provisioner, extraArgs has been set to an empty object.
										"extraArgs": map[string]interface{}{},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Should drop label",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// and most specifically it verifies that when a template label is deleted, it gets deleted
			// from the generated object (and it is not being treated as instance specific value).
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile KCP",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"label.with.dots/owned": "true",
										"anotherLabel":          "true",
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"label.with.dots/owned": "true",
										"anotherLabel":          "true",
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "Drop the label with dots",
					desired: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										// label.with.dots/owned has been removed by e.g a change in Cluster.Topology.ControlPlane.Labels.
										"anotherLabel": "true",
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"machineTemplate": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										// Reconcile to drop label.with.dots/owned label.
										"anotherLabel": "true",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Should drop label in metadata (even if outside of .spec.machineTemplate.metadata)",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// and most specifically it verifies that when a spec.template label is deleted, it gets deleted
			// from the generated object (and it is not being treated as instance specific value).
			// E.g. AzureMachineTemplate has .spec.template.metadata.labels.
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile",
					desired: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"label.with.dots/owned": "true",
										"anotherLabel":          "true",
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										"label.with.dots/owned": "true",
										"anotherLabel":          "true",
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "Drop the label with dots",
					desired: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										// label.with.dots/owned has been removed e.g a change in ClusterClass.
										"anotherLabel": "true",
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"metadata": map[string]interface{}{
									"labels": map[string]interface{}{
										// Reconcile to drop label.with.dots/owned label.
										"anotherLabel": "true",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Should enforce field",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// by reverting user changes to a manged field.
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile",
					desired: object{
						spec: map[string]interface{}{
							"stringField": "ccValue",
						},
					},
					want: object{
						spec: map[string]interface{}{
							"stringField": "ccValue",
						},
					},
				},
				externalStep{
					name: "User changes value",
					object: object{
						spec: map[string]interface{}{
							"stringField": "userValue",
						},
					},
				},
				reconcileStep{
					name: "Reconcile overwrites value",
					desired: object{
						spec: map[string]interface{}{
							// ClusterClass still proposing the old value.
							"stringField": "ccValue",
						},
					},
					want: object{
						spec: map[string]interface{}{
							// Reconcile to restore the old value.
							"stringField": "ccValue",
						},
					},
				},
			},
		},
		{
			name: "Should preserve user-defined field while dropping managed field",
			// Note: This test verifies that Topology treats changes to fields existing in templates as authoritative
			// but allows setting additional/instance-specific values.
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile KCP",
					desired: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
										},
									},
								},
							},
						},
					},
				},
				externalStep{
					name: "User adds an additional extraArg",
					object: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											"enable-hostpath-provisioner": "true",
											// User adds enable-garbage-collector.
											"enable-garbage-collector": "true",
										},
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "Previously set extraArgs is dropped from KCP, user-specified field is preserved.",
					desired: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									// enable-hostpath-provisioner has been removed by e.g a change in ClusterClass (and extraArgs with it).
									"controllerManager": map[string]interface{}{},
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"kubeadmConfigSpec": map[string]interface{}{
								"clusterConfiguration": map[string]interface{}{
									"controllerManager": map[string]interface{}{
										"extraArgs": map[string]interface{}{
											// Reconcile to drop enable-hostpath-provisioner,
											// while preserving user-defined enable-garbage-collector field.
											"enable-garbage-collector": "true",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Should preserve user-defined object field while dropping managed fields",
			// Note: This test verifies that reconcileReferencedObject treats changes to fields existing in templates as authoritative
			// but allows setting additional/instance-specific values.
			reconcileSteps: []interface{}{
				reconcileStep{
					name: "Initially reconcile",
					desired: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{},
							},
						},
					},
				},
				externalStep{
					name: "User adds an additional object",
					object: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"userDefinedObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "ClusterClass starts having an opinion about some fields",
					desired: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"clusterClassObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
									"clusterClassField": true,
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									// User fields are preserved.
									"userDefinedObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
									// ClusterClass authoritative fields are added.
									"clusterClassObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
									"clusterClassField": true,
								},
							},
						},
					},
				},
				reconcileStep{
					name: "ClusterClass stops having an opinion on the field",
					desired: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									"clusterClassObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
									// clusterClassField has been removed by e.g a change in ClusterClass (and extraArgs with it).
								},
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									// Reconcile to drop clusterClassField,
									// while preserving user-defined field and clusterClassField.
									"userDefinedObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
									"clusterClassObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
								},
							},
						},
					},
				},
				reconcileStep{
					name: "ClusterClass stops having an opinion on the object",
					desired: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{}, // clusterClassObject has been removed by e.g a change in ClusterClass (and extraArgs with it).
							},
						},
					},
					want: object{
						spec: map[string]interface{}{
							"template": map[string]interface{}{
								"spec": map[string]interface{}{
									// Reconcile to drop clusterClassObject,
									// while preserving user-defined field.
									"userDefinedObject": map[string]interface{}{
										"boolField":   true,
										"stringField": "def",
									},
									"clusterClassObject": map[string]interface{}{},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).Build()
			r := Reconciler{
				Client: fakeClient,
				// NOTE: Intentionally using a fake recorder, so the test can also be run without testenv.
				recorder: record.NewFakeRecorder(32),
			}

			s := scope.New(&clusterv1.Cluster{})
			s.Blueprint = &scope.ClusterBlueprint{
				ClusterClass: &clusterv1.ClusterClass{},
			}

			for i, step := range tt.reconcileSteps {
				var currentControlPlane *unstructured.Unstructured

				// Get current ControlPlane (on later steps).
				if i > 0 {
					currentControlPlane = &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "KubeadmControlPlane",
							"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
						},
					}
					g.Expect(fakeClient.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "my-cluster"}, currentControlPlane)).To(Succeed())
				}

				if step, ok := step.(externalStep); ok {
					// This is a user step, so let's just update the object.
					patchHelper, err := patch.NewHelper(currentControlPlane, fakeClient)
					g.Expect(err).ToNot(HaveOccurred())

					g.Expect(unstructured.SetNestedField(currentControlPlane.Object, step.object.spec, "spec")).To(Succeed())
					g.Expect(patchHelper.Patch(context.Background(), currentControlPlane)).To(Succeed())
					continue
				}

				if step, ok := step.(reconcileStep); ok {
					// This is a reconcile step, so let's execute a reconcile and then validate the result.

					// Set the current control plane.
					s.Current.ControlPlane = &scope.ControlPlaneState{
						Object: currentControlPlane,
					}
					// Set the desired control plane.
					s.Desired = &scope.ClusterState{
						ControlPlane: &scope.ControlPlaneState{
							Object: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"kind":       "KubeadmControlPlane",
									"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
									"metadata": map[string]interface{}{
										"name":      "my-cluster",
										"namespace": metav1.NamespaceDefault,
									},
									"spec": step.desired.spec,
								},
							},
						},
					}
					if currentControlPlane != nil {
						// Set the annotation of the current control plane.
						annotations, found, err := unstructured.NestedFieldCopy(currentControlPlane.Object, "metadata", "annotations")
						g.Expect(err).ToNot(HaveOccurred())
						g.Expect(found).To(BeTrue())
						g.Expect(unstructured.SetNestedField(s.Desired.ControlPlane.Object.Object, annotations, "metadata", "annotations")).To(Succeed())
					}

					// Execute a reconcile.0
					g.Expect(r.reconcileReferencedObject(ctx, reconcileReferencedObjectInput{
						cluster: s.Current.Cluster,
						current: s.Current.ControlPlane.Object,
						desired: s.Desired.ControlPlane.Object,
						opts: []mergepatch.HelperOption{
							mergepatch.AuthoritativePaths{
								// Note: Just using .spec.machineTemplate.metadata here as an example.
								contract.ControlPlane().MachineTemplate().Metadata().Path(),
							},
						}})).To(Succeed())

					// Build the object for comparison.
					want := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"kind":       "KubeadmControlPlane",
							"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
							"metadata": map[string]interface{}{
								"name":      "my-cluster",
								"namespace": metav1.NamespaceDefault,
							},
							"spec": step.want.spec,
						},
					}

					// Get the reconciled object.
					got := want.DeepCopy() // this is required otherwise Get will modify want
					g.Expect(fakeClient.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "my-cluster"}, got)).To(Succeed())

					// Compare want with got.
					// Ignore .metadata.resourceVersion and .metadata.annotations as we don't care about them in this test.
					unstructured.RemoveNestedField(got.Object, "metadata", "resourceVersion")
					unstructured.RemoveNestedField(got.Object, "metadata", "annotations")
					g.Expect(got).To(EqualObject(want), fmt.Sprintf("Step %q failed: %v", step.name, cmp.Diff(want, got)))
					continue
				}

				panic(fmt.Errorf("unknown step type %T", step))
			}
		})
	}
}

func TestReconcileMachineDeploymentMachineHealthCheck(t *testing.T) {
	md := builder.MachineDeployment(metav1.NamespaceDefault, "md-1").WithLabels(
		map[string]string{
			clusterv1.ClusterTopologyMachineDeploymentLabelName: "machine-deployment-one",
		}).
		Build()

	maxUnhealthy := intstr.Parse("45%")
	mhcBuilder := builder.MachineHealthCheck(metav1.NamespaceDefault, "md-1").
		WithSelector(*selectorForMachineDeploymentMHC(md)).
		WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		}).
		WithOwnerReferences([]metav1.OwnerReference{*ownerReferenceTo(md)}).
		WithClusterName("cluster1")

	infrastructureMachineTemplate := builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-1").Build()
	bootstrapTemplate := builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-1").Build()

	tests := []struct {
		name    string
		current []*scope.MachineDeploymentState
		desired []*scope.MachineDeploymentState
		want    []*clusterv1.MachineHealthCheck
	}{
		{
			name:    "Create a MachineHealthCheck if the MachineDeployment is being created",
			current: nil,
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().Build()},
		},
		{
			name: "Create a new MachineHealthCheck if the MachineDeployment is modified to include one",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					nil)},
			// MHC is added in the desired state of the MachineDeployment
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().Build()}},
		{
			name: "Update MachineHealthCheck spec adding a field if the spec adds a field",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().WithMaxUnhealthy(&maxUnhealthy).Build())},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().
					WithMaxUnhealthy(&maxUnhealthy).
					Build()},
		},
		{
			name: "Update MachineHealthCheck spec removing a field if the spec removes a field",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().WithMaxUnhealthy(&maxUnhealthy).Build()),
			},
			desired: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			want: []*clusterv1.MachineHealthCheck{
				mhcBuilder.DeepCopy().Build(),
			},
		},
		{
			name: "Delete MachineHealthCheck spec if the MachineDeployment is modified to remove an existing one",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			desired: []*scope.MachineDeploymentState{newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate, nil)},
			want:    []*clusterv1.MachineHealthCheck{},
		},
		{
			name: "Delete MachineHealthCheck spec if the MachineDeployment is deleted",
			current: []*scope.MachineDeploymentState{
				newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate, bootstrapTemplate,
					mhcBuilder.DeepCopy().Build()),
			},
			desired: []*scope.MachineDeploymentState{},
			want:    []*clusterv1.MachineHealthCheck{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeObjs := make([]client.Object, 0)
			for _, mdts := range tt.current {
				fakeObjs = append(fakeObjs, mdts.Object.DeepCopy())
				fakeObjs = append(fakeObjs, mdts.InfrastructureMachineTemplate.DeepCopy())
				fakeObjs = append(fakeObjs, mdts.BootstrapTemplate.DeepCopy())
				if mdts.MachineHealthCheck != nil {
					fakeObjs = append(fakeObjs, mdts.MachineHealthCheck.DeepCopy())
				}
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			currentMachineDeploymentStates := toMachineDeploymentTopologyStateMap(tt.current)
			s := scope.New(builder.Cluster(metav1.NamespaceDefault, "cluster-1").Build())
			s.Current.MachineDeployments = currentMachineDeploymentStates

			s.Desired = &scope.ClusterState{MachineDeployments: toMachineDeploymentTopologyStateMap(tt.desired)}

			r := Reconciler{
				Client:   fakeClient,
				recorder: env.GetEventRecorderFor("test"),
			}

			err := r.reconcileMachineDeployments(ctx, s)
			g.Expect(err).ToNot(HaveOccurred())

			var gotMachineHealthCheckList clusterv1.MachineHealthCheckList
			g.Expect(fakeClient.List(ctx, &gotMachineHealthCheckList)).To(Succeed())
			g.Expect(gotMachineHealthCheckList.Items).To(HaveLen(len(tt.want)))

			g.Expect(len(tt.want)).To(Equal(len(gotMachineHealthCheckList.Items)))

			for _, wantMHC := range tt.want {
				for _, gotMHC := range gotMachineHealthCheckList.Items {
					if wantMHC.Name == gotMHC.Name {
						actual := gotMHC
						g.Expect(wantMHC).To(EqualObject(&actual, IgnoreAutogeneratedMetadata))
					}
				}
			}
		})
	}
}

func newFakeMachineDeploymentTopologyState(name string, infrastructureMachineTemplate, bootstrapTemplate *unstructured.Unstructured, machineHealthCheck *clusterv1.MachineHealthCheck) *scope.MachineDeploymentState {
	return &scope.MachineDeploymentState{
		Object: builder.MachineDeployment(metav1.NamespaceDefault, name).
			WithInfrastructureTemplate(infrastructureMachineTemplate).
			WithBootstrapTemplate(bootstrapTemplate).
			WithLabels(map[string]string{clusterv1.ClusterTopologyMachineDeploymentLabelName: name + "-topology"}).
			Build(),
		InfrastructureMachineTemplate: infrastructureMachineTemplate,
		BootstrapTemplate:             bootstrapTemplate,
		MachineHealthCheck:            machineHealthCheck,
	}
}

func toMachineDeploymentTopologyStateMap(states []*scope.MachineDeploymentState) map[string]*scope.MachineDeploymentState {
	ret := map[string]*scope.MachineDeploymentState{}
	for _, state := range states {
		ret[state.Object.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]] = state
	}
	return ret
}

func TestReconciler_reconcileMachineHealthCheck(t *testing.T) {
	// create a controlPlane object with enough information to be used as an OwnerReference for the MachineHealthCheck.
	cp := builder.ControlPlane(metav1.NamespaceDefault, "cp1").Build()
	cp.SetUID("very-unique-identifier")
	mhcBuilder := builder.MachineHealthCheck(metav1.NamespaceDefault, "cp1").
		WithSelector(*selectorForControlPlaneMHC()).
		WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
			{
				Type:    corev1.NodeReady,
				Status:  corev1.ConditionUnknown,
				Timeout: metav1.Duration{Duration: 5 * time.Minute},
			},
		}).
		WithClusterName("cluster1")
	tests := []struct {
		name    string
		current *clusterv1.MachineHealthCheck
		desired *clusterv1.MachineHealthCheck
		want    *clusterv1.MachineHealthCheck
		wantErr bool
	}{
		{
			name:    "Create a MachineHealthCheck",
			current: nil,
			desired: mhcBuilder.DeepCopy().Build(),
			want:    mhcBuilder.DeepCopy().Build(),
		},
		{
			name:    "Successfully create a valid Ownerreference on the MachineHealthCheck",
			current: nil,
			// update the unhealthy conditions in the MachineHealthCheck
			desired: mhcBuilder.DeepCopy().
				// Desired object has an incomplete owner reference which has no UID.
				WithOwnerReferences([]metav1.OwnerReference{{Name: cp.GetName(), Kind: cp.GetKind(), APIVersion: cp.GetAPIVersion()}}).
				Build(),
			// Want a reconciled object with a full ownerReference including UID
			want: mhcBuilder.DeepCopy().
				WithOwnerReferences([]metav1.OwnerReference{{Name: cp.GetName(), Kind: cp.GetKind(), APIVersion: cp.GetAPIVersion(), UID: cp.GetUID()}}).
				Build(),
			wantErr: false,
		},

		{
			name:    "Update a MachineHealthCheck with changes",
			current: mhcBuilder.DeepCopy().Build(),
			// update the unhealthy conditions in the MachineHealthCheck
			desired: mhcBuilder.DeepCopy().WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 1000 * time.Minute},
				},
			}).Build(),
			want: mhcBuilder.DeepCopy().WithUnhealthyConditions([]clusterv1.UnhealthyCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionUnknown,
					Timeout: metav1.Duration{Duration: 1000 * time.Minute},
				},
			}).Build(),
		},
		{
			name:    "Don't change a MachineHealthCheck with no difference between desired and current",
			current: mhcBuilder.DeepCopy().Build(),
			// update the unhealthy conditions in the MachineHealthCheck
			desired: mhcBuilder.DeepCopy().Build(),
			want:    mhcBuilder.DeepCopy().Build(),
		},
		{
			name:    "Delete a MachineHealthCheck",
			current: mhcBuilder.DeepCopy().Build(),
			// update the unhealthy conditions in the MachineHealthCheck
			desired: nil,
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := &clusterv1.MachineHealthCheck{}
			r := Reconciler{
				Client: fake.NewClientBuilder().
					WithScheme(fakeScheme).
					WithObjects([]client.Object{cp}...).
					Build(),
				recorder: env.GetEventRecorderFor("test"),
			}
			if tt.current != nil {
				g.Expect(r.Client.Create(ctx, tt.current)).To(Succeed())
			}
			if err := r.reconcileMachineHealthCheck(ctx, tt.current, tt.desired); err != nil {
				if !tt.wantErr {
					t.Errorf("reconcileMachineHealthCheck() error = %v, wantErr %v", err, tt.wantErr)
				}
			}

			if err := r.Client.Get(ctx, client.ObjectKeyFromObject(mhcBuilder.Build()), got); err != nil {
				if !tt.wantErr {
					t.Errorf("reconcileMachineHealthCheck() error = %v, wantErr %v", err, tt.wantErr)
				}

				// Delete case: If we want to find nothing and the Get call returns a NotFound error from the API this is a deletion case and the test succeeds.
				if tt.want == nil && apierrors.IsNotFound(err) {
					return
				}
			}

			g.Expect(got).To(EqualObject(tt.want, IgnoreAutogeneratedMetadata))
		})
	}
}

func Test_createErrorWithoutObjectName(t *testing.T) {
	originalError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusUnprocessableEntity,
			Reason:  metav1.StatusReasonInvalid,
			Message: "DockerMachineTemplate.infrastructure.cluster.x-k8s.io \"docker-template-one\" is invalid: spec.template.spec.preLoadImages: Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
			Details: &metav1.StatusDetails{
				Group: "infrastructure.cluster.x-k8s.io",
				Kind:  "DockerMachineTemplate",
				Name:  "docker-template-one",
				Causes: []metav1.StatusCause{
					{
						Type:    "FieldValueInvalid",
						Message: "Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
						Field:   "spec.template.spec.preLoadImages",
					},
				},
			},
		}}
	wantError := &apierrors.StatusError{
		ErrStatus: metav1.Status{
			Status: metav1.StatusFailure,
			Code:   http.StatusUnprocessableEntity,
			Reason: metav1.StatusReasonInvalid,
			// The only difference between the two objects should be in the Message section.
			Message: "failed to create DockerMachineTemplate.infrastructure.cluster.x-k8s.io: FieldValueInvalid: spec.template.spec.preLoadImages: Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
			Details: &metav1.StatusDetails{
				Group: "infrastructure.cluster.x-k8s.io",
				Kind:  "DockerMachineTemplate",
				Name:  "docker-template-one",
				Causes: []metav1.StatusCause{
					{
						Type:    "FieldValueInvalid",
						Message: "Invalid value: \"array\": spec.template.spec.preLoadImages in body must be of type string: \"array\"",
						Field:   "spec.template.spec.preLoadImages",
					},
				},
			},
		},
	}
	t.Run("Transform a create error correctly", func(t *testing.T) {
		g := NewWithT(t)
		err := createErrorWithoutObjectName(originalError, nil)
		g.Expect(err).To(Equal(wantError), cmp.Diff(err, wantError))
	})
}
