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

package drain

import (
	"time"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

const (
	// ttl is the duration for which we keep entries in the cache.
	ttl = 10 * time.Minute

	// expirationInterval is the interval in which we will remove expired entries
	// from the cache.
	expirationInterval = 10 * time.Hour
)

// CacheEntry is an entry of the drain cache. It stores at which time a Machine was drained the last time.
type CacheEntry struct {
	Machine   types.NamespacedName
	LastDrain time.Time
}

// Cache caches the time when the last drain was done for a Machine.
// Specifically we only use it to ensure we only retry drains
// at a specific interval and not more often.
type Cache interface {
	// Add adds the given entry to the Cache.
	// Note: entries expire after the ttl.
	Add(entry CacheEntry)

	// Has checks if the given key (still) exists in the Cache.
	// Note: entries expire after the ttl.
	Has(machineName types.NamespacedName) (CacheEntry, bool)
}

// NewCache creates a new cache.
func NewCache() Cache {
	r := &drainCache{
		Store: cache.NewTTLStore(func(obj interface{}) (string, error) {
			// We only add CacheEntries to the cache, so it's safe to cast to CacheEntry.
			return obj.(CacheEntry).Machine.String(), nil
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

type drainCache struct {
	cache.Store
}

// Add adds the given entry to the Cache.
// Note: entries expire after the ttl.
func (r *drainCache) Add(entry CacheEntry) {
	// Note: We can ignore the error here because by only allowing CacheEntries
	// and providing the corresponding keyFunc ourselves we can guarantee that
	// the error never occurs.
	_ = r.Store.Add(entry)
}

// Has checks if the given key (still) exists in the Cache.
// Note: entries expire after the ttl.
func (r *drainCache) Has(machineName types.NamespacedName) (CacheEntry, bool) {
	// Note: We can ignore the error here because GetByKey never returns an error.
	item, exists, _ := r.Store.GetByKey(machineName.String())
	if exists {
		return item.(CacheEntry), true
	}
	return CacheEntry{}, false
}
