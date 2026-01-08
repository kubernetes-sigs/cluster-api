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

package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/testcerts"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	fakev1alpha1 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha1"
	fakev1alpha2 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha2"
	"sigs.k8s.io/cluster-api/util/cache"
)

func TestClient_httpCall(t *testing.T) {
	g := NewWithT(t)

	tableTests := []struct {
		name     string
		request  runtime.Object
		response runtime.Object
		opts     *httpCallOptions
		wantErr  bool
	}{
		{
			name:     "error if request, response and options are nil",
			request:  nil,
			response: nil,
			opts:     nil,
			wantErr:  true,
		},
		{
			name:     "error if catalog is not set",
			request:  &fakev1alpha1.FakeRequest{},
			response: &fakev1alpha1.FakeResponse{},
			opts: &httpCallOptions{
				catalog: nil,
			},
			wantErr: true,
		},
		{
			name:     "error if hooks is not registered with catalog",
			request:  &fakev1alpha1.FakeRequest{},
			response: &fakev1alpha1.FakeResponse{},
			opts: &httpCallOptions{
				catalog: runtimecatalog.New(),
			},
			wantErr: true,
		},
		{
			name: "succeed for valid request and response objects",
			request: &fakev1alpha1.FakeRequest{
				// Note: Intentionally setting TypeMeta here to test if everything works if TypeMeta is set.
				TypeMeta: metav1.TypeMeta{
					Kind:       "FakeRequest",
					APIVersion: fakev1alpha1.GroupVersion.Identifier(),
				},
			},
			response: &fakev1alpha1.FakeResponse{},
			opts: func() *httpCallOptions {
				c := runtimecatalog.New()
				g.Expect(fakev1alpha1.AddToCatalog(c)).To(Succeed())

				// get same gvh for hook by using the FakeHook and catalog
				gvh, err := c.GroupVersionHook(fakev1alpha1.FakeHook)
				g.Expect(err).To(Succeed())

				return &httpCallOptions{
					catalog:         c,
					registrationGVH: gvh,
					hookGVH:         gvh,
				}
			}(),
			wantErr: false,
		},
		{
			name: "success if request and response are valid objects - with conversion",
			request: &fakev1alpha2.FakeRequest{
				// Note: Intentionally setting TypeMeta here to test if everything works if TypeMeta is set.
				TypeMeta: metav1.TypeMeta{
					Kind:       "FakeRequest",
					APIVersion: fakev1alpha2.GroupVersion.Identifier(),
				},
			},
			response: &fakev1alpha2.FakeResponse{},
			opts: func() *httpCallOptions {
				c := runtimecatalog.New()
				// register fakev1alpha1 and fakev1alpha2 to enable conversion
				g.Expect(fakev1alpha1.AddToCatalog(c)).To(Succeed())
				g.Expect(fakev1alpha2.AddToCatalog(c)).To(Succeed())

				// get same gvh for hook by using the FakeHook and catalog
				registrationGVH, err := c.GroupVersionHook(fakev1alpha1.FakeHook)
				g.Expect(err).To(Succeed())
				hookGVH, err := c.GroupVersionHook(fakev1alpha2.FakeHook)
				g.Expect(err).To(Succeed())

				return &httpCallOptions{
					catalog:         c,
					registrationGVH: registrationGVH,
					hookGVH:         hookGVH,
				}
			}(),
			wantErr: false,
		},
		{
			name:     "succeed if request doesn't define TypeMeta",
			request:  &fakev1alpha2.FakeRequest{},
			response: &fakev1alpha2.FakeResponse{},
			opts: func() *httpCallOptions {
				c := runtimecatalog.New()
				// register fakev1alpha1 and fakev1alpha2 to enable conversion
				g.Expect(fakev1alpha2.AddToCatalog(c)).To(Succeed())

				// get same gvh for hook by using the FakeHook and catalog
				gvh, err := c.GroupVersionHook(fakev1alpha2.FakeHook)
				g.Expect(err).To(Succeed())

				return &httpCallOptions{
					catalog:         c,
					registrationGVH: gvh,
					hookGVH:         gvh,
				}
			}(),
			wantErr: false,
		},
		{
			name:     "success if request doesn't define TypeMeta - with conversion",
			request:  &fakev1alpha2.FakeRequest{},
			response: &fakev1alpha2.FakeResponse{},
			opts: func() *httpCallOptions {
				c := runtimecatalog.New()
				// register fakev1alpha1 and fakev1alpha2 to enable conversion
				g.Expect(fakev1alpha1.AddToCatalog(c)).To(Succeed())
				g.Expect(fakev1alpha2.AddToCatalog(c)).To(Succeed())

				// get same gvh for hook by using the FakeHook and catalog
				registrationGVH, err := c.GroupVersionHook(fakev1alpha1.FakeHook)
				g.Expect(err).To(Succeed())
				hookGVH, err := c.GroupVersionHook(fakev1alpha2.FakeHook)
				g.Expect(err).To(Succeed())

				return &httpCallOptions{
					catalog:         c,
					hookGVH:         hookGVH,
					registrationGVH: registrationGVH,
				}
			}(),
			wantErr: false,
		},
	}
	for _, tt := range tableTests {
		t.Run(tt.name, func(*testing.T) {
			// a http server is only required if we have a valid catalog, otherwise httpCall will not reach out to the server
			if tt.opts != nil && tt.opts.catalog != nil {
				// create http server with fakeHookHandler
				mux := http.NewServeMux()
				mux.HandleFunc("/", fakeHookHandler)

				srv := newUnstartedTLSServer(mux)
				srv.StartTLS()
				defer srv.Close()

				// set url to srv for in tt.opts
				tt.opts.config.URL = srv.URL
				tt.opts.config.CABundle = testcerts.CACert

				// set httpClient in tt.opts
				// Note: cert and key file are not necessary, because in this test the server do not requires client authentication with certificates signed by a given CA.
				u, err := url.Parse(srv.URL)
				g.Expect(err).ToNot(HaveOccurred())

				httpClient, err := createHTTPClient("", "", testcerts.CACert, u.Hostname())
				g.Expect(err).ToNot(HaveOccurred())
				tt.opts.httpClient = httpClient
			}

			err := httpCall(context.TODO(), tt.request, tt.response, tt.opts)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func fakeHookHandler(w http.ResponseWriter, _ *http.Request) {
	// Setting GVK because we directly Marshal to JSON.
	response := &fakev1alpha1.FakeResponse{
		TypeMeta: metav1.TypeMeta{
			Kind:       "FakeHookResponse",
			APIVersion: fakev1alpha1.GroupVersion.Identifier(),
		},
		Second: "",
		First:  1,
	}
	respBody, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBody)
}

func TestURLForExtension(t *testing.T) {
	type args struct {
		config               runtimev1.ClientConfig
		gvh                  runtimecatalog.GroupVersionHook
		extensionHandlerName string
	}

	type want struct {
		scheme string
		host   string
		path   string
	}

	gvh := runtimecatalog.GroupVersionHook{
		Group:   "test.runtime.cluster.x-k8s.io",
		Version: "v1alpha1",
		Hook:    "testhook.test-extension",
	}

	tests := []struct {
		name    string
		args    args
		want    want
		wantErr bool
	}{
		{
			name: "ClientConfig using service should have correct URL values",
			args: args{
				config: runtimev1.ClientConfig{
					Service: runtimev1.ServiceReference{
						Namespace: "test1",
						Name:      "extension-service",
						Port:      ptr.To[int32](8443),
					},
				},
				gvh:                  gvh,
				extensionHandlerName: "test-handler",
			},
			want: want{
				scheme: "https",
				host:   "extension-service.test1.svc:8443",
				path:   runtimecatalog.GVHToPath(gvh, "test-handler"),
			},
			wantErr: false,
		},
		{
			name: "ClientConfig using service and CAbundle should have correct URL values",
			args: args{
				config: runtimev1.ClientConfig{
					Service: runtimev1.ServiceReference{
						Namespace: "test1",
						Name:      "extension-service",
						Port:      ptr.To[int32](8443),
					},
					CABundle: []byte("some-ca-data"),
				},
				gvh:                  gvh,
				extensionHandlerName: "test-handler",
			},
			want: want{
				scheme: "https",
				host:   "extension-service.test1.svc:8443",
				path:   runtimecatalog.GVHToPath(gvh, "test-handler"),
			},
			wantErr: false,
		},
		{
			name: "ClientConfig using URL should have correct URL values",
			args: args{
				config: runtimev1.ClientConfig{
					URL: "https://extension-host.com",
				},
				gvh:                  gvh,
				extensionHandlerName: "test-handler",
			},
			want: want{
				scheme: "https",
				host:   "extension-host.com",
				path:   runtimecatalog.GVHToPath(gvh, "test-handler"),
			},
			wantErr: false,
		},
		{
			name: "should error if both Service and URL are missing",
			args: args{
				config:               runtimev1.ClientConfig{},
				gvh:                  gvh,
				extensionHandlerName: "test-handler",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			u, err := urlForExtension(tt.args.config, tt.args.gvh, tt.args.extensionHandlerName)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(u.Scheme).To(Equal(tt.want.scheme))
				g.Expect(u.Host).To(Equal(tt.want.host))
				g.Expect(u.Path).To(Equal(tt.want.path))
			}
		})
	}
}

func Test_defaultAndValidateDiscoveryResponse(t *testing.T) {
	var invalidFailurePolicy runtimehooksv1.FailurePolicy = "DONT_FAIL"
	cat := runtimecatalog.New()
	_ = fakev1alpha1.AddToCatalog(cat)

	tests := []struct {
		name      string
		discovery *runtimehooksv1.DiscoveryResponse
		wantErr   bool
	}{
		{
			name: "succeed with valid skeleton DiscoveryResponse",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "extension",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook:       "FakeHook",
						APIVersion: fakev1alpha1.GroupVersion.String(),
					},
				}},
			},
			wantErr: false,
		},
		{
			name: "error if handler name has capital letters",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "HAS-CAPITAL-LETTERS",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook:       "FakeHook",
						APIVersion: fakev1alpha1.GroupVersion.String(),
					},
				}},
			},
			wantErr: true,
		},
		{
			name: "error if handler name has full stops",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "has.full.stops",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook:       "FakeHook",
						APIVersion: fakev1alpha1.GroupVersion.String(),
					},
				}},
			},
			wantErr: true,
		},
		{
			name: "error with TimeoutSeconds of over 30 seconds",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "ext1",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook:       "FakeHook",
						APIVersion: fakev1alpha1.GroupVersion.String(),
					},
					TimeoutSeconds: ptr.To[int32](100),
				}},
			},
			wantErr: true,
		},
		{
			name: "error with TimeoutSeconds of less than 0",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "ext1",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook:       "FakeHook",
						APIVersion: fakev1alpha1.GroupVersion.String(),
					},
					TimeoutSeconds: ptr.To[int32](-1),
				}},
			},
			wantErr: true,
		},
		{
			name: "error with FailurePolicy not Fail or Ignore",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "ext1",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook:       "FakeHook",
						APIVersion: fakev1alpha1.GroupVersion.String(),
					},
					TimeoutSeconds: ptr.To[int32](20),
					FailurePolicy:  &invalidFailurePolicy,
				}},
			},
			wantErr: true,
		},
		{
			name: "error when handler name is duplicated",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{
					{
						Name: "ext1",
						RequestHook: runtimehooksv1.GroupVersionHook{
							Hook:       "FakeHook",
							APIVersion: fakev1alpha1.GroupVersion.String(),
						},
					},
					{
						Name: "ext1",
						RequestHook: runtimehooksv1.GroupVersionHook{
							Hook:       "FakeHook",
							APIVersion: fakev1alpha1.GroupVersion.String(),
						},
					},
					{
						Name: "ext2",
						RequestHook: runtimehooksv1.GroupVersionHook{
							Hook:       "FakeHook",
							APIVersion: fakev1alpha1.GroupVersion.String(),
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "error if handler GroupVersionHook is not registered",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "ext1",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook: "FakeHook",
						// Version v1alpha2 is not registered with the catalog
						APIVersion: fakev1alpha2.GroupVersion.String(),
					},
				}},
			},
			wantErr: true,
		},
		{
			name: "error if handler GroupVersion can not be parsed",
			discovery: &runtimehooksv1.DiscoveryResponse{
				Handlers: []runtimehooksv1.ExtensionHandler{{
					Name: "ext1",
					RequestHook: runtimehooksv1.GroupVersionHook{
						Hook: "FakeHook",
						// Version v1alpha2 is not registered with the catalog
						APIVersion: "too/many/slashes",
					},
				}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := defaultAndValidateDiscoveryResponse(cat, tt.discovery); (err != nil) != tt.wantErr {
				t.Errorf("defaultAndValidateDiscoveryResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_CallExtension(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}

	validExtensionHandlerWithFailPolicy := runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "15",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				// Set a fake URL, in test cases where we start the test server the URL will be overridden.
				URL:      "https://127.0.0.1/",
				CABundle: testcerts.CACert,
			},
			NamespaceSelector: &metav1.LabelSelector{},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "valid-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
			},
		},
	}
	validExtensionHandlerWithIgnorePolicy := runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "15",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				// Set a fake URL, in test cases where we start the test server the URL will be overridden.
				URL:      "https://127.0.0.1/",
				CABundle: testcerts.CACert,
			},
			NamespaceSelector: &metav1.LabelSelector{}},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "valid-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyIgnore,
				},
			},
		},
	}
	type args struct {
		hook     runtimecatalog.Hook
		name     string
		request  runtimehooksv1.RequestObject
		response runtimehooksv1.ResponseObject
	}
	tests := []struct {
		name                       string
		registeredExtensionConfigs []runtimev1.ExtensionConfig
		args                       args
		testServer                 testServerConfig
		wantErr                    bool
		wantResponseCached         bool
	}{
		{
			name:                       "should fail when hook and request/response are not compatible",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: false,
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.SecondFakeRequest{},
				response: &fakev1alpha1.SecondFakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},
		{
			name:                       "should fail when hook GVH does not match the registered ExtensionHandler",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: false,
			},
			args: args{
				hook:     fakev1alpha1.SecondFakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.SecondFakeRequest{},
				response: &fakev1alpha1.SecondFakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},
		{
			name:                       "should fail if ExtensionHandler is not registered",
			registeredExtensionConfigs: nil,
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusSuccess),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "unregistered-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},
		{
			name:                       "should succeed when calling ExtensionHandler with success response and FailurePolicyFail",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusSuccess),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            false,
			wantResponseCached: true,
		},
		{
			name:                       "should succeed when calling ExtensionHandler with success response and FailurePolicyIgnore",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithIgnorePolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusSuccess),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            false,
			wantResponseCached: true,
		},
		{
			name:                       "should fail when calling ExtensionHandler with failure response and FailurePolicyFail",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusFailure),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},
		{
			name:                       "should fail when calling ExtensionHandler with failure response and FailurePolicyIgnore",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithIgnorePolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusFailure),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},

		{
			name:                       "should succeed with unreachable extension and FailurePolicyIgnore",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithIgnorePolicy},
			testServer: testServerConfig{
				start: false,
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            false,
			wantResponseCached: false, // Note: We only want to cache entirely successful responses.
		},
		{
			name:                       "should fail with unreachable extension and FailurePolicyFail",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: false,
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},
		{
			name:                       "should fail when calling ExtensionHandler with unknown response status and FailurePolicyFail",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatus("Unknown")),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},
		{
			name:                       "should fail when calling ExtensionHandler with unknown response status and FailurePolicyIgnore",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithIgnorePolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatus("Unknown")),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr:            true,
			wantResponseCached: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			var serverCallCount int
			if tt.testServer.start {
				srv := createSecureTestServer(tt.testServer, func() {
					serverCallCount++
				})
				srv.StartTLS()
				defer srv.Close()

				// Set the URL to the real address of the test server.
				for i := range tt.registeredExtensionConfigs {
					tt.registeredExtensionConfigs[i].Spec.ClientConfig.URL = fmt.Sprintf("https://%s/", srv.Listener.Addr().String())
				}
			}

			cat := runtimecatalog.New()
			_ = fakev1alpha1.AddToCatalog(cat)
			_ = fakev1alpha2.AddToCatalog(cat)
			fakeClient := fake.NewClientBuilder().
				WithObjects(ns).
				Build()

			c, _, err := New(Options{
				Catalog:  cat,
				Registry: registry(tt.registeredExtensionConfigs),
				Client:   fakeClient,
			})
			g.Expect(err).ToNot(HaveOccurred())

			obj := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster",
					Namespace: "foo",
				},
			}
			// Call once without caching.
			err = c.CallExtension(context.Background(), tt.args.hook, obj, tt.args.name, tt.args.request, tt.args.response)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			// Call again with caching.
			serverCallCount = 0
			cache := cache.New[runtimeclient.CallExtensionCacheEntry](cache.DefaultTTL)
			err = c.CallExtension(context.Background(), tt.args.hook, obj, tt.args.name, tt.args.request, tt.args.response,
				runtimeclient.WithCaching{Cache: cache, CacheKeyFunc: cacheKeyFunc})
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}

			if tt.wantResponseCached {
				// When we expect the response to be cached we expect 1 call to the server.
				g.Expect(serverCallCount).To(Equal(1))
				cacheEntry, isCached := cache.Has("valid-extension-15")
				g.Expect(isCached).To(BeTrue())
				g.Expect(cacheEntry).ToNot(BeNil())

				err = c.CallExtension(context.Background(), tt.args.hook, obj, tt.args.name, tt.args.request, tt.args.response,
					runtimeclient.WithCaching{Cache: cache, CacheKeyFunc: cacheKeyFunc})
				// When we expect the response to be cached we always expect no errors.
				g.Expect(err).ToNot(HaveOccurred())
				// As the response is cached we expect no further calls to the server.
				g.Expect(serverCallCount).To(Equal(1))
				cacheEntry, isCached = cache.Has("valid-extension-15")
				g.Expect(isCached).To(BeTrue())
				g.Expect(cacheEntry).ToNot(BeNil())
			} else {
				_, isCached := cache.Has("valid-extension-15")
				g.Expect(isCached).To(BeFalse())
			}
		})
	}
}

