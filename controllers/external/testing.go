/*
Copyright 2019 The Kubernetes Authors.

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

package external

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
)

var (
	// TestGenericBootstrapCRD is a generic boostrap CRD.
	// Deprecated: This field will be removed in a next release.
	TestGenericBootstrapCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "bootstrapmachines.bootstrap.cluster.x-k8s.io",
			Labels: map[string]string{
				clusterv1.GroupVersion.String(): "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "bootstrap.cluster.x-k8s.io",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "BootstrapMachine",
				Plural: "bootstrapmachines",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha4",
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
							},
						},
					},
				},
			},
		},
	}

	// TestGenericBootstrapTemplateCRD is a generic boostrap template CRD.
	// Deprecated: This field will be removed in a next release.
	TestGenericBootstrapTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "bootstrapmachinetemplates.bootstrap.cluster.x-k8s.io",
			Labels: map[string]string{
				clusterv1.GroupVersion.String(): "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "bootstrap.cluster.x-k8s.io",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "BootstrapMachineTemplate",
				Plural: "bootstrapmachinetemplates",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha4",
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
							},
						},
					},
				},
			},
		},
	}

	// TestGenericInfrastructureCRD is a generic infrastructure CRD.
	// Deprecated: This field will be removed in a next release.
	TestGenericInfrastructureCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "infrastructuremachines.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				clusterv1.GroupVersion.String(): "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "infrastructure.cluster.x-k8s.io",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "InfrastructureMachine",
				Plural: "infrastructuremachines",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha4",
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
							},
						},
					},
				},
			},
		},
	}

	// TestGenericInfrastructureTemplateCRD is a generic infrastructure template CRD.
	// Deprecated: This field will be removed in a next release.
	TestGenericInfrastructureTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "infrastructuremachinetemplates.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				clusterv1.GroupVersion.String(): "v1alpha4",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "infrastructure.cluster.x-k8s.io",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "InfrastructureMachineTemplate",
				Plural: "infrastructuremachinetemplates",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha4",
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
							},
						},
					},
				},
			},
		},
	}

	// TestGenericInfrastructureRemediationCRD is a generic infrastructure remediation CRD.
	// Deprecated: This field will be removed in a next release.
	TestGenericInfrastructureRemediationCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "infrastructureremediations.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				clusterv1.GroupVersion.String(): "v1alpha3",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "infrastructure.cluster.x-k8s.io",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "InfrastructureRemediation",
				Plural: "infrastructureremediations",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha3",
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
							},
						},
					},
				},
			},
		},
	}

	// TestGenericInfrastructureRemediationTemplateCRD is a generic infrastructure remediation template CRD.
	// Deprecated: This field will be removed in a next release.
	TestGenericInfrastructureRemediationTemplateCRD = &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiextensionsv1.SchemeGroupVersion.String(),
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "infrastructureremediationtemplates.infrastructure.cluster.x-k8s.io",
			Labels: map[string]string{
				clusterv1.GroupVersion.String(): "v1alpha3",
			},
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "infrastructure.cluster.x-k8s.io",
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:   "InfrastructureRemediationTemplate",
				Plural: "infrastructureremediationtemplates",
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha3",
					Served:  true,
					Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
					},
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
								"status": {
									Type:                   "object",
									XPreserveUnknownFields: pointer.BoolPtr(true),
								},
							},
						},
					},
				},
			},
		},
	}
)
