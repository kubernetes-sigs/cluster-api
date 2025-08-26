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

// Package fake is used to help with testing functions that need a fake RuntimeClient.
package fake

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
)

// RuntimeClientBuilder is used to build a fake runtime client.
type RuntimeClientBuilder struct {
	ready              bool
	catalog            *runtimecatalog.Catalog
	callAllResponses   map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject
	callAllValidations func(object runtimehooksv1.RequestObject) error
	callResponses      map[string]runtimehooksv1.ResponseObject
	callValidations    func(object runtimehooksv1.RequestObject) error
}

// NewRuntimeClientBuilder returns a new builder for the fake runtime client.
func NewRuntimeClientBuilder() *RuntimeClientBuilder {
	return &RuntimeClientBuilder{}
}

// WithCatalog can be use the provided catalog in the fake runtime client.
func (f *RuntimeClientBuilder) WithCatalog(catalog *runtimecatalog.Catalog) *RuntimeClientBuilder {
	f.catalog = catalog
	return f
}

// WithCallAllExtensionResponses can be used to dictate the responses for CallAllExtensions.
func (f *RuntimeClientBuilder) WithCallAllExtensionResponses(responses map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject) *RuntimeClientBuilder {
	f.callAllResponses = responses
	return f
}

// WithCallAllExtensionValidations can be used to validate the incoming request for CallAllExtensions.
func (f *RuntimeClientBuilder) WithCallAllExtensionValidations(callAllValidations func(object runtimehooksv1.RequestObject) error) *RuntimeClientBuilder {
	f.callAllValidations = callAllValidations
	return f
}

// WithCallExtensionResponses can be used to dictate the responses for CallExtension.
func (f *RuntimeClientBuilder) WithCallExtensionResponses(responses map[string]runtimehooksv1.ResponseObject) *RuntimeClientBuilder {
	f.callResponses = responses
	return f
}

// WithCallExtensionValidations can be used to validate the incoming request for CallExtensions.
func (f *RuntimeClientBuilder) WithCallExtensionValidations(callValidations func(object runtimehooksv1.RequestObject) error) *RuntimeClientBuilder {
	f.callValidations = callValidations
	return f
}

// MarkReady can be used to mark the fake runtime client as either ready or not ready.
func (f *RuntimeClientBuilder) MarkReady(ready bool) *RuntimeClientBuilder {
	f.ready = ready
	return f
}

// Build returns the fake runtime client.
func (f *RuntimeClientBuilder) Build() *RuntimeClient {
	return &RuntimeClient{
		isReady:            f.ready,
		callAllResponses:   f.callAllResponses,
		callAllValidations: f.callAllValidations,
		callResponses:      f.callResponses,
		callValidations:    f.callValidations,
		catalog:            f.catalog,
		callAllTracker:     map[string]int{},
	}
}

var _ runtimeclient.Client = &RuntimeClient{}

// RuntimeClient is a fake implementation of runtimeclient.Client.
type RuntimeClient struct {
	isReady            bool
	catalog            *runtimecatalog.Catalog
	callAllResponses   map[runtimecatalog.GroupVersionHook]runtimehooksv1.ResponseObject
	callAllValidations func(object runtimehooksv1.RequestObject) error
	callResponses      map[string]runtimehooksv1.ResponseObject
	callValidations    func(object runtimehooksv1.RequestObject) error

	callAllTracker map[string]int
}

// CallAllExtensions implements Client.
func (fc *RuntimeClient) CallAllExtensions(ctx context.Context, hook runtimecatalog.Hook, _ metav1.Object, req runtimehooksv1.RequestObject, response runtimehooksv1.ResponseObject) error {
	defer func() {
		fc.callAllTracker[runtimecatalog.HookName(hook)]++
	}()

	gvh, err := fc.catalog.GroupVersionHook(hook)
	if err != nil {
		return errors.Wrap(err, "failed to compute GVH")
	}

	if fc.callAllValidations != nil {
		if err := fc.callAllValidations(req); err != nil {
			return err
		}
	}

	expectedResponse, ok := fc.callAllResponses[gvh]
	if !ok {
		// This should actually panic because an error here would mean a mistake in the test setup.
		panic(fmt.Sprintf("test response not available hook for %q", gvh))
	}

	if err := fc.catalog.Convert(expectedResponse, response, ctx); err != nil {
		// This should actually panic because an error here would mean a mistake in the test setup.
		panic("cannot update response")
	}

	if response.GetStatus() == runtimehooksv1.ResponseStatusFailure {
		return errors.Errorf("runtime hook %q failed", gvh)
	}
	return nil
}

// CallExtension implements Client.
func (fc *RuntimeClient) CallExtension(ctx context.Context, _ runtimecatalog.Hook, _ metav1.Object, name string, req runtimehooksv1.RequestObject, response runtimehooksv1.ResponseObject, _ ...runtimeclient.CallExtensionOption) error {
	if fc.callValidations != nil {
		if err := fc.callValidations(req); err != nil {
			return err
		}
	}

	expectedResponse, ok := fc.callResponses[name]
	if !ok {
		// This should actually panic because an error here would mean a mistake in the test setup.
		panic(fmt.Sprintf("test response not available for extension %q", name))
	}

	if err := fc.catalog.Convert(expectedResponse, response, ctx); err != nil {
		// This should actually panic because an error here would mean a mistake in the test setup.
		panic("cannot update response")
	}

	// If the received response is a failure then return an error.
	if response.GetStatus() == runtimehooksv1.ResponseStatusFailure {
		return errors.Errorf("ExtensionHandler %s failed with message %s", name, response.GetMessage())
	}
	return nil
}

// Discover implements Client.
func (fc *RuntimeClient) Discover(context.Context, *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error) {
	panic("unimplemented")
}

// IsReady implements Client.
func (fc *RuntimeClient) IsReady() bool {
	return fc.isReady
}

// Register implements Client.
func (fc *RuntimeClient) Register(_ *runtimev1.ExtensionConfig) error {
	panic("unimplemented")
}

// Unregister implements Client.
func (fc *RuntimeClient) Unregister(_ *runtimev1.ExtensionConfig) error {
	panic("unimplemented")
}

// WarmUp implements Client.
func (fc *RuntimeClient) WarmUp(_ *runtimev1.ExtensionConfigList) error {
	panic("unimplemented")
}

// CallAllCount return the number of times a hook was called.
func (fc *RuntimeClient) CallAllCount(hook runtimecatalog.Hook) int {
	return fc.callAllTracker[runtimecatalog.HookName(hook)]
}