func TestClient_CallExtensionWithClientAuthentication(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}

	validExtensionHandlerWithFailPolicy := runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "15",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				// Set a fake URL, in test cases where we start the test server the URL will be overridden.
				URL:      "https://127.0.0.1/",
				CABundle: testcerts.CACert,
			},
			NamespaceSelector: &metav1.LabelSelector{},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "valid-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
			},
		},
	}

	g := NewWithT(t)

	tmpDir := t.TempDir()
	clientCertFile := filepath.Join(tmpDir, "tls.crt")
	g.Expect(os.WriteFile(clientCertFile, testcerts.ClientCert, 0600)).To(Succeed())
	clientKeyFile := filepath.Join(tmpDir, "tls.key")
	g.Expect(os.WriteFile(clientKeyFile, testcerts.ClientKey, 0600)).To(Succeed())

	var serverCallCount int
	createServer := func() *httptest.Server {
		return createSecureTestServer(testServerConfig{
			start: true,
			responses: map[string]testServerResponse{
				"/*": response(runtimehooksv1.ResponseStatusSuccess),
			},
		}, func() {
			serverCallCount++
		})
	}
	srv := createServer()

	// Setup the runtime extension server so it requires client authentication with certificates signed by a given CA.
	certpool := x509.NewCertPool()
	certpool.AppendCertsFromPEM(testcerts.CACert)
	srv.TLS.ClientAuth = tls.RequireAndVerifyClientCert
	srv.TLS.ClientCAs = certpool

	srv.StartTLS()
	listenerAddr := srv.Listener.Addr()

	// Set the URL to the real address of the test server.
	validExtensionHandlerWithFailPolicy.Spec.ClientConfig.URL = fmt.Sprintf("https://%s/", srv.Listener.Addr().String())

	cat := runtimecatalog.New()
	_ = fakev1alpha1.AddToCatalog(cat)
	_ = fakev1alpha2.AddToCatalog(cat)
	fakeClient := fake.NewClientBuilder().
		WithObjects(ns).
		Build()

	c, certWatcher, err := New(Options{
		// Add client authentication credentials to the client
		CertFile: clientCertFile,
		KeyFile:  clientKeyFile,
		Catalog:  cat,
		Registry: registry([]runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy}),
		Client:   fakeClient,
	})
	g.Expect(err).ToNot(HaveOccurred())
	go func() {
		_ = certWatcher.Start(t.Context())
	}()

	obj := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "foo",
		},
	}

	err = c.CallExtension(context.Background(), fakev1alpha1.FakeHook, obj, "valid-extension", &fakev1alpha1.FakeRequest{}, &fakev1alpha1.FakeResponse{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(serverCallCount).To(Equal(1))

	// Test case 1: Rotate client cert/key on client-side and then client ca on server-side.

	// Rotate client cert/key on client-side.
	g.Expect(os.WriteFile(clientCertFile, testClientCert, 0600)).To(Succeed())
	g.Expect(os.WriteFile(clientKeyFile, testClientKey, 0600)).To(Succeed())

	// Validate that the client rotated the client cert/key.
	// The server still uses the old client ca so it should reject the client cert.
	g.Eventually(func(g Gomega) {
		err = c.CallExtension(context.Background(), fakev1alpha1.FakeHook, obj, "valid-extension", &fakev1alpha1.FakeRequest{}, &fakev1alpha1.FakeResponse{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("test.runtime.cluster.x-k8s.io/v1alpha1/fakehook/valid-extension?timeout=1s\": remote error: tls: unknown certificate authority"))
	}, 10*time.Second).Should(Succeed())

	// Rotate client ca on server-side.
	srv.Close()
	srv = createServer()
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", listenerAddr.String()) // Ensure the server uses the same port as before.
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(srv.Listener.Close()).To(Succeed())
	srv.Listener = l
	certpool = x509.NewCertPool()
	certpool.AppendCertsFromPEM(testCACert)
	srv.TLS.ClientAuth = tls.RequireAndVerifyClientCert
	srv.TLS.ClientCAs = certpool
	srv.StartTLS()

	// Validate that the server rotated the client ca.
	// The server now uses the new client ca so it should accept the client cert.
	g.Eventually(func(g Gomega) {
		err = c.CallExtension(context.Background(), fakev1alpha1.FakeHook, obj, "valid-extension", &fakev1alpha1.FakeRequest{}, &fakev1alpha1.FakeResponse{})
		g.Expect(err).ToNot(HaveOccurred())
	}, 10*time.Second).Should(Succeed())

	// Test case 2: Rotate client ca on server-side and then client cert/key on client-side.

	// Rotate client ca on server-side.
	srv.Close()
	srv = createServer()
	l, err = (&net.ListenConfig{}).Listen(t.Context(), "tcp", listenerAddr.String()) // Ensure the server uses the same port as before.
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(srv.Listener.Close()).To(Succeed())
	srv.Listener = l
	certpool = x509.NewCertPool()
	certpool.AppendCertsFromPEM(testcerts.CACert)
	srv.TLS.ClientAuth = tls.RequireAndVerifyClientCert
	srv.TLS.ClientCAs = certpool
	srv.StartTLS()

	// Validate that the server rotated the client ca.
	// The server now uses the new client ca so it should reject the client cert.
	g.Eventually(func(g Gomega) {
		err = c.CallExtension(context.Background(), fakev1alpha1.FakeHook, obj, "valid-extension", &fakev1alpha1.FakeRequest{}, &fakev1alpha1.FakeResponse{})
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("test.runtime.cluster.x-k8s.io/v1alpha1/fakehook/valid-extension?timeout=1s\": remote error: tls: unknown certificate authority"))
	}, 10*time.Second).Should(Succeed())

	// Rotate client cert/key on client-side.
	g.Expect(os.WriteFile(clientCertFile, testcerts.ClientCert, 0600)).To(Succeed())
	g.Expect(os.WriteFile(clientKeyFile, testcerts.ClientKey, 0600)).To(Succeed())

	// Validate that the client rotated the client cert/key.
	// The client now uses the new client cert so the server should accept it.
	g.Eventually(func(g Gomega) {
		err = c.CallExtension(context.Background(), fakev1alpha1.FakeHook, obj, "valid-extension", &fakev1alpha1.FakeRequest{}, &fakev1alpha1.FakeResponse{})
		g.Expect(err).ToNot(HaveOccurred())
	}, 10*time.Second).Should(Succeed())

	srv.Close()
}

func TestClient_GetHttpClient(t *testing.T) {
	g := NewWithT(t)

	extension1 := runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "extension1",
			ResourceVersion: "15",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL:      "https://serverA.example.com/",
				CABundle: testcerts.CACert,
			},
		},
	}

	extension2 := runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "extension2",
			ResourceVersion: "36",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL:      "https://serverA.example.com/",
				CABundle: testcerts.CACert,
			},
		},
	}

	extension3 := runtimev1.ExtensionConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "extension3",
			ResourceVersion: "54",
		},
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL:      "https://serverB.example.com/",
				CABundle: testcerts.CACert, // in a real example also CA should be different, but the host name is already enough to require a different client.
			},
		},
	}

	c, _, err := New(Options{})
	g.Expect(err).ToNot(HaveOccurred())

	internalClient := c.(*client)
	g.Expect(internalClient.httpClientsCache.Len()).To(Equal(0))

	// Get http client for extension 1
	gotClientExtension1, err := internalClient.getHTTPClient(extension1.Spec.ClientConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gotClientExtension1).ToNot(BeNil())

	// Check http client cache have only one item
	g.Expect(internalClient.httpClientsCache.Len()).To(Equal(1))
	_, ok := internalClient.httpClientsCache.Has(newHTTPClientEntryKey("serverA.example.com", extension1.Spec.ClientConfig.CABundle))
	g.Expect(ok).To(BeTrue())

	// Check http client cache is used for the same extension
	gotClientExtension1Again, err := internalClient.getHTTPClient(extension1.Spec.ClientConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gotClientExtension1Again).To(Equal(gotClientExtension1))

	// Get http client for extension 2, same server
	gotClientExtension2, err := internalClient.getHTTPClient(extension2.Spec.ClientConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gotClientExtension2).ToNot(BeNil())
	g.Expect(gotClientExtension2).To(Equal(gotClientExtension1))

	// Check http client cache have two items
	g.Expect(internalClient.httpClientsCache.Len()).To(Equal(1))
	_, ok = internalClient.httpClientsCache.Has(newHTTPClientEntryKey("serverA.example.com", extension2.Spec.ClientConfig.CABundle))
	g.Expect(ok).To(BeTrue())

	// Get http client for extension 3, another server
	gotClientExtension3, err := internalClient.getHTTPClient(extension3.Spec.ClientConfig)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gotClientExtension3).ToNot(BeNil())

	// Check http client cache have two items
	g.Expect(internalClient.httpClientsCache.Len()).To(Equal(2))
	_, ok = internalClient.httpClientsCache.Has(newHTTPClientEntryKey("serverA.example.com", extension1.Spec.ClientConfig.CABundle))
	g.Expect(ok).To(BeTrue())
	_, ok = internalClient.httpClientsCache.Has(newHTTPClientEntryKey("serverB.example.com", extension2.Spec.ClientConfig.CABundle))
	g.Expect(ok).To(BeTrue())
}

