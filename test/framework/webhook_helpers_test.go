/*
Copyright 2026 The Kubernetes Authors.

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

package framework

import (
	"testing"

	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// validCABundle is a PEM-encoded self-signed certificate (only used to test PEM parsing).
var validCABundle = []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----
`)

func Test_caBundleContainsCertificate(t *testing.T) {
	tests := []struct {
		name     string
		caBundle []byte
		want     bool
	}{
		{
			name:     "nil CA bundle",
			caBundle: nil,
			want:     false,
		},
		{
			name:     "empty CA bundle",
			caBundle: []byte{},
			want:     false,
		},
		{
			name:     "placeholder CA bundle (caBundle: Cg==)",
			caBundle: []byte("\n"),
			want:     false,
		},
		{
			name:     "garbage CA bundle",
			caBundle: []byte("not a certificate"),
			want:     false,
		},
		{
			name:     "valid PEM certificate",
			caBundle: validCABundle,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(caBundleContainsCertificate(tt.caBundle)).To(Equal(tt.want))
		})
	}
}

func Test_pendingCABundleInjections(t *testing.T) {
	annotations := map[string]string{injectCAFromAnnotation: "capd-system/capd-serving-cert"}

	validatingWebhookConfiguration := func(name string, annotations map[string]string, caBundles ...[]byte) admissionregistrationv1.ValidatingWebhookConfiguration {
		webhookConfiguration := admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
		}
		for _, caBundle := range caBundles {
			webhookConfiguration.Webhooks = append(webhookConfiguration.Webhooks, admissionregistrationv1.ValidatingWebhook{
				ClientConfig: admissionregistrationv1.WebhookClientConfig{CABundle: caBundle},
			})
		}
		return webhookConfiguration
	}
	mutatingWebhookConfiguration := func(name string, annotations map[string]string, caBundles ...[]byte) admissionregistrationv1.MutatingWebhookConfiguration {
		webhookConfiguration := admissionregistrationv1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
		}
		for _, caBundle := range caBundles {
			webhookConfiguration.Webhooks = append(webhookConfiguration.Webhooks, admissionregistrationv1.MutatingWebhook{
				ClientConfig: admissionregistrationv1.WebhookClientConfig{CABundle: caBundle},
			})
		}
		return webhookConfiguration
	}
	crd := func(name string, annotations map[string]string, conversion *apiextensionsv1.CustomResourceConversion) apiextensionsv1.CustomResourceDefinition {
		return apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations},
			Spec:       apiextensionsv1.CustomResourceDefinitionSpec{Conversion: conversion},
		}
	}
	webhookConversion := func(caBundle []byte) *apiextensionsv1.CustomResourceConversion {
		return &apiextensionsv1.CustomResourceConversion{
			Strategy: apiextensionsv1.WebhookConverter,
			Webhook: &apiextensionsv1.WebhookConversion{
				ClientConfig: &apiextensionsv1.WebhookClientConfig{CABundle: caBundle},
			},
		}
	}

	tests := []struct {
		name                            string
		validatingWebhookConfigurations []admissionregistrationv1.ValidatingWebhookConfiguration
		mutatingWebhookConfigurations   []admissionregistrationv1.MutatingWebhookConfiguration
		crds                            []apiextensionsv1.CustomResourceDefinition
		want                            []string
	}{
		{
			name: "all injected",
			validatingWebhookConfigurations: []admissionregistrationv1.ValidatingWebhookConfiguration{
				validatingWebhookConfiguration("capd-validating-webhook-configuration", annotations, validCABundle),
			},
			mutatingWebhookConfigurations: []admissionregistrationv1.MutatingWebhookConfiguration{
				mutatingWebhookConfiguration("capd-mutating-webhook-configuration", annotations, validCABundle),
			},
			crds: []apiextensionsv1.CustomResourceDefinition{
				crd("dockermachinetemplates.infrastructure.cluster.x-k8s.io", annotations, webhookConversion(validCABundle)),
			},
			want: []string{},
		},
		{
			name: "pending injection into webhook configurations and CRD",
			validatingWebhookConfigurations: []admissionregistrationv1.ValidatingWebhookConfiguration{
				validatingWebhookConfiguration("capd-validating-webhook-configuration", annotations, nil),
			},
			mutatingWebhookConfigurations: []admissionregistrationv1.MutatingWebhookConfiguration{
				mutatingWebhookConfiguration("capd-mutating-webhook-configuration", annotations, []byte("\n")),
			},
			crds: []apiextensionsv1.CustomResourceDefinition{
				crd("dockermachinetemplates.infrastructure.cluster.x-k8s.io", annotations, webhookConversion(nil)),
			},
			want: []string{
				"ValidatingWebhookConfiguration/capd-validating-webhook-configuration",
				"MutatingWebhookConfiguration/capd-mutating-webhook-configuration",
				"CustomResourceDefinition/dockermachinetemplates.infrastructure.cluster.x-k8s.io",
			},
		},
		{
			name: "one webhook of many without CA bundle",
			validatingWebhookConfigurations: []admissionregistrationv1.ValidatingWebhookConfiguration{
				validatingWebhookConfiguration("capd-validating-webhook-configuration", annotations, validCABundle, nil),
			},
			want: []string{"ValidatingWebhookConfiguration/capd-validating-webhook-configuration"},
		},
		{
			name: "objects without the inject annotation are ignored",
			validatingWebhookConfigurations: []admissionregistrationv1.ValidatingWebhookConfiguration{
				validatingWebhookConfiguration("some-other-webhook-configuration", nil, nil),
			},
			mutatingWebhookConfigurations: []admissionregistrationv1.MutatingWebhookConfiguration{
				mutatingWebhookConfiguration("some-other-webhook-configuration", nil, nil),
			},
			crds: []apiextensionsv1.CustomResourceDefinition{
				crd("machines.cluster.x-k8s.io", nil, webhookConversion(nil)),
			},
			want: []string{},
		},
		{
			name: "annotated CRD without webhook conversion is ignored",
			crds: []apiextensionsv1.CustomResourceDefinition{
				crd("dockermachinetemplates.infrastructure.cluster.x-k8s.io", annotations, nil),
				crd("dockerclusters.infrastructure.cluster.x-k8s.io", annotations, &apiextensionsv1.CustomResourceConversion{Strategy: apiextensionsv1.NoneConverter}),
			},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := pendingCABundleInjections(
				&admissionregistrationv1.ValidatingWebhookConfigurationList{Items: tt.validatingWebhookConfigurations},
				&admissionregistrationv1.MutatingWebhookConfigurationList{Items: tt.mutatingWebhookConfigurations},
				&apiextensionsv1.CustomResourceDefinitionList{Items: tt.crds},
			)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
