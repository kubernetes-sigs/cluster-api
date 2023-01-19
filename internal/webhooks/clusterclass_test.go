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
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/api/v1beta1/index"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
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
		WithIndex(&clusterv1.Cluster{}, index.ClusterClassNameField, index.ClusterByClusterClassClassName).
		Build()

	// Create the webhook and add the fakeClient as its client.
	webhook := &ClusterClass{Client: fakeClient}
	t.Run("for ClusterClass", util.CustomDefaultValidateTest(ctx, in, webhook))

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
			err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
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
		{
			name: "create pass if valid machineHealthCheck defined for ControlPlane with MachineInfrastructure set",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:    corev1.NodeReady,
							Status:  corev1.ConditionUnknown,
							Timeout: metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					NodeStartupTimeout: &metav1.Duration{
						Duration: time.Duration(6000000000000)}}).
				Build(),
		},
		{
			name: "create fail if MachineHealthCheck defined for ControlPlane with MachineInfrastructure unset",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				// No ControlPlaneMachineInfrastructure makes this an invalid creation request.
				WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{
					NodeStartupTimeout: &metav1.Duration{
						Duration: time.Duration(6000000000000)}}).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail if ControlPlane MachineHealthCheck does not define UnhealthyConditions",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneInfrastructureMachineTemplate(
					builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpInfra1").
						Build()).
				WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{
					NodeStartupTimeout: &metav1.Duration{
						Duration: time.Duration(6000000000000)}}).
				Build(),
			expectErr: true,
		},
		{
			name: "create pass if MachineDeployment MachineHealthCheck is valid",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{
							UnhealthyConditions: []clusterv1.UnhealthyCondition{
								{
									Type:    corev1.NodeReady,
									Status:  corev1.ConditionUnknown,
									Timeout: metav1.Duration{Duration: 5 * time.Minute},
								},
							},
							NodeStartupTimeout: &metav1.Duration{
								Duration: time.Duration(6000000000000)}}).
						Build()).
				Build(),
		},
		{
			name: "create fail if MachineDeployment MachineHealthCheck NodeStartUpTimeout is too short",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{
							UnhealthyConditions: []clusterv1.UnhealthyCondition{
								{
									Type:    corev1.NodeReady,
									Status:  corev1.ConditionUnknown,
									Timeout: metav1.Duration{Duration: 5 * time.Minute},
								},
							},
							NodeStartupTimeout: &metav1.Duration{
								// nodeStartupTimeout is too short here - 600ns.
								Duration: time.Duration(600)}}).
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "create fail if MachineDeployment MachineHealthCheck does not define UnhealthyConditions",
			in: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "infra1").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{
							NodeStartupTimeout: &metav1.Duration{
								Duration: time.Duration(6000000000000)}}).
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
				WithIndex(&clusterv1.Cluster{}, index.ClusterClassNameField, index.ClusterByClusterClassClassName).
				Build()

			// Create the webhook and add the fakeClient as its client.
			webhook := &ClusterClass{Client: fakeClient}
			err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
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
			name: "error if a control plane MachineHealthCheck that is in use by a cluster is removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
							Enable: pointer.Bool(true),
						}).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{
					UnhealthyConditions: []clusterv1.UnhealthyCondition{
						{
							Type:    corev1.NodeReady,
							Status:  corev1.ConditionUnknown,
							Timeout: metav1.Duration{Duration: 5 * time.Minute},
						},
					},
				}).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "pass if a control plane MachineHealthCheck is removed but no cluster enforces it",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{}).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "pass if a control plane MachineHealthCheck is removed when clusters are not using it (clusters have overrides in topology)",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
							Enable: pointer.Bool(true),
							MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
								UnhealthyConditions: []clusterv1.UnhealthyCondition{
									{
										Type:    corev1.NodeReady,
										Status:  corev1.ConditionUnknown,
										Timeout: metav1.Duration{Duration: 5 * time.Minute},
									},
								},
							},
						}).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{}).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "error if a MachineDeployment MachineHealthCheck that is in use by a cluster is removed",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithMachineDeployment(builder.MachineDeploymentTopology("md1").
							WithClass("mdclass1").
							WithMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
								Enable: pointer.Bool(true),
							}).
							Build()).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{}).
						Build(),
				).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			expectErr: true,
		},
		{
			name: "pass if a MachineDeployment MachineHealthCheck is removed but no cluster enforces it",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithMachineDeployment(builder.MachineDeploymentTopology("md1").
							WithClass("mdclass1").
							Build()).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{}).
						Build(),
				).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
				Build(),
			expectErr: false,
		},
		{
			name: "pass if a MachineDeployment MachineHealthCheck is removed when clusters are not using it (clusters have overrides in topology)",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithTopology(builder.ClusterTopology().
						WithClass("clusterclass1").
						WithMachineDeployment(builder.MachineDeploymentTopology("md1").
							WithClass("mdclass1").
							WithMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
								Enable: pointer.Bool(true),
								MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
									UnhealthyConditions: []clusterv1.UnhealthyCondition{
										{
											Type:    corev1.NodeReady,
											Status:  corev1.ConditionUnknown,
											Timeout: metav1.Duration{Duration: 5 * time.Minute},
										},
									},
								},
							}).
							Build()).
						Build()).
					Build(),
			},
			oldClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{}).
						Build(),
				).
				Build(),
			newClusterClass: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass1").
				WithInfrastructureClusterTemplate(
					builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
				WithControlPlaneTemplate(
					builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
						Build()).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("mdclass1").
						WithInfrastructureTemplate(
							builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "infra1").Build()).
						WithBootstrapTemplate(
							builder.BootstrapTemplate(metav1.NamespaceDefault, "bootstrap1").Build()).
						Build(),
				).
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
				WithIndex(&clusterv1.Cluster{}, index.ClusterClassNameField, index.ClusterByClusterClassClassName).
				Build()

			// Create the webhook and add the fakeClient as its client.
			webhook := &ClusterClass{Client: fakeClient}
			err := webhook.validate(ctx, tt.oldClusterClass, tt.newClusterClass)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestClusterClassValidationWithVariableChecks(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create or update ClusterClasses.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	clusterClassBuilder := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithInfrastructureClusterTemplate(
			builder.InfrastructureClusterTemplate(metav1.NamespaceDefault, "inf").Build()).
		WithControlPlaneTemplate(
			builder.ControlPlaneTemplate(metav1.NamespaceDefault, "cp1").
				Build())
	tests := []struct {
		name            string
		oldClusterClass *clusterv1.ClusterClass
		newClusterClass *clusterv1.ClusterClass
		clusters        []client.Object
		expectErr       bool
	}{
		{
			name: "Pass if a Minimum ClusterClassVariable is changed but does not invalidate an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`5`),
									},
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Minimum: pointer.Int64(1),
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
								// Minimum changed to a value that does not invalidate ClusterVariable.
								Minimum: pointer.Int64(3),
							},
						},
					}).
				Build(),
			expectErr: false,
		},
		{
			name: "Error if a Minimum ClusterClassVariable is changed and invalidates an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Minimum: pointer.Int64(1),
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
								// Minimum changed to a value that invalidates ClusterVariable.
								Minimum: pointer.Int64(100),
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Error if a Minimum ClusterClassVariable is changed and invalidates an existing ClusterVariable override",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`20`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`10`),
									},
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Minimum: pointer.Int64(1),
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
								// Minimum changed to a value that invalidates the ClusterVariable override.
								Minimum: pointer.Int64(15),
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Pass if a Maximum ClusterClassVariable is changed but does not invalidate an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`3`),
									},
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Maximum: pointer.Int64(5),
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
								// Maximum changed to a value that does not invalidate ClusterVariable.
								Maximum: pointer.Int64(100),
							},
						},
					}).
				Build(),
			expectErr: false,
		},
		{
			name: "Error if a Maximum ClusterClassVariable is changed and invalidates an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "integer",
								Maximum: pointer.Int64(5),
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
								// Maximum changed to a value that invalidates ClusterVariable.
								Maximum: pointer.Int64(3),
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Pass if an Enum ClusterClassVariable is changed but does not invalidate an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "zone",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"us-east-1"`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "zone",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"us-east-2"`),
									},
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								Enum: []apiextensionsv1.JSON{
									{Raw: []byte(`"us-east-1"`)},
									{Raw: []byte(`"us-east-2"`)},
									{Raw: []byte(`"us-east-3"`)},
								},
							},
						},
					}).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								Enum: []apiextensionsv1.JSON{
									{Raw: []byte(`"us-east-1"`)},
									{Raw: []byte(`"us-east-2"`)},
									{Raw: []byte(`"us-east-4"`)},
								},
							},
						},
					}).
				Build(),
			expectErr: false,
		},
		{
			name: "Error if an Enum ClusterClassVariable is changed to invalidate an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "zone",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"us-east-1"`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								Enum: []apiextensionsv1.JSON{
									{Raw: []byte(`"us-east-1"`)},
									{Raw: []byte(`"us-east-2"`)},
								},
							},
						},
					}).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
								Enum: []apiextensionsv1.JSON{
									// "us-east-1" removed from enum
									{Raw: []byte(`"us-east-2"`)},
									{Raw: []byte(`"us-east-3"`)},
								},
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Error if Type ClusterClassVariable is changed (Integer -> String) invalidating an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								// Type changed to a value that invalidates ClusterVariable.
								Type: "string",
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Pass if Type ClusterClassVariable is changed (Integer -> Number) not invalidating an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`5`),
									},
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								// Type changed to a value that does not invalidate ClusterVariable.
								Type: "number",
							},
						},
					}).
				Build(),
			expectErr: false,
		},
		{
			name: "Error if Type ClusterClassVariable is changed ( Number-> Integer) invalidating an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4.1`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								// Type changed to a value that does not invalidate ClusterVariable.
								Type: "integer",
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Pass if Type ClusterClassVariable is changed (Object -> Map) not invalidating an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "config",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`{"stringField":"value1"}`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "config",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"stringField": {
										Type: "string",
									},
								},
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "config",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								// Type changed to a map which allows the same format (map[string]string).
								Type: "object",
								AdditionalProperties: &clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					}).
				Build(),
			expectErr: false,
		},
		{
			name: "Error if Type ClusterClassVariable is changed (Object -> Map) invalidating an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "config",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`{"stringField":"value1","numberField":1}`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "config",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"stringField": {
										Type: "string",
									},
									"numberField": {
										Type: "number",
									},
								},
							},
						},
					}).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "config",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "object",
								AdditionalProperties: &clusterv1.JSONSchemaProps{
									Type: "string",
								},
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Error if clusterClass update removes variable which is referenced in an existing cluster",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte{'1'},
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte{'1'},
									},
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name: "hdd",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cdrom",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name: "hdd",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		{
			name: "Pass if an update is made to a ClusterClass with required variables, but with no changes to variables",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								},
								clusterv1.ClusterVariable{
									Name: "hdd",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
									// Note: hdd is only required as top-level variable not as override.
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name:     "hdd",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name:     "hdd",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
				).
				Build(),
			expectErr: false,
		},
		{
			name: "Pass if adding a non-required variable to the ClusterClass",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "zone",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"first-zone"`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(clusterv1.ClusterVariable{
									Name: "zone",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"second-zone"`),
									},
								}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name:     "location",
						Required: false,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				).
				Build(),
			expectErr: false,
		},
		{
			name: "Error if adding a non-required variable to the ClusterClass that invalidates an existing ClusterVariable",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "zone",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"first-zone"`),
									},
								},
								clusterv1.ClusterVariable{
									Name: "location",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"first-zone"`),
									},
								}).
							WithMachineDeployment(builder.MachineDeploymentTopology("md-1").
								WithVariables(
									clusterv1.ClusterVariable{
										Name: "zone",
										Value: apiextensionsv1.JSON{
											Raw: []byte(`"first-zone"`),
										},
									},
									clusterv1.ClusterVariable{
										Name: "location",
										Value: apiextensionsv1.JSON{
											Raw: []byte(`"first-zone"`),
										},
									}).
								Build(),
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "zone",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name:     "location",
						Required: false,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:      "string",
								MaxLength: pointer.Int64(5),
							},
						},
					},
				).
				Build(),
			expectErr: true,
		},

		{
			name: "Error if required variable is added but not defined in all clusters",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								},
								clusterv1.ClusterVariable{
									Name: "hdd",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`"hdd0"`),
									},
								},
							).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name:     "hdd",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					}).
				Build(),
			expectErr: true,
		},
		// This case is the same as the above but has many clusters and variables.
		{
			name: "Error if required variable is added but not defined in all clusters (with many variables)",
			clusters: []client.Object{
				builder.Cluster(metav1.NamespaceDefault, "cluster1").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster2").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								},
								clusterv1.ClusterVariable{
									Name: "hdd",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							Build()).
					Build(),
				builder.Cluster(metav1.NamespaceDefault, "cluster3").
					WithLabels(map[string]string{clusterv1.ClusterTopologyOwnedLabel: ""}).
					WithTopology(
						builder.ClusterTopology().
							WithClass("class1").
							WithVariables(
								clusterv1.ClusterVariable{
									Name: "cpu",
									Value: apiextensionsv1.JSON{
										Raw: []byte(`4`),
									},
								}).
							Build()).
					Build(),
			},
			oldClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name:     "memory",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name:     "hdd",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
				).
				Build(),
			newClusterClass: clusterClassBuilder.
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name: "cdrom",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name: "cpu",
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "integer",
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name:     "hdd",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "number",
							},
						},
					},
				).
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
				WithObjects(tt.clusters...).
				WithIndex(&clusterv1.Cluster{}, index.ClusterClassNameField, index.ClusterByClusterClassClassName).
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
