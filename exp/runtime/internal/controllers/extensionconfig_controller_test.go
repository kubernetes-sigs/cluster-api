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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	"sigs.k8s.io/cluster-api/util"
)

func TestExtensionReconciler_Reconcile(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()

	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-extension-config")
	g.Expect(err).ToNot(HaveOccurred())

	cat := runtimecatalog.New()
	registry := runtimeregistry.New()
	runtimeClient := runtimeclient.New(runtimeclient.Options{
		Catalog:  cat,
		Registry: registry,
	})

	g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())

	r := &Reconciler{
		Client:        env.GetClient(),
		APIReader:     env.GetAPIReader(),
		RuntimeClient: runtimeClient,
	}

	server := fakeExtensionServer(discoveryHandler("first", "second", "third"))
	extensionConfig := fakeExtensionConfigForURL(ns.Name, "ext1", server.URL)
	defer server.Close()
	// Create the ExtensionConfig.
	g.Expect(env.CreateAndWait(ctx, extensionConfig)).To(Succeed())
	defer func() {
		g.Expect(env.CleanupAndWait(ctx, extensionConfig)).To(Succeed())
	}()
	t.Run("fail reconcile if registry has not been warmed up", func(t *testing.T) {
		// Attempt to reconcile. This will be an error as the registry has not been warmed up at this point.
		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).To(BeNil())
		// If the registry isn't warm the reconcile loop will return Requeue: True
		g.Expect(res.Requeue).To(Equal(true))
	})

	t.Run("successful reconcile and discovery on ExtensionConfig create", func(t *testing.T) {
		// Warm up the registry before trying reconciliation again.
		warmup := &warmupRunnable{
			Client:        env.GetClient(),
			APIReader:     env.GetAPIReader(),
			RuntimeClient: runtimeClient,
		}
		g.Expect(warmup.Start(ctx)).To(Succeed())

		// Reconcile the extension and assert discovery has succeeded.
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).To(BeNil())

		config := &runtimev1.ExtensionConfig{}
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(extensionConfig), config)).To(Succeed())

		// Expect three handlers for the extension and expect the name to be the handler name plus the extension name.
		handlers := config.Status.Handlers
		g.Expect(len(handlers)).To(Equal(3))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))
		g.Expect(handlers[1].Name).To(Equal("second.ext1"))
		g.Expect(handlers[2].Name).To(Equal("third.ext1"))

		conditions := config.GetConditions()
		g.Expect(len(conditions)).To(Equal(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
		_, err = registry.Get("first.ext1")
		g.Expect(err).To(BeNil())
		_, err = registry.Get("second.ext1")
		g.Expect(err).To(BeNil())
		_, err = registry.Get("third.ext1")
		g.Expect(err).To(BeNil())
	})

	t.Run("Successful reconcile and discovery on Extension update", func(t *testing.T) {
		// Start a new ExtensionServer where the second handler is removed.
		updatedServer := fakeExtensionServer(discoveryHandler("first", "third"))
		defer updatedServer.Close()
		// Close the original server  it's no longer serving.
		server.Close()

		// Patch the extension with the new server endpoint.
		patch := client.MergeFrom(extensionConfig.DeepCopy())
		extensionConfig.Spec.ClientConfig.URL = &updatedServer.URL

		g.Expect(env.Patch(ctx, extensionConfig, patch)).To(Succeed())

		// Wait until the object is updated in the client cache before continuing.
		g.Eventually(func() error {
			conf := &runtimev1.ExtensionConfig{}
			err := env.Get(ctx, util.ObjectKey(extensionConfig), conf)
			if err != nil {
				return err
			}
			if *conf.Spec.ClientConfig.URL != updatedServer.URL {
				return errors.Errorf("URL not set on updated object: got: %s, want: %s", *conf.Spec.ClientConfig.URL, updatedServer.URL)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(BeNil())

		// Reconcile the extension and assert discovery has succeeded.
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).To(BeNil())

		var config runtimev1.ExtensionConfig
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(extensionConfig), &config)).To(Succeed())

		// Expect two handlers for the extension and expect the name to be the handler name plus the extension name.
		handlers := config.Status.Handlers
		g.Expect(len(handlers)).To(Equal(2))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))
		g.Expect(handlers[1].Name).To(Equal("third.ext1"))
		conditions := config.GetConditions()
		g.Expect(len(conditions)).To(Equal(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))

		_, err = registry.Get("first.ext1")
		g.Expect(err).To(BeNil())
		_, err = registry.Get("third.ext1")
		g.Expect(err).To(BeNil())

		// Second should not be found in the registry:
		_, err = registry.Get("second.ext1")
		g.Expect(err).ToNot(BeNil())
	})
	t.Run("Successful reconcile and deregister on ExtensionConfig delete", func(t *testing.T) {
		g.Expect(env.CleanupAndWait(ctx, extensionConfig)).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(env.Get(ctx, util.ObjectKey(extensionConfig), extensionConfig)).To(Not(Succeed()))
		_, err = registry.Get("first.ext1")
		g.Expect(err).ToNot(BeNil())
		_, err = registry.Get("third.ext1")
		g.Expect(err).ToNot(BeNil())
	})
}

