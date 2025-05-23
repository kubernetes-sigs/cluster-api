/*
Copyright 2024 The Kubernetes Authors.

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

package clusterctl

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const certManagerCAAnnotation = "cert-manager.io/inject-ca-from"

func verifyCAInjection(ctx context.Context, c client.Client) error {
	v := newCAInjectionVerifier(c)

	errs := v.verifyCustomResourceDefinitions(ctx)
	errs = append(errs, v.verifyMutatingWebhookConfigurations(ctx)...)
	errs = append(errs, v.verifyValidatingWebhookConfigurations(ctx)...)

	return kerrors.NewAggregate(errs)
}

// certificateInjectionVerifier waits for cert-managers ca-injector to inject the
// referred CA certificate to all CRDs, MutatingWebhookConfigurations and
// ValidatingWebhookConfigurations.
// As long as the correct CA certificates are not injected the kube-apiserver will
// reject the requests due to certificate verification errors.
type certificateInjectionVerifier struct {
	Client client.Client
}

// newCAInjectionVerifier creates a new CRD migrator.
func newCAInjectionVerifier(client client.Client) *certificateInjectionVerifier {
	return &certificateInjectionVerifier{
		Client: client,
	}
}

func (c *certificateInjectionVerifier) verifyCustomResourceDefinitions(ctx context.Context) []error {
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := c.Client.List(ctx, crds, client.HasLabels{clusterv1.ProviderNameLabel}); err != nil {
		return []error{err}
	}

	errs := []error{}
	for i := range crds.Items {
		crd := crds.Items[i]
		ca, err := c.getCACertificateFor(ctx, &crd)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ca == "" {
			continue
		}

		if crd.Spec.Conversion.Webhook == nil || crd.Spec.Conversion.Webhook.ClientConfig == nil {
			continue
		}

		if string(crd.Spec.Conversion.Webhook.ClientConfig.CABundle) != ca {
			changedCRD := crd.DeepCopy()
			changedCRD.Spec.Conversion.Webhook.ClientConfig.CABundle = nil
			errs = append(errs, fmt.Errorf("injected CA for CustomResourceDefinition %s does not match", crd.Name))
			if err := c.Client.Patch(ctx, changedCRD, client.MergeFrom(&crd)); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func (c *certificateInjectionVerifier) verifyMutatingWebhookConfigurations(ctx context.Context) []error {
	mutateHooks := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	if err := c.Client.List(ctx, mutateHooks, client.HasLabels{clusterv1.ProviderNameLabel}); err != nil {
		return []error{err}
	}

	errs := []error{}
	for i := range mutateHooks.Items {
		mutateHook := mutateHooks.Items[i]
		ca, err := c.getCACertificateFor(ctx, &mutateHook)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ca == "" {
			continue
		}

		var changed bool
		changedHook := mutateHook.DeepCopy()
		for i := range mutateHook.Webhooks {
			webhook := mutateHook.Webhooks[i]
			if string(webhook.ClientConfig.CABundle) != ca {
				changed = true
				webhook.ClientConfig.CABundle = nil
				changedHook.Webhooks[i] = webhook
				errs = append(errs, fmt.Errorf("injected CA for MutatingWebhookConfiguration %s hook %s does not match", mutateHook.Name, webhook.Name))
			}
		}
		if changed {
			if err := c.Client.Patch(ctx, changedHook, client.MergeFrom(&mutateHook)); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

func (c *certificateInjectionVerifier) verifyValidatingWebhookConfigurations(ctx context.Context) []error {
	validateHooks := &admissionregistrationv1.ValidatingWebhookConfigurationList{}
	if err := c.Client.List(ctx, validateHooks, client.HasLabels{clusterv1.ProviderNameLabel}); err != nil {
		return []error{err}
	}

	errs := []error{}
	for i := range validateHooks.Items {
		validateHook := validateHooks.Items[i]
		ca, err := c.getCACertificateFor(ctx, &validateHook)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ca == "" {
			continue
		}

		var changed bool
		changedHook := validateHook.DeepCopy()
		for i := range validateHook.Webhooks {
			webhook := validateHook.Webhooks[i]
			if string(webhook.ClientConfig.CABundle) != ca {
				changed = true
				webhook.ClientConfig.CABundle = nil
				changedHook.Webhooks[i] = webhook
				errs = append(errs, fmt.Errorf("injected CA for ValidatingWebhookConfiguration %s hook %s does not match", validateHook.Name, webhook.Name))
			}
		}
		if changed {
			if err := c.Client.Patch(ctx, changedHook, client.MergeFrom(&validateHook)); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errs
}

// getCACertificateFor returns the ca certificate from the secret referred by the
// Certificate object. It reads the namespaced name of the Certificate from the
// injection annotation of the passed object.
func (c *certificateInjectionVerifier) getCACertificateFor(ctx context.Context, obj client.Object) (string, error) {
	annotationValue, ok := obj.GetAnnotations()[certManagerCAAnnotation]
	if !ok || annotationValue == "" {
		return "", nil
	}

	certificateObjKey, err := splitObjectKey(annotationValue)
	if err != nil {
		return "", errors.Wrapf(err, "getting certificate object key for %s %s", obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
	}

	certificate := &unstructured.Unstructured{}
	certificate.SetKind("Certificate")
	certificate.SetAPIVersion("cert-manager.io/v1")

	if err := c.Client.Get(ctx, certificateObjKey, certificate); err != nil {
		return "", errors.Wrapf(err, "getting certificate %s for %s %s", certificateObjKey, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
	}

	secretName, _, err := unstructured.NestedString(certificate.Object, "spec", "secretName")
	if err != nil || secretName == "" {
		return "", errors.Wrapf(err, "reading .spec.secretName name from certificate %s for %s %s", certificateObjKey, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
	}

	secretObjKey := client.ObjectKey{Namespace: certificate.GetNamespace(), Name: secretName}
	certificateSecret := &corev1.Secret{}
	if err := c.Client.Get(ctx, secretObjKey, certificateSecret); err != nil {
		return "", errors.Wrapf(err, "getting secret %s for certificate %s for %s %s", secretObjKey, certificateObjKey, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
	}

	ca, ok := certificateSecret.Data["ca.crt"]
	if !ok {
		return "", errors.Errorf("data for \"ca.crt\" not found in secret %s for %s %s", secretObjKey, obj.GetObjectKind().GroupVersionKind().Kind, obj.GetName())
	}

	return string(ca), nil
}

// splitObjectKey splits the string by the name separator and returns it as client.ObjectKey.
func splitObjectKey(nameStr string) (client.ObjectKey, error) {
	splitPoint := strings.IndexRune(nameStr, types.Separator)
	if splitPoint == -1 {
		return client.ObjectKey{}, errors.Errorf("expected object key %s to contain namespace and name", nameStr)
	}
	return client.ObjectKey{Namespace: nameStr[:splitPoint], Name: nameStr[splitPoint+1:]}, nil
}