func cacheKeyFunc(extensionName, extensionConfigResourceVersion string, request runtimehooksv1.RequestObject) string {
	// Note: extensionName is identical to the value of the name parameter passed into CallExtension.
	s := fmt.Sprintf("%s-%s", extensionName, extensionConfigResourceVersion)
	for k, v := range request.GetSettings() {
		s += fmt.Sprintf(",%s=%s", k, v)
	}
	return s
}

func TestPrepareRequest(t *testing.T) {
	t.Run("request should have the correct settings", func(t *testing.T) {
		tests := []struct {
			name         string
			request      runtimehooksv1.RequestObject
			registration *runtimeregistry.ExtensionRegistration
			want         map[string]string
		}{
			{
				name: "settings in request should be preserved as is if there are not setting in the registration",
				request: &runtimehooksv1.BeforeClusterCreateRequest{
					CommonRequest: runtimehooksv1.CommonRequest{
						Settings: map[string]string{
							"key1": "value1",
						},
					},
				},
				registration: &runtimeregistry.ExtensionRegistration{},
				want: map[string]string{
					"key1": "value1",
				},
			},
			{
				name: "settings in registration should be used as is if there are no settings in the request",
				request: &runtimehooksv1.BeforeClusterCreateRequest{
					CommonRequest: runtimehooksv1.CommonRequest{},
				},
				registration: &runtimeregistry.ExtensionRegistration{
					Settings: map[string]string{
						"key1": "value1",
					},
				},
				want: map[string]string{
					"key1": "value1",
				},
			},
			{
				name: "settings in request and registry should be merged with request taking precedence",
				request: &runtimehooksv1.BeforeClusterCreateRequest{
					CommonRequest: runtimehooksv1.CommonRequest{
						Settings: map[string]string{
							"key1": "value1",
							"key2": "value2",
						},
					},
				},
				registration: &runtimeregistry.ExtensionRegistration{
					Settings: map[string]string{
						"key1": "value11",
						"key3": "value3",
					},
				},
				want: map[string]string{
					"key1": "value1",
					"key2": "value2",
					"key3": "value3",
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				g := NewWithT(t)
				var originalRegistrationSettings map[string]string
				if tt.registration.Settings != nil {
					originalRegistrationSettings = map[string]string{}
					for k, v := range tt.registration.Settings {
						originalRegistrationSettings[k] = v
					}
				}

				g.Expect(cloneAndAddSettings(tt.request, tt.registration.Settings).GetSettings()).To(Equal(tt.want))
				// Make sure that the original settings in the registration are not modified.
				g.Expect(tt.registration.Settings).To(Equal(originalRegistrationSettings))
			})
		}
	})
}

