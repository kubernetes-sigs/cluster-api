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

package webhooks

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/builder"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	ctx        = ctrl.SetupSignalHandler()
	fakeScheme = runtime.NewScheme()
)

func init() {
	_ = clusterv1.AddToScheme(fakeScheme)
}

func TestClusterClassDefaultNamespaces(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	namespace := "default"

	in := builder.ClusterClass(namespace, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate("", "infra1").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate("", "cp1").
				Build()).
		WithControlPlaneInfrastructureMachineTemplate(
			builder.InfrastructureMachineTemplate("", "cpInfra1").
				Build()).
		WithWorkerMachineDeploymentClasses(
			*builder.MachineDeploymentClass("aa").
				WithInfrastructureTemplate(
					builder.InfrastructureMachineTemplate("", "infra1").Build()).
				WithBootstrapTemplate(
					builder.BootstrapTemplate("", "bootstrap1").Build()).
				Build()).
		Build()

	fakeClient := fake.NewClientBuilder().
		WithScheme(fakeScheme).
		Build()

	// Create the webhook and add the fakeClient as its client.
	webhook := &ClusterClass{Client: fakeClient}
	t.Run("for ClusterClass", customDefaultValidateTest(ctx, in, webhook))

	g := NewWithT(t)
	g.Expect(webhook.Default(ctx, in)).To(Succeed())

	// Namespace defaulted on references
	g.Expect(in.Spec.Infrastructure.Ref.Namespace).To(Equal(namespace))
	g.Expect(in.Spec.ControlPlane.Ref.Namespace).To(Equal(namespace))
	g.Expect(in.Spec.ControlPlane.MachineInfrastructure.Ref.Namespace).To(Equal(namespace))
	for i := range in.Spec.Workers.MachineDeployments {
		g.Expect(in.Spec.Workers.MachineDeployments[i].Template.Bootstrap.Ref.Namespace).To(Equal(namespace))
		g.Expect(in.Spec.Workers.MachineDeployments[i].Template.Infrastructure.Ref.Namespace).To(Equal(namespace))
	}
}

