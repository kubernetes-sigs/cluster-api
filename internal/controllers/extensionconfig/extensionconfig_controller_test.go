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

package extensionconfig

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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/feature"
	internalruntimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	fakev1alpha1 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
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
	runtimeClient, _, err := internalruntimeclient.New(internalruntimeclient.Options{
		Catalog:  cat,
		Registry: registry,
	})
	g.Expect(err).ToNot(HaveOccurred())
	registryReadOnly := runtimeregistry.New()
	runtimeClientReadOnly, _, err := internalruntimeclient.New(internalruntimeclient.Options{
		Catalog:  cat,
		Registry: registryReadOnly,
	})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())

	r := &Reconciler{
		Client:        env.GetAPIReader().(client.Client),
		APIReader:     env.GetAPIReader(),
		RuntimeClient: runtimeClient,
	}
	rReadOnly := &Reconciler{
		Client:        env.GetAPIReader().(client.Client),
		APIReader:     env.GetAPIReader(),
		RuntimeClient: runtimeClientReadOnly,
		ReadOnly:      true,
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

	t.Run("fail reconcile if registry has not been warmed up", func(*testing.T) {
		// Attempt to reconcile. This will be an error as the registry has not been warmed up at this point.
		res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())
		// If the registry isn't warm the reconcile loop will return Requeue: True
		g.Expect(res.RequeueAfter).To((Equal(10 * time.Second)))
	})

	t.Run("successful reconcile and discovery on ExtensionConfig create", func(*testing.T) {
		// Warm up the registry before trying reconciliation again (these are no-ops as ExtensionConfig doesn't exist yet).
		warmup := &warmupRunnable{
			Client:        env.GetAPIReader().(client.Client),
			APIReader:     env.GetAPIReader(),
			RuntimeClient: runtimeClient,
		}
		g.Expect(warmup.Start(ctx)).To(Succeed())
		runtimeClientReadOnly, _, err := internalruntimeclient.New(internalruntimeclient.Options{
			Catalog:  cat,
			Registry: registryReadOnly,
		})
		g.Expect(err).ToNot(HaveOccurred())
		warmupReadOnly := &warmupRunnable{
			ReadOnly:      true,
			warmupTimeout: 3 * time.Second, // Use a short timeout so the test doesn't take too long
			Client:        env.GetAPIReader().(client.Client),
			APIReader:     env.GetAPIReader(),
			RuntimeClient: runtimeClientReadOnly,
		}
		g.Expect(warmupReadOnly.Start(ctx)).To(Succeed())

		// Create the ExtensionConfig.
		g.Expect(env.CreateAndWait(ctx, extensionConfig)).To(Succeed())
		t.Cleanup(func() {
			g.Expect(env.CleanupAndWait(ctx, extensionConfig)).To(Succeed())
		})

		// Reconcile the extension and assert discovery has succeeded (the first reconcile adds the Paused condition).
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())

		// Wait until the ExtensionConfig in the cache has the Paused condition so the next Reconcile can do discovery.
		g.Eventually(func(g Gomega) {
			conf := &runtimev1.ExtensionConfig{}
			g.Expect(env.Get(ctx, util.ObjectKey(extensionConfig), conf)).To(Succeed())
			pausedCondition := conditions.Get(conf, clusterv1.PausedCondition)
			g.Expect(pausedCondition).ToNot(BeNil())
			g.Expect(pausedCondition.ObservedGeneration).To(Equal(conf.Generation))
		}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())

		// Initially Reconcile on the read-only Reconciler should fail if ExtensionConfig has not been reconciled yet.
		_, err = rReadOnly.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("failed to validate ExtensionConfig: caBundle is not set on ExtensionConfig ext1"))

		// Reconcile should succeed on the regular Reconciler.
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())

		// Regular registry should have the handlers.
		_, err = registry.Get("first.ext1")
		g.Expect(err).ToNot(HaveOccurred())
		_, err = registry.Get("second.ext1")
		g.Expect(err).ToNot(HaveOccurred())
		_, err = registry.Get("third.ext1")
		g.Expect(err).ToNot(HaveOccurred())

		// Read-only registry should not have the handlers yet
		_, err = registryReadOnly.Get("first.ext1")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("failed to get extension handler \"first.ext1\" from registry: handler with name \"first.ext1\" has not been registered"))

		// Reconcile should succeed on the read-only Reconciler now as well.
		_, err = rReadOnly.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())

		// Read-only registry should have the handlers now as well.
		_, err = registryReadOnly.Get("first.ext1")
		g.Expect(err).ToNot(HaveOccurred())
		_, err = registryReadOnly.Get("second.ext1")
		g.Expect(err).ToNot(HaveOccurred())
		_, err = registryReadOnly.Get("third.ext1")
		g.Expect(err).ToNot(HaveOccurred())

		config := &runtimev1.ExtensionConfig{}
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(extensionConfig), config)).To(Succeed())

		// Expect three handlers for the extension and expect the name to be the handler name plus the extension name.
		handlers := config.Status.Handlers
		g.Expect(handlers).To(HaveLen(3))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))
		g.Expect(handlers[1].Name).To(Equal("second.ext1"))
		g.Expect(handlers[2].Name).To(Equal("third.ext1"))

		conditions := config.GetV1Beta1Conditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredV1Beta1Condition))

		v1beta2Conditions := config.GetConditions()
		g.Expect(v1beta2Conditions).To(HaveLen(2)) // Second condition is paused.
		g.Expect(v1beta2Conditions[0].Type).To(Equal(runtimev1.ExtensionConfigDiscoveredCondition))
		g.Expect(v1beta2Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		g.Expect(v1beta2Conditions[0].Reason).To(Equal(runtimev1.ExtensionConfigDiscoveredReason))
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
		extensionConfig.Spec.ClientConfig.URL = updatedServer.URL

		g.Expect(env.Patch(ctx, extensionConfig, patch)).To(Succeed())

		// Wait until the object is updated in the client cache before continuing.
		g.Eventually(func() error {
			conf := &runtimev1.ExtensionConfig{}
			err := env.Get(ctx, util.ObjectKey(extensionConfig), conf)
			if err != nil {
				return err
			}
			if conf.Spec.ClientConfig.URL != updatedServer.URL {
				return errors.Errorf("URL not set on updated object: got: %s, want: %s", conf.Spec.ClientConfig.URL, updatedServer.URL)
			}
			return nil
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		// Reconcile the extension and assert discovery has succeeded.
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())

		_, err = rReadOnly.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())

		for _, registry := range []runtimeregistry.ExtensionRegistry{registry, registryReadOnly} {
			_, err := registry.Get("first.ext1")
			g.Expect(err).ToNot(HaveOccurred())
			_, err = registry.Get("third.ext1")
			g.Expect(err).ToNot(HaveOccurred())

			// Second should not be found in the registry, as it has been removed from the ExtensionServer.
			_, err = registry.Get("second.ext1")
			g.Expect(err).To(HaveOccurred())
		}

		var config runtimev1.ExtensionConfig
		g.Expect(env.GetAPIReader().Get(ctx, util.ObjectKey(extensionConfig), &config)).To(Succeed())

		// Expect two handlers for the extension and expect the name to be the handler name plus the extension name.
		handlers := config.Status.Handlers
		g.Expect(handlers).To(HaveLen(2))
		g.Expect(handlers[0].Name).To(Equal("first.ext1"))
		g.Expect(handlers[1].Name).To(Equal("third.ext1"))
		conditions := config.GetV1Beta1Conditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredV1Beta1Condition))

		v1beta2Conditions := config.GetConditions()
		g.Expect(v1beta2Conditions).To(HaveLen(2)) // Second condition is paused.
		g.Expect(v1beta2Conditions[0].Type).To(Equal(runtimev1.ExtensionConfigDiscoveredCondition))
		g.Expect(v1beta2Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		g.Expect(v1beta2Conditions[0].Reason).To(Equal(runtimev1.ExtensionConfigDiscoveredReason))
	})
	t.Run("Successful reconcile and deregister on ExtensionConfig delete", func(*testing.T) {
		g.Expect(env.CleanupAndWait(ctx, extensionConfig)).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())
		_, err = rReadOnly.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(extensionConfig)})
		g.Expect(err).ToNot(HaveOccurred())

		for _, registry := range []runtimeregistry.ExtensionRegistry{registry, registryReadOnly} {
			g.Expect(env.Get(ctx, util.ObjectKey(extensionConfig), extensionConfig)).To(Not(Succeed()))
			_, err = registry.Get("first.ext1")
			g.Expect(err).To(HaveOccurred())
			_, err = registry.Get("third.ext1")
			g.Expect(err).To(HaveOccurred())
		}
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

		runtimeClient, _, err := internalruntimeclient.New(internalruntimeclient.Options{
			Catalog:  cat,
			Registry: registry,
		})
		g.Expect(err).ToNot(HaveOccurred())

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
		conditions := discoveredExtensionConfig.GetV1Beta1Conditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionTrue))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredV1Beta1Condition))

		v1beta2Conditions := discoveredExtensionConfig.GetConditions()
		g.Expect(v1beta2Conditions).To(HaveLen(1))
		g.Expect(v1beta2Conditions[0].Type).To(Equal(runtimev1.ExtensionConfigDiscoveredCondition))
		g.Expect(v1beta2Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		g.Expect(v1beta2Conditions[0].Reason).To(Equal(runtimev1.ExtensionConfigDiscoveredReason))
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

		runtimeClient, _, err := internalruntimeclient.New(internalruntimeclient.Options{
			Catalog:  cat,
			Registry: registry,
		})
		g.Expect(err).ToNot(HaveOccurred())

		extensionConfig := fakeExtensionConfigForURL(ns.Name, extensionName, "https://localhost:31239")
		extensionConfig.Spec.ClientConfig.CABundle = testcerts.CACert

		discoveredExtensionConfig, err := discoverExtensionConfig(ctx, runtimeClient, extensionConfig)
		g.Expect(err).To(HaveOccurred())

		// Expect exactly one handler and expect the name to be the handler name plus the extension name.
		handlers := discoveredExtensionConfig.Status.Handlers
		g.Expect(handlers).To(BeEmpty())

		// Expect exactly one condition and expect the condition to have type RuntimeExtensionDiscoveredCondition and
		// Status false.
		conditions := discoveredExtensionConfig.GetV1Beta1Conditions()
		g.Expect(conditions).To(HaveLen(1))
		g.Expect(conditions[0].Status).To(Equal(corev1.ConditionFalse))
		g.Expect(conditions[0].Type).To(Equal(runtimev1.RuntimeExtensionDiscoveredV1Beta1Condition))

		v1beta2Conditions := discoveredExtensionConfig.GetConditions()
		g.Expect(v1beta2Conditions).To(HaveLen(1))
		g.Expect(v1beta2Conditions[0].Type).To(Equal(runtimev1.ExtensionConfigDiscoveredCondition))
		g.Expect(v1beta2Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		g.Expect(v1beta2Conditions[0].Reason).To(Equal(runtimev1.ExtensionConfigNotDiscoveredReason))
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

func Test_validateExtensionConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         *runtimev1.ExtensionConfig
		wantErr        bool
		wantErrMessage string
	}{
		{
			name:           "caBundle not set",
			config:         extensionConfig(nil),
			wantErr:        true,
			wantErrMessage: "caBundle is not set on ExtensionConfig default/extensionconfig",
		},
		{
			name:           "condition missing",
			config:         extensionConfig([]byte("caBundle")),
			wantErr:        true,
			wantErrMessage: "Discovered condition not yet set on ExtensionConfig default/extensionconfig",
		},
		{
			name: "condition false",
			config: extensionConfig([]byte("caBundle"), metav1.Condition{
				Type:               runtimev1.ExtensionConfigDiscoveredCondition,
				Status:             metav1.ConditionFalse,
				Reason:             runtimev1.ExtensionConfigNotDiscoveredReason,
				ObservedGeneration: 1, // ExtensionConfig has generation 1.
			}),
			wantErr:        true,
			wantErrMessage: "Discovered condition on ExtensionConfig default/extensionconfig must have status: True (instead it has: False)",
		},
		{
			name: "condition observedGeneration outdated",
			config: extensionConfig([]byte("caBundle"), metav1.Condition{
				Type:               runtimev1.ExtensionConfigDiscoveredCondition,
				Status:             metav1.ConditionTrue,
				Reason:             runtimev1.ExtensionConfigDiscoveredReason,
				ObservedGeneration: 0, // ExtensionConfig has generation 1.
			}),
			wantErr:        true,
			wantErrMessage: "Discovered condition on ExtensionConfig default/extensionconfig must have observedGeneration: 1 (instead it has: 0)",
		},
		{
			name: "All good",
			config: extensionConfig([]byte("caBundle"), metav1.Condition{
				Type:               runtimev1.ExtensionConfigDiscoveredCondition,
				Status:             metav1.ConditionTrue,
				Reason:             runtimev1.ExtensionConfigDiscoveredReason,
				ObservedGeneration: 1, // ExtensionConfig has generation 1.
			}),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := validateExtensionConfig(tt.config)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.wantErrMessage))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
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
		CommonResponse: runtimehooksv1.CommonResponse{
			Status: runtimehooksv1.ResponseStatusSuccess,
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
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: url,
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

func extensionConfig(caBundle []byte, conditions ...metav1.Condition) *runtimev1.ExtensionConfig {
	return &runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "extensionconfig",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				CABundle: caBundle,
			},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Conditions: conditions,
		},
	}
}