func TestClient_GetAllExtensions(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Labels: map[string]string{
				"kubernetes.io/metadata.name": "foo",
			},
		},
	}
	nsDifferent := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "different",
			Labels: map[string]string{
				"kubernetes.io/metadata.name": "different",
			},
		},
	}
	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "foo",
		},
	}
	clusterDifferentNamespace := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "different",
		},
	}

	extensionConfig := runtimev1.ExtensionConfig{
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				// Set a fake URL, in test cases where we start the test server the URL will be overridden.
				URL:      "https://127.0.0.1/",
				CABundle: testcerts.CACert,
			},
			// The extensions in this ExtensionConfig will be only registered for the foo namespace.
			NamespaceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "kubernetes.io/metadata.name",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{ns.Name},
					},
				},
			},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "first-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
				{
					Name: "second-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
				{
					Name: "third-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
			},
		},
	}

	tests := []struct {
		name                       string
		registeredExtensionConfigs []runtimev1.ExtensionConfig
		hook                       runtimecatalog.Hook
		cluster                    *clusterv1.Cluster
		wantExtensions             []string
		wantErr                    bool
	}{
		{
			name:                       "should return extensions if ExtensionHandlers are registered for the hook",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{extensionConfig},
			hook:                       fakev1alpha1.FakeHook,
			cluster:                    cluster,
			wantExtensions:             []string{"first-extension", "second-extension", "third-extension"},
		},
		{
			name:                       "should return no extensions if ExtensionHandlers are registered for the hook in a different namespace",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{extensionConfig},
			hook:                       fakev1alpha1.FakeHook,
			cluster:                    clusterDifferentNamespace,
			wantExtensions:             []string{},
		},
		{
			name:                       "should return no extensions if no ExtensionHandlers are registered for the hook",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{},
			hook:                       fakev1alpha1.SecondFakeHook,
			cluster:                    cluster,
			wantExtensions:             []string{},
		},
		{
			name:                       "should return error if hook is not registered in the catalog",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{},
			hook:                       "UnknownHook",
			wantErr:                    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

			cat := runtimecatalog.New()
			_ = fakev1alpha1.AddToCatalog(cat)
			_ = fakev1alpha2.AddToCatalog(cat)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns, nsDifferent).
				Build()
			c, _, err := New(Options{
				Catalog:  cat,
				Registry: registry(tt.registeredExtensionConfigs),
				Client:   fakeClient,
			})
			g.Expect(err).ToNot(HaveOccurred())

			gotExtensions, err := c.GetAllExtensions(context.Background(), tt.hook, tt.cluster)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(gotExtensions).To(ConsistOf(tt.wantExtensions))
		})
	}
}

