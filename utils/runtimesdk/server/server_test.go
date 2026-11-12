/*
Copyright The Kubernetes Authors.

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

package server

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	runtimecatalog "sigs.k8s.io/cluster-api/api/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
)

func TestCallHandlerRequestBodyLimit(t *testing.T) {
	t.Run("oversized body is rejected without invoking the handler", func(t *testing.T) {
		g := NewWithT(t)
		s, reached := newTestServerWithDiscoveryHandler(g)

		// Valid JSON followed by whitespace, exceeding the limit.
		body := "{}" + strings.Repeat(" ", maxExtensionRequestBodyBytes+1)
		req := httptest.NewRequestWithContext(t.Context(), "POST", "/test", strings.NewReader(body))

		resp := s.callHandler(firstHandler(s), req)

		g.Expect(*reached).To(BeFalse())
		g.Expect(resp.GetStatus()).To(Equal(runtimehooksv1.ResponseStatusFailure))
		g.Expect(resp.GetMessage()).To(ContainSubstring("error reading request"))
	})

	t.Run("normal body is processed", func(t *testing.T) {
		g := NewWithT(t)
		s, reached := newTestServerWithDiscoveryHandler(g)

		req := httptest.NewRequestWithContext(t.Context(), "POST", "/test", strings.NewReader("{}"))

		resp := s.callHandler(firstHandler(s), req)

		g.Expect(*reached).To(BeTrue())
		g.Expect(resp.GetStatus()).To(Equal(runtimehooksv1.ResponseStatusSuccess))
	})
}

func newTestServerWithDiscoveryHandler(g *WithT) (*Server, *bool) {
	catalog := runtimecatalog.New()
	g.Expect(runtimehooksv1.AddToCatalog(catalog)).To(Succeed())

	s, err := New(Options{Catalog: catalog})
	g.Expect(err).ToNot(HaveOccurred())

	reached := false
	g.Expect(s.AddExtensionHandler(ExtensionHandler{
		Hook: runtimehooksv1.Discovery,
		Name: "test",
		HandlerFunc: func(_ context.Context, _ *runtimehooksv1.DiscoveryRequest, resp *runtimehooksv1.DiscoveryResponse) {
			reached = true
			resp.SetStatus(runtimehooksv1.ResponseStatusSuccess)
		},
	})).To(Succeed())

	return s, &reached
}

func firstHandler(s *Server) ExtensionHandler {
	for _, h := range s.handlers {
		return h
	}
	return ExtensionHandler{}
}
