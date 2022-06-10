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

package builder

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// InfrastructureGroupVersion is group version used for infrastructure objects.
	InfrastructureGroupVersion = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1beta1"}

	// GenericInfrastructureMachineKind is the Kind for the GenericInfrastructureMachine.
	GenericInfrastructureMachineKind = "GenericInfrastructureMachine"
	// GenericInfrastructureMachineCRD is a generic infrastructure machine CRD.
	GenericInfrastructureMachineCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureMachineKind))

	// GenericInfrastructureMachineTemplateKind is the Kind for the GenericInfrastructureMachineTemplate.
	GenericInfrastructureMachineTemplateKind = "GenericInfrastructureMachineTemplate"
	// GenericInfrastructureMachineTemplateCRD is a generic infrastructure machine template CRD.
	GenericInfrastructureMachineTemplateCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureMachineTemplateKind))

	// GenericInfrastructureClusterKind is the kind for the GenericInfrastructureCluster type.
	GenericInfrastructureClusterKind = "GenericInfrastructureCluster"
	// GenericInfrastructureClusterCRD is a generic infrastructure machine CRD.
	GenericInfrastructureClusterCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureClusterKind))

	// GenericInfrastructureClusterTemplateKind is the kind for the GenericInfrastructureClusterTemplate type.
	GenericInfrastructureClusterTemplateKind = "GenericInfrastructureClusterTemplate"
	// GenericInfrastructureClusterTemplateCRD is a generic infrastructure machine template CRD.
	GenericInfrastructureClusterTemplateCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureClusterTemplateKind))

	// TODO: drop generic CRDs in favour of typed test CRDs.

	// TestInfrastructureClusterKind is the kind for the TestInfrastructureCluster type.
	TestInfrastructureClusterKind = "TestInfrastructureCluster"
	// TestInfrastructureClusterCRD is a test infrastructure machine CRD.
	TestInfrastructureClusterCRD = testInfrastructureClusterCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureClusterKind))

	// TestInfrastructureMachineTemplateKind is the kind for the TestInfrastructureMachineTemplate type.
	TestInfrastructureMachineTemplateKind = "TestInfrastructureMachineTemplate"
	// TestInfrastructureMachineTemplateCRD is a test infrastructure machine template CRD.
	TestInfrastructureMachineTemplateCRD = testInfrastructureMachineTemplateCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureMachineTemplateKind))

	// TestInfrastructureMachineKind is the kind for the TestInfrastructureMachine type.
	TestInfrastructureMachineKind = "TestInfrastructureMachine"
	// TestInfrastructureMachineCRD is a test infrastructure machine CRD.
	TestInfrastructureMachineCRD = testInfrastructureMachineCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureMachineKind))
)

func testInfrastructureClusterCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"spec": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// Mandatory field from the Cluster API contract
				"controlPlaneEndpoint": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"host": {Type: "string"},
						"port": {Type: "integer"},
					},
					Required: []string{"host", "port"},
				},
				// General purpose fields to be used in different test scenario.
				"foo": {Type: "string"},
				"bar": {Type: "string"},
				"fooMap": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"foo": {Type: "string"},
					},
				},
				"fooList": {
					Type: "array",
					Items: &apiextensionsv1.JSONSchemaPropsOrArray{
						Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"foo": {Type: "string"},
							},
						},
					},
				},
				// Field for testing
			},
		},
		"status": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// mandatory field from the Cluster API contract
				"ready": {Type: "boolean"},
				// General purpose fields to be used in different test scenario.
				"foo": {Type: "string"},
				"bar": {Type: "string"},
			},
			Required: []string{"ready"},
		},
	})
}

func testInfrastructureMachineTemplateCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"spec": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// Mandatory field from the Cluster API contract
				"template": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"metadata": metadataSchema,
						"spec":     machineSpecSchema,
					},
				},
			},
		},
	})
}

func testInfrastructureMachineCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"spec": machineSpecSchema,
		"status": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// mandatory field from the Cluster API contract
				"ready": {Type: "boolean"},
				// General purpose fields to be used in different test scenario.
				"foo": {Type: "string"},
				"bar": {Type: "string"},
			},
		},
	})
}

var (
	machineSpecSchema = apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			// Mandatory field from the Cluster API contract
			"providerID": {Type: "string"},
			// General purpose fields to be used in different test scenario.
			"foo": {Type: "string"},
			"bar": {Type: "string"},
		},
	}
)
