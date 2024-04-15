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

package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/repository"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

const certManagerCAAnnotation = "cert-manager.io/inject-ca-from"

func waitForCAInjection(ctx context.Context, installQueue []repository.Components, proxy Proxy) error {
	c, err := proxy.NewClient(ctx)
	if err != nil {
		return err
	}
	caInjectionVerifier := newCAInjectionVerifier(c)

	return caInjectionVerifier.Run(ctx, installQueue)
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

// Run runs the verification for all objects passed from the installQueue.
func (c *certificateInjectionVerifier) Run(ctx context.Context, installQueue []repository.Components) error {
	annotatedObjects := []*unstructured.Unstructured{}
	for _, components := range installQueue {
		for _, obj := range components.Objs() {
			obj := obj
			if value, ok := obj.GetAnnotations()[certManagerCAAnnotation]; ok {
				_ = value
				annotatedObjects = append(annotatedObjects, &obj)
			}
		}
	}

	start := time.Now()
	log := logf.Log
	log.V(2).Info("Verifying CA injection for objects")

	if err := retryWithExponentialBackoff(ctx, newVerifyBackoff(), func(ctx context.Context) error {
		var errs []error
		for _, obj := range annotatedObjects {
			errs = []error{}
			if err := c.checkObject(ctx, *obj); err != nil {
				errs = append(errs, err)
			}
		}

		return kerrors.NewAggregate(errs)
	}); err != nil {
		return err
	}

	log.V(2).Info("CA injection verification for objects passed", "duration", time.Since(start).String())

	return nil
}

// getCACertificateFor returns the ca certificate from the secret referred by the
// Certificate object. It reads the namespaced name of the Certificate from the
// injection annotation of the passed object.
func (c *certificateInjectionVerifier) getCACertificateFor(ctx context.Context, obj unstructured.Unstructured) (string, error) {
	annotationValue, ok := obj.GetAnnotations()[certManagerCAAnnotation]
	if !ok || annotationValue == "" {
		return "", fmt.Errorf("getting value for injection annotation")
	}

	certificateObjKey := splitObjectKey(annotationValue)

	certificate := &unstructured.Unstructured{}
	certificate.SetKind("Certificate")
	certificate.SetAPIVersion("cert-manager.io/v1")

	if err := c.Client.Get(ctx, certificateObjKey, certificate); err != nil {
		return "", errors.Wrapf(err, "getting certificate %s", certificateObjKey)
	}

	secretName, _, err := unstructured.NestedString(certificate.Object, "spec", "secretName")
	if err != nil || secretName == "" {
		return "", errors.Wrapf(err, "reading .spec.secretName name from certificate %s", certificateObjKey)
	}

	secretObjKey := client.ObjectKey{Namespace: certificate.GetNamespace(), Name: secretName}
	certificateSecret := &corev1.Secret{}
	if err := c.Client.Get(ctx, secretObjKey, certificateSecret); err != nil {
		return "", errors.Wrapf(err, "getting secret %s", &certificateObjKey)
	}

	ca, ok := certificateSecret.Data["ca.crt"]
	if !ok {
		return "", errors.Errorf("data for \"ca.crt\" not found in secret %s", secretObjKey)
	}

	return string(ca), nil
}

// splitObjectKey splits the string by the name separator and returns it as client.ObjectKey.
func splitObjectKey(nameStr string) client.ObjectKey {
	splitPoint := strings.IndexRune(nameStr, types.Separator)
	if splitPoint == -1 {
		return client.ObjectKey{Name: nameStr}
	}
	return client.ObjectKey{Namespace: nameStr[:splitPoint], Name: nameStr[splitPoint+1:]}
}

// checkObject gets the desired CA certificate and compares it relevant field on
// the object which is first read from kube-apiserver.
func (c *certificateInjectionVerifier) checkObject(ctx context.Context, obj unstructured.Unstructured) error {
	// get the CA certificate from the Certificate's secret which is referred in an
	// annotation at the object.
	ca, err := c.getCACertificateFor(ctx, obj)
	if err != nil {
		return err
	}

	// Build the object key from the passed object.
	objKey := client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}

	// Get the object and assert the CA certificate.
	switch obj.GetKind() {
	case customResourceDefinitionKind:
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := c.Client.Get(ctx, objKey, crd); err != nil {
			return err
		}

		if crd.Spec.Conversion.Webhook == nil || crd.Spec.Conversion.Webhook.ClientConfig == nil || string(crd.Spec.Conversion.Webhook.ClientConfig.CABundle) != ca {
			return fmt.Errorf("injected CA for CustomResourceDefinition %s does not match", objKey)
		}
	case mutatingWebhookConfigurationKind:
		mutatingWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{}
		if err := c.Client.Get(ctx, objKey, mutatingWebhook); err != nil {
			return err
		}
		for _, webhook := range mutatingWebhook.Webhooks {
			if string(webhook.ClientConfig.CABundle) != ca {
				return fmt.Errorf("injected CA for MutatingWebhookConfiguration %s does not match", objKey)
			}
		}
	case validatingWebhookConfigurationKind:
		validatingWebhook := &admissionregistrationv1.ValidatingWebhookConfiguration{}
		if err := c.Client.Get(ctx, objKey, validatingWebhook); err != nil {
			return err
		}
		for _, webhook := range validatingWebhook.Webhooks {
			if string(webhook.ClientConfig.CABundle) != ca {
				return fmt.Errorf("injected CA for ValidatingWebhookConfiguration %s does not match", objKey)
			}
		}
	default:
		return fmt.Errorf("unknown object type %s", obj.GetKind())
	}

	return nil
}

// newVerifyBackoff creates a new API Machinery backoff parameter set suitable for use with clusterctl verify operations.
func newVerifyBackoff() wait.Backoff {
	// Return a exponential backoff configuration which returns durations for a total time of ~5m.
	// Example: 0, .5s, 1.2s, 2.3s, 4s, 6s, 10s, 16s, 24s, 37s, 57s, 85s, 129s, 194s, 291s
	// Jitter is added as a random fraction of the duration multiplied by the jitter factor.
	return wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   1.5,
		Steps:    14,
		Jitter:   0.4,
	}
}
