/*
Copyright 2022 The Kubernetes Authors.

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

package controllers

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/testcerts"
	utilfeature "k8s.io/component-base/featuregate/testing"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	internalruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	fakev1alpha1 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha1"
)

func Test_warmupRunnable_Start(t *testing.T) {
	g := NewWithT(t)
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	t.Run("succeed to warm up registry on Start", func(t *testing.T) {
		ns, err := env.CreateNamespace(ctx, "test-runtime-extension")
		g.Expect(err).ToNot(HaveOccurred())

		caCertSecret := fakeCASecret(ns.Name, "ext1-webhook", testcerts.CACert)
		// Create the secret which contains the fake ca certificate.
		g.Expect(env.CreateAndWait(ctx, caCertSecret)).To(Succeed())
		defer func() {
			g.Expect(env.CleanupAndWait(ctx, caCertSecret)).To(Succeed())
		}()

		cat := runtimecatalog.New()
		g.Expect(fakev1alpha1.AddToCatalog(cat)).To(Succeed())

		registry := runtimeregistry.New()
		g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())

		for _, name := range []string{"ext1", "ext2", "ext3"} {
			server, err := fakeSecureExtensionServer(discoveryHandler("first", "second", "third"))
			g.Expect(err).ToNot(HaveOccurred())
			defer server.Close()
			extensionConfig := fakeExtensionConfigForURL(ns.Name, name, server.URL)
			extensionConfig.Annotations[runtimev1.InjectCAFromSecretAnnotation] = caCertSecret.GetNamespace() + "/" + caCertSecret.GetName()

			// Create the ExtensionConfig.
			g.Expect(env.CreateAndWait(ctx, extensionConfig)).To(Succeed())
			defer func(namespace, name, url string) {
				g.Expect(env.CleanupAndWait(ctx, fakeExtensionConfigForURL(namespace, name, url))).To(Succeed())
			}(ns.Name, name, server.URL)
		}

		r := &warmupRunnable{
			Client:    env.GetClient(),
			APIReader: env.GetAPIReader(),
			RuntimeClient: internalruntimeclient.New(internalruntimeclient.Options{
				Catalog:  cat,
				Registry: registry,
			}),
		}

		if err := r.Start(ctx); err != nil {
			t.Error(err)
		}
		list := &runtimev1.ExtensionConfigList{}
		g.Expect(env.GetAPIReader().List(ctx, list)).To(Succeed())
		g.Expect(list.Items).To(HaveLen(3))
		for i, config := range list.Items {
			// Expect three handlers for each extension and expect the name to be the handler name plus the extension name.
			handlers := config.Status.Handlers
			g.Expect(handlers).To(HaveLen(3))
			g.Expect(handlers[0].Name).To(Equal(fmt.Sprintf("first.ext%d", i+1)))
			g.Expect(handlers[1].Name).To(Equal(fmt.Sprintf("second.ext%d", i+1)))
			g.Expect(handlers[2].Name).To(Equal(fmt.Sprintf("third.ext%d", i+1)))

			conditions := config.GetConditions()
			g.Expect(conditions).To(HaveLen(1))
			g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
			g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
		}
	})

	t.Run("fail to warm up registry on Start with broken extension", func(t *testing.T) {
		// This test should time out and throw a failure.
		ns, err := env.CreateNamespace(ctx, "test-runtime-extension")
		g.Expect(err).ToNot(HaveOccurred())

		caCertSecret := fakeCASecret(ns.Name, "ext1-webhook", testcerts.CACert)
		// Create the secret which contains the ca certificate.
		g.Expect(env.CreateAndWait(ctx, caCertSecret)).To(Succeed())
		defer func() {
			g.Expect(env.CleanupAndWait(ctx, caCertSecret)).To(Succeed())
		}()

		cat := runtimecatalog.New()
		g.Expect(fakev1alpha1.AddToCatalog(cat)).To(Succeed())
		registry := runtimeregistry.New()
		g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())

		// Do not create an extension server for an extension with this name, but do create an extensionconfig.
		brokenExtension := "ext2"
		for _, name := range []string{"ext1", "ext2", "ext3"} {
			if name == brokenExtension {
				g.Expect(env.CreateAndWait(ctx, fakeExtensionConfigForURL(ns.Name, name, "https://localhost:1234"))).To(Succeed())
				continue
			}
			server, err := fakeSecureExtensionServer(discoveryHandler("first", "second", "third"))
			g.Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			extensionConfig := fakeExtensionConfigForURL(ns.Name, name, server.URL)
			extensionConfig.Annotations[runtimev1.InjectCAFromSecretAnnotation] = caCertSecret.GetNamespace() + "/" + caCertSecret.GetName()

			// Create the ExtensionConfig.
			g.Expect(env.CreateAndWait(ctx, extensionConfig)).To(Succeed())
		}

		r := &warmupRunnable{
			Client:    env.GetClient(),
			APIReader: env.GetAPIReader(),
			RuntimeClient: internalruntimeclient.New(internalruntimeclient.Options{
				Catalog:  cat,
				Registry: registry,
			}),
			warmupInterval: 500 * time.Millisecond,
			warmupTimeout:  5 * time.Second,
		}

		if err := r.Start(ctx); err == nil {
			t.Error(errors.New("expected error on start up"))
		}
		list := &runtimev1.ExtensionConfigList{}
		g.Expect(env.GetAPIReader().List(ctx, list)).To(Succeed())
		g.Expect(list.Items).To(HaveLen(3))

		for i, config := range list.Items {
			handlers := config.Status.Handlers
			conditions := config.GetConditions()

			// Expect no handlers and a failed condition for the broken extension.
			if config.Name == brokenExtension {
				g.Expect(conditions).To(HaveLen(1))
				g.Expect(conditions[0].Status).To(Equal(corev1.ConditionFalse))
				g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
				g.Expect(handlers).To(BeEmpty())

				continue
			}

			// For other extensions expect handler name plus the extension name, and expect the condition to be True.
			g.Expect(handlers).To(HaveLen(3))
			g.Expect(handlers[0].Name).To(Equal(fmt.Sprintf("first.ext%d", i+1)))
			g.Expect(handlers[1].Name).To(Equal(fmt.Sprintf("second.ext%d", i+1)))
			g.Expect(handlers[2].Name).To(Equal(fmt.Sprintf("third.ext%d", i+1)))

			g.Expect(conditions).To(HaveLen(1))
			g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
			g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
		}
	})
}
