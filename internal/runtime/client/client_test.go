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
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	fakev1alpha1 "sigs.k8s.io/cluster-api/internal/runtime/catalog/test/v1alpha1"
	fakev1alpha2 "sigs.k8s.io/cluster-api/internal/runtime/catalog/test/v1alpha2"
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
			name:     "should error if request, response and options are nil",
			request:  nil,
			response: nil,
			opts:     nil,
			wantErr:  true,
		},
		{
			name:     "should error if catalog is not set",
			request:  &fakev1alpha1.FakeRequest{},
			response: &fakev1alpha1.FakeResponse{},
			opts: &httpCallOptions{
				catalog: nil,
			},
			wantErr: true,
		},
		{
			name:     "should error if hooks is not registered with catalog",
			request:  &fakev1alpha1.FakeRequest{},
			response: &fakev1alpha1.FakeResponse{},
			opts: &httpCallOptions{
				catalog: runtimecatalog.New(),
			},
			wantErr: true,
		},
		{
			name: "should succeed for valid request and response objects",
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
					catalog: c,
					gvh:     gvh,
				}
			}(),
			wantErr: false,
		},
		{
			name: "should success if response and response are valid objects - with conversion",
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
				gvh, err := c.GroupVersionHook(fakev1alpha1.FakeHook)
				g.Expect(err).To(Succeed())

				return &httpCallOptions{
					catalog: c,
					gvh:     gvh,
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
