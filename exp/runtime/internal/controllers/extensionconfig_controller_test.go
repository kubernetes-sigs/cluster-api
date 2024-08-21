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
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/testcerts"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/feature"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	fakev1alpha1 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
)

func TestExtensionReconciler_Reconcile(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)

	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-extension-config")
	g.Expect(err).ToNot(HaveOccurred())

	cat := runtimecatalog.New()
	g.Expect(fakev1alpha1.AddToCatalog(cat)).To(Succeed())

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

	caCertSecret := fakeCASecret(ns.Name, "ext1-webhook", testcerts.CACert)
	server, err := fakeSecureExtensionServer(discoveryHandler("first", "second", "third"))
	g.Expect(err).ToNot(HaveOccurred())
	defer server.Close()
	extensionConfig := fakeExtensionConfigForURL(ns.Name, "ext1", server.URL)
	extensionConfig.Annotations[runtimev1.InjectCAFromSecretAnnotation] = caCertSecret.GetNamespace() + "/" + caCertSecret.GetName()

	// Create the secret which contains the ca certificate.
	g.Expect(env.CreateAndWait(ctx, caCertSecret)).To(Succeed())
	defer func() {
		g.Expect(env.CleanupAndWait(ctx, caCertSecret)).To(Succeed())
	}()
	// Create the ExtensionConfig.
	g.Expect(env.CreateAndWait(ctx, extensionConfig)).To(Succeed())
	defer func() {
		g.Expect(env.CleanupAndWait(ctx, extensionConfig)).To(Succeed())
	}()
	t.Run("fail reconcile if registry has not been warmed up", func(*testing.T) {
		// Attempt to reconcile. This will be an error as the registry has not been warmed up at this point.
		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())
		// If the registry isn't warm the reconcile loop will return Requeue: True
		g.Expect(res.Requeue).To(BeTrue())
	})

	t.Run("successful reconcile and discovery on ExtensionConfig create", func(*testing.T) {
		// Warm up the registry before trying reconciliation again.
		warmup := &warmupRunnable{
			Client:        env.GetClient(),
			APIReader:     env.GetAPIReader(),
			RuntimeClient: runtimeClient,
		}
		g.Expect(warmup.Start(ctx)).To(Succeed())

		// Reconcile the extension and assert discovery has succeeded.
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())

		config := &runtimev1.ExtensionConfig{}
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(extensionConfig), config)).To(Succeed())

		// Expect three handlers for the extension and expect the name to be the handler name plus the extension name.
		handlers := config.Status.Handlers
		g.Expect(handlers).To(HaveLen(3))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))
		g.Expect(handlers[1].Name).To(Equal("second.ext1"))
		g.Expect(handlers[2].Name).To(Equal("third.ext1"))

		conditions := config.GetConditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
		_, err = registry.Get("first.ext1")
		g.Expect(err).ToNot(HaveOccurred())
		_, err = registry.Get("second.ext1")
		g.Expect(err).ToNot(HaveOccurred())
		_, err = registry.Get("third.ext1")
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("Successful reconcile and discovery on Extension update", func(*testing.T) {
		// Start a new ExtensionServer where the second handler is removed.
		updatedServer, err := fakeSecureExtensionServer(discoveryHandler("first", "third"))
		g.Expect(err).ToNot(HaveOccurred())
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
		g.Expect(err).ToNot(HaveOccurred())

		var config runtimev1.ExtensionConfig
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(extensionConfig), &config)).To(Succeed())

		// Expect two handlers for the extension and expect the name to be the handler name plus the extension name.
		handlers := config.Status.Handlers
		g.Expect(handlers).To(HaveLen(2))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))
		g.Expect(handlers[1].Name).To(Equal("third.ext1"))
		conditions := config.GetConditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))

		_, err = registry.Get("first.ext1")
		g.Expect(err).ToNot(HaveOccurred())
		_, err = registry.Get("third.ext1")
		g.Expect(err).ToNot(HaveOccurred())

		// Second should not be found in the registry:
		_, err = registry.Get("second.ext1")
		g.Expect(err).To(HaveOccurred())
	})
	t.Run("Successful reconcile and deregister on ExtensionConfig delete", func(*testing.T) {
		g.Expect(env.CleanupAndWait(ctx, extensionConfig)).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(env.Get(ctx, util.ObjectKey(extensionConfig), extensionConfig)).To(Not(Succeed()))
		_, err = registry.Get("first.ext1")
		g.Expect(err).To(HaveOccurred())
		_, err = registry.Get("third.ext1")
		g.Expect(err).To(HaveOccurred())
	})
}