func TestExtensionReconciler_discoverExtensionConfig(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-runtime-extension")
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("test discovery of a single extension", func(t *testing.T) {
		cat := runtimecatalog.New()
		registry := runtimeregistry.New()
		g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())
		extensionName := "ext1"
		srv1 := fakeExtensionServer(discoveryHandler("first"))
		defer srv1.Close()

		runtimeClient := runtimeclient.New(runtimeclient.Options{
			Catalog:  cat,
			Registry: registry,
		})

		extensionConfig := fakeExtensionConfigForURL(ns.Name, extensionName, srv1.URL)

		discoveredExtensionConfig, err := discoverExtensionConfig(ctx, runtimeClient, extensionConfig)
		g.Expect(err).To(BeNil())

		// Expect exactly one handler and expect the name to be the handler name plus the extension name.
		handlers := discoveredExtensionConfig.Status.Handlers
		g.Expect(len(handlers)).To(Equal(1))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))

		// Expect exactly one condition and expect the condition to have type RuntimeExtensionDiscoveredCondition and
		// Status true.
		conditions := discoveredExtensionConfig.GetConditions()
		g.Expect(len(conditions)).To(Equal(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
	})
	t.Run("fail discovery for non-running extension", func(t *testing.T) {
		cat := runtimecatalog.New()
		registry := runtimeregistry.New()
		g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())
		extensionName := "ext1"

		// Don't set up a server to run the extensionDiscovery handler.
		// srv1 := fakeExtensionServer(discoveryHandler("first"))
		// defer srv1.Close()

		runtimeClient := runtimeclient.New(runtimeclient.Options{
			Catalog:  cat,
			Registry: registry,
		})

		extensionConfig := fakeExtensionConfigForURL(ns.Name, extensionName, "http://localhost:31239")

		discoveredExtensionConfig, err := discoverExtensionConfig(ctx, runtimeClient, extensionConfig)
		g.Expect(err).ToNot(BeNil())

		// Expect exactly one handler and expect the name to be the handler name plus the extension name.
		handlers := discoveredExtensionConfig.Status.Handlers
		g.Expect(len(handlers)).To(Equal(0))

		// Expect exactly one condition and expect the condition to have type RuntimeExtensionDiscoveredCondition and
		// Status false.
		conditions := discoveredExtensionConfig.GetConditions()
		g.Expect(len(conditions)).To(Equal(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionFalse))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
	})
}

func discoveryHandler(handlerList ...string) func(http.ResponseWriter, *http.Request) {
	handlers := []runtimehooksv1.ExtensionHandler{}
	for _, name := range handlerList {
		handlers = append(handlers, runtimehooksv1.ExtensionHandler{
			Name: name,
			RequestHook: runtimehooksv1.GroupVersionHook{
				Hook:       name,
				APIVersion: runtimehooksv1.GroupVersion.String(),
			},
		})
	}
	response := &runtimehooksv1.DiscoveryResponse{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DiscoveryResponse",
			APIVersion: runtimehooksv1.GroupVersion.String(),
		},
		Handlers: handlers,
	}
	respBody, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}
}

func fakeExtensionConfigForURL(namespace, name, url string) *runtimev1.ExtensionConfig {
	return &runtimev1.ExtensionConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtensionConfig",
			APIVersion: runtimehooksv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String(url),
			},
			NamespaceSelector: nil,
		},
	}
}

func fakeExtensionServer(discoveryHandler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", discoveryHandler)
	srv := httptest.NewServer(mux)
	return srv
}