func TestClient_CallAllExtensions(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}

	extensionConfig := runtimev1.ExtensionConfig{
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				// Set a fake URL, in test cases where we start the test server the URL will be overridden.
				URL:      "https://127.0.0.1/",
				CABundle: testcerts.CACert,
			},
			NamespaceSelector: &metav1.LabelSelector{},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "first-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
				{
					Name: "second-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
				{
					Name: "third-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: 1,
					FailurePolicy:  runtimev1.FailurePolicyFail,
				},
			},
		},
	}

	type args struct {
		hook     runtimecatalog.Hook
		request  runtimehooksv1.RequestObject
		response runtimehooksv1.ResponseObject
	}
	tests := []struct {
		name                       string
		registeredExtensionConfigs []runtimev1.ExtensionConfig
		args                       args
		testServer                 testServerConfig
		wantErr                    bool
	}{
		{
			name:                       "should fail when hook and request/response are not compatible",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{extensionConfig},
			testServer: testServerConfig{
				start: false,
			},
			args: args{
				hook:     fakev1alpha1.SecondFakeHook,
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: true,
		},
		{
			name:                       "should succeed when no ExtensionHandlers are registered for the hook",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{},
			testServer: testServerConfig{
				start: false,
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: false,
		},
		{
			name:                       "should succeed when calling ExtensionHandlers with success responses",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{extensionConfig},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusSuccess),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: false,
		},
		{
			name:                       "should fail when calling ExtensionHandlers with failure responses",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{extensionConfig},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusFailure),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: true,
		},
		{
			name:                       "should fail when one of the ExtensionHandlers returns a failure responses",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{extensionConfig},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/test.runtime.cluster.x-k8s.io/v1alpha1/fakehook/first-extension.*":  response(runtimehooksv1.ResponseStatusSuccess),
					"/test.runtime.cluster.x-k8s.io/v1alpha1/fakehook/second-extension.*": response(runtimehooksv1.ResponseStatusFailure),
					"/test.runtime.cluster.x-k8s.io/v1alpha1/fakehook/third-extension.*":  response(runtimehooksv1.ResponseStatusSuccess),
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: true,
		},
		{
			name:                       "should fail when one of the ExtensionHandlers returns 404",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{extensionConfig},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/test.runtime.cluster.x-k8s.io/v1alpha1/fakehook/first-extension.*":  response(runtimehooksv1.ResponseStatusSuccess),
					"/test.runtime.cluster.x-k8s.io/v1alpha1/fakehook/second-extension.*": response(runtimehooksv1.ResponseStatusFailure),
					// third-extension has no handler.
				},
			},
			args: args{
				hook:     fakev1alpha1.FakeHook,
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.testServer.start {
				srv := createSecureTestServer(tt.testServer)
				srv.StartTLS()
				defer srv.Close()

				// Set the URL to the real address of the test server.
				for i := range tt.registeredExtensionConfigs {
					tt.registeredExtensionConfigs[i].Spec.ClientConfig.URL = fmt.Sprintf("https://%s/", srv.Listener.Addr().String())
				}
			}

			scheme := runtime.NewScheme()
			g.Expect(clusterv1.AddToScheme(scheme)).To(Succeed())
			g.Expect(corev1.AddToScheme(scheme)).To(Succeed())

			cat := runtimecatalog.New()
			_ = fakev1alpha1.AddToCatalog(cat)
			_ = fakev1alpha2.AddToCatalog(cat)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(ns).
				Build()
			c, _, err := New(Options{
				Catalog:  cat,
				Registry: registry(tt.registeredExtensionConfigs),
				Client:   fakeClient,
			})
			g.Expect(err).ToNot(HaveOccurred())

			obj := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster",
					Namespace: "foo",
				},
			}
			err = c.CallAllExtensions(context.Background(), tt.args.hook, obj, tt.args.request, tt.args.response)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func Test_client_matchNamespace(t *testing.T) {
	g := NewWithT(t)
	foo := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Labels: map[string]string{
				corev1.LabelMetadataName: "foo",
			},
		},
	}
	bar := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
			Labels: map[string]string{
				corev1.LabelMetadataName: "bar",
			},
		},
	}
	matchingMatchExpressions, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      corev1.LabelMetadataName,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"foo", "bar"},
			},
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	notMatchingMatchExpressions, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      corev1.LabelMetadataName,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"non-existing", "other"},
			},
		},
	})
	g.Expect(err).ToNot(HaveOccurred())
	tests := []struct {
		name               string
		selector           labels.Selector
		namespace          string
		existingNamespaces []ctrlclient.Object
		want               bool
		wantErr            bool
	}{
		{
			name:               "match with single label selector",
			selector:           labels.SelectorFromSet(labels.Set{corev1.LabelMetadataName: foo.Name}),
			namespace:          "foo",
			existingNamespaces: []ctrlclient.Object{foo, bar},
			want:               true,
			wantErr:            false,
		},
		{
			name:               "error with non-existent namespace",
			selector:           labels.SelectorFromSet(labels.Set{corev1.LabelMetadataName: foo.Name}),
			namespace:          "non-existent",
			existingNamespaces: []ctrlclient.Object{foo, bar},
			want:               false,
			wantErr:            true,
		},
		{
			name:               "doesn't match if namespaceSelector doesn't match namespace",
			selector:           labels.SelectorFromSet(labels.Set{corev1.LabelMetadataName: bar.Name}),
			namespace:          "foo",
			existingNamespaces: []ctrlclient.Object{foo, bar},
			want:               false,
			wantErr:            false,
		},
		{
			name:               "match if match expressions match namespace",
			selector:           matchingMatchExpressions,
			namespace:          "bar",
			existingNamespaces: []ctrlclient.Object{foo, bar},
			want:               true,
			wantErr:            false,
		},
		{
			name:               "doesn't match if match expressions doesn't match namespace",
			selector:           notMatchingMatchExpressions,
			namespace:          "foo",
			existingNamespaces: []ctrlclient.Object{foo, bar},
			want:               false,
			wantErr:            false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := client{
				client: fake.NewClientBuilder().
					WithObjects(tt.existingNamespaces...).
					Build(),
			}
			got, err := c.matchNamespace(context.Background(), tt.selector, tt.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("matchNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("matchNamespace() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_aggregateResponses(t *testing.T) {
	tests := []struct {
		name              string
		aggregateResponse runtimehooksv1.ResponseObject
		responses         []runtimehooksv1.ResponseObject
		want              runtimehooksv1.ResponseObject
	}{
		{
			name:              "Aggregate response if there is only one response",
			aggregateResponse: fakeSuccessResponse(""),
			responses: []runtimehooksv1.ResponseObject{
				fakeSuccessResponse("test"),
			},
			want: fakeSuccessResponse("test"),
		},
		{
			name:              "Aggregate retry response if there is only one response",
			aggregateResponse: fakeRetryableSuccessResponse(0, ""),
			responses: []runtimehooksv1.ResponseObject{
				fakeRetryableSuccessResponse(5, "test"),
			},
			want: fakeRetryableSuccessResponse(5, "test"),
		},
		{
			name:              "Aggregate retry responses to lowest non-zero retryAfterSeconds value",
			aggregateResponse: fakeRetryableSuccessResponse(0, ""),
			responses: []runtimehooksv1.ResponseObject{
				fakeRetryableSuccessResponse(0, "test1"),
				fakeRetryableSuccessResponse(1, "test2"),
				fakeRetryableSuccessResponse(5, ""),
				fakeRetryableSuccessResponse(4, ""),
				fakeRetryableSuccessResponse(3, ""),
			},
			want: fakeRetryableSuccessResponse(1, "test1, test2"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregateSuccessfulResponses(tt.aggregateResponse, tt.responses)

			if !cmp.Equal(tt.aggregateResponse, tt.want) {
				t.Errorf("aggregateSuccessfulResponses() got = %v, want %v", tt.aggregateResponse, tt.want)
			}
		})
	}
}

type testServerConfig struct {
	start     bool
	responses map[string]testServerResponse
}

type testServerResponse struct {
	response           runtime.Object
	responseStatusCode int
}

func response(status runtimehooksv1.ResponseStatus) testServerResponse {
	return testServerResponse{
		response: &fakev1alpha1.FakeResponse{
			CommonResponse: runtimehooksv1.CommonResponse{
				Status: status,
			},
		},
		responseStatusCode: http.StatusOK,
	}
}

func createSecureTestServer(server testServerConfig, callbacks ...func()) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		for _, callback := range callbacks {
			callback()
		}

		// Write the response for the first match in tt.testServer.responses.
		for pathRegex, resp := range server.responses {
			if !regexp.MustCompile(pathRegex).MatchString(r.URL.Path) {
				continue
			}

			respBody, err := json.Marshal(resp.response)
			if err != nil {
				panic(err)
			}
			w.WriteHeader(resp.responseStatusCode)
			_, _ = w.Write(respBody)
			return
		}

		// Otherwise write a 404.
		w.WriteHeader(http.StatusNotFound)
	})

	srv := newUnstartedTLSServer(mux)

	return srv
}

func registry(configs []runtimev1.ExtensionConfig) runtimeregistry.ExtensionRegistry {
	registry := runtimeregistry.New()
	err := registry.WarmUp(&runtimev1.ExtensionConfigList{
		Items: configs,
	})
	if err != nil {
		panic(err)
	}
	return registry
}

func fakeSuccessResponse(message string) *fakev1alpha1.FakeResponse {
	return &fakev1alpha1.FakeResponse{
		CommonResponse: runtimehooksv1.CommonResponse{
			Message: message,
			Status:  runtimehooksv1.ResponseStatusSuccess,
		},
	}
}

func fakeRetryableSuccessResponse(retryAfterSeconds int32, message string) *fakev1alpha1.RetryableFakeResponse {
	return &fakev1alpha1.RetryableFakeResponse{
		CommonResponse: runtimehooksv1.CommonResponse{
			Message: message,
			Status:  runtimehooksv1.ResponseStatusSuccess,
		},
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: retryAfterSeconds,
		},
	}
}

