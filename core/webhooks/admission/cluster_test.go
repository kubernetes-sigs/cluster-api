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

package admission

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blang/semver/v4"
	. "github.com/onsi/gomega"
	pkgerrors "github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	controlplanev1 "sigs.k8s.io/cluster-api/api/controlplane/kubeadm/v1beta2"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	"sigs.k8s.io/cluster-api/core/webhooks/admission/testutil"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/test/builder"
)

func TestClusterTopologyDefaultNamespaces(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	g := NewWithT(t)

	c := builder.Cluster("fooboo", "cluster1").
		WithTopology(builder.ClusterTopology().
			WithClass("foo").
			WithVersion("v1.19.1").
			WithMachineDeployment(
				builder.MachineDeploymentTopology("md1").
					WithClass("aa").
					Build()).
			Build()).
		Build()

	clusterClass := builder.ClusterClass("fooboo", "foo").
		WithControlPlaneInfrastructureMachineTemplate(&unstructured.Unstructured{}).
		WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("aa").Build()).
		Build()
	conditions.Set(clusterClass, metav1.Condition{
		Type:   clusterv1.ClusterClassVariablesReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassVariablesReadyReason,
	})
	// Sets up the fakeClient for the test case. This is required because the test uses a Managed Topology.
	fakeClient := fake.NewClientBuilder().
		WithObjects(clusterClass).
		WithScheme(fakeScheme).
		Build()

	// Create the webhook and add the fakeClient as its client.
	webhook := &Cluster{Client: fakeClient}
	t.Run("for Cluster", testutil.CustomDefaultValidateTest[*clusterv1.Cluster](ctx, c, webhook))

	g.Expect(webhook.Default(ctx, c)).To(Succeed())
}

