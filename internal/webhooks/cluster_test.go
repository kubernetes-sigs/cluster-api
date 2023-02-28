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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/test/builder"
	"sigs.k8s.io/cluster-api/internal/webhooks/util"
	"sigs.k8s.io/cluster-api/util/conditions"
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
	t.Run("for Cluster", util.CustomDefaultValidateTest(ctx, c, webhook))

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
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
					},
				},
				).
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
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
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
				WithStatusVariables([]clusterv1.ClusterClassStatusVariable{
					{
						Name: "location",
						Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
							{
								Required: true,
								From:     clusterv1.VariableDefinitionFromInline,
								Schema: clusterv1.VariableSchema{
									OpenAPIV3Schema: clusterv1.JSONSchemaProps{
										Type:    "string",
										Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
									},
								},
							},
						},
					},
					{
						Name: "count",
						Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
							{
								Required: true,
								From:     clusterv1.VariableDefinitionFromInline,
								Schema: clusterv1.VariableSchema{
									OpenAPIV3Schema: clusterv1.JSONSchemaProps{
										Type:    "number",
										Default: &apiextensionsv1.JSON{Raw: []byte(`0.1`)},
									},
								},
							},
						},
					},
				}...).
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
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
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
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "httpProxy",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
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
		{
			name: "Use one value for multiple definitions when variables don't conflict",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: true,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: true,
							From:     "anotherpatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
					},
				},
				).
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
			name: "Add defaults for each definitionFrom if variable is defined for some definitionFrom",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: true,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: true,
							From:     "anotherpatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
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
							Raw: []byte(`"us-west"`),
						},
						DefinitionFrom: "somepatch",
					},
				},
			},
			expect: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"us-west"`),
						},
						DefinitionFrom: "somepatch",
					},
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"us-east"`),
						},
						DefinitionFrom: clusterv1.VariableDefinitionFromInline,
					},
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"us-east"`),
						},
						DefinitionFrom: "anotherpatch",
					},
				},
			},
		},
		{
			name: "set definitionFrom on defaults when variables conflict",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name:                "location",
					DefinitionsConflict: true,
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"first-region"`)},
								},
							},
						},
						{
							Required: true,
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"another-region"`)},
								},
							},
						},
						{
							Required: true,
							From:     "anotherpatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
					},
				},
				).
				Build(),
			topology: &clusterv1.Topology{},
			expect: &clusterv1.Topology{
				Variables: []clusterv1.ClusterVariable{
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"first-region"`),
						},
						DefinitionFrom: clusterv1.VariableDefinitionFromInline,
					},
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"another-region"`),
						},
						DefinitionFrom: "somepatch",
					},
					{
						Name: "location",
						Value: apiextensionsv1.JSON{
							Raw: []byte(`"us-east"`),
						},
						DefinitionFrom: "anotherpatch",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setting Class and Version here to avoid obfuscating the test cases above.
			tt.topology.Class = "class1"
			tt.topology.Version = "v1.22.2"
			tt.expect.Class = "class1"
			tt.expect.Version = "v1.22.2"

			cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(tt.topology).
				Build()

			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.MarkTrue(tt.clusterClass, clusterv1.ClusterClassVariablesReconciledCondition)
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.clusterClass).
				WithScheme(fakeScheme).
				Build()
			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			webhook := &Cluster{Client: fakeClient}

			// Test if defaulting works in combination with validation.
			util.CustomDefaultValidateTest(ctx, cluster, webhook)(t)
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

	clusterClass := builder.ClusterClass("fooboo", "foo").Build()
	conditions.MarkTrue(clusterClass, clusterv1.ClusterClassVariablesReconciledCondition)
	// Sets up the fakeClient for the test case. This is required because the test uses a Managed Topology.
	fakeClient := fake.NewClientBuilder().
		WithObjects(clusterClass).
		WithScheme(fakeScheme).
		Build()

	// Create the webhook and add the fakeClient as its client.
	webhook := &Cluster{Client: fakeClient}
	t.Run("for Cluster", util.CustomDefaultValidateTest(ctx, c, webhook))

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
			{
				name:      "pass with undefined CIDR ranges",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{}},
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{}},
					}).
					Build(),
			},
			{
				name:      "pass with nil CIDR ranges",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: nil},
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: nil},
					}).
					Build(),
			},
			{
				name:      "pass with valid IPv4 CIDR ranges",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.10.10.10/24"}},
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.10.10.10/24"}},
					}).
					Build(),
			},
			{
				name:      "pass with valid IPv6 CIDR ranges",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64"}},
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64"}},
					}).
					Build(),
			},
			{
				name:      "pass with valid dualstack CIDR ranges",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64", "10.10.10.10/24"}},
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64", "10.10.10.10/24"}},
					}).
					Build(),
			},
			{
				name:      "pass if multiple CIDR ranges of IPv4 are passed",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.10.10.10/24", "11.11.11.11/24"}},
					}).
					Build(),
			},
			{
				name:      "pass if multiple CIDR ranges of IPv6 are passed",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"2002::1234:abcd:ffff:c0a8:101/64", "2004::1234:abcd:ffff:c0a8:101/64"}},
					}).
					Build(),
			},
			{
				name:      "pass if too many cidr ranges are specified in the clusterNetwork pods field",
				expectErr: false,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Pods: &clusterv1.NetworkRanges{
							CIDRBlocks: []string{"10.10.10.10/24", "11.11.11.11/24", "12.12.12.12/24"}}}).
					Build(),
			},
			{
				name:      "fails if service cidr ranges are not valid",
				expectErr: true,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Services: &clusterv1.NetworkRanges{
							// Invalid ranges: missing network suffix
							CIDRBlocks: []string{"10.10.10.10", "11.11.11.11"}}}).
					Build(),
			},
			{
				name:      "fails if pod cidr ranges are not valid",
				expectErr: true,
				in: builder.Cluster("fooNamespace", "cluster1").
					WithClusterNetwork(&clusterv1.ClusterNetwork{
						Pods: &clusterv1.NetworkRanges{
							// Invalid ranges: missing network suffix
							CIDRBlocks: []string{"10.10.10.10", "11.11.11.11"}}}).
					Build(),
			},
			{
				name:      "pass with name of under 63 characters",
				expectErr: false,
				in:        builder.Cluster("fooNamespace", "short-name").Build(),
			},
			{
				name:      "pass with _, -, . characters in name",
				in:        builder.Cluster("fooNamespace", "thisNameContains.A_Non-Alphanumeric").Build(),
				expectErr: false,
			},
			{
				name:      "fails if cluster name is longer than 63 characters",
				in:        builder.Cluster("fooNamespace", "thisNameIsReallyMuchLongerThanTheMaximumLengthOfSixtyThreeCharacters").Build(),
				expectErr: true,
			},
			{
				name:      "error when name starts with NonAlphanumeric character",
				in:        builder.Cluster("fooNamespace", "-thisNameStartsWithANonAlphanumeric").Build(),
				expectErr: true,
			},
			{
				name:      "error when name ends with NonAlphanumeric character",
				in:        builder.Cluster("fooNamespace", "thisNameEndsWithANonAlphanumeric.").Build(),
				expectErr: true,
			},
			{
				name:      "error when name contains invalid NonAlphanumeric character",
				in:        builder.Cluster("fooNamespace", "thisNameContainsInvalid!@NonAlphanumerics").Build(),
				expectErr: true,
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
		name                        string
		clusterClassStatusVariables []clusterv1.ClusterClassStatusVariable
		in                          *clusterv1.Cluster
		old                         *clusterv1.Cluster
		expectErr                   bool
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
			clusterClassStatusVariables: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
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
			clusterClassStatusVariables: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
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
			clusterClassStatusVariables: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
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
			clusterClassStatusVariables: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
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
			clusterClassStatusVariables: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: true,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
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
			name: "should pass even when variable override is missing the corresponding top-level variable",
			clusterClassStatusVariables: []clusterv1.ClusterClassStatusVariable{
				{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: false,
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
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
			expectErr: false,
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
				WithStatusVariables(tt.clusterClassStatusVariables...).
				Build()

			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.MarkTrue(class, clusterv1.ClusterClassVariablesReconciledCondition)
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
			name: "Accept a cluster with an existing ClusterClass named in cluster.spec.topology.class",
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
			name: "Accept a cluster with non-existent ClusterClass referenced cluster.spec.topology.class",
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
			wantErr: false,
		},
		{
			name: "Reject a cluster that has MHC enabled for control plane but is missing MHC definition in cluster topology and clusterclass",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
							Enable: pointer.Bool(true),
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				Build(),
			wantErr: true,
		},
		{
			name: "Reject a cluster that MHC override defined for control plane but is missing unhealthy conditions",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
							MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
								UnhealthyConditions: []clusterv1.UnhealthyCondition{},
							},
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				Build(),
			wantErr: true,
		},
		{
			name: "Reject a cluster that MHC override defined for control plane but is set when control plane is missing machineInfrastructure",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
							MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
								UnhealthyConditions: []clusterv1.UnhealthyCondition{
									{
										Type:   corev1.NodeReady,
										Status: corev1.ConditionFalse,
									},
								},
							},
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				Build(),
			wantErr: true,
		},
		{
			name: "Accept a cluster that has MHC enabled for control plane with control plane MHC defined in ClusterClass",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
							Enable: pointer.Bool(true),
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckClass{}).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept a cluster that has MHC enabled for control plane with control plane MHC defined in cluster topology",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
							Enable: pointer.Bool(true),
							MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
								UnhealthyConditions: []clusterv1.UnhealthyCondition{
									{
										Type:   corev1.NodeReady,
										Status: corev1.ConditionFalse,
									},
								},
							},
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
				Build(),
			wantErr: false,
		},
		{
			name: "Reject a cluster that has MHC enabled for machine deployment but is missing MHC definition in cluster topology and ClusterClass",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("md1").
								WithClass("worker-class").
								WithMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
									Enable: pointer.Bool(true),
								}).
								Build(),
						).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("worker-class").Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "Reject a cluster that has MHC override defined for machine deployment but is missing unhealthy conditions",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("md1").
								WithClass("worker-class").
								WithMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
									MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
										UnhealthyConditions: []clusterv1.UnhealthyCondition{},
									},
								}).
								Build(),
						).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("worker-class").Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "Accept a cluster that has MHC enabled for machine deployment with machine deployment MHC defined in ClusterClass",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("md1").
								WithClass("worker-class").
								WithMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
									Enable: pointer.Bool(true),
								}).
								Build(),
						).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("worker-class").
						WithMachineHealthCheckClass(&clusterv1.MachineHealthCheckClass{}).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept a cluster that has MHC enabled for machine deployment with machine deployment MHC defined in cluster topology",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("md1").
								WithClass("worker-class").
								WithMachineHealthCheck(&clusterv1.MachineHealthCheckTopology{
									Enable: pointer.Bool(true),
									MachineHealthCheckClass: clusterv1.MachineHealthCheckClass{
										UnhealthyConditions: []clusterv1.UnhealthyCondition{
											{
												Type:   corev1.NodeReady,
												Status: corev1.ConditionFalse,
											},
										},
									},
								}).
								Build(),
						).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("worker-class").Build(),
				).
				Build(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.MarkTrue(tt.class, clusterv1.ClusterClassVariablesReconciledCondition)
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
			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.MarkTrue(tt.firstClass, clusterv1.ClusterClassVariablesReconciledCondition)
			conditions.MarkTrue(tt.secondClass, clusterv1.ClusterClassVariablesReconciledCondition)

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
		{
			name: "Reject cluster update if ClusterClass does not exist",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("class1").
					WithVersion("v1.22.2").
					WithControlPlaneReplicas(3).
					Build()).
				Build(),
			clusterClass:
			// ClusterClass name is different to that in the Cluster `.spec.topology.class`
			builder.ClusterClass(metav1.NamespaceDefault, "completely-different-class").
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.MarkTrue(tt.clusterClass, clusterv1.ClusterClassVariablesReconciledCondition)
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

// TestClusterClassPollingErrors tests when a Cluster can be reconciled given different reconcile states of the ClusterClass.
func TestClusterClassPollingErrors(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	g := NewWithT(t)
	ref := &corev1.ObjectReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
		Namespace:  "default",
	}

	topology := builder.ClusterTopology().WithClass("class1").WithVersion("v1.24.3").Build()
	secondTopology := builder.ClusterTopology().WithClass("class2").WithVersion("v1.24.3").Build()
	notFoundTopology := builder.ClusterTopology().WithClass("doesnotexist").WithVersion("v1.24.3").Build()

	baseClusterClass := builder.ClusterClass(metav1.NamespaceDefault, "class1").
		WithInfrastructureClusterTemplate(refToUnstructured(ref)).
		WithControlPlaneTemplate(refToUnstructured(ref)).
		WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref))

	// ccFullyReconciled is a ClusterClass with a matching generation and observed generation, and VariablesReconciled=True.
	ccFullyReconciled := baseClusterClass.DeepCopy().Build()
	ccFullyReconciled.Generation = 1
	ccFullyReconciled.Status.ObservedGeneration = 1
	conditions.MarkTrue(ccFullyReconciled, clusterv1.ClusterClassVariablesReconciledCondition)

	// secondFullyReconciled is a second ClusterClass with a matching generation and observed generation, and VariablesReconciled=True.
	secondFullyReconciled := ccFullyReconciled.DeepCopy()
	secondFullyReconciled.SetName("class2")

	// ccGenerationMismatch is a ClusterClass with a mismatched generation and observed generation, but VariablesReconciledCondition=True.
	ccGenerationMismatch := baseClusterClass.DeepCopy().Build()
	ccGenerationMismatch.Generation = 999
	ccGenerationMismatch.Status.ObservedGeneration = 1
	conditions.MarkTrue(ccGenerationMismatch, clusterv1.ClusterClassVariablesReconciledCondition)

	// ccVariablesReconciledFalse with VariablesReconciled=False.
	ccVariablesReconciledFalse := baseClusterClass.DeepCopy().Build()
	conditions.MarkFalse(ccGenerationMismatch, clusterv1.ClusterClassVariablesReconciledCondition, "", clusterv1.ConditionSeverityError, "")

	tests := []struct {
		name           string
		cluster        *clusterv1.Cluster
		oldCluster     *clusterv1.Cluster
		clusterClasses []*clusterv1.ClusterClass
		wantErr        bool
	}{
		{
			name:           "Pass on create if ClusterClass is fully reconciled",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled},
			wantErr:        false,
		},
		{
			name:           "Pass on create if ClusterClass generation does not match observedGeneration",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccGenerationMismatch},
			wantErr:        false,
		},
		{
			name:           "Pass on create if ClusterClass generation matches observedGeneration but VariablesReconciled=False",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccVariablesReconciledFalse},
			wantErr:        false,
		},
		{
			name:           "Pass on create if ClusterClass is not found",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(notFoundTopology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled},
			wantErr:        false,
		},
		{
			name:           "Pass on update if oldCluster ClusterClass is fully reconciled",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(secondTopology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled, secondFullyReconciled},
			wantErr:        false,
		},
		{
			name:           "Fail on update if oldCluster ClusterClass generation does not match observedGeneration",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(secondTopology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccGenerationMismatch, secondFullyReconciled},
			wantErr:        true,
		},
		{
			name:           "Fail on update if old Cluster ClusterClass is not found",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(notFoundTopology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled},
			wantErr:        true,
		},
		{
			name:           "Fail on update if new Cluster ClusterClass is not found",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(notFoundTopology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Sets up a reconcile with a fakeClient for the test case.
			objs := []client.Object{}
			for _, cc := range tt.clusterClasses {
				objs = append(objs, cc)
			}
			c := &Cluster{Client: fake.NewClientBuilder().
				WithObjects(objs...).
				WithScheme(fakeScheme).
				Build()}

			// Checks the return error.
			if tt.wantErr {
				g.Expect(c.validate(ctx, tt.oldCluster, tt.cluster)).NotTo(Succeed())
			} else {
				g.Expect(c.validate(ctx, tt.oldCluster, tt.cluster)).To(Succeed())
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
