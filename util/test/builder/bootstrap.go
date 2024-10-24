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
	// BootstrapGroupVersion is group version used for bootstrap objects.
	BootstrapGroupVersion = schema.GroupVersion{Group: "bootstrap.cluster.x-k8s.io", Version: "v1beta1"}

	// GenericBootstrapConfigKind is the Kind for the GenericBootstrapConfig.
	GenericBootstrapConfigKind = "GenericBootstrapConfig"
	// GenericBootstrapConfigCRD is a generic bootstrap CRD.
	GenericBootstrapConfigCRD = untypedCRD(BootstrapGroupVersion.WithKind(GenericBootstrapConfigKind))

	// GenericBootstrapConfigTemplateKind is the Kind for the GenericBootstrapConfigTemplate.
	GenericBootstrapConfigTemplateKind = "GenericBootstrapConfigTemplate"
	// GenericBootstrapConfigTemplateCRD is a generic bootstrap template CRD.
	GenericBootstrapConfigTemplateCRD = untypedCRD(BootstrapGroupVersion.WithKind(GenericBootstrapConfigTemplateKind))

	// TODO: drop generic CRDs in favour of typed test CRDs.

	// TestBootstrapConfigTemplateKind is the kind for the TestBootstrapConfigTemplate type.
	TestBootstrapConfigTemplateKind = "TestBootstrapConfigTemplate"
	// TestBootstrapConfigTemplateCRD is a test bootstrap config template CRD.
	TestBootstrapConfigTemplateCRD = testBootstrapConfigTemplateCRD(BootstrapGroupVersion.WithKind(TestBootstrapConfigTemplateKind))

	// TestBootstrapConfigKind is the kind for the TestBootstrapConfig type.
	TestBootstrapConfigKind = "TestBootstrapConfig"
	// TestBootstrapConfigCRD is a test bootstrap config CRD.
	TestBootstrapConfigCRD = testBootstrapConfigCRD(BootstrapGroupVersion.WithKind(TestBootstrapConfigKind))
)

func testBootstrapConfigTemplateCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
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
						"spec": bootstrapConfigSpecSchema,
					},
				},
			},
		},
	})
}

func testBootstrapConfigCRD(gvk schema.GroupVersionKind) *apiextensionsv1.CustomResourceDefinition {
	return generateCRD(gvk, map[string]apiextensionsv1.JSONSchemaProps{
		"metadata": {
			// NOTE: in CRD there is only a partial definition of metadata schema.
			// Ref https://github.com/kubernetes-sigs/controller-tools/blob/59485af1c1f6a664655dad49543c474bb4a0d2a2/pkg/crd/gen.go#L185
			Type: "object",
		},
		"spec": bootstrapConfigSpecSchema,
		"status": {
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				// mandatory field from the Cluster API contract
				"ready":          {Type: "boolean"},
				"dataSecretName": {Type: "string"},
				// General purpose fields to be used in different test scenario.
				"foo": {Type: "string"},
				"bar": {Type: "string"},
			},
		},
	})
}

var (
	bootstrapConfigSpecSchema = apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			// General purpose fields to be used in different test scenario.
			"foo": {Type: "string"},
			"bar": {Type: "string"},
		},
	}
)