func TestExtensionReconciler_discoverExtensionConfig(t *testing.T) {
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)
	utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)
	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-runtime-extension")
	g.Expect(err).ToNot(HaveOccurred())

	t.Run("test discovery of a single extension", func(*testing.T) {
		cat := runtimecatalog.New()
		g.Expect(fakev1alpha1.AddToCatalog(cat)).To(Succeed())

		registry := runtimeregistry.New()
		g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())
		extensionName := "ext1"
		srv1, err := fakeSecureExtensionServer(discoveryHandler("first"))
		g.Expect(err).ToNot(HaveOccurred())
		defer srv1.Close()

		runtimeClient := runtimeclient.New(runtimeclient.Options{
			Catalog:  cat,
			Registry: registry,
		})

		extensionConfig := fakeExtensionConfigForURL(ns.Name, extensionName, srv1.URL)
		extensionConfig.Spec.ClientConfig.CABundle = testcerts.CACert

		discoveredExtensionConfig, err := discoverExtensionConfig(ctx, runtimeClient, extensionConfig)
		g.Expect(err).ToNot(HaveOccurred())

		// Expect exactly one handler and expect the name to be the handler name plus the extension name.
		handlers := discoveredExtensionConfig.Status.Handlers
		g.Expect(handlers).To(HaveLen(1))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))

		// Expect exactly one condition and expect the condition to have type RuntimeExtensionDiscoveredCondition and
		// Status true.
		conditions := discoveredExtensionConfig.GetConditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
	})
	t.Run("fail discovery for non-running extension", func(*testing.T) {
		cat := runtimecatalog.New()
		g.Expect(fakev1alpha1.AddToCatalog(cat)).To(Succeed())
		registry := runtimeregistry.New()
		g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())
		extensionName := "ext1"

		// Don't set up a server to run the extensionDiscovery handler.
		// srv1 := fakeSecureExtensionServer(discoveryHandler("first"))
		// defer srv1.Close()

		runtimeClient := runtimeclient.New(runtimeclient.Options{
			Catalog:  cat,
			Registry: registry,
		})

		extensionConfig := fakeExtensionConfigForURL(ns.Name, extensionName, "https://localhost:31239")
		extensionConfig.Spec.ClientConfig.CABundle = testcerts.CACert

		discoveredExtensionConfig, err := discoverExtensionConfig(ctx, runtimeClient, extensionConfig)
		g.Expect(err).To(HaveOccurred())

		// Expect exactly one handler and expect the name to be the handler name plus the extension name.
		handlers := discoveredExtensionConfig.Status.Handlers
		g.Expect(handlers).To(BeEmpty())

		// Expect exactly one condition and expect the condition to have type RuntimeExtensionDiscoveredCondition and
		// Status false.
		conditions := discoveredExtensionConfig.GetConditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionFalse))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredCondition))
	})
}

func Test_reconcileCABundle(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

	tests := []struct {
		name         string
		client       client.Client
		config       *runtimev1.ExtensionConfig
		wantCABundle []byte
		wantErr      bool
	}{
		{
			name:    "No-op because no annotation is set",
			client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
			config:  fakeCAInjectionRuntimeExtensionConfig("some-namespace", "some-extension-config", "", ""),
			wantErr: false,
		},
		{
			name: "Inject ca-bundle",
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				fakeCASecret("some-namespace", "some-ca-secret", []byte("some-ca-data")),
			).Build(),
			config:       fakeCAInjectionRuntimeExtensionConfig("some-namespace", "some-extension-config", "some-namespace/some-ca-secret", ""),
			wantCABundle: []byte(`some-ca-data`),
			wantErr:      false,
		},
		{
			name: "Update ca-bundle",
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				fakeCASecret("some-namespace", "some-ca-secret", []byte("some-new-data")),
			).Build(),
			config:       fakeCAInjectionRuntimeExtensionConfig("some-namespace", "some-extension-config", "some-namespace/some-ca-secret", "some-old-ca-data"),
			wantCABundle: []byte(`some-new-data`),
			wantErr:      false,
		},
		{
			name:    "Fail because secret does not exist",
			client:  fake.NewClientBuilder().WithScheme(scheme).WithObjects().Build(),
			config:  fakeCAInjectionRuntimeExtensionConfig("some-namespace", "some-extension-config", "some-namespace/some-ca-secret", ""),
			wantErr: true,
		},
		{
			name: "Fail because secret does not contain a ca.crt",
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				fakeCASecret("some-namespace", "some-ca-secret", nil),
			).Build(),
			config:  fakeCAInjectionRuntimeExtensionConfig("some-namespace", "some-extension-config", "some-namespace/some-ca-secret", ""),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := reconcileCABundle(context.TODO(), tt.client, tt.config)
			g.Expect(err != nil).To(Equal(tt.wantErr))

			g.Expect(tt.config.Spec.ClientConfig.CABundle).To(Equal(tt.wantCABundle))
		})
	}
}

func discoveryHandler(handlerList ...string) func(http.ResponseWriter, *http.Request) {
	handlers := []runtimehooksv1.ExtensionHandler{}
	for _, name := range handlerList {
		handlers = append(handlers, runtimehooksv1.ExtensionHandler{
			Name: name,
			RequestHook: runtimehooksv1.GroupVersionHook{
				Hook:       "FakeHook",
				APIVersion: fakev1alpha1.GroupVersion.String(),
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

	return func(w http.ResponseWriter, _ *http.Request) {
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
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: ptr.To(url),
			},
			NamespaceSelector: nil,
		},
	}
}

func fakeSecureExtensionServer(discoveryHandler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", discoveryHandler)

	sCert, err := tls.X509KeyPair(testcerts.ServerCert, testcerts.ServerKey)
	if err != nil {
		return nil, err
	}
	testServer := httptest.NewUnstartedServer(mux)
	testServer.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{sCert},
	}
	testServer.StartTLS()

	return testServer, nil
}

func fakeCASecret(namespace, name string, caData []byte) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{},
	}
	if caData != nil {
		secret.Data["ca.crt"] = caData
	}
	return secret
}

func fakeCAInjectionRuntimeExtensionConfig(namespace, name, annotationString, caBundleData string) *runtimev1.ExtensionConfig {
	ext := &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
	}
	if annotationString != "" {
		ext.Annotations[runtimev1.InjectCAFromSecretAnnotation] = annotationString
	}
	if caBundleData != "" {
		ext.Spec.ClientConfig.CABundle = []byte(caBundleData)
	}
	return ext
}
