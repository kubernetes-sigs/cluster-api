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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	runtimeregistry "sigs.k8s.io/cluster-api/internal/runtime/registry"
	fakev1alpha1 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha1"
	fakev1alpha2 "sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha2"
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
		t.Run(tt.name, func(t *testing.T) {
			// a http server is only required if we have a valid catalog, otherwise httpCall will not reach out to the server
			if tt.opts != nil && tt.opts.catalog != nil {
				// create http server with fakeHookHandler
				mux := http.NewServeMux()
				mux.HandleFunc("/", fakeHookHandler)
				srv := httptest.NewServer(mux)
				defer srv.Close()

				// set url to srv for in tt.opts
				tt.opts.config.URL = pointer.String(srv.URL)
			}

			err := httpCall(context.TODO(), tt.request, tt.response, tt.opts)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func fakeHookHandler(w http.ResponseWriter, r *http.Request) {
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
					Service: &runtimev1.ServiceReference{
						Namespace: "test1",
						Name:      "extension-service",
						Port:      pointer.Int32(8443),
					},
				},
				gvh:                  gvh,
				extensionHandlerName: "test-handler",
			},
			want: want{
				scheme: "http",
				host:   "extension-service.test1.svc:8443",
				path:   runtimecatalog.GVHToPath(gvh, "test-handler"),
			},
			wantErr: false,
		},
		{
			name: "ClientConfig using service and CAbundle should have correct URL values",
			args: args{
				config: runtimev1.ClientConfig{
					Service: &runtimev1.ServiceReference{
						Namespace: "test1",
						Name:      "extension-service",
						Port:      pointer.Int32(8443),
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
					URL: pointer.String("https://extension-host.com"),
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
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(u.Scheme).To(Equal(tt.want.scheme))
				g.Expect(u.Host).To(Equal(tt.want.host))
				g.Expect(u.Path).To(Equal(tt.want.path))
			}
		})
	}
}

