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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	hooksv2 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha2"
	hooksv3 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha3"
	"sigs.k8s.io/cluster-api/feature"
	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/internal/runtime/client"
)

func TestExtensionReconciler_Reconcile(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.RuntimeSDK, true)()

	g := NewWithT(t)
	ns, err := env.CreateNamespace(ctx, "test-runtime-extension")
	g.Expect(err).ToNot(HaveOccurred())
	workingExtension1 := &runtimev1.ExtensionConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Extension",
			APIVersion: "runtime.cluster.x-k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "working-1",
			Namespace: ns.Name,
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String("https://extension-address.com"),
			},
		},
	}
	workingExtension2 := workingExtension1.DeepCopy()
	workingExtension2.Name = "working-2"
	brokenExtension := &runtimev1.ExtensionConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Extension",
			APIVersion: "runtime.cluster.x-k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-extension",
			Namespace: ns.Name,
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String("https://extension-address.com"),
			},
			NamespaceSelector: nil,
		},
	}
	extensionWithDiscovery1 := &runtimev1.ExtensionConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Extension",
			APIVersion: "runtime.cluster.x-k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extension-with-discovery",
			Namespace: ns.Name,
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig:      runtimev1.ClientConfig{},
			NamespaceSelector: nil,
		},
	}

	extensionWithDiscovery2 := &runtimev1.ExtensionConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Extension",
			APIVersion: "runtime.cluster.x-k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "extension-with-discovery-but-again",
			Namespace: ns.Name,
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig:      runtimev1.ClientConfig{},
			NamespaceSelector: nil,
		},
	}
	hookStore := map[string][]runtimev1.ExtensionHandler{
		namespacedName(workingExtension1).String(): {
			runtimev1.ExtensionHandler{
				Name: "first",
				RequestHook: runtimev1.GroupVersionHook{
					APIVersion: "v1alpha1",
					Hook:       "first",
				},
			},
		},
		namespacedName(workingExtension2).String(): {
			runtimev1.ExtensionHandler{
				Name: "first",
				RequestHook: runtimev1.GroupVersionHook{
					APIVersion: "v1alpha1",
					Hook:       "first",
				},
			},
		},
	}

	t.Run("create and discover extension on startup", func(t *testing.T) {
		var result runtimev1.ExtensionConfig

		r := &Reconciler{
			Client:    env.GetClient(),
			APIReader: env.GetAPIReader(),
			RuntimeClient: &fakeClient{
				extensionStore: hookStore,
			},
		}

		// Create and attempt to discover a working, discoverable extension. Expect no error.
		g.Expect(env.CreateAndWait(ctx, workingExtension1.DeepCopy())).To(Succeed())
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(workingExtension1)})
		g.Expect(err).To(BeNil())
		g.Expect(env.GetAPIReader().Get(ctx, namespacedName(workingExtension1), &result)).To(Succeed())

		g.Expect(result.Status.Conditions[0].Status).To(Equal(corev1.ConditionTrue))

		// Create and attempt to discover a broken, undiscoverable extension. Expect an error.
		g.Expect(env.CreateAndWait(ctx, brokenExtension.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(brokenExtension)})
		g.Expect(err).NotTo(BeNil())

		// Set reconciler ready to false and recreate all objects to simulate a restart. Expect discovery of working extension with an overall error.
		g.Expect(env.CleanupAndWait(ctx, workingExtension1, brokenExtension)).To(Succeed())
	})

	t.Run("Set conditions for both successful and failed discovery", func(t *testing.T) {
		var results runtimev1.ExtensionConfigList
		r := &Reconciler{
			Client:    env.GetClient(),
			APIReader: env.GetAPIReader(),
			RuntimeClient: &fakeClient{
				extensionStore: hookStore,
			},
		}

		g.Expect(env.CreateAndWait(ctx, workingExtension1.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(workingExtension1)})
		g.Expect(err).To(BeNil())

		g.Expect(env.CreateAndWait(ctx, brokenExtension.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(brokenExtension)})
		g.Expect(err).NotTo(BeNil())
		time.Sleep(time.Millisecond * 500)
		g.Expect(env.List(ctx, &results)).To(Succeed())
		for _, extension := range results.Items {
			if extension.Name == workingExtension1.Name {
				g.Expect(len(extension.GetConditions())).To(Not(Equal(0)))
				for _, condition := range extension.GetConditions() {
					if condition.Type == runtimev1.RuntimeExtensionDiscovered {
						g.Expect(condition.Status).To(Equal(corev1.ConditionTrue))
					}
					g.Expect(len(extension.Status.Handlers)).To(Equal(1))
				}
			}
			if extension.Name == brokenExtension.Name {
				g.Expect(len(extension.GetConditions())).To(Not(Equal(0)))
				for _, condition := range extension.GetConditions() {
					if condition.Type == runtimev1.RuntimeExtensionDiscovered {
						g.Expect(condition.Status).To(Equal(corev1.ConditionFalse))
					}
				}
				g.Expect(len(extension.Status.Handlers)).To(Equal(0))
			}
		}
		g.Expect(env.CleanupAndWait(ctx, workingExtension1, brokenExtension)).To(Succeed())
	})

	t.Run("discover extensions after warm up", func(t *testing.T) {
		var results runtimev1.ExtensionConfigList
		r := &Reconciler{
			Client:    env.GetClient(),
			APIReader: env.GetAPIReader(),
			RuntimeClient: &fakeClient{
				extensionStore: hookStore,
			},
		}

		// Reconcile initially to warm up.
		g.Expect(env.CreateAndWait(ctx, workingExtension1.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(workingExtension1)})
		g.Expect(err).To(BeNil())

		// Register a new extension and reconcile again with a warmed up registry.
		g.Expect(env.CreateAndWait(ctx, workingExtension2.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(workingExtension2)})
		g.Expect(err).To(BeNil())

		g.Expect(env.GetAPIReader().List(ctx, &results)).To(Succeed())
		for _, extension := range results.Items {
			if extension.Name == workingExtension1.Name {
				g.Expect(len(extension.GetConditions())).To(Not(Equal(0)))
				for _, condition := range extension.GetConditions() {
					if condition.Type == runtimev1.RuntimeExtensionDiscovered {
						g.Expect(condition.Status).To(Equal(corev1.ConditionTrue))
					}
					g.Expect(len(extension.Status.Handlers)).To(Equal(1))
				}
			}
			if extension.Name == workingExtension2.Name {
				g.Expect(len(extension.GetConditions())).To(Not(Equal(0)))
				for _, condition := range extension.GetConditions() {
					if condition.Type == runtimev1.RuntimeExtensionDiscovered {
						g.Expect(condition.Status).To(Equal(corev1.ConditionTrue))
					}
					g.Expect(len(extension.Status.Handlers)).To(Equal(1))
				}
			}
		}
		g.Expect(env.CleanupAndWait(ctx, workingExtension1, workingExtension2)).To(Succeed())
	})

	t.Run("test discovery using real client", func(t *testing.T) {
		var results runtimev1.ExtensionConfigList
		cat := catalog.New()
		g.Expect(runtimehooksv1.AddToCatalog(cat)).To(Succeed())
		g.Expect(hooksv2.AddToCatalog(cat)).To(Succeed())
		g.Expect(hooksv3.AddToCatalog(cat)).To(Succeed())

		mux := http.NewServeMux()
		mux.HandleFunc("/hooks.runtime.cluster.x-k8s.io/v1alpha1/discovery", func(w http.ResponseWriter, r *http.Request) {
			resp := runtimehooksv1.DiscoveryHookResponse{
				TypeMeta: metav1.TypeMeta{},
				Status:   "",
				Message:  "what a message",
				Extensions: []runtimehooksv1.RuntimeExtension{
					{
						Name: "first",
						Hook: runtimehooksv1.Hook{
							Name:       "first",
							APIVersion: "v1alpha1",
						},
					},
				},
			}

			respBody, err := json.Marshal(resp)
			if err != nil {
				panic(err)
			}

			w.WriteHeader(http.StatusOK)
			_, err = w.Write(respBody)
			if err != nil {
				panic(err)
			}
		})

		ts := httptest.NewServer(mux)
		defer ts.Close()

		extensionWithDiscovery1.Spec.ClientConfig.URL = &ts.URL
		extensionWithDiscovery2.Spec.ClientConfig.URL = &ts.URL

		r := &Reconciler{
			Client:    env.GetClient(),
			APIReader: env.GetAPIReader(),
			RuntimeClient: runtimeclient.New(runtimeclient.Options{
				Catalog: cat,
			}),
		}

		g.Expect(env.CreateAndWait(ctx, extensionWithDiscovery1.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(extensionWithDiscovery1)})
		g.Expect(err).To(BeNil())

		g.Expect(env.CreateAndWait(ctx, extensionWithDiscovery2.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(extensionWithDiscovery2)})
		g.Expect(err).To(BeNil())

		// Do a number of additional reconciles to see that the extension isn't deregistered.
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(extensionWithDiscovery1)})
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(brokenExtension)})

		g.Expect(env.GetAPIReader().List(ctx, &results)).To(Succeed())
		for _, extension := range results.Items {
			if extension.Name == extensionWithDiscovery1.Name {
				g.Expect(len(extension.GetConditions())).To(Not(Equal(0)))
				for _, condition := range extension.GetConditions() {
					if condition.Type == runtimev1.RuntimeExtensionDiscovered {
						g.Expect(condition.Status).To(Equal(corev1.ConditionTrue))
					}
					g.Expect(len(extension.Status.Handlers)).To(Equal(1))
				}
			}
			if extension.Name == extensionWithDiscovery2.Name {
				g.Expect(len(extension.GetConditions())).To(Not(Equal(0)))
				for _, condition := range extension.GetConditions() {
					if condition.Type == runtimev1.RuntimeExtensionDiscovered {
						g.Expect(condition.Status).To(Equal(corev1.ConditionTrue))
					}
					g.Expect(len(extension.Status.Handlers)).To(Equal(1))
				}
			}
		}
		g.Expect(env.CleanupAndWait(ctx, extensionWithDiscovery1, extensionWithDiscovery2)).To(Succeed())
	})

	t.Run("test removal of extension", func(t *testing.T) {
		var result runtimev1.ExtensionConfig

		r := &Reconciler{
			Client:    env.GetClient(),
			APIReader: env.GetAPIReader(),
			RuntimeClient: &fakeClient{
				extensionStore: hookStore,
			},
		}

		// Create and attempt to discover a working, discoverable extension. Expect no error.
		g.Expect(env.CreateAndWait(ctx, workingExtension1.DeepCopy())).To(Succeed())
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(workingExtension1)})
		g.Expect(err).To(BeNil())
		g.Expect(env.GetAPIReader().Get(ctx, namespacedName(workingExtension1), &result)).To(Succeed())

		// Delete and attempt to discover a removal of the extension. Expect no error.
		g.Expect(env.CleanupAndWait(ctx, workingExtension1.DeepCopy())).To(Succeed())
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName(workingExtension1)})
		g.Expect(err).To(BeNil())

		// Check again if the extension was removed
		g.Expect(env.GetAPIReader().Get(ctx, namespacedName(workingExtension1), &result)).To(HaveOccurred())
	})
}

