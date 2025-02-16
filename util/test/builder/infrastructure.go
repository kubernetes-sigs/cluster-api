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

	// GenericInfrastructureMachinePoolTemplateKind is the Kind for the GenericInfrastructureMachinePoolTemplate.
	GenericInfrastructureMachinePoolTemplateKind = "GenericInfrastructureMachinePoolTemplate"
	// GenericInfrastructureMachinePoolTemplateCRD is a generic infrastructure machine pool template CRD.
	GenericInfrastructureMachinePoolTemplateCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureMachinePoolTemplateKind))

	// GenericInfrastructureMachinePoolKind is the Kind for the GenericInfrastructureMachinePool.
	GenericInfrastructureMachinePoolKind = "GenericInfrastructureMachinePool"
	// GenericInfrastructureMachinePoolCRD is a generic infrastructure machine pool CRD.
	GenericInfrastructureMachinePoolCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureMachinePoolKind))

	// GenericInfrastructureClusterKind is the kind for the GenericInfrastructureCluster type.
	GenericInfrastructureClusterKind = "GenericInfrastructureCluster"
	// GenericInfrastructureClusterCRD is a generic infrastructure machine CRD.
	GenericInfrastructureClusterCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureClusterKind))

	// GenericInfrastructureClusterTemplateKind is the kind for the GenericInfrastructureClusterTemplate type.
	GenericInfrastructureClusterTemplateKind = "GenericInfrastructureClusterTemplate"
	// GenericInfrastructureClusterTemplateCRD is a generic infrastructure machine template CRD.
	GenericInfrastructureClusterTemplateCRD = untypedCRD(InfrastructureGroupVersion.WithKind(GenericInfrastructureClusterTemplateKind))

	// TODO: drop generic CRDs in favour of typed test CRDs.

	// TestInfrastructureClusterTemplateKind is the kind for the TestInfrastructureClusterTemplate type.
	TestInfrastructureClusterTemplateKind = "TestInfrastructureClusterTemplate"
	// TestInfrastructureClusterTemplateCRD is a test infrastructure machine template CRD.
	TestInfrastructureClusterTemplateCRD = testInfrastructureClusterTemplateCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureClusterTemplateKind))

	// TestInfrastructureClusterKind is the kind for the TestInfrastructureCluster type.
	TestInfrastructureClusterKind = "TestInfrastructureCluster"
	// TestInfrastructureClusterCRD is a test infrastructure machine CRD.
	TestInfrastructureClusterCRD = testInfrastructureClusterCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureClusterKind))

	// TestInfrastructureMachineTemplateKind is the kind for the TestInfrastructureMachineTemplate type.
	TestInfrastructureMachineTemplateKind = "TestInfrastructureMachineTemplate"
	// TestInfrastructureMachineTemplateCRD is a test infrastructure machine template CRD.
	TestInfrastructureMachineTemplateCRD = testInfrastructureMachineTemplateCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureMachineTemplateKind))

	// TestInfrastructureMachinePoolTemplateKind is the kind for the TestInfrastructureMachinePoolTemplate type.
	TestInfrastructureMachinePoolTemplateKind = "TestInfrastructureMachinePoolTemplate"
	// TestInfrastructureMachinePoolTemplateCRD is a test infrastructure machine pool template CRD.
	TestInfrastructureMachinePoolTemplateCRD = testInfrastructureMachinePoolTemplateCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureMachinePoolTemplateKind))

	// TestInfrastructureMachinePoolKind is the kind for the TestInfrastructureMachinePool type.
	TestInfrastructureMachinePoolKind = "TestInfrastructureMachinePool"
	// TestInfrastructureMachinePoolCRD is a test infrastructure machine CRD.
	TestInfrastructureMachinePoolCRD = testInfrastructureMachinePoolCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureMachinePoolKind))

	// TestInfrastructureMachineKind is the kind for the TestInfrastructureMachine type.
	TestInfrastructureMachineKind = "TestInfrastructureMachine"
	// TestInfrastructureMachineCRD is a test infrastructure machine CRD.
	TestInfrastructureMachineCRD = testInfrastructureMachineCRD(InfrastructureGroupVersion.WithKind(TestInfrastructureMachineKind))
)

func testInfrastructureClusterTemplateCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
		"spec": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// Mandatory field from the Cluster API contract
				"template": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"spec": clusterSpecSchema,
					},
				},
			},
		},
	})
}

func testInfrastructureClusterCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
		"spec": clusterSpecSchema,
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
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
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

func testInfrastructureMachinePoolTemplateCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
		"spec": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// Mandatory field from the Cluster API contract
				"template": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"metadata": metadataSchema,
						"spec":     machinePoolSpecSchema,
					},
				},
			},
		},
	})
}

func testInfrastructureMachineCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
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

func testInfrastructureMachinePoolCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
		"spec": machinePoolSpecSchema,
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
	clusterSpecSchema = apiextensionsv1.JSONSchemaProps{
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
	}

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

	machinePoolSpecSchema = apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			// Mandatory field from the Cluster API contract
			"providerIDList": {
				Type: "array",
				Items: &apiextensionsv1.JSONSchemaPropsOrArray{
					Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "string",
					},
				},
			},
			// General purpose fields to be used in different test scenario.
			"foo": {Type: "string"},
			"bar": {Type: "string"},
		},
	}
)
