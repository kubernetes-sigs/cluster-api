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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/test/builder"
)

func TestClusterDefaultNamespaces(t *testing.T) {
	g := NewWithT(t)

	c := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "fooboo",
		},
		Spec: clusterv1.ClusterSpec{
			InfrastructureRef: &corev1.ObjectReference{},
			ControlPlaneRef:   &corev1.ObjectReference{},
		},
	}
	webhook := &Cluster{}
	t.Run("for Cluster", customDefaultValidateTest(ctx, c, webhook))
	g.Expect(webhook.Default(ctx, c)).To(Succeed())

	g.Expect(c.Spec.InfrastructureRef.Namespace).To(Equal(c.Namespace))
	g.Expect(c.Spec.ControlPlaneRef.Namespace).To(Equal(c.Namespace))
}

// TestClusterDefaultVariables cases where cluster.spec.topology.class is altered.
func TestClusterDefaultVariables(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	tests := []struct {
		name         string
		clusterClass *clusterv1.ClusterClass
		topology     *clusterv1.Topology
		expect       *clusterv1.Topology
	}{
		{
			name: "default a single variable to its correct values",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithVariables(clusterv1.ClusterClassVariable{
					Name:     "location",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "string",
							Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
						},
					},
				}).
				Build(),
			topology: &clusterv1.Topology{},
			expect: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"us-east"`),
						},
					},
				},
			},
		},
		{
			name: "don't change a variable if it is already set",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithVariables(clusterv1.ClusterClassVariable{
					Name:     "location",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type:    "string",
							Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
						},
					},
				},
				).
				Build(),
			topology: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"A different location"`),
						},
					},
				},
			},
			expect: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"A different location"`),
						},
					},
				},
			},
		},
		{
			name: "default many variables to their correct values",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name:     "location",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "string",
								Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
							},
						},
					},
					clusterv1.ClusterClassVariable{
						Name:     "count",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "number",
								Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
							},
						},
					}).
				Build(),
			topology: &clusterv1.Topology{},
			expect: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"us-east"`),
						},
					},
					{
						Name: "count",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`0.1`),
						},
					},
				},
			},
		},
		{
			name: "don't add new variable overrides",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("default-worker").Build(),
				).
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name:     "location",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type:    "string",
								Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
							},
						},
					}).
				Build(),
			topology: &clusterv1.Topology{
				Workers: &clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
						},
					},
				},
			},
			expect: &clusterv1.Topology{
				Workers: &clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							// "location" has not been added to .variables.overrides.
						},
					},
				},
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"us-east"`),
						},
					},
				},
			},
		},
		{
			name: "default nested fields of variable overrides",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("default-worker").Build(),
				).
				WithVariables(
					clusterv1.ClusterClassVariable{
						Name:     "httpProxy",
						Required: true,
						Schema: clusterv1.VariableSchema{
							OpenAPIV3Schema: clusterv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]clusterv1.JSONSchemaProps{
									"enabled": {
										Type: "boolean",
									},
									"url": {
										Type:    "string",
										Default: &apiextensionsv1.JSON{Raw: []byte(`"http://localhost:3128"`)},
									},
								},
							},
						},
					}).
				Build(),
			topology: &clusterv1.Topology{
				Workers: &clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							Variables: &clusterv1.MachineDeploymentVariables{
								Overrides: []clusterv1.ClusterVariable{
									{
										Name:  "httpProxy",
										Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)},
									},
								},
							},
						},
					},
				},
				Variables: []clusterv1.ClusterVariable{
					{
						Name:  "httpProxy",
						Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)},
					},
				},
			},
			expect: &clusterv1.Topology{
				Workers: &clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							Variables: &clusterv1.MachineDeploymentVariables{
								Overrides: []clusterv1.ClusterVariable{
									{
										Name: "httpProxy",
										// url has been added by defaulting.
										Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true,"url":"http://localhost:3128"}`)},
									},
								},
							},
						},
					},
				},
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "httpProxy",
						Value: apiextensionsv1.JSON{
							// url has been added by defaulting.
							Raw: []byte(`{"enabled":true,"url":"http://localhost:3128"}`),
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		// Setting Class and Version here to avoid obfuscating the test cases above.
		tt.topology.Class = "class1"
		tt.topology.Version = "v1.22.2"
		tt.expect.Class = "class1"
		tt.expect.Version = "v1.22.2"

		cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
			WithTopology(tt.topology).
			Build()
		fakeClient := fake.NewClientBuilder().
			WithObjects(tt.clusterClass).
			WithScheme(fakeScheme).
			Build()
		// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
		webhook := &Cluster{Client: fakeClient}

		t.Run(tt.name, func(t *testing.T) {
			// Test if defaulting works in combination with validation.
			customDefaultValidateTest(ctx, cluster, webhook)(t)
			// Test defaulting.
			t.Run("default", func(t *testing.T) {
				g := NewWithT(t)
				g.Expect(webhook.Default(ctx, cluster)).To(Succeed())
				g.Expect(cluster.Spec.Topology).To(Equal(tt.expect))
			})
		})
	}
}

