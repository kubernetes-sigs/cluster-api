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
	"fmt"
	"regexp"
	"testing"

	"sigs.k8s.io/cluster-api/internal/testtypes"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/contract"
	"sigs.k8s.io/cluster-api/controllers/topology/internal/scope"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileCluster(t *testing.T) {
	cluster1 := testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
		Build()
	cluster1WithReferences := testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").
		WithInfrastructureCluster(testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infrastructure-cluster1").
			Build()).
		WithControlPlane(testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "control-plane1").Build()).
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

			// TODO: stop setting ResourceVersion when building objects
			tt.desired.SetResourceVersion("")
			s.Desired = &scope.ClusterState{Cluster: tt.desired}

			r := ClusterReconciler{
				Client: fakeClient,
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

			g.Expect(got.Spec.InfrastructureRef).To(Equal(tt.want.Spec.InfrastructureRef), cmp.Diff(got, tt.want))
			g.Expect(got.Spec.ControlPlaneRef).To(Equal(tt.want.Spec.ControlPlaneRef), cmp.Diff(got, tt.want))
		})
	}
}

func TestReconcileInfrastructureCluster(t *testing.T) {
	g := NewWithT(t)

	clusterInfrastructure1 := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infrastructure-cluster1").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure2 := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infrastructure-cluster2").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure3 := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infrastructure-cluster3").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure3WithInstanceSpecificChanges := clusterInfrastructure3.DeepCopy()
	clusterInfrastructure3WithInstanceSpecificChanges.SetLabels(map[string]string{"foo": "bar"})
	clusterInfrastructure4 := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infrastructure-cluster4").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	clusterInfrastructure4WithTemplateOverridingChanges := clusterInfrastructure4.DeepCopy()
	err := unstructured.SetNestedField(clusterInfrastructure4WithTemplateOverridingChanges.UnstructuredContent(), false, "spec", "fakeSetting")
	g.Expect(err).ToNot(HaveOccurred())
	clusterInfrastructure5 := testtypes.NewInfrastructureClusterBuilder(metav1.NamespaceDefault, "infrastructure-cluster5").
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

			s := scope.New(nil)
			s.Current.InfrastructureCluster = tt.current

			// TODO: stop setting ResourceVersion when building objects
			tt.desired.SetResourceVersion("")
			s.Desired = &scope.ClusterState{InfrastructureCluster: tt.desired}

			r := ClusterReconciler{
				Client: fakeClient,
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
	infrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infra1").Build()
	infrastructureMachineTemplate2 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infra2").Build()

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
	controlPlane1 := testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "cp1").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate).
		Build()
	controlPlane2 := testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "cp2").
		WithInfrastructureMachineTemplate(infrastructureMachineTemplate2).
		Build()
	// ControlPlane object with novel field in the spec.
	controlPlane3 := controlPlane1.DeepCopy()
	err = unstructured.SetNestedField(controlPlane3.UnstructuredContent(), true, "spec", "differentSetting")
	g.Expect(err).ToNot(HaveOccurred())
	// ControlPlane object with a new label.
	controlPlaneWithInstanceSpecificChanges := controlPlane1.DeepCopy()
	controlPlaneWithInstanceSpecificChanges.SetLabels(map[string]string{"foo": "bar"})
	// ControlPlane object with the same name as controlPlane1 but a different InfrastructureMachineTemplate

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
			desired: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			want:    &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			wantErr: false,
		},
		{
			name:    "Fail on updating ControlPlaneObject with incompatible changes, here a different Kind for the infrastructureMachineTemplate",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			desired: &scope.ControlPlaneState{Object: controlPlane2, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			wantErr: true,
		},
		{
			name:    "Update to ControlPlaneObject with no update to the underlying infrastructure",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			desired: &scope.ControlPlaneState{Object: controlPlane3, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			want:    &scope.ControlPlaneState{Object: controlPlane3, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			wantErr: false,
		},
		{
			name:    "Update to ControlPlaneObject with underlying infrastructure.",
			class:   ccWithControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{InfrastructureMachineTemplate: nil},
			desired: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			want:    &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate.DeepCopy()},
			wantErr: false,
		},
		{
			name:    "Update to ControlPlaneObject with no underlying infrastructure",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlane1},
			desired: &scope.ControlPlaneState{Object: controlPlane3},
			want:    &scope.ControlPlaneState{Object: controlPlane3},
			wantErr: false,
		},
		{
			name:    "Preserve specific changes to the ControlPlaneObject",
			class:   ccWithoutControlPlaneInfrastructure,
			current: &scope.ControlPlaneState{Object: controlPlaneWithInstanceSpecificChanges, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			desired: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			want:    &scope.ControlPlaneState{Object: controlPlaneWithInstanceSpecificChanges, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeObjs := make([]client.Object, 0)
			s := scope.New(testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").Build())
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

			// TODO: stop setting ResourceVersion when building objects
			if tt.desired.InfrastructureMachineTemplate != nil {
				tt.desired.InfrastructureMachineTemplate.SetResourceVersion("")
			}
			if tt.desired.Object != nil {
				tt.desired.Object.SetResourceVersion("")
			}
			r := ClusterReconciler{
				Client: fakeClient,
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
			gotControlPlaneObject := testtypes.NewControlPlaneBuilder("", "").Build()
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
	infrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infra1").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()
	infrastructureMachineTemplate2 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infra2").
		WithSpecFields(map[string]interface{}{"spec.template.spec.fakeSetting": true}).
		Build()

	// Create the blueprint mandating controlPlaneInfrastructure.
	blueprint := &scope.ClusterBlueprint{
		ClusterClass: testtypes.NewClusterClassBuilder(metav1.NamespaceDefault, "class1").
			WithControlPlaneInfrastructureMachineTemplate(infrastructureMachineTemplate).
			Build(),
		ControlPlane: &scope.ControlPlaneBlueprint{
			InfrastructureMachineTemplate: infrastructureMachineTemplate,
		},
	}

	// Create Cluster object for test cases.
	cluster := testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster1").Build()

	// Infrastructure object with a different Kind.
	incompatibleInfrastructureMachineTemplate := infrastructureMachineTemplate2.DeepCopy()
	incompatibleInfrastructureMachineTemplate.SetKind("incompatibleInfrastructureMachineTemplate")
	updatedInfrastructureMachineTemplate := infrastructureMachineTemplate.DeepCopy()
	err := unstructured.SetNestedField(updatedInfrastructureMachineTemplate.UnstructuredContent(), true, "spec", "differentSetting")
	g.Expect(err).ToNot(HaveOccurred())
	// Create ControlPlaneObjects for test cases.
	controlPlane1 := testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "cp1").
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
	controlPlane3 := testtypes.NewControlPlaneBuilder(metav1.NamespaceDefault, "cp1").
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
			current: &scope.ControlPlaneState{Object: controlPlane1},
			desired: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			want:    &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			wantErr: false,
		},
		{
			name:    "Update desired InfrastructureMachineTemplate connected to controlPlane",
			current: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			desired: &scope.ControlPlaneState{Object: controlPlane3, InfrastructureMachineTemplate: updatedInfrastructureMachineTemplate},
			want:    &scope.ControlPlaneState{Object: controlPlane3, InfrastructureMachineTemplate: updatedInfrastructureMachineTemplate},
			wantErr: false,
		},
		{
			name:    "Fail on updating infrastructure with incompatible changes",
			current: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: infrastructureMachineTemplate},
			desired: &scope.ControlPlaneState{Object: controlPlane1, InfrastructureMachineTemplate: incompatibleInfrastructureMachineTemplate},
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

			// TODO: stop setting ResourceVersion when building objects
			if tt.desired.InfrastructureMachineTemplate != nil {
				tt.desired.InfrastructureMachineTemplate.SetResourceVersion("")
			}
			if tt.desired.Object != nil {
				tt.desired.Object.SetResourceVersion("")
			}
			r := ClusterReconciler{
				Client: fakeClient,
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
			gotControlPlaneObject := testtypes.NewControlPlaneBuilder("", "").Build()
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
			gotInfrastructureMachineTemplate := testtypes.NewInfrastructureMachineTemplateBuilder("", "").Build()
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
func TestReconcileMachineDeployments(t *testing.T) {
	infrastructureMachineTemplate1 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-1").Build()
	bootstrapTemplate1 := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-1").Build()
	md1 := newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate1, bootstrapTemplate1)

	infrastructureMachineTemplate2 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-2").Build()
	bootstrapTemplate2 := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-2").Build()
	md2 := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2, bootstrapTemplate2)
	infrastructureMachineTemplate2WithChanges := infrastructureMachineTemplate2.DeepCopy()
	infrastructureMachineTemplate2WithChanges.SetLabels(map[string]string{"foo": "bar"})
	md2WithRotatedInfrastructureMachineTemplate := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2WithChanges, bootstrapTemplate2)

	infrastructureMachineTemplate3 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-3").Build()
	bootstrapTemplate3 := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-3").Build()
	md3 := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3)
	bootstrapTemplate3WithChanges := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate3WithChanges.SetLabels(map[string]string{"foo": "bar"})
	md3WithRotatedBootstrapTemplate := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges)
	bootstrapTemplate3WithChangeKind := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate3WithChangeKind.SetKind("AnotherGenericBootstrapTemplate")
	md3WithRotatedBootstrapTemplateChangedKind := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges)

	infrastructureMachineTemplate4 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-4").Build()
	bootstrapTemplate4 := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-4").Build()
	md4 := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4, bootstrapTemplate4)
	infrastructureMachineTemplate4WithChanges := infrastructureMachineTemplate4.DeepCopy()
	infrastructureMachineTemplate4WithChanges.SetLabels(map[string]string{"foo": "bar"})
	bootstrapTemplate4WithChanges := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate4WithChanges.SetLabels(map[string]string{"foo": "bar"})
	md4WithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4WithChanges, bootstrapTemplate4WithChanges)

	infrastructureMachineTemplate5 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-5").Build()
	bootstrapTemplate5 := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-5").Build()
	md5 := newFakeMachineDeploymentTopologyState("md-5", infrastructureMachineTemplate5, bootstrapTemplate5)
	infrastructureMachineTemplate5WithChangedKind := infrastructureMachineTemplate5.DeepCopy()
	infrastructureMachineTemplate5WithChangedKind.SetKind("ChangedKind")
	md5WithChangedInfrastructureMachineTemplateKind := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate5WithChangedKind, bootstrapTemplate5)

	infrastructureMachineTemplate6 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-6").Build()
	bootstrapTemplate6 := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-6").Build()
	md6 := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6)
	bootstrapTemplate6WithChangedNamespace := bootstrapTemplate6.DeepCopy()
	bootstrapTemplate6WithChangedNamespace.SetNamespace("ChangedNamespace")
	md6WithChangedBootstrapTemplateNamespace := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6WithChangedNamespace)

	infrastructureMachineTemplate7 := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-7").Build()
	bootstrapTemplate7 := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-7").Build()
	md7 := newFakeMachineDeploymentTopologyState("md-7", infrastructureMachineTemplate7, bootstrapTemplate7)

	infrastructureMachineTemplate8Create := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-8-create").Build()
	bootstrapTemplate8Create := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-8-create").Build()
	md8Create := newFakeMachineDeploymentTopologyState("md-8-create", infrastructureMachineTemplate8Create, bootstrapTemplate8Create)
	infrastructureMachineTemplate8Delete := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-8-delete").Build()
	bootstrapTemplate8Delete := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-8-delete").Build()
	md8Delete := newFakeMachineDeploymentTopologyState("md-8-delete", infrastructureMachineTemplate8Delete, bootstrapTemplate8Delete)
	infrastructureMachineTemplate8Update := testtypes.NewInfrastructureMachineTemplateBuilder(metav1.NamespaceDefault, "infrastructure-machine-8-update").Build()
	bootstrapTemplate8Update := testtypes.NewBootstrapTemplateBuilder(metav1.NamespaceDefault, "bootstrap-config-8-update").Build()
	md8Update := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8Update, bootstrapTemplate8Update)
	infrastructureMachineTemplate8UpdateWithChanges := infrastructureMachineTemplate8Update.DeepCopy()
	infrastructureMachineTemplate8UpdateWithChanges.SetLabels(map[string]string{"foo": "bar"})
	bootstrapTemplate8UpdateWithChanges := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate8UpdateWithChanges.SetLabels(map[string]string{"foo": "bar"})
	md8UpdateWithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8UpdateWithChanges, bootstrapTemplate8UpdateWithChanges)

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
			s := scope.New(testtypes.NewClusterBuilder(metav1.NamespaceDefault, "cluster-1").Build())
			s.Current.MachineDeployments = currentMachineDeploymentStates

			// TODO: stop setting ResourceVersion when building objects
			for _, md := range tt.desired {
				md.Object.SetResourceVersion("")
				md.BootstrapTemplate.SetResourceVersion("")
				md.InfrastructureMachineTemplate.SetResourceVersion("")
			}
			s.Desired = &scope.ClusterState{MachineDeployments: toMachineDeploymentTopologyStateMap(tt.desired)}

			r := ClusterReconciler{
				Client: fakeClient,
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
					// We don't want to compare resourceVersions as they are slightly different between the test cases
					// and it's not worth the effort.
					gotBootstrapTemplate.SetResourceVersion("")
					g.Expect(gotBootstrapTemplate).To(Equal(*wantMachineDeploymentState.BootstrapTemplate))

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
					// We don't want to compare resourceVersions as they are slightly different between the test cases
					// and it's not worth the effort.
					gotInfrastructureMachineTemplate.SetResourceVersion("")
					g.Expect(gotInfrastructureMachineTemplate).To(Equal(*wantMachineDeploymentState.InfrastructureMachineTemplate))

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

func newFakeMachineDeploymentTopologyState(name string, infrastructureMachineTemplate, bootstrapTemplate *unstructured.Unstructured) *scope.MachineDeploymentState {
	return &scope.MachineDeploymentState{
		Object: testtypes.NewMachineDeploymentBuilder(metav1.NamespaceDefault, name).
			WithInfrastructureTemplate(infrastructureMachineTemplate).
			WithBootstrapTemplate(bootstrapTemplate).
			WithLabels(map[string]string{clusterv1.ClusterTopologyMachineDeploymentLabelName: name + "-topology"}).
			Build(),
		InfrastructureMachineTemplate: infrastructureMachineTemplate,
		BootstrapTemplate:             bootstrapTemplate,
	}
}

func toMachineDeploymentTopologyStateMap(states []*scope.MachineDeploymentState) map[string]*scope.MachineDeploymentState {
	ret := map[string]*scope.MachineDeploymentState{}
	for _, state := range states {
		ret[state.Object.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]] = state
	}
	return ret
}
