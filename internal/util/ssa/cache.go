/*
Copyright 2023 The Kubernetes Authors.

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

package ssa

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"sigs.k8s.io/cluster-api/internal/util/hash"
)

const (
	// ttl is the duration for which we keep the keys in the cache.
	ttl = 10 * time.Minute

	// expirationInterval is the interval in which we will remove expired keys
	// from the cache.
	expirationInterval = 10 * time.Hour
)

// Cache caches SSA request results.
// Specifically we only use it to cache that a certain request
// doesn't have to be repeated anymore because there was no diff.
type Cache interface {
	// Add adds the given key to the Cache.
	// Note: keys expire after the ttl.
	Add(key string)

	// Has checks if the given key (still) exists in the Cache.
	// Note: keys expire after the ttl.
	Has(key string) bool
}

// NewCache creates a new cache.
func NewCache() Cache {
	r := &ssaCache{
		Store: cache.NewTTLStore(func(obj interface{}) (string, error) {
			// We only add strings to the cache, so it's safe to cast to string.
			return obj.(string), nil
		}, ttl),
	}
	go func() {
		for {
			// Call list to clear the cache of expired items.
			// We have to do this periodically as the cache itself only expires
			// items lazily. If we don't do this the cache grows indefinitely.
			r.List()

			time.Sleep(expirationInterval)
		}
	}()
	return r
}

type ssaCache struct {
	cache.Store
}

// Add adds the given key to the Cache.
// Note: keys expire after the ttl.
func (r *ssaCache) Add(key string) {
	// Note: We can ignore the error here because by only allowing strings
	// and providing the corresponding keyFunc ourselves we can guarantee that
	// the error never occurs.
	_ = r.Store.Add(key)
}

// Has checks if the given key (still) exists in the Cache.
// Note: keys expire after the ttl.
func (r *ssaCache) Has(key string) bool {
	// Note: We can ignore the error here because GetByKey never returns an error.
	_, exists, _ := r.Store.GetByKey(key)
	return exists
}

// ComputeRequestIdentifier computes a request identifier for the cache.
// The identifier is unique for a specific request to ensure we don't have to re-run the request
// once we found out that it would not produce a diff.
// The identifier consists of: gvk, namespace, name and resourceVersion of the original object and a hash of the modified
// object. This ensures that we re-run the request as soon as either original or modified changes.
func ComputeRequestIdentifier(scheme *runtime.Scheme, original, modified client.Object) (string, error) {
	modifiedObjectHash, err := hash.Compute(modified)
	if err != nil {
		return "", errors.Wrapf(err, "failed to calculate request identifier: failed to compute hash for modified object")
	}

	gvk, err := apiutil.GVKForObject(original, scheme)
	if err != nil {
		return "", errors.Wrapf(err, "failed to calculate request identifier: failed to get GroupVersionKind of original object %s", klog.KObj(original))
	}

	return fmt.Sprintf("%s.%s.%s.%d", gvk.String(), klog.KObj(original), original.GetResourceVersion(), modifiedObjectHash), nil
}
