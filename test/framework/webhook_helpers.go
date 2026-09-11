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
	"context"
	"encoding/pem"
	"fmt"
	"strings"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	. "sigs.k8s.io/cluster-api/test/framework/ginkgoextensions"
)

// injectCAFromAnnotation is the annotation cert-manager's cainjector uses to discover
// webhook configurations and CRDs it has to inject the CA bundle into.
const injectCAFromAnnotation = "cert-manager.io/inject-ca-from"

// WaitForWebhookCABundleInjectionInput is the input for WaitForWebhookCABundleInjection.
type WaitForWebhookCABundleInjectionInput struct {
	Lister Lister
}

// WaitForWebhookCABundleInjection waits until cert-manager's cainjector has injected the CA bundle
// into all ValidatingWebhookConfigurations, MutatingWebhookConfigurations and CRD conversion
// webhooks belonging to Cluster API providers and annotated with cert-manager.io/inject-ca-from.
// Provider Deployments becoming available doesn't guarantee that the CA bundle is already injected;
// until injection is done the kube-apiserver fails webhook calls with "tls: failed to verify
// certificate: x509: certificate signed by unknown authority". Waiting for injection avoids those
// errors when objects are created right after providers have been installed (e.g. on clusterctl move).
func WaitForWebhookCABundleInjection(ctx context.Context, input WaitForWebhookCABundleInjectionInput, intervals ...interface{}) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for WaitForWebhookCABundleInjection")
	Expect(input.Lister).ToNot(BeNil(), "Invalid argument. input.Lister can't be nil when calling WaitForWebhookCABundleInjection")

	Byf("Waiting for CA bundles to be injected into provider webhook configurations")
	Eventually(func() error {
		validatingWebhookConfigurations := &admissionregistrationv1.ValidatingWebhookConfigurationList{}
		if err := input.Lister.List(ctx, validatingWebhookConfigurations, capiProviderOptions()...); err != nil {
			return errors.Wrap(err, "failed to list ValidatingWebhookConfigurations")
		}
		mutatingWebhookConfigurations := &admissionregistrationv1.MutatingWebhookConfigurationList{}
		if err := input.Lister.List(ctx, mutatingWebhookConfigurations, capiProviderOptions()...); err != nil {
			return errors.Wrap(err, "failed to list MutatingWebhookConfigurations")
		}
		crds := &apiextensionsv1.CustomResourceDefinitionList{}
		if err := input.Lister.List(ctx, crds, capiProviderOptions()...); err != nil {
			return errors.Wrap(err, "failed to list CustomResourceDefinitions")
		}

		if pending := pendingCABundleInjections(validatingWebhookConfigurations, mutatingWebhookConfigurations, crds); len(pending) > 0 {
			return errors.Errorf("CA bundle not yet injected into: %s", strings.Join(pending, ", "))
		}
		return nil
	}, intervals...).Should(Succeed(), "Failed to wait for CA bundle injection into provider webhook configurations")
}

// pendingCABundleInjections returns the annotated webhook configurations and CRDs which don't have
// a valid CA bundle injected yet.
func pendingCABundleInjections(validatingWebhookConfigurations *admissionregistrationv1.ValidatingWebhookConfigurationList, mutatingWebhookConfigurations *admissionregistrationv1.MutatingWebhookConfigurationList, crds *apiextensionsv1.CustomResourceDefinitionList) []string {
	pending := []string{}

	for _, webhookConfiguration := range validatingWebhookConfigurations.Items {
		if _, ok := webhookConfiguration.Annotations[injectCAFromAnnotation]; !ok {
			continue
		}
		for _, webhook := range webhookConfiguration.Webhooks {
			if !caBundleContainsCertificate(webhook.ClientConfig.CABundle) {
				pending = append(pending, fmt.Sprintf("ValidatingWebhookConfiguration/%s", webhookConfiguration.Name))
				break
			}
		}
	}

	for _, webhookConfiguration := range mutatingWebhookConfigurations.Items {
		if _, ok := webhookConfiguration.Annotations[injectCAFromAnnotation]; !ok {
			continue
		}
		for _, webhook := range webhookConfiguration.Webhooks {
			if !caBundleContainsCertificate(webhook.ClientConfig.CABundle) {
				pending = append(pending, fmt.Sprintf("MutatingWebhookConfiguration/%s", webhookConfiguration.Name))
				break
			}
		}
	}

	for _, crd := range crds.Items {
		if _, ok := crd.Annotations[injectCAFromAnnotation]; !ok {
			continue
		}
		if crd.Spec.Conversion == nil || crd.Spec.Conversion.Strategy != apiextensionsv1.WebhookConverter {
			continue
		}
		if crd.Spec.Conversion.Webhook == nil || crd.Spec.Conversion.Webhook.ClientConfig == nil ||
			!caBundleContainsCertificate(crd.Spec.Conversion.Webhook.ClientConfig.CABundle) {
			pending = append(pending, fmt.Sprintf("CustomResourceDefinition/%s", crd.Name))
		}
	}

	return pending
}

// caBundleContainsCertificate returns true if the given CA bundle contains at least one PEM-encoded
// certificate. This guards against both empty CA bundles and placeholder values like "\n"
// (caBundle: Cg==) which are used in webhook manifests before cert-manager injects the actual CA.
func caBundleContainsCertificate(caBundle []byte) bool {
	for block, rest := pem.Decode(caBundle); block != nil; block, rest = pem.Decode(rest) {
		if block.Type == "CERTIFICATE" {
			return true
		}
	}
	return false
}
