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

// Package test contains catalog tests
// Note: They have to be outside the catalog package to be realistic. Otherwise using
// test types with different versions would result in a cyclic dependency and thus
// wouldn't be possible.
package test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	runtimecatalog "sigs.k8s.io/cluster-api/internal/runtime/catalog"
	"sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha1"
	"sigs.k8s.io/cluster-api/internal/runtime/test/v1alpha2"
)

var c = runtimecatalog.New()

func init() {
	_ = v1alpha1.AddToCatalog(c)
	_ = v1alpha2.AddToCatalog(c)
}

func TestCatalog(t *testing.T) {
	g := NewWithT(t)

	verify := func(hook runtimecatalog.Hook, expectedGV schema.GroupVersion) {
		// Test GroupVersionHook
		hookGVH, err := c.GroupVersionHook(hook)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(hookGVH.GroupVersion()).To(Equal(expectedGV))
		g.Expect(hookGVH.Hook).To(Equal("FakeHook"))

		// Test Request
		requestGVK, err := c.Request(hookGVH)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requestGVK.GroupVersion()).To(Equal(expectedGV))
		g.Expect(requestGVK.Kind).To(Equal("FakeRequest"))

		// Test Response
		responseGVK, err := c.Response(hookGVH)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(responseGVK.GroupVersion()).To(Equal(expectedGV))
		g.Expect(responseGVK.Kind).To(Equal("FakeResponse"))

		// Test NewRequest
		request, err := c.NewRequest(hookGVH)
		g.Expect(err).ToNot(HaveOccurred())

		// Test NewResponse
		response, err := c.NewResponse(hookGVH)
		g.Expect(err).ToNot(HaveOccurred())

		// Test ValidateRequest/ValidateResponse
		g.Expect(c.ValidateRequest(hookGVH, request)).To(Succeed())
		g.Expect(c.ValidateResponse(hookGVH, response)).To(Succeed())
	}

	verify(v1alpha1.FakeHook, v1alpha1.GroupVersion)
	verify(v1alpha2.FakeHook, v1alpha2.GroupVersion)
}

func TestValidateRequest(t *testing.T) {
	v1alpha1Hook, err := c.GroupVersionHook(v1alpha1.FakeHook)
	if err != nil {
		panic("failed to get GVH of hook")
	}
	v1alpha1HookRequest, err := c.NewRequest(v1alpha1Hook)
	if err != nil {
		panic("failed to create request for hook")
	}

	v1alpha2Hook, err := c.GroupVersionHook(v1alpha2.FakeHook)
	if err != nil {
		panic("failed to get GVH of hook")
	}

	tests := []struct {
		name      string
		hook      runtimecatalog.GroupVersionHook
		request   runtime.Object
		wantError bool
	}{
		{
			name:      "should succeed when hook and request match",
			hook:      v1alpha1Hook,
			request:   v1alpha1HookRequest,
			wantError: false,
		},
		{
			name:      "should error when hook and request do not match",
			hook:      v1alpha2Hook,
			request:   v1alpha1HookRequest,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := c.ValidateRequest(tt.hook, tt.request)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestValidateResponse(t *testing.T) {
	v1alpha1Hook, err := c.GroupVersionHook(v1alpha1.FakeHook)
	if err != nil {
		panic("failed to get GVH of hook")
	}
	v1alpha1HookResponse, err := c.NewResponse(v1alpha1Hook)
	if err != nil {
		panic("failed to create request for hook")
	}

	v1alpha2Hook, err := c.GroupVersionHook(v1alpha2.FakeHook)
	if err != nil {
		panic("failed to get GVH of hook")
	}

	tests := []struct {
		name      string
		hook      runtimecatalog.GroupVersionHook
		response  runtime.Object
		wantError bool
	}{
		{
			name:      "should succeed when hook and response match",
			hook:      v1alpha1Hook,
			response:  v1alpha1HookResponse,
			wantError: false,
		},
		{
			name:      "should error when hook and response do not match",
			hook:      v1alpha2Hook,
			response:  v1alpha1HookResponse,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := c.ValidateResponse(tt.hook, tt.response)
			if tt.wantError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

type GoodRequest struct {
	metav1.TypeMeta `json:",inline"`

	First string `json:"first"`
}

func (in *GoodRequest) DeepCopyObject() runtime.Object {
	panic("implement me!")
}

// BadRequest does not implement runtime.Object interface (missing DeepCopyObject function).
type BadRequest struct {
	metav1.TypeMeta `json:",inline"`

	First string `json:"first"`
}

type GoodResponse struct {
	metav1.TypeMeta `json:",inline"`

	First string `json:"first"`
}

func (out *GoodResponse) DeepCopyObject() runtime.Object {
	panic("implement me!")
}

// BadResponse does not implement runtime.Object interface (missing DeepCopyObject function).
type BadResponse struct {
	metav1.TypeMeta `json:",inline"`

	First string `json:"first"`
}

func GoodHook(*GoodRequest, *GoodResponse) {}

func HookWithReturn(*GoodRequest) *GoodResponse { return nil }

func HookWithNoInputs() {}

func HookWithThreeInputs(*GoodRequest, *GoodRequest, *GoodResponse) {}

func HookWithBadRequestAndResponse(*BadRequest, *BadResponse) {}

func TestAddHook(t *testing.T) {
	c := runtimecatalog.New()

	tests := []struct {
		name      string
		hook      runtimecatalog.Hook
		hookMeta  *runtimecatalog.HookMeta
		wantPanic bool
	}{
		{
			name:      "should pass for valid hook",
			hook:      GoodHook,
			hookMeta:  &runtimecatalog.HookMeta{},
			wantPanic: false,
		},
		{
			name:      "should fail for hook with a return value",
			hook:      HookWithReturn,
			hookMeta:  &runtimecatalog.HookMeta{},
			wantPanic: true,
		},
		{
			name:      "should fail for a hook with no inputs",
			hook:      HookWithNoInputs,
			hookMeta:  &runtimecatalog.HookMeta{},
			wantPanic: true,
		},
		{
			name:      "should fail for a hook with more than two arguments",
			hook:      HookWithThreeInputs,
			hookMeta:  &runtimecatalog.HookMeta{},
			wantPanic: true,
		},
		{
			name:      "should fail for hook with bad request and response arguments",
			hook:      HookWithBadRequestAndResponse,
			hookMeta:  &runtimecatalog.HookMeta{},
			wantPanic: true,
		},
		{
			name:      "should fail if the hookMeta is nil",
			hook:      GoodHook,
			hookMeta:  nil,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			testFunc := func() {
				c.AddHook(v1alpha1.GroupVersion, tt.hook, tt.hookMeta)
			}
			if tt.wantPanic {
				g.Expect(testFunc).Should(Panic())
			} else {
				g.Expect(testFunc).ShouldNot(Panic())
			}
		})
	}
}