type fakeClient struct {
	extensionStore map[string][]runtimev1.ExtensionHandler
	ready          bool
}

func (c *fakeClient) Hook(service catalog.Hook) runtimeclient.HookClient {
	panic("not implemented")
}

func (c *fakeClient) IsReady() bool {
	return c.ready
}

// Set registry ready to true.
func (c *fakeClient) WarmUp(extList *runtimev1.ExtensionConfigList) error {
	for i, extension := range extList.Items {
		ext := extension
		discovered, err := c.Extension(&ext).Discover(context.TODO())
		if err != nil {
			return err
		}
		extList.Items[i] = *discovered
	}
	c.ready = true
	return nil
}

func (c *fakeClient) SetRegistryReady(ready bool) {
	c.ready = ready
}

type fakeExtensionClient struct {
	client *fakeClient
	ext    *runtimev1.ExtensionConfig
}

func (c *fakeClient) Extension(ext *runtimev1.ExtensionConfig) runtimeclient.ExtensionClient {
	return fakeExtensionClient{
		client: c,
		ext:    ext,
	}
}

func (e fakeExtensionClient) Discover(ctx context.Context) (*runtimev1.ExtensionConfig, error) {
	v, ok := e.client.extensionStore[namespacedName(e.ext).String()]
	if !ok {
		return e.ext, errors.New("runtimeExtensions could not be found for Extension")
	}
	modifiedExtension := &runtimev1.ExtensionConfig{}
	e.ext.DeepCopyInto(modifiedExtension)
	modifiedExtension.Status.Handlers = v
	if err := e.client.Extension(modifiedExtension).Register(); err != nil {
		return e.ext, errors.New("failed to register extension")
	}
	return modifiedExtension, nil
}

// this doesn't do anything but reset the registration in the extension.
func (e fakeExtensionClient) Register() error {
	return nil
}

func (e fakeExtensionClient) Unregister() error {
	return nil
}

// namespacedName returns the NamespacedName for the extension.
func namespacedName(e *runtimev1.ExtensionConfig) types.NamespacedName {
	return types.NamespacedName{
		Namespace: e.Namespace,
		Name:      e.Name,
	}
}
