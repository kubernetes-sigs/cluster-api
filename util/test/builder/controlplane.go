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
	// ControlPlaneGroupVersion is group version used for control plane objects.
	ControlPlaneGroupVersion = schema.GroupVersion{Group: "controlplane.cluster.x-k8s.io", Version: "v1beta1"}

	// GenericControlPlaneKind is the Kind for the GenericControlPlane.
	GenericControlPlaneKind = "GenericControlPlane"
	// GenericControlPlaneCRD is a generic control plane CRD.
	GenericControlPlaneCRD = untypedCRD(ControlPlaneGroupVersion.WithKind(GenericControlPlaneKind))

	// GenericControlPlaneTemplateKind is the Kind for the GenericControlPlaneTemplate.
	GenericControlPlaneTemplateKind = "GenericControlPlaneTemplate"
	// GenericControlPlaneTemplateCRD is a generic control plane template CRD.
	GenericControlPlaneTemplateCRD = untypedCRD(ControlPlaneGroupVersion.WithKind(GenericControlPlaneTemplateKind))

	// TODO: drop generic CRDs in favour of typed test CRDs.

	// TestControlPlaneTemplateKind is the Kind for the TestControlPlaneTemplate.
	TestControlPlaneTemplateKind = "TestControlPlaneTemplate"
	// TestControlPlaneTemplateCRD is a test control plane template CRD.
	TestControlPlaneTemplateCRD = testControlPlaneTemplateCRD(ControlPlaneGroupVersion.WithKind(TestControlPlaneTemplateKind))

	// TestControlPlaneKind is the Kind for the TestControlPlane.
	TestControlPlaneKind = "TestControlPlane"
	// TestControlPlaneCRD is a test control plane CRD.
	TestControlPlaneCRD = testControlPlaneCRD(ControlPlaneGroupVersion.WithKind(TestControlPlaneKind))
)

func testControlPlaneTemplateCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
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
						"spec": controPlaneSpecSchema,
					},
				},
			},
		},
	})
}

func testControlPlaneCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
		"spec": controPlaneSpecSchema,
		"status": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// mandatory field from the Cluster API contract
				"ready": {Type: "boolean"},
				// mandatory field from the Cluster API contract - replicas support
				"replicas":            {Type: "integer", Format: "int32"},
				"selector":            {Type: "string"},
				"readyReplicas":       {Type: "integer", Format: "int32"},
				"unavailableReplicas": {Type: "integer", Format: "int32"},
				"updatedReplicas":     {Type: "integer", Format: "int32"},
				// Mandatory field from the Cluster API contract - version support
				"version": {Type: "string"},
				// General purpose fields to be used in different test scenario.
				"foo": {Type: "string"},
				"bar": {Type: "string"},
			},
		},
	})
}

var (
	controPlaneSpecSchema = apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			// Mandatory field from the Cluster API contract - version support
			"version": {
				Type: "string",
			},
			// mandatory field from the Cluster API contract - replicas support
			"replicas": {
				Type:   "integer",
				Format: "int32",
			},
			// mandatory field from the Cluster API contract - using Machines support
			"machineTemplate": {
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"metadata":            metadataSchema,
					"infrastructureRef":   refSchema,
					"nodeDeletionTimeout": {Type: "string"},
					"nodeDrainTimeout":    {Type: "string"},
				},
			},
			// General purpose fields to be used in different test scenario.
			"foo": {Type: "string"},
			"bar": {Type: "string"},
			// Copy of a subset of KCP spec fields to test server side apply on deep nested structs
			"kubeadmConfigSpec": {
				Type: "object",
				Properties: map[string]apiextensionsv1.JSONSchemaProps{
					"clusterConfiguration": {
						Type: "object",
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"controllerManager": {
								Type: "object",
								Properties: map[string]apiextensionsv1.JSONSchemaProps{
									"extraArgs": {
										Type: "object",
										AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
											Schema: &apiextensionsv1.JSONSchemaProps{Type: "string"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
)