// TestClusterDefaultAndValidateVariables cases where cluster.spec.topology.class is altered.
func TestClusterDefaultAndValidateVariables(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	tests := []struct {
		name           string
		clusterClass   *clusterv1.ClusterClass
		topology       *clusterv1.Topology
		oldTopology    *clusterv1.Topology
		expect         *clusterv1.Topology
		wantErr        bool
		wantErrMessage string
	}{
		{
			name: "default a single variable to its correct values",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
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
			name: "should pass with empty oldTopology",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				Build(),
			topology:    &clusterv1.Topology{},
			oldTopology: &clusterv1.Topology{},
			expect: &clusterv1.Topology{
				ClassRef: clusterv1.ClusterClassRef{
					Name: "class1",
				},
				Version:   "v1.22.2",
				Variables: []clusterv1.ClusterVariable{},
			},
		},
		{
			name: "don't change a variable if it is already set",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
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
								Required: ptr.To(true),
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
								Required: ptr.To(true),
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("default-worker").Build(),
				).
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "location",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
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
				ControlPlane: clusterv1.ControlPlaneTopology{},
				Workers: clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
						},
					},
					MachinePools: []clusterv1.MachinePoolTopology{
						{
							Class: "default-worker",
							Name:  "mp-1",
						},
					},
				},
			},
			expect: &clusterv1.Topology{
				ControlPlane: clusterv1.ControlPlaneTopology{
					// "location" has not been added to .variables.overrides.
				},
				Workers: clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							// "location" has not been added to .variables.overrides.
						},
					},
					MachinePools: []clusterv1.MachinePoolTopology{
						{
							Class: "default-worker",
							Name:  "mp-1",
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("default-worker").Build(),
				).
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "httpProxy",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
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
				ControlPlane: clusterv1.ControlPlaneTopology{
					Variables: clusterv1.ControlPlaneVariables{
						Overrides: []clusterv1.ClusterVariable{
							{
								Name:  "httpProxy",
								Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)},
							},
						},
					},
				},
				Workers: clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							Variables: clusterv1.MachineDeploymentVariables{
								Overrides: []clusterv1.ClusterVariable{
									{
										Name:  "httpProxy",
										Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true}`)},
									},
								},
							},
						},
					},
					MachinePools: []clusterv1.MachinePoolTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							Variables: clusterv1.MachinePoolVariables{
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
				ControlPlane: clusterv1.ControlPlaneTopology{
					Variables: clusterv1.ControlPlaneVariables{
						Overrides: []clusterv1.ClusterVariable{
							{
								Name: "httpProxy",
								// url has been added by defaulting.
								Value: apiextensionsv1.JSON{Raw: []byte(`{"enabled":true,"url":"http://localhost:3128"}`)},
							},
						},
					},
				},
				Workers: clusterv1.WorkersTopology{
					MachineDeployments: []clusterv1.MachineDeploymentTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							Variables: clusterv1.MachineDeploymentVariables{
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
					MachinePools: []clusterv1.MachinePoolTopology{
						{
							Class: "default-worker",
							Name:  "md-1",
							Variables: clusterv1.MachinePoolVariables{
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
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
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
			name: "Keep one value if variable has multiple non-conflicting definitions",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name:                "location",
					DefinitionsConflict: ptr.To(false),
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"us-east"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
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
					},
				},
			},
		},
		{
			name: "Should fail if variable has conflicting definitions",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name:                "location",
					DefinitionsConflict: ptr.To(true),
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"first-region"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"another-region"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
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
			wantErr:  true,
			wantErrMessage: "Cluster.cluster.x-k8s.io \"cluster1\" is invalid: " +
				"spec.topology.variables: Invalid value: \"[Name: location]\": " +
				"variable definitions in the ClusterClass not valid: " +
				"variable \"location\" has conflicting definitions",
		},
		{
			name: "Should fail if variable is set and has conflicting definitions",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name:                "location",
					DefinitionsConflict: ptr.To(true),
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"first-region"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
							From:     "somepatch",
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type:    "string",
									Default: &apiextensionsv1.JSON{Raw: []byte(`"another-region"`)},
								},
							},
						},
						{
							Required: ptr.To(true),
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
					},
				},
			},
			wantErr: true,
			wantErrMessage: "Cluster.cluster.x-k8s.io \"cluster1\" is invalid: " +
				"spec.topology.variables: Invalid value: \"[Name: location]\": " +
				"variable definitions in the ClusterClass not valid: " +
				"variable \"location\" has conflicting definitions",
		},
		// Testing validation of variables.
		{
			name: "should fail when required variable is missing top-level",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().Build(),
			expect:   builder.ClusterTopology().Build(),
			wantErr:  true,
		},
		{
			name: "should fail when top-level variable is invalid",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				},
				).Build(),
			topology: builder.ClusterTopology().
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`"text"`)},
				}).
				Build(),
			expect:  builder.ClusterTopology().Build(),
			wantErr: true,
		},
		{
			name: "should fail when ControlPlane variable override is invalid",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name: "cpu",
					// This value is invalid.
					Value: apiextensionsv1.JSON{Raw: []byte(`"text"`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("aa").
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("aa").
					Build()).
				Build(),
			expect:  builder.ClusterTopology().Build(),
			wantErr: true,
		},
		{
			name: "should fail when MD variable override is invalid",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("aa").
					WithVariables(clusterv1.ClusterVariable{
						Name: "cpu",
						// This value is invalid.
						Value: apiextensionsv1.JSON{Raw: []byte(`"text"`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("aa").
					Build()).
				Build(),
			expect:  builder.ClusterTopology().Build(),
			wantErr: true,
		},
		{
			name: "should fail when MP variable override is invalid",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("aa").
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("aa").
					WithVariables(clusterv1.ClusterVariable{
						Name: "cpu",
						// This value is invalid.
						Value: apiextensionsv1.JSON{Raw: []byte(`"text"`)},
					}).
					Build()).
				Build(),
			expect:  builder.ClusterTopology().Build(),
			wantErr: true,
		},
		{
			name: "should pass when required variable exists top-level",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				// Variable is not required in ControlPlane, MachineDeployment or MachinePool topologies.
				Build(),
			expect: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				// Variable is not required in ControlPlane, MachineDeployment or MachinePool topologies.
				Build(),
		},
		{
			name: "should pass when top-level variable and override are valid",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("md1").Build()).
				WithWorkerMachinePoolClasses(*builder.MachinePoolClass("mp1").Build()).
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							Required: ptr.To(true),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				Build(),
			expect: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				Build(),
		},
		{
			name: "should pass even when variable override is missing the corresponding top-level variable",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("md1").Build()).
				WithWorkerMachinePoolClasses(*builder.MachinePoolClass("mp1").Build()).
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							// For optional variables, it is optional to set top-level variables
							// but overrides can be set even if the top-level variables are not set.
							Required: ptr.To(false),
							From:     clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				Build(),
			expect: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables([]clusterv1.ClusterVariable{}...).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`2`)},
					}).
					Build()).
				Build(),
		},
		// Testing validation of variables with CEL.
		{
			name: "should pass when CEL transition rules are skipped",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("md1").Build()).
				WithWorkerMachinePoolClasses(*builder.MachinePoolClass("mp1").Build()).
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self > oldSelf",
									}},
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-10`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-11`)},
					}).
					Build()).
				Build(),
			expect: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-10`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-11`)},
					}).
					Build()).
				Build(),
		},
		{
			name: "should pass when CEL transition rules are running",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("md1").Build()).
				WithWorkerMachinePoolClasses(*builder.MachinePoolClass("mp1").Build()).
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self > oldSelf",
									}},
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-10`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-11`)},
					}).
					Build()).
				Build(),
			oldTopology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-6`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-6`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-11`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-12`)},
					}).
					Build()).
				Build(),
			expect: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-10`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-11`)},
					}).
					Build()).
				Build(),
		},
		{
			name: "should fail when CEL transition rules are running and failing",
			clusterClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("md1").Build()).
				WithWorkerMachinePoolClasses(*builder.MachinePoolClass("mp1").Build()).
				WithStatusVariables(clusterv1.ClusterClassStatusVariable{
					Name: "cpu",
					Definitions: []clusterv1.ClusterClassStatusVariableDefinition{
						{
							From: clusterv1.VariableDefinitionFromInline,
							Schema: clusterv1.VariableSchema{
								OpenAPIV3Schema: clusterv1.JSONSchemaProps{
									Type: "integer",
									XValidations: []clusterv1.ValidationRule{{
										Rule: "self > oldSelf",
									}},
								},
							},
						},
					},
				}).Build(),
			topology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-5`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`-4`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-10`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`-11`)},
					}).
					Build()).
				Build(),
			oldTopology: builder.ClusterTopology().
				WithClass("foo").
				WithVersion("v1.19.1").
				WithVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`0`)},
				}).
				WithControlPlaneVariables(clusterv1.ClusterVariable{
					Name:  "cpu",
					Value: apiextensionsv1.JSON{Raw: []byte(`0`)},
				}).
				WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
					WithClass("md1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`0`)},
					}).
					Build()).
				WithMachinePool(builder.MachinePoolTopology("workers1").
					WithClass("mp1").
					WithVariables(clusterv1.ClusterVariable{
						Name:  "cpu",
						Value: apiextensionsv1.JSON{Raw: []byte(`0`)},
					}).
					Build()).
				Build(),
			wantErr: true,
			wantErrMessage: "Cluster.cluster.x-k8s.io \"cluster1\" is invalid: [" +
				"spec.topology.variables[cpu].value: Invalid value: \"-5\": failed rule: self > oldSelf, " +
				"spec.topology.controlPlane.variables.overrides[cpu].value: Invalid value: \"-4\": failed rule: self > oldSelf, " +
				"spec.topology.workers.machineDeployments[workers1].variables.overrides[cpu].value: Invalid value: \"-10\": failed rule: self > oldSelf, " +
				"spec.topology.workers.machinePools[workers1].variables.overrides[cpu].value: Invalid value: \"-11\": failed rule: self > oldSelf]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setting Class and Version here to avoid obfuscating the test cases above.
			tt.topology.ClassRef.Name = "class1"
			tt.topology.Version = "v1.22.2"
			if tt.expect != nil {
				tt.expect.ClassRef.Name = "class1"
				tt.expect.Version = "v1.22.2"
			}

			cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(tt.topology).
				Build()

			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.Set(tt.clusterClass, metav1.Condition{
				Type:   clusterv1.ClusterClassVariablesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterClassVariablesReadyReason,
			})
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.clusterClass).
				WithScheme(fakeScheme).
				Build()
			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			webhook := &Cluster{Client: fakeClient, decoder: admission.NewDecoder(fakeScheme)}

			// Test defaulting.
			t.Run("default", func(t *testing.T) {
				g := NewWithT(t)

				// Add old cluster to request if oldTopology is set.
				webhookCtx := ctx
				if tt.oldTopology != nil {
					oldCluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
						WithTopology(tt.oldTopology).
						Build()
					jsonObj, err := json.Marshal(oldCluster)
					g.Expect(err).ToNot(HaveOccurred())

					webhookCtx = admission.NewContextWithRequest(ctx, admission.Request{
						AdmissionRequest: admissionv1.AdmissionRequest{
							Operation: admissionv1.Update,
							OldObject: runtime.RawExtension{
								Raw:    jsonObj,
								Object: oldCluster,
							},
						},
					})
				}

				err := webhook.Default(webhookCtx, cluster)
				if tt.wantErr {
					g.Expect(err).To(HaveOccurred())
					if tt.wantErrMessage != "" {
						g.Expect(err.Error()).To(ContainSubstring(tt.wantErrMessage))
					}
					return
				}
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(cluster.Spec.Topology).To(BeComparableTo(*tt.expect))
			})

			// Test if defaulting works in combination with validation.
			// Note this test is not run for the case where the webhook should fail.
			if tt.wantErr {
				t.Skip("skipping test for combination of defaulting and validation (not supported by the test)")
			}
			testutil.CustomDefaultValidateTest[*clusterv1.Cluster](ctx, cluster, webhook)(t)
		})
	}
}

func TestClusterDefaultTopologyVersion(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	g := NewWithT(t)

	c := builder.Cluster("fooboo", "cluster1").
		WithTopology(builder.ClusterTopology().
			WithClass("foo").
			WithVersion("1.19.1").
			Build()).
		Build()

	clusterClass := builder.ClusterClass("fooboo", "foo").Build()
	conditions.Set(clusterClass, metav1.Condition{
		Type:   clusterv1.ClusterClassVariablesReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassVariablesReadyReason,
	})
	// Sets up the fakeClient for the test case. This is required because the test uses a Managed Topology.
	fakeClient := fake.NewClientBuilder().
		WithObjects(clusterClass).
		WithScheme(fakeScheme).
		Build()

	// Create the webhook and add the fakeClient as its client.
	webhook := &Cluster{Client: fakeClient}
	t.Run("for Cluster", testutil.CustomDefaultValidateTest[*clusterv1.Cluster](ctx, c, webhook))

	g.Expect(webhook.Default(ctx, c)).To(Succeed())

	g.Expect(c.Spec.Topology.Version).To(HavePrefix("v"))
}

func TestClusterFailOnMissingClassField(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	g := NewWithT(t)

	c := builder.Cluster("fooboo", "cluster1").
		WithTopology(builder.ClusterTopology().
			WithClass(""). // THis is invalid.
			WithVersion("1.19.1").
			Build()).
		Build()

	g.Expect((&Cluster{}).Default(ctx, c)).ToNot(Succeed())
}

func TestClusterValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.

	tests := []struct {
		name         string
		in           *clusterv1.Cluster
		old          *clusterv1.Cluster
		expectErr    bool
		expectErrStr string
	}{
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
			name:         "fails if topology is set but feature flag is disabled",
			expectErr:    true,
			expectErrStr: "spec.topology: Forbidden: can be set only if the ClusterTopology feature flag is enabled",
			in: builder.Cluster("fooNamespace", "cluster1").
				WithInfrastructureCluster(
					builder.InfrastructureClusterTemplate("fooNamespace", "infra1").Build()).
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithTopology(&clusterv1.Topology{
					ClassRef: clusterv1.ClusterClassRef{
						Name: "class",
					},
				}).
				Build(),
		},
		{
			name:         "fails if none of spec.controlPlaneRef, spec.infrastructureRef or spec.topology is set",
			expectErr:    true,
			expectErrStr: "spec: Forbidden: one of spec.controlPlaneRef, spec.infrastructureRef or spec.topology must be set",
			in: builder.Cluster("fooNamespace", "cluster1").
				Build(),
		},
		{
			name:      "pass with undefined CIDR ranges",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{},
					},
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: []string{},
					},
				}).
				Build(),
		},
		{
			name:      "pass with nil CIDR ranges",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: nil,
					},
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: nil,
					},
				}).
				Build(),
		},
		{
			name:      "pass with valid IPv4 CIDR ranges",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"10.10.10.10/24"},
					},
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"10.10.10.10/24"},
					},
				}).
				Build(),
		},
		{
			name:      "pass with valid IPv6 CIDR ranges",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64"},
					},
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64"},
					},
				}).
				Build(),
		},
		{
			name:      "pass with valid dualstack CIDR ranges",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64", "10.10.10.10/24"},
					},
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"2004::1234:abcd:ffff:c0a8:101/64", "10.10.10.10/24"},
					},
				}).
				Build(),
		},
		{
			name:      "pass if multiple CIDR ranges of IPv4 are passed",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"10.10.10.10/24", "11.11.11.11/24"},
					},
				}).
				Build(),
		},
		{
			name:      "pass if multiple CIDR ranges of IPv6 are passed",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"2002::1234:abcd:ffff:c0a8:101/64", "2004::1234:abcd:ffff:c0a8:101/64"},
					},
				}).
				Build(),
		},
		{
			name:      "pass if too many cidr ranges are specified in the clusterNetwork pods field",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Pods: clusterv1.NetworkRanges{
						CIDRBlocks: []string{"10.10.10.10/24", "11.11.11.11/24", "12.12.12.12/24"},
					},
				}).
				Build(),
		},
		{
			name:         "fails if service cidr ranges are not valid",
			expectErr:    true,
			expectErrStr: "[spec.clusterNetwork.services.cidrBlocks[0]: Invalid value: \"10.10.10.10\": invalid CIDR address: 10.10.10.10, spec.clusterNetwork.services.cidrBlocks[1]: Invalid value: \"11.11.11.11\": invalid CIDR address: 11.11.11.11]",
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Services: clusterv1.NetworkRanges{
						// Invalid ranges: missing network suffix
						CIDRBlocks: []string{"10.10.10.10", "11.11.11.11"},
					},
				}).
				Build(),
		},
		{
			name:         "fails if pod cidr ranges are not valid",
			expectErr:    true,
			expectErrStr: "[spec.clusterNetwork.pods.cidrBlocks[0]: Invalid value: \"10.10.10.10\": invalid CIDR address: 10.10.10.10, spec.clusterNetwork.pods.cidrBlocks[1]: Invalid value: \"11.11.11.11\": invalid CIDR address: 11.11.11.11]",
			in: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				WithClusterNetwork(clusterv1.ClusterNetwork{
					Pods: clusterv1.NetworkRanges{
						// Invalid ranges: missing network suffix
						CIDRBlocks: []string{"10.10.10.10", "11.11.11.11"},
					},
				}).
				Build(),
		},
		{
			name:      "pass with name of under 63 characters",
			expectErr: false,
			in: builder.Cluster("fooNamespace", "short-name").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				Build(),
		},
		{
			name: "pass with _, -, . characters in name",
			in: builder.Cluster("fooNamespace", "thisNameContains.A_Non-Alphanumeric").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				Build(),
			expectErr: false,
		},
		{
			name: "fails if cluster name is longer than 63 characters",
			in: builder.Cluster("fooNamespace", "thisNameIsReallyMuchLongerThanTheMaximumLengthOfSixtyThreeCharacters").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				Build(),
			expectErr:    true,
			expectErrStr: "must be a valid label value must be no more than 63 bytes",
		},
		{
			name: "error when name starts with NonAlphanumeric character",
			in: builder.Cluster("fooNamespace", "-thisNameStartsWithANonAlphanumeric").WithControlPlane(
				builder.ControlPlane("fooNamespace", "cp1").Build()).
				Build(),
			expectErr:    true,
			expectErrStr: "must be a valid label value a valid label must be an empty string or consist of alphanumeric characters",
		},
		{
			name: "error when name ends with NonAlphanumeric character",
			in: builder.Cluster("fooNamespace", "thisNameEndsWithANonAlphanumeric.").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				Build(),
			expectErr:    true,
			expectErrStr: "must be a valid label value a valid label must be an empty string or consist of alphanumeric characters",
		},
		{
			name: "error when name contains invalid NonAlphanumeric character",
			in: builder.Cluster("fooNamespace", "thisNameContainsInvalid!@NonAlphanumerics").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				Build(),
			expectErr:    true,
			expectErrStr: "must be a valid label value a valid label must be an empty string or consist of alphanumeric characters",
		},
		{
			name: "error when controlPlaneRef gets unset",
			in: builder.Cluster("fooNamespace", "cluster1").
				Build(),
			old: builder.Cluster("fooNamespace", "cluster1").
				WithControlPlane(
					builder.ControlPlane("fooNamespace", "cp1").Build()).
				Build(),
			expectErr:    true,
			expectErrStr: "spec: Forbidden: one of spec.controlPlaneRef, spec.infrastructureRef or spec.topology must be set, spec.controlPlaneRef: Forbidden: cannot be removed",
		},
		{
			name: "error when infrastructureRef gets unset",
			in: builder.Cluster("fooNamespace", "cluster1").
				Build(),
			old: builder.Cluster("fooNamespace", "cluster1").
				WithInfrastructureCluster(
					builder.InfrastructureClusterTemplate("fooNamespace", "infra1").Build()).
				Build(),
			expectErr:    true,
			expectErrStr: "spec.infrastructureRef: Forbidden: cannot be removed, spec: Forbidden: one of spec.controlPlaneRef, spec.infrastructureRef or spec.topology must be set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create the webhook.
			webhook := &Cluster{}

			warnings, err := webhook.validate(ctx, tt.old, tt.in)
			g.Expect(warnings).To(BeEmpty())
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(tt.expectErrStr).ToNot(BeEmpty())
				g.Expect(err.Error()).To(ContainSubstring(tt.expectErrStr))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestClusterTopologyValidation(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to set Cluster.Topologies.
	// Enabling the feature flag temporarily for this test.
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopologyWorkerVersionPinning, true)

	tests := []struct {
		name                         string
		in                           *clusterv1.Cluster
		old                          *clusterv1.Cluster
		additionalObjects            []client.Object
		clusterClassVersions         []string
		generateUpgradePlanExtension string
		expectErr                    bool
		expectWarning                bool
	}{
		{
			// The skew guard must also run on create, not only when spec.topology.version changes.
			name:      "should reject a Cluster created with a pinned MachineDeployment outside the version skew policy",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.35.0").
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").WithClass("aa").WithVersion("v1.31.0").Build()).
					Build()).
				Build(),
		},
		{
			name:      "should accept a Cluster created with a pinned MachineDeployment within the version skew policy",
			expectErr: false,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.34.0").
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").WithClass("aa").WithVersion("v1.31.0").Build()).
					Build()).
				Build(),
		},
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
			name:      "should return error when topology does not have valid version",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("1.17.2").Build()).
				Build(),
		},
		{
			name:      "should return error when topology does not have valid version",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1").Build()).
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
			name:      "should pass when changing build tag - not sortable",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3+ANCBG0").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3+ANCBG1").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				// Note: CRD is needed to look up the apiVersion from contract labels.
				builder.GenericControlPlaneCRD,
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.2.3+ANCBG0").
					WithStatusFields(map[string]interface{}{"status.version": "v1.2.3+ANCBG0"}).
					Build(),
			},
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
			name:      "fails when kubernetes version are defined in CC and version does not match (on create)",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.3.2").
					Build()).
				Build(),
			clusterClassVersions: []string{"v1.2.3", "v1.3.1", "v1.4.0"},
		},
		{
			name:      "fails when kubernetes version are defined in CC and version does not match",
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
					WithVersion("v1.3.2").
					Build()).
				Build(),
			clusterClassVersions: []string{"v1.2.3", "v1.3.1", "v1.4.0"},
		},
		{
			name:      "fails when upgrading but the AfterClusterUpgrade is still pending",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithAnnotations(map[string]string{
					runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
				}).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithAnnotations(map[string]string{
					runtimev1.PendingHooksAnnotation: "AfterClusterUpgrade",
				}).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.3.2").
					Build()).
				Build(),
		},
		{
			name: "should allow upgrading >1 minor version when kubernetes version are defined in CC",
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.4.0").
					Build()).
				Build(),
			clusterClassVersions: []string{"v1.2.3", "v1.3.1", "v1.4.0"},
			additionalObjects: []client.Object{
				// Note: CRD is needed to look up the apiVersion from contract labels.
				builder.GenericControlPlaneCRD,
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.2.3").
					WithStatusFields(map[string]interface{}{"status.version": "v1.2.3"}).
					Build(),
			},
		},
		{
			name: "should allow upgrading >1 minor version when kubernetes version are defined in CC - with build tags",
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3+ANCBG0").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.4.0+BXCBG0").
					Build()).
				Build(),
			clusterClassVersions: []string{"v1.2.3+ANCBG0", "v1.3.1+QPAVG0", "v1.4.0+BXCBG0"},
			additionalObjects: []client.Object{
				// Note: CRD is needed to look up the apiVersion from contract labels.
				builder.GenericControlPlaneCRD,
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.2.3+ANCBG0").
					WithStatusFields(map[string]interface{}{"status.version": "v1.2.3+ANCBG0"}).
					Build(),
			},
		},
		{
			name: "should allow upgrading >1 minor version when generateUpgradePlan extension is defined in CC",
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.2.3").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.4.0").
					Build()).
				Build(),
			generateUpgradePlanExtension: "foo",
			additionalObjects: []client.Object{
				// Note: CRD is needed to look up the apiVersion from contract labels.
				builder.GenericControlPlaneCRD,
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.2.3").
					WithStatusFields(map[string]interface{}{"status.version": "v1.2.3"}).
					Build(),
			},
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
			name:      "should return error when duplicated MachinePools names exists in a Topology",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachinePool(
						builder.MachinePoolTopology("workers1").
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
			name:      "should pass when MachinePools names in a Topology are unique",
			expectErr: false,
			in: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachinePool(
						builder.MachinePoolTopology("workers2").
							WithClass("bb").
							Build()).
					Build()).
				Build(),
		},
		{
			name:      "should update",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
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
					WithMachinePool(
						builder.MachinePoolTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachinePool(
						builder.MachinePoolTopology("workers2").
							WithClass("bb").
							Build()).
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
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
					WithMachinePool(
						builder.MachinePoolTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachinePool(
						builder.MachinePoolTopology("workers2").
							WithClass("bb").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.19.1").
					WithStatusFields(map[string]interface{}{"status.version": "v1.19.1"}).
					Build(),
				// Note: CRD is needed to look up the apiVersion from contract labels.
				builder.GenericControlPlaneCRD,
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.19.1").Build(),
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.19.1").Build(),
			},
		},
		{
			name:      "should return error when upgrade concurrency annotation value is < 1",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithAnnotations(map[string]string{
					clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation: "-1",
				}).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.2").
					Build()).
				Build(),
		},
		{
			name:      "should return error when upgrade concurrency annotation value is not numeric",
			expectErr: true,
			in: builder.Cluster("fooboo", "cluster1").
				WithAnnotations(map[string]string{
					clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation: "abc",
				}).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.2").
					Build()).
				Build(),
		},
		{
			name:      "should pass upgrade concurrency annotation value is >= 1",
			expectErr: false,
			in: builder.Cluster("fooboo", "cluster1").
				WithAnnotations(map[string]string{
					clusterv1.ClusterTopologyUpgradeConcurrencyAnnotation: "2",
				}).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.2").
					Build()).
				Build(),
		},
		{
			name:      "should update if cluster is fully upgraded and up to date",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.20.2").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.19.1").
					WithStatusFields(map[string]interface{}{"status.version": "v1.19.1"}).
					Build(),
				// Note: CRD is needed to look up the apiVersion from contract labels.
				builder.GenericControlPlaneCRD,
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.19.1").Build(),
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.19.1").Build(),
			},
		},
		{
			name:          "should skip validation if cluster kcp is not yet provisioned but annotation is set",
			expectErr:     false,
			expectWarning: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithAnnotations(map[string]string{clusterv1.ClusterTopologyUnsafeUpdateVersionAnnotation: "true"}).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.20.2").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.18.1").Build(),
			},
		},
		{
			name:      "should block update if cluster kcp is not yet provisioned",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.20.2").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.18.1").Build(),
			},
		},
		{
			name:      "should block update if md is not yet upgraded",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.20.2").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.19.1").
					WithStatusFields(map[string]interface{}{"status.version": "v1.19.1"}).
					Build(),
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.18.1").Build(),
			},
		},
		{
			name:      "should block update if mp is not yet upgraded",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			in: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.20.2").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.19.1").
					WithStatusFields(map[string]interface{}{"status.version": "v1.19.1"}).
					Build(),
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.18.1").Build(),
			},
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("bb").Build(),
					*builder.MachinePoolClass("aa").Build(),
				).
				Build()

			if tt.clusterClassVersions != nil {
				class.Spec.KubernetesVersions = tt.clusterClassVersions
			}
			class.Spec.Upgrade.External.GenerateUpgradePlanExtension = tt.generateUpgradePlanExtension

			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.Set(class, metav1.Condition{
				Type:   clusterv1.ClusterClassVariablesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterClassVariablesReadyReason,
			})
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(class).
				WithObjects(tt.additionalObjects...).
				WithScheme(fakeScheme).
				Build()

			// Use an empty fakeClusterCache here because the real cases are tested in Test_validateTopologyMachinePoolVersions.
			fakeClusterCacheReader := &fakeClusterCache{client: fake.NewFakeClient()}

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			webhook := &Cluster{Client: fakeClient, ClusterCacheReader: fakeClusterCacheReader}

			warnings, err := webhook.validate(ctx, tt.old, tt.in)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			if tt.expectWarning {
				g.Expect(warnings).ToNot(BeEmpty())
			} else {
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

// TestClusterTopologyValidationWithClient tests the additional cases introduced in new validation in the webhook package.
func TestClusterTopologyValidationWithClient(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)

	tests := []struct {
		name            string
		cluster         *clusterv1.Cluster
		class           *clusterv1.ClusterClass
		classReconciled bool
		objects         []client.Object
		wantErr         bool
		wantWarnings    bool
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
			classReconciled: true,
			wantErr:         false,
		},
		{
			name: "Warning for a cluster with non-existent ClusterClass referenced cluster.spec.topology.class",
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
			// There should be a warning for a ClusterClass which can not be found.
			wantWarnings: true,
			wantErr:      false,
		},
		{
			name: "Warning for a cluster with an unreconciled ClusterClass named in cluster.spec.topology.class",
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
			classReconciled: false,
			// There should be a warning for a ClusterClass which is not yet reconciled.
			wantWarnings: true,
			wantErr:      false,
		},
		{
			name: "Reject a cluster that has MHC enabled for control plane but is missing MHC definition in cluster topology and clusterclass",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
							Enabled: ptr.To(true),
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				Build(),
			classReconciled: true,
			wantErr:         true,
		},
		{
			name: "Accept a cluster that has MHC override defined for control plane and does not set unhealthy conditions",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
							Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
								UnhealthyNodeConditions:    []clusterv1.UnhealthyNodeCondition{},
								UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{},
								NodeStartupTimeoutSeconds:  ptr.To(int32(30)),
							},
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithControlPlaneInfrastructureMachineTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinframachinetemplate").Build()).
				Build(),
			classReconciled: true,
			wantErr:         false,
		},
		{
			name: "Reject a cluster that MHC override defined for control plane but is set when control plane is missing machineInfrastructure",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
							Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:   corev1.NodeReady,
										Status: corev1.ConditionFalse,
									},
								},
								UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
									{
										Type:   controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
										Status: metav1.ConditionFalse,
									},
								},
							},
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				Build(),
			classReconciled: true,
			wantErr:         true,
		},
		{
			name: "Accept a cluster that has MHC enabled for control plane with control plane MHC defined in ClusterClass",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
							Enabled: ptr.To(true),
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneClassHealthCheck{
					Checks: clusterv1.ControlPlaneClassHealthCheckChecks{
						UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
							{
								Type:           corev1.NodeReady,
								Status:         corev1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
						UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
							{
								Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
								Status:         metav1.ConditionUnknown,
								TimeoutSeconds: ptr.To(int32(5 * 60)),
							},
						},
					},
				}).
				Build(),
			classReconciled: true,
			wantErr:         false,
		},
		{
			name: "Accept a cluster that has MHC enabled for control plane with control plane MHC defined in cluster topology",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithControlPlaneMachineHealthCheck(clusterv1.ControlPlaneTopologyHealthCheck{
							Enabled: ptr.To(true),
							Checks: clusterv1.ControlPlaneTopologyHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:   corev1.NodeReady,
										Status: corev1.ConditionFalse,
									},
								},
								UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
									{
										Type:   controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
										Status: metav1.ConditionFalse,
									},
								},
							},
						}).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithControlPlaneInfrastructureMachineTemplate(builder.InfrastructureMachineTemplate(metav1.NamespaceDefault, "cpinframachinetemplate").Build()).
				Build(),
			classReconciled: true,
			wantErr:         false,
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
								WithMachineHealthCheck(clusterv1.MachineDeploymentTopologyHealthCheck{
									Enabled: ptr.To(true),
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
			classReconciled: true,
			wantErr:         true,
		},
		{
			name: "Accept a cluster that has MHC override defined for machine deployment and does not set unhealthy conditions",
			cluster: builder.Cluster(metav1.NamespaceDefault, "cluster1").
				WithTopology(
					builder.ClusterTopology().
						WithClass("clusterclass").
						WithVersion("v1.22.2").
						WithControlPlaneReplicas(3).
						WithMachineDeployment(
							builder.MachineDeploymentTopology("md1").
								WithClass("worker-class").
								WithMachineHealthCheck(clusterv1.MachineDeploymentTopologyHealthCheck{
									Checks: clusterv1.MachineDeploymentTopologyHealthCheckChecks{
										UnhealthyNodeConditions:    []clusterv1.UnhealthyNodeCondition{},
										UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{},
										NodeStartupTimeoutSeconds:  ptr.To(int32(30)),
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
			classReconciled: true,
			wantErr:         false,
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
								WithMachineHealthCheck(clusterv1.MachineDeploymentTopologyHealthCheck{
									Enabled: ptr.To(true),
								}).
								Build(),
						).
						Build()).
				Build(),
			class: builder.ClusterClass(metav1.NamespaceDefault, "clusterclass").
				WithWorkerMachineDeploymentClasses(
					*builder.MachineDeploymentClass("worker-class").
						WithMachineHealthCheckClass(clusterv1.MachineDeploymentClassHealthCheck{
							Checks: clusterv1.MachineDeploymentClassHealthCheckChecks{
								UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
									{
										Type:           corev1.NodeReady,
										Status:         corev1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
								UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
									{
										Type:           controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
										Status:         metav1.ConditionUnknown,
										TimeoutSeconds: ptr.To(int32(5 * 60)),
									},
								},
							},
						}).
						Build(),
				).
				Build(),
			classReconciled: true,
			wantErr:         false,
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
								WithMachineHealthCheck(clusterv1.MachineDeploymentTopologyHealthCheck{
									Enabled: ptr.To(true),
									Checks: clusterv1.MachineDeploymentTopologyHealthCheckChecks{
										UnhealthyNodeConditions: []clusterv1.UnhealthyNodeCondition{
											{
												Type:   corev1.NodeReady,
												Status: corev1.ConditionFalse,
											},
										},
										UnhealthyMachineConditions: []clusterv1.UnhealthyMachineCondition{
											{
												Type:   controlplanev1.KubeadmControlPlaneMachineEtcdPodHealthyCondition,
												Status: metav1.ConditionFalse,
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
			classReconciled: true,
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			if tt.classReconciled {
				conditions.Set(tt.class, metav1.Condition{
					Type:   clusterv1.ClusterClassVariablesReadyCondition,
					Status: metav1.ConditionTrue,
					Reason: clusterv1.ClusterClassVariablesReadyReason,
				})
			}
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.class).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			c := &Cluster{Client: fakeClient}

			// Checks the return error.
			warnings, err := c.ValidateCreate(ctx, tt.cluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			if tt.wantWarnings {
				g.Expect(warnings).ToNot(BeEmpty())
			} else {
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

// TestClusterTopologyValidationForTopologyClassChange cases where cluster.spec.topology.class is altered.
func TestClusterTopologyValidationForTopologyClassChange(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)

	cluster := builder.Cluster(metav1.NamespaceDefault, "cluster1").
		WithTopology(
			builder.ClusterTopology().
				WithClass("class1").
				WithVersion("v1.22.2").
				WithControlPlaneReplicas(3).
				Build()).
		Build()

	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
	}
	compatibleNameChangeRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "differentbaz",
	}
	compatibleAPIVersionChangeRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo2",
		Kind:       "barTemplate",
		Name:       "differentbaz",
	}
	incompatibleKindRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "another-barTemplate",
		Name:       "another-baz",
	}
	incompatibleAPIGroupRef := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.nottest.io/foo",
		Kind:       "barTemplate",
		Name:       "another-baz",
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

		// MachineDeploymentClass & MachinePoolClass changes
		{
			name: "Accept cluster.topology.class change with a compatible MachineDeploymentClass and MachinePoolClass InfrastructureTemplate",
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(compatibleNameChangeRef)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with an incompatible MachineDeploymentClass and MachinePoolClass BootstrapTemplate",
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(compatibleNameChangeRef)).
						WithBootstrapTemplate(refToUnstructured(incompatibleKindRef)).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with a deleted MachineDeploymentClass and MachinePoolClass",
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
					*builder.MachinePoolClass("bb").
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: false,
		},
		{
			name: "Accept cluster.topology.class change with an added MachineDeploymentClass and MachinePoolClass",
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
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
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
					*builder.MachinePoolClass("bb").
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

		// MachinePoolClass reject changes
		{
			name: "Reject cluster.topology.class change with an incompatible Kind change to MachinePoolClass InfrastructureTemplate",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(incompatibleKindRef)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: true,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible APIGroup change to MachinePoolClass InfrastructureTemplate",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(ref)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithWorkerMachinePoolClasses(
					*builder.MachinePoolClass("aa").
						WithInfrastructureTemplate(refToUnstructured(incompatibleAPIGroupRef)).
						WithBootstrapTemplate(refToUnstructured(ref)).
						Build(),
				).
				Build(),
			wantErr: true,
		},

		// Kubernetes Version changes.
		{
			name: "Accept cluster.topology.class change with a compatible Kubernetes Version",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(compatibleNameChangeRef)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithVersions("v1.22.2", "v1.23.2").
				Build(),
			wantErr: false,
		},
		{
			name: "Reject cluster.topology.class change with an incompatible Kubernetes Version",
			firstClass: builder.ClusterClass(metav1.NamespaceDefault, "class1").
				WithInfrastructureClusterTemplate(refToUnstructured(ref)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				Build(),
			secondClass: builder.ClusterClass(metav1.NamespaceDefault, "class2").
				WithInfrastructureClusterTemplate(refToUnstructured(compatibleNameChangeRef)).
				WithControlPlaneTemplate(refToUnstructured(ref)).
				WithControlPlaneInfrastructureMachineTemplate(refToUnstructured(ref)).
				WithVersions("v1.33.0", "v1.34.0").
				Build(),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.Set(tt.firstClass, metav1.Condition{
				Type:   clusterv1.ClusterClassVariablesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterClassVariablesReadyReason,
			})
			conditions.Set(tt.secondClass, metav1.Condition{
				Type:   clusterv1.ClusterClassVariablesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterClassVariablesReadyReason,
			})

			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.firstClass, tt.secondClass).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			c := &Cluster{Client: fakeClient}

			// Create and updated cluster which uses the name of the second class from the test definition in its '.spec.topology.'
			secondCluster := cluster.DeepCopy()
			secondCluster.Spec.Topology.ClassRef.Name = tt.secondClass.Name

			// Checks the return error.
			warnings, err := c.ValidateUpdate(ctx, cluster, secondCluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(warnings).To(BeEmpty())
		})
	}
}

// TestMovingBetweenManagedAndUnmanaged cluster tests cases where a clusterClass is added or removed during a cluster update.
func TestMovingBetweenManagedAndUnmanaged(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
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
		t.Run(tt.name, func(*testing.T) {
			// Mark this condition to true so the webhook sees the ClusterClass as up to date.
			conditions.Set(tt.clusterClass, metav1.Condition{
				Type:   clusterv1.ClusterClassVariablesReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: clusterv1.ClusterClassVariablesReadyReason,
			})
			// Sets up the fakeClient for the test case.
			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.clusterClass, tt.cluster).
				WithScheme(fakeScheme).
				Build()

			// Create the webhook and add the fakeClient as its client. This is required because the test uses a Managed Topology.
			c := &Cluster{Client: fakeClient}

			// Create and updated cluster which uses the name of the second class from the test definition in its '.spec.topology.'
			updatedCluster := tt.cluster.DeepCopy()
			updatedCluster.Spec.Topology = ptr.Deref(tt.updatedTopology, clusterv1.Topology{})

			// Checks the return error.
			warnings, err := c.ValidateUpdate(ctx, tt.cluster, updatedCluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				// Errors may be duplicated as warnings. There should be no warnings in this case if there are no errors.
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

// TestClusterClassPollingErrors tests when a Cluster can be reconciled given different reconcile states of the ClusterClass.
func TestClusterClassPollingErrors(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	g := NewWithT(t)
	ref := &clusterv1.ClusterClassTemplateReference{
		APIVersion: "group.test.io/foo",
		Kind:       "barTemplate",
		Name:       "baz",
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
	conditions.Set(ccFullyReconciled, metav1.Condition{
		Type:   clusterv1.ClusterClassVariablesReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassVariablesReadyReason,
	})

	// secondFullyReconciled is a second ClusterClass with a matching generation and observed generation, and VariablesReconciled=True.
	secondFullyReconciled := ccFullyReconciled.DeepCopy()
	secondFullyReconciled.SetName("class2")

	// ccGenerationMismatch is a ClusterClass with a mismatched generation and observed generation, but VariablesReconciledCondition=True.
	ccGenerationMismatch := baseClusterClass.DeepCopy().Build()
	ccGenerationMismatch.Generation = 999
	ccGenerationMismatch.Status.ObservedGeneration = 1
	conditions.Set(ccGenerationMismatch, metav1.Condition{
		Type:   clusterv1.ClusterClassVariablesReadyCondition,
		Status: metav1.ConditionTrue,
		Reason: clusterv1.ClusterClassVariablesReadyReason,
	})

	// ccVariablesReconciledFalse with VariablesReconciled=False.
	ccVariablesReconciledFalse := baseClusterClass.Build().DeepCopy()
	conditions.Set(ccVariablesReconciledFalse, metav1.Condition{
		Type:   clusterv1.ClusterClassVariablesReadyCondition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.ClusterClassVariablesReadyVariableDiscoveryFailedReason,
	})

	tests := []struct {
		name           string
		cluster        *clusterv1.Cluster
		oldCluster     *clusterv1.Cluster
		clusterClasses []*clusterv1.ClusterClass
		injectedErr    interceptor.Funcs
		wantErr        bool
		wantWarnings   bool
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
			wantWarnings:   true,
		},
		{
			name:           "Pass on create if ClusterClass generation matches observedGeneration but VariablesReconciled=False",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccVariablesReconciledFalse},
			wantErr:        false,
			wantWarnings:   true,
		},
		{
			name:           "Pass on create if ClusterClass is not found",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(notFoundTopology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled},
			wantErr:        false,
			wantWarnings:   true,
		},
		{
			name:           "Pass on update if oldCluster ClusterClass is fully reconciled",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(secondTopology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled, secondFullyReconciled},
			wantErr:        false,
		},
		{
			name:           "Pass on update if oldCluster ClusterClass generation does not match observedGeneration",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(secondTopology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccGenerationMismatch, secondFullyReconciled},
			wantErr:        false,
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
			wantWarnings:   true,
		},
		{
			name:           "Fail on update if new ClusterClass returns connection error",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(secondTopology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled, secondFullyReconciled},
			injectedErr: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					// Throw an error if the second ClusterClass `class2` used as the new ClusterClass is being retrieved.
					if key.Name == secondTopology.ClassRef.Name {
						return pkgerrors.New("connection error")
					}
					return client.Get(ctx, key, obj)
				},
			},
			wantErr:      true,
			wantWarnings: false,
		},
		{
			name:           "Fail on update if old ClusterClass returns connection error",
			cluster:        builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(secondTopology).Build(),
			oldCluster:     builder.Cluster(metav1.NamespaceDefault, "cluster1").WithTopology(topology).Build(),
			clusterClasses: []*clusterv1.ClusterClass{ccFullyReconciled, secondFullyReconciled},
			injectedErr: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
					// Throw an error if the ClusterClass `class1` used as the old ClusterClass is being retrieved.
					if key.Name == topology.ClassRef.Name {
						return pkgerrors.New("connection error")
					}
					return client.Get(ctx, key, obj)
				},
			},
			wantErr:      true,
			wantWarnings: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(*testing.T) {
			// Sets up a reconcile with a fakeClient for the test case.
			objs := []client.Object{}
			for _, cc := range tt.clusterClasses {
				objs = append(objs, cc)
			}
			c := &Cluster{
				Client: fake.NewClientBuilder().
					WithInterceptorFuncs(tt.injectedErr).
					WithScheme(fakeScheme).
					WithObjects(objs...).
					Build(),
			}

			// Checks the return error.
			warnings, err := c.validate(ctx, tt.oldCluster, tt.cluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			if tt.wantWarnings {
				g.Expect(warnings).NotTo(BeEmpty())
			} else {
				g.Expect(warnings).To(BeEmpty())
			}
		})
	}
}

func Test_validateTopologyControlPlaneVersion(t *testing.T) {
	tests := []struct {
		name              string
		expectErr         bool
		old               *clusterv1.Cluster
		additionalObjects []client.Object
	}{
		{
			name:      "should update if kcp is fully upgraded and up to date",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.19.1").
					WithStatusFields(map[string]interface{}{"status.version": "v1.19.1"}).
					Build(),
				// Note: CRD is needed to look up the apiVersion from contract labels.
				builder.GenericControlPlaneCRD,
			},
		},
		{
			name:      "should block update if kcp is provisioning",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.19.1").
					Build(),
			},
		},
		{
			name:      "should block update if kcp is upgrading",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.18.1").
					WithStatusFields(map[string]interface{}{"status.version": "v1.17.1"}).
					Build(),
			},
		},
		{
			name:      "should block update if kcp is not yet upgraded",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.ControlPlane("fooboo", "cluster1-cp").WithVersion("v1.18.1").
					WithStatusFields(map[string]interface{}{"status.version": "v1.18.1"}).
					Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.additionalObjects...).
				WithScheme(fakeScheme).
				Build()

			oldVersion, err := semver.ParseTolerant(tt.old.Spec.Topology.Version)
			g.Expect(err).ToNot(HaveOccurred())

			err = validateTopologyControlPlaneVersion(ctx, fakeClient, tt.old, oldVersion)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func Test_validateTopologyWorkerVersions(t *testing.T) {
	tests := []struct {
		name            string
		versionPinning  bool
		topologyVersion string
		mdVersion       string
		mdAnnotations   map[string]string
		mpVersion       string
		expectErrs      []string
	}{
		{
			name:            "should pass if no version is pinned",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
		},
		{
			name:            "should pass if valid versions are pinned",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "v1.33.0",
			mpVersion:       "v1.33.0",
		},
		{
			// The window is topology.version minus MaxWorkerMinorVersionSkew up to topology.version.
			name:            "should pass at the edge of the skew budget",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "v1.31.0",
			mpVersion:       "v1.31.0",
		},
		{
			// Skew is judged on major.minor, so pre-release and build metadata do not affect it.
			name:            "should ignore pre-release and build metadata when judging skew",
			versionPinning:  true,
			topologyVersion: "v1.34.0-rc.0",
			mdVersion:       "v1.31.5+vmware.1",
			mpVersion:       "v1.34.0",
		},
		{
			name:            "should reject a pinned version if the feature gate is disabled",
			versionPinning:  false,
			topologyVersion: "v1.34.0",
			mdVersion:       "v1.33.0",
			mpVersion:       "v1.33.0",
			expectErrs: []string{
				"spec.topology.workers.machineDeployments[md-0].version: Forbidden",
				"spec.topology.workers.machinePools[mp-0].version: Forbidden",
			},
		},
		{
			name:            "should reject a pinned version without a v prefix",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "1.33.0",
			expectErrs:      []string{"must start with v"},
		},
		{
			name:            "should reject a pinned version that is not a semantic version",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "vfoo",
			expectErrs:      []string{"must be a valid semantic version"},
		},
		{
			// Parsed strictly, like Cluster.spec.topology.version. A tolerantly parsed "v1.34" would
			// be admitted here and then rejected by the MachineDeployment webhook.
			name:            "should reject a pinned version that is not strict semver",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "v1.34",
			expectErrs:      []string{"must be a valid semantic version"},
		},
		{
			name:            "should reject a pin on a newer minor than the topology version",
			versionPinning:  true,
			topologyVersion: "v1.33.0",
			mdVersion:       "v1.34.0",
			expectErrs:      []string{"version cannot be greater than Cluster.spec.topology.version \"v1.33.0\""},
		},
		{
			// The skew check is minor-granular, so the ceiling is what catches this one.
			name:            "should reject a pin on a newer patch than the topology version",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "v1.34.9",
			expectErrs:      []string{"version cannot be greater than Cluster.spec.topology.version \"v1.34.0\""},
		},
		{
			name:            "should reject a MachineDeployment pin outside the skew budget",
			versionPinning:  true,
			topologyVersion: "v1.35.0",
			mdVersion:       "v1.31.0",
			expectErrs:      []string{"spec.topology.workers.machineDeployments[md-0].version: Invalid value: \"v1.31.0\": version does not conform to the Kubernetes version skew policy with Cluster.spec.topology.version \"v1.35.0\""},
		},
		{
			name:            "should reject a MachinePool pin outside the skew budget",
			versionPinning:  true,
			topologyVersion: "v1.35.0",
			mpVersion:       "v1.31.0",
			expectErrs:      []string{"spec.topology.workers.machinePools[mp-0].version: Invalid value: \"v1.31.0\": version does not conform to the Kubernetes version skew policy"},
		},
		{
			// One error per offending pin, so the user sees every one that has to move.
			name:            "should report every pin outside the skew budget",
			versionPinning:  true,
			topologyVersion: "v1.35.0",
			mdVersion:       "v1.31.0",
			mpVersion:       "v1.30.0",
			expectErrs: []string{
				"spec.topology.workers.machineDeployments[md-0].version",
				"spec.topology.workers.machinePools[mp-0].version",
			},
		},
		{
			name:            "should reject a pinned version combined with the defer-upgrade annotation",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "v1.33.0",
			mdAnnotations:   map[string]string{clusterv1.ClusterTopologyDeferUpgradeAnnotation: ""},
			expectErrs:      []string{"cannot be set together with the " + clusterv1.ClusterTopologyDeferUpgradeAnnotation},
		},
		{
			name:            "should reject a pinned version combined with the hold-upgrade-sequence annotation",
			versionPinning:  true,
			topologyVersion: "v1.34.0",
			mdVersion:       "v1.33.0",
			mdAnnotations:   map[string]string{clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation: ""},
			expectErrs:      []string{"cannot be set together with the " + clusterv1.ClusterTopologyHoldUpgradeSequenceAnnotation},
		},
		{
			// An unparsable topology version is already reported for spec.topology.version.
			name:            "should not report skew for an unparsable topology version",
			versionPinning:  true,
			topologyVersion: "vfoo",
			mdVersion:       "v1.31.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopologyWorkerVersionPinning, tt.versionPinning)

			topology := builder.ClusterTopology().WithClass("foo").WithVersion(tt.topologyVersion).
				WithMachineDeployment(builder.MachineDeploymentTopology("md-0").WithClass("aa").
					WithVersion(tt.mdVersion).WithAnnotations(tt.mdAnnotations).Build()).
				WithMachinePool(builder.MachinePoolTopology("mp-0").WithClass("aa").
					WithVersion(tt.mpVersion).Build()).
				Build()

			errs := validateTopologyWorkerVersions(*topology, field.NewPath("spec", "topology"))

			if len(tt.expectErrs) == 0 {
				g.Expect(errs).To(BeEmpty())
				return
			}
			g.Expect(errs).To(HaveLen(len(tt.expectErrs)))
			for i, expectErr := range tt.expectErrs {
				g.Expect(errs[i].Error()).To(ContainSubstring(expectErr))
			}
		})
	}
}

func Test_validateTopologyWorkerVersionsUpdate(t *testing.T) {
	// clusterWithPins returns a Cluster with a control plane and one pinned MachineDeployment/MachinePool.
	clusterWithPins := func(topologyVersion, mdVersion, mpVersion string) *clusterv1.Cluster {
		return builder.Cluster("fooboo", "cluster1").
			WithControlPlane(builder.ControlPlane("fooboo", "cluster1-cp").Build()).
			WithTopology(builder.ClusterTopology().WithClass("foo").WithVersion(topologyVersion).
				WithMachineDeployment(builder.MachineDeploymentTopology("md-0").WithClass("aa").WithVersion(mdVersion).Build()).
				WithMachinePool(builder.MachinePoolTopology("mp-0").WithClass("aa").WithVersion(mpVersion).Build()).
				Build()).
			Build()
	}

	tests := []struct {
		name                     string
		old                      *clusterv1.Cluster
		new                      *clusterv1.Cluster
		controlPlaneVersion      string
		runningMachineDeployment string
		expectErrs               []string
		expectWarning            bool
	}{
		{
			name:                "should pass if nothing changed",
			old:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			new:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
		},
		{
			name:                "should pass on the first pin, equal to the control plane version",
			old:                 clusterWithPins("v1.32.0", "", ""),
			new:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
		},
		{
			name:                "should pass when raising a pin towards the control plane version",
			old:                 clusterWithPins("v1.32.0", "v1.31.0", "v1.31.0"),
			new:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
		},
		{
			name:                "should reject a pin above the control plane version",
			old:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			new:                 clusterWithPins("v1.32.0", "v1.33.0", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
			expectErrs:          []string{"version cannot be greater than the control plane version"},
		},
		{
			name:                "should reject a downgrade of a pin",
			old:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			new:                 clusterWithPins("v1.32.0", "v1.31.0", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
			expectErrs:          []string{"version cannot be decreased"},
		},
		{
			name:                "should reject a pin violating the version skew policy",
			old:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			new:                 clusterWithPins("v1.32.0", "v1.28.0", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
			// Note: a decrease is also reported, the skew error is the second one.
			expectErrs: []string{"version cannot be decreased", "does not conform to the Kubernetes version skew policy"},
		},
		{
			name:                "should reject unpinning while the pin is behind the topology version",
			old:                 clusterWithPins("v1.32.0", "v1.31.0", "v1.32.0"),
			new:                 clusterWithPins("v1.32.0", "", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
			expectErrs:          []string{"version can be unset only if it is equal to Cluster.spec.topology.version"},
		},
		{
			name:                "should pass when unpinning a pin equal to the topology version",
			old:                 clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			new:                 clusterWithPins("v1.32.0", "", "v1.32.0"),
			controlPlaneVersion: "v1.32.0",
		},
		{
			// The first pin has no previous pin to compare to, so it is validated against the version
			// the MachineDeployment actually runs.
			name:                     "should reject a first pin below the running version",
			old:                      clusterWithPins("v1.32.0", "", "v1.32.0"),
			new:                      clusterWithPins("v1.32.0", "v1.31.0", "v1.32.0"),
			controlPlaneVersion:      "v1.32.0",
			runningMachineDeployment: "v1.32.0",
			expectErrs:               []string{"version cannot be lower than the version currently running"},
		},
		{
			name:                     "should pass a first pin equal to the running version",
			old:                      clusterWithPins("v1.32.0", "", "v1.32.0"),
			new:                      clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			controlPlaneVersion:      "v1.32.0",
			runningMachineDeployment: "v1.32.0",
		},
		{
			name:          "should warn but not reject if the control plane cannot be read",
			old:           clusterWithPins("v1.32.0", "v1.31.0", "v1.32.0"),
			new:           clusterWithPins("v1.32.0", "v1.32.0", "v1.32.0"),
			expectWarning: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopologyWorkerVersionPinning, true)

			objs := []client.Object{builder.GenericControlPlaneCRD}
			if tt.controlPlaneVersion != "" {
				objs = append(objs, builder.ControlPlane("fooboo", "cluster1-cp").WithVersion(tt.controlPlaneVersion).
					WithStatusFields(map[string]interface{}{"status.version": tt.controlPlaneVersion}).
					Build())
			}
			if tt.runningMachineDeployment != "" {
				objs = append(objs, builder.MachineDeployment("fooboo", "cluster1-md-0").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "md-0",
				}).WithVersion(tt.runningMachineDeployment).Build())
			}
			webhook := &Cluster{Client: fake.NewClientBuilder().WithObjects(objs...).WithScheme(fakeScheme).Build()}

			warnings, errs := webhook.validateTopologyWorkerVersionsUpdate(ctx, tt.old, tt.new, field.NewPath("spec", "topology"))

			if tt.expectWarning {
				g.Expect(warnings).ToNot(BeEmpty())
			} else {
				g.Expect(warnings).To(BeEmpty())
			}

			if len(tt.expectErrs) == 0 {
				g.Expect(errs).To(BeEmpty())
				return
			}
			g.Expect(errs).To(HaveLen(len(tt.expectErrs)))
			for i, expectErr := range tt.expectErrs {
				g.Expect(errs[i].Error()).To(ContainSubstring(expectErr))
			}
		})
	}
}

func Test_validateTopologyMachineDeploymentVersions(t *testing.T) {
	tests := []struct {
		name              string
		expectErr         bool
		versionPinning    bool
		old               *clusterv1.Cluster
		additionalObjects []client.Object
	}{
		{
			name:      "should update if no machine deployment is exists",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			additionalObjects: []client.Object{},
		},
		{
			name:      "should update if machine deployments are fully upgraded and up to date",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.19.1").Build(),
			},
		},
		{
			name:      "should block update if machine deployment is not yet upgraded",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.18.1").Build(),
			},
		},
		{
			name:      "should block update if machine deployment is upgrading",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.19.1").WithSelector(*metav1.SetAsLabelSelector(labels.Set{clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1"})).Build(),
				builder.Machine("fooboo", "cluster1-workers1-1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel: "cluster1",
					// clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.18.1").Build(),
			},
		},
		{
			// A pinned MachineDeployment intentionally sits at another version, so it must not be
			// reported as completing a previous upgrade.
			name:           "should update if machine deployment pins an older version",
			expectErr:      false,
			versionPinning: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							WithVersion("v1.18.1").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.18.1").Build(),
			},
		},
		{
			// The pin only silences the version comparison, not the real upgrading check.
			name:           "should block update if pinned machine deployment is upgrading",
			expectErr:      true,
			versionPinning: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							WithVersion("v1.18.1").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.18.1").WithSelector(*metav1.SetAsLabelSelector(labels.Set{clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1"})).Build(),
				builder.Machine("fooboo", "cluster1-workers1-1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.17.1").Build(),
			},
		},
		{
			// With the gate disabled the pin is inert.
			name:           "should block update if machine deployment pins an older version and the feature gate is disabled",
			expectErr:      true,
			versionPinning: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachineDeployment(
						builder.MachineDeploymentTopology("workers1").
							WithClass("aa").
							WithVersion("v1.18.1").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachineDeployment("fooboo", "cluster1-workers1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                          "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:                 "",
					clusterv1.ClusterTopologyMachineDeploymentNameLabel: "workers1",
				}).WithVersion("v1.18.1").Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopologyWorkerVersionPinning, tt.versionPinning)

			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.additionalObjects...).
				WithScheme(fakeScheme).
				Build()

			oldVersion, err := semver.ParseTolerant(tt.old.Spec.Topology.Version)
			g.Expect(err).ToNot(HaveOccurred())

			err = validateTopologyMachineDeploymentVersions(ctx, fakeClient, tt.old, oldVersion)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func Test_validateTopologyMachinePoolVersions(t *testing.T) {
	tests := []struct {
		name              string
		expectErr         bool
		old               *clusterv1.Cluster
		versionPinning    bool
		additionalObjects []client.Object
		workloadObjects   []client.Object
	}{
		{
			name:      "should update if no machine pool is exists",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					Build()).
				Build(),
			additionalObjects: []client.Object{},
			workloadObjects:   []client.Object{},
		},
		{
			name:      "should update if machine pools are fully upgraded and up to date",
			expectErr: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.19.1").Build(),
			},
			workloadObjects: []client.Object{},
		},
		{
			name:      "should block update if machine pool is not yet upgraded",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.18.1").Build(),
			},
			workloadObjects: []client.Object{},
		},
		{
			name:      "should block update machine pool is upgrading",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.19.1").WithStatus(clusterv1.MachinePoolStatus{NodeRefs: []corev1.ObjectReference{{Name: "mp-node-1"}}}).Build(),
			},
			workloadObjects: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "mp-node-1"},
					Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.18.1"}},
				},
			},
		},
		{
			name:      "should block update if it cannot get the node of a machine pool",
			expectErr: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.19.1").WithStatus(clusterv1.MachinePoolStatus{NodeRefs: []corev1.ObjectReference{{Name: "mp-node-1"}}}).Build(),
			},
			workloadObjects: []client.Object{},
		},
		{
			// A pinned MachinePool intentionally sits at another version, so it must not be reported
			// as completing a previous upgrade.
			name:           "should update if machine pool pins an older version",
			expectErr:      false,
			versionPinning: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							WithVersion("v1.18.1").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.18.1").Build(),
			},
			workloadObjects: []client.Object{},
		},
		{
			// The pin only silences the version comparison, not the real upgrading check.
			name:           "should block update if pinned machine pool is upgrading",
			expectErr:      true,
			versionPinning: true,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							WithVersion("v1.18.1").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.18.1").WithStatus(clusterv1.MachinePoolStatus{NodeRefs: []corev1.ObjectReference{{Name: "mp-node-1"}}}).Build(),
			},
			workloadObjects: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{Name: "mp-node-1"},
					Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.17.1"}},
				},
			},
		},
		{
			// With the gate disabled the pin is inert.
			name:           "should block update if machine pool pins an older version and the feature gate is disabled",
			expectErr:      true,
			versionPinning: false,
			old: builder.Cluster("fooboo", "cluster1").
				WithTopology(builder.ClusterTopology().
					WithClass("foo").
					WithVersion("v1.19.1").
					WithMachinePool(
						builder.MachinePoolTopology("pool1").
							WithClass("aa").
							WithVersion("v1.18.1").
							Build()).
					Build()).
				Build(),
			additionalObjects: []client.Object{
				builder.MachinePool("fooboo", "cluster1-pool1").WithLabels(map[string]string{
					clusterv1.ClusterNameLabel:                    "cluster1",
					clusterv1.ClusterTopologyOwnedLabel:           "",
					clusterv1.ClusterTopologyMachinePoolNameLabel: "pool1",
				}).WithVersion("v1.18.1").Build(),
			},
			workloadObjects: []client.Object{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopologyWorkerVersionPinning, tt.versionPinning)

			fakeClient := fake.NewClientBuilder().
				WithObjects(tt.additionalObjects...).
				WithScheme(fakeScheme).
				Build()

			oldVersion, err := semver.ParseTolerant(tt.old.Spec.Topology.Version)
			g.Expect(err).ToNot(HaveOccurred())

			fakeClusterCacheTracker := &fakeClusterCache{
				client: fake.NewClientBuilder().
					WithObjects(tt.workloadObjects...).
					Build(),
			}

			err = validateTopologyMachinePoolVersions(ctx, fakeClient, fakeClusterCacheTracker, tt.old, oldVersion)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestValidateAutoscalerAnnotationsForCluster(t *testing.T) {
	tests := []struct {
		name         string
		expectErr    bool
		cluster      *clusterv1.Cluster
		clusterClass *clusterv1.ClusterClass
	}{
		{
			name:      "no workers in topology",
			expectErr: false,
			cluster:   &clusterv1.Cluster{},
		},
		{
			name:      "replicas is not set",
			expectErr: false,
			cluster: builder.Cluster("ns", "name").WithTopology(
				builder.ClusterTopology().
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						Build()).
					Build()).
				Build(),
		},
		{
			name:      "replicas is set but there are no autoscaler annotations",
			expectErr: false,
			cluster: builder.Cluster("ns", "name").WithTopology(
				builder.ClusterTopology().
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithReplicas(2).
						Build(),
					).
					Build()).
				Build(),
		},
		{
			name:      "replicas is set on an MD that has only one annotation",
			expectErr: true,
			cluster: builder.Cluster("ns", "name").WithTopology(
				builder.ClusterTopology().
					WithMachineDeployment(builder.MachineDeploymentTopology("workers2").
						WithReplicas(2).
						WithAnnotations(map[string]string{
							clusterv1.AutoscalerMinSizeAnnotation: "2",
						}).
						Build(),
					).
					Build()).
				Build(),
		},
		{
			name:      "replicas is set on an MD that has two annotations",
			expectErr: true,
			cluster: builder.Cluster("ns", "name").WithTopology(
				builder.ClusterTopology().
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithReplicas(2).
						Build(),
					).
					WithMachineDeployment(builder.MachineDeploymentTopology("workers2").
						WithReplicas(2).
						WithAnnotations(map[string]string{
							clusterv1.AutoscalerMinSizeAnnotation: "2",
							clusterv1.AutoscalerMaxSizeAnnotation: "20",
						}).
						Build(),
					).
					Build()).
				Build(),
		},
		{
			name:      "replicas is set, there are no autoscaler annotations on the Cluster MDT, but there is no matching ClusterClass MDC",
			expectErr: false,
			cluster: builder.Cluster("ns", "name").WithTopology(
				builder.ClusterTopology().
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithClass("mdc1").
						WithReplicas(2).
						Build(),
					).
					Build()).
				Build(),
			clusterClass: builder.ClusterClass("ns", "name").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("mdc2").Build()).
				Build(),
		},
		{
			name:      "replicas is set, there are no autoscaler annotations on the Cluster MDT, but there is only one on the ClusterClass MDC",
			expectErr: true,
			cluster: builder.Cluster("ns", "name").WithTopology(
				builder.ClusterTopology().
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithClass("mdc1").
						WithReplicas(2).
						Build(),
					).
					Build()).
				Build(),
			clusterClass: builder.ClusterClass("ns", "name").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("mdc1").
					WithAnnotations(map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "2",
					}).
					Build()).
				Build(),
		},
		{
			name:      "replicas is set, there are no autoscaler annotations on the Cluster MDT, but there are two on the ClusterClass MDC",
			expectErr: true,
			cluster: builder.Cluster("ns", "name").WithTopology(
				builder.ClusterTopology().
					WithMachineDeployment(builder.MachineDeploymentTopology("workers1").
						WithClass("mdc1").
						WithReplicas(2).
						Build(),
					).
					Build()).
				Build(),
			clusterClass: builder.ClusterClass("ns", "name").
				WithWorkerMachineDeploymentClasses(*builder.MachineDeploymentClass("mdc1").
					WithAnnotations(map[string]string{
						clusterv1.AutoscalerMinSizeAnnotation: "2",
						clusterv1.AutoscalerMaxSizeAnnotation: "20",
					}).
					Build()).
				Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := validateAutoscalerAnnotationsForCluster(tt.cluster, tt.clusterClass)
			if tt.expectErr {
				g.Expect(err).ToNot(BeEmpty())
			} else {
				g.Expect(err).To(BeEmpty())
			}
		})
	}
}

func refToUnstructured(ref *clusterv1.ClusterClassTemplateReference) *unstructured.Unstructured {
	gvk := ref.GroupVersionKind()
	output := &unstructured.Unstructured{}
	output.SetKind(gvk.Kind)
	output.SetAPIVersion(gvk.GroupVersion().String())
	output.SetName(ref.Name)
	return output
}

type fakeClusterCache struct {
	client client.Reader
}

func (f *fakeClusterCache) GetReader(_ context.Context, _ types.NamespacedName) (client.Reader, error) {
	return f.client, nil
}