func newUnstartedTLSServer(handler http.Handler) *httptest.Server {
	cert, err := tls.X509KeyPair(testcerts.ServerCert, testcerts.ServerKey)
	if err != nil {
		panic(err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
	}
	return srv
}

func TestNameForHandler(t *testing.T) {
	tests := []struct {
		name            string
		handler         runtimehooksv1.ExtensionHandler
		extensionConfig *runtimev1.ExtensionConfig
		want            string
		wantErr         bool
	}{
		{
			name:            "return well formatted name",
			handler:         runtimehooksv1.ExtensionHandler{Name: "discover-variables"},
			extensionConfig: &runtimev1.ExtensionConfig{ObjectMeta: metav1.ObjectMeta{Name: "runtime1"}},
			want:            "discover-variables.runtime1",
		},
		{
			name:            "return well formatted name",
			handler:         runtimehooksv1.ExtensionHandler{Name: "discover-variables"},
			extensionConfig: nil,
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NameForHandler(tt.handler, tt.extensionConfig)
			if (err != nil) != tt.wantErr {
				t.Errorf("NameForHandler() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NameForHandler() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtensionNameFromHandlerName(t *testing.T) {
	tests := []struct {
		name                  string
		registeredHandlerName string
		want                  string
		wantErr               bool
	}{
		{
			name:                  "Get name from correctly formatted handler name",
			registeredHandlerName: "discover-variables.runtime1",
			want:                  "runtime1",
		},
		{
			name: "error from incorrectly formatted handler name",
			// Two periods make this name badly formed.
			registeredHandlerName: "discover-variables.runtime.1",
			wantErr:               true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtensionNameFromHandlerName(tt.registeredHandlerName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtensionNameFromHandlerName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtensionNameFromHandlerName() got = %v, want %v", got, tt.want)
			}
		})
	}
}

// The following certs were generated using openssl by the https://github.com/kubernetes/kubernetes/blob/481c2d8e03508dba2c28aeb4bba48ce48904183b/staging/src/k8s.io/apiserver/pkg/admission/plugin/webhook/testcerts/gencerts.sh
// script and are used as certificates for the Runtime SDK unit tests.

var testCACert = []byte(`-----BEGIN CERTIFICATE-----
MIIDSzCCAjOgAwIBAgIUbnfp0PkokQadBOyXy4Os7NwQ90owDQYJKoZIhvcNAQEL
BQAwNDEyMDAGA1UEAwwpZ2VuZXJpY193ZWJob29rX2FkbWlzc2lvbl9wbHVnaW5f
dGVzdHNfY2EwIBcNMjYwMTA4MDgzNTE3WhgPMjI5OTEwMjQwODM1MTdaMDQxMjAw
BgNVBAMMKWdlbmVyaWNfd2ViaG9va19hZG1pc3Npb25fcGx1Z2luX3Rlc3RzX2Nh
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAkhzooe6GxCl8Isq6AEPo
3cqohJkukNcPHv4L0NThfClkguR8xvZxTIi4KlBYCeYTlR89pZ93Zi6pQwmIadOx
JsArM4bv+1rSn9e1jJhqZeq98mcpP5JbwjzWfn8Zc8tRLhkuvmvtgAfaay52czfL
RzHfW7gTEIgBGhmloSIPJEyRcx1e62RYKpVZ0STkOIXiLJXG/31i6Rkr2jYO9gkK
xoqN9NRh+e1QHSDa5bRNMZNoY5hXA4y4RL5jNFT+y7OhlBUZPC9Vf+7Wlbf7yKyJ
Wi07owCEL+1kGLb2wbXKz0gIJvPNZVZbaubNM4rRmDvDnwYW0BCvk291fnvV6Csp
ZQIDAQABo1MwUTAdBgNVHQ4EFgQUatgdaU6aZqIfwyH+wVJov6z38kYwHwYDVR0j
BBgwFoAUatgdaU6aZqIfwyH+wVJov6z38kYwDwYDVR0TAQH/BAUwAwEB/zANBgkq
hkiG9w0BAQsFAAOCAQEAWicoW4ewb63lk+aEfFyUIZ2al8tPOqTm4UbHOUebV9hc
OQWX8bYVgUMPGim/xWp8cxDWTVLWK7fVW10C3it33/N04tr5ZbZyg6OF/ZnGVSaL
VIsZJ7VL/oNtuHuwYGmqLRfLodEo3JZ+f2P0+vSHndNrZA9lRj2/JRTxnV+cmJQC
z3zBHITNox004o3FNvieMYRYWR8DEEyDSxymC3FJE3/ICGxGsdGPdwrxe9qzzvmq
Sa4jzY5JwNyPgBKqS7QU1BJfD1kHHKVAvxy+dfmCjB6FqdwVWr9/7sm6sKY52+3/
iHAv6fww8ADsxGM7dj89x41gWjHWSagqNdVsVSJK4A==
-----END CERTIFICATE-----`)

var testClientKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC1FB39kw4XWCGA
4yRcCpyLD0sFJADwzoCfkxxU1jjIRQTQ5BioeMWFLpqGA02ds0w4YhxvZ3pIPIyJ
oVNPbtvgZDrNQdYEpxNPiva5Mq6/C09uhyHcGGOaL+TxjU211MHtyF5hZ3J6Um6j
v5Lc+Ztw9E5JoH/K9RB1ZhvasGRFOLF3J5ENy9LH9sfvyct/ILii3rgeWYR9gU64
BB8+h1oUA35eAnuM1oq6GJweg+GsntIXL7my2sruxQQgE6bpXfMP9IQ1G6QXZYbW
BmZzQJCkOQzcEV6ZM2+uJ+SyhwpaOLR7kxakI17lAJT5aqnpGopPKAyRFeX2dDez
qAm57z6RAgMBAAECggEAAhIVPLYph1akksTQ1d+bhiiPhvhwLVDDM5qa9z+4MwFn
tRwinwyQf6h2hQ6fx4GfycDvdSOrCDfvCHozNG7pWJcffVjhzGL9C3VkmF4OFapY
scQdp5a8ztbPiKaWa1Gf7RWT5LZqM/ViMAEBD4H9hyINYiCnIshAya2Npyd0t2jn
XtvJfRVHU6j1xLKb9Mi67IqiOvgu68eqCC6Iu6pORSq2dBogUnVCDqigvS8/74ao
5IuvoFan1jd3Imr7HWEDiQ9yMmjo4n8DVXmJb4gNZ9QVDGk+r1VXNVZFubp16xuP
rbpbl2jRlOBfcq3ZAZ557/dc/nPkX1wUQBhKSsawlQKBgQDlV4Iv29AH/zQmxOyB
YZI96ExDhvVK7yRgepidNH/rphS3iRpjTnOVB+8rgRva4dWdpr5BWAPVKlh7T5h/
yBivIOtIwIbLOombnUJyLYWjIsQL8+MeDqNizSurQs0VckBfMePaW/gzgOLptWtU
tGmM1Ki9CP6gMmxRln1Mlruk1QKBgQDKIHI4cm0QHw7xybFetGw4TxADmciiqtQ2
gyP42PfEsUpel+dosJ/CV5p1bs92JQVaoM88vMhEsRsV4LIyMcQHIdrxb0AO8/Ow
hx8FW4Levmh95/Apg3c93bfa0DFoxTDa+2OTeBvM9SsxlOmMq4uYIOyUOjd/3Y2u
xbtx0htAzQKBgQCDBaBxuRG7T9g6gexf6h9DUPAo7/Q5EDBnEgMYZMLkHKjfReuW
al5r+PFxmDwSq0x/2Z/98suVv7B3Gj0UW3uGqbbhhGQ9vL6a8ZfhZRJg5d68uWO6
a0B6lJ5rJCnII9KU0ArNWBePTQXV4PhllwBqHaAdBwN4//WUEvaYh9DB1QKBgQCa
Eb9e3YHarwHyNb54pOh0x3c6d2di7voRj0bFMYUzLby1e+6Nc0xjk+kNqGiE8tUw
7rDo6DFzgthVhc/uyNZWZW0Bab6XZ0aSgXyY1ddcuCDoD/qVejtTMgUpylZPOTfz
Q3n0d7IhOaQyCAM6Eay3SilrFzEkyxlrZhdqPDA/5QKBgDFeL019De6talEsJeHv
WM++CHGGbOI32dvxxDVv3PxcU/r61enMcbhkwUd0qJgwQn42hweK0ckHdGVdeAvr
gevHKRvAYepUlR4PzhEJQe0QeWr/q0TANoxnMCxHofSyovL9HIa4yrMjtFHZTHcb
zJwtsqamfHeVw25siB81kRmQ
-----END PRIVATE KEY-----`)

var testClientCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDojCCAoqgAwIBAgIUZ5EwWWnN25m+D89JdWzUYS1mhm8wDQYJKoZIhvcNAQEL
BQAwNDEyMDAGA1UEAwwpZ2VuZXJpY193ZWJob29rX2FkbWlzc2lvbl9wbHVnaW5f
dGVzdHNfY2EwIBcNMjYwMTA4MDgzNTE4WhgPMjI5OTEwMjQwODM1MThaMDgxNjA0
BgNVBAMMLWdlbmVyaWNfd2ViaG9va19hZG1pc3Npb25fcGx1Z2luX3Rlc3RzX2Ns
aWVudDCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALUUHf2TDhdYIYDj
JFwKnIsPSwUkAPDOgJ+THFTWOMhFBNDkGKh4xYUumoYDTZ2zTDhiHG9nekg8jImh
U09u2+BkOs1B1gSnE0+K9rkyrr8LT26HIdwYY5ov5PGNTbXUwe3IXmFncnpSbqO/
ktz5m3D0Tkmgf8r1EHVmG9qwZEU4sXcnkQ3L0sf2x+/Jy38guKLeuB5ZhH2BTrgE
Hz6HWhQDfl4Ce4zWiroYnB6D4aye0hcvubLayu7FBCATpuld8w/0hDUbpBdlhtYG
ZnNAkKQ5DNwRXpkzb64n5LKHClo4tHuTFqQjXuUAlPlqqekaik8oDJEV5fZ0N7Oo
CbnvPpECAwEAAaOBpTCBojAJBgNVHRMEAjAAMAsGA1UdDwQEAwIF4DAdBgNVHSUE
FjAUBggrBgEFBQcDAgYIKwYBBQUHAwEwKQYDVR0RBCIwIIcEfwAAAYIYd2ViaG9v
ay10ZXN0LmRlZmF1bHQuc3ZjMB0GA1UdDgQWBBQ5VoPKkOz4BYxMkse1cEkFcq0n
jDAfBgNVHSMEGDAWgBRq2B1pTppmoh/DIf7BUmi/rPfyRjANBgkqhkiG9w0BAQsF
AAOCAQEAII227bpDLQpfOvPNA3dNKJodqjHY+iyMRaifRU3syLR3ez3+eqb66dtq
r89VhkRhBjki5aLrnP9baS48A47s/14/rVtKshzCy5PXjce8DuXEBLJH6yMTXogb
JTkc96AzktJHMqef2WjMGyIDMpyaAf8ebYZPWXBIEMcAqH05svbH6RPQUPfLbD1v
7KMKMpRtJ4bMw7gk3rPouApJ0Af/wvCu5gnLG3yZ0Pmni/ZJySrI0f4xt6wEq4F1
iUgf+qDPz62c/QeC9HlNb15pn7YRRcOYry864zQb/ZFtqPj276KIkrGqbhft05D+
MW5osM41axoAD70MpoMXdSRcAR7fUg==
-----END CERTIFICATE-----`)
