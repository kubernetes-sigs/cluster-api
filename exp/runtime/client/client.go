/*
Copyright 2024 The Kubernetes Authors.

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

// Package client provides the Runtime SDK client.
package client

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	runtimev1 "sigs.k8s.io/cluster-api/exp/runtime/api/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimehooksv1 "sigs.k8s.io/cluster-api/exp/runtime/hooks/api/v1alpha1"
	"sigs.k8s.io/cluster-api/util/cache"
)

// CallExtensionOption is the interface for configuration that modifies CallExtensionOptions for a CallExtension call.
type CallExtensionOption interface {
	// ApplyToOptions applies this configuration to the given CallExtensionOptions.
	ApplyToOptions(*CallExtensionOptions)
}

// CallExtensionCacheEntry is a cache entry for the cache that can be used with the CallExtension call via
// the WithCaching option.
type CallExtensionCacheEntry struct {
	CacheKey string
	Response runtimehooksv1.ResponseObject
}

// Key returns the cache key of a CallExtensionCacheEntry.
func (c CallExtensionCacheEntry) Key() string {
	return c.CacheKey
}

// WithCaching enables caching for the CallExtension call.
type WithCaching struct {
	Cache        cache.Cache[CallExtensionCacheEntry]
	CacheKeyFunc func(extensionName, extensionConfigResourceVersion string, request runtimehooksv1.RequestObject) string
}

// ApplyToOptions applies WithCaching to the given CallExtensionOptions.
func (w WithCaching) ApplyToOptions(in *CallExtensionOptions) {
	in.WithCaching = true
	in.Cache = w.Cache
	in.CacheKeyFunc = w.CacheKeyFunc
}

// CallExtensionOptions contains the options for the CallExtension call.
type CallExtensionOptions struct {
	WithCaching  bool
	Cache        cache.Cache[CallExtensionCacheEntry]
	CacheKeyFunc func(extensionName, extensionConfigResourceVersion string, request runtimehooksv1.RequestObject) string
}

// Client is the runtime client to interact with extensions.
type Client interface {
	// WarmUp can be used to initialize a "cold" RuntimeClient with all
	// known runtimev1.ExtensionConfigs at a given time.
	// After WarmUp completes the RuntimeClient is considered ready.
	WarmUp(extensionConfigList *runtimev1.ExtensionConfigList) error

	// IsReady return true after the RuntimeClient finishes warmup.
	IsReady() bool

	// Discover makes the discovery call on the extension and returns an updated ExtensionConfig
	// with extension handlers information in the ExtensionConfig status.
	Discover(context.Context, *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error)

	// Register registers the ExtensionConfig.
	Register(extensionConfig *runtimev1.ExtensionConfig) error

	// Unregister unregisters the ExtensionConfig.
	Unregister(extensionConfig *runtimev1.ExtensionConfig) error

	// CallAllExtensions calls all the ExtensionHandler registered for the hook.
	CallAllExtensions(ctx context.Context, hook runtimecatalog.Hook, forObject metav1.Object, request runtimehooksv1.RequestObject, response runtimehooksv1.ResponseObject) error

	// CallExtension calls the ExtensionHandler with the given name.
	CallExtension(ctx context.Context, hook runtimecatalog.Hook, forObject metav1.Object, name string, request runtimehooksv1.RequestObject, response runtimehooksv1.ResponseObject, opts ...CallExtensionOption) error
}