func TestClient_CallExtension(t *testing.T) {
	testHostPort := "127.0.0.1:9090"

	fpFail := runtimev1.FailurePolicyFail
	fpIgnore := runtimev1.FailurePolicyIgnore

	validExtensionHandlerWithFailPolicy := runtimev1.ExtensionConfig{
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String(fmt.Sprintf("http://%s/", testHostPort)),
			},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "valid-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: pointer.Int32Ptr(1),
					FailurePolicy:  &fpFail,
				},
			},
		},
	}
	validExtensionHandlerWithIgnorePolicy := runtimev1.ExtensionConfig{
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String(fmt.Sprintf("http://%s/", testHostPort)),
			},
		},
		Status: runtimev1.ExtensionConfigStatus{
			Handlers: []runtimev1.ExtensionHandler{
				{
					Name: "valid-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: pointer.Int32Ptr(1),
					FailurePolicy:  &fpIgnore,
				},
			},
		},
	}

	type args struct {
		hook     runtimecatalog.Hook
		name     string
		request  runtime.Object
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
			wantErr: true,
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
			wantErr: true,
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
			wantErr: true,
		},
		{
			name:                       "should succeed when calling ExtensionHandler with success response and FailurePolicyFail",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusSuccess),
				},
			}, args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: false,
		},
		{
			name:                       "should succeed when calling ExtensionHandler with success response and FailurePolicyIgnore",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithIgnorePolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusSuccess),
				},
			}, args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: false,
		},
		{
			name:                       "should fail when calling ExtensionHandler with failure response and FailurePolicyFail",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusFailure),
				},
			}, args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: true,
		},
		{
			name:                       "should fail when calling ExtensionHandler with failure response and FailurePolicyIgnore",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithIgnorePolicy},
			testServer: testServerConfig{
				start: true,
				responses: map[string]testServerResponse{
					"/*": response(runtimehooksv1.ResponseStatusFailure),
				},
			}, args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: true,
		},

		{
			name:                       "should succeed with unreachable extension and FailurePolicyIgnore",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithIgnorePolicy},
			testServer: testServerConfig{
				start: false,
			}, args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
				request:  &fakev1alpha1.FakeRequest{},
				response: &fakev1alpha1.FakeResponse{},
			},
			wantErr: false,
		},
		{
			name:                       "should fail with unreachable extension and FailurePolicyFail",
			registeredExtensionConfigs: []runtimev1.ExtensionConfig{validExtensionHandlerWithFailPolicy},
			testServer: testServerConfig{
				start: false,
			}, args: args{
				hook:     fakev1alpha1.FakeHook,
				name:     "valid-extension",
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
				if tt.testServer.hostPort == "" {
					tt.testServer.hostPort = testHostPort
				}
				srv := createTestServer(g, tt.testServer)
				srv.Start()
				defer srv.Close()
			}

			cat := runtimecatalog.New()
			_ = fakev1alpha1.AddToCatalog(cat)
			_ = fakev1alpha2.AddToCatalog(cat)

			c := New(Options{
				Catalog:  cat,
				Registry: registry(tt.registeredExtensionConfigs),
			})

			err := c.CallExtension(context.Background(), tt.args.hook, tt.args.name, tt.args.request, tt.args.response)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestClient_CallAllExtensions(t *testing.T) {
	testHostPort := "127.0.0.1:9090"

	fpFail := runtimev1.FailurePolicyFail

	extensionConfig := runtimev1.ExtensionConfig{
		Spec: runtimev1.ExtensionConfigSpec{
			ClientConfig: runtimev1.ClientConfig{
				URL: pointer.String(fmt.Sprintf("http://%s/", testHostPort)),
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
					TimeoutSeconds: pointer.Int32Ptr(1),
					FailurePolicy:  &fpFail,
				},
				{
					Name: "second-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: pointer.Int32Ptr(1),
					FailurePolicy:  &fpFail,
				},
				{
					Name: "third-extension",
					RequestHook: runtimev1.GroupVersionHook{
						APIVersion: fakev1alpha1.GroupVersion.String(),
						Hook:       "FakeHook",
					},
					TimeoutSeconds: pointer.Int32Ptr(1),
					FailurePolicy:  &fpFail,
				},
			},
		},
	}

	type args struct {
		hook     runtimecatalog.Hook
		request  runtime.Object
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
				if tt.testServer.hostPort == "" {
					tt.testServer.hostPort = testHostPort
				}
				srv := createTestServer(g, tt.testServer)
				srv.Start()
				defer srv.Close()
			}

			cat := runtimecatalog.New()
			_ = fakev1alpha1.AddToCatalog(cat)
			_ = fakev1alpha2.AddToCatalog(cat)

			c := New(Options{
				Catalog:  cat,
				Registry: registry(tt.registeredExtensionConfigs),
			})

			err := c.CallAllExtensions(context.Background(), tt.args.hook, tt.args.request, tt.args.response)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
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
			aggregateResponse: fakeSuccessResponse(),
			responses: []runtimehooksv1.ResponseObject{
				fakeSuccessResponse(),
			},
			want: fakeSuccessResponse(),
		},
		{
			name:              "Aggregate retry response if there is only one response",
			aggregateResponse: fakeRetryableSuccessResponse(0),
			responses: []runtimehooksv1.ResponseObject{
				fakeRetryableSuccessResponse(5),
			},
			want: fakeRetryableSuccessResponse(5),
		},
		{
			name:              "Aggregate retry responses to lowest non-zero retryAfterSeconds value",
			aggregateResponse: fakeRetryableSuccessResponse(0),
			responses: []runtimehooksv1.ResponseObject{
				fakeRetryableSuccessResponse(0),
				fakeRetryableSuccessResponse(1),
				fakeRetryableSuccessResponse(5),
				fakeRetryableSuccessResponse(4),
				fakeRetryableSuccessResponse(3),
			},
			want: fakeRetryableSuccessResponse(1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregateSuccessfulResponses(tt.aggregateResponse, tt.responses)

			if !reflect.DeepEqual(tt.aggregateResponse, tt.want) {
				t.Errorf("aggregateSuccessfulResponses() got = %v, want %v", tt.aggregateResponse, tt.want)
			}
		})
	}
}

type testServerConfig struct {
	start     bool
	responses map[string]testServerResponse
	hostPort  string
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

func createTestServer(g *WithT, server testServerConfig) *httptest.Server {
	l, err := net.Listen("tcp", server.hostPort)
	g.Expect(err).NotTo(HaveOccurred())

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
	srv := httptest.NewUnstartedServer(mux)
	// NewUnstartedServer creates a listener. Close that listener and replace
	// with the one we created.
	g.Expect(srv.Listener.Close()).To(Succeed())
	srv.Listener = l

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

func fakeSuccessResponse() *fakev1alpha1.FakeResponse {
	return &fakev1alpha1.FakeResponse{
		TypeMeta: metav1.TypeMeta{
			Kind:       "FakeResponse",
			APIVersion: "v1alpha1",
		},
		CommonResponse: runtimehooksv1.CommonResponse{
			Message: "",
			Status:  runtimehooksv1.ResponseStatusSuccess,
		},
	}
}

func fakeRetryableSuccessResponse(retryAfterSeconds int32) *fakev1alpha1.RetryableFakeResponse {
	return &fakev1alpha1.RetryableFakeResponse{
		TypeMeta: metav1.TypeMeta{
			Kind:       "FakeResponse",
			APIVersion: "v1alpha1",
		},
		CommonResponse: runtimehooksv1.CommonResponse{
			Message: "",
			Status:  runtimehooksv1.ResponseStatusSuccess,
		},
		CommonRetryResponse: runtimehooksv1.CommonRetryResponse{
			RetryAfterSeconds: retryAfterSeconds,
		},
	}
}