func TestClusterDefaultTopologyVersion(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	g := NewWithT(t)

	c := builder.Cluster("fooboo", "cluster1").
		WithTopology(builder.ClusterTopology().
			WithClass("foo").
			WithVersion("1.19.1").
			Build()).
		Build()

	// Sets up the fakeClient for the test case. This is required because the test uses a Managed Topology.
	fakeClient := fake.NewClientBuilder().
		WithObjects(builder.ClusterClass("fooboo", "foo").Build()).
		WithScheme(fakeScheme).
		Build()

	// Create the webhook and add the fakeClient as its client.
	webhook := &Cluster{Client: fakeClient}
	t.Run("for Cluster", customDefaultValidateTest(ctx, c, webhook))
	g.Expect(webhook.Default(ctx, c)).To(Succeed())

	g.Expect(c.Spec.Topology.Version).To(HavePrefix("v"))
}

func TestClusterValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.

	var (
		tests = []struct {
			name      string
			in        *clusterv1.Cluster
			old       *clusterv1.Cluster
			expectErr bool
		}{
			{
				name:      "should return error when cluster namespace and infrastructure ref namespace mismatch",
				expectErr: true,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithInfrastructureCluster(
						builder.InfrastructureClusterTemplate("barNamespace", "infra1").Build()).
					WithControlPlane(
						builder.ControlPlane("fooNamespace", "cp1").Build()).
					Build(),
			},
			{
				name:      "should return error when cluster namespace and controlPlane ref namespace mismatch",
				expectErr: true,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithInfrastructureCluster(
						builder.InfrastructureClusterTemplate("fooNamespace", "infra1").Build()).
					WithControlPlane(
						builder.ControlPlane("barNamespace", "cp1").Build()).
					Build(),
			},
			{
				name:      "should succeed when namespaces match",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithInfrastructureCluster(
						builder.InfrastructureClusterTemplate("fooNamespace", "infra1").Build()).
					WithControlPlane(
						builder.ControlPlane("fooNamespace", "cp1").Build()).
					Build(),
			},
			{
				name:      "fails if topology is set but feature flag is disabled",
				expectErr: true,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithInfrastructureCluster(
						builder.InfrastructureClusterTemplate("fooNamespace", "infra1").Build()).
					WithControlPlane(
						builder.ControlPlane("fooNamespace", "cp1").Build()).
					WithTopology(&clusterv1.Topology{}).
					Build(),
			},
		}
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create the webhook.
			webhook := &Cluster{}

			err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestClusterTopologyValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	tests := []struct {
		name                  string
		clusterClassVariables []clusterv1.ClusterClassVariable
		in                    *clusterv1.Cluster
		old                   *clusterv1.Cluster
		expectErr             bool
	}{
		{
			name:      "should return error when topology does not have class",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(&clusterv1.Topology{}).
				Build(),
		},
		{
			name:      "should return error when topology does not have valid version",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("invalid").Build()).
				Build(),
		},
		{
			name:      "should return error when downgrading topology version - major",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v2.2.3").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3").
					Build()).
				Build(),
		},
		{
			name:      "should return error when downgrading topology version - minor",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.1.3").
					Build()).
				Build(),
		},
		{
			name:      "should return error when downgrading topology version - patch",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.2").
					Build()).
				Build(),
		},
		{
			name:      "should return error when downgrading topology version - pre-release",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3-xyz.2").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3-xyz.1").
					Build()).
				Build(),
		},
		{
			name:      "should return error when downgrading topology version - build tag",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3+xyz.2").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3+xyz.1").
					Build()).
				Build(),
		},
		{
			name:      "should return error when upgrading +2 minor version",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.4.0").
					Build()).
				Build(),
		},
		{
			name:      "should return error when duplicated MachineDeployments names exists in a Topology",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("bb").
							Build()).
					Build()).
				Build(),
		},
		{
			name:      "should pass when MachineDeployments names in a Topology are unique",
			expectErr: false,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers2").
							WithClass("bb").
							Build()).
					Build()).
				Build(),
		},
		{
			name:      "should update",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers2").
							WithClass("bb").
							Build()).
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.2").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers2").
							WithClass("bb").
							Build()).
					Build()).
				Build(),
		},
		{
			name: "should pass when required variable exists top-level",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					// Variable is not required in MachineDeployment topologies.
					Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "should fail when required variable is missing top-level",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should fail when top-level variable is invalid",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`"text"`)},
					}).
					Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should pass when top-level variable and override are valid",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithClass("aa").
						WithVariables(clusterv1.ClusterVariable{
							Name:  "cpu",
							Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
						}).
						Build()).
					Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "should fail when variable override is invalid",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: true,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithClass("aa").
						WithVariables(clusterv1.ClusterVariable{
							Name:  "cpu",
							Value: apiextensionsv1.JSON{Raw: []byte(`"text"`)},
						}).
						Build()).
					Build()).
				Build(),
			expectErr: true,
		},
		{
			name: "should fail when variable override is missing the corresponding top-level variable",
			clusterClassVariables: []clusterv1.ClusterClassVariable{
				{
					Name:     "cpu",
					Required: false,
					Schema: clusterv1.VariableSchema{
						OpenAPIV3Schema: clusterv1.JSONSchemaProps{
							Type: "integer",
						},
					},
				},
			},
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithClass("aa").
						WithVariables(clusterv1.ClusterVariable{
							Name:  "cpu",
							Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
						}).
						Build()).
					Build()).
				Build(),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			class := builder.ClusterClass("fooboo", "foo").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("bb").Build(),
					*builder.MachineDeploymentClass("aa").Build(),
				).
				WithVariables(tt.clusterClassVariables...).
				Build()
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(class).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			webhook := &Cluster{Client: fakeClient}

			err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