func TestClusterClassValidationFeatureGated(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.

	tests := []struct {
		name      string
		in        *clusterv1.ClusterClass
		old       *clusterv1.ClusterClass
		expectErr bool
	}{
		{
			name: "creation should fail if feature flag is disabled, no matter the ClusterClass is valid(or not)",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("", "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate("", "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate("", "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update should fail if feature flag is disabled, no matter the ClusterClass is valid(or not)",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("", "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate("", "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate("", "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("", "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate("", "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate("", "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("", "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			webhook := &ClusterClass{}
			if tt.expectErr {
				g.Expect(webhook.validate(ctx, tt.old, tt.in)).NotTo(Succeed())
			} else {
				g.Expect(webhook.validate(ctx, tt.old, tt.in)).To(Succeed())
			}
		})
	}
}

func TestClusterClassValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	refBadTemplate := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "bar",
		Name:       "baz",
		Namespace:  "default",
	}
	refBadAPIVersion := &corev1.ObjectReference{
		APIVersion: "group/test.io/v1/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	incompatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	compatibleRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/another-foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
		Namespace:  "default",
	}

	tests := []struct {
		name      string
		in        *clusterv1.ClusterClass
		old       *clusterv1.ClusterClass
		expectErr bool
	}{

		/*
			CREATE Tests
		*/

		{
			name: "create pass",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},

		// empty name in ref tests
		{
			name: "create fail infrastructureCluster has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail controlPlane class has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail control plane class machineInfrastructure has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail machineDeployment Bootstrap has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail machineDeployment Infrastructure has empty name",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap").Build()).
						Build()).
				Build(),
			expectErr: true,
		},

		// inconsistent namespace in ref tests
		{
			name: "create fail if InfrastructureCluster has inconsistent namespace",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate("WrongNamespace", "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail if controlPlane has inconsistent namespace",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate("WrongNamespace", "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail if controlPlane machineInfrastructure has inconsistent namespace",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate("WrongNamespace", "cpInfra1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail if machineDeployment / bootstrap has inconsistent namespace",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate("WrongNamespace", "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail if machineDeployment / infrastructure has inconsistent namespace",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate("WrongNamespace", "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},

		// bad template in ref tests
		{
			name: "create fail if bad template in InfrastructureCluster",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(refBadTemplate)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in controlPlane machineInfrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(refBadTemplate)).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in ControlPlane",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(refBadTemplate)).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machineDeployment Bootstrap",

			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(refBadTemplate)).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail if bad template in machineDeployment Infrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(refBadTemplate)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},

		// bad apiVersion in ref tests
		{
			name: "create fail with a bad APIVersion for template in InfrastructureCluster",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(refBadAPIVersion)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in controlPlane machineInfrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(refBadAPIVersion)).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in ControlPlane",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(refBadAPIVersion)).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in machineDeployment Bootstrap",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							refToUnstructured(refBadAPIVersion)).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},
		{
			name: "create fail with a bad APIVersion for template in machineDeployment Infrastructure",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(refBadAPIVersion)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			old:       nil,
			expectErr: true,
		},

		// create test
		{
			name: "create fail if duplicated machineDeploymentClasses",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},

		/*
			UPDATE Tests
		*/

		{
			name: "update pass in case of no changes",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update pass if infrastructureCluster changes in a compatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(compatibleRef)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails if infrastructureCluster changes in an incompatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(ref)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update pass if controlPlane changes in a compatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(compatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(compatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails if controlPlane changes in an incompatible way (controlPlane template)",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(incompatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(compatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update fails if controlPlane changes in an incompatible way (controlPlane infrastructureMachineTemplate)",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					refToUnstructured(compatibleRef)).
				WithControlPlaneInfrastructureMachineTemplate(
					refToUnstructured(incompatibleRef)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update pass if a machine deployment changes in a compatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(compatibleRef)).
						WithBootstrapTemplate(
							refToUnstructured(incompatibleRef)).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails a machine deployment changes in an incompatible way",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(ref)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							refToUnstructured(incompatibleRef)).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "update pass if a machine deployment class gets added",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("BB").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "update fails if a duplicated deployment class gets added",
			old: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).Build(),
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				Build()

			webhook := &ClusterClass{Client: fakeClient}

			// Create the webhook and add the fakeClient as its client.
			if tt.expectErr {
				g.Expect(webhook.validate(ctx, tt.old, tt.in)).NotTo(Succeed())
			} else {
				g.Expect(webhook.validate(ctx, tt.old, tt.in)).To(Succeed())
			}
		})
	}
}

func TestClusterClassValidationWithClusterAwareChecks(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	tests := []struct {
		name            string
		oldClusterClass *clusterv1.ClusterClass
		newClusterClass *clusterv1.ClusterClass
		clusters        []client.Object
		expectErr       bool
	}{
		{
			name: "error if a MachineDeploymentClass in use gets removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "error if many MachineDeploymentClasses, used in multiple Clusters using the modified ClusterClass, are removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers2").
									WithClass("aa").
									Build(),
							).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("aa").
									Build(),
							).
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers2").
									WithClass("aa").
									Build(),
							).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster3").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "pass if a similar MachineDeploymentClass is deleted when it is only used in Clusters not belonging to the ClusterClass",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class2").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").

									// A MachineDeploymentClass with the same name is in ClusterClass "class1" but
									// this cluster is based on ClusterClass "class2" and does not impact deletion.
									WithClass("aa").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "pass if a MachineDeploymentClass not in use gets removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithMachineDeployment(
								builder.MachineDeploymentTopology("workers1").
									WithClass("bb").
									Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build()).
				Build(),
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(tt.clusters...).
				Build()

			// Create the webhook and add the fakeClient as its client.
			webhook := &ClusterClass{Client: fakeClient}

			if tt.expectErr {
				g.Expect(webhook.validate(ctx, tt.oldClusterClass, tt.newClusterClass)).NotTo(Succeed())
			} else {
				g.Expect(webhook.validate(ctx, tt.oldClusterClass, tt.newClusterClass)).To(Succeed())
			}
		})
	}
}

func TestClusterClass_ValidateDelete(t *testing.T) {
	class := builder.ClusterClass(metav1.NamespaceDefault, "class1").Build()

	tests := []struct {
		name      string
		clusters  []client.Object
		expectErr bool
	}{
		{
			name: "allow deletion if a cluster exists but does not reference the ClusterClass for deletion",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class2").
							Build()).
					Build(),
			},
			expectErr: false,
		},
		{
			name: "error if cluster exists with a reference to the ClusterClass for deletion",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							Build()).
					Build(),
			},
			expectErr: true,
		},
		{
			name: "error if multiple clusters exist and at least one references to the ClusterClass for deletion",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class2").
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster3").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class3").
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster4").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class4").
							Build()).
					Build(),
			},
			expectErr: true,
		},
		{
			name: "allow deletion if multiple clusters exist and none of them references to the ClusterClass for deletion",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class5").
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class2").
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster3").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class3").
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster4").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class4").
							Build()).
					Build(),
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.clusters...).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client.
			webhook := &ClusterClass{Client: fakeClient}

			if tt.expectErr {
				g.Expect(webhook.ValidateDelete(ctx, class)).NotTo(Succeed())
			} else {
				g.Expect(webhook.ValidateDelete(ctx, class)).To(Succeed())
			}
		})
	}
}
