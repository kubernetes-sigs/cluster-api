/*
Copyright 2021 The Kubernetes Authors.

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

	"sigs.k8s.io/cluster-api/internal/runtime/catalog"
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
			name:     "all nil, err",
			request:  nil,
			response: nil,
			opts:     nil,
			wantErr:  true,
		},
		{
			name:     "nil catalog, err",
			request:  &fakev1alpha1.FakeRequest{},
			response: &fakev1alpha1.FakeResponse{},
			opts: &httpCallOptions{
				catalog: nil,
			},
			wantErr: true,
		},
		{
			name:     "empty catalog, err",
			request:  &fakev1alpha1.FakeRequest{},
			response: &fakev1alpha1.FakeResponse{},
			opts: &httpCallOptions{
				catalog: catalog.New(),
			},
			wantErr: true,
		},
		{
			name: "ok, no conversion",
			request: &fakev1alpha1.FakeRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "FakeRequest",
					APIVersion: fakev1alpha1.GroupVersion.Identifier(),
				},
			},
			response: &fakev1alpha1.FakeResponse{},
			opts: func() *httpCallOptions {
				c := catalog.New()
				c.AddHook(
					fakev1alpha1.GroupVersion,
					fakev1alpha1.FakeHook,
					&catalog.HookMeta{},
				)

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
			name: "ok, conversion",
			request: &fakev1alpha2.FakeRequest{
				TypeMeta: metav1.TypeMeta{
					Kind:       "FakeRequest",
					APIVersion: fakev1alpha2.GroupVersion.Identifier(),
				},
			},
			response: &fakev1alpha2.FakeResponse{},
			opts: func() *httpCallOptions {
				c := catalog.New()
				// register fakev1alpha1 to enable conversion
				g.Expect(fakev1alpha1.AddToCatalog(c)).To(Succeed())
				c.AddHook(
					fakev1alpha1.GroupVersion,
					fakev1alpha1.FakeHook,
					&catalog.HookMeta{},
				)

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

			assert := g.Expect(httpCall(context.TODO(), tt.request, tt.response, tt.opts))
			if tt.wantErr {
				assert.To(HaveOccurred())
			} else {
				assert.To(Succeed())
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