// TestClusterTopologyValidationWithClient tests the additional cases introduced in new validation in the webhook package.
func TestClusterTopologyValidationWithClient(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	tests := []struct {
		name    string
		cluster *clusterv1.Cluster
		class   *clusterv1.ClusterClass
		objects []client.Object
		wantErr bool
	}{
		{
			name: "Accept a cluster with an existing clusterclass named in cluster.spec.topology.class",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				Build(),
			wantErr: false,
		},
		{
			name: "Reject a cluster which has a non-existent clusterclass named in cluster.spec.topology.class",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("wrongName").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.class).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			c := &Cluster{Client: fakeClient}

			// Checks the return error.
			if tt.wantErr {
				g.Expect(c.ValidateCreate(ctx, tt.cluster)).NotTo(Succeed())
			} else {
				g.Expect(c.ValidateCreate(ctx, tt.cluster)).To(Succeed())
			}
		})
	}
}

// TestClusterTopologyValidationForTopologyClassChange cases where cluster.spec.topology.class is altered.
func TestClusterTopologyValidationForTopologyClassChange(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)

	cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
		WithTopology(
			builder.ClusterTopology().
				WithClass("class1").
				WithVersion("v1.22.2").
				WithControlPlaneReplicas(3).
				Build()).
		Build()

	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}
	compatibleNameChangeRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "differentbaz",
		Namespace:  "default",
	}
	compatibleAPIVersionChangeRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo2",
		Kind:       "barTemplate",
		Name:       "differentbaz",
		Namespace:  "default",
	}
	incompatibleKindRef := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "another-baz",
		Namespace:  "default",
	}
	incompatibleAPIGroupRef := &corev1.ObjectReference{
		APIVersion: "group.nottest.io/foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
		Namespace:  "default",
	}

	tests := []struct {
		name        string
		cluster     *clusterv1.Cluster
		firstClass  *clusterv1.ClusterClass
		secondClass *clusterv1.ClusterClass
		wantErr     bool
	}{
		// InfrastructureCluster changes.
		{
			name: "Accept cluster.topology.class change with a compatible infrastructureCluster Kind ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(compatibleNameChangeRef)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with a compatible infrastructureCluster APIVersion ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(compatibleAPIVersionChangeRef)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			wantErr: false,
		},

		{
			name: "Reject cluster.topology.class change with an incompatible infrastructureCluster Kind ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(incompatibleKindRef)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			wantErr: true,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible infrastructureCluster APIGroup ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(incompatibleAPIGroupRef)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			wantErr: true,
		},

		// ControlPlane changes.
		{
			name: "Accept cluster.topology.class change with a compatible controlPlaneTemplate ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(compatibleNameChangeRef)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with a compatible controlPlaneTemplate ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(compatibleAPIVersionChangeRef)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			wantErr: false,
		},

		{
			name: "Reject cluster.topology.class change with an incompatible controlPlane Kind ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(incompatibleKindRef)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			wantErr: true,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible controlPlane APIVersion ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(incompatibleAPIGroupRef)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(compatibleNameChangeRef)).
				Build(),
			wantErr: true,
		},
		{
			name: "Accept cluster.topology.class change with a compatible controlPlane.MachineInfrastructure ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(compatibleNameChangeRef)).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with a compatible controlPlane.MachineInfrastructure ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(compatibleAPIVersionChangeRef)).
				Build(),
			wantErr: false,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible controlPlane.MachineInfrastructure Kind ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(incompatibleKindRef)).
				Build(),
			wantErr: true,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible controlPlane.MachineInfrastructure APIVersion ref change",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(incompatibleAPIGroupRef)).
				Build(),
			wantErr: true,
		},

		// MachineDeploymentClass changes
		{
			name: "Accept cluster.topology.class change with a compatible MachineDeploymentClass InfrastructureTemplate",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(compatibleNameChangeRef)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with an incompatible MachineDeploymentClass BootstrapTemplate",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(compatibleNameChangeRef)).
						WithBootstrapTemplate(refToUnstructured(incompatibleKindRef)).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with a deleted MachineDeploymentClass",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with an added MachineDeploymentClass",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
					*builder.MachineDeploymentClass("bb").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible Kind change to MachineDeploymentClass InfrastructureTemplate",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(incompatibleKindRef)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible APIGroup change to MachineDeploymentClass InfrastructureTemplate",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("aa").
						WithInfrastructureTemplate(refToUnstructured(incompatibleAPIGroupRef)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.firstClass, tt.secondClass).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			c := &Cluster{Client: fakeClient}

			// Create and updated cluster which uses the name of the second class from the test definition in its '.spec.topology.'
			secondCluster := cluster.DeepCopy()
			secondCluster.Spec.Topology.Class = tt.secondClass.Name

			// Checks the return error.
			if tt.wantErr {
				g.Expect(c.ValidateUpdate(ctx, cluster, secondCluster)).NotTo(Succeed())
			} else {
				g.Expect(c.ValidateUpdate(ctx, cluster, secondCluster)).To(Succeed())
			}
		})
	}
}

