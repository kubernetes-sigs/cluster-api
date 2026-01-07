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

package cache

import (
	"fmt"
	"time"

	kcache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
)

const (
	// DefaultTTL is the duration for which we keep entries in the cache.
	DefaultTTL = 10 * time.Minute

	// HookCacheDefaultTTL is the duration for which we keep entries in the hook cache.
	HookCacheDefaultTTL = 24 * time.Hour

	// expirationInterval is the interval in which we will remove expired entries
	// from the cache.
	expirationInterval = 10 * time.Hour
)

// Entry is the interface for the type of Cache entries.
type Entry interface {
	// Key returns the cache key of an Entry.
	Key() string
}

// Cache caches objects of type Entry.
type Cache[E Entry] interface {
	// Add adds the given entry to the Cache.
	// Note: entries expire after the ttl.
	Add(entry E)

	// Has checks if the given key (still) exists in the Cache.
	// Note: entries expire after the ttl.
	Has(key string) (E, bool)

	// Len returns the number of entries in the cache.
	Len() int

	// DeleteAll deletes all entries from the cache.
	DeleteAll()
}

// New creates a new cache.
// ttl is the duration for which we keep entries in the cache.
func New[E Entry](ttl time.Duration) Cache[E] {
	r := &cache[E]{
		Store: kcache.NewTTLStore(func(obj interface{}) (string, error) {
			// We only add objects of type E to the cache, so it's safe to cast to E.
			return obj.(E).Key(), nil
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

type cache[E Entry] struct {
	kcache.Store
}

// Add adds the given entry to the Cache.
// Note: entries expire after the ttl.
func (r *cache[E]) Add(entry E) {
	// Note: We can ignore the error here because the key func we pass into NewTTLStore never
	// returns errors.
	_ = r.Store.Add(entry)
}

// Has checks if the given key (still) exists in the Cache.
// Note: entries expire after the ttl.
func (r *cache[E]) Has(key string) (E, bool) {
	// Note: We can ignore the error here because GetByKey never returns an error.
	item, exists, _ := r.GetByKey(key)
	if exists {
		return item.(E), true
	}
	return *new(E), false
}

func (r *cache[E]) Len() int {
	return len(r.ListKeys())
}

func (r *cache[E]) DeleteAll() {
	// Note: We are intentionally using Replace instead of List + Delete because the latter would be racy.
	_ = r.Store.Replace(nil, "")
}

// HookEntry is an Entry for the hook Cache.
type HookEntry struct {
	ObjectKey       client.ObjectKey
	HookName        string
	ReconcileAfter  time.Time
	ResponseMessage string
}

// NewHookEntry creates a new HookEntry based on an object and a reconcileAfter time.
func NewHookEntry(obj client.Object, hook runtimecatalog.Hook, reconcileAfter time.Time, responseMessage string) HookEntry {
	return HookEntry{
		ObjectKey:       client.ObjectKeyFromObject(obj),
		HookName:        runtimecatalog.HookName(hook),
		ReconcileAfter:  reconcileAfter,
		ResponseMessage: responseMessage,
	}
}

// NewHookEntryKey returns the key of a HookEntry based on an object.
func NewHookEntryKey(obj client.Object, hook runtimecatalog.Hook) string {
	return HookEntry{
		ObjectKey: client.ObjectKeyFromObject(obj),
		HookName:  runtimecatalog.HookName(hook),
	}.Key()
}

var _ Entry = &HookEntry{}

// Key returns the cache key of a HookEntry.
func (r HookEntry) Key() string {
	return fmt.Sprintf("%s: %s", r.HookName, r.ObjectKey.String())
}

// ShouldRequeue returns if the current Reconcile should be requeued.
func (r HookEntry) ShouldRequeue(now time.Time) (requeueAfter time.Duration, requeue bool) {
	if r.ReconcileAfter.IsZero() {
		return time.Duration(0), false
	}

	if r.ReconcileAfter.After(now) {
		return r.ReconcileAfter.Sub(now), true
	}

	return time.Duration(0), false
}

// ToResponse uses the values from a HookEntry to set status, message and retryAfterSeconds on a RetryResponseObject.
// requeueAfter is passed in to make it possible to use durations computed from previous ShouldRequeue calls.
func (r HookEntry) ToResponse(resp runtimehooksv1.RetryResponseObject, requeueAfter time.Duration) runtimehooksv1.RetryResponseObject {
	resp.SetStatus(runtimehooksv1.ResponseStatusSuccess)
	resp.SetMessage(r.ResponseMessage)
	if requeueAfter > 0 {
		// Note: We have to set at least 1 if requeueAfter > 0 otherwise components like
		// the HookResponseTracker won't detect this correctly as a blocking response.
		resp.SetRetryAfterSeconds(max(int32(requeueAfter.Round(time.Second).Seconds()), 1))
	}
	return resp
}