// TestMovingBetweenManagedAndUnmanaged cluster tests cases where a clusterClass is added or removed during a cluster update.
func TestMovingBetweenManagedAndUnmanaged(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}

	g := NewWithT(t)

	tests := []struct {
		name            string
		cluster         *clusterv1.Cluster
		clusterClass    *clusterv1.ClusterClass
		updatedTopology *clusterv1.Topology
		wantErr         bool
	}{
		{
			name: "Reject cluster moving from Unmanaged to Managed i.e. adding the spec.topology.class field on update",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				Build(),
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			updatedTopology: builder.ClusterTopology().
				WithClass("class1").
				WithVersion("v1.22.2").
				WithControlPlaneReplicas(3).
				Build(),
			wantErr: true,
		},
		{
			name: "Allow cluster moving from Unmanaged to Managed i.e. adding the spec.topology.class field on update " +
				"if and only if ClusterTopologyUnsafeUpdateClassNameAnnotation is set",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithAnnotations(map[string]string{clusterv1.ClusterTopologyUnsafeUpdateClassNameAnnotation: ""}).
				Build(),
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			updatedTopology: builder.ClusterTopology().
				WithClass("class1").
				WithVersion("v1.22.2").
				WithControlPlaneReplicas(3).
				Build(),
			wantErr: false,
		},
		{
			name: "Reject cluster moving from Managed to Unmanaged i.e. removing the spec.topology.class field on update",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("class1").
					WithVersion("v1.22.2").
					WithControlPlaneReplicas(3).
					Build()).
				Build(),
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			updatedTopology: nil,
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.clusterClass, tt.cluster).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			c := &Cluster{Client: fakeClient}

			// Create and updated cluster which uses the name of the second class from the test definition in its '.spec.topology.'
			updatedCluster := tt.cluster.DeepCopy()
			updatedCluster.Spec.Topology = tt.updatedTopology

			// Checks the return error.
			if tt.wantErr {
				g.Expect(c.ValidateUpdate(ctx, tt.cluster, updatedCluster)).NotTo(Succeed())
			} else {
				g.Expect(c.ValidateUpdate(ctx, tt.cluster, updatedCluster)).To(Succeed())
			}
		})
	}
}

func refToUnstructured(ref *corev1.ObjectReference) *unstructured.Unstructured {
	gvk := ref.GetObjectKind().GroupVersionKind()
	output := &unstructured.Unstructured{}
	output.SetKind(gvk.Kind)
	output.SetAPIVersion(gvk.GroupVersion().String())
	output.SetName(ref.Name)
	output.SetNamespace(ref.Namespace)
	return output
}
